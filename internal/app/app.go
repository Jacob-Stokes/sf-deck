// Package app holds the shared startup + lifecycle context for every
// sf-deck entry point — the TUI today, and the headless CLI / agent
// API surfaces planned in docs/headless-mode-plan.md.
//
// What lives here:
//
//   - App: a typed bag of the long-lived services every entry point
//     needs (settings, cache, devproject store, usage tracker, orgs).
//   - Open: one function that loads + opens everything in the right
//     order, returning an *App ready to use. Replaces the inline
//     setup that used to live at the top of cmd/sf-deck/main.go.
//   - Close: idempotent cleanup, safe to defer.
//   - ResolveOrg / SafetyFor / CanWrite / TargetArg: helpers every
//     command (TUI gesture or headless verb) calls before mutating an
//     org. Sharing them between TUI and headless is the whole point.
//
// What does NOT live here:
//
//   - Presentation (TUI rendering / Bubble Tea state). That stays in
//     internal/ui.
//   - Business logic per domain (chips, projects, records, …). Each
//     domain gets its own internal/services/<X> package in later
//     phases of the headless plan. App is the substrate they share,
//     not the home for their logic.
package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/services/apexops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/bundles"
	"github.com/Jacob-Stokes/sf-deck/internal/services/metadataops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/notificationops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/services/permissionops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/records"
	"github.com/Jacob-Stokes/sf-deck/internal/services/userops"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/usage"
)

// App is the shared runtime context. Construct via Open; Close when
// done. Every long-lived service the TUI or a headless command might
// reach for is parked here as a typed field, so call sites stay
// readable and the wiring graph is one struct deep.
//
// Zero value is NOT useful — use Open. Each field can be nil if its
// underlying open failed (cache always succeeds in practice, but
// usage tracker / devproject store degrade gracefully); callers
// check before using.
type App struct {
	// Settings carries the on-disk TOML configuration. Always non-nil
	// after Open — failures here are fatal because the TUI / CLI
	// can't meaningfully run without per-user preferences.
	Settings *settings.Settings

	// Cache is the SQLite key-value cache backing Resource[T]. Always
	// non-nil after Open.
	Cache *cache.Cache

	// Projects is the dev-project / saved-query / saved-apex / tag
	// store. May be nil if the open failed (rare — bad disk perms).
	// Callers fall back to "feature unavailable" rather than crashing.
	Projects *devproject.Store

	// StartupWarnings collects non-fatal startup problems the UI
	// should surface on first paint (e.g. devprojects.db recovered
	// from backup after corruption). Empty on a clean start.
	StartupWarnings []string

	// Usage is the API-call tracker. May be nil when the tracker
	// failed to open or when the caller opted out via opts.SkipUsage
	// (headless commands that don't need it).
	Usage *usage.Tracker

	// Orgs is the resolved list of authenticated orgs. Populated by
	// Open via sf.ListOrgs; nil/empty when no orgs are connected.
	Orgs []sf.Org

	// SaveSettings is the persistence function service callers pass
	// to write-paths. Defaults to Settings.Save; tests override with
	// a no-op so they don't touch the real settings.toml. Keeping the
	// indirection on the App (rather than per-service-call) means
	// every command goes through the same hook.
	SaveSettings func() error

	// WriteGate is the shared org-resolution + safety policy used by
	// Salesforce-mutating services. App.CanWrite delegates to it during the
	// adapter migration so existing CLI/TUI behavior remains compatible.
	WriteGate *orgwrite.Gate

	// Metadata is the safety-enforced Tooling metadata write service shared
	// by CLI and IPC adapters (and, incrementally, the TUI).
	Metadata        *metadataops.Service
	MetadataEditors *metadataops.EditorService

	// Apex is the safety-enforced anonymous execution service shared by all
	// product surfaces.
	Apex *apexops.Service

	// Records is the safety-enforced record mutation service shared by all
	// product surfaces.
	Records *records.Service

	// Bundles gates bundle retrieve/validate/deploy/report against the exact
	// effective bundle target.
	Bundles *bundles.Service

	// Permissions owns TUI permission-set, object-permission, and FLS writes.
	Permissions *permissionops.Service

	// Users owns full-safety user administration operations.
	Users *userops.Service

	// Notifications owns per-user notification read-state writes.
	Notifications *notificationops.Service

	// demoDir is the throwaway temp dir holding the demo cache +
	// devproject store when opened with OpenOptions.Demo. Removed on
	// Close. Empty outside demo mode.
	demoDir string

	// cacheDir is the throwaway temp dir holding the ephemeral cache
	// when opened with OpenOptions.NoCache. Removed on Close. Empty
	// otherwise.
	cacheDir string

	// tagsDir is a dedicated throwaway temp dir for the ephemeral
	// dev-project / tag store (OpenOptions.NoTags) when NoCache didn't
	// already provide one to reuse. Removed on Close. Empty otherwise.
	tagsDir string
}

// OpenOptions tunes Open's behaviour for different entry points.
// Zero value is sane defaults for the TUI; headless commands set
// flags to skip heavy / interactive setup they don't need.
type OpenOptions struct {
	// SkipUsage disables the api-call tracker. Headless commands
	// that don't talk to Salesforce shouldn't open the usage db.
	SkipUsage bool

	// SkipDevProjects disables the dev-project / saved store. The
	// TUI always wants it; some narrow headless commands (org list,
	// safety inspection) don't.
	SkipDevProjects bool

	// SkipOrgs disables org enumeration. Useful for commands that do
	// not resolve or contact an org and do not want to pay ListOrgs
	// latency on startup.
	SkipOrgs bool

	// SkipApplog skips the per-session structured log. The TUI
	// always opens it; tests / short-lived CLI invocations may opt
	// out.
	SkipApplog bool

	// Demo boots against an entirely fictional world for
	// `sf-deck --demo`: the cache and devproject store land in a
	// throwaway temp dir (removed on Close), the usage tracker and
	// org enumeration are skipped, and settings are expected to be
	// ephemeral (the caller sets settings.Ephemeral before Open so
	// the user's real settings.toml is never read or written). The
	// caller seeds the demo cache and sets the sf/resource/ui demo
	// flags — Open only provides the isolated stores.
	Demo bool

	// NoCache runs against a REAL org but with an EPHEMERAL data
	// cache: the cache.db lives in a throwaway temp dir (removed on
	// Close), so the session starts cold (fresh fetches from
	// Salesforce) and writes nothing to ~/.sf-deck/cache.db. Unlike
	// Demo, everything else is normal — real orgs, real settings,
	// real devprojects/tags, real usage tracking. Use for a
	// fresh-eyes session, demoing against a live org without
	// polluting the persistent cache, or to rule out stale cache
	// when debugging. Ignored when Demo is set (demo is already
	// ephemeral).
	NoCache bool

	// NoTags runs against a REAL org but with an EPHEMERAL dev-project
	// / tag store: the devprojects.db lives in a throwaway temp dir
	// (removed on Close), so tags, dev projects, saved queries, and
	// saved apex from this session write nothing to the persistent
	// ~/.sf-deck/devprojects.db and the real one is never read. Real
	// orgs, real settings, real data cache otherwise. Ignored when
	// Demo (already ephemeral) or SkipDevProjects (no store at all).
	NoTags bool
}

// Open loads the user's settings, opens all long-lived stores, and
// returns an App ready to drive both the TUI and headless commands.
//
// Order of operations matters: settings before cache (TTL config
// reads from settings), REST client invalidation before any sf calls
// (so a stale token cache from a previous process doesn't bleed
// in), usage tracker installation before sf.OnCall is set so the
// first shell-out lands in the tracker.
//
// Failures are partitioned by criticality:
//
//   - Settings + cache failures are fatal and returned as errors.
//   - Devproject / usage failures are logged via applog and the
//     corresponding field is left nil; the App is still returned.
//   - Org list failures leave Orgs empty and are logged.
func Open(opts OpenOptions) (*App, error) {
	if opts.Demo {
		// Demo never tracks usage and never enumerates real orgs —
		// the seeded cache is the org list.
		opts.SkipUsage = true
		opts.SkipOrgs = true
	}
	// Harden the base ~/.sf-deck dir to owner-only on EVERY entry point,
	// before anything opens a database inside it. This used to live only
	// in applog.Init(), which headless commands skip (SkipApplog) — so a
	// headless launch left an existing 0755 dir untightened and created
	// cache.db / devprojects.db world-readable (0644), exposing cached
	// org data, saved SOQL, and Apex history on a multi-user machine.
	// Runs unconditionally here so the TUI and headless share the same
	// floor. Best-effort: a chmod failure shouldn't block startup.
	hardenBaseDir()

	if !opts.SkipApplog {
		applog.Init()
	}

	// Settings.Load() always returns a usable struct (degraded to
	// empty on parse error) plus an error describing the load
	// failure. We log the error but don't fail — every entry point
	// can boot without prior settings.
	st, loadErr := settings.Load()
	if st == nil {
		return nil, errors.New("settings load returned nil")
	}
	if loadErr != nil {
		applog.Warn("app.settings_load_warning", map[string]any{"err": loadErr.Error()})
	}

	// Push the [ui.api] settings (timeouts, poll intervals, forced
	// API version) into the sf package so EVERY entry point — TUI,
	// headless, demo — respects the user's tuning. The TUI used to
	// do this exclusively, which meant headless validates/deploys
	// shipped with the 5-minute fallback timeout regardless of the
	// user's longer setting. Now it just works.
	sf.ApplyConfig(sf.Config{
		HTTPTimeout:     time.Duration(st.APIHTTPTimeoutSec()) * time.Second,
		CLITimeout:      time.Duration(st.APICLITimeoutSec()) * time.Second,
		RetrieveTimeout: time.Duration(st.APIRetrieveTimeoutSec()) * time.Second,
		DeployDeadline:  time.Duration(st.APIDeployTimeoutSec()) * time.Second,
		DeployPoll:      time.Duration(st.APIDeployPollMs()) * time.Millisecond,
		BulkPoll:        time.Duration(st.APIBulkPollMs()) * time.Millisecond,
		APIVersion:      st.APIVersionOverride(),
	})

	var (
		c        *cache.Cache
		err      error
		demoDir  string
		cacheDir string // throwaway temp dir for --no-cache; removed on Close
	)
	switch {
	case opts.Demo:
		demoDir, err = os.MkdirTemp("", "sf-deck-demo-")
		if err != nil {
			return nil, fmt.Errorf("demo temp dir: %w", err)
		}
		c, err = cache.OpenPath(filepath.Join(demoDir, "cache.db"))
	case opts.NoCache:
		// Ephemeral data cache against a real org: temp cache.db,
		// removed on Close. Settings / devprojects / usage stay on
		// their normal persistent paths.
		cacheDir, err = os.MkdirTemp("", "sf-deck-nocache-")
		if err != nil {
			return nil, fmt.Errorf("no-cache temp dir: %w", err)
		}
		c, err = cache.OpenPath(filepath.Join(cacheDir, "cache.db"))
	default:
		c, err = cache.Open()
	}
	if err != nil {
		return nil, fmt.Errorf("open cache: %w", err)
	}

	// One-time cleanup: records used to be cached to disk; they
	// shouldn't have been (stale data + privacy). Purge any rows
	// from prior versions. List-view metadata + results carry the
	// same rationale.
	_, _ = c.DeleteKeyPrefix("records:")
	_, _ = c.DeleteKeyPrefix("listviews:")
	_, _ = c.DeleteKeyPrefix("listview:")

	// Start every process with a clean in-memory REST client cache.
	// Auth / alias lifecycle changes also invalidate this during the
	// session; this protects tests + embedded callers + multi-run
	// scripts.
	sf.InvalidateRESTClients()

	a := &App{
		Settings:     st,
		Cache:        c,
		SaveSettings: st.Save,
		demoDir:      demoDir,
		cacheDir:     cacheDir,
	}
	a.WriteGate = orgwrite.NewGate(a.ResolveOrg, a.SafetyFor)
	a.Metadata = metadataops.New(a.WriteGate)
	a.MetadataEditors = metadataops.NewEditor(a.WriteGate)
	a.Apex = apexops.New(a.WriteGate)
	a.Records = records.New(a.WriteGate)
	a.Permissions = permissionops.New(a.WriteGate)
	a.Users = userops.New(a.WriteGate)
	a.Notifications = notificationops.New(a.WriteGate)

	if !opts.SkipUsage {
		if t, err := usage.Open(); err == nil {
			a.Usage = t
			sf.OnCall = func(alias string, args []string, e error, dur time.Duration) {
				t.Bump(alias, args, e, dur)
			}
		} else {
			applog.Warn("app.usage_open_failed", map[string]any{"err": err.Error()})
		}
	}

	if !opts.SkipDevProjects {
		var dp *devproject.Store
		switch {
		case opts.Demo:
			// Throwaway store beside the demo cache — saved queries
			// and projects work in the demo without ever touching
			// the user's real devprojects.db.
			dp, err = devproject.OpenPath(filepath.Join(demoDir, "devprojects.db"))
		case opts.NoTags:
			// Ephemeral tag / dev-project store against a real org.
			// Reuse the no-cache temp dir when we made one, else a
			// dedicated one (both removed on Close).
			dir := cacheDir
			if dir == "" {
				dir, err = os.MkdirTemp("", "sf-deck-notags-")
				if err == nil {
					a.tagsDir = dir
				}
			}
			if err == nil {
				dp, err = devproject.OpenPath(filepath.Join(dir, "devprojects.db"))
			}
		default:
			dp, err = devproject.Open()
		}
		if err != nil && devproject.RecoveredFromBackup(err) {
			// Soft error: the store is USABLE but was rebuilt from
			// the .bak (or fresh) after corruption. Keep it AND
			// surface the data-loss warning.
			applog.Warn("app.devproject_recovered", map[string]any{"err": err.Error()})
			a.StartupWarnings = append(a.StartupWarnings, err.Error())
			err = nil
		}
		if err == nil && dp != nil {
			a.Projects = dp
			if trimErr := dp.TrimSOQLHistory(500); trimErr != nil {
				applog.Warn("app.trim_soql_history_failed",
					map[string]any{"err": trimErr.Error()})
			}
			if trimErr := dp.TrimApexHistory(500); trimErr != nil {
				applog.Warn("app.trim_apex_history_failed",
					map[string]any{"err": trimErr.Error()})
			}
		} else if err != nil {
			applog.Warn("app.devproject_open_failed", map[string]any{"err": err.Error()})
		}
	}

	if !opts.SkipOrgs {
		if orgs, err := sf.ListOrgs(); err == nil {
			a.Orgs = orgs
		} else {
			applog.Warn("app.list_orgs_failed", map[string]any{"err": err.Error()})
		}
	}
	a.Bundles = bundles.New(a.Projects, a.WriteGate)

	return a, nil
}

// hardenBaseDir tightens ~/.sf-deck to owner-only (0700), creating it
// if absent. os.MkdirAll only sets the mode on dirs it CREATES, so an
// install first created 0755 by an older build (or by whichever process
// raced in first) keeps its loose mode — the explicit Chmod is what
// fixes existing dirs. Owner-only on the base blocks other local users
// from traversing in, protecting every file inside (cache.db,
// devprojects.db, instances.json) regardless of each file's own mode.
// Best-effort: startup must not fail because a chmod didn't stick.
func hardenBaseDir() {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return
	}
	base := filepath.Join(home, ".sf-deck")
	_ = os.MkdirAll(base, 0o700)
	_ = os.Chmod(base, 0o700)
}

// Close releases every long-lived resource. Safe to call multiple
// times (each underlying handle is checked for nil first) so callers
// can defer this without worrying about partial-init state.
func (a *App) Close() error {
	if a == nil {
		return nil
	}
	var firstErr error
	if a.Cache != nil {
		if err := a.Cache.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		a.Cache = nil
	}
	if a.Usage != nil {
		if err := a.Usage.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		a.Usage = nil
		// Reset the global hook so subsequent processes don't fire
		// callbacks into a closed db.
		sf.OnCall = nil
	}
	if a.Projects != nil {
		if err := a.Projects.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		a.Projects = nil
	}
	if a.demoDir != "" {
		_ = os.RemoveAll(a.demoDir)
		a.demoDir = ""
	}
	if a.cacheDir != "" {
		_ = os.RemoveAll(a.cacheDir)
		a.cacheDir = ""
	}
	if a.tagsDir != "" {
		_ = os.RemoveAll(a.tagsDir)
		a.tagsDir = ""
	}
	applog.Close()
	return firstErr
}

// DemoDir returns the temp dir created by app.Open when Demo is on.
// Empty when this App was opened with Demo=false. Callers that need
// to drop demo fixtures next to the cache/devprojects DBs (e.g. the
// bundle-skeleton seeder) read it via this accessor so the field
// stays unexported.
func (a *App) DemoDir() string {
	if a == nil {
		return ""
	}
	return a.demoDir
}

// ResolveOrg looks up an Org by alias OR username. Headless callers
// pass --org which can be either; this helper is the single resolver
// so the lookup precedence + error messages stay consistent.
//
// Empty target → the user's pinned default if one is set (matches
// what the TUI opens), falling back to the first connected org. The
// pin lives at the sf-deck level (settings.toml), independent of
// sf CLI's lastUsed-driven default.
func (a *App) ResolveOrg(target string) (sf.Org, error) {
	if a == nil {
		return sf.Org{}, errors.New("nil app")
	}
	orgs := a.orgsOrCache()
	if target == "" {
		if len(orgs) == 0 {
			// Cache empty AND in-memory empty. The cache only ever
			// gets repopulated by the TUI's Resource layer on a live
			// refresh, so if the user hasn't opened the TUI yet (or
			// IPC came in before first paint) we need to shell out
			// ourselves. Refresh + retry once.
			if fresh := a.refreshOrgsFromSF(); len(fresh) > 0 {
				orgs = fresh
			} else {
				return sf.Org{}, errors.New("no orgs connected — run `sf org list` first")
			}
		}
		if a.Settings != nil {
			if pinned := a.Settings.DefaultOrgUsername(); pinned != "" {
				for _, o := range orgs {
					if o.Username == pinned {
						return o, nil
					}
				}
			}
		}
		return orgs[0], nil
	}
	if o, ok := findOrg(orgs, target); ok {
		return o, nil
	}
	// Miss against (possibly stale) cache. Refresh once and retry —
	// catches the "alias-added-since-last-cache-write" case where
	// the alias is real but our snapshot doesn't have it. Worst
	// case adds a ~1s shell-out to the FIRST IPC request that
	// names an unknown alias; subsequent calls see the refreshed
	// list.
	if fresh := a.refreshOrgsFromSF(); len(fresh) > 0 {
		if o, ok := findOrg(fresh, target); ok {
			return o, nil
		}
	}
	return sf.Org{}, fmt.Errorf("org %q not found", target)
}

// findOrg does the alias-then-username two-pass lookup. Pulled out
// so ResolveOrg can run it twice (cache, then post-refresh) without
// duplicating the search loop.
func findOrg(orgs []sf.Org, target string) (sf.Org, bool) {
	for _, o := range orgs {
		if o.Alias == target {
			return o, true
		}
	}
	for _, o := range orgs {
		if o.Username == target {
			return o, true
		}
	}
	return sf.Org{}, false
}

// refreshOrgsFromSF shells out to `sf org list`, mirrors the result
// onto a.Orgs, and persists to cache.db so the next ResolveOrg
// (and the TUI, on its next paint) sees the same set. Returns the
// fresh list or nil on error — callers fall back to whatever they
// had.
func (a *App) refreshOrgsFromSF() []sf.Org {
	if a == nil {
		return nil
	}
	orgs, err := sf.ListOrgs()
	if err != nil {
		return nil
	}
	a.Orgs = orgs
	if a.Cache != nil {
		rows := orgsToCacheRows(orgs)
		_ = a.Cache.PutOrgs(rows)
	}
	return orgs
}

// orgsToCacheRows mirrors the converter that lives in ui/uilayout —
// duplicated here because internal/app can't depend on internal/ui.
// Same fields, same direction; if the cache shape grows, both must
// move in lockstep.
func orgsToCacheRows(orgs []sf.Org) []cache.OrgRow {
	out := make([]cache.OrgRow, len(orgs))
	for i, o := range orgs {
		out[i] = cache.OrgRow{
			Username:        o.Username,
			Alias:           o.Alias,
			InstanceURL:     o.InstanceURL,
			OrgID:           o.OrgID,
			IsSandbox:       o.IsSandbox,
			IsScratch:       o.IsScratch,
			IsDevHub:        o.IsDevHub,
			Status:          o.Status,
			ExpirationDate:  o.ExpirationDate,
			IsDefault:       o.IsDefault,
			IsDefaultDevHub: o.IsDefaultDevHub,
		}
	}
	return out
}

// orgsOrCache returns a.Orgs when populated, otherwise reads the
// cached org list from cache.db. The TUI defers `sf org list` to
// first-paint via its own Resource layer, leaving a.Orgs empty at
// startup — IPC bundle handlers (which need ResolveOrg from
// request 1) would otherwise have nothing to match against. The
// cache is the same source the TUI's Resource would populate, so
// this is just "use what's already on disk instead of shelling out
// again."
//
// Returns nil when both are empty (first launch ever, no cache).
// Caller surfaces a clear error.
func (a *App) orgsOrCache() []sf.Org {
	if len(a.Orgs) > 0 {
		return a.Orgs
	}
	if a.Cache == nil {
		return nil
	}
	rows, _, err := a.Cache.GetOrgs()
	if err != nil || len(rows) == 0 {
		return nil
	}
	out := make([]sf.Org, 0, len(rows))
	for _, r := range rows {
		out = append(out, sf.Org{
			Username:        r.Username,
			Alias:           r.Alias,
			InstanceURL:     r.InstanceURL,
			OrgID:           r.OrgID,
			IsSandbox:       r.IsSandbox,
			IsScratch:       r.IsScratch,
			IsDevHub:        r.IsDevHub,
			Status:          r.Status,
			ExpirationDate:  r.ExpirationDate,
			IsDefault:       r.IsDefault,
			IsDefaultDevHub: r.IsDefaultDevHub,
		})
	}
	return out
}

// TargetArg is the value to pass to `sf -o <target>`. Prefers alias
// over username (alias is shorter + stable). Mirrors what the TUI
// uses everywhere.
func TargetArg(o sf.Org) string {
	if o.Alias != "" {
		return o.Alias
	}
	return o.Username
}

// SafetyFor resolves the effective safety level for the given org.
// Combines per-user / per-alias overrides with kind-based defaults
// (production → read_only, sandbox → records, scratch → full) per
// settings.Resolve.
func (a *App) SafetyFor(o sf.Org) settings.SafetyLevel {
	if a == nil || a.Settings == nil {
		// Defensive — without settings we can't resolve, so be safe.
		return settings.SafetyReadOnly
	}
	return a.Settings.Resolve(o.Username, orgKind(o), o.Alias)
}

// CanWrite reports whether the given org permits the requested
// write kind under the effective safety policy. Returns nil on
// allow, a typed BlockedError on deny so headless callers can
// surface a structured response without re-resolving.
func (a *App) CanWrite(o sf.Org, kind settings.WriteKind) error {
	if a != nil && a.WriteGate != nil {
		return a.WriteGate.Check(o, kind)
	}
	// Compatibility for tests and embedded callers that construct App
	// directly rather than through Open. The same shared implementation is
	// still used; only the pre-built field is absent.
	return orgwrite.NewGate(nil, a.SafetyFor).Check(o, kind)
}

// MetadataWrites returns the configured metadata service. The fallback keeps
// embedded callers and older tests that construct App directly working while
// still routing through the shared gate implementation.
func (a *App) MetadataWrites() *metadataops.Service {
	if a == nil {
		return nil
	}
	if a.Metadata != nil {
		return a.Metadata
	}
	gate := a.WriteGate
	if gate == nil {
		gate = orgwrite.NewGate(a.ResolveOrg, a.SafetyFor)
	}
	return metadataops.New(gate)
}

func (a *App) MetadataEditorWrites() *metadataops.EditorService {
	if a == nil {
		return nil
	}
	if a.MetadataEditors != nil {
		return a.MetadataEditors
	}
	gate := a.WriteGate
	if gate == nil {
		gate = orgwrite.NewGate(a.ResolveOrg, a.SafetyFor)
	}
	return metadataops.NewEditor(gate)
}

// ApexWrites returns the configured anonymous-Apex service, with the same
// compatibility fallback used by MetadataWrites for directly-built Apps.
func (a *App) ApexWrites() *apexops.Service {
	if a == nil {
		return nil
	}
	if a.Apex != nil {
		return a.Apex
	}
	gate := a.WriteGate
	if gate == nil {
		gate = orgwrite.NewGate(a.ResolveOrg, a.SafetyFor)
	}
	return apexops.New(gate)
}

// RecordWrites returns the configured record mutation service, with a
// compatibility fallback for directly-built Apps.
func (a *App) RecordWrites() *records.Service {
	if a == nil {
		return nil
	}
	if a.Records != nil {
		return a.Records
	}
	gate := a.WriteGate
	if gate == nil {
		gate = orgwrite.NewGate(a.ResolveOrg, a.SafetyFor)
	}
	return records.New(gate)
}

// BundleWrites returns the configured Salesforce-facing bundle service.
func (a *App) BundleWrites() *bundles.Service {
	if a == nil {
		return nil
	}
	if a.Bundles != nil {
		return a.Bundles
	}
	gate := a.WriteGate
	if gate == nil {
		gate = orgwrite.NewGate(a.ResolveOrg, a.SafetyFor)
	}
	return bundles.New(a.Projects, gate)
}

// PermissionWrites returns the configured permission mutation service.
func (a *App) PermissionWrites() *permissionops.Service {
	if a == nil {
		return nil
	}
	if a.Permissions != nil {
		return a.Permissions
	}
	gate := a.WriteGate
	if gate == nil {
		gate = orgwrite.NewGate(a.ResolveOrg, a.SafetyFor)
	}
	return permissionops.New(gate)
}

// UserWrites returns the configured user administration service.
func (a *App) UserWrites() *userops.Service {
	if a == nil {
		return nil
	}
	if a.Users != nil {
		return a.Users
	}
	gate := a.WriteGate
	if gate == nil {
		gate = orgwrite.NewGate(a.ResolveOrg, a.SafetyFor)
	}
	return userops.New(gate)
}

// NotificationWrites returns the configured notification mutation service.
func (a *App) NotificationWrites() *notificationops.Service {
	if a == nil {
		return nil
	}
	if a.Notifications != nil {
		return a.Notifications
	}
	gate := a.WriteGate
	if gate == nil {
		gate = orgwrite.NewGate(a.ResolveOrg, a.SafetyFor)
	}
	return notificationops.New(gate)
}

// BlockedError is returned by CanWrite when the safety policy
// refuses the write. Headless commands marshal this into the
// standard JSON error envelope ({code: safety_blocked, …}); the
// TUI surfaces it as a flash banner.
type BlockedError = orgwrite.BlockedError

// writeKindLabel remains as the app-level presentation helper used by legacy
// tests and error copy while BlockedError itself is owned by orgwrite.
func writeKindLabel(k settings.WriteKind) string {
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

// orgKind translates an sf.Org's textual kind back into the typed
// settings.OrgKind. sf.Org.Kind returns "Production" / "Sandbox" /
// "Scratch" / "DevHub" (capitalized); settings.OrgKind constants are
// the same capitalized strings.
func orgKind(o sf.Org) settings.OrgKind {
	switch o.Kind() {
	case "Production":
		return settings.KindProduction
	case "Sandbox":
		return settings.KindSandbox
	case "Scratch":
		return settings.KindScratch
	case "DevHub":
		return settings.KindDevHub
	}
	return settings.KindProduction
}
