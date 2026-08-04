package main

import (
	"flag"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/headless/cli"
	productlegal "github.com/Jacob-Stokes/sf-deck/internal/legal"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

// TestUsageListsEveryHeadlessCommand guards against the --help command
// list drifting from the real noun registry. A user evaluating sf-deck
// from the terminal discovers the headless surface only through this
// text, so a missing or stale entry is a copy bug, not just a doc nit.
func TestUsageListsEveryHeadlessCommand(t *testing.T) {
	// Pull the "  <noun>  description" leaders out of the commands block.
	leader := regexp.MustCompile(`(?m)^  ([a-z]+)\s{2,}\S`)
	listed := map[string]bool{}
	for _, m := range leader.FindAllStringSubmatch(usageCommands, -1) {
		listed[m[1]] = true
	}

	for noun := range cli.KnownNouns {
		if !listed[noun] {
			t.Errorf("KnownNouns has %q but --help doesn't list it", noun)
		}
	}
	for noun := range listed {
		if !cli.KnownNouns[noun] {
			t.Errorf("--help lists %q but it isn't a real command", noun)
		}
	}
}

func TestHeadlessPreflightHelpers(t *testing.T) {
	for _, noun := range []string{"legal", "data", "instance", "verbs"} {
		if got := headlessRouteFor(noun); got != headlessLocal {
			t.Errorf("headlessRouteFor(%q) = %v", noun, got)
		}
	}
	if got := headlessRouteFor("update"); got != headlessUpdate {
		t.Errorf("update route = %v", got)
	}
	if got := headlessRouteFor("org"); got != headlessApp {
		t.Errorf("org route = %v", got)
	}

	args := cli.Args{Noun: "org", Verb: "list"}
	if command, required := headlessLegalRequirement(args, nil); !required || command != "org.list" {
		t.Fatalf("nil settings requirement = %q, %v", command, required)
	}
	if command, required := headlessLegalRequirement(cli.Args{Noun: "org"}, &settings.Settings{}); !required || command != "org" {
		t.Fatalf("fresh settings requirement = %q, %v", command, required)
	}
	st := &settings.Settings{}
	st.AcceptLegal(productlegal.PolicyVersion, time.Now())
	if command, required := headlessLegalRequirement(args, st); required || command != "" {
		t.Fatalf("accepted requirement = %q, %v", command, required)
	}

	if got := headlessMode(cli.Args{JSON: true}); got != headless.JSONMode {
		t.Errorf("JSON mode = %v", got)
	}
	if got := headlessMode(cli.Args{}); got != headless.TextMode {
		t.Errorf("text mode = %v", got)
	}
}

// TestUsageHeaderNamesTheTool is a smoke check that the header copy is
// present and describes the product, not just Go's bare flag dump.
func TestUsageHeaderNamesTheTool(t *testing.T) {
	if !strings.Contains(usageHeader, "sf-deck") ||
		!strings.Contains(usageHeader, "Salesforce") {
		t.Error("usage header should name sf-deck and Salesforce")
	}
}

func TestMainVersionFlag(t *testing.T) {
	oldArgs := os.Args
	oldStdout := os.Stdout
	oldFlags := flag.CommandLine
	t.Cleanup(func() {
		os.Args = oldArgs
		os.Stdout = oldStdout
		flag.CommandLine = oldFlags
	})

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	os.Args = []string{"sf-deck", "--version"}
	flag.CommandLine = flag.NewFlagSet("sf-deck-test", flag.ContinueOnError)

	main()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	if got := string(out); !strings.Contains(got, "sf-deck dev") ||
		!strings.Contains(got, "commit:") ||
		!strings.Contains(got, "built:") {
		t.Fatalf("--version output = %q", got)
	}
}

func TestValidatePprofAddrRequiresLoopback(t *testing.T) {
	for _, addr := range []string{"localhost:6060", "127.0.0.1:6060", "[::1]:6060"} {
		if err := validatePprofAddr(addr); err != nil {
			t.Errorf("validatePprofAddr(%q): %v", addr, err)
		}
	}
	for _, addr := range []string{":6060", "0.0.0.0:6060", "192.0.2.1:6060", "bad-address"} {
		if err := validatePprofAddr(addr); err == nil {
			t.Errorf("validatePprofAddr(%q) unexpectedly succeeded", addr)
		}
	}
}
