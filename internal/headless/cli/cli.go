// Package cli is the argument parser + dispatcher for sf-deck's
// headless commands.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

// Args is the parsed top-level invocation. CLI consumers (cmd/sf-deck)
// call Parse on os.Args[1:]; Parse handles --json + the noun/verb
// detection but doesn't touch noun-specific flags (each subcommand
// owns its own FlagSet, fed from Args.Rest).
type Args struct {
	JSON bool

	Noun string

	Verb string

	Rest []string
}

// IsHeadless reports whether the parsed args actually invoke a
// headless command. cmd/sf-deck uses this to decide whether to dispatch
// or fall through to the TUI.
func (a Args) IsHeadless() bool {
	return KnownNouns[a.Noun]
}

// KnownNouns is the registry of valid first-positional commands. The
// TUI fall-through path checks this BEFORE the flag parser runs, so a
// user can still pass `--dump-keymap` without it being misread as a
// noun.
//
// New nouns are added here as services come online. Keep alphabetised
// so the help text stays stable.
var KnownNouns = map[string]bool{
	"chip":         true,
	"tag":          true,
	"project":      true,
	"org":          true,
	"soql":         true,
	"object":       true,
	"record":       true,
	"report":       true,
	"notification": true,
	"apex":         true,
	"metadata":     true,
	"instance":     true,
	"bundle":       true,
	"data":         true,
	"legal":        true,
	"verbs":        true,
	"update":       true,
}

// Parse splits argv into the top-level shape. It's lenient: unknown
// flags are passed through into Rest so per-noun parsers can handle
// them. --json is global and may appear before or after the noun.
func Parse(argv []string) Args {
	out := Args{}
	filtered := make([]string, 0, len(argv))
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		if a == "--" {
			filtered = append(filtered, argv[i:]...)
			break
		}
		if a == "--json" {
			out.JSON = true
			continue
		}
		filtered = append(filtered, a)
	}
	argv = filtered
	i := 0
	for ; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--":
			i++
			goto done
		case strings.HasPrefix(a, "-"):
			goto done
		default:
			out.Noun = a
			i++
			goto consumeVerb
		}
	}
done:
	out.Rest = append(out.Rest, argv[i:]...)
	return out

consumeVerb:
	// If the next token is positional, it's the verb. Otherwise
	// (flag or end-of-args), there's no verb.
	if i < len(argv) && !strings.HasPrefix(argv[i], "-") {
		out.Verb = argv[i]
		i++
	}
	out.Rest = append(out.Rest, argv[i:]...)
	return out
}

// Dispatch runs a parsed headless command and returns the process
// exit code. stdout / stderr are wired into headless.Response so
// tests can capture output without spawning sub-processes.
//
// The *app.App is the shared startup context (settings, cache, orgs).
// Caller is responsible for app.Open + defer a.Close; we just consume
// the references.
//
// On unknown noun/verb, Dispatch writes a "invalid_argument" response
// and returns the matching exit code rather than panicking — keeps
// the wire shape uniform even for malformed input.
func Dispatch(a *app.App, args Args, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	mode := headless.TextMode
	if args.JSON {
		mode = headless.JSONMode
	}

	switch args.Noun {
	case "chip":
		return dispatchChip(a, args, stdout, mode)
	case "tag":
		return dispatchTag(a, args, stdout, mode)
	case "project":
		return dispatchProject(a, args, stdout, mode)
	case "org":
		return dispatchOrg(a, args, stdout, mode)
	case "soql":
		return dispatchSOQL(a, args, stdout, mode)
	case "object":
		return dispatchObject(a, args, stdout, mode)
	case "record":
		return dispatchRecord(a, args, stdout, mode)
	case "report":
		return dispatchReport(a, args, stdout, mode)
	case "notification":
		return dispatchNotification(a, args, stdout, mode)
	case "apex":
		return dispatchApex(a, args, stdout, mode)
	case "metadata":
		return dispatchMetadata(a, args, stdout, mode)
	case "instance":
		return dispatchInstance(a, args, stdout, mode)
	case "bundle":
		return dispatchBundle(a, args, stdout, mode)
	case "data":
		return dispatchData(args, stdout, mode)
	case "legal":
		return dispatchLegal(args, stdout, mode)
	case "verbs":
		return dispatchVerbs(a, args, stdout, mode)
	case "update":
		return dispatchUpdate(a, args, stdout, mode)
	}

	r := headless.Fail("", "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown command %q", args.Noun), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func writeSafetyBlocked(command, orgUser string, be app.BlockedError, stdout io.Writer, mode headless.WriteMode) int {
	r := headless.Fail(command, orgUser, headless.ErrSafetyBlocked,
		be.Error(),
		map[string]any{
			"required_write_kind": writeKindString(be.Required),
			"effective_safety":    be.Actual.String(),
			"target":              be.Target,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func writeKindString(k settings.WriteKind) string {
	switch k {
	case settings.WriteRecord:
		return "records"
	case settings.WriteMetadata:
		return "metadata"
	case settings.WriteAnonymous:
		return "full"
	}
	return "unknown"
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard) // we render our own errors
	return fs
}
