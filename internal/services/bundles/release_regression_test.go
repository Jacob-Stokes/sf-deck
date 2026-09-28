package bundles

import (
	"context"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAsyncSubmissionDoesNotMarkDeployed(t *testing.T) {
	s, id, _ := bundleStore(t, "test")
	gate := orgwrite.NewGate(func(string) (sf.Org, error) { return sf.Org{Username: "test@example.test"}, nil }, func(sf.Org) settings.SafetyLevel { return settings.SafetyFull })
	svc := NewWithRemote(s, gate, &fakeRemote{})
	if _, err := svc.DeployAsync(context.Background(), OperationInput{BundleID: id}); err != nil {
		t.Fatal(err)
	}
	b, err := fetchBundle(s, id)
	if err != nil {
		t.Fatal(err)
	}
	if !b.LastDeployedAt.IsZero() {
		t.Fatal("submission marked deployed")
	}
	if _, err := svc.Deploy(context.Background(), OperationInput{BundleID: id}); err != nil {
		t.Fatal(err)
	}
	b, err = fetchBundle(s, id)
	if err != nil {
		t.Fatal(err)
	}
	if b.LastDeployedAt.IsZero() {
		t.Fatal("completed deploy not marked")
	}
}

func TestCreateWritesRecordSidecar(t *testing.T) {
	s, _, _ := bundleStore(t, "test")
	if _, err := s.AddItem(devproject.Item{DevProjectID: "project-1", OrgUser: "test@example.test", Kind: devproject.KindRecord, Ref: "001000000000001", Name: "Fictional account", Type: "Account"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bundle")
	result, err := Create(s, CreateInput{ProjectID: "project-1", OrgUser: "test@example.test", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(path, "records.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if result.RecordsExported != 1 || !strings.Contains(string(content), "001000000000001") {
		t.Fatalf("sidecar mismatch: %s", content)
	}
}
