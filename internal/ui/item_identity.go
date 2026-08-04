package ui

// ItemIdentity is the unified "what's under the cursor right now"
// answer that every cursored-item gesture consults — tag picker,
// project collect, openable lookup, and friends.
//
// Tab/SubtabSpec.Identity is the per-surface resolver; one place to
// describe "this tab's currently cursored thing." Anything that
// operates on the cursored item walks resolveItemIdentity() once and
// gets a uniform shape back, regardless of tab.
//
// Why this beats the older per-gesture switch chains:
//
//   - Adding a new taggable / collectable / openable surface is one
//     Identity closure on its TabSpec entry, not three switches in
//     three different files.
//   - Bugs like "schema subtab default branch tags fields on every
//     non-schema subtab too" can't happen — each subtab declares its
//     own Identity closure or returns ok=false.
//   - The closure is a pure data extractor: it doesn't mutate the
//     model, so calling it from multiple gestures is safe.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// ItemIdentity is the resolved cursored-item descriptor.
//
// Kind + Ref are the canonical (devproject.ItemKind, ref-string)
// pair used by tags, projects, and collect — same shape as
// TagLookupKey / Item.Ref.
//
// Label is the user-visible name to show in the tag picker / collect
// flash / similar UI strings. Falls back to Ref when the underlying
// item has no friendlier name.
//
// Openable, when non-nil, gives the cursored item a sf.Openable so
// the o / O gestures can route through the same identity. Set on
// surfaces that have a clean Openable mapping; left nil where
// Open routes through a registry surface (chipSurface / openSurface).
type ItemIdentity struct {
	Kind     devproject.ItemKind
	Ref      string
	Label    string
	Openable sf.Openable
	// Namespace is the managed-package prefix when the cursored item
	// belongs to a managed package (e.g. "sf_devops" for DevOps Center
	// classes). Empty for native components. Captured here so collect
	// flows can populate Item.Namespace without re-querying the org.
	Namespace string
}

// resolveItemIdentity walks the spec resolver chain (subtab →
// tab) and returns the first non-zero Identity result. Returns
// ok=false when no resolver is registered or when the registered
// resolver itself reports nothing selected.
func (m Model) resolveItemIdentity() (ItemIdentity, bool) {
	spec, sub := m.activeSpec()
	if spec == nil {
		return ItemIdentity{}, false
	}
	if sub != nil && sub.Identity != nil {
		if it, ok := sub.Identity(m); ok {
			return it, true
		}
	}
	if spec.Identity != nil {
		return spec.Identity(m)
	}
	return ItemIdentity{}, false
}
