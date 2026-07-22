package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// newTestApp returns an App with a stubbed Settings + Orgs slice.
// Cache / Projects / Usage are left nil — none of the safety helpers
// touch them.
func newTestApp(orgs []sf.Org, st *settings.Settings) *App {
	if st == nil {
		st = &settings.Settings{}
	}
	return &App{Settings: st, Orgs: orgs}
}

func TestResolveOrg(t *testing.T) {
	prod := sf.Org{Alias: "prod", Username: "boss@example.com"}
	sand := sf.Org{Alias: "sand", Username: "qa@example.com.sandbox", IsSandbox: true}
	scratch := sf.Org{Username: "test-abc123@example.com", IsScratch: true} // no alias

	a := newTestApp([]sf.Org{prod, sand, scratch}, nil)

	t.Run("empty target returns first org", func(t *testing.T) {
		got, err := a.ResolveOrg("")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got.Username != prod.Username {
			t.Errorf("got %q, want %q", got.Username, prod.Username)
		}
	})

	t.Run("alias lookup", func(t *testing.T) {
		got, err := a.ResolveOrg("sand")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got.Username != sand.Username {
			t.Errorf("got %q, want %q", got.Username, sand.Username)
		}
	})

	t.Run("username lookup", func(t *testing.T) {
		got, err := a.ResolveOrg("test-abc123@example.com")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !got.IsScratch {
			t.Errorf("got %+v, want scratch", got)
		}
	})

	t.Run("alias preferred over conflicting username", func(t *testing.T) {
		// Build orgs where org A's alias equals org B's username — alias
		// match should win.
		conflict := sf.Org{Alias: "shared@x", Username: "alice@example.com"}
		other := sf.Org{Username: "shared@x"}
		conflictApp := newTestApp([]sf.Org{other, conflict}, nil)
		got, err := conflictApp.ResolveOrg("shared@x")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got.Username != "alice@example.com" {
			t.Errorf("got %q, want alias winner alice@example.com", got.Username)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := a.ResolveOrg("missing")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "missing") {
			t.Errorf("err = %v, want substring 'missing'", err)
		}
	})

	t.Run("empty target prefers pinned default", func(t *testing.T) {
		pinned := &settings.Settings{Orgs: map[string]settings.OrgConfig{
			scratch.Username: {Default: true},
		}}
		app := newTestApp([]sf.Org{prod, sand, scratch}, pinned)
		got, err := app.ResolveOrg("")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got.Username != scratch.Username {
			t.Errorf("got %q, want pinned scratch %q", got.Username, scratch.Username)
		}
	})

	t.Run("pinned org missing from connected list falls back", func(t *testing.T) {
		// Pin a username that isn't in the connected org list (e.g.
		// user logged out via sf CLI while sf-deck wasn't running).
		// Should silently fall back to the first connected org
		// rather than erroring.
		pinned := &settings.Settings{Orgs: map[string]settings.OrgConfig{
			"ghost@example.com": {Default: true},
		}}
		app := newTestApp([]sf.Org{prod, sand}, pinned)
		got, err := app.ResolveOrg("")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got.Username != prod.Username {
			t.Errorf("got %q, want %q", got.Username, prod.Username)
		}
	})

	t.Run("empty target with no orgs", func(t *testing.T) {
		// Stub ListOrgs so the refresh-on-empty fallback can't pick
		// up the real machine's orgs and accidentally "succeed."
		// Restore on exit so later sub-tests see the real impl.
		orig := sf.ListOrgs
		sf.ListOrgs = func() ([]sf.Org, error) { return nil, nil }
		defer func() { sf.ListOrgs = orig }()

		empty := newTestApp(nil, nil)
		_, err := empty.ResolveOrg("")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil app", func(t *testing.T) {
		var nilApp *App
		_, err := nilApp.ResolveOrg("anything")
		if err == nil {
			t.Fatal("expected error on nil app")
		}
	})
}

func TestSafetyFor_KindDefaults(t *testing.T) {
	// Empty Settings → fall through to kind-based defaults.
	a := newTestApp(nil, &settings.Settings{})

	cases := []struct {
		name string
		org  sf.Org
		want settings.SafetyLevel
	}{
		{"production", sf.Org{Username: "p@x"}, settings.SafetyReadOnly},
		{"sandbox", sf.Org{Username: "s@x", IsSandbox: true}, settings.SafetyRecords},
		{"scratch", sf.Org{Username: "sc@x", IsScratch: true}, settings.SafetyFull},
		{"devhub", sf.Org{Username: "d@x", IsDevHub: true}, settings.SafetyRecords},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := a.SafetyFor(c.org); got != c.want {
				t.Errorf("SafetyFor(%s) = %s, want %s",
					c.name, got, c.want)
			}
		})
	}
}

func TestSafetyFor_AliasOverride(t *testing.T) {
	// Sandbox would default to "records" — but per-alias override sets
	// it to "read_only".
	st := &settings.Settings{
		Orgs: map[string]settings.OrgConfig{
			"sand": {Safety: "read_only"},
		},
	}
	a := newTestApp(nil, st)
	o := sf.Org{Alias: "sand", Username: "qa@example.com.sandbox", IsSandbox: true}
	if got := a.SafetyFor(o); got != settings.SafetyReadOnly {
		t.Errorf("SafetyFor(sandbox with alias override) = %s, want read_only", got)
	}
}

func TestSafetyFor_NilGuards(t *testing.T) {
	// Nil app should default to read-only — most conservative.
	var nilApp *App
	if got := nilApp.SafetyFor(sf.Org{IsScratch: true}); got != settings.SafetyReadOnly {
		t.Errorf("nil app SafetyFor = %s, want read_only", got)
	}
	// App with nil Settings same story.
	a := &App{}
	if got := a.SafetyFor(sf.Org{IsScratch: true}); got != settings.SafetyReadOnly {
		t.Errorf("nil settings SafetyFor = %s, want read_only", got)
	}
}

func TestCanWrite(t *testing.T) {
	prod := sf.Org{Alias: "prod", Username: "boss@example.com"}
	sand := sf.Org{Alias: "sand", Username: "qa@example.com", IsSandbox: true}
	scratch := sf.Org{Alias: "scr", Username: "scr@example.com", IsScratch: true}

	a := newTestApp([]sf.Org{prod, sand, scratch}, &settings.Settings{})

	t.Run("prod blocks record write", func(t *testing.T) {
		err := a.CanWrite(prod, settings.WriteRecord)
		if err == nil {
			t.Fatal("expected block")
		}
		var be BlockedError
		if !errors.As(err, &be) {
			t.Fatalf("err type = %T, want BlockedError", err)
		}
		if be.Target != "prod" {
			t.Errorf("Target = %q, want prod", be.Target)
		}
		if be.Required != settings.WriteRecord {
			t.Errorf("Required = %v, want WriteRecord", be.Required)
		}
		if be.Actual != settings.SafetyReadOnly {
			t.Errorf("Actual = %v, want SafetyReadOnly", be.Actual)
		}
		// Message should mention the actual level + what was required.
		msg := be.Error()
		if !strings.Contains(msg, "read_only") || !strings.Contains(msg, "records") {
			t.Errorf("Error() = %q, want substrings 'read_only' and 'records'", msg)
		}
	})

	t.Run("sandbox allows record write", func(t *testing.T) {
		if err := a.CanWrite(sand, settings.WriteRecord); err != nil {
			t.Errorf("CanWrite = %v, want nil", err)
		}
	})

	t.Run("sandbox blocks metadata write", func(t *testing.T) {
		err := a.CanWrite(sand, settings.WriteMetadata)
		if err == nil {
			t.Fatal("expected block")
		}
		var be BlockedError
		if !errors.As(err, &be) {
			t.Fatalf("err type = %T, want BlockedError", err)
		}
		if be.Required != settings.WriteMetadata {
			t.Errorf("Required = %v, want WriteMetadata", be.Required)
		}
	})

	t.Run("scratch allows anonymous apex", func(t *testing.T) {
		if err := a.CanWrite(scratch, settings.WriteAnonymous); err != nil {
			t.Errorf("CanWrite scratch anon = %v, want nil", err)
		}
	})

	t.Run("sandbox blocks anonymous apex", func(t *testing.T) {
		err := a.CanWrite(sand, settings.WriteAnonymous)
		if err == nil {
			t.Fatal("expected block")
		}
	})
}

func TestBlockedError_Message(t *testing.T) {
	be := BlockedError{
		Target:   "prod",
		Username: "boss@x",
		Required: settings.WriteMetadata,
		Actual:   settings.SafetyReadOnly,
	}
	msg := be.Error()
	if !strings.Contains(msg, "prod") {
		t.Errorf("missing target: %q", msg)
	}
	if !strings.Contains(msg, "read_only") {
		t.Errorf("missing actual: %q", msg)
	}
	if !strings.Contains(msg, "metadata") {
		t.Errorf("missing required: %q", msg)
	}
}

func TestWriteKindLabel(t *testing.T) {
	cases := []struct {
		k    settings.WriteKind
		want string
	}{
		{settings.WriteRecord, "records"},
		{settings.WriteMetadata, "metadata"},
		{settings.WriteAnonymous, "full"},
		{settings.WriteKind(99), "unknown"},
	}
	for _, c := range cases {
		if got := writeKindLabel(c.k); got != c.want {
			t.Errorf("writeKindLabel(%d) = %q, want %q", c.k, got, c.want)
		}
	}
}

func TestTargetArg(t *testing.T) {
	if got := TargetArg(sf.Org{Alias: "dev", Username: "u@x"}); got != "dev" {
		t.Errorf("alias case = %q, want dev", got)
	}
	if got := TargetArg(sf.Org{Username: "u@x"}); got != "u@x" {
		t.Errorf("no-alias case = %q, want u@x", got)
	}
}

func TestOrgKind(t *testing.T) {
	cases := []struct {
		name string
		o    sf.Org
		want settings.OrgKind
	}{
		{"production", sf.Org{}, settings.KindProduction},
		{"sandbox", sf.Org{IsSandbox: true}, settings.KindSandbox},
		{"scratch", sf.Org{IsScratch: true}, settings.KindScratch},
		{"devhub", sf.Org{IsDevHub: true}, settings.KindDevHub},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := orgKind(c.o); got != c.want {
				t.Errorf("orgKind = %v, want %v", got, c.want)
			}
		})
	}
}
