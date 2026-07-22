package orgwrite

import (
	"errors"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func TestReadTargetFailsClosedWithoutResolver(t *testing.T) {
	for _, gate := range []*Gate{nil, NewGate(nil, nil)} {
		if _, err := gate.ReadTarget("prod"); err == nil {
			t.Fatal("ReadTarget must fail when resolver is unavailable")
		}
	}
}

func TestRequireFailsClosedWithoutSafetyPolicy(t *testing.T) {
	g := NewGate(func(string) (sf.Org, error) {
		return sf.Org{Alias: "prod", Username: "user@example.com"}, nil
	}, nil)
	if _, err := g.Require("prod", settings.WriteRecord); err == nil {
		t.Fatal("Require must fail when safety policy is unavailable")
	}
}

func TestReadTargetCanonicalIdentity(t *testing.T) {
	tests := []struct {
		name string
		org  sf.Org
		want string
	}{
		{name: "alias preferred", org: sf.Org{Alias: "dev", Username: "u@example.com"}, want: "dev"},
		{name: "username fallback", org: sf.Org{Username: "u@example.com"}, want: "u@example.com"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGate(func(string) (sf.Org, error) { return tc.org, nil }, nil)
			got, err := g.ReadTarget("input")
			if err != nil {
				t.Fatal(err)
			}
			if got.CLIArg != tc.want || got.Username != tc.org.Username || got.Org != tc.org {
				t.Fatalf("target = %#v, want arg=%q username=%q org=%#v", got, tc.want, tc.org.Username, tc.org)
			}
		})
	}
}

func TestRequireResolvesOnceAndReturnsExecutionTarget(t *testing.T) {
	calls := 0
	o := sf.Org{Alias: "scratch", Username: "scratch@example.com", IsScratch: true}
	g := NewGate(func(target string) (sf.Org, error) {
		calls++
		if target != "requested" {
			t.Fatalf("resolver target = %q", target)
		}
		return o, nil
	}, func(got sf.Org) settings.SafetyLevel {
		if got != o {
			t.Fatalf("safety org = %#v, want %#v", got, o)
		}
		return settings.SafetyFull
	})
	got, err := g.Require("requested", settings.WriteAnonymous)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", calls)
	}
	if got.CLIArg != "scratch" || got.Username != o.Username {
		t.Fatalf("target = %#v", got)
	}
}

func TestRequireSafetyMatrix(t *testing.T) {
	levels := []settings.SafetyLevel{
		settings.SafetyReadOnly,
		settings.SafetyRecords,
		settings.SafetyMetadata,
		settings.SafetyFull,
	}
	kinds := []settings.WriteKind{
		settings.WriteRecord,
		settings.WriteMetadata,
		settings.WriteAnonymous,
	}
	for _, level := range levels {
		for _, kind := range kinds {
			g := NewGate(func(string) (sf.Org, error) {
				return sf.Org{Alias: "org", Username: "u@example.com"}, nil
			}, func(sf.Org) settings.SafetyLevel { return level })
			_, err := g.Require("org", kind)
			if level.Allows(kind) {
				if err != nil {
					t.Errorf("level %s kind %d unexpectedly blocked: %v", level.String(), kind, err)
				}
				continue
			}
			var blocked BlockedError
			if !errors.As(err, &blocked) {
				t.Errorf("level %s kind %d error = %T, want BlockedError", level.String(), kind, err)
				continue
			}
			if blocked.Target != "org" || blocked.Username != "u@example.com" ||
				blocked.Required != kind || blocked.Actual != level {
				t.Errorf("blocked = %#v", blocked)
			}
		}
	}
}

func TestRequirePropagatesResolutionError(t *testing.T) {
	want := errors.New("org missing")
	g := NewGate(func(string) (sf.Org, error) { return sf.Org{}, want },
		func(sf.Org) settings.SafetyLevel { return settings.SafetyFull })
	if _, err := g.Require("missing", settings.WriteRecord); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestBlockedErrorMessage(t *testing.T) {
	err := BlockedError{
		Target: "prod", Required: settings.WriteAnonymous, Actual: settings.SafetyMetadata,
	}
	if got, want := err.Error(), "prod is metadata; requires full"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}
