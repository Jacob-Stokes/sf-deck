package ui

import (
	"fmt"
	"sort"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// Per-surface sidebars for the code tabs: /apex (classes + triggers),
// /flows (definitions + versions), and /soql (editor + schema).
// Split out of sidebar.go.

// sidebarApex routes the right-pane render for /apex by active
// subtab. Classes + Triggers each get their own renderer; VF Pages
// + Components fall through to an empty placeholder until they
// have detail data.
func (m Model) sidebarApex(inner int) string {
	switch m.currentSubtab() {
	case SubtabApexClasses:
		return m.sidebarApexClass(inner)
	case SubtabApexTriggers:
		return m.sidebarApexTrigger(inner)
	}
	return sideEmpty("—")
}

// sidebarApexClass renders the cursored Apex class's metadata in
// the right pane. Pills above the kv rows show managed-package
// status + IsValid so they're scannable without parsing column
// values.
func (m Model) sidebarApexClass(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}
	a, ok := d.ApexClassList.Selected()
	if !ok {
		return sideEmpty("no matches")
	}
	rows := []kv{
		{"id", a.ID},
		{"status", dashIfEmpty(a.Status)},
		{"valid", yesNo(a.IsValid)},
	}
	if a.NamespacePrefix != "" {
		rows = append(rows, kv{"namespace", a.NamespacePrefix})
	}
	if a.ApiVersion > 0 {
		rows = append(rows, kv{"api version", fmt.Sprintf("v%.1f", a.ApiVersion)})
	}
	if a.LengthNoComments > 0 {

		rows = append(rows, kv{"size", fmt.Sprintf("%s (%d chars)", compactChars(a.LengthNoComments), a.LengthNoComments)})
	}
	if a.LastModifiedDate != "" {
		rows = append(rows, kv{"modified", prettyDate(a.LastModifiedDate)})
	}
	var extra []string
	extra = append(extra, m.sidebarTagsProjectsExtra(devproject.KindApexClass, a.ID, o.Username, inner)...)
	extra = append(extra, "", sideDim("  ↵ open body · "+
		firstPretty(Keys.OpenDefault)+" Lightning", inner))
	return m.kvPanelTagged(inner, a.Name, markPillsForApexClass(a),
		devproject.KindApexClass, a.ID, o.Username, rows, extra...)
}

// sidebarApexTrigger is sidebarApexClass's twin for the Triggers
// subtab. Same shape; trigger-specific fields swap in (parent
// sObject, events).
func (m Model) sidebarApexTrigger(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}
	t, ok := d.ApexTriggerList.Selected()
	if !ok {
		return sideEmpty("no matches")
	}
	rows := []kv{
		{"id", t.ID},
		{"sobject", t.Table},
		{"status", dashIfEmpty(t.Status)},
		{"valid", yesNo(t.Valid)},
	}
	if t.NamespacePrefix != "" {
		rows = append(rows, kv{"namespace", t.NamespacePrefix})
	}
	if t.Events != "" {
		rows = append(rows, kv{"events", t.Events})
	}
	if t.ApiVer > 0 {
		rows = append(rows, kv{"api version", fmt.Sprintf("v%.1f", t.ApiVer)})
	}
	if t.Len > 0 {
		rows = append(rows, kv{"lines", fmt.Sprintf("%d", t.Len)})
	}
	var extra []string
	extra = append(extra, m.sidebarTagsProjectsExtra(devproject.KindApexTrigger, t.ID, o.Username, inner)...)
	extra = append(extra, "", sideDim("  ↵ open trigger detail · "+
		firstPretty(Keys.OpenDefault)+" Lightning", inner))
	return m.kvPanelTagged(inner, t.Name, markPillsForApexTrigger(t),
		devproject.KindApexTrigger, t.ID, o.Username, rows, extra...)
}

func (m Model) sidebarFlow(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}
	f, ok := d.FlowList.Selected()
	if !ok {
		return sideEmpty("no matches")
	}
	rows := []kv{
		{"dev name", f.DeveloperName},
		{"label", f.MasterLabel},
		{"type", f.ProcessType},
		{"status", f.Status},
	}
	if f.Namespace != "" {
		rows = append(rows, kv{"namespace", f.Namespace})
	}
	if f.APIVersion > 0 {
		rows = append(rows, kv{"api version", fmt.Sprintf("v%d", f.APIVersion)})
	}
	if f.ActiveVersionNum > 0 {
		rows = append(rows, kv{"active", fmt.Sprintf("v%d", f.ActiveVersionNum)})
	}
	if f.LatestVersionNum > 0 {
		rows = append(rows, kv{"latest", fmt.Sprintf("v%d", f.LatestVersionNum)})
	}
	if f.ActiveVersionID != "" {
		rows = append(rows, kv{"active id", f.ActiveVersionID})
	}
	if f.LatestVersionID != "" && f.LatestVersionID != f.ActiveVersionID {
		rows = append(rows, kv{"latest id", f.LatestVersionID})
	}
	rows = append(rows, kv{"def id", f.DefinitionID})
	if f.LastModifiedDate != "" {
		rows = append(rows, kv{"modified", prettyDate(f.LastModifiedDate) + " · " + f.LastModifiedBy})
	}

	if f.CreatedDate != "" {
		val := prettyDate(f.CreatedDate)
		if f.CreatedBy != "" {
			val += " · " + f.CreatedBy
		}
		rows = append(rows, kv{"created", val})
	}

	extra := []string{}
	if f.Description != "" {
		extra = append(extra, "", sideSection("description"),
			sideDim("  "+wrap(f.Description, inner-2), inner))
	}
	extra = append(extra, m.sidebarTagsProjectsExtra(devproject.KindFlow, f.DefinitionID, o.Username, inner)...)
	extra = append(extra, "", sideDim("  ↵ versions  ·  "+
		firstPretty(Keys.OpenDefault)+" Flow Builder  ·  "+
		firstPretty(Keys.YankDefault)+" copy URL", inner))
	title := f.DeveloperName
	if title == "" {
		title = "Flow"
	}
	return m.kvPanelTagged(inner, title, markPillsForFlow(f),
		devproject.KindFlow, f.DefinitionID, o.Username, rows, extra...)
}

func (m Model) sidebarFlowVersion(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil || d.FlowCur == "" {
		return sideEmpty("—")
	}
	r, ok := d.FlowVersions[d.FlowCur]
	if !ok || r.FetchedAt().IsZero() {
		return sideEmpty("loading…")
	}
	versions := r.Value()
	if len(versions) == 0 {
		return sideEmpty("no versions")
	}
	idx := d.Cursors.Get(cursorKindFlowVersion, len(versions), d.FlowCur)
	v := versions[idx]

	// Active marker.
	var header sf.Flow
	for _, ff := range d.Flows.Value() {
		if ff.DefinitionID == d.FlowCur {
			header = ff
			break
		}
	}
	activeMark := ""
	if v.ID == header.ActiveVersionID {
		activeMark = " ★ active"
	}

	rows := []kv{
		{"version", fmt.Sprintf("v%d%s", v.VersionNumber, activeMark)},
		{"status", v.Status},
		{"type", v.ProcessType},
		{"api version", fmt.Sprintf("v%d", v.APIVersion)},
		{"created", prettyDate(v.CreatedDate) + " · " + v.CreatedBy},
		{"modified", prettyDate(v.LastModifiedDate) + " · " + v.LastModifiedBy},
		{"version id", v.ID},
	}
	extra := []string{}
	if v.Description != "" {
		extra = append(extra, "", sideSection("description"),
			sideDim("  "+wrap(v.Description, inner-2), inner))
	}
	extra = append(extra, "", sideDim(
		"  "+firstPretty(Keys.OpenDefault)+" → Flow Builder · "+
			firstPretty(Keys.OpenMenu)+" → pick target", inner))
	return renderKVPanel(inner, "Flow v"+itoaFn(v.VersionNumber), rows, extra...)
}

func (m Model) sidebarSOQL(inner int) string {

	if m.soqlEditing {
		if panel := m.sidebarSOQLSchema(inner); panel != "" {
			return panel
		}
	}
	if len(m.soqlResult.Records) == 0 {
		var extra []string
		if len(m.soqlHistory) > 0 {
			extra = append(extra, "", sideSection("history"))
			for i := len(m.soqlHistory) - 1; i >= 0 && i >= len(m.soqlHistory)-10; i-- {
				extra = append(extra, sideDim("  "+m.soqlHistory[i], inner))
			}
		}
		return renderKVPanel(inner, "SOQL", nil, append([]string{sideDim("  no results", inner)}, extra...)...)
	}
	rec, ok := m.soqlSelectedRecord()
	if !ok {
		return renderKVPanel(inner, "SOQL", nil, sideDim("  no results", inner))
	}
	d, _ := m.activeOrgState()
	entry := soqlProjectionFor(d, m.soqlResult.Records, m.soqlSearchPtr(), theme.Current.ID, m.soqlInput.Value())
	idx := soqlTableAdapter(&m, entry).DisplayCursor()
	keys := make([]string, 0, len(rec))
	for k := range rec {
		if k == "attributes" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := []kv{
		{"rows", fmt.Sprintf("%d", m.soqlResult.TotalSize)},
		{"row", fmt.Sprintf("%d / %d", idx+1, len(entry.filtered))},
	}
	extra := []string{"", sideSection("record")}
	for _, k := range keys {
		extra = append(extra, sideKV(k, formatCell(rec[k]), inner))
	}
	return renderKVPanel(inner, "SOQL", rows, extra...)
}

// sidebarSOQLSchema renders the describe of the FROM sObject (when
// resolvable) as a scannable field reference. Returns "" when the
// query has no FROM yet OR the describe isn't loaded yet — caller
// falls through to the result-row sidebar in that case.
//
// Layout: header with sObject name + label, then per-field rows
// showing `apiName  type  badges`. Reference fields show their
// target sObject; picklist fields show value count; required
// fields get a `*` marker.
func (m Model) sidebarSOQLSchema(inner int) string {
	query := m.soqlInput.Value()
	sobject := extractFromSObject(query)
	if sobject == "" {
		return renderKVPanel(inner, "Schema", nil,
			sideDim("  type FROM <sObject> to see schema", inner))
	}
	d, _ := m.activeOrgState()
	if d == nil {
		return renderKVPanel(inner, "Schema", nil, sideDim("  no org", inner))
	}

	o, ok := m.currentOrg()
	if !ok {
		return renderKVPanel(inner, "Schema", nil, sideDim("  no org", inner))
	}
	r := d.EnsureDescribe(targetArg(o), sobject)
	if r == nil {
		return renderKVPanel(inner, "Schema", nil, sideDim("  describe failed", inner))
	}
	if r.FetchedAt().IsZero() {

		return renderKVPanel(inner, "Schema · "+sobject, nil,
			sideDim("  loading describe… (type any key to dispatch fetch)", inner))
	}
	desc := r.Value()
	rows := []kv{
		{"sObject", desc.Name},
		{"label", desc.Label},
		{"fields", fmt.Sprintf("%d", len(desc.Fields))},
	}

	fields := make([]sf.Field, len(desc.Fields))
	copy(fields, desc.Fields)
	sort.Slice(fields, func(i, j int) bool {
		rank := func(f sf.Field) int {
			if f.Name == "Id" {
				return 0
			}
			if f.NameField {
				return 1
			}
			return 2
		}
		ri, rj := rank(fields[i]), rank(fields[j])
		if ri != rj {
			return ri < rj
		}
		return fields[i].Name < fields[j].Name
	})
	extra := []string{"", sideSection("fields")}
	for _, f := range fields {
		extra = append(extra, renderSchemaFieldRow(f, inner))
	}
	return renderKVPanel(inner, "Schema", rows, extra...)
}
