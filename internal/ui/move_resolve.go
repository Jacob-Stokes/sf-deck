package ui

// Per-kind cross-org resolution for the Move-to-org gesture.
//
// resolveMoveRef answers: "in THIS (target) org, what is the org-local
// ref for the resource named <name>, and is its list even loaded yet?"
//
// Return contract:
//
//	ref    — the target org's org-local ref to hand to drillByKind
//	found  — a matching resource exists in the target org
//	ready  — the list we'd match against has loaded (fetch complete);
//	         when false, callers should wait for the next resource msg
//	         rather than concluding "not found."
//
// For API-name-keyed kinds (sObject, field) the name IS the ref and no
// list scan is needed, so ready is always true.

import (
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

func resolveMoveRef(d *orgData, kind devproject.ItemKind, name, typeHint string) (ref string, found, ready bool) {
	switch kind {
	case devproject.KindSObject:
		// Verify the object exists in the target org's browseable list
		// before we commit to switching. Ref == API name.
		if !moveListReady(&d.SObjects) {
			return "", false, false
		}
		if sObjectPresent(d, name) {
			return name, true, true
		}
		return "", false, true

	case devproject.KindField:
		// Ref == "<sobj>.<field>"; verify the PARENT object exists in
		// the target org. The specific field is confirmed lazily when
		// the field-detail drill fetches the describe — checking it
		// here would need a synchronous describe. typeHint carries the
		// parent sObject.
		if !moveListReady(&d.SObjects) {
			return "", false, false
		}
		parent := typeHint
		if parent == "" {
			if i := strings.IndexByte(name, '.'); i >= 0 {
				parent = name[:i]
			}
		}
		if parent != "" && sObjectPresent(d, parent) {
			return name, true, true
		}
		return "", false, true

	// The three Id-keyed kinds read their backing Resource directly
	// (not the ListView) so a match works even for a target org that
	// isn't selected — its ListView may never have been synced, but the
	// Resource is populated by applyResourceMsg regardless of selection.
	case devproject.KindFlow:
		if !moveListReady(&d.Flows) {
			return "", false, false
		}
		for _, f := range d.Flows.Value() {
			if strings.EqualFold(f.DeveloperName, name) {
				return f.DefinitionID, true, true
			}
		}
		return "", false, true

	case devproject.KindApexClass:
		if !moveListReady(&d.ApexClasses) {
			return "", false, false
		}
		for _, c := range d.ApexClasses.Value() {
			if strings.EqualFold(c.Name, name) {
				return c.ID, true, true
			}
		}
		return "", false, true

	case devproject.KindLWC:
		if !moveListReady(&d.LWCBundles) {
			return "", false, false
		}
		for _, b := range d.LWCBundles.Value() {
			if strings.EqualFold(b.DeveloperName, name) {
				return b.ID, true, true
			}
		}
		return "", false, true
	}
	return "", false, true
}

// sObjectPresent reports whether the target org's browseable object
// list contains an object with the given API name (case-insensitive).
func sObjectPresent(d *orgData, apiName string) bool {
	for _, o := range d.SObjects.Value() {
		if strings.EqualFold(o.Name, apiName) {
			return true
		}
	}
	return false
}

// moveListReady reports whether a resource has completed at least one
// fetch (from cache or live), so an empty Items() means "genuinely no
// match" rather than "not loaded yet."
func moveListReady[T any](r *Resource[[]T]) bool {
	return !r.FetchedAt().IsZero()
}
