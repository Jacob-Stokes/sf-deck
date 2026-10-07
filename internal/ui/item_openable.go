package ui

// item_openable.go — wraps a devproject.Item in the real per-kind
// sf struct (sf.Flow, sf.ApexClassRow, sf.SObject, …) so that
// Targets() and YankTargets() reuse the SAME implementations the
// top-level /flows, /apex, /objects, etc. tabs use.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func openableForItem(m Model, it devproject.Item) sf.Openable {
	d := m.data[it.OrgUser]
	switch it.Kind {
	case devproject.KindSObject:
		if d != nil {
			for _, s := range d.SObjects.Value() {
				if s.Name == it.Ref {
					return s
				}
			}
		}
		return sf.SObject{Name: it.Ref, Label: it.Name}

	case devproject.KindField:
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
		if d != nil {
			for _, a := range d.AuraBundles.Value() {
				if a.ID == it.Ref {
					return a
				}
			}
		}
		return sf.AuraBundle{ID: it.Ref, MasterLabel: it.Name}

	case devproject.KindValidationRule:
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

func splitSObjectField(ref string) (string, string) {
	for i, r := range ref {
		if r == '.' {
			return ref[:i], ref[i+1:]
		}
	}
	return "", ""
}

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
