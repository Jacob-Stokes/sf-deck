package ui

// /compare — Screen 4: the per-component body diff drill-in.
//
// Default view is side-by-side A│B (source left, target right) with
// red deletions / green additions; `u` toggles a single-column unified
// diff for terminals too narrow to split long lines. Bodies are fetched
// lazily on open (via the provider) and diffed with internal/diff.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/diff"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/highlight"
)

// openCompareDiff fetches both bodies for the selected inventory row and
// opens the side-by-side diff view.
func (m *Model) openCompareDiff(d *orgData) tea.Cmd {
	if d.Run == nil {
		return nil
	}
	row, ok := d.InventoryList.Selected()
	if !ok {
		return nil
	}
	// Snapshot path (Auto / Metadata API). Retained bodies diff with ZERO
	// further API calls. But a body may have been DROPPED at retrieve time
	// (over the memory budget — typically Profiles/StaticResources); the
	// inventory still knows its status (from the hash), so re-fetch the
	// missing side(s) live, then diff.
	if d.Run.hashA != nil || d.Run.hashB != nil || d.Run.snapA != nil || d.Run.snapB != nil {
		aHave := snapHasBody(d.Run.snapA, row.Type, row.Key)
		bHave := snapHasBody(d.Run.snapB, row.Type, row.Key)
		// A-only / B-only rows legitimately have no counterpart — those
		// "absent" sides are not a dropped body, so don't re-fetch them.
		aMissing := !aHave && row.AID != ""
		bMissing := !bHave && row.BID != ""
		if !aMissing && !bMissing {
			res := diff.BodyDiffFromSnapshots(row, d.Run.snapA, d.Run.snapB)
			d.Diff = &compareDiffView{Row: row, Result: res, Lang: compareLangFor(row.Type)}
			return nil
		}
		// Re-fetch the dropped side(s) live, off the UI loop.
		d.Diff = &compareDiffView{Row: row, Lang: compareLangFor(row.Type), Loading: true}
		return m.refetchCompareBodies(d, row, aMissing, bMissing)
	}
	// Tooling path: lazily fetch the two bodies via the provider. Only
	// reachable for the Tooling method (no snapshot). Object-rooted
	// CHILD rows (keyed "Object.Child") have no per-component provider
	// fetch — they only exist inside the snapshot path — so guard against
	// the broken RetrieveMetadataXML fallback mishandling them.
	if strings.Contains(row.Key, ".") {
		m.flash("open this comparison with the Auto/Metadata API method to diff " + row.Type)
		return nil
	}
	p, ok := providerByLabel(row.Type)
	if !ok {
		m.flash("no provider for " + row.Type)
		return nil
	}
	source, target := d.Run.Source.OrgRef(), d.Run.Target.OrgRef()
	res, err := diff.BodyDiff(row, source, target, p)
	d.Diff = &compareDiffView{
		Row:    row,
		Result: res,
		Lang:   compareLangFor(row.Type),
		Err:    err,
	}
	return nil
}

// closeCompareDiff returns from the body diff to the inventory list.
func (m *Model) closeCompareDiff(d *orgData) {
	d.Diff = nil
}

// snapHasBody reports whether a retained body exists for a row.
func snapHasBody(snap diff.Snapshot, typeLabel, key string) bool {
	if snap == nil {
		return false
	}
	m := snap[typeLabel]
	if m == nil {
		return false
	}
	_, ok := m[key]
	return ok
}

// compareBodyFetchedMsg delivers re-fetched dropped bodies for a drill-in.
type compareBodyFetchedMsg struct {
	OrgKey string
	Row    diff.Row
	ABody  string
	BBody  string
	HadA   bool // whether ABody was (re)fetched (vs already retained)
	HadB   bool
	Err    error
}

// refetchCompareBodies re-fetches the dropped side(s) of one component
// off the UI loop and emits compareBodyFetchedMsg. Only the missing
// side(s) are fetched; the retained side (if any) rides through.
func (m *Model) refetchCompareBodies(d *orgData, row diff.Row, aMissing, bMissing bool) tea.Cmd {
	source, target := d.Run.Source.OrgRef(), d.Run.Target.OrgRef()
	// Capture the retained bodies for the non-missing side.
	var aHave, bHave string
	if !aMissing {
		aHave = snapBody(d.Run.snapA, row.Type, row.Key)
	}
	if !bMissing {
		bHave = snapBody(d.Run.snapB, row.Type, row.Key)
	}
	orgKey := ""
	if len(m.orgs) > 0 {
		orgKey = m.orgs[m.selected].Username
	}
	return func() tea.Msg {
		out := compareBodyFetchedMsg{OrgKey: orgKey, Row: row, ABody: aHave, BBody: bHave, HadA: aMissing, HadB: bMissing}
		if aMissing {
			b, err := fetchOneComponentBody(source, row.Type, row.Key)
			if err != nil {
				out.Err = err
			}
			out.ABody = b
		}
		if bMissing {
			b, err := fetchOneComponentBody(target, row.Type, row.Key)
			if err != nil && out.Err == nil {
				out.Err = err
			}
			out.BBody = b
		}
		return out
	}
}

// applyCompareBodyFetched fills the drill-in diff once the re-fetch lands.
func (m *Model) applyCompareBodyFetched(msg compareBodyFetchedMsg) {
	d, ok := m.data[msg.OrgKey]
	if !ok || d.Diff == nil || d.Diff.Row.Key != msg.Row.Key || d.Diff.Row.Type != msg.Row.Type {
		return // user navigated away / opened a different row
	}
	if msg.Err != nil {
		d.Diff.Loading = false
		d.Diff.Err = msg.Err
		return
	}
	d.Diff.Result = diff.Text(diff.PrettyXML(msg.ABody), diff.PrettyXML(msg.BBody))
	d.Diff.Loading = false
}

// comparePreviewFetchedMsg delivers a re-fetched body for the side-panel
// preview. Unlike compareBodyFetchedMsg (drill-in), it stores the body
// back into the snapshot so the preview (and a later drill-in) rebuild
// from it.
type comparePreviewFetchedMsg struct {
	OrgKey string
	Row    diff.Row
	ABody  string
	BBody  string
	FetchA bool
	FetchB bool
	Err    error
}

// refetchComparePreview fetches the dropped side(s) for the preview, off
// the UI loop, emitting comparePreviewFetchedMsg.
func (m *Model) refetchComparePreview(d *orgData, row diff.Row, aMissing, bMissing bool) tea.Cmd {
	source, target := d.Run.Source.OrgRef(), d.Run.Target.OrgRef()
	orgKey := ""
	if len(m.orgs) > 0 {
		orgKey = m.orgs[m.selected].Username
	}
	return func() tea.Msg {
		out := comparePreviewFetchedMsg{OrgKey: orgKey, Row: row, FetchA: aMissing, FetchB: bMissing}
		if aMissing {
			b, err := fetchOneComponentBody(source, row.Type, row.Key)
			if err != nil {
				out.Err = err
			}
			out.ABody = b
		}
		if bMissing {
			b, err := fetchOneComponentBody(target, row.Type, row.Key)
			if err != nil && out.Err == nil {
				out.Err = err
			}
			out.BBody = b
		}
		return out
	}
}

// applyComparePreviewFetched stores the fetched body into the snapshot and
// clears the loading flag so the preview rebuilds on next render.
func (m *Model) applyComparePreviewFetched(msg comparePreviewFetchedMsg) {
	d, ok := m.data[msg.OrgKey]
	if !ok || d.Run == nil {
		return
	}
	d.previewLoading = false
	if msg.Err != nil {
		return // leave it as "not cached"; user can retry
	}
	if msg.FetchA && d.Run.snapA != nil {
		if d.Run.snapA[msg.Row.Type] == nil {
			d.Run.snapA[msg.Row.Type] = map[string]string{}
		}
		d.Run.snapA[msg.Row.Type][msg.Row.Key] = msg.ABody
	}
	if msg.FetchB && d.Run.snapB != nil {
		if d.Run.snapB[msg.Row.Type] == nil {
			d.Run.snapB[msg.Row.Type] = map[string]string{}
		}
		d.Run.snapB[msg.Row.Type][msg.Row.Key] = msg.BBody
	}
	// Force a rebuild on next render (the body is now present).
	d.previewKey = ""
}

// snapBody returns a retained body or "".
func snapBody(snap diff.Snapshot, typeLabel, key string) string {
	if snap == nil {
		return ""
	}
	if mm := snap[typeLabel]; mm != nil {
		return mm[key]
	}
	return ""
}

// fetchOneComponentBody re-fetches a single component's body live. Handles
// the object-child case (key "Object.Child" → retrieve the parent object
// and extract), CustomObject itself, Apex (bulk column), and plain
// top-level types (single-name SOAP readMetadata).
func fetchOneComponentBody(alias, typeLabel, key string) (string, error) {
	switch {
	case compareObjectChildTypes[typeLabel]:
		// Re-fetch the parent object with child extraction; pick our key out.
		objectName := key
		if dot := strings.IndexByte(key, '.'); dot >= 0 {
			objectName = key[:dot]
		}
		snap, err := sf.RetrieveViaSOAP(alias, "CustomObject", []string{objectName}, true)
		if err != nil {
			return "", err
		}
		if mm := snap[typeLabel]; mm != nil {
			return mm[key], nil
		}
		return "", nil
	case apexCompareTypes[typeLabel]:
		bodies, err := sf.BulkApexBodies(alias, typeLabel)
		if err != nil {
			return "", err
		}
		return bodies[key], nil
	default: // CustomObject + every plain top-level type
		extractChildren := typeLabel == "CustomObject"
		snap, err := sf.RetrieveViaSOAP(alias, typeLabel, []string{key}, extractChildren)
		if err != nil {
			return "", err
		}
		if mm := snap[typeLabel]; mm != nil {
			return mm[key], nil
		}
		return "", nil
	}
}

func compareLangFor(typeLabel string) string {
	switch typeLabel {
	case "ApexClass", "ApexTrigger":
		return highlight.LangApex
	default:
		return highlight.LangPlain
	}
}

// renderCompareDiff draws the body diff (side-by-side or unified).
func (m Model) renderCompareDiff(w, innerH int, d *orgData) string {
	dv := d.Diff
	inner := w - 4
	var head []string
	mode := "side-by-side"
	if dv.Unified {
		mode = "unified"
	}
	head = append(head, sectionTitle(fmt.Sprintf("COMPARE · %s · %s", dv.Row.Key, dv.Row.Type)))
	head = append(head, dimLine(fmt.Sprintf("  %d added · %d removed · %s   ·   %s",
		dv.Result.Added, dv.Result.Removed, mode, m.compareTitleArrow(d.Run)), inner))
	head = append(head, "")

	if dv.Loading {
		head = append(head, theme.Subtle.Render(
			"  "+compareSpinner(m.compareFrame)+" fetching body (too large to keep cached — loading live)…"))
		head = append(head, "")
		head = append(head, dimLine("  esc back", inner))
		return strings.Join(head, "\n")
	}

	if dv.Err != nil {
		head = append(head, lipgloss.NewStyle().Foreground(theme.Red).Render("  "+dv.Err.Error()))
		head = append(head, "")
		head = append(head, dimLine("  esc back", inner))
		return strings.Join(head, "\n")
	}

	budget := innerH - len(head) - 2
	if budget < 3 {
		budget = 3
	}

	var body []string
	if dv.Unified {
		body = renderUnifiedDiff(dv, inner, budget)
	} else {
		body = renderSideBySideDiff(dv, inner, budget)
	}
	foot := dimLine("  ↑↓ scroll · u "+toggleWord(dv.Unified)+" · [ ] prev/next component · esc back", inner)
	out := append(head, body...)
	out = append(out, "", foot)
	return strings.Join(out, "\n")
}

func toggleWord(unified bool) string {
	if unified {
		return "side-by-side"
	}
	return "unified"
}

// renderSideBySideDiff lays out two columns: A (source) left, B (target)
// right, separated by a vertical bar. Deletions tint A red, insertions
// tint B green, equal lines are plain on both. A blank "(absent)" gap
// fills the side that has no counterpart on that row.
func renderSideBySideDiff(dv *compareDiffView, inner, budget int) []string {
	colW := (inner - 3) / 2 // 3 = " │ " separator
	if colW < 8 {
		colW = 8
	}
	red := lipgloss.NewStyle().Foreground(theme.Red)
	green := lipgloss.NewStyle().Foreground(theme.Green)
	dim := lipgloss.NewStyle().Foreground(theme.FgDim)
	bar := dim.Render("│")

	lines := dv.Result.Lines
	start := clampScroll(dv.Scroll, len(lines))
	var out []string
	for i := start; i < len(lines) && len(out) < budget; i++ {
		l := lines[i]
		var aCell, bCell string
		switch l.Op {
		case diff.OpEqual:
			aCell = fmt.Sprintf("%4d  %s", l.ALine, l.Text)
			bCell = fmt.Sprintf("%4d  %s", l.BLine, l.BText)
			aCell = ansi.Truncate(aCell, colW, "…")
			bCell = ansi.Truncate(bCell, colW, "…")
		case diff.OpDelete:
			aCell = red.Render(ansi.Truncate(fmt.Sprintf("%4d -%s", l.ALine, l.Text), colW, "…"))
			bCell = dim.Render(ansi.Truncate("       (absent)", colW, "…"))
		case diff.OpInsert:
			aCell = dim.Render(ansi.Truncate("       (absent)", colW, "…"))
			bCell = green.Render(ansi.Truncate(fmt.Sprintf("%4d +%s", l.BLine, l.Text), colW, "…"))
		}
		out = append(out, " "+padCellTo(aCell, colW)+" "+bar+" "+padCellTo(bCell, colW))
	}
	return out
}

// renderUnifiedDiff is the narrow-terminal fallback: a single git-style
// column with +/- prefixes.
func renderUnifiedDiff(dv *compareDiffView, inner, budget int) []string {
	red := lipgloss.NewStyle().Foreground(theme.Red)
	green := lipgloss.NewStyle().Foreground(theme.Green)
	lines := dv.Result.Lines
	start := clampScroll(dv.Scroll, len(lines))
	var out []string
	for i := start; i < len(lines) && len(out) < budget; i++ {
		l := lines[i]
		var s string
		switch l.Op {
		case diff.OpEqual:
			s = "  " + l.Text
		case diff.OpDelete:
			s = red.Render("- " + l.Text)
		case diff.OpInsert:
			s = green.Render("+ " + l.Text)
		}
		out = append(out, " "+ansi.Truncate(s, inner-1, "…"))
	}
	return out
}

// padCellTo right-pads an (already truncated, possibly styled) cell to
// width using the visible width so the separator bar stays aligned.
func padCellTo(s string, w int) string {
	gap := w - ansi.StringWidth(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

// stepCompareComponent moves the inventory cursor by delta and reopens
// the diff for the new row — the [ / ] prev/next-component nav.
func (m *Model) stepCompareComponent(d *orgData, delta int) tea.Cmd {
	if d.Diff == nil {
		return nil
	}
	d.InventoryList.MoveBy(delta)
	return m.openCompareDiff(d)
}
