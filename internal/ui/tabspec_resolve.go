package ui

// Surface resolution helpers that walk the TabSpec/SubtabSpec
// inheritance for chip/open/list surfaces.

import (
	tea "charm.land/bubbletea/v2"
)

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

func (m Model) resolveCycleChip() func(m *Model, delta int) tea.Cmd {
	if spec := lookupTabSpec(m.tab()); spec != nil {
		if spec.CycleChip != nil {
			return spec.CycleChip
		}
	}
	return nil
}

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
