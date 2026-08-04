package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/instance"
)

// dispatchInstance routes `sf-deck instance <verb>`. Today only `list`
// — the discovery entry point for agents using the controller skill.
//
// We don't use *app.App for anything here (registry reads happen
// against the filesystem, not the org cache) but we keep the
// signature symmetric with the other dispatchers for consistency.
func dispatchInstance(a *app.App, args Args, stdout io.Writer, mode headless.WriteMode) int {
	_ = a
	verb := args.Verb
	rest := args.Rest
	switch verb {
	case "list":
		return instanceList(rest, stdout, mode)
	}
	r := headless.Fail("instance."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown verb %q (expected list)", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func instanceList(rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("instance list")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("instance.list", err, stdout, mode)
	}
	f, err := instance.Read()
	if err != nil {
		return writeInstanceErr("instance.list", err, stdout, mode)
	}
	// Surface as a simple array of records — agents iterate the
	// entries to find the instance they want by number or label.
	type row struct {
		Number    int    `json:"number"`
		PID       int    `json:"pid"`
		StartedAt string `json:"started_at"`
		Socket    string `json:"socket,omitempty"`
		Label     string `json:"label,omitempty"`
	}
	out := make([]row, 0, len(f.Entries))
	for _, e := range f.Entries {
		out = append(out, row{
			Number: e.Number, PID: e.PID, StartedAt: e.StartedAt,
			Socket: e.Socket, Label: e.Label,
		})
	}
	path, _ := instance.Path()
	r := headless.Success("instance.list", "", "", false, map[string]any{
		"instances":     out,
		"count":         len(out),
		"registry_path": path,
	})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func writeInstanceErr(command string, err error, stdout io.Writer, mode headless.WriteMode) int {
	if errors.Is(err, errors.New("")) {
		// placeholder for future typed errors; falls through to generic.
	}
	return writeArgErr(command, err, stdout, mode)
}
