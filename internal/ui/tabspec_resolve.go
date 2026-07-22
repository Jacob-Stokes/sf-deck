package ui

// Surface resolution helpers that walk the TabSpec/SubtabSpec
// inheritance for chip/open/list surfaces.
//
// Resolution order:
//
//   1. Active SubtabSpec's field (per-subtab override)
//   2. Parent TabSpec's field
//   3. nil (no surface, dispatcher falls back to bespoke switch)
//
// Every surface-bearing tab now has its surfaces wired directly into
// TabSpec — see tab_registry.go. The earlier "fallback to standalone
// chipSurfaces() / openSurfaces() / listSurfaces() map" step is gone:
// those maps and their *SurfaceFor() Model methods existed during
// the migration as a transitional fallback, but TabSpec is now the
// single source of truth.

import (
	tea "charm.land/bubbletea/v2"
)

// resolveChipSurface returns the chipSurface for the active
// (Tab, Subtab), walking subtab → tab. Nil = no chip strip.
func (m Model) resolveChipSurface() *chipSurface {
	if spec := lookupTabSpec(m.tab()); spec != nil {
		if sub := spec.activeSubtabSpec(m); sub != nil && sub.Chips != nil {
			return sub.Chips
		}
		if spec.Chips != nil {
			return spec.Chips
		}
	}
	// /records pre-drill (no sObject yet) shares the /objects chip
	// surface — same data, same predicates. Drill-in switches to the
	// bespoke per-sobject path that doesn't fit this registry. This
	// is the only legacy surface lookup that survives outside TabSpec
	// because the mode-switch on RecordsSObjectCur isn't keyed by
	// (Tab, Subtab).
	if m.tab() == TabRecords {
		if len(m.orgs) > 0 {
			if d := m.data[m.orgs[m.selected].Username]; d != nil && d.RecordsSObjectCur != "" {
				return nil
			}
		}
		return &objectsChipSurface
	}
	return nil
}

// resolveOpenSurface mirrors resolveChipSurface for openSurfaces.
func (m Model) resolveOpenSurface() *openSurface {
	if spec := lookupTabSpec(m.tab()); spec != nil {
		if sub := spec.activeSubtabSpec(m); sub != nil && sub.Open != nil {
			return sub.Open
		}
		if spec.Open != nil {
			return spec.Open
		}
	}
	return nil
}

// resolveListSurface mirrors resolveChipSurface for listSurfaces.
func (m Model) resolveListSurface() *listSurface {
	if spec := lookupTabSpec(m.tab()); spec != nil {
		if sub := spec.activeSubtabSpec(m); sub != nil && sub.List != nil {
			return sub.List
		}
		if spec.List != nil {
			return spec.List
		}
	}
	return nil
}

// resolveSearchPtr walks the spec hierarchy for the bespoke
// SearchPtr escape hatch. Used when no listSurface is registered
// for the active surface but the tab still has a hand-rolled search
// state (e.g. /reports' per-folder cursor).
func (m Model) resolveSearchPtr() func(m Model) *searchState {
	if spec := lookupTabSpec(m.tab()); spec != nil {
		if sub := spec.activeSubtabSpec(m); sub != nil && sub.SearchPtr != nil {
			return sub.SearchPtr
		}
		if spec.SearchPtr != nil {
			return spec.SearchPtr
		}
	}
	return nil
}

// resolveMoveCursor walks the spec hierarchy for bespoke MoveCursor.
func (m Model) resolveMoveCursor() func(m *Model, delta int) {
	if spec := lookupTabSpec(m.tab()); spec != nil {
		if sub := spec.activeSubtabSpec(m); sub != nil && sub.MoveCursor != nil {
			return sub.MoveCursor
		}
		if spec.MoveCursor != nil {
			return spec.MoveCursor
		}
	}
	return nil
}

// resolveResetCursor walks the spec hierarchy for bespoke
// ResetCursor.
func (m Model) resolveResetCursor() func(m *Model) {
	if spec := lookupTabSpec(m.tab()); spec != nil {
		if sub := spec.activeSubtabSpec(m); sub != nil && sub.ResetCursor != nil {
			return sub.ResetCursor
		}
		if spec.ResetCursor != nil {
			return spec.ResetCursor
		}
	}
	return nil
}

// resolveActivate walks the spec hierarchy for the bespoke Activate
// (Enter) handler. Returns nil when nothing is registered.
func (m Model) resolveActivate() func(m *Model) tea.Cmd {
	if spec := lookupTabSpec(m.tab()); spec != nil {
		if sub := spec.activeSubtabSpec(m); sub != nil && sub.Activate != nil {
			return sub.Activate
		}
		if spec.Activate != nil {
			return spec.Activate
		}
	}
	return nil
}

// resolveCycleChip walks the spec hierarchy for the bespoke
// CycleChip (← / →) handler — used by tabs whose chip cursor
// doesn't fit the chipSurface registry.
func (m Model) resolveCycleChip() func(m *Model, delta int) tea.Cmd {
	if spec := lookupTabSpec(m.tab()); spec != nil {
		if spec.CycleChip != nil {
			return spec.CycleChip
		}
	}
	return nil
}

// resolveRenderer returns the renderer to call for the active
// (Tab, Subtab). Subtab Renderer takes priority; falls back to the
// tab's Renderer when subtabs are unstructured (no per-subtab body).
// Nil means the caller renders an empty pane.
func (m Model) resolveRenderer() func(m Model, w, innerH int) string {
	if spec := lookupTabSpec(m.tab()); spec != nil {
		if sub := spec.activeSubtabSpec(m); sub != nil && sub.Renderer != nil {
			return sub.Renderer
		}
		if spec.Renderer != nil {
			return spec.Renderer
		}
	}
	return nil
}

// setSubtabWithOnEnter returns a SetSubtabIdx closure that:
//  1. Calls applyIdx(m, i) to mutate the per-tab cursor field.
//  2. Looks up the subtab at index i on the named Tab.
//  3. Calls its OnEnter hook if set.
//
// Centralises the lazy-load pattern that SOQL uses (and any future
// tab with cached snapshots that hydrate on first entry). The inline
// switch this replaces was tab_registry.go knowing per-tab subtab
// semantics — now subtabs declare their own entry hook and the
// registry stays declarative.
//
// applyIdx isolates the per-tab state mutation (the cursor field
// lives on Model, not on the spec) so this helper can stay generic.
func setSubtabWithOnEnter(tab Tab, applyIdx func(m *Model, i int)) func(m *Model, i int) {
	return func(m *Model, i int) {
		applyIdx(m, i)
		spec := lookupTabSpec(tab)
		if spec == nil || i < 0 || i >= len(spec.Subtabs) {
			return
		}
		if onEnter := spec.Subtabs[i].OnEnter; onEnter != nil {
			onEnter(m)
		}
	}
}
