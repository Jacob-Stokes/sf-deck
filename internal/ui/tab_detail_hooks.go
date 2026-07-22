package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func (m *Model) moveFieldDetailCursor(delta int) {
	// Cursor walks the navigable rows of the MAIN pane (every property,
	// editable or read-only), not the sidebar action menu. The sidebar
	// is info-only now.
	n := m.fieldDetailNavCount()
	m.fieldActionCur = clampDelta(m.fieldActionCur, delta, n)
}

func (m *Model) activateFieldDetail() tea.Cmd {
	// Enter fires the action the cursored MAIN-pane row maps to.
	// Read-only rows (SOQL caps, picklist values, …) are a no-op.
	idx, ok := m.fieldDetailActionForCursor()
	if !ok {
		return nil
	}
	mm, cmd := StartAction(*m, fieldRegistry, idx)
	*m = mm
	return cmd
}

// ensureFieldDetailData is the EnsureData hook for TabFieldDetail.
// Mirrors ensureObjectDetailData: triggers the describe fetch for
// d.DescribeCur so the field detail renders real content instead of
// sitting at "loading describe…" forever when the user drilled in
// from a path that hadn't already populated the describe (most
// commonly /dev-projects or /tag-detail).
//
// The /objects → Schema path that's been working all along
// pre-fetches the describe before the user ever drills into a field;
// THAT's why the field detail rendered correctly from there. Without
// this hook other entry paths sit at the loading placeholder
// indefinitely because nothing else triggers the describe Resource.
func (m *Model) ensureFieldDetailData(d *orgData, o sf.Org) tea.Cmd {
	if d == nil || d.DescribeCur == "" {
		return nil
	}
	r := d.EnsureDescribe(targetArg(o), d.DescribeCur)
	return r.Ensure(m.cache)
}

func (m *Model) moveValidationDetailCursor(delta int) {
	// Cursor walks the navigable MAIN-pane rows; sidebar is info-only.
	n := m.validationDetailNavCount()
	m.validationActionCur = clampDelta(m.validationActionCur, delta, n)
}

func (m *Model) activateValidationDetail() tea.Cmd {
	idx, ok := m.validationDetailActionForCursor()
	if !ok {
		return nil
	}
	mm, cmd := StartAction(*m, validationRegistry, idx)
	*m = mm
	return cmd
}

func (m *Model) ensureValidationDetailData(d *orgData, o sf.Org) tea.Cmd {
	if d.DescribeCur == "" || d.ValidationRules.DrillID == "" {
		return nil
	}
	r := d.EnsureValidationRuleDetail(targetArg(o), d.ValidationRules.DrillID)
	return r.Ensure(m.cache)
}

func (m Model) refreshValidationDetailData(d *orgData) tea.Cmd {
	cmds := []tea.Cmd{}
	if d.DescribeCur != "" {
		if vr, ok := d.ValidationRules.Lists[d.DescribeCur]; ok {
			cmds = append(cmds, vr.Refresh(m.cache))
		}
	}
	if d.ValidationRules.DrillID != "" {
		if det, ok := d.ValidationRules.Details[d.ValidationRules.DrillID]; ok {
			cmds = append(cmds, det.Refresh(m.cache))
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) moveRecordTypeDetailCursor(delta int) {
	n := m.recordTypeDetailNavCount()
	m.recordTypeActionCur = clampDelta(m.recordTypeActionCur, delta, n)
}

func (m *Model) activateRecordTypeDetail() tea.Cmd {
	idx, ok := m.recordTypeDetailActionForCursor()
	if !ok {
		return nil
	}
	mm, cmd := StartAction(*m, recordTypeRegistry, idx)
	*m = mm
	return cmd
}

func (m *Model) ensureRecordTypeDetailData(d *orgData, o sf.Org) tea.Cmd {
	if d.DescribeCur == "" || d.RecordTypes.DrillID == "" {
		return nil
	}
	r := d.EnsureRecordTypeDetail(targetArg(o), d.RecordTypes.DrillID)
	return r.Ensure(m.cache)
}

func (m Model) refreshRecordTypeDetailData(d *orgData) tea.Cmd {
	cmds := []tea.Cmd{}
	if d.DescribeCur != "" {
		if rt, ok := d.RecordTypes.Lists[d.DescribeCur]; ok {
			cmds = append(cmds, rt.Refresh(m.cache))
		}
	}
	if d.RecordTypes.DrillID != "" {
		if det, ok := d.RecordTypes.Details[d.RecordTypes.DrillID]; ok {
			cmds = append(cmds, det.Refresh(m.cache))
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) moveTriggerDetailCursor(delta int) {
	if !m.bodyFocus {
		// Row cursor has focus — walk the 3 main-pane action rows
		// (status / edit body / delete).
		m.triggerActionCur = clampDelta(m.triggerActionCur, delta, 3)
		return
	}
	// Body focused — scroll the Apex source.
	d := m.activeOrgData()
	if d == nil || d.Triggers.DrillID == "" {
		return
	}
	r, ok := d.Triggers.Details[d.Triggers.DrillID]
	if !ok || r == nil {
		return
	}
	body := r.Value().Body
	if body == "" {
		return
	}
	m.codeViewMoveCursor(d, triggerBodyID(d.Triggers.DrillID), lineCount(body), delta)
}

func (m *Model) activateTriggerDetail() tea.Cmd {
	// Enter fires the cursored row's action. When the body is focused,
	// the natural action is "edit body" (Enter while reading the
	// source opens the editor).
	idx := trgActEditBody
	if !m.bodyFocus {
		mapped, ok := triggerNavActionForCursor(m.triggerActionCur)
		if !ok {
			return nil
		}
		idx = mapped
	}
	mm, cmd := StartAction(*m, triggerRegistry, idx)
	*m = mm
	return cmd
}

func (m *Model) ensureTriggerDetailData(d *orgData, o sf.Org) tea.Cmd {
	if d.DescribeCur == "" || d.Triggers.DrillID == "" {
		return nil
	}
	r := d.EnsureTriggerDetail(targetArg(o), d.Triggers.DrillID)
	return r.Ensure(m.cache)
}

func (m Model) refreshTriggerDetailData(d *orgData) tea.Cmd {
	cmds := []tea.Cmd{}
	if d.DescribeCur != "" {
		if tr, ok := d.Triggers.Lists[d.DescribeCur]; ok {
			cmds = append(cmds, tr.Refresh(m.cache))
		}
	}
	if d.Triggers.DrillID != "" {
		if det, ok := d.Triggers.Details[d.Triggers.DrillID]; ok {
			cmds = append(cmds, det.Refresh(m.cache))
		}
	}
	return tea.Batch(cmds...)
}
