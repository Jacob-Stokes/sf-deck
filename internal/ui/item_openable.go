package ui

// item_openable.go — wraps a devproject.Item in the real per-kind
// sf struct (sf.Flow, sf.ApexClassRow, sf.SObject, …) so that
// Targets() and YankTargets() reuse the SAME implementations the
// top-level /flows, /apex, /objects, etc. tabs use.
//
// Why this matters: each typed struct already knows how to build
// its full Lightning destination menu (Flow Builder + Setup +
// list view) and its yank menu (id + DeveloperName + URL). We don't
// reinvent any of it — we just instantiate the struct with the
// fields we have (typically Id and a couple of others) and let
// the existing impl do the work.
//
// Fields we DON'T have on a devproject.Item (ActiveVersionID for
// flows, ApiVersion for apex, IsValid flags, ModifiedBy…) become
// zero-values. The Targets() impls all handle these gracefully —
// missing fields just drop the targets that needed them, never
// crash. The result is "what we have works the same as anywhere
// else; what we're missing is just absent from the menu."

import (
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// openableForItem returns the live per-kind sf typed-row that
// implements both sf.Openable and (for most kinds) sf.Yankable,
// reusing the existing implementation. Returns nil for kinds with
// no Lightning destination at all (apex snippets, saved soql
// queries — both sf-deck-local concepts).
//
// Where possible, the FULL row from the org's cached resource is
// returned — that way the per-kind Targets() impl sees every field
// (ActiveVersionID for flows, ApiVersion for apex, the
// folder/developer pair for reports, etc.) and produces the same
// menu users see on the kind's top-level tab. Falls back to a
// minimal synthetic row when the cache hasn't loaded for that org
// or the item isn't present — degraded but always functional.
func openableForItem(m Model, it devproject.Item) sf.Openable {
	d := m.data[it.OrgUser]
	switch it.Kind {
	case devproject.KindSObject:
		// Ref = API name. Find the full SObject when loaded.
		if d != nil {
			for _, s := range d.SObjects.Value() {
				if s.Name == it.Ref {
					return s
				}
			}
		}
		return sf.SObject{Name: it.Ref, Label: it.Name}

	case devproject.KindField:
		// Ref = "<sobject>.<field>". Find the full Field on the parent
		// describe so the menu carries every URL the top-level
		// /objects → Schema path produces.
		sobj, fname := splitSObjectField(it.Ref)
		if sobj == "" || fname == "" {
			return nil
		}
		if d != nil {
			if desc, ok := d.Describes[sobj]; ok && !desc.FetchedAt().IsZero() {
				for _, f := range desc.Value().Fields {
					if f.Name == fname {
						return sf.FieldRef{SObjectName: sobj, Field: f}
					}
				}
			}
		}
		return sf.FieldRef{
			SObjectName: sobj,
			Field:       sf.Field{Name: fname, Label: it.Name},
		}

	case devproject.KindFlow:
		// Ref = FlowDefinitionId. Look up the full Flow so the Flow
		// Builder target (which needs ActiveVersionID / LatestVersionID)
		// is part of the menu — same shape as /flows.
		if d != nil {
			for _, f := range d.Flows.Value() {
				if f.DefinitionID == it.Ref {
					return f
				}
			}
		}
		return sf.Flow{
			DefinitionID:  it.Ref,
			DeveloperName: it.Name,
		}

	case devproject.KindApexClass:
		if d != nil {
			for _, c := range d.ApexClasses.Value() {
				if c.ID == it.Ref {
					return c
				}
			}
		}
		return sf.ApexClassRow{ID: it.Ref, Name: it.Name}

	case devproject.KindApexTrigger:
		// sf.TriggerRow doesn't implement Openable directly — keep the
		// triggerOpenable fallback. Still try to pull the cached row
		// for richer name / parent context.
		if it.Ref == "" {
			return nil
		}
		name, parent := it.Name, it.Type
		if d != nil {
			for _, t := range d.ApexTriggersFlat.Value() {
				if t.ID == it.Ref {
					if t.Name != "" {
						name = t.Name
					}
					if t.Table != "" {
						parent = t.Table
					}
					break
				}
			}
		}
		return triggerOpenable{id: it.Ref, name: name, parent: parent}

	case devproject.KindReport:
		if d != nil {
			for _, r := range d.Reports.Value() {
				if r.ID == it.Ref {
					return r
				}
			}
		}
		return sf.ReportSummary{ID: it.Ref, Name: it.Name, FolderName: it.Type}

	case devproject.KindPermissionSet:
		if d != nil {
			for _, p := range d.PermSets.Value() {
				if p.ID == it.Ref {
					return p
				}
			}
		}
		return sf.PermissionSet{ID: it.Ref, Label: it.Name}

	case devproject.KindPermissionSetGroup:
		// No PSG cache on orgData; the synthetic struct carries
		// enough for the basic open-in-Setup link.
		return sf.PermissionSetGroup{ID: it.Ref, MasterLabel: it.Name}

	case devproject.KindProfile:
		if d != nil {
			for _, p := range d.Profiles.Value() {
				if p.ID == it.Ref {
					return p
				}
			}
		}
		return sf.Profile{ID: it.Ref, Name: it.Name}

	case devproject.KindRecord:
		// Canonical ref is "<sObject>:<Id>"; legacy items stored the
		// bare Id with the sObject only in Type. splitRecordKey returns
		// ("", ref) for the legacy shape — fall back to Type then.
		sobj, id := splitRecordKey(it.Ref)
		if sobj == "" {
			sobj, id = it.Type, it.Ref
		}
		if sobj == "" || id == "" {
			return nil
		}
		return sf.RecordRef{
			Record: map[string]any{
				"Id":   id,
				"Name": it.Name,
				"attributes": map[string]any{
					"type": sobj,
				},
			},
		}

	case devproject.KindLWC:
		if d != nil {
			for _, l := range d.LWCBundles.Value() {
				if l.ID == it.Ref {
					return l
				}
			}
		}
		return sf.LWCBundle{ID: it.Ref, MasterLabel: it.Name}

	case devproject.KindAura:
		// AuraBundle has its own Openable impl distinct from LWC.
		if d != nil {
			for _, a := range d.AuraBundles.Value() {
				if a.ID == it.Ref {
					return a
				}
			}
		}
		return sf.AuraBundle{ID: it.Ref, MasterLabel: it.Name}

	case devproject.KindValidationRule:
		// sf.ValidationRuleRow is scoped per-object internally — it
		// doesn't implement Openable. Build a small Openable here.
		if it.Type == "" || it.Ref == "" {
			return nil
		}
		return validationRuleOpenable{id: it.Ref, name: it.Name, parent: it.Type}

	case devproject.KindRecordType:
		if it.Type == "" || it.Ref == "" {
			return nil
		}
		return recordTypeOpenable{id: it.Ref, name: it.Name, parent: it.Type}

	case devproject.KindQueue:
		return sf.QueueRow{ID: it.Ref, Name: it.Name}

	case devproject.KindPublicGroup:
		return sf.PublicGroupRow{ID: it.Ref, Name: it.Name}
	}
	return nil
}

// --- fallback Openables for kinds with no direct sf.* impl ---

// triggerOpenable is the minimal Openable for an apex trigger row.
// The sf.TriggerRow struct doesn't implement Openable (it's used
// inside per-sobject views where the parent comes from the
// surrounding tab); we replicate the trigger's Lightning targets
// here from just (id, name, parent).
type triggerOpenable struct {
	id, name, parent string
}

func (t triggerOpenable) Targets() []sf.OpenTarget {
	out := []sf.OpenTarget{
		{ID: "trigger",
			Label: "Apex Trigger",
			Path:  "/lightning/setup/ApexTriggers/page?address=%2F" + t.id},
	}
	if t.parent != "" {
		out = append(out, sf.OpenTarget{
			ID:    "object",
			Label: "Object Manager · " + t.parent,
			Path:  "/lightning/setup/ObjectManager/" + t.parent + "/Details/view",
		})
	}
	out = append(out, sf.OpenTarget{
		ID: "list", Label: "All Apex Triggers",
		Path: "/lightning/setup/ApexTriggers/home",
	})
	return out
}

func (t triggerOpenable) YankTargets() []sf.YankTarget {
	return commonItemYank(t.id, t.name, t.parent)
}

type validationRuleOpenable struct {
	id, name, parent string
}

func (v validationRuleOpenable) Targets() []sf.OpenTarget {
	return []sf.OpenTarget{
		{ID: "rule",
			Label: "Validation Rule",
			Path:  "/lightning/setup/ObjectManager/" + v.parent + "/ValidationRules/" + v.id + "/view"},
		{ID: "list",
			Label: "All Validation Rules on " + v.parent,
			Path:  "/lightning/setup/ObjectManager/" + v.parent + "/ValidationRules/view"},
	}
}

func (v validationRuleOpenable) YankTargets() []sf.YankTarget {
	return commonItemYank(v.id, v.name, v.parent)
}

type recordTypeOpenable struct {
	id, name, parent string
}

func (r recordTypeOpenable) Targets() []sf.OpenTarget {
	return []sf.OpenTarget{
		{ID: "rt",
			Label: "Record Type",
			Path:  "/lightning/setup/ObjectManager/" + r.parent + "/RecordTypes/" + r.id + "/view"},
		{ID: "list",
			Label: "All Record Types on " + r.parent,
			Path:  "/lightning/setup/ObjectManager/" + r.parent + "/RecordTypes/view"},
	}
}

func (r recordTypeOpenable) YankTargets() []sf.YankTarget {
	return commonItemYank(r.id, r.name, r.parent)
}

// commonItemYank renders the boilerplate yank menu for the fallback
// Openables — id, name, parent. Used by triggerOpenable +
// validationRuleOpenable + recordTypeOpenable so they all expose
// the same shape.
func commonItemYank(id, name, parent string) []sf.YankTarget {
	var out []sf.YankTarget
	if id != "" {
		out = append(out, sf.YankTarget{
			ID: "id", Label: "Id", Value: id, Shortcut: "i",
		})
	}
	if name != "" && name != id {
		out = append(out, sf.YankTarget{
			ID: "name", Label: "Name", Value: name, Shortcut: "n",
		})
	}
	if parent != "" {
		out = append(out, sf.YankTarget{
			ID: "parent", Label: "Parent sObject", Value: parent, Shortcut: "p",
		})
	}
	return out
}

// splitSObjectField splits "Account.Phone" into ("Account", "Phone").
// Returns empty strings when the input isn't dotted.
func splitSObjectField(ref string) (string, string) {
	for i, r := range ref {
		if r == '.' {
			return ref[:i], ref[i+1:]
		}
	}
	return "", ""
}

// identityFromTagDetail is the TabSpec.Identity closure for
// /tag-detail. Resolves the cursored row in m.tagItems and wraps it
// in an ItemIdentity carrying both the (Kind, Ref) for downstream
// surfaces AND the openable so `o` works.
func identityFromTagDetail(m Model) (ItemIdentity, bool) {
	rows := m.tagItems.Filtered()
	if len(rows) == 0 {
		return ItemIdentity{}, false
	}
	cur := m.tagItems.Cursor()
	if cur >= len(rows) {
		return ItemIdentity{}, false
	}
	return identityForItem(m, rows[cur]), true
}

// identityFromDevProjectItems is the SubtabSpec.Identity closure for
// the /dev-project-detail Items subtab. Same shape as tag detail.
func identityFromDevProjectItems(m Model) (ItemIdentity, bool) {
	d := m.activeOrgData()
	if d == nil {
		return ItemIdentity{}, false
	}
	rows := d.DevProjectItems.Filtered()
	if len(rows) == 0 {
		return ItemIdentity{}, false
	}
	cur := d.DevProjectItems.Cursor()
	if cur >= len(rows) {
		return ItemIdentity{}, false
	}
	return identityForItem(m, rows[cur]), true
}

// identityForItem packages a devproject.Item into the ItemIdentity
// shape the open / drill machinery expects. The Openable is
// kind-specific (see openableForItem); kinds with no Lightning
// destination get nil here so the open dispatcher gracefully
// flashes "nothing to open here" rather than firing a half-built
// URL.
//
// Model is threaded through so openableForItem can pull the FULL
// per-kind row from the org's cached resource — produces a
// fuller open / yank menu (identical to the top-level tab) when
// the resource is loaded.
func identityForItem(m Model, it devproject.Item) ItemIdentity {
	label := it.Name
	if label == "" {
		label = it.Ref
	}
	return ItemIdentity{
		Kind:     it.Kind,
		Ref:      it.Ref,
		Label:    label,
		Openable: openableForItem(m, it),
	}
}
