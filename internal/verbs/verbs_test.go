package verbs

import (
	"strings"
	"testing"
)

func TestNoDuplicateQualifiedNames(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range registry {
		q := s.Qualified()
		if seen[q] {
			t.Errorf("duplicate Spec.Qualified() = %q", q)
		}
		seen[q] = true
	}
}

func TestEverySpecHasASurface(t *testing.T) {
	for _, s := range registry {
		if s.CLI == nil && s.IPC == nil && !s.TUIOnly {
			t.Errorf("%s declares no CLI, no IPC, and is not TUIOnly — unreachable", s.Qualified())
		}
	}
}

func TestSummariesAreSingleLine(t *testing.T) {
	for _, s := range registry {
		if strings.Contains(s.Summary, "\n") {
			t.Errorf("%s.Summary contains a newline; keep summaries single-line", s.Qualified())
		}
	}
}

func TestIPCCommandMatchesQualifiedName(t *testing.T) {
	// Conventional rule: the IPC Command should match the Spec's
	// Qualified() name. Catches typos like "soql.hist.list" vs
	// the registry's "soql.history.list".
	for _, s := range registry {
		if s.IPC == nil {
			continue
		}
		if s.IPC.Command != s.Qualified() {
			t.Errorf("%s: IPC.Command = %q, expected %q",
				s.Qualified(), s.IPC.Command, s.Qualified())
		}
	}
}

func TestSpecsForSurfaceFilters(t *testing.T) {
	cli := SpecsForSurface(SurfaceCLI)
	ipc := SpecsForSurface(SurfaceIPC)
	if len(cli) == 0 {
		t.Error("no CLI specs found")
	}
	if len(ipc) == 0 {
		t.Error("no IPC specs found")
	}
	for _, s := range cli {
		if !s.HasCLI() {
			t.Errorf("%s in CLI surface but HasCLI() = false", s.Qualified())
		}
	}
	for _, s := range ipc {
		if !s.HasIPC() {
			t.Errorf("%s in IPC surface but HasIPC() = false", s.Qualified())
		}
	}
}

func TestByQualifiedLookup(t *testing.T) {
	// Sanity: a well-known verb should be findable.
	if _, ok := ByQualified("bundle.deploy"); !ok {
		t.Error("bundle.deploy not findable via ByQualified")
	}
	if _, ok := ByQualified("nonexistent.verb"); ok {
		t.Error("ByQualified returned true for a fake verb")
	}
}
