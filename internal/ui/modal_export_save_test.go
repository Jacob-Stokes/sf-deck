package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestExportSaveRequiresSecondEnterToOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.csv")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	overwrite := false
	m := Model{}
	m.exportSave = &exportSaveState{
		Path: path,
		Confirm: func(_ string, _ bool, confirmed bool) tea.Cmd {
			called = true
			overwrite = confirmed
			return nil
		},
	}

	first, _ := m.handleExportSaveKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = first.(Model)
	if called || m.exportSave == nil || m.exportSave.Err == "" {
		t.Fatalf("first Enter called=%v state=%#v", called, m.exportSave)
	}
	second, _ := m.handleExportSaveKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = second.(Model)
	if !called || !overwrite || m.exportSave != nil {
		t.Fatalf("second Enter called=%v overwrite=%v state=%#v", called, overwrite, m.exportSave)
	}
}

func TestExportSaveNewPathDoesNotRequestOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.csv")
	called := false
	m := Model{}
	m.exportSave = &exportSaveState{
		Path: path,
		Confirm: func(_ string, _ bool, overwrite bool) tea.Cmd {
			called = true
			if overwrite {
				t.Error("new path marked as overwrite")
			}
			return nil
		},
	}
	next, _ := m.handleExportSaveKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if !called || m.exportSave != nil {
		t.Fatalf("called=%v state=%#v", called, m.exportSave)
	}
}
