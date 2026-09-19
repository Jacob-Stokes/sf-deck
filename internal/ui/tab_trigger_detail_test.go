package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func TestTriggerDetailStartsWithScrollableCode(t *testing.T) {
	for _, origin := range []Tab{TabApex, TabObjectDetail, TabDevProjectDetail} {
		t.Run(origin.String(), func(t *testing.T) {
			const username = "trigger-test@example.test"
			const triggerID = "01q000000000001"
			d := &orgData{}
			d.username, d.target = username, username
			d.RecentLoaded = true
			d.Tab = origin
			r := &Resource[sf.TriggerDetail]{}
			r.Set(sf.TriggerDetail{Name: "ExampleTrigger", Body: strings.Repeat("// fictional code\n", 100)})
			d.Triggers.Details = map[string]*Resource[sf.TriggerDetail]{triggerID: r}
			m := Model{
				modelOrgs: modelOrgs{
					orgs: []sf.Org{{Username: username}},
					data: map[string]*orgData{username: d},
				},
				modelServices: modelServices{settings: &settings.Settings{}},
				modelRuntime:  modelRuntime{focus: focusMain, wheel: &wheelRuntime{}},
			}
			m.triggerDetailDrill("Account", triggerID, origin)
			if !m.bodyFocus || m.triggerDetailBackTab() != origin {
				t.Fatal("drill must focus code and preserve its return tab")
			}
			bodyID := triggerBodyID(triggerID)
			for range 30 {
				next, _ := m.handleKey(kp("j"))
				m = next.(Model)
			}
			m.renderTriggerDetail(100, 20)
			if d.BodyCursor[bodyID] != 30 || d.BodyScroll[bodyID] == 0 || m.triggerActionCur != 0 {
				t.Fatal("keyboard movement must scroll code, not action rows")
			}
			next, _ := m.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
			m = next.(Model)
			if d.BodyCursor[bodyID] != 31 {
				t.Fatal("mouse wheel must scroll code immediately")
			}
			next, _ = m.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
			m = next.(Model)
			next, _ = m.handleKey(kp("j"))
			m = next.(Model)
			if m.bodyFocus || m.triggerActionCur != 1 || d.BodyCursor[bodyID] != 31 {
				t.Fatal("Tab must still switch navigation to the actions")
			}
			next, _ = m.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
			m = next.(Model)
			next, _ = m.handleKey(kp("k"))
			m = next.(Model)
			if !m.bodyFocus || d.BodyCursor[bodyID] != 30 {
				t.Fatal("Tab must switch navigation back to code")
			}
		})
	}
}
