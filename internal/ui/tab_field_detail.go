package ui

// Field-detail page.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// fieldDetailRow is one rendered line in the field-detail main pane.
// Navigable rows take the cursor; ActionIdx >= 0 wires the row to an
// entry in fieldActionsFor (Enter fires its modal). ActionIdx < 0 is
// read-only (cursor rests there, Enter is a no-op). Danger marks the
// destructive delete row so it renders in red.
type fieldDetailRow struct {
	Text      string
	Navigable bool
	ActionIdx int
	Danger    bool
	// Locked marks an action row whose edit is unavailable on this
	// field (e.g. label/description/delete on a standard field). The
	// row still renders + takes the cursor, but the trailing
	// affordance is suppressed and Enter flashes the reason (handled
	// by StartAction's Disabled gate).
	Locked    bool
	YankValue string
}

// Field action indices — must track fieldActionsFor's order.
const (
	fieldActLabel       = 0
	fieldActHelp        = 1
	fieldActDescription = 2
	fieldActDefault     = 3
	fieldActRequired    = 4
	fieldActUnique      = 5
	fieldActExternalID  = 6
	fieldActDelete      = 7
)

func fieldDescriptionDisplay(f sf.Field, description string, descLoaded bool) string {
	if !f.Custom {
		return "—"
	}
	if !descLoaded {
		return "…  (loading)"
	}
	if description == "" {
		return "—"
	}
	return description
}

func fieldDetailRows(sobject string, f sf.Field, description string, descLoaded bool, inner int) []fieldDetailRow {
	var rows []fieldDetailRow
	title := func(s string) {
		rows = append(rows, fieldDetailRow{Text: sectionTitle(s)})
	}
	blank := func() { rows = append(rows, fieldDetailRow{Text: ""}) }
	dim := func(s string) {
		rows = append(rows, fieldDetailRow{Text: dimLine(s, inner)})
	}
	// locked reports whether an action is unavailable on this field.
	// Mirrors fieldActionsFor's Disabled hooks: every action except
	// "edit help text" is gated to custom fields.
	locked := func(action int) bool {
		if action < 0 {
			return false
		}
		if action == fieldActHelp {
			return false
		}
		return !f.Custom
	}
	kv := func(k, val string, action int) {
		rows = append(rows, fieldDetailRow{
			Text:      kvLine(k, val, inner),
			Navigable: true,
			ActionIdx: action,
			Locked:    locked(action),
		})
	}

	titleLine := sobject + "." + f.Name
	if f.Label != "" && f.Label != f.Name {
		titleLine += "  —  " + f.Label
	}
	rows = append(rows, fieldDetailRow{Text: sectionTitle(titleLine)})
	rows = append(rows, fieldDetailRow{Text: dimLine("  "+fieldTypeDisplay(f)+"  ·  "+summaryKind(f), inner)})
	blank()

	title("IDENTITY")
	kv("label", dashIfEmpty(f.Label), fieldActLabel)
	kv("help text", dashIfEmpty(f.InlineHelpText), fieldActHelp)
	kv("description", fieldDescriptionDisplay(f, description, descLoaded), fieldActDescription)
	def := ""
	if s, ok := stringish(f.DefaultValue); ok {
		def = s
	}
	if def == "" && f.DefaultValueFormula != "" {
		def = f.DefaultValueFormula
	}
	kv("default", dashIfEmpty(def), fieldActDefault)
	blank()

	title("CONSTRAINTS")
	kv("required", yesNo(!f.Nillable), fieldActRequired)
	kv("unique", yesNo(f.Unique), fieldActUnique)
	kv("external id", yesNo(f.ExternalID), fieldActExternalID)
	if f.Length > 0 {
		kv("length", fmt.Sprintf("%d", f.Length), noAction)
	}
	if f.Precision > 0 || f.Scale > 0 {
		kv("precision / scale", fmt.Sprintf("%d / %d", f.Precision, f.Scale), noAction)
	}
	if f.Digits > 0 {
		kv("digits", fmt.Sprintf("%d", f.Digits), noAction)
	}
	blank()

	if len(f.ReferenceTo) > 0 || f.RelationshipName != "" {
		title("REFERENCE")
		if len(f.ReferenceTo) > 0 {
			kv("targets", strings.Join(f.ReferenceTo, ", "), noAction)
		}
		if f.RelationshipName != "" {
			kv("relationship", f.RelationshipName, noAction)
		}
		if f.CascadeDelete {
			kv("on delete", "cascade", noAction)
		} else if f.RestrictedDelete {
			kv("on delete", "restricted", noAction)
		}
		if f.WriteRequiresMasterRead {
			kv("write requires master read", "yes", noAction)
		}
		blank()
	}

	if len(f.PicklistValues) > 0 {
		title(fmt.Sprintf("PICKLIST · %d values", len(f.PicklistValues)))
		meta := []string{}
		if f.RestrictedPicklist {
			meta = append(meta, "restricted")
		} else {
			meta = append(meta, "unrestricted")
		}
		if f.DependentPicklist {
			meta = append(meta, "dependent")
		}
		if f.ControllerName != "" {
			meta = append(meta, "controller="+f.ControllerName)
		}
		dim("  " + strings.Join(meta, " · "))
		for _, pv := range f.PicklistValues {
			marker := "  "
			label := pv.Label
			if pv.Value != "" && pv.Value != pv.Label {
				label = fmt.Sprintf("%s (%s)", pv.Label, pv.Value)
			}
			if pv.DefaultValue {
				marker = "  ★ "
			} else if !pv.Active {
				marker = "  · "
				label = lipgloss.NewStyle().Foreground(theme.FgDim).Strikethrough(true).Render(label)
			}
			yankVal := pv.Value
			if yankVal == "" {
				yankVal = pv.Label
			}
			rows = append(rows, fieldDetailRow{
				Text:      ansi.Truncate(marker+lipgloss.NewStyle().Foreground(theme.Fg).Render(label), inner, "…"),
				Navigable: true,
				ActionIdx: noAction,
				YankValue: yankVal,
			})
		}
		blank()
	}

	if f.CalculatedFormula != "" {
		title("FORMULA")
		// One fieldDetailRow per wrapped line — the row model assumes
		// one rendered line per row (cursor math), so a single
		// multi-line row would corrupt scrolling. The pane is cursor-
		// scrollable, so long formulas are fully reachable (vertical
		// truncation isn't needed here).
		for _, l := range strings.Split(wrap(f.CalculatedFormula, inner-4), "\n") {
			dim("  " + strings.TrimLeft(l, " "))
		}
		blank()
	}

	// SECURITY / BEHAVIOR — read-only flags.
	secFlags := []struct {
		k string
		v bool
	}{
		{"encrypted", f.Encrypted},
		{"case sensitive", f.CaseSensitive},
		{"auto number", f.AutoNumber},
		{"html formatted", f.HTMLFormatted},
	}
	secShown := false
	for _, fl := range secFlags {
		if fl.v {
			if !secShown {
				title("SECURITY / BEHAVIOR")
				secShown = true
			}
			kv(fl.k, "yes", noAction)
		}
	}
	if secShown {
		blank()
	}

	title("SOQL")
	kv("filterable", yesNo(f.Filterable), noAction)
	kv("sortable", yesNo(f.Sortable), noAction)
	kv("groupable", yesNo(f.Groupable), noAction)
	kv("aggregatable", yesNo(f.Aggregatable), noAction)
	kv("createable", yesNo(f.Createable), noAction)
	kv("updateable", yesNo(f.Updateable), noAction)

	// DANGER ZONE — the destructive delete, isolated at the bottom so
	// it's never fired by accident while toggling above.
	blank()
	rows = append(rows, fieldDetailRow{Text: redLine("DANGER ZONE")})
	rows = append(rows, fieldDetailRow{
		Text:      "  " + redLine("delete field"),
		Navigable: true,
		ActionIdx: fieldActDelete,
		Danger:    true,
		Locked:    locked(fieldActDelete),
	})

	return rows
}

func (m Model) renderFieldDetail(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	d := m.ensureOrgDataRef(o.Username)
	if d.DescribeCur == "" || d.FieldCur == "" {
		return theme.Subtle.Render("  press enter on a field in /objects → schema first")
	}
	r, ok := d.Describes[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		return theme.Subtle.Render("  loading describe…")
	}
	f, ok := findFieldByName(r.Value().Fields, d.FieldCur)
	if !ok {
		return theme.Subtle.Render("  field not found (may have been renamed)")
	}

	desc, descLoaded := fieldDescriptionCache(d, d.DescribeCur, d.FieldCur)
	rows := fieldDetailRows(d.DescribeCur, f, desc, descLoaded, inner)

	navAbs := fieldDetailNavIndex(rows)
	curNav := m.fieldActionCur
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
		out[i] = renderFieldDetailLine(row, i == cursorRow, active, inner)
	}
	return scrollLinesToCursor(out, cursorRow, innerH)
}

func renderFieldDetailLine(row fieldDetailRow, cursored, active bool, inner int) string {
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
		hintTxt := fieldActionHint(row.ActionIdx)
		if row.Locked {
			hintTxt = "  (custom fields only)"
		}
		hint := lipgloss.NewStyle().Foreground(theme.FgDim).Render(hintTxt)
		if ansi.StringWidth(line)+ansi.StringWidth(hintTxt) <= inner {
			line += hint
		}
	}
	return line
}

func fieldActionHint(idx int) string {
	switch idx {
	case fieldActRequired, fieldActUnique, fieldActExternalID:
		return "  ↵ toggle"
	case fieldActDelete:
		return "  ⚠ ↵ delete"
	default:
		return "  ↵ edit"
	}
}

func fieldDetailNavIndex(rows []fieldDetailRow) []int {
	var idx []int
	for i, row := range rows {
		if row.Navigable {
			idx = append(idx, i)
		}
	}
	return idx
}

func (m Model) fieldDetailRowModel() ([]fieldDetailRow, bool) {
	o, ok := m.currentOrg()
	if !ok {
		return nil, false
	}
	d := m.data[o.Username]
	if d == nil || d.DescribeCur == "" || d.FieldCur == "" {
		return nil, false
	}
	r, ok := d.Describes[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		return nil, false
	}
	f, ok := findFieldByName(r.Value().Fields, d.FieldCur)
	if !ok {
		return nil, false
	}
	return fieldDetailRows(d.DescribeCur, f, "", true, 60), true
}

// fieldDetailNavCount is the number of navigable rows for the current
// Model state — used to clamp the row cursor.
func (m Model) fieldDetailNavCount() int {
	rows, ok := m.fieldDetailRowModel()
	if !ok {
		return 0
	}
	return len(fieldDetailNavIndex(rows))
}

func (m Model) fieldDetailActionForCursor() (int, bool) {
	rows, ok := m.fieldDetailRowModel()
	if !ok {
		return noAction, false
	}
	navAbs := fieldDetailNavIndex(rows)
	cur := m.fieldActionCur
	if cur < 0 || cur >= len(navAbs) {
		return noAction, false
	}
	row := rows[navAbs[cur]]
	if row.ActionIdx < 0 {
		return noAction, false
	}
	return row.ActionIdx, true
}

func (m Model) cursoredFieldDetailYankValue() (string, bool) {
	if m.tab() != TabFieldDetail {
		return "", false
	}
	rows, ok := m.fieldDetailRowModel()
	if !ok {
		return "", false
	}
	navAbs := fieldDetailNavIndex(rows)
	cur := m.fieldActionCur
	if cur < 0 || cur >= len(navAbs) {
		return "", false
	}
	if v := rows[navAbs[cur]].YankValue; v != "" {
		return v, true
	}
	return "", false
}

func findFieldByName(fields []sf.Field, name string) (sf.Field, bool) {
	for _, f := range fields {
		if f.Name == name {
			return f, true
		}
	}
	return sf.Field{}, false
}

func summaryKind(f sf.Field) string {
	parts := []string{}
	if f.Custom {
		parts = append(parts, "custom")
	} else {
		parts = append(parts, "standard")
	}
	if f.NameField {
		parts = append(parts, "name field")
	}
	if f.AutoNumber {
		parts = append(parts, "auto-number")
	}
	if f.Encrypted {
		parts = append(parts, "encrypted")
	}
	if f.CalculatedFormula != "" {
		parts = append(parts, "formula")
	}
	return strings.Join(parts, " · ")
}
