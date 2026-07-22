package ui

// Details subtab of TabObjectDetail.
//
// Shows the sObject's own metadata: labels, identity, describe
// capability flags, a compact field-count summary. Editable rows
// (label / plural label / description and the five FEATURES toggles)
// are navigable directly in the MAIN pane: arrow keys walk every
// content row, and Enter / ctrl+e on an actionable row fires the
// edit or toggle modal in place. The right sidebar is now info-only
// — it merely reflects which action the cursored row maps to — so
// the user can safely hide it.
//
// objectDetailRows is the single source of truth: both this renderer
// and the cursor/activate hooks (tab_object_hooks.go) consume it so
// the visible row order and the navigable index can never drift.

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// objectDetailRow is one rendered line in the Details main pane.
// Section titles, blank spacers and dim footnotes are non-navigable
// (Navigable=false) so the cursor skips them; key/value rows are
// navigable. actionIdx >= 0 marks a row that maps to an entry in
// objectRegistry.Actions — Enter / ctrl+e on it opens that modal.
// actionIdx < 0 is a read-only row (cursor still rests there, but
// Enter is a no-op).
type objectDetailRow struct {
	Text      string // pre-rendered base text (before cursor styling)
	Navigable bool
	ActionIdx int // index into objectActionsFor(...); -1 = read-only
}

const noAction = -1

// objectDetailRows builds the ordered row model for the Details
// subtab. It mirrors renderObjectDetails' layout exactly. The
// describe must be loaded (caller guards) — v is the sObject
// describe, base is the CustomObjectBaseline (may be nil if not yet
// fetched), inner is the available text width.
//
// Action indices line up with objectActionsFor:
//
//	0 label · 1 pluralLabel · 2 description ·
//	3 reports · 4 activities · 5 feeds · 6 history · 7 search
func objectDetailRows(v sf.SObjectDescribe, base *sf.CustomObjectBaseline, r objectDetailMeta, inner int) []objectDetailRow {
	var rows []objectDetailRow
	title := func(s string) {
		rows = append(rows, objectDetailRow{Text: sectionTitle(s), ActionIdx: noAction})
	}
	blank := func() {
		rows = append(rows, objectDetailRow{Text: "", ActionIdx: noAction})
	}
	dim := func(s string) {
		rows = append(rows, objectDetailRow{Text: dimLine(s, inner), ActionIdx: noAction})
	}
	// kv adds a navigable key/value row. action >= 0 wires it to a
	// modal; noAction leaves it read-only (still navigable).
	kv := func(k, val string, action int) {
		rows = append(rows, objectDetailRow{
			Text:      kvLine(k, val, inner),
			Navigable: true,
			ActionIdx: action,
		})
	}

	// Title line: API name + label pair.
	titleLine := v.Name
	if v.Label != "" && v.Label != v.Name {
		titleLine += "  —  " + v.Label
	}
	rows = append(rows, objectDetailRow{Text: sectionTitle(titleLine), ActionIdx: noAction})
	rows = append(rows, objectDetailRow{
		Text: dimLine("  "+summaryObjectKind(v)+"  ·  "+
			humanAge(r.FetchedAt)+stateSuffix(r.Busy, r.Err), inner),
		ActionIdx: noAction,
	})
	blank()

	// IDENTITY
	title("IDENTITY")
	kv("api name", v.Name, noAction)
	if v.Label != "" {
		kv("label", v.Label, 0)
	}
	if v.LabelPlural != "" {
		kv("plural label", v.LabelPlural, 1)
	}
	if v.KeyPrefix != "" {
		kv("key prefix", v.KeyPrefix, noAction)
	}
	kind := "standard"
	if v.Custom {
		kind = "custom"
	}
	kv("kind", kind, noAction)
	// description lives in IDENTITY conceptually; the value isn't in
	// the describe so show the baseline's value (or a hint) and wire
	// the edit action regardless.
	desc := ""
	if base != nil {
		desc = base.Description
	}
	kv("description", dashIfEmpty(desc), 2)
	blank()

	// CAPABILITIES (from describe — always available, no extra call).
	title("CAPABILITIES")
	kv("queryable", yesNo(v.Queryable), noAction)
	kv("createable", yesNo(v.Creatable), noAction)
	kv("updateable", yesNo(v.Updatable), noAction)
	kv("deletable", yesNo(v.Deletable), noAction)
	blank()

	// FEATURES — metadata-level toggles from the CustomObjectBaseline.
	// Single-word labels keep each row on one line in the narrowed
	// pane (the old "history tracking" / "chatter feeds" / "global
	// search" labels wrapped when the sidebar was open).
	title("FEATURES")
	if base != nil {
		kv("reports", boolPtrLabel(base.EnableReports), 3)
		kv("activities", boolPtrLabel(base.EnableActivities), 4)
		kv("feeds", boolPtrLabel(base.EnableFeeds), 5)
		kv("history", boolPtrLabel(base.EnableHistory), 6)
		kv("search", boolPtrLabel(base.EnableSearch), 7)
	} else {
		dim("  loading current toggle state…")
	}
	blank()

	// FIELDS summary
	customCount := 0
	for _, f := range v.Fields {
		if f.Custom {
			customCount++
		}
	}
	title("FIELDS")
	kv("total", fmt.Sprintf("%d (%d custom · %d standard)",
		len(v.Fields), customCount, len(v.Fields)-customCount), noAction)
	dim("  (schema subtab → full browsable list + per-field drill)")

	if !v.Custom {
		blank()
		dim("  Object-level edits require a custom object — standard objects" +
			" have no CustomObject row.")
	}
	return rows
}

// objectDetailMeta carries the describe Resource's render-relevant
// state without threading the full Resource through objectDetailRows.
type objectDetailMeta struct {
	FetchedAt time.Time
	Busy      bool
	Err       error
}

// renderObjectDetails is the main-pane renderer for the Details
// subtab of TabObjectDetail.
func (m Model) renderObjectDetails(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	d := m.ensureOrgDataRef(o.Username)
	if d.DescribeCur == "" {
		return theme.Subtle.Render("  press enter on an object in /objects first")
	}
	r, ok := d.Describes[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		// Surface a describe error rather than spinning forever — an
		// errored describe never sets FetchedAt (e.g. inaccessible
		// managed-object NOT_FOUND).
		if r != nil && r.Err() != nil {
			return m.describeErrorLine(d.DescribeCur, r.Err())
		}
		return theme.Subtle.Render("  loading describe…")
	}
	v := r.Value()
	base, _ := readObjectBaselineForDetails(d, d.DescribeCur)
	meta := objectDetailMeta{FetchedAt: r.FetchedAt(), Busy: r.Busy(), Err: r.Err()}

	rows := objectDetailRows(v, base, meta, inner)

	// The cursor lives on navigable rows; objectActionCur indexes into
	// the navigable subset. Translate it to an absolute row index so we
	// can highlight the right line. Active only when this pane has focus
	// (the action menu in the sidebar is no longer interactive).
	navAbs := objectDetailNavIndex(rows)
	curNav := m.objectActionCur
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
	active := m.focus == focusMain

	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = renderObjectDetailLine(row, i == cursorRow, active, inner)
	}
	return scrollLinesToCursor(out, cursorRow, innerH)
}

// renderObjectDetailLine applies cursor styling to one row. The
// cursored row gets a left bar (bright when focused) and, when it
// maps to an action, a trailing edit/toggle affordance so the user
// knows Enter does something here.
func renderObjectDetailLine(row objectDetailRow, cursored, active bool, inner int) string {
	if !cursored {
		// Non-cursored rows render with a 2-space gutter to align with
		// the cursor bar's "▌ " prefix.
		return "  " + row.Text
	}
	barColor := theme.Muted
	if active {
		barColor = theme.BorderHi
	}
	bar := lipgloss.NewStyle().Foreground(barColor).Render("▌") + " "
	line := bar + row.Text
	if row.ActionIdx >= 0 {
		hintTxt := "  ↵ edit"
		if row.ActionIdx >= 3 {
			hintTxt = "  ↵ toggle"
		}
		hint := lipgloss.NewStyle().Foreground(theme.FgDim).Render(hintTxt)
		// Only append the hint if there's room — keep within inner.
		if ansi.StringWidth(line)+ansi.StringWidth(hintTxt) <= inner {
			line += hint
		}
	}
	return line
}

// objectDetailNavIndex returns the absolute row indices of every
// navigable row, in order. The Nth entry is the absolute row index of
// the Nth navigable row — lets the cursor (which counts navigable
// rows) map back to a render position.
func objectDetailNavIndex(rows []objectDetailRow) []int {
	var idx []int
	for i, row := range rows {
		if row.Navigable {
			idx = append(idx, i)
		}
	}
	return idx
}

// objectDetailNavCount is the number of navigable rows for the current
// Model state — used to clamp the row cursor. Returns 0 when the
// describe isn't loaded.
func (m Model) objectDetailNavCount() int {
	rows, ok := m.objectDetailRowModel()
	if !ok {
		return 0
	}
	return len(objectDetailNavIndex(rows))
}

// objectDetailActionForCursor returns the action index the cursored
// row maps to, or (noAction, false) when the cursor is on a read-only
// row or the describe isn't loaded.
func (m Model) objectDetailActionForCursor() (int, bool) {
	rows, ok := m.objectDetailRowModel()
	if !ok {
		return noAction, false
	}
	navAbs := objectDetailNavIndex(rows)
	cur := m.objectActionCur
	if cur < 0 || cur >= len(navAbs) {
		return noAction, false
	}
	row := rows[navAbs[cur]]
	if row.ActionIdx < 0 {
		return noAction, false
	}
	return row.ActionIdx, true
}

// objectDetailRowModel rebuilds the row model for the current Model
// state. Returns (nil, false) when there's no org / describe yet.
// Width is approximate (cursor logic doesn't depend on exact wrap);
// uses the live pane width when available.
func (m Model) objectDetailRowModel() ([]objectDetailRow, bool) {
	o, ok := m.currentOrg()
	if !ok {
		return nil, false
	}
	d := m.data[o.Username]
	if d == nil || d.DescribeCur == "" {
		return nil, false
	}
	r, ok := d.Describes[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		return nil, false
	}
	base, _ := readObjectBaselineForDetails(d, d.DescribeCur)
	meta := objectDetailMeta{FetchedAt: r.FetchedAt(), Busy: r.Busy(), Err: r.Err()}
	// Width only affects truncation, not which rows are navigable or
	// which action they map to — the only things the cursor logic
	// reads. A nominal width keeps the row set identical to the live
	// render.
	return objectDetailRows(r.Value(), base, meta, 60), true
}

// readObjectBaselineForDetails returns the cached baseline for
// sobject if the resource has fetched. Returns (nil, false) when
// not yet loaded — caller shows a "loading…" line while we wait.
func readObjectBaselineForDetails(d *orgData, sobject string) (*sf.CustomObjectBaseline, bool) {
	if d == nil {
		return nil, false
	}
	r, ok := d.CustomObjectBaselines[sobject]
	if !ok || r.FetchedAt().IsZero() {
		return nil, false
	}
	return r.Value(), true
}

// boolPtrLabel formats a *bool flag for display. nil = "unknown"
// (Salesforce didn't return a value for this flag — common on
// some standard objects whose toggle is implicit). Otherwise
// renders the standard yes/no shape used elsewhere in this view.
func boolPtrLabel(b *bool) string {
	if b == nil {
		return "unknown"
	}
	return yesNo(*b)
}

// summaryObjectKind condenses custom/standard + keyPrefix into a
// short one-line badge under the title.
func summaryObjectKind(v sf.SObjectDescribe) string {
	var parts []string
	if v.Custom {
		parts = append(parts, "custom")
	} else {
		parts = append(parts, "standard")
	}
	if v.KeyPrefix != "" {
		parts = append(parts, "prefix "+v.KeyPrefix)
	}
	return strings.Join(parts, " · ")
}
