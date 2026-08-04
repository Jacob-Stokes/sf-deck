package ui

// Per-tab / per-subtab Identity resolvers. Each closure returns the
// (kind, ref, label) of the cursored item on its surface. Wired into
// TabSpec.Identity / SubtabSpec.Identity in tab_registry.go.
//
// Adding a new taggable surface = one closure here + one Identity
// pointer on the relevant TabSpec/SubtabSpec entry. The dispatchers
// (openTagPickerForCursored, future collect / open routes) consult
// resolveItemIdentity instead of carrying a per-tab switch.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// --- /objects ----------------------------------------------------------

func identityFromObjectsList(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil {
		return ItemIdentity{}, false
	}
	it, ok := d.SObjectList.Selected()
	if !ok {
		return ItemIdentity{}, false
	}
	return ItemIdentity{
		Kind:     devproject.KindSObject,
		Ref:      it.Name,
		Label:    it.Name,
		Openable: it,
	}, true
}

// --- /flows ------------------------------------------------------------

func identityFromFlowsList(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil {
		return ItemIdentity{}, false
	}
	f, ok := d.FlowList.Selected()
	if !ok {
		return ItemIdentity{}, false
	}
	label := f.DeveloperName
	if f.MasterLabel != "" {
		label = f.MasterLabel
	}
	return ItemIdentity{
		Kind:     devproject.KindFlow,
		Ref:      f.DefinitionID,
		Label:    label,
		Openable: f,
	}, true
}

// --- /apex (subtab-aware) ---------------------------------------------

func identityFromApexClassesList(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil {
		return ItemIdentity{}, false
	}
	a, ok := d.ApexClassList.Selected()
	if !ok {
		return ItemIdentity{}, false
	}
	return ItemIdentity{
		Kind:      devproject.KindApexClass,
		Ref:       a.ID,
		Label:     a.Name,
		Openable:  a,
		Namespace: a.NamespacePrefix,
	}, true
}

func identityFromApexTriggersList(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil {
		return ItemIdentity{}, false
	}
	t, ok := d.ApexTriggerList.Selected()
	if !ok {
		return ItemIdentity{}, false
	}
	// TriggerRow has no Openable — open routes through trigger
	// detail drill via the existing openSurface entry, which fires
	// before Identity in cursorOpenable.
	return ItemIdentity{
		Kind:      devproject.KindApexTrigger,
		Ref:       t.ID,
		Label:     t.Name,
		Namespace: t.NamespacePrefix,
	}, true
}

// --- /components LWC + Aura -------------------------------------------

func identityFromLWCList(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil {
		return ItemIdentity{}, false
	}
	b, ok := d.LWCBundleList.Selected()
	if !ok {
		return ItemIdentity{}, false
	}
	return ItemIdentity{
		Kind:     devproject.KindLWC,
		Ref:      b.ID,
		Label:    b.DeveloperName,
		Openable: b,
	}, true
}

func identityFromAuraList(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil {
		return ItemIdentity{}, false
	}
	b, ok := d.AuraBundleList.Selected()
	if !ok {
		return ItemIdentity{}, false
	}
	return ItemIdentity{
		Kind:     devproject.KindAura,
		Ref:      b.ID,
		Label:    b.DeveloperName,
		Openable: b,
	}, true
}

// --- /perms (subtab-aware) --------------------------------------------

func identityFromPermSetsList(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil {
		return ItemIdentity{}, false
	}
	p, ok := d.PermSetList.Selected()
	if !ok {
		return ItemIdentity{}, false
	}
	return ItemIdentity{
		Kind:     devproject.KindPermissionSet,
		Ref:      p.ID,
		Label:    p.Name,
		Openable: p,
	}, true
}

func identityFromPSGsList(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil {
		return ItemIdentity{}, false
	}
	g, ok := d.PSGList.Selected()
	if !ok {
		return ItemIdentity{}, false
	}
	return ItemIdentity{
		Kind:     devproject.KindPermissionSetGroup,
		Ref:      g.ID,
		Label:    g.DeveloperName,
		Openable: g,
	}, true
}

func identityFromProfilesList(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil {
		return ItemIdentity{}, false
	}
	p, ok := d.ProfileList.Selected()
	if !ok {
		return ItemIdentity{}, false
	}
	return ItemIdentity{
		Kind:     devproject.KindProfile,
		Ref:      p.ID,
		Label:    p.Name,
		Openable: p,
	}, true
}

func identityFromQueuesList(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil {
		return ItemIdentity{}, false
	}
	q, ok := d.QueueList.Selected()
	if !ok {
		return ItemIdentity{}, false
	}
	return ItemIdentity{
		Kind:     devproject.KindQueue,
		Ref:      q.ID,
		Label:    q.Name,
		Openable: q,
	}, true
}

func identityFromPublicGroupsList(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil {
		return ItemIdentity{}, false
	}
	g, ok := d.PublicGroupList.Selected()
	if !ok {
		return ItemIdentity{}, false
	}
	return ItemIdentity{
		Kind:     devproject.KindPublicGroup,
		Ref:      g.ID,
		Label:    g.Name,
		Openable: g,
	}, true
}

// identityFromObjectDetail dispatches to the right per-subtab
// resolver based on the current SubtabXX value. ObjectDetail's
// subtabs aren't declared in TabSpec.Subtabs (they're rendered via
// a render-time switch instead), so the registry hook lives on the
// TabSpec.Identity slot and dispatches manually here. Each branch
// is explicit — no default fallback that could silently tag fields
// from a non-schema subtab.
func identityFromObjectDetail(m Model) (ItemIdentity, bool) {
	switch m.currentSubtab() {
	case SubtabSchema:
		return identityFromSchemaField(m)
	case SubtabValidation:
		return identityFromValidationRule(m)
	case SubtabRecordTypes:
		return identityFromRecordType(m)
	case SubtabTriggers:
		return identityFromTriggersList(m)
	case SubtabObjectFlows:
		return identityFromObjectFlow(m)
	}
	return ItemIdentity{}, false
}

// identityFromObjectFlow resolves the cursored row on the object
// drill's Flows subtab. Same Kind/Ref shape as the /flows list so
// tagging, recents, and o (Flow Builder) behave identically from
// either entry point. No layouts equivalent — KindLayout doesn't
// exist yet (see devproject/types.go "Future:"), so o is a no-op
// on the Layouts subtab.
func identityFromObjectFlow(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil || d.DescribeCur == "" {
		return ItemIdentity{}, false
	}
	r := d.ObjectFlows.Lists[d.DescribeCur]
	if r == nil {
		return ItemIdentity{}, false
	}
	rows := r.Value()
	sel := d.ObjectFlows.Cursors[d.DescribeCur]
	if sel < 0 || sel >= len(rows) {
		return ItemIdentity{}, false
	}
	row := rows[sel]
	return ItemIdentity{
		Kind:     devproject.KindFlow,
		Ref:      row.DefinitionID,
		Label:    row.Label,
		Openable: row,
	}, true
}

// --- /object-detail subtabs (Schema / Validation / RecordTypes / Triggers) ---

func identityFromSchemaField(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil || d.DescribeCur == "" {
		return ItemIdentity{}, false
	}
	r, ok := d.Describes[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		return ItemIdentity{}, false
	}
	f, ok := d.cursoredField(d.DescribeCur, r)
	if !ok {
		return ItemIdentity{}, false
	}
	ref := r.Value().Name + "." + f.Name
	return ItemIdentity{
		Kind:     devproject.KindField,
		Ref:      ref,
		Label:    ref,
		Openable: sf.FieldRef{SObjectName: r.Value().Name, Field: f},
	}, true
}

func identityFromValidationRule(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil || d.DescribeCur == "" {
		return ItemIdentity{}, false
	}
	r, ok := d.ValidationRules.Lists[d.DescribeCur]
	if !ok {
		return ItemIdentity{}, false
	}
	rules := r.Value()
	idx := d.ValidationRules.Cursors[d.DescribeCur]
	if idx < 0 || idx >= len(rules) {
		return ItemIdentity{}, false
	}
	rule := rules[idx]
	return ItemIdentity{
		Kind:  devproject.KindValidationRule,
		Ref:   rule.ID,
		Label: d.DescribeCur + " / " + rule.ValidationName,
	}, true
}

func identityFromRecordType(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil || d.DescribeCur == "" {
		return ItemIdentity{}, false
	}
	r, ok := d.RecordTypes.Lists[d.DescribeCur]
	if !ok {
		return ItemIdentity{}, false
	}
	rts := r.Value()
	idx := d.RecordTypes.Cursors[d.DescribeCur]
	if idx < 0 || idx >= len(rts) {
		return ItemIdentity{}, false
	}
	rt := rts[idx]
	return ItemIdentity{
		Kind:  devproject.KindRecordType,
		Ref:   rt.ID,
		Label: d.DescribeCur + " / " + rt.DeveloperName,
	}, true
}

func identityFromTriggersList(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil || d.DescribeCur == "" {
		return ItemIdentity{}, false
	}
	r, ok := d.Triggers.Lists[d.DescribeCur]
	if !ok {
		return ItemIdentity{}, false
	}
	trigs := r.Value()
	idx := d.Triggers.Cursors[d.DescribeCur]
	if idx < 0 || idx >= len(trigs) {
		return ItemIdentity{}, false
	}
	t := trigs[idx]
	return ItemIdentity{
		Kind:  devproject.KindApexTrigger,
		Ref:   t.ID,
		Label: d.DescribeCur + " / " + t.Name,
	}, true
}

// --- /record (drill-in) -----------------------------------------------

func identityFromRecordDetail(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return ItemIdentity{}, false
	}
	sobject, id := splitRecordKey(d.RecordDetailCur)
	if sobject == "" || id == "" {
		return ItemIdentity{}, false
	}
	out := ItemIdentity{
		Kind:  devproject.KindRecord,
		Ref:   sobject + ":" + id,
		Label: sobject + " " + id,
	}
	if ref := recordRefForDrill(d, m); ref != nil {
		out.Openable = *ref
	}
	return out, true
}

// --- /field (drill-in) ------------------------------------------------

// identityFromFieldDetail covers the field-drill tab, where the
// cursored item is the field itself (regardless of any field-detail
// subtab cursor). Returns the same FieldRef shape as the schema
// subtab so o / K / t all behave consistently.
func identityFromFieldDetail(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil || d.DescribeCur == "" || d.FieldCur == "" {
		return ItemIdentity{}, false
	}
	r, ok := d.Describes[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		return ItemIdentity{}, false
	}
	for _, f := range r.Value().Fields {
		if f.Name == d.FieldCur {
			ref := r.Value().Name + "." + f.Name
			return ItemIdentity{
				Kind:     devproject.KindField,
				Ref:      ref,
				Label:    ref,
				Openable: sf.FieldRef{SObjectName: r.Value().Name, Field: f},
			}, true
		}
	}
	return ItemIdentity{}, false
}

// --- /recent ----------------------------------------------------------
