package ui

// TabValidationDetail — full-pane view of a single ValidationRule.
// Drill from the Validation subtab's list (enter on a row).
//
// Layout parallels TabFieldDetail / TabObjectDetail: every editable
// property is a navigable row in the MAIN pane (active toggle, error
// message, condition formula, description) plus a DANGER ZONE delete
// at the bottom. Arrow keys walk the rows; Enter fires the edit /
// toggle / delete modal. The right sidebar is INFO-ONLY (reflects
// which action the cursored row maps to) so it can be hidden safely.
//
// validationDetailRows is the single source of truth — both this
// renderer and the cursor/activate hooks consume it so the visible
// order and the navigable index can't drift.

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// detailRow is the shared structured-row shape for the drill-detail
// surfaces (validation / record type / trigger). Navigable rows take
// the cursor; ActionIdx >= 0 wires the row to an entry in the
// registry's action list. Danger marks a destructive row (rendered
// red). Locked suppresses the affordance + flashes a reason on Enter.
type detailRow struct {
	Text      string
	Navigable bool
	ActionIdx int
	Danger    bool
	Locked    bool
}

// Validation action indices — track validationActionsFor's order.
const (
	vrActToggleActive = 0
	vrActErrorMsg     = 1
	vrActFormula      = 2
	vrActDescription  = 3
	vrActDelete       = 4
)

// validationDetailRows builds the ordered row model. det is the
// fetched rule Metadata, rowName the display name for the title.
func validationDetailRows(sobject, rowName string, det sf.ValidationRuleDetail, fetchedAt detailMeta, inner int) []detailRow {
	b := newDetailRowBuilder(inner)

	title := sobject + "  /  " + rowName
	b.title(title)
	status := "inactive"
	if det.Active {
		status = "active"
	}
	b.dim("  " + status + "  ·  id " + det.ID + "  ·  " +
		humanAge(fetchedAt.FetchedAt) + stateSuffix(fetchedAt.Busy, fetchedAt.Err))
	b.blank()

	// STATUS — the active toggle.
	b.title("STATUS")
	b.kv("active", yesNo(det.Active), vrActToggleActive)
	b.blank()

	// ERROR MESSAGE
	b.title("ERROR MESSAGE")
	b.kvWrapped("message", det.ErrorMessage, vrActErrorMsg)
	b.blank()

	// ERROR FIELD (read-only — not separately editable here).
	if det.ErrorDisplayField != "" {
		b.title("ERROR FIELD")
		b.kv("field", det.ErrorDisplayField, noAction)
		b.blank()
	}

	// CONDITION FORMULA
	b.title("CONDITION FORMULA")
	b.kvWrapped("formula", det.ErrorConditionFormula, vrActFormula)
	b.blank()

	// DESCRIPTION
	b.title("DESCRIPTION")
	b.kvWrapped("description", det.Description, vrActDescription)

	// DANGER ZONE
	b.dangerSection("delete rule", vrActDelete)
	return b.rows
}

// renderValidationDetail is the main-pane renderer for
// TabValidationDetail.
func (m Model) renderValidationDetail(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	d := m.ensureOrgDataRef(o.Username)
	if d.DescribeCur == "" || d.ValidationRules.DrillID == "" {
		return theme.Subtle.Render("  press enter on a rule in the Validation subtab first")
	}
	r, ok := d.ValidationRules.Details[d.ValidationRules.DrillID]
	if !ok || r.FetchedAt().IsZero() {
		if r != nil && r.Busy() {
			return theme.Subtle.Render("  loading rule Metadata…")
		}
		if r != nil && r.Err() != nil {
			return redLine("  " + r.Err().Error())
		}
		return theme.Subtle.Render("  fetching rule Metadata…")
	}
	rows := m.validationDetailRowsFor(d, inner)
	return renderDetailRows(rows, m.validationActionCur, m.focus == focusMain, inner, innerH)
}

// validationDetailRowsFor resolves the row model for the current
// drilled rule. nil-safe — returns nil if the detail isn't loaded.
func (m Model) validationDetailRowsFor(d *orgData, inner int) []detailRow {
	r, ok := d.ValidationRules.Details[d.ValidationRules.DrillID]
	if !ok || r == nil || r.FetchedAt().IsZero() {
		return nil
	}
	var rowName string
	if lr, ok := d.ValidationRules.Lists[d.DescribeCur]; ok {
		for _, vr := range lr.Value() {
			if vr.ID == d.ValidationRules.DrillID {
				rowName = vr.ValidationName
				break
			}
		}
	}
	if rowName == "" {
		rowName = d.ValidationRules.DrillID
	}
	meta := detailMeta{FetchedAt: r.FetchedAt(), Busy: r.Busy(), Err: r.Err()}
	return validationDetailRows(d.DescribeCur, rowName, r.Value(), meta, inner)
}

func (m Model) validationDetailRowModel() ([]detailRow, bool) {
	o, ok := m.currentOrg()
	if !ok {
		return nil, false
	}
	d := m.data[o.Username]
	if d == nil || d.DescribeCur == "" || d.ValidationRules.DrillID == "" {
		return nil, false
	}
	rows := m.validationDetailRowsFor(d, 60)
	if rows == nil {
		return nil, false
	}
	return rows, true
}

func (m Model) validationDetailNavCount() int {
	rows, ok := m.validationDetailRowModel()
	if !ok {
		return 0
	}
	return len(detailNavIndex(rows))
}

func (m Model) validationDetailActionForCursor() (int, bool) {
	rows, ok := m.validationDetailRowModel()
	if !ok {
		return noAction, false
	}
	return detailActionForCursor(rows, m.validationActionCur)
}

// sidebarValidationActions renders the TabValidationDetail right
// sidebar — a context panel for the cursored row.
func (m Model) sidebarValidationActions(inner int) string {
	ctx := m.validationRowContext()
	ctx.Hints = detailNavHints(true)
	return m.sidebarRowContext("RULE · CONTEXT", inner, ctx)
}

// validationRowContext builds the context panel for the cursored rule
// row. Writes route through the Tooling ValidationRule entity.
func (m Model) validationRowContext() rowContext {
	idx, ok := m.validationDetailActionForCursor()
	if !ok {
		return rowContext{ReadOnlyMsg: "read-only row. The active toggle, error message, condition formula, description + the DANGER ZONE delete carry a ↵ hint."}
	}
	rows := RegistryRows(m, validationRegistry)
	if idx < 0 || idx >= len(rows) {
		return rowContext{}
	}
	a := rows[idx]
	ctx := rowContext{
		Heading: a.Label,
		Help:    a.Hint,
		Routing: "Tooling API · ValidationRule patch",
		Danger:  idx == vrActDelete,
	}
	if !a.Allowed {
		ctx.Blocked = a.Reason
	}
	switch idx {
	case vrActToggleActive:
		ctx.Affects = "whether the rule fires on insert / update."
	case vrActErrorMsg:
		ctx.Affects = "the text users see when the rule blocks a save."
	case vrActFormula:
		ctx.Affects = "the boolean condition — true triggers the error."
	case vrActDescription:
		ctx.Affects = "the Setup-only description."
	case vrActDelete:
		ctx.Routing = "Tooling API · delete ValidationRule"
		ctx.Affects = "permanently removes the rule. No undo."
	}
	if det, ok := m.currentValidationDetail(); ok {
		ctx.Current = validationActionCurrentValue(idx, det)
	}
	return ctx
}

// currentValidationDetail returns the drilled rule's fetched Metadata.
func (m Model) currentValidationDetail() (sf.ValidationRuleDetail, bool) {
	o, ok := m.currentOrg()
	if !ok {
		return sf.ValidationRuleDetail{}, false
	}
	d := m.data[o.Username]
	if d == nil || d.ValidationRules.DrillID == "" {
		return sf.ValidationRuleDetail{}, false
	}
	r, ok := d.ValidationRules.Details[d.ValidationRules.DrillID]
	if !ok || r == nil || r.FetchedAt().IsZero() {
		return sf.ValidationRuleDetail{}, false
	}
	return r.Value(), true
}

func validationActionCurrentValue(idx int, det sf.ValidationRuleDetail) string {
	switch idx {
	case vrActToggleActive:
		return yesNo(det.Active)
	case vrActErrorMsg:
		return dashIfEmpty(det.ErrorMessage)
	case vrActFormula:
		return dashIfEmpty(det.ErrorConditionFormula)
	case vrActDescription:
		return dashIfEmpty(det.Description)
	}
	return ""
}

// --- shared detail-row builder + render helpers -------------------------

// detailMeta carries a Resource's render-relevant state.
type detailMeta struct {
	FetchedAt time.Time
	Busy      bool
	Err       error
}

// detailRowBuilder accumulates detailRow values with consistent
// styling. Shared by the three registry-backed detail surfaces.
type detailRowBuilder struct {
	rows  []detailRow
	inner int
}

func newDetailRowBuilder(inner int) *detailRowBuilder {
	return &detailRowBuilder{inner: inner}
}

func (b *detailRowBuilder) title(s string) {
	b.rows = append(b.rows, detailRow{Text: sectionTitle(s)})
}
func (b *detailRowBuilder) blank() { b.rows = append(b.rows, detailRow{Text: ""}) }
func (b *detailRowBuilder) dim(s string) {
	b.rows = append(b.rows, detailRow{Text: dimLine(s, b.inner)})
}

// kv adds a navigable key/value row wired to action (noAction =
// read-only). Long values truncate to one line.
func (b *detailRowBuilder) kv(k, val string, action int) {
	b.rows = append(b.rows, detailRow{
		Text:      kvLine(k, dashIfEmpty(val), b.inner),
		Navigable: true,
		ActionIdx: action,
	})
}

// kvWrapped is like kv but for the long free-text fields (error
// message, formula, description) — still one navigable row, truncated.
func (b *detailRowBuilder) kvWrapped(k, val string, action int) {
	display := val
	if display == "" {
		display = "—  (opens to edit)"
	}
	b.rows = append(b.rows, detailRow{
		Text:      kvLine(k, display, b.inner),
		Navigable: true,
		ActionIdx: action,
	})
}

// dangerSection appends a blank, a red DANGER ZONE title, and a single
// destructive row wired to action.
func (b *detailRowBuilder) dangerSection(label string, action int) {
	b.blank()
	b.rows = append(b.rows, detailRow{Text: redLine("DANGER ZONE")})
	b.rows = append(b.rows, detailRow{
		Text:      "  " + redLine(label),
		Navigable: true,
		ActionIdx: action,
		Danger:    true,
	})
}

// detailNavIndex returns the absolute row indices of the navigable
// rows, in order.
func detailNavIndex(rows []detailRow) []int {
	var idx []int
	for i, row := range rows {
		if row.Navigable {
			idx = append(idx, i)
		}
	}
	return idx
}

// detailActionForCursor maps a navigable-row cursor to the action it
// fires, or (noAction, false) for read-only / out-of-range.
func detailActionForCursor(rows []detailRow, cur int) (int, bool) {
	navAbs := detailNavIndex(rows)
	if cur < 0 || cur >= len(navAbs) {
		return noAction, false
	}
	row := rows[navAbs[cur]]
	if row.ActionIdx < 0 {
		return noAction, false
	}
	return row.ActionIdx, true
}

// renderDetailRows lays out the detail rows with cursor highlighting,
// scrolling the view to keep the cursored row visible when the row set
// is taller than height. curNav is the navigable-row cursor; active
// brightens the bar when the pane has focus.
func renderDetailRows(rows []detailRow, curNav int, active bool, inner, height int) string {
	navAbs := detailNavIndex(rows)
	if curNav < 0 {
		curNav = 0
	}
	if curNav >= len(navAbs) {
		curNav = len(navAbs) - 1
	}
	cursorRow := -1
	if curNav >= 0 && len(navAbs) > 0 {
		cursorRow = navAbs[curNav]
	}
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = renderDetailLine(row, i == cursorRow, active, inner)
	}
	return scrollLinesToCursor(out, cursorRow, height)
}

// scrollLinesToCursor returns a viewport slice of lines that keeps the
// row at cursorAbs visible, given height visible lines. When the
// content fits, all lines are returned unchanged. Uses the same
// 1/3-from-top bias as the list scroller so the feel matches other
// surfaces. A scroll affordance ("· N more ↑/↓") replaces the first /
// last visible line when content is clipped in that direction, so the
// user can tell the view is windowed.
func scrollLinesToCursor(lines []string, cursorAbs, height int) string {
	n := len(lines)
	if height <= 0 || n <= height {
		return strings.Join(lines, "\n")
	}
	sel := cursorAbs
	if sel < 0 {
		sel = 0
	}
	start, end := scrollWindow(sel, n, height)
	win := append([]string(nil), lines[start:end]...)
	// Replace the top / bottom line with a "more" marker when clipped,
	// so the windowing is visible without stealing extra height.
	if start > 0 && len(win) > 0 {
		win[0] = sideDim(fmt.Sprintf("  ↑ %d more", start), 9999)
	}
	if end < n && len(win) > 0 {
		win[len(win)-1] = sideDim(fmt.Sprintf("  ↓ %d more", n-end), 9999)
	}
	return strings.Join(win, "\n")
}

// scrollLinesKeepTop is a top-anchored variant of scrollLinesToCursor:
// it keeps row 0 visible until the cursor would fall below the bottom
// of the window, then scrolls just enough to keep the cursor on screen
// (no 1/3-from-top bias). Used by surfaces with a tall fixed header
// (the /home logo) where snapping the cursor to 1/3 would hide the
// header on first paint even though nothing needs scrolling yet.
func scrollLinesKeepTop(lines []string, cursorAbs, height int) string {
	n := len(lines)
	if height <= 0 || n <= height {
		return strings.Join(lines, "\n")
	}
	// The first/last visible rows may become "↑/↓ more" markers, which
	// would hide the cursor if it landed on them. Reserve those rows:
	// the cursor must stay within [start+topResv, end-1-botResv]. We
	// compute start so the cursor sits just above the bottom marker when
	// scrolling down, and just below the top marker when scrolling up.
	start := 0
	// Bottom marker is present whenever there's content below the window.
	// Keep the cursor at most (height-2) rows below start so it never
	// lands on the bottom marker row (height-1).
	if cursorAbs > start+height-2 {
		start = cursorAbs - (height - 2)
	}
	// Clamp so we never scroll past the end.
	if start > n-height {
		start = n - height
	}
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > n {
		end = n
	}
	// If scrolling up put the cursor on the top marker row, nudge down.
	if start > 0 && cursorAbs == start {
		start++
		end = start + height
		if end > n {
			end = n
			start = end - height
		}
	}
	win := append([]string(nil), lines[start:end]...)
	if start > 0 && len(win) > 0 {
		win[0] = sideDim(fmt.Sprintf("  ↑ %d more", start), 9999)
	}
	if end < n && len(win) > 0 {
		win[len(win)-1] = sideDim(fmt.Sprintf("  ↓ %d more", n-end), 9999)
	}
	return strings.Join(win, "\n")
}

func renderDetailLine(row detailRow, cursored, active bool, inner int) string {
	if !cursored {
		return "  " + row.Text
	}
	barColor := theme.Muted
	if active {
		barColor = theme.BorderHi
	}
	if row.Danger && active {
		barColor = theme.Red
	}
	bar := lipgloss.NewStyle().Foreground(barColor).Render("▌") + " "
	line := bar + row.Text
	if row.ActionIdx >= 0 {
		hintTxt := "  ↵ edit"
		if row.Danger {
			hintTxt = "  ⚠ ↵ delete"
		} else if row.ActionIdx == 0 {
			// First action on these surfaces is the active/status toggle.
			hintTxt = "  ↵ toggle"
		}
		if row.Locked {
			hintTxt = "  (locked)"
		}
		hint := lipgloss.NewStyle().Foreground(theme.FgDim).Render(hintTxt)
		if ansi.StringWidth(line)+ansi.StringWidth(hintTxt) <= inner {
			line += hint
		}
	}
	return line
}
