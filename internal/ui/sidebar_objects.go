package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// Per-surface sidebars for /objects + object-detail subtabs: object
// list/detail, records, fields (+ FLS actions), record types,
// triggers, validation rules. Split out of sidebar.go.

// sidebarObjectDetailDispatch routes to the right per-subtab sidebar
// for ObjectDetail, whose subtabs aren't declared in TabSpec.Subtabs
// (they fan out from a render-time switch instead). Mirror of the
// Identity dispatcher; explicit branches per subtab — no default
// fallback so a future SubtabXX without an entry no-ops cleanly.
func sidebarObjectDetailDispatch(m Model, inner int) string {
	switch m.currentSubtab() {
	case SubtabDetails:
		return m.sidebarObjectActions(inner)
	case SubtabSchema:
		return m.sidebarField(inner)
	case SubtabValidation:
		return m.sidebarValidationRule(inner)
	case SubtabRecordTypes:
		return m.sidebarRecordType(inner)
	case SubtabTriggers:
		return m.sidebarTrigger(inner)
	case SubtabFLS:
		return m.sidebarFLS(inner)
	case SubtabRecords:
		return m.sidebarObjectDetailRecord(inner)
	}
	return ""
}

// sidebarObjectDetailRecord is the Object-drill Records-subtab sidebar.
// Shows the selected row's full KV. Source depends on the active chip:
// synthetic "recent" pulls from d.Records; a Salesforce list view
// pulls from d.ListViewResults.
func (m Model) sidebarObjectDetailRecord(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil || d.DescribeCur == "" {
		return sideEmpty("—")
	}
	idx := recordsCursorDisplay(d, d.DescribeCur)
	rec, ok := currentRecordAt(d, d.DescribeCur, idx)
	if !ok {
		return sideEmpty("loading…")
	}
	title := d.DescribeCur
	if name, ok := rec["Name"].(string); ok && name != "" {
		title = name
	}
	var rows []kv
	if id, ok := rec["Id"].(string); ok {
		rows = append(rows, kv{"Id", id})
	}
	var keys []string
	for k := range rec {
		if k == "attributes" || k == "Id" {
			continue
		}
		keys = append(keys, k)
	}
	sortStrings(keys)
	for _, k := range keys {
		rows = append(rows, kv{k, formatCell(rec[k])})
	}
	return renderKVPanel(inner, title, rows)
}

// sidebarRecords shows the full selected record as KV when in record-
// list mode, or the selected sObject's summary when in picker mode.
func (m Model) sidebarRecords(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}

	if d.RecordsSObjectCur == "" {
		return m.sidebarObjects(inner)
	}

	if currentRecordRowCount(d, d.RecordsSObjectCur) == 0 {
		return sideEmpty("no records")
	}
	idx := recordsCursorDisplay(d, d.RecordsSObjectCur)
	rec, ok2 := currentRecordAt(d, d.RecordsSObjectCur, idx)
	if !ok2 {
		return sideEmpty("loading…")
	}
	title := d.RecordsSObjectCur
	if name, ok := rec["Name"].(string); ok && name != "" {
		title = name
	}
	var rows []kv

	if id, ok := rec["Id"].(string); ok {
		rows = append(rows, kv{"Id", id})
	}
	var keys []string
	for k := range rec {
		if k == "attributes" || k == "Id" {
			continue
		}
		keys = append(keys, k)
	}

	sortStrings(keys)
	for _, k := range keys {
		rows = append(rows, kv{k, formatCell(rec[k])})
	}
	return renderKVPanel(inner, title, rows)
}

func (m Model) sidebarObjects(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil || d.SObjects.FetchedAt().IsZero() {
		return sideEmpty("—")
	}
	it, ok := d.SObjectList.Selected()
	if !ok {
		return sideEmpty("no matches")
	}

	kind := "standard"
	if sf.IsCustom(it.Name) {
		kind = "custom"
	}
	setup := "system"
	if it.IsCustomizable {
		setup = "yes (manageable)"
	}

	rows := []kv{
		{"kind", kind},
		{"setup", setup},
	}
	if it.Label != "" && it.Label != it.Name {
		rows = append([]kv{{"label", it.Label}}, rows...)
	}

	if it.KeyPrefix != "" {
		rows = append(rows, kv{"key prefix", it.KeyPrefix})
	}
	if it.Namespace != "" {
		rows = append(rows, kv{"namespace", it.Namespace})
	}
	if it.DeploymentStatus != "" && it.DeploymentStatus != "Deployed" {
		rows = append(rows, kv{"deployment", it.DeploymentStatus})
	}
	rows = append(rows,
		kv{"triggerable", yesNo(it.ApexTriggerable)},
		kv{"workflow", yesNo(it.WorkflowEnabled)},
	)
	if it.LastModifiedDate != "" {
		rows = append(rows, kv{"modified", prettyDate(it.LastModifiedDate)})
	}

	// If we have a cached describe for this one, show the deeper
	// caps split into individual rows (the "QCUD" mnemonic is opaque
	// without context). Each cap also implies what actions are safe
	// to wire up on this sObject.
	var extra []string
	if r, ok := d.Describes[it.Name]; ok && !r.FetchedAt().IsZero() {
		v := r.Value()
		extra = append(extra, "", sideSection(fmt.Sprintf("schema · %d fields", len(v.Fields))))
		extra = append(extra, sideKV("queryable", yesNo(v.Queryable), inner))
		extra = append(extra, sideKV("createable", yesNo(v.Creatable), inner))
		extra = append(extra, sideKV("updateable", yesNo(v.Updatable), inner))
		extra = append(extra, sideKV("deleteable", yesNo(v.Deletable), inner))
	}
	extra = append(extra, m.sidebarTagsProjectsExtra(devproject.KindSObject, it.Name, o.Username, inner)...)
	extra = append(extra, "", sideDim("  ↵ open fields  ·  "+
		firstPretty(Keys.OpenDefault)+" Lightning  ·  "+
		firstPretty(Keys.YankDefault)+" copy URL", inner))
	return m.kvPanelTagged(inner, it.Name, markPillsForSObject(it.Name),
		devproject.KindSObject, it.Name, o.Username, rows, extra...)
}

// sidebarField is the full "everything about this field" detail view.
// Structured as Object-Manager-style sections (IDENTITY · CONSTRAINTS ·
// REFERENCE · PICKLIST · FORMULA · SECURITY · SOQL) so admins don't
// have to scan a single flat kv dump.
//
// Each section is optional — reference-less fields omit REFERENCE,
// non-picklists omit PICKLIST, and so on.
func (m Model) sidebarField(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil || d.DescribeCur == "" {
		return sideEmpty("—")
	}
	r, ok := d.Describes[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {

		if r != nil && r.Err() != nil {
			return sideEmpty("describe failed — see main pane")
		}
		return sideEmpty("loading describe…")
	}
	f, ok := d.cursoredField(d.DescribeCur, r)
	if !ok {
		return sideEmpty("no matches")
	}

	// Each section builds a []kv (or plain strings) and is joined at
	// the end. Sections separate with a blank line + dim title.
	fieldRef := r.Value().Name + "." + f.Name
	var out []string
	// In stacked mode fold tags/projects into the title line (they're
	// suppressed from the body below by sidebarTagsProjectsSection); in
	// RHS mode the plain title stays and the body section renders.
	titleLine := sideTitle(f.Name)
	if m.compactSidebarPills() {
		titleLine = m.stackedTitleWithTagsProjects(sideTitle(f.Name), "",
			devproject.KindField, fieldRef, o.Username, inner)
	}
	out = append(out, titleLine, sideSeparator(inner))

	identity := []kv{}
	if f.Label != "" && f.Label != f.Name {
		identity = append(identity, kv{"label", f.Label})
	}
	identity = append(identity, kv{"on", r.Value().Name})
	identity = append(identity, kv{"type", sidebarFieldTypeDisplay(f)})
	if f.SoapType != "" && f.SoapType != f.Type {
		identity = append(identity, kv{"soap type", f.SoapType})
	}
	kindParts := []string{}
	if f.Custom {
		kindParts = append(kindParts, "custom")
	} else {
		kindParts = append(kindParts, "standard")
	}
	if f.NameField {
		kindParts = append(kindParts, "name field")
	}
	identity = append(identity, kv{"kind", strings.Join(kindParts, " · ")})
	for _, r := range identity {
		if r.V == "" {
			continue
		}
		out = append(out, sideKV(r.K, r.V, inner))
	}

	constraints := []kv{
		{"required", yesNo(!f.Nillable)},
		{"unique", yesNo(f.Unique)},
		{"external id", yesNo(f.ExternalID)},
	}
	if f.Length > 0 {
		constraints = append(constraints, kv{"length", fmt.Sprintf("%d", f.Length)})
	}
	if f.Precision > 0 || f.Scale > 0 {
		constraints = append(constraints,
			kv{"precision/scale", fmt.Sprintf("%d / %d", f.Precision, f.Scale)})
	}
	if f.Digits > 0 {
		constraints = append(constraints, kv{"digits", fmt.Sprintf("%d", f.Digits)})
	}
	out = append(out, "", sideSection("constraints"))
	for _, r := range constraints {
		out = append(out, sideKV(r.K, r.V, inner))
	}

	if len(f.ReferenceTo) > 0 || f.RelationshipName != "" {
		out = append(out, "", sideSection("reference"))
		out = append(out, sideKV("targets", strings.Join(f.ReferenceTo, ", "), inner))
		if f.RelationshipName != "" {
			out = append(out, sideKV("relationship", f.RelationshipName, inner))
		}
		if f.CascadeDelete {
			out = append(out, sideKV("on delete", "cascade", inner))
		} else if f.RestrictedDelete {
			out = append(out, sideKV("on delete", "restricted", inner))
		}
		if f.WriteRequiresMasterRead {
			out = append(out, sideKV("write-requires-master-read", "yes", inner))
		}
	}

	if len(f.PicklistValues) > 0 {
		out = append(out, "",
			sideSection(fmt.Sprintf("picklist · %d values", len(f.PicklistValues))))
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
		out = append(out, sideDim("  "+strings.Join(meta, " · "), inner))
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
			out = append(out, ansi.Truncate(
				marker+lipgloss.NewStyle().Foreground(theme.Fg).Render(label),
				inner, "…"))
		}
	}

	if f.CalculatedFormula != "" {
		out = append(out, "", sideSection("formula"),
			sideDim("  "+wrap(f.CalculatedFormula, inner-2), inner))
	}
	if dv, ok := stringish(f.DefaultValue); ok && dv != "" {
		out = append(out, "", sideSection("default value"),
			sideDim("  "+wrap(dv, inner-2), inner))
	}
	if f.DefaultValueFormula != "" {
		out = append(out, "", sideSection("default formula"),
			sideDim("  "+wrap(f.DefaultValueFormula, inner-2), inner))
	}

	security := []kv{}
	if f.Encrypted {
		security = append(security, kv{"encrypted", "yes"})
	}
	if f.CaseSensitive {
		security = append(security, kv{"case sensitive", "yes"})
	}
	if f.AutoNumber {
		security = append(security, kv{"auto number", "yes"})
	}
	if f.HTMLFormatted {
		security = append(security, kv{"html formatted", "yes"})
	}
	if len(security) > 0 {
		out = append(out, "", sideSection("security / behavior"))
		for _, r := range security {
			out = append(out, sideKV(r.K, r.V, inner))
		}
	}

	out = append(out, "", sideSection("soql"))
	out = append(out, sideKV("filterable", yesNo(f.Filterable), inner))
	out = append(out, sideKV("sortable", yesNo(f.Sortable), inner))
	out = append(out, sideKV("groupable", yesNo(f.Groupable), inner))
	out = append(out, sideKV("aggregatable", yesNo(f.Aggregatable), inner))
	out = append(out, sideKV("createable", yesNo(f.Createable), inner))
	out = append(out, sideKV("updateable", yesNo(f.Updateable), inner))

	if f.InlineHelpText != "" {
		out = append(out, "", sideSection("help"),
			sideDim("  "+wrap(f.InlineHelpText, inner-2), inner))
	}

	// fieldRef computed above. In stacked mode this returns "" (tags/
	// projects were folded into the title); RHS mode renders the body.
	if section := m.sidebarTagsProjectsSection(devproject.KindField, fieldRef, o.Username, inner); section != "" {
		out = append(out, section)
	}

	laid, _ := reflowLinesToBudget(out, inner, m.sidebarFieldBudget())
	return strings.Join(laid, "\n")
}

// sidebarFieldBudget is the content-height the schema-subtab field
// sidebar may use before it should reflow into columns. The title +
// separator that renderKVPanel-style headers add aren't present here
// (sidebarField emits its own title), so the budget is the full
// available sidebar height.
func (m Model) sidebarFieldBudget() int {
	return m.sidebarInnerH
}

// sidebarFieldTypeDisplay matches the TYPE column's virtual-type
// expansion so the sidebar is consistent with the table.
func sidebarFieldTypeDisplay(f sf.Field) string {
	switch {
	case f.AutoNumber:
		return "autonumber"
	case f.Encrypted:
		if f.Type == "string" {
			return "encrypted"
		}
		return "encrypted " + f.Type
	case f.CalculatedFormula != "" && f.Type != "":
		return f.Type + " (formula)"
	}
	if f.Length > 0 && (f.Type == "string" || f.Type == "textarea") {
		return fmt.Sprintf("%s(%d)", f.Type, f.Length)
	}
	if (f.Type == "double" || f.Type == "currency" || f.Type == "percent") &&
		(f.Precision > 0 || f.Scale > 0) {
		return fmt.Sprintf("%s(%d,%d)", f.Type, f.Precision, f.Scale)
	}
	return f.Type
}

// renderSchemaFieldRow formats one field row in the schema sidebar.
// Layout: "  fieldName  type · flags". Reference fields append
// "→ TargetSObject". Picklists append the value count. Wider
// sidebar widths reveal more detail; narrow widths truncate.
func renderSchemaFieldRow(f sf.Field, inner int) string {
	prefix := "  "
	nameStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	typeStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	if f.NameField {
		nameStyle = nameStyle.Foreground(theme.Yellow).Bold(true)
	} else if f.Name == "Id" {
		nameStyle = nameStyle.Foreground(theme.Cyan).Bold(true)
	}
	name := f.Name
	typeStr := f.Type
	if f.Type == "reference" && len(f.ReferenceTo) > 0 {
		typeStr = "→ " + strings.Join(f.ReferenceTo, "|")
	} else if f.Type == "picklist" {
		typeStr = fmt.Sprintf("picklist (%d)", len(f.PicklistValues))
	}
	// Capability badges: only show the ones that DIFFER from the
	// usual defaults (filterable+sortable are common; non-
	// filterable is the notable case).
	var badges []string
	if !f.Filterable {
		badges = append(badges, "noFilter")
	}
	if !f.Sortable {
		badges = append(badges, "noSort")
	}
	if !f.Nillable && !f.AutoNumber && !f.Createable {
		badges = append(badges, "system")
	}
	if f.Custom {
		badges = append(badges, "custom")
	}
	suffix := ""
	if len(badges) > 0 {
		suffix = " · " + strings.Join(badges, " ")
	}
	body := prefix + nameStyle.Render(name) + "  " + typeStyle.Render(typeStr+suffix)

	return ansi.Truncate(body, inner, "…")
}

// sidebarFieldActions is the TabFieldDetail right sidebar — a context
// panel for the cursored field-detail row.
func (m Model) sidebarFieldActions(inner int) string {
	ctx := m.fieldRowContext()
	ctx.Hints = detailNavHints(true)
	return m.sidebarRowContext("FIELD · CONTEXT", inner, ctx)
}

// fieldRowContext builds the context panel for the cursored field
// row. The field actions write via the Tooling CustomField API (not
// the Metadata deploy that object edits use).
func (m Model) fieldRowContext() rowContext {
	idx, ok := m.fieldDetailActionForCursor()
	if !ok {
		return rowContext{ReadOnlyMsg: "describe attribute — read-only. The editable rows (label, help, description, default, required/unique/external-id, delete) carry a ↵ hint."}
	}
	rows := RegistryRows(m, fieldRegistry)
	if idx < 0 || idx >= len(rows) {
		return rowContext{}
	}
	a := rows[idx]
	ctx := rowContext{
		Heading: a.Label,
		Help:    a.Hint,
		Routing: "Tooling API · CustomField patch",
		Danger:  idx == fieldActDelete,
	}
	if !a.Allowed {
		ctx.Blocked = a.Reason
	}
	switch idx {
	case fieldActLabel:
		ctx.Affects = "the field's display label everywhere it's shown."
	case fieldActHelp:
		ctx.Affects = "the (i) help bubble on Lightning record pages."
	case fieldActDescription:
		ctx.Affects = "the Setup-only description."
	case fieldActDefault:
		ctx.Affects = "the value pre-filled on new records."
	case fieldActRequired:
		ctx.Affects = "whether insert/update fails when the field is blank."
	case fieldActUnique:
		ctx.Affects = "whether duplicate values are rejected."
	case fieldActExternalID:
		ctx.Affects = "whether the field can key Bulk/REST upserts."
	case fieldActDelete:
		ctx.Routing = "Tooling API · delete CustomField"
		ctx.Affects = "permanently removes the field AND every record's value. No undo."
	}

	if o, ok := m.currentOrg(); ok {
		if d := m.data[o.Username]; d != nil && d.DescribeCur != "" && d.FieldCur != "" {
			if r, ok := d.Describes[d.DescribeCur]; ok && !r.FetchedAt().IsZero() {
				if f, ok := findFieldByName(r.Value().Fields, d.FieldCur); ok {
					ctx.Current = fieldActionCurrentValue(idx, f)
				}
			}
		}
	}
	return ctx
}

// fieldActionCurrentValue returns the "now:" value for a field action.
func fieldActionCurrentValue(idx int, f sf.Field) string {
	switch idx {
	case fieldActLabel:
		return dashIfEmpty(f.Label)
	case fieldActHelp:
		return dashIfEmpty(f.InlineHelpText)
	case fieldActDescription:
		return "(opens to load)"
	case fieldActDefault:
		if s, ok := stringish(f.DefaultValue); ok && s != "" {
			return s
		}
		return dashIfEmpty(f.DefaultValueFormula)
	case fieldActRequired:
		return yesNo(!f.Nillable)
	case fieldActUnique:
		return yesNo(f.Unique)
	case fieldActExternalID:
		return yesNo(f.ExternalID)
	}
	return ""
}

// sidebarObjectActions is the Details-subtab right sidebar. It is now
// INFO-ONLY: the action menu lives in the main pane (arrow keys walk
// the editable rows; Enter / ctrl+e fires them). This sidebar just
// mirrors the catalog of available actions and highlights whichever
// one the cursored main-pane row maps to, so the user can safely hide
// it without losing the ability to act.
func (m Model) sidebarObjectActions(inner int) string {
	ctx := m.objectRowContext()

	ctx.Hints = []string{
		firstPretty(Keys.OpenDefault) + " Lightning",
		"tab subtabs",
		firstPretty(Keys.Refresh) + " refresh",
	}
	return m.sidebarRowContext("OBJECT · CONTEXT", inner, ctx)
}

// objectRowContext builds the context panel for the cursored Details
// row. Read-only rows explain themselves; editable rows carry the
// deploy routing + consequence so the user knows what a write ships.
func (m Model) objectRowContext() rowContext {
	idx, ok := m.objectDetailActionForCursor()
	if !ok {
		return rowContext{ReadOnlyMsg: "describe field — read-only here. Edit label / description / the FEATURES toggles instead (they carry a ↵ hint)."}
	}
	rows := RegistryRows(m, objectRegistry)
	if idx < 0 || idx >= len(rows) {
		return rowContext{}
	}
	a := rows[idx]
	ctx := rowContext{
		Heading: a.Label,
		Help:    a.Hint,
		Routing: "Metadata API · CustomObject deploy (diff-previewed)",
	}
	if !a.Allowed {
		ctx.Blocked = a.Reason
	}

	if o, ok := m.currentOrg(); ok {
		if d := m.data[o.Username]; d != nil && d.DescribeCur != "" {
			if r, ok := d.Describes[d.DescribeCur]; ok && !r.FetchedAt().IsZero() {
				v := r.Value()
				base, _ := readObjectBaselineForDetails(d, d.DescribeCur)
				ctx.Current = objectActionCurrentValue(idx, v, base)
			}
		}
	}
	switch idx {
	case 0, 1:
		ctx.Affects = "the user-facing name across pages, list views, reports."
	case 2:
		ctx.Affects = "the Setup / Object Manager description only."
	case 3:
		ctx.Affects = "whether records appear in Report Builder."
	case 4:
		ctx.Affects = "whether Tasks + Events can relate — can't cleanly re-disable."
	case 5:
		ctx.Affects = "the Chatter feed panel on records."
	case 6:
		ctx.Affects = "whether per-field history tracking takes effect."
	case 7:
		ctx.Affects = "whether records surface in global search."
	}
	return ctx
}

// objectActionCurrentValue returns the current value string for an
// object action index, used as the "now:" line in the context panel.
func objectActionCurrentValue(idx int, v sf.SObjectDescribe, base *sf.CustomObjectBaseline) string {
	switch idx {
	case 0:
		return dashIfEmpty(v.Label)
	case 1:
		return dashIfEmpty(v.LabelPlural)
	case 2:
		if base != nil {
			return dashIfEmpty(base.Description)
		}
		return "—"
	}
	if base == nil {
		return "unknown"
	}
	switch idx {
	case 3:
		return boolPtrLabel(base.EnableReports)
	case 4:
		return boolPtrLabel(base.EnableActivities)
	case 5:
		return boolPtrLabel(base.EnableFeeds)
	case 6:
		return boolPtrLabel(base.EnableHistory)
	case 7:
		return boolPtrLabel(base.EnableSearch)
	}
	return ""
}

// sidebarRecordType renders a compact summary of the currently-
// selected record type on the Record Types subtab. Drill into a
// record type (enter) to open TabRecordTypeDetail for the full
// Metadata + action menu.
func (m Model) sidebarRecordType(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil || d.DescribeCur == "" {
		return sideEmpty("—")
	}
	r, ok := d.RecordTypes.Lists[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		return sideEmpty("loading…")
	}
	rts := r.Value()
	if len(rts) == 0 {
		return sideEmpty("no record types")
	}
	idx := d.RecordTypes.Cursors[d.DescribeCur]
	if idx < 0 || idx >= len(rts) {
		idx = 0
	}
	rt := rts[idx]
	rows := []kv{
		{"on", d.DescribeCur},
		{"id", rt.ID},
		{"api name", rt.DeveloperName},
		{"label", rt.Name},
		{"active", yesNo(rt.Active)},
	}
	var extra []string
	if rt.Description != "" {
		extra = append(extra, "", sideSection("description"),
			sideDim("  "+wrap(rt.Description, inner-2), inner))
	}
	extra = append(extra, m.sidebarTagsProjectsExtra(devproject.KindRecordType, rt.ID, o.Username, inner)...)
	extra = append(extra, "", sideDim("  ↵ open for full metadata + actions", inner))
	title := rt.DeveloperName
	if title == "" {
		title = rt.Name
	}
	return m.kvPanelTagged(inner, title, nil,
		devproject.KindRecordType, rt.ID, o.Username, rows, extra...)
}

// sidebarTrigger renders a compact summary of the currently-selected
// trigger on the Triggers subtab. Drill (enter) to open
// TabTriggerDetail for the body + action menu.
func (m Model) sidebarTrigger(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil || d.DescribeCur == "" {
		return sideEmpty("—")
	}
	r, ok := d.Triggers.Lists[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		return sideEmpty("loading…")
	}
	trigs := r.Value()
	if len(trigs) == 0 {
		return sideEmpty("no triggers")
	}
	idx := d.Triggers.Cursors[d.DescribeCur]
	if idx < 0 || idx >= len(trigs) {
		idx = 0
	}
	t := trigs[idx]
	status := t.Status
	if !t.Valid && t.Status == "Active" {
		status += " (invalid)"
	}
	rows := []kv{
		{"on", d.DescribeCur},
		{"id", t.ID},
		{"status", status},
		{"api", fmt.Sprintf("%.1f", t.ApiVer)},
		{"length", fmt.Sprintf("%d", t.Len)},
	}
	var extra []string
	if t.Events != "" {
		extra = append(extra, "", sideSection("events"),
			sideDim("  "+wrap(t.Events, inner-2), inner))
	}
	extra = append(extra, m.sidebarTagsProjectsExtra(devproject.KindApexTrigger, t.ID, o.Username, inner)...)
	extra = append(extra, "", sideDim("  ↵ open for body + actions", inner))
	return m.kvPanelTagged(inner, t.Name, nil,
		devproject.KindApexTrigger, t.ID, o.Username, rows, extra...)
}

// sidebarValidationRule renders a compact summary of the currently-
// selected rule on the Validation subtab. Drill into a rule (enter)
// to open TabValidationDetail for the full formula body + action
// menu.
func (m Model) sidebarValidationRule(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil || d.DescribeCur == "" {
		return sideEmpty("—")
	}
	r, ok := d.ValidationRules.Lists[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		return sideEmpty("loading…")
	}
	rules := r.Value()
	if len(rules) == 0 {
		return sideEmpty("no rules")
	}
	idx := d.ValidationRules.Cursors[d.DescribeCur]
	if idx < 0 || idx >= len(rules) {
		idx = 0
	}
	rule := rules[idx]

	rows := []kv{
		{"on", d.DescribeCur},
		{"id", rule.ID},
		{"active", yesNo(rule.Active)},
	}
	var extra []string
	if rule.Description != "" {
		extra = append(extra, "", sideSection("description"),
			sideDim("  "+wrap(rule.Description, inner-2), inner))
	}
	extra = append(extra, m.sidebarTagsProjectsExtra(devproject.KindValidationRule, rule.ID, o.Username, inner)...)
	extra = append(extra, "", sideDim("  ↵ open for full body + actions", inner))
	return m.kvPanelTagged(inner, rule.ValidationName, nil,
		devproject.KindValidationRule, rule.ID, o.Username, rows, extra...)
}
