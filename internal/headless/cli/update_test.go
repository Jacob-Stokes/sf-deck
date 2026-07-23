package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/buildinfo"
	"github.com/Jacob-Stokes/sf-deck/internal/updatecheck"
)

type updateStub struct {
	result updatecheck.Result
	err    error
	opts   updatecheck.Options
}

func (s *updateStub) Check(_ context.Context, _ string, opts updatecheck.Options) (updatecheck.Result, error) {
	s.opts = opts
	return s.result, s.err
}

func TestUpdateCheckJSON(t *testing.T) {
	buildinfo.Set("0.1.0", "abc", "today")
	t.Cleanup(func() { buildinfo.Set("dev", "none", "unknown") })
	stub := &updateStub{result: updatecheck.Result{
		CurrentVersion:  "v0.1.0",
		LatestVersion:   "v0.2.0",
		UpdateAvailable: true,
		Kind:            "minor",
		ReleaseURL:      "https://example.test/v0.2.0",
	}}
	a := newTestApp()
	a.Updates = stub
	code, got, _ := runCLI(t, a, "update", "check", "--force", "--json")
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	if !stub.opts.Force {
		t.Fatal("--force was not forwarded")
	}
	data, ok := got["data"].(map[string]any)
	if !ok || data["kind"] != "minor" || data["update_available"] != true {
		t.Fatalf("data = %#v", got["data"])
	}
	if _, exists := data["published_at"]; exists {
		t.Fatalf("zero published_at should be omitted: %#v", data)
	}
}

func TestUpdateCheckText(t *testing.T) {
	stub := &updateStub{result: updatecheck.Result{
		CurrentVersion:  "v0.1.0",
		LatestVersion:   "v0.1.1",
		UpdateAvailable: true,
		Kind:            "patch",
		ReleaseURL:      "https://example.test/v0.1.1",
	}}
	a := newTestApp()
	a.Updates = stub
	code, _, out := runCLI(t, a, "update", "check")
	if code != 0 || !strings.Contains(out, "patch update available") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestUpdateCheckError(t *testing.T) {
	a := newTestApp()
	a.Updates = &updateStub{err: errors.New("offline")}
	code, got, _ := runCLI(t, a, "update", "check", "--json")
	if code != 1 {
		t.Fatalf("code = %d", code)
	}
	errBody, _ := got["error"].(map[string]any)
	if errBody["code"] != "internal_error" {
		t.Fatalf("error = %#v", errBody)
	}
}
