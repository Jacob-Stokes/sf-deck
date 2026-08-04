package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/exporters"
)

func TestFreshBundleExportRefusesNonEmptyDestination(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "package.xml")
	want := []byte("<Package>keep me</Package>")
	if err := os.WriteFile(manifest, want, 0o600); err != nil {
		t.Fatal(err)
	}
	m := Model{}
	cmd := m.exportProjectBundle(exportProjectPathPickedMsg{
		DevID: "p1", DevName: "Project", Format: exporters.FormatSfdxProject, Path: dir,
	}, nil, "")
	if cmd != nil {
		t.Fatal("refused export unexpectedly returned work")
	}
	got, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("existing manifest was changed: %q", got)
	}
	if !strings.Contains(m.banner, "export refused") {
		t.Fatalf("banner = %q, want refusal", m.banner)
	}
}

func TestDevProjectSingleFileExportRefusesOverwriteAndWritesPrivately(t *testing.T) {
	store, err := devproject.OpenPath(filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.CreateDevProject(devproject.DevProject{
		ID: "p1", Name: "Project", CreatedAt: time.Now(), TouchedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddItem(devproject.Item{
		DevProjectID: "p1", Kind: devproject.KindSObject, Ref: "Account", Name: "Account",
	}); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "project.json")
	want := []byte("keep existing contents")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}
	m := Model{modelServices: modelServices{
		devProjects: store,
		exports:     &exportRegistry{},
	}}
	msg := exportProjectPathPickedMsg{
		DevID: "p1", DevName: "Project", Format: exporters.FormatJSON, Path: path,
		ScopeAllOrgs: true,
	}
	m.applyExportProjectPathPicked(msg)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("unconfirmed overwrite changed destination: %q", got)
	}
	if !strings.Contains(m.banner, "file already exists") {
		t.Fatalf("banner = %q, want existing-file error", m.banner)
	}

	msg.Overwrite = true
	m.applyExportProjectPathPicked(msg)
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) == string(want) || !strings.Contains(string(got), "Account") {
		t.Fatalf("confirmed export was not published: %q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("export permissions = %o, want 600", perm)
	}
}
