// Package orgproject is the bridge between the persistent dev-project
// store and the UI's "loaded project" feature.
//
// The store (internal/devproject) holds projects + their items.
// The UI (internal/ui) wants to ask "is this object/flow/record in
// scope right now?" — without each surface having to read the store
// or thread items around. Scope is the answer.
//
// Lifecycle:
//   - User loads a DevProject for the active org via `_` on the
//     dev-projects list. Settings persists "<org-user> → <devproject-id>".
//   - On startup (and whenever the loaded id changes), the UI
//     hydrates a Scope from the store: query items filtered to the
//     active org, bucket by kind, stash on Model. Refreshed lazily
//     when items are K-collected into the loaded project.
//   - Surfaces (records / objects / flows / records-subtab / reports)
//     ask m.activeScope() for the current Scope, build a synthetic
//     project chip when it's loaded, attach a predicate that defers
//     to Scope.HasObject / HasFlow / HasReport / HasRecord.
//
// The package name is preserved as orgproject for migration ergonomics
// — every call site already imports it. Conceptually it's now
// "DevProject-as-seen-by-the-active-org" rather than a free-standing
// OrgProject row, but the surface area is identical.
package orgproject

import (
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

// Scope is the loaded dev-project's items reshaped for predicate
// lookups. Each map is keyed by the same string the surface compares
// against: SObject API name, Flow definition id, etc. A nil Scope or
// one with empty ProjectID means "nothing loaded" (Loaded() returns
// false).
type Scope struct {
	// ProjectID is the DevProject row id. Empty when nothing's loaded.
	ProjectID string
	// ProjectName is the user-visible label, captured at load time so
	// the UI doesn't need to re-query the store on every render.
	ProjectName string
	// OrgUser is the org this Scope was hydrated for. Items in the
	// project from other orgs are excluded — the per-org filter is
	// done at hydrate time so render-path lookups stay O(1).
	OrgUser string
	// Items by kind. Maps used as sets — value is always true.
	Objects      map[string]bool // sObject API names (KindSObject)
	FlowIDs      map[string]bool // FlowDefinition ids (KindFlow)
	ApexIDs      map[string]bool // ApexClass ids (KindApexClass)
	TriggerIDs   map[string]bool // ApexTrigger ids (KindApexTrigger)
	LWCIDs       map[string]bool // LightningComponentBundle ids (KindLWC)
	AuraIDs      map[string]bool // AuraDefinitionBundle ids (KindAura)
	ReportIDs    map[string]bool // Report ids (KindReport)
	PermSets     map[string]bool // PermissionSet ids (KindPermissionSet)
	PSGs         map[string]bool // PermissionSetGroup ids (KindPermissionSetGroup)
	Profiles     map[string]bool // Profile ids (KindProfile)
	Queues       map[string]bool // Queue (Group) ids (KindQueue)
	PublicGroups map[string]bool // Public Group ids (KindPublicGroup)
	// Records is keyed by "<sobject>:<id>" so a project containing
	// "Account/001…" and "Contact/003…" can answer "is this Account
	// row in scope?" without scanning. Populated from KindRecord
	// items where Item.Type holds the sObject and Item.Ref holds Id.
	Records map[string]bool
}

// Loaded reports whether a non-empty project is currently loaded.
// nil-safe so callers can write `if scope.Loaded() { ... }` without
// a nil-check first.
func (s *Scope) Loaded() bool {
	return s != nil && s.ProjectID != ""
}

// Empty reports whether the loaded project has zero items of any
// kind FOR THE SCOPE'S ORG (used by the chip-builder to hide the
// synthetic chip when there'd be nothing to filter to). Loaded but
// empty is fine — it means "I've loaded this project; haven't added
// anything from this org yet."
func (s *Scope) Empty() bool {
	if !s.Loaded() {
		return true
	}
	return len(s.Objects) == 0 && len(s.FlowIDs) == 0 &&
		len(s.ApexIDs) == 0 && len(s.TriggerIDs) == 0 &&
		len(s.LWCIDs) == 0 && len(s.AuraIDs) == 0 &&
		len(s.ReportIDs) == 0 && len(s.Records) == 0 &&
		len(s.PermSets) == 0 && len(s.PSGs) == 0 && len(s.Profiles) == 0 &&
		len(s.Queues) == 0 && len(s.PublicGroups) == 0
}

// HasObject reports membership in the project's collected sObjects.
// nil-safe and returns false when nothing's loaded.
func (s *Scope) HasObject(name string) bool {
	if s == nil {
		return false
	}
	return s.Objects[name]
}

// HasFlow / HasApex are the per-kind membership checks. nil-safe.
func (s *Scope) HasFlow(id string) bool {
	if s == nil {
		return false
	}
	return s.FlowIDs[id]
}

func (s *Scope) HasApex(id string) bool {
	if s == nil {
		return false
	}
	return s.ApexIDs[id]
}

// HasTrigger / HasLWC / HasAura mirror the per-kind membership checks
// for /apex (triggers subtab) and /components (LWC + Aura subtabs).
func (s *Scope) HasTrigger(id string) bool {
	if s == nil {
		return false
	}
	return s.TriggerIDs[id]
}

func (s *Scope) HasLWC(id string) bool {
	if s == nil {
		return false
	}
	return s.LWCIDs[id]
}

func (s *Scope) HasAura(id string) bool {
	if s == nil {
		return false
	}
	return s.AuraIDs[id]
}

// HasPermSet / HasPSG / HasProfile mirror the per-kind checks for
// the /perms subtabs' project-chip predicates.
func (s *Scope) HasPermSet(id string) bool {
	if s == nil {
		return false
	}
	return s.PermSets[id]
}

func (s *Scope) HasPSG(id string) bool {
	if s == nil {
		return false
	}
	return s.PSGs[id]
}

func (s *Scope) HasProfile(id string) bool {
	if s == nil {
		return false
	}
	return s.Profiles[id]
}

// HasQueue / HasPublicGroup mirror the per-kind membership checks
// for /perms's Queues + Public Groups subtabs. nil-safe.
func (s *Scope) HasQueue(id string) bool {
	if s == nil {
		return false
	}
	return s.Queues[id]
}

func (s *Scope) HasPublicGroup(id string) bool {
	if s == nil {
		return false
	}
	return s.PublicGroups[id]
}

// HasReport reports membership in the project's collected reports.
func (s *Scope) HasReport(id string) bool {
	if s == nil {
		return false
	}
	return s.ReportIDs[id]
}

// HasRecord reports membership for a given (sobject, id) pair. The
// composite key is what the project stores; surfaces pass both halves
// rather than constructing the key inline so the lookup contract
// stays in one place.
func (s *Scope) HasRecord(sobject, id string) bool {
	if s == nil || sobject == "" || id == "" {
		return false
	}
	return s.Records[sobject+":"+id]
}

// RecordIDsFor returns the set of record ids in the project for the
// given sObject. Used by the records-subtab chip to emit a SOQL
// `WHERE Id IN (...)` clause — the predicate path can't bake the
// list into a server-side filter, so we hand the surface the slice
// it needs.
func (s *Scope) RecordIDsFor(sobject string) []string {
	if s == nil || sobject == "" {
		return nil
	}
	prefix := sobject + ":"
	out := make([]string, 0, len(s.Records))
	for k := range s.Records {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			out = append(out, k[len(prefix):])
		}
	}
	return out
}

// ScopeOptions seeds Hydrate with metadata that doesn't live in
// devproject.Store but is useful on the UI side — primarily the
// human-readable label to display in the header pill.
type ScopeOptions struct {
	ProjectName string
}

// Hydrate builds a Scope from the store's items for the given
// DevProject id, filtered to one org. Returns a non-nil empty Scope
// (Loaded() == false) when devProjectID is "" or the project has
// been deleted, so callers can treat the result uniformly.
//
// orgUser="" is allowed for diagnostic / "show every org" callers
// but the regular UI path always passes the active org's username
// so the membership maps reflect what's in scope HERE.
func Hydrate(store *devproject.Store, devProjectID, orgUser string, opts ScopeOptions) (*Scope, error) {
	out := &Scope{
		OrgUser:      orgUser,
		Objects:      map[string]bool{},
		FlowIDs:      map[string]bool{},
		ApexIDs:      map[string]bool{},
		TriggerIDs:   map[string]bool{},
		LWCIDs:       map[string]bool{},
		AuraIDs:      map[string]bool{},
		ReportIDs:    map[string]bool{},
		PermSets:     map[string]bool{},
		PSGs:         map[string]bool{},
		Profiles:     map[string]bool{},
		Queues:       map[string]bool{},
		PublicGroups: map[string]bool{},
		Records:      map[string]bool{},
	}
	if store == nil || devProjectID == "" {
		return out, nil
	}
	dp, err := store.GetDevProject(devProjectID)
	if err != nil {
		return out, err
	}
	if dp == nil {
		// Stale id — project was deleted. Caller should clear
		// settings; we just return an empty Scope.
		return out, nil
	}
	out.ProjectID = dp.ID
	if opts.ProjectName != "" {
		out.ProjectName = opts.ProjectName
	} else {
		out.ProjectName = dp.Name
	}
	items, err := store.ListItems(devProjectID, orgUser)
	if err != nil {
		return out, err
	}
	for _, it := range items {
		switch it.Kind {
		case devproject.KindSObject:
			out.Objects[it.Ref] = true
		case devproject.KindFlow:
			out.FlowIDs[it.Ref] = true
		case devproject.KindApexClass:
			out.ApexIDs[it.Ref] = true
		case devproject.KindApexTrigger:
			out.TriggerIDs[it.Ref] = true
		case devproject.KindLWC:
			out.LWCIDs[it.Ref] = true
		case devproject.KindAura:
			out.AuraIDs[it.Ref] = true
		case devproject.KindReport:
			out.ReportIDs[it.Ref] = true
		case devproject.KindRecord:
			if it.Type == "" || it.Ref == "" {
				continue
			}
			out.Records[it.Type+":"+it.Ref] = true
		case devproject.KindPermissionSet:
			out.PermSets[it.Ref] = true
		case devproject.KindPermissionSetGroup:
			out.PSGs[it.Ref] = true
		case devproject.KindProfile:
			out.Profiles[it.Ref] = true
		case devproject.KindQueue:
			out.Queues[it.Ref] = true
		case devproject.KindPublicGroup:
			out.PublicGroups[it.Ref] = true
		}
	}
	return out, nil
}
