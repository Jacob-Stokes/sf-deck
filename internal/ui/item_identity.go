package ui

// ItemIdentity is the unified "what's under the cursor right now"
// answer that every cursored-item gesture consults — tag picker,
// project collect, openable lookup, and friends.

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
	Kind      devproject.ItemKind
	Ref       string
	Label     string
	Openable  sf.Openable
	Namespace string
}

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
