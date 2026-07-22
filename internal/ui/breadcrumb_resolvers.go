package ui

// Per-tab breadcrumb resolvers — the path segments rendered after
// the tab name in the header. Wired into TabSpec.Breadcrumb /
// SubtabSpec.Breadcrumb in tab_registry.go.
//
// Segments compose (parent → cursored item) so a user can read the
// header and know exactly where they are without consulting any
// in-pane title. Each closure returns nil when there's no useful
// context to surface (no org, no cursor, fetch in flight).
//
// Surfaces whose breadcrumb is just "the cursored item label"
// often reuse identity-based helpers below.

import "fmt"

// breadcrumbFromObjectDetail builds the multi-segment breadcrumb
// for the object-drill tab: sObject name → subtab label → cursored
// row. Keeps the per-subtab branch self-contained because each
// subtab has a different "cursored row" shape.
func breadcrumbFromObjectDetail(m Model) []string {
	d := m.activeOrgData()
	if d == nil || d.DescribeCur == "" {
		return nil
	}
	out := []string{d.DescribeCur}

	subs := objectDrillSubtabs()
	if len(subs) > 1 {
		sel := m.objectSubtab()
		if sel >= 0 && sel < len(subs) {
			out = append(out, subs[sel].Label)
		}
	}

	switch m.currentSubtab() {
	case SubtabSchema:
		if r, ok := d.Describes[d.DescribeCur]; ok {
			if f, ok := d.cursoredField(d.DescribeCur, r); ok {
				out = append(out, f.Name)
			}
		}
	case SubtabRecords:
		idx := recordsCursorDisplay(d, d.DescribeCur)
		if rec, ok := currentRecordAt(d, d.DescribeCur, idx); ok {
			if name, ok := rec["Name"].(string); ok && name != "" {
				out = append(out, name)
			} else if id, ok := rec["Id"].(string); ok {
				out = append(out, id)
			}
		}
	}
	return out
}

// breadcrumbFromObjectsList — the cursored sObject's API name.
func breadcrumbFromObjectsList(m Model) []string {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	if sel, ok := d.SObjectList.Selected(); ok {
		return []string{sel.Name}
	}
	return nil
}

// breadcrumbFromFieldDetail — sObject → field name.
func breadcrumbFromFieldDetail(m Model) []string {
	d := m.activeOrgData()
	if d == nil || d.DescribeCur == "" {
		return nil
	}
	out := []string{d.DescribeCur}
	if d.FieldCur != "" {
		out = append(out, d.FieldCur)
	}
	return out
}

// breadcrumbFromValidationDetail — sObject → rule name.
func breadcrumbFromValidationDetail(m Model) []string {
	d := m.activeOrgData()
	if d == nil || d.DescribeCur == "" {
		return nil
	}
	out := []string{d.DescribeCur}
	if r, ok := d.ValidationRules.Lists[d.DescribeCur]; ok {
		for _, vr := range r.Value() {
			if vr.ID == d.ValidationRules.DrillID {
				out = append(out, vr.ValidationName)
				break
			}
		}
	}
	return out
}

// breadcrumbFromRecordTypeDetail — sObject → record type dev name.
func breadcrumbFromRecordTypeDetail(m Model) []string {
	d := m.activeOrgData()
	if d == nil || d.DescribeCur == "" {
		return nil
	}
	out := []string{d.DescribeCur}
	if r, ok := d.RecordTypes.Lists[d.DescribeCur]; ok {
		for _, rt := range r.Value() {
			if rt.ID == d.RecordTypes.DrillID {
				out = append(out, rt.DeveloperName)
				break
			}
		}
	}
	return out
}

// breadcrumbFromTriggerDetail — sObject → trigger name.
func breadcrumbFromTriggerDetail(m Model) []string {
	d := m.activeOrgData()
	if d == nil || d.DescribeCur == "" {
		return nil
	}
	out := []string{d.DescribeCur}
	if r, ok := d.Triggers.Lists[d.DescribeCur]; ok {
		for _, t := range r.Value() {
			if t.ID == d.Triggers.DrillID {
				out = append(out, t.Name)
				break
			}
		}
	}
	return out
}

// breadcrumbFromRecords — picker mode shows just the cursored
// sObject; record-list mode shows sObject → record name/id.
func breadcrumbFromRecords(m Model) []string {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	if d.RecordsSObjectCur == "" {
		if sel, ok := d.SObjectList.Selected(); ok {
			return []string{sel.Name}
		}
		return nil
	}
	out := []string{d.RecordsSObjectCur}
	idx := recordsCursorDisplay(d, d.RecordsSObjectCur)
	if rec, ok := currentRecordAt(d, d.RecordsSObjectCur, idx); ok {
		if name, ok := rec["Name"].(string); ok && name != "" {
			out = append(out, name)
		} else if id, ok := rec["Id"].(string); ok {
			out = append(out, id)
		}
	}
	return out
}

// breadcrumbFromFlows — cursored flow's DeveloperName.
func breadcrumbFromFlows(m Model) []string {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	if sel, ok := d.FlowList.Selected(); ok {
		return []string{sel.DeveloperName}
	}
	return nil
}

// breadcrumbFromFlowDetail — flow DeveloperName → version label.
func breadcrumbFromFlowDetail(m Model) []string {
	d := m.activeOrgData()
	if d == nil || d.FlowCur == "" {
		return nil
	}
	var out []string
	for _, f := range d.Flows.Value() {
		if f.DefinitionID == d.FlowCur {
			out = append(out, f.DeveloperName)
			break
		}
	}
	if r, ok := d.FlowVersions[d.FlowCur]; ok {
		versions := r.Value()
		idx := d.Cursors.Get(cursorKindFlowVersion, len(versions), d.FlowCur)
		if idx < len(versions) {
			out = append(out, fmt.Sprintf("v%d", versions[idx].VersionNumber))
		}
	}
	return out
}

// breadcrumbFromReportDetail — folder → report name.
func breadcrumbFromReportDetail(m Model) []string {
	d := m.activeOrgData()
	if d == nil || d.ReportCur == "" {
		return nil
	}
	for _, r := range d.Reports.Value() {
		if r.ID == d.ReportCur {
			out := []string{}
			if r.FolderName != "" {
				out = append(out, r.FolderName)
			}
			out = append(out, r.Name)
			return out
		}
	}
	return nil
}

// breadcrumbFromRecordDetail — sObject → resolved record display
// name (Name field, falling back to Id).
func breadcrumbFromRecordDetail(m Model) []string {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return nil
	}
	sobj, id := splitRecordKey(d.RecordDetailCur)
	out := []string{}
	if sobj != "" {
		out = append(out, sobj)
	}
	name := id
	if r, ok := d.RecordDetails[d.RecordDetailCur]; ok && !r.FetchedAt().IsZero() {
		if dn := recordDisplayName(r.Value()); dn != "" {
			name = dn
		}
	}
	if name != "" {
		out = append(out, name)
	}
	return out
}
