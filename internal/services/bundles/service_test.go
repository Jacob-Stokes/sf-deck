package bundles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type fakeRemote struct {
	calls  []string
	target string
	path   string
	out    []byte
}

func (f *fakeRemote) call(name, path, target string) ([]byte, error) {
	f.calls = append(f.calls, name)
	f.path, f.target = path, target
	return f.out, nil
}
func (f *fakeRemote) Retrieve(path, target string) ([]byte, error) {
	return f.call("retrieve", path, target)
}
func (f *fakeRemote) Deploy(path, target string, _ sf.DeployOpts) ([]byte, error) {
	return f.call("deploy", path, target)
}
func (f *fakeRemote) DeployAsync(path, target string, _ sf.DeployOpts) ([]byte, error) {
	return f.call("deploy-async", path, target)
}
func (f *fakeRemote) Validate(path, target string, _ sf.DeployOpts) ([]byte, error) {
	return f.call("validate", path, target)
}
func (f *fakeRemote) ValidateAsync(path, target string, _ sf.DeployOpts) ([]byte, error) {
	return f.call("validate-async", path, target)
}
func (f *fakeRemote) Report(path, target, jobID string) ([]byte, error) {
	return f.call("report", path, target)
}

func bundleStore(t *testing.T, defaultTarget string) (*devproject.Store, string, string) {
	t.Helper()
	store, err := devproject.OpenPath(filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Now()
	project := devproject.DevProject{ID: "project-1", Name: "Test", CreatedAt: now, TouchedAt: now}
	if err := store.CreateDevProject(project); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sfdx-project.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle, err := store.CreateBundle(project.ID, dir, defaultTarget)
	if err != nil {
		t.Fatal(err)
	}
	return store, bundle.ID, dir
}

func TestDeployGatesBundleDefaultTargetNotSelectedOrg(t *testing.T) {
	store, bundleID, _ := bundleStore(t, "prod")
	remote := &fakeRemote{}
	gate := orgwrite.NewGate(func(target string) (sf.Org, error) {
		if target == "prod" {
			return sf.Org{Alias: "prod", Username: "prod@example.com"}, nil
		}
		return sf.Org{Alias: "scratch", Username: "scratch@example.com", IsScratch: true}, nil
	}, func(org sf.Org) settings.SafetyLevel {
		if org.Alias == "prod" {
			return settings.SafetyReadOnly
		}
		return settings.SafetyFull
	})
	service := NewWithRemote(store, gate, remote)
	_, err := service.Deploy(context.Background(), OperationInput{BundleID: bundleID})
	var blocked orgwrite.BlockedError
	if !errors.As(err, &blocked) || blocked.Target != "prod" {
		t.Fatalf("err = %#v, want blocked prod", err)
	}
	if len(remote.calls) != 0 {
		t.Fatalf("remote called despite bundle-target denial: %v", remote.calls)
	}
}

func TestWriteOperationsUseCanonicalGatedTarget(t *testing.T) {
	store, bundleID, path := bundleStore(t, "input")
	remote := &fakeRemote{out: []byte(`{"ok":true}`)}
	gate := orgwrite.NewGate(func(string) (sf.Org, error) {
		return sf.Org{Alias: "resolved", Username: "u@example.com"}, nil
	}, func(sf.Org) settings.SafetyLevel { return settings.SafetyMetadata })
	service := NewWithRemote(store, gate, remote)
	operations := []struct {
		name string
		run  func() (OperationResult, error)
	}{
		{"deploy", func() (OperationResult, error) {
			return service.Deploy(context.Background(), OperationInput{BundleID: bundleID})
		}},
		{"deploy-async", func() (OperationResult, error) {
			return service.DeployAsync(context.Background(), OperationInput{BundleID: bundleID})
		}},
		{"validate", func() (OperationResult, error) {
			return service.Validate(context.Background(), OperationInput{BundleID: bundleID})
		}},
		{"validate-async", func() (OperationResult, error) {
			return service.ValidateAsync(context.Background(), OperationInput{BundleID: bundleID})
		}},
	}
	for _, operation := range operations {
		result, err := operation.run()
		if err != nil || result.Target.CLIArg != "resolved" || remote.target != "resolved" || remote.path != path {
			t.Fatalf("%s result=%#v err=%v remote=%#v", operation.name, result, err, remote)
		}
	}
}

func TestReadOperationsResolveExactTarget(t *testing.T) {
	store, bundleID, _ := bundleStore(t, "origin")
	remote := &fakeRemote{out: []byte("output")}
	gate := orgwrite.NewGate(func(target string) (sf.Org, error) {
		return sf.Org{Alias: "canonical-" + target, Username: target + "@example.com"}, nil
	}, func(sf.Org) settings.SafetyLevel { return settings.SafetyReadOnly })
	service := NewWithRemote(store, gate, remote)
	result, err := service.Retrieve(context.Background(), OperationInput{BundleID: bundleID})
	if err != nil || result.Target.CLIArg != "canonical-origin" || remote.target != "canonical-origin" {
		t.Fatalf("retrieve result=%#v err=%v remote=%#v", result, err, remote)
	}
	result, err = service.Report(context.Background(), ReportInput{BundleID: bundleID, Target: "override", JobID: "0Af"})
	if err != nil || result.Target.CLIArg != "canonical-override" || remote.target != "canonical-override" {
		t.Fatalf("report result=%#v err=%v remote=%#v", result, err, remote)
	}
}

func TestMissingDependenciesFailClosed(t *testing.T) {
	valid := OperationInput{BundleID: "bundle"}
	for _, service := range []*Service{nil, NewWithRemote(nil, nil, nil)} {
		if _, err := service.Deploy(context.Background(), valid); err == nil {
			t.Fatal("missing dependencies accepted")
		}
	}
}
