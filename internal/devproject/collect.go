package devproject

// "Collect" — adapter from a generic sf.Openable into a typed Item.
//
// The shell's shift+K flow hands us whatever's under the cursor. This
// file knows how to recognise the kinds we support and emit a stable
// (Kind, Ref, Type, Name) tuple. Unsupported kinds return ok=false so
// the modal can refuse cleanly.
//
// Adding support for a new kind = one new case here. The store schema
// itself is type-agnostic.

import (
	"fmt"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// FromOpenable inspects a cursored item and returns the (Kind, Ref,
// Type, Name) tuple that uniquely addresses it within an org project.
// Returns ok=false for kinds we don't support yet.
func FromOpenable(item sf.Openable) (kind ItemKind, ref, typ, name string, ok bool) {
	if item == nil {
		return "", "", "", "", false
	}
	switch t := item.(type) {
	case sf.SObject:
		return KindSObject, t.Name, "", displayLabelForSObject(t), true
	case sf.FieldRef:
		if t.SObjectName == "" || t.Field.Name == "" {
			return "", "", "", "", false
		}
		ref := t.SObjectName + "." + t.Field.Name
		nm := t.Field.Label
		if nm == "" {
			nm = t.Field.Name
		}
		return KindField, ref, t.SObjectName, nm, true
	case sf.Flow:
		ref := t.DefinitionID
		if ref == "" {
			return "", "", "", "", false
		}
		nm := t.DeveloperName
		if nm == "" {
			nm = t.MasterLabel
		}
		return KindFlow, ref, "Flow", nm, true
	case sf.FlowVersion:
		if t.ID == "" {
			return "", "", "", "", false
		}
		nm := t.MasterLabel
		if nm == "" {
			nm = t.ID
		}
		return KindFlowVersion, t.ID, t.DefinitionID, nm, true
	case sf.RecordRef:
		id, _ := t.Record["Id"].(string)
		if id == "" {
			return "", "", "", "", false
		}
		sobj, _ := sObjectFromRecord(t.Record)
		if sobj == "" {
			// No sObject (aggregate rows, exotic payloads) — we can't
			// build the canonical "<sObject>:<Id>" ref every tag/project
			// lookup keys by, so refuse rather than store an orphan.
			return "", "", "", "", false
		}
		nm := recordDisplayName(t.Record)
		if nm == "" {
			nm = id
		}
		// Ref MUST be "<sObject>:<Id>" — the shape all gutter/sidebar
		// lookups and the identity resolvers use. This used to return
		// the bare Id (sObject only in typ), so collected records never
		// matched a PROJECTS-column lookup and showed no pill.
		return KindRecord, sobj + ":" + id, sobj, nm, true
	case sf.ReportSummary:
		if t.ID == "" {
			return "", "", "", "", false
		}
		nm := t.Name
		if nm == "" {
			nm = t.ID
		}
		return KindReport, t.ID, t.FolderName, nm, true
	case sf.PermissionSet:
		if t.ID == "" {
			return "", "", "", "", false
		}
		nm := t.Label
		if nm == "" {
			nm = t.Name
		}
		return KindPermissionSet, t.ID, t.Name, nm, true
	case sf.PermissionSetGroup:
		if t.ID == "" {
			return "", "", "", "", false
		}
		nm := t.MasterLabel
		if nm == "" {
			nm = t.DeveloperName
		}
		return KindPermissionSetGroup, t.ID, t.DeveloperName, nm, true
	case sf.Profile:
		if t.ID == "" {
			return "", "", "", "", false
		}
		// Type carries the implicit-permset id so writers (FLS, object
		// perms) can target it without re-querying.
		return KindProfile, t.ID, t.PermissionSetID, t.Name, true
	case sf.ApexClassRow:
		if t.ID == "" {
			return "", "", "", "", false
		}
		return KindApexClass, t.ID, "ApexClass", t.Name, true
	case sf.LWCBundle:
		if t.ID == "" {
			return "", "", "", "", false
		}
		nm := t.MasterLabel
		if nm == "" {
			nm = t.DeveloperName
		}
		return KindLWC, t.ID, t.DeveloperName, nm, true
	case sf.AuraBundle:
		if t.ID == "" {
			return "", "", "", "", false
		}
		nm := t.MasterLabel
		if nm == "" {
			nm = t.DeveloperName
		}
		return KindAura, t.ID, t.DeveloperName, nm, true
	case sf.QueueRow:
		if t.ID == "" {
			return "", "", "", "", false
		}
		nm := t.Name
		if nm == "" {
			nm = t.DeveloperName
		}
		return KindQueue, t.ID, t.DeveloperName, nm, true
	case sf.PublicGroupRow:
		if t.ID == "" {
			return "", "", "", "", false
		}
		nm := t.Name
		if nm == "" {
			nm = t.DeveloperName
		}
		return KindPublicGroup, t.ID, t.DeveloperName, nm, true
	}
	return "", "", "", "", false
}

// SupportedKinds returns the ItemKind list the collector recognises.
// Useful for "what can I shift+K?" hints in the future.
func SupportedKinds() []ItemKind {
	return []ItemKind{
		KindSObject, KindField,
		KindFlow, KindFlowVersion,
		KindRecord, KindApexClass, KindReport,
		KindPermissionSet, KindPermissionSetGroup, KindProfile,
		KindValidationRule, KindRecordType, KindApexTrigger,
		KindLWC, KindAura,
		KindQueue, KindPublicGroup,
	}
}

// LabelForItem returns a user-visible label for an Item — falls back
// to "<kind> <ref>" when the captured Name is empty.
func LabelForItem(it Item) string {
	if it.Name != "" {
		return it.Name
	}
	if it.Type != "" {
		return fmt.Sprintf("%s %s", it.Type, it.Ref)
	}
	return fmt.Sprintf("%s %s", it.Kind, it.Ref)
}

func displayLabelForSObject(s sf.SObject) string {
	if s.Label != "" && s.Label != s.Name {
		return s.Name + " — " + s.Label
	}
	return s.Name
}

func sObjectFromRecord(rec map[string]any) (string, bool) {
	attrs, ok := rec["attributes"].(map[string]any)
	if !ok {
		return "", false
	}
	t, _ := attrs["type"].(string)
	return t, t != ""
}

func recordDisplayName(rec map[string]any) string {
	for _, k := range []string{"Name", "Subject", "CaseNumber", "DeveloperName", "Title"} {
		if v, ok := rec[k].(string); ok && v != "" {
			return v
		}
	}
	if id, ok := rec["Id"].(string); ok {
		return id
	}
	return ""
}
