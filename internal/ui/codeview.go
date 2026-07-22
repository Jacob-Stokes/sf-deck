package ui

// Code-body viewport rendering — the shared "syntax-highlighted body
// + scrollable viewport + cursor highlight" path used by the Apex
// class detail, Apex trigger detail, and per-file LWC / Aura detail
// renderers. Pulled out of the per-tab files because all four were
// growing the same shape independently.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/highlight"
)

// codeViewSpec is everything renderCodeView needs to draw a scrollable
// code body in a fixed-height region of the main pane.
//
// Body / Lang are the source. Lines is what the highlighter produced
// (one styled line per source line) — the renderer takes both because
// callers usually compute Lines once + reuse for height calculations.
//
// BodyID is the cursor / scroll cache key. Apex passes the class Id;
// trigger passes the trigger Id; LWC passes "<bundleId>:<filePath>".
// Empty BodyID disables persistence — the cursor lives only for the
// current paint and resets on the next call.
//
// Inner is the pane width (no border padding); Height is the budget
// in rows for the body region (gutter + content). The renderer slices
// the line slice into a viewport of Height rows and returns exactly
// that many lines.
//
// Focused signals whether the code body currently owns the j / k
// cursor. When false (action sidebar has focus on a tab that splits
// the two), the cursor highlight dims so the user can tell at a
// glance which surface their next keystroke will steer.
type codeViewSpec struct {
	BodyID  string
	Body    string
	Lang    string
	Inner   int
	Height  int
	Focused bool
}

// renderCodeView returns the viewport-sliced, gutter-prefixed,
// cursor-highlighted lines ready to be appended to the body string.
// Always returns at most spec.Height lines (or fewer if the body is
// shorter); never returns nil. Cursor + scroll state is read from
// (and written back to) the supplied orgData maps when BodyID is set.
func (m Model) renderCodeView(d *orgData, spec codeViewSpec) []string {
	if spec.Height <= 0 {
		return nil
	}
	if spec.Body == "" {
		return []string{theme.Subtle.Render("  (empty)")}
	}
	lines := highlight.Highlight(spec.Body, spec.Lang)
	if len(lines) == 0 {
		lines = strings.Split(spec.Body, "\n")
	}
	total := len(lines)
	gutterW := len(fmt.Sprintf("%d", total))

	// Record what this paint shows so the find / hscroll key handlers
	// act on exactly the on-screen body (gated by tab+subtab match).
	var findState *codeFindState
	var findMatches []codeMatch
	if spec.BodyID != "" {
		d.CodeViewLast = codeViewLastPaint{
			Tab: m.tab(), Sub: m.currentSubtab(),
			BodyID: spec.BodyID, Body: spec.Body,
		}
		findState = codeFindStateFor(d, spec.BodyID, false)
		findMatches = codeFindMatchesFor(findState, spec.Body)
	}
	findVisible := findState != nil && (findState.Active || findState.Buffer != "")
	findBar := ""
	if findVisible {
		// The bar consumes the first row of the body budget.
		findBar = renderCodeFindBar(findState, len(findMatches), spec.Inner)
		spec.Height--
		if spec.Height <= 0 {
			return []string{findBar}
		}
	}
	hscroll := 0
	if spec.BodyID != "" && d.BodyHScroll != nil {
		hscroll = d.BodyHScroll[spec.BodyID]
	}

	// Resolve cursor + scroll. Body-cursor steers the highlight ring
	// AND drives auto-scroll (cursor stays inside the visible window).
	cursor, scroll := 0, 0
	if spec.BodyID != "" {
		if c, ok := d.BodyCursor[spec.BodyID]; ok {
			cursor = c
		}
		if s, ok := d.BodyScroll[spec.BodyID]; ok {
			scroll = s
		}
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= total {
		cursor = total - 1
	}
	// Clamp scroll so the cursor sits in the viewport.
	if cursor < scroll {
		scroll = cursor
	}
	if cursor >= scroll+spec.Height {
		scroll = cursor - spec.Height + 1
	}
	if scroll < 0 {
		scroll = 0
	}
	if scroll > total-spec.Height {
		scroll = total - spec.Height
	}
	if scroll < 0 {
		scroll = 0
	}
	// Persist resolved values so a no-key paint still self-corrects
	// the stored state (e.g. if the body shrank under our feet).
	if spec.BodyID != "" {
		if d.BodyCursor == nil {
			d.BodyCursor = map[string]int{}
		}
		if d.BodyScroll == nil {
			d.BodyScroll = map[string]int{}
		}
		d.BodyCursor[spec.BodyID] = cursor
		d.BodyScroll[spec.BodyID] = scroll
	}

	gutterStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	cursorBg := theme.BorderHi
	cursorLineBg := theme.Panel
	if !spec.Focused {
		// Body doesn't own focus — render a quieter cursor so it's
		// clear j / k will steer the action sidebar instead.
		cursorBg = theme.Border
		cursorLineBg = theme.BgAlt
	}
	cursorGutterStyle := lipgloss.NewStyle().
		Foreground(theme.Bg).
		Background(cursorBg).
		Bold(true)
	cursorLineStyle := lipgloss.NewStyle().Background(cursorLineBg)

	out := make([]string, 0, spec.Height+1)
	if findVisible {
		out = append(out, findBar)
	}
	end := scroll + spec.Height
	if end > total {
		end = total
	}
	for i := scroll; i < end; i++ {
		num := fmt.Sprintf(" %*d ", gutterW, i+1)
		var gutter string
		if i == cursor {
			gutter = cursorGutterStyle.Render(num) + " "
		} else {
			gutter = gutterStyle.Render(num) + " "
		}
		body := lines[i]
		// Lines holding find matches re-render from RAW source with
		// the match spans styled — chroma's ANSI can't be re-
		// backgrounded mid-span, so match colour wins over syntax
		// colour on exactly these lines.
		lineHasMatch := false
		if findVisible && findState.memoByLine != nil {
			if idxs := findState.memoByLine[i]; len(idxs) > 0 && i < len(findState.memoLines) {
				body = renderCodeFindLine(findState.memoLines[i], findMatches, idxs, findState.Idx)
				lineHasMatch = true
			}
		}
		if hscroll > 0 {
			// Shift only the body — the gutter stays put. The "…"
			// prefix marks a horizontally scrolled view.
			body = ansi.TruncateLeft(body, hscroll, "…")
		}
		if i == cursor && !lineHasMatch {
			// Highlight the cursor line by overlaying a panel-toned
			// background on the gutter+body composite. lipgloss can't
			// retroactively recolour ANSI inside the body, so we just
			// pad to inner width and let the background fill the rest.
			// Skipped on matched lines — the overlay would clobber the
			// match backgrounds; the cursor gutter still marks the row.
			content := gutter + body
			content = ansi.Truncate(content, spec.Inner, "…")
			pad := spec.Inner - ansi.StringWidth(content)
			if pad > 0 {
				content += strings.Repeat(" ", pad)
			}
			out = append(out, cursorLineStyle.Render(content))
			continue
		}
		out = append(out, ansi.Truncate(gutter+body, spec.Inner, "…"))
	}
	return out
}

// codeViewMoveCursor adjusts d.BodyCursor[bodyID] by delta, clamped
// to [0, lineCount-1]. Wired by per-tab MoveCursor closures when the
// user presses j / k / G / etc. on a code-detail surface.
func (m *Model) codeViewMoveCursor(d *orgData, bodyID string, lineCount, delta int) {
	if d == nil || bodyID == "" || lineCount <= 0 {
		return
	}
	if d.BodyCursor == nil {
		d.BodyCursor = map[string]int{}
	}
	cur := d.BodyCursor[bodyID]
	cur += delta
	if cur < 0 {
		cur = 0
	}
	if cur >= lineCount {
		cur = lineCount - 1
	}
	d.BodyCursor[bodyID] = cur
}
