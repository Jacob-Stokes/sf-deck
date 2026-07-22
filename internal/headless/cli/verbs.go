package cli

// verbs CLI noun — introspection over the registry. Agents
// (and humans) call `sf-deck verbs list --json` to discover what
// nouns and verbs exist, what's on CLI vs IPC, and what safety
// gate each one carries.

import (
	"errors"
	"fmt"
	"io"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/verbs"
)

func dispatchVerbs(_ *app.App, args Args, stdout io.Writer, mode headless.WriteMode) int {
	verb := args.Verb
	if verb == "" {
		verb = "list"
	}
	switch verb {
	case "list":
		return verbsList(args.Rest, stdout, mode)
	}
	r := headless.Fail("verbs."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown verbs verb %q", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func verbsList(rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("verbs list")
	surface := fs.String("surface", "",
		"filter to one surface: cli, ipc, tui (empty = all)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("verbs.list", err, stdout, mode)
	}
	var specs []verbs.Spec
	switch *surface {
	case "":
		specs = verbs.Specs()
	case "cli":
		specs = verbs.SpecsForSurface(verbs.SurfaceCLI)
	case "ipc":
		specs = verbs.SpecsForSurface(verbs.SurfaceIPC)
	case "tui":
		specs = verbs.SpecsForSurface(verbs.SurfaceTUI)
	default:
		return writeArgErr("verbs.list",
			errors.New("--surface must be cli, ipc, tui, or empty"), stdout, mode)
	}
	r := headless.Success("verbs.list", "", "", false,
		map[string]any{
			"verbs":   verbsToJSON(specs),
			"count":   len(specs),
			"surface": *surface,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// verbsToJSON projects Specs into a JSON-friendly shape. Bindings
// become nested objects when non-nil, omitted when nil — so JSON
// consumers can ask "is X on IPC?" by checking for key presence.
func verbsToJSON(specs []verbs.Spec) []map[string]any {
	out := make([]map[string]any, 0, len(specs))
	for _, s := range specs {
		entry := map[string]any{
			"noun":      s.Noun,
			"verb":      s.Verb,
			"qualified": s.Qualified(),
			"summary":   s.Summary,
			"stability": s.Stability,
		}
		if s.Safety != "" {
			entry["safety"] = string(s.Safety)
		}
		if s.Notes != "" {
			entry["notes"] = s.Notes
		}
		if s.TUIOnly {
			entry["tui_only"] = true
		}
		if s.CLI != nil {
			entry["cli"] = map[string]any{
				"usage":    s.CLI.Usage,
				"flags":    flagsToJSON(s.CLI.Flags),
				"examples": s.CLI.Examples,
			}
		}
		if s.IPC != nil {
			entry["ipc"] = map[string]any{
				"command":  s.IPC.Command,
				"args":     fieldsToJSON(s.IPC.Args),
				"examples": s.IPC.Examples,
				"async":    s.IPC.Async,
			}
		}
		out = append(out, entry)
	}
	return out
}

func flagsToJSON(fs []verbs.FlagSpec) []map[string]any {
	out := make([]map[string]any, 0, len(fs))
	for _, f := range fs {
		out = append(out, map[string]any{
			"name":        f.Name,
			"type":        f.Type,
			"required":    f.Required,
			"description": f.Description,
		})
	}
	return out
}

func fieldsToJSON(fs []verbs.FieldSpec) []map[string]any {
	out := make([]map[string]any, 0, len(fs))
	for _, f := range fs {
		out = append(out, map[string]any{
			"name":        f.Name,
			"type":        f.Type,
			"required":    f.Required,
			"description": f.Description,
		})
	}
	return out
}
