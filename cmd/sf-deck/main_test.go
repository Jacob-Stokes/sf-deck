package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/headless/cli"
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

// TestUsageHeaderNamesTheTool is a smoke check that the header copy is
// present and describes the product, not just Go's bare flag dump.
func TestUsageHeaderNamesTheTool(t *testing.T) {
	if !strings.Contains(usageHeader, "sf-deck") ||
		!strings.Contains(usageHeader, "Salesforce") {
		t.Error("usage header should name sf-deck and Salesforce")
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
