package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/buildinfo"
	"github.com/Jacob-Stokes/sf-deck/internal/control"
	"github.com/Jacob-Stokes/sf-deck/internal/headless/cli"
	"github.com/Jacob-Stokes/sf-deck/internal/instance"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/resource"
	"github.com/Jacob-Stokes/sf-deck/internal/updatecheck"
	"github.com/Jacob-Stokes/sf-deck/internal/usage"
)

// version, commit, and date are stamped at build time by goreleaser
// via -ldflags. A bare `go build` leaves them at "dev" so a
// developer running locally still gets a reasonable `--version`
// instead of empty strings.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// trackerAdapter bridges *usage.Tracker to ui.UsageTracker. The ui
// package can't import usage (its UsageCall type would create a
// cycle), so we re-shape Recent() here.
type trackerAdapter struct{ t *usage.Tracker }

func (a trackerAdapter) Today() int                   { return a.t.Today() }
func (a trackerAdapter) TodayForOrg(alias string) int { return a.t.TodayForOrg(alias) }
func (a trackerAdapter) TodayForOrgKeys(aliases ...string) int {
	return a.t.TodayForOrgKeys(aliases...)
}
func (a trackerAdapter) Recent() []ui.UsageCall {
	src := a.t.Recent()
	out := make([]ui.UsageCall, len(src))
	for i, c := range src {
		out[i] = ui.UsageCall{
			At: c.At, Alias: c.Alias, Command: c.Command,
			Args: c.Args, OK: c.OK, Err: c.Err, Caller: c.Caller,
		}
	}
	return out
}

func main() {
	buildinfo.Set(version, commit, date)

	// Headless dispatch wins when the first positional arg is a
	// known noun. Detect this BEFORE flag.Parse so a noun like
	// "chip" doesn't get eaten by the global FlagSet, and so
	// headless commands can run with their own (subcommand-local)
	// flag parsing rather than the TUI's `-dump-keymap`.
	args := cli.Parse(os.Args[1:])
	if args.IsHeadless() {
		runHeadless(args)
		return
	}

	// Opt-in pprof server for leak-hunting a live TUI. Off unless
	// SF_DECK_PPROF is set to a listen address (e.g. "localhost:6060").
	// Zero cost otherwise — nothing imported into the hot path, just a
	// background HTTP listener you can point `go tool pprof` at while
	// the TUI runs. Never enabled in normal use.
	startPprofFromEnv()

	flag.Usage = printUsage
	dumpKeymap := flag.Bool("dump-keymap", false,
		"Print the effective keybindings as TOML and exit. "+
			"Redirect to ~/.sf-deck/keybindings.toml to customize.")
	demo := flag.Bool("demo", false,
		"Run against fictional demo orgs — no Salesforce connection, "+
			"auth, or sf CLI required. Nothing real is read or written.")
	noCache := flag.Bool("no-cache", false,
		"Run against real orgs with an ephemeral data cache: start "+
			"cold (fresh fetches), write nothing to ~/.sf-deck/cache.db. "+
			"Settings, dev projects, and tags are unaffected.")
	noSettings := flag.Bool("no-settings", false,
		"Ignore ~/.sf-deck/settings.toml: run with built-in defaults "+
			"and persist nothing. Data cache, dev projects, and tags "+
			"are unaffected.")
	noTags := flag.Bool("no-tags", false,
		"Run against real orgs with an ephemeral dev-project / tag "+
			"store: write nothing to ~/.sf-deck/devprojects.db (tags, "+
			"dev projects, saved queries + apex are session-only).")
	enableControl := flag.Bool("control", false,
		"Open a Unix-domain control socket at ~/.sf-deck/control-<N>.sock "+
			"so other processes can drive this instance. Off by default.")
	controlLabel := flag.String("label", "",
		"Optional human label for this instance (shown in instance list). "+
			"Mainly useful when multiple sf-deck windows are running.")
	showVersion := flag.Bool("version", false,
		"Print the version and exit.")
	flag.Parse()

	if *showVersion {
		fmt.Printf("sf-deck %s\ncommit:  %s\nbuilt:   %s\n", version, commit, date)
		return
	}
	if os.Getenv("SF_DECK_CONTROL") == "1" {
		*enableControl = true
	}

	// Teach the resource layer to treat a per-org demo short-circuit
	// (sf.ErrDemoTarget) as benign — an injected demo org serves seeded
	// cache and must not surface a fetch error. Wired unconditionally
	// (not just under --demo) because a persisted demo org can coexist
	// with real orgs in a normal launch.
	resource.BenignFetchErr = sf.IsDemoTargetErr

	if *demo {
		// Flip every layer's demo switch BEFORE anything loads or
		// shells out: settings become in-memory only, resources stop
		// going stale and serve the seed forever, and the sf package
		// refuses live calls outright (the backstop guarantee that a
		// demo can never touch a real org).
		settings.Ephemeral = true
		resource.DemoMode = true
		sf.DemoMode = true
		ui.Demo = true
	}

	// --no-settings: run with built-in defaults, persist nothing.
	// Same in-memory settings switch demo uses, without the rest of
	// demo mode (real orgs, real cache/tags).
	if *noSettings {
		settings.Ephemeral = true
	}

	// Load user-defined keybindings before the TUI starts. Keymap is
	// a TUI-only concern; the headless app.Open below doesn't touch
	// it.
	km, warn := ui.LoadKeymap()
	ui.Keys = km

	if *dumpKeymap {
		fmt.Print(ui.Keys.DumpTOML())
		return
	}

	// SkipOrgs: the TUI's orgsRes Resource enumerates orgs lazily on
	// Init() with its own cache layer.  Letting app.Open shell out to
	// `sf` synchronously here just blocked first paint for ~1.5s.
	//
	// IPC bundle handlers also need ResolveOrg to work from request 1
	// — but the resolver falls through to the orgs cache when a.Orgs
	// is empty, so we don't need to pay the startup tax even when
	// --control is on. First launch ever with no cache → the
	// resolver surfaces a "run sf org list" error to the agent.
	a, err := app.Open(app.OpenOptions{SkipOrgs: true, Demo: *demo, NoCache: *noCache, NoTags: *noTags})
	if err != nil {
		fmt.Fprintln(os.Stderr, "startup:", err)
		os.Exit(1)
	}
	defer a.Close()

	if *demo {
		// Seed the throwaway cache with the fictional Northwind world.
		// From here the normal cache-first data path does the rest.
		if err := ui.SeedDemoCache(a.Cache); err != nil {
			fmt.Fprintln(os.Stderr, "demo seed:", err)
			os.Exit(1)
		}
		// Seed the devprojects store (projects, items, bundles, tags,
		// saved queries, apex snippets, soql history) so the modern
		// /dev-projects, /bundles, /tags, /soql saved+history surfaces
		// all have data on a fresh demo launch.
		if err := ui.SeedDemoDevProjects(a.Projects, a.DemoDir()); err != nil {
			fmt.Fprintln(os.Stderr, "demo seed (devprojects):", err)
			os.Exit(1)
		}
	}

	if a.Usage != nil {
		ui.Usage = trackerAdapter{t: a.Usage}
	}

	model := ui.New(a.Cache).WithWriteServices(ui.WriteServices{
		Apex: a.ApexWrites(), Records: a.RecordWrites(), Bundles: a.BundleWrites(),
		Permissions: a.PermissionWrites(),
		Metadata:    a.MetadataWrites(), MetadataEditors: a.MetadataEditorWrites(),
		Users: a.UserWrites(),
	}).WithUpdateChecker(a.Updates)
	if a.Projects != nil {
		model = model.WithDevProjects(a.Projects)
	}
	if warn != "" {
		// Surface the keymap parse warning so the user knows their
		// config file was ignored.
		model = model.WithStartupWarning(warn)
	}
	for _, w := range a.StartupWarnings {
		// App-level non-fatal problems (e.g. devprojects.db restored
		// from backup) — the user must SEE data-loss warnings, not
		// find them in the session log.
		model = model.WithStartupWarning(w)
	}

	// Claim an instance slot. We always claim — even without --control
	// — so the top-left badge can show which window is which. The
	// listener starts only when --control is set, in which case the
	// Listen() call re-claims with the real socket path.
	instanceNumber, controlState, controlServer := setupInstance(*enableControl, *controlLabel, a)
	if controlServer != nil {
		defer controlServer.Close()
	} else {
		// Even without the listener, release on clean shutdown so the
		// slot frees up for the next window.
		defer instance.Release(os.Getpid())
	}
	model.AttachControl(controlState, instanceNumber)

	// Alt screen is set on the View return value now (bubbletea v2);
	// no program-level option for it anymore. See internal/ui/render.go.
	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// setupInstance claims a registry slot for this sf-deck process and
// (when enableControl is true) starts the IPC listener bound to a
// per-instance socket. Returns the slot number, the live ControlState
// the Model talks to, and the listener server (nil when disabled)
// so main can defer its Close().
//
// On any failure (registry unwritable, socket bind error) we degrade
// gracefully — the TUI still starts with badge fallback "1" and a
// stderr warning, rather than refusing to launch over an IPC hiccup.
func setupInstance(enableControl bool, label string, a *app.App) (int, *ui.ControlState, *control.Server) {
	if !enableControl {
		entry, err := instance.Claim(os.Getpid(), "", label)
		if err != nil {
			fmt.Fprintln(os.Stderr, "warning: instance registry:", err)
			return 1, nil, nil
		}
		return entry.Number, nil, nil
	}
	// Bundle / project / data-plane IPC handlers need the
	// devprojects store + an org resolver + safety reader +
	// settings + a save callback. All live on app.App, so the
	// listener gets the same surface area the headless CLI does.
	cs := ui.NewControlState(
		a.Projects,
		a.ResolveOrg,
		a.SafetyFor,
		a.Settings,
		a.SaveSettings,
		ui.ControlServices{
			Metadata: a.MetadataWrites(), Apex: a.ApexWrites(), Records: a.RecordWrites(),
			Bundles: a.BundleWrites(),
		},
	)
	srv := &control.Server{Backend: cs, Label: label}
	entry, err := srv.Listen(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: control listener:", err)
		return 1, cs, nil
	}
	return entry.Number, cs, srv
}

// printUsage is the `--help` / bad-flag output for the TUI entry point.
// Go's default flag usage lists only the two flags and never mentions
// the headless commands, so a terminal user has no way to discover them.
func printUsage() {
	w := flag.CommandLine.Output()
	fmt.Fprint(w, usageHeader)
	fmt.Fprint(w, usageCommands)
}

const usageHeader = `sf-deck — a terminal UI for working across your Salesforce orgs.

Usage:
  sf-deck [flags]              Launch the interactive TUI
  sf-deck <command> [args]     Run a headless command (JSON-friendly)

Run with no arguments to open the TUI against the orgs that
` + "`sf org list`" + ` already knows. Press ? inside for keybindings.

Flags:
  -demo          Launch against a built-in fictional org — no auth
                 needed. The fastest way to try sf-deck.
  -no-cache      Real orgs, ephemeral cache: start cold and write
                 nothing to ~/.sf-deck/cache.db (settings, projects,
                 and tags are kept). Good for a fresh-eyes session.
  -no-settings   Ignore settings.toml — run with built-in defaults
                 and persist nothing.
  -no-tags       Real orgs, ephemeral dev-project / tag store: write
                 nothing to ~/.sf-deck/devprojects.db.
  -version       Print version, commit, and build date, then exit.
  -dump-keymap   Print the effective keybindings as TOML and exit.
                 Redirect to ~/.sf-deck/keybindings.toml to customize.
  -control       Start the local IPC control socket (for agents /
                 scripting). Off by default.
  -label <name>  Human label for this instance in the control socket
                 registry. Only meaningful with -control.
`

const usageCommands = `
Headless commands (add --help to any for its flags):
  org           List orgs, show details, get/set safety level
  object        List/describe sObjects and their fields
  record        Get, update, and list recently viewed records
  soql          Run and export SOQL queries; manage saved queries
  report        List, run, and export reports
  apex          Execute anonymous Apex; manage saved snippets
  metadata      Get, create, update, and delete metadata
  notification  List and mark notifications read
  chip          Manage saved list-view chips
  tag           Manage tags and tagged items
  project       Manage dev projects and their items
  bundle        Create and operate sfdx-project bundles linked to dev projects
  instance      List the sf-deck instances currently running
  update        Check for a newer stable sf-deck release
  verbs         Discover the full noun.verb surface (single source of truth)

Add --json to any command for a stable, scriptable envelope.

Docs:  https://github.com/Jacob-Stokes/sf-deck
`

// runHeadless boots app context with headless-friendly options
// (skip usage tracker + applog for fast, quiet startup) and
// dispatches the command. Exits with the response's typed exit code.
//
// DevProjects + Orgs are opened here:
//   - DevProjects (tags / projects / saved queries / saved apex)
//     is cheap (sqlite header check + schema apply, <10ms)
//   - Orgs requires shelling out to `sf` which is slower (~200ms+)
//     but the org-targeted verbs need it AND every safety check
//     needs to know the org's kind.
func runHeadless(args cli.Args) {
	if args.Noun == "update" {
		// Release discovery needs neither Salesforce nor any local database,
		// so keep it usable on a fresh machine before the sf CLI is installed.
		a := &app.App{Updates: updatecheck.New()}
		os.Exit(cli.Dispatch(a, args, os.Stdout, os.Stderr))
	}
	a, err := app.Open(app.OpenOptions{
		SkipUsage:  true,
		SkipApplog: true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "startup:", err)
		os.Exit(1)
	}
	defer a.Close()
	os.Exit(cli.Dispatch(a, args, os.Stdout, os.Stderr))
}
