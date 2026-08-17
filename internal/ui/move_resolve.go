package ui

import (
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

func resolveMoveRef(d *orgData, kind devproject.ItemKind, name, typeHint string) (ref string, found, ready bool) {
	switch kind {
	case devproject.KindSObject:
		if !moveListReady(&d.SObjects) {
			return "", false, false
		}
		if sObjectPresent(d, name) {
			return name, true, true
		}
		return "", false, true

	case devproject.KindField:
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

func sObjectPresent(d *orgData, apiName string) bool {
	for _, o := range d.SObjects.Value() {
		if strings.EqualFold(o.Name, apiName) {
			return true
		}
	}
	return false
}

func moveListReady[T any](r *Resource[[]T]) bool {
	return !r.FetchedAt().IsZero()
}
