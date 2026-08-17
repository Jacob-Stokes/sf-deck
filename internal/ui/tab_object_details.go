package ui

// Details subtab of TabObjectDetail.

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

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
	kv := func(k, val string, action int) {
		rows = append(rows, objectDetailRow{
			Text:      kvLine(k, val, inner),
			Navigable: true,
			ActionIdx: action,
		})
	}

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
	desc := ""
	if base != nil {
		desc = base.Description
	}
	kv("description", dashIfEmpty(desc), 2)
	blank()

	title("CAPABILITIES")
	kv("queryable", yesNo(v.Queryable), noAction)
	kv("createable", yesNo(v.Creatable), noAction)
	kv("updateable", yesNo(v.Updatable), noAction)
	kv("deletable", yesNo(v.Deletable), noAction)
	blank()

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

type objectDetailMeta struct {
	FetchedAt time.Time
	Busy      bool
	Err       error
}

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

func renderObjectDetailLine(row objectDetailRow, cursored, active bool, inner int) string {
	if !cursored {
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
		if ansi.StringWidth(line)+ansi.StringWidth(hintTxt) <= inner {
			line += hint
		}
	}
	return line
}

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
	return objectDetailRows(r.Value(), base, meta, 60), true
}

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
