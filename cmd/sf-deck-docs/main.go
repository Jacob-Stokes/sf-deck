// Command sf-deck-docs regenerates the auto-generated reference
// pages of the docs site from the verb + keymap registries:
//
//	docs-site/docs/reference/cli.md
//	docs-site/docs/reference/ipc.md
//	docs-site/docs/reference/keymap.md
//
// Run from the repo root:
//
//	go run ./cmd/sf-deck-docs
//
// CI runs `go run ./cmd/sf-deck-docs -check` to fail when the docs
// would change relative to the current registry state.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/keymap"
	"github.com/Jacob-Stokes/sf-deck/internal/verbs"
)

const (
	cliPath    = "docs-site/docs/reference/cli.md"
	ipcPath    = "docs-site/docs/reference/ipc.md"
	keymapPath = "docs-site/docs/reference/keymap.md"
)

func main() {
	check := flag.Bool("check", false, "exit non-zero if docs would change (CI use)")
	flag.Parse()

	files := map[string]string{
		cliPath:    renderCLI(),
		ipcPath:    renderIPC(),
		keymapPath: renderKeymap(),
	}

	changed := false
	for path, content := range files {
		didChange, err := writeIfDifferent(path, content, *check)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if didChange {
			changed = true
			if !*check {
				fmt.Println("wrote", path)
			}
		}
	}

	if *check && changed {
		fmt.Fprintln(os.Stderr, "docs out of date — re-run `go run ./cmd/sf-deck-docs`")
		os.Exit(1)
	}
	if !*check && !changed {
		fmt.Println("docs already up to date.")
	}
}

func writeIfDifferent(path, content string, check bool) (bool, error) {
	root, err := repoRoot()
	if err != nil {
		return false, err
	}
	full := filepath.Join(root, path)
	existing, err := os.ReadFile(full)
	if err == nil && string(existing) == content {
		return false, nil
	}
	if check {
		return true, nil
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(full, []byte(content), 0o644)
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		dir = filepath.Dir(dir)
	}
	return "", fmt.Errorf("go.mod not found")
}

// ----- CLI reference -----------------------------------------------

func renderCLI() string {
	var b strings.Builder
	b.WriteString(`# CLI reference

Every sf-deck CLI noun and verb. Auto-generated from
` + "`internal/verbs/registry.go`" + `. Re-run

    go run ./cmd/sf-deck-docs

after editing the registry.

Pass ` + "`--json`" + ` on every command. The JSON envelope is the
stable contract; text mode is for humans.

`)
	groups := groupSpecsByNoun(verbs.SpecsForSurface(verbs.SurfaceCLI))
	for _, noun := range groups.nouns {
		fmt.Fprintf(&b, "## `%s`\n\n", noun)
		for _, s := range groups.byNoun[noun] {
			fmt.Fprintf(&b, "### `%s`\n\n", s.Qualified())
			fmt.Fprintf(&b, "%s\n\n", s.Summary)
			if s.Safety != "" {
				fmt.Fprintf(&b, "- Safety gate: `%s`\n", s.Safety)
			}
			if s.Stability != "" && s.Stability != "stable" {
				fmt.Fprintf(&b, "- Stability: `%s`\n", s.Stability)
			}
			if s.CLI != nil && s.CLI.Usage != "" {
				fmt.Fprintf(&b, "- Usage: `%s`\n", s.CLI.Usage)
			}
			if s.IPC != nil {
				fmt.Fprintf(&b, "- IPC equivalent: `%s`\n", s.IPC.Command)
			}
			if s.Notes != "" {
				fmt.Fprintf(&b, "- Note: %s\n", s.Notes)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

// ----- IPC reference -----------------------------------------------

func renderIPC() string {
	var b strings.Builder
	b.WriteString(`# IPC reference

Every sf-deck IPC command. Auto-generated from
` + "`internal/verbs/registry.go`" + `.

IPC reaches a running sf-deck instance over a Unix socket. Find the
socket via ` + "`sf-deck instance list --json`" + `; it's at
` + "`~/.sf-deck/control-<N>.sock`" + `.

Wire format is one JSON object per line. Request:

    {"command": "noun.verb", "args": {...}}

Response (success):

    {"ok": true, "command": "noun.verb", "data": {...}, "changed": true}

Response (failure):

    {"ok": false, "command": "noun.verb", "error": {"code": "...", "message": "..."}}

`)
	groups := groupSpecsByNoun(verbs.SpecsForSurface(verbs.SurfaceIPC))
	for _, noun := range groups.nouns {
		fmt.Fprintf(&b, "## `%s.*`\n\n", noun)
		for _, s := range groups.byNoun[noun] {
			fmt.Fprintf(&b, "### `%s`\n\n", s.IPC.Command)
			fmt.Fprintf(&b, "%s\n\n", s.Summary)
			if s.IPC.Async {
				b.WriteString("- Async: agent polls via the matching report verb\n")
			}
			if s.Safety != "" {
				fmt.Fprintf(&b, "- Safety gate: `%s`\n", s.Safety)
			}
			if len(s.IPC.Args) > 0 {
				b.WriteString("- Arguments:\n\n")
				b.WriteString("    | Name | Type | Required | Description |\n")
				b.WriteString("    |---|---|---|---|\n")
				for _, a := range s.IPC.Args {
					req := ""
					if a.Required {
						req = "yes"
					} else {
						req = "no"
					}
					desc := a.Description
					if desc == "" {
						desc = "—"
					}
					fmt.Fprintf(&b, "    | `%s` | `%s` | %s | %s |\n",
						a.Name, a.Type, req, desc)
				}
				b.WriteString("\n")
			}
			if s.CLI != nil && s.CLI.Usage != "" {
				fmt.Fprintf(&b, "- CLI equivalent: `%s`\n", s.CLI.Usage)
			}
			if s.Notes != "" {
				fmt.Fprintf(&b, "- Note: %s\n", s.Notes)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

// ----- Keymap reference --------------------------------------------

func renderKeymap() string {
	var b strings.Builder
	b.WriteString(`# Keymap reference

Every sf-deck key binding. Auto-generated from
` + "`internal/ui/keymap/commands.go`" + `.

Press ` + "`?`" + ` on any screen for the live keymap (context-
sensitive — shows only what works here).

To override any binding, run ` + "`sf-deck --dump-keymap > ~/.sf-deck/keybindings.toml`" + ` and
edit the file.

`)
	groups := groupKeymapByCategory(keymap.Commands)
	for _, cat := range groups.cats {
		fmt.Fprintf(&b, "## %s\n\n", cat)
		b.WriteString("| Action | Default key(s) | Where |\n")
		b.WriteString("|---|---|---|\n")
		for _, c := range groups.byCat[cat] {
			keys := strings.Join(c.Default, ", ")
			if keys == "" {
				keys = "—"
			}
			fmt.Fprintf(&b, "| %s | `%s` | %s |\n", c.Label, keys, c.When)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ----- shared --------------------------------------------------

type nounIndex struct {
	nouns  []string
	byNoun map[string][]verbs.Spec
}

func groupSpecsByNoun(specs []verbs.Spec) nounIndex {
	out := nounIndex{byNoun: map[string][]verbs.Spec{}}
	for _, s := range specs {
		if _, ok := out.byNoun[s.Noun]; !ok {
			out.nouns = append(out.nouns, s.Noun)
		}
		out.byNoun[s.Noun] = append(out.byNoun[s.Noun], s)
	}
	sort.Strings(out.nouns)
	for n := range out.byNoun {
		sort.Slice(out.byNoun[n], func(i, j int) bool {
			return out.byNoun[n][i].Verb < out.byNoun[n][j].Verb
		})
	}
	return out
}

type catIndex struct {
	cats  []string
	byCat map[string][]keymap.Command
}

func groupKeymapByCategory(cmds []keymap.Command) catIndex {
	out := catIndex{byCat: map[string][]keymap.Command{}}
	for _, c := range cmds {
		cat := c.Category
		if cat == "" {
			cat = "Other"
		}
		if _, ok := out.byCat[cat]; !ok {
			out.cats = append(out.cats, cat)
		}
		out.byCat[cat] = append(out.byCat[cat], c)
	}
	sort.Strings(out.cats)
	for c := range out.byCat {
		sort.Slice(out.byCat[c], func(i, j int) bool {
			return out.byCat[c][i].Label < out.byCat[c][j].Label
		})
	}
	return out
}
