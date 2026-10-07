package ui

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) dispatchDrillMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case validationRulePoppedMsg:
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
