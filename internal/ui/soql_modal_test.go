package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func TestSOQLTabEditEscExitsMode(t *testing.T) {
	m := Model{
		modelSOQL: modelSOQL{
			soqlSession: newSOQLSession("SELECT Id FROM Account"),
		},
	}
	m.soqlEditing = true
	m.soqlInput.Focus()

	next, _ := m.handleSOQLKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := next.(Model)
	if got.soqlEditing {
		t.Fatalf("soqlEditing stayed true after esc")
	}
}

func TestSOQLTabEditEnterStartsRun(t *testing.T) {
	m := Model{
		modelSOQL: modelSOQL{
			soqlSession: newSOQLSession("SELECT Id FROM Account"),
		},
		modelOrgs: modelOrgs{
			orgs: []sf.Org{{Username: "user@example.test", Alias: "test"}},
		},
	}
	m.soqlEditing = true
	m.soqlInput.Focus()

	next, cmd := m.handleSOQLKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := next.(Model)
	if got.soqlEditing {
		t.Fatalf("soqlEditing stayed true after enter")
	}
	if !got.soqlRunning {
		t.Fatalf("soqlRunning = false, want true")
	}
	if got.soqlCancel == nil {
		t.Fatalf("soqlCancel is nil after starting run")
	}
	if cmd == nil {
		t.Fatalf("run command is nil")
	}
}

func TestSOQLModalResultDoesNotOverwriteTabSession(t *testing.T) {
	m := Model{
		modelSOQL: modelSOQL{
			soqlSession: newSOQLSession("SELECT Id FROM Account"),
		},
		modelTransient: modelTransient{
			soqlModal: &soqlModalState{
				Title:   "Related rows",
				session: newSOQLSession("SELECT Id FROM Contact"),
			},
		},
	}
	m.soqlRunning = true
	m.soqlRunGen = 1
	m.soqlModal.session.soqlRunning = true
	m.soqlModal.session.soqlRunGen = 2

	next, _ := m.Update(soqlResultMsg{
		session:   soqlSessionModal,
		sessionID: m.soqlModal.session.id,
		gen:       2,
		data: sf.QueryResult{
			TotalSize: 1,
			Records:   []map[string]any{{"Id": "003000000000001"}},
		},
	})
	got := next.(Model)
	if !got.soqlRunning {
		t.Fatalf("tab session running flag changed; modal result should not touch tab session")
	}
	if len(got.soqlResult.Records) != 0 {
		t.Fatalf("tab session records = %d, want untouched empty result", len(got.soqlResult.Records))
	}
	if got.soqlModal == nil {
		t.Fatalf("modal was unexpectedly closed")
	}
	if got.soqlModal.session.soqlRunning {
		t.Fatalf("modal session still running after modal result")
	}
	if got, want := len(got.soqlModal.session.soqlResult.Records), 1; got != want {
		t.Fatalf("modal records = %d, want %d", got, want)
	}
}

func TestSOQLModalDropsResultForPreviousModalSession(t *testing.T) {
	oldSession := newSOQLSession("SELECT Id FROM Contact")
	newSession := newSOQLSession("SELECT Id FROM Opportunity")
	newSession.soqlRunning = true
	newSession.soqlRunGen = 1
	m := Model{
		modelTransient: modelTransient{
			soqlModal: &soqlModalState{
				Title:   "New modal",
				session: newSession,
			},
		},
	}

	next, _ := m.Update(soqlResultMsg{
		session:   soqlSessionModal,
		sessionID: oldSession.id,
		gen:       1,
		data: sf.QueryResult{
			TotalSize: 1,
			Records:   []map[string]any{{"Id": "003000000000001"}},
		},
	})
	got := next.(Model)
	if got.soqlModal == nil {
		t.Fatalf("modal was unexpectedly closed")
	}
	if !got.soqlModal.session.soqlRunning {
		t.Fatalf("new modal running flag changed after stale result")
	}
	if len(got.soqlModal.session.soqlResult.Records) != 0 {
		t.Fatalf("stale result updated new modal session")
	}
}
