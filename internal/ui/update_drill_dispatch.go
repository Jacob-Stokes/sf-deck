package ui

// Drill-pop message dispatch.
//
// Pairs with the other update_*_dispatch.go files. Handles the
// "the entity I was drilled into got deleted, pop back to its parent
// list and refresh" callbacks for validation rules, record types,
// triggers, and fields. Each follows the same shape:
//
//   1. If currently on the detail tab, pop to the parent tab.
//   2. Clear the per-entity drill cache key.
//   3. Fire msg.innerCmd which the delete handler attached (typically
//      a list-refresh cmd so the parent list reflects the deletion).

import (
	tea "charm.land/bubbletea/v2"
)

// dispatchDrillMsg routes drill-pop callbacks.
func (m Model) dispatchDrillMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case validationRulePoppedMsg:
		// Pop back to the Validation subtab (the rule we were
		// drilled into no longer exists) and fire the list refresh.
		if m.tab() == TabValidationDetail {
			m.setTab(TabObjectDetail)
		}
		if len(m.orgs) > 0 {
			if d := m.data[m.orgs[m.selected].Username]; d != nil {
				d.ValidationRules.ClearDrill(msg.ruleID)
			}
		}
		return m, msg.innerCmd, true

	case recordTypePoppedMsg:
		// Pop back to the Record Types subtab (the record type we
		// were drilled into no longer exists) and fire the list
		// refresh.
		if m.tab() == TabRecordTypeDetail {
			m.setTab(TabObjectDetail)
		}
		if len(m.orgs) > 0 {
			if d := m.data[m.orgs[m.selected].Username]; d != nil {
				d.RecordTypes.ClearDrill(msg.rtID)
			}
		}
		return m, msg.innerCmd, true

	case triggerPoppedMsg:
		// Pop back from TabTriggerDetail after a delete.
		if m.tab() == TabTriggerDetail {
			m.setTab(m.triggerDetailBackTab())
		}
		if len(m.orgs) > 0 {
			if d := m.data[m.orgs[m.selected].Username]; d != nil {
				d.Triggers.ClearDrill(msg.id)
			}
		}
		return m, msg.innerCmd, true

	case fieldDeletedMsg:
		// Pop back from TabFieldDetail (the field is gone) and fire
		// the describe refresh so the Schema list reflects the
		// deletion. Evict the now-stale CustomField Id precisely.
		if m.tab() == TabFieldDetail {
			m.setTab(TabObjectDetail)
		}
		if len(m.orgs) > 0 {
			if d := m.data[m.orgs[m.selected].Username]; d != nil {
				d.FieldCur = ""
				if msg.cacheKey != "" {
					// customIDMu: edit-modal goroutines write this map
					// via customFieldIDCached — the unlocked delete was
					// a fatal-map-race candidate.
					d.customIDMu.Lock()
					delete(d.CustomFieldIDs, msg.cacheKey)
					d.customIDMu.Unlock()
				}
			}
		}
		return m, msg.innerCmd, true
	}
	return m, nil, false
}
