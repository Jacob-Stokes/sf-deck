package ui

import (
	"errors"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"testing"
)

func TestRecordSaveCompletionKeepsOriginAndNewEdits(t *testing.T) {
	a, b := &orgData{}, &orgData{}
	first := a.ensureEditSession("Account", "001000000000001")
	second := b.ensureEditSession("Account", "001000000000001")
	first.Saving = true
	first.Dirty["Name"] = "newer draft"
	first.Dirty["Phone"] = "123"
	first.Editing = &EditState{}
	second.Saving = true
	m := Model{modelOrgs: modelOrgs{orgs: []sf.Org{{Username: "a@example.test"}, {Username: "b@example.test"}}, selected: 1, data: map[string]*orgData{"a@example.test": a, "b@example.test": b}}}
	msg := recordEditSaveMsg{OrgUser: "a@example.test", Target: "a@example.test", Session: first, Sobject: "Account", RecordID: first.RecordID, Fields: map[string]any{"Name": "submitted", "Phone": "123"}}
	_ = m.applyRecordEditSave(msg) // Do not execute the returned refresh command.
	if first.Saving || !second.Saving {
		t.Fatal("completion touched the wrong org")
	}
	if first.Dirty["Name"] != "newer draft" || first.Editing == nil {
		t.Fatal("new edits lost")
	}
	if _, ok := first.Dirty["Phone"]; ok {
		t.Fatal("saved field still dirty")
	}
	if a.EditSessions[editSessionKey("Account", first.RecordID)] != first {
		t.Fatal("draft session deleted")
	}
	replacement := &recordEditSession{Saving: true}
	a.EditSessions[editSessionKey("Account", first.RecordID)] = replacement
	_ = m.applyRecordEditSave(msg)
	if !replacement.Saving {
		t.Fatal("stale completion changed replacement session")
	}
}

func TestOrgSwitchClearsSOQLAndRejectsLateResults(t *testing.T) {
	m := Model{modelOrgs: modelOrgs{orgs: []sf.Org{{Username: "a@example.test"}, {Username: "b@example.test"}}, selectedUsername: "a@example.test"}}
	m.soqlResult = sf.QueryResult{Records: []map[string]any{{"Id": "001000000000001"}}}
	cancelled := false
	m.soqlCancel = func() { cancelled = true }
	m.soqlRunning = true
	m.setSelectedOrg(0)
	if len(m.soqlResult.Records) == 0 || cancelled {
		t.Fatal("same-org selection cleared results")
	}
	m.setSelectedOrg(1)
	if !cancelled || m.soqlRunning || len(m.soqlResult.Records) != 0 {
		t.Fatal("old query survived org switch")
	}
	next, _ := m.Update(soqlResultMsg{orgUser: "a@example.test", data: sf.QueryResult{Records: []map[string]any{{"Id": "001000000000001"}}}})
	if len(next.(Model).soqlResult.Records) != 0 {
		t.Fatal("late result crossed org boundary")
	}
}

func TestRecordSaveFailurePreservesDraftAndSuccessClearsSavedFields(t *testing.T) {
	d := &orgData{}
	s := d.ensureEditSession("Account", "001000000000001")
	s.Dirty["Name"] = "draft"
	s.Saving = true
	m := Model{modelOrgs: modelOrgs{data: map[string]*orgData{"a@example.test": d}}}
	msg := recordEditSaveMsg{OrgUser: "a@example.test", Target: "a@example.test", Session: s, Sobject: s.Sobject, RecordID: s.RecordID, Fields: map[string]any{"Name": "draft"}, Err: errors.New("test failure")}
	_ = m.applyRecordEditSave(msg)
	if s.Saving || s.LastError == "" || s.Dirty["Name"] != "draft" {
		t.Fatal("failure lost draft")
	}
	msg.Err = nil
	_ = m.applyRecordEditSave(msg)
	if len(d.EditSessions) != 0 {
		t.Fatal("successful saved-only draft was not cleared")
	}
}

func TestINValueEscapesSOQL(t *testing.T) {
	if got := formatINValue("", `O'Brien\path`); got != `'O\'Brien\\path'` {
		t.Fatalf("got %q", got)
	}
	if got := formatINValue(42, "42"); got != "42" {
		t.Fatalf("got %q", got)
	}
}
