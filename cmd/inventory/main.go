package main

// cmd/inventory — emits docs/development/surfaces.md by walking sf-deck's
// tabRegistry. Run from repo root:
//
//	go run ./cmd/inventory
//
// Pass -qa for docs/development/qa-checklist.md or -check to fail when the
// checked-in output is stale.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Jacob-Stokes/sf-deck/internal/ui"
)

func main() {
	qa := flag.Bool("qa", false, "emit the generated QA checklist instead of the surface inventory")
	check := flag.Bool("check", false, "exit non-zero if the checked-in output is stale")
	flag.Parse()
	out := "docs/development/surfaces.md"
	build := ui.BuildSurfaceInventoryMarkdown
	if *qa {
		out = "docs/development/qa-checklist.md"
		build = ui.BuildQAChecklistMarkdown
	}
	if v := os.Getenv("SF_DECK_INVENTORY_OUT"); v != "" {
		out = v
	}
	md := build()
	existing, err := os.ReadFile(out)
	if err == nil && string(existing) == md {
		if !*check {
			fmt.Printf("%s already up to date\n", out)
		}
		return
	}
	if *check {
		fmt.Fprintf(os.Stderr, "%s is stale; rerun go run ./cmd/inventory", out)
		if *qa {
			fmt.Fprint(os.Stderr, " -qa")
		}
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "inventory: mkdir: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, []byte(md), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "inventory: write: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d bytes)\n", out, len(md))
}
