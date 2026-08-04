package devproject

import (
	"errors"
	"strings"
	"testing"
)

func TestSavedApex_CreateAndGet(t *testing.T) {
	s := openTestStore(t)
	a, err := s.CreateSavedApex("Reset Test User", "Clear opt-outs",
		"User u = [SELECT Id FROM User WHERE Username='qa@acme.com' LIMIT 1];\nu.UserPermissionsMarketingUser = false;\nupdate u;")
	if err != nil {
		t.Fatalf("CreateSavedApex: %v", err)
	}
	if !strings.HasPrefix(a.ID, "ax_") {
		t.Errorf("ID prefix wrong: %q", a.ID)
	}
	if a.CreatedAt.IsZero() || a.UpdatedAt.IsZero() {
		t.Errorf("timestamps zero: %+v", a)
	}
	got, err := s.GetSavedApex(a.ID)
	if err != nil {
		t.Fatalf("GetSavedApex: %v", err)
	}
	if got.Name != a.Name || got.Body != a.Body || got.Description != a.Description {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, a)
	}
}

func TestSavedApex_RejectsEmpty(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.CreateSavedApex("  ", "", "System.debug('x');"); !errors.Is(err, ErrSavedApexEmpty) {
		t.Errorf("blank name should ErrSavedApexEmpty; got %v", err)
	}
	if _, err := s.CreateSavedApex("name", "", "  "); !errors.Is(err, ErrSavedApexEmpty) {
		t.Errorf("blank body should ErrSavedApexEmpty; got %v", err)
	}
}

func TestSavedApex_DeleteCascades(t *testing.T) {
	s := openTestStore(t)
	a, err := s.CreateSavedApex("test", "", "System.debug('hi');")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.DeleteSavedApex(a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.DeleteSavedApex(a.ID); !errors.Is(err, ErrSavedApexNotFound) {
		t.Errorf("second delete should ErrSavedApexNotFound; got %v", err)
	}
}

func TestApexHistory_RoundTrip(t *testing.T) {
	s := openTestStore(t)
	id, err := s.LogApexHistory(ApexHistoryEntry{
		OrgUser:    "u@a.com",
		Body:       "System.debug('hi');",
		DurationMs: 42,
		Compiled:   true,
		Success:    true,
		LogID:      "07L0",
		LogBody:    "00:00:00.000 USER_DEBUG hi",
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive id, got %d", id)
	}
	rows, err := s.ListApexHistory("u@a.com", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	got := rows[0]
	if !got.Compiled || !got.Success || got.DurationMs != 42 {
		t.Errorf("row mismatch: %+v", got)
	}
	if got.LogBody != "00:00:00.000 USER_DEBUG hi" {
		t.Errorf("log body lost: %q", got.LogBody)
	}
}
