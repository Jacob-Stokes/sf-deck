package soqlauto

import (
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// WalkHops traverses a dotted relationship chain across the describe
// graph. Starting from root sObject, it follows each hop's
// relationshipName to the field's referenceTo[]; the resulting
// candidate-describe set replaces the current set at each step.
//
// Polymorphic refs (e.g. Task.WhoId → [Contact, Lead]) naturally
// produce multi-describe sets — every downstream suggestion call
// unions fields across all of them.
//
// Returns the final candidate describes + a slice of sObject names
// whose describes are needed but not cached. Callers should
// EnsureDescribe each name and re-invoke once they land.
//
// Special cases:
//   - root not loaded → returns (nil, [root])
//   - any hop's referenceTo target not loaded → continues with what
//     we have but appends to loading
//   - dead-end hop (no matching relationshipName) → returns (nil, loading)
//     so the caller knows to emit no field suggestions
func WalkHops(snap Snapshot, root string, hops []string) (current []sf.SObjectDescribe, loading []string) {
	if root == "" {
		return nil, nil
	}
	ref := snap.Describes(root)
	switch ref.Status {
	case StatusLoaded:
		if ref.Describe == nil {
			return nil, nil
		}
		current = []sf.SObjectDescribe{*ref.Describe}
	default:
		if snap.EnsureDescribe != nil {
			snap.EnsureDescribe(root)
		}
		return nil, []string{root}
	}

	for _, hop := range hops {
		next := make([]sf.SObjectDescribe, 0, 2)
		seen := map[string]bool{}
		for _, desc := range current {
			for _, f := range desc.Fields {
				if !strings.EqualFold(f.RelationshipName, hop) {
					continue
				}
				for _, ref := range f.ReferenceTo {
					if seen[ref] {
						continue
					}
					seen[ref] = true
					d := snap.Describes(ref)
					switch d.Status {
					case StatusLoaded:
						if d.Describe != nil {
							next = append(next, *d.Describe)
						}
					default:
						if snap.EnsureDescribe != nil {
							snap.EnsureDescribe(ref)
						}
						loading = append(loading, ref)
					}
				}
			}
		}
		if len(next) == 0 {
			// Dead-end: hop didn't match any relationship, or all
			// matching refs are unloaded. Keep `loading` populated
			// so the caller can show "loading X metadata…".
			return nil, loading
		}
		current = next
	}
	return current, loading
}

// ResolveChildSObject looks up the child sObject for a relationship
// name on a parent describe. Used by the subquery branch — when the
// caret is inside `(SELECT ... FROM Contacts)`, "Contacts" is the
// relationshipName, not an actual sObject API name.
//
// Returns "" when the relationship doesn't exist on the parent.
func ResolveChildSObject(parent *sf.SObjectDescribe, relationshipName string) string {
	if parent == nil {
		return ""
	}
	for _, c := range parent.ChildRelationships {
		if strings.EqualFold(c.RelationshipName, relationshipName) {
			return c.ChildSObject
		}
	}
	return ""
}
