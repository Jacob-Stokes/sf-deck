package ui

// Cache & refresh policy modal — the single visible source of truth
// for "how long does sf-deck consider each resource fresh before it
// auto-refreshes." Reached from = → Cache & refresh policy.
//
// Shape: a list of every Resource the app knows about, with each row
// showing key · default · effective. Enter on a row opens an edit
// modal (same primitive Inspector URL uses) accepting a Go duration
// string (e.g. "5m", "1h", "30s", "0" to disable auto-refresh).
//
// Adding a new Resource type is one new entry in cacheResourceCatalog —
// the modal picks it up automatically.

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// cacheResource describes one configurable Resource TTL. The Key is
// what settings.toml stores under [ui.cache.ttl]; Default is the
// fallback when the user hasn't overridden it; Description is the
// row hint shown in the modal.
type cacheResource struct {
	Key         string
	Default     time.Duration
	Description string
}

// cacheResourceCatalog enumerates every Resource whose TTL is
// surfaced in the modal. Order matters — this is the order rows
// appear. New Resources slot in here; the modal picks them up
// automatically.
//
// Keep this in sync with the actual TTL keys used in newOrgData and
// the lazy Ensure helpers (search for `ttl(` / `d.ttl(` to find
// every callsite).
var cacheResourceCatalog = []cacheResource{
	// Global (cross-org), loaded eagerly at startup.
	{Key: "orgs", Default: time.Minute,
		Description: "the org list shown in the left rail (sf org list)"},
	{Key: "projects", Default: 10 * time.Minute,
		Description: "local sfdx project directories discovered for /bundles"},
	// Org-wide, loaded eagerly at startup.
	{Key: "home", Default: 10 * time.Minute,
		Description: "user home: org info, daily API usage, current user id"},
	{Key: "sobjects", Default: 4 * time.Hour,
		Description: "the sObject catalogue (every queryable sObject in the org)"},
	{Key: "describes", Default: 4 * time.Hour,
		Description: "per-sObject describe (fields, picklists, references)"},
	{Key: "apex_logs", Default: 30 * time.Second,
		Description: "/apexlogs — short TTL because logs are live data"},
	{Key: "deploys", Default: 2 * time.Minute,
		Description: "/deploys — short TTL, delta-refresh fills in newer rows"},
	{Key: "packages", Default: 2 * time.Hour,
		Description: "installed packages list"},
	{Key: "flows", Default: 15 * time.Minute,
		Description: "/flows — flow definitions + active version metadata"},
	{Key: "permsets_full", Default: 30 * time.Minute,
		Description: "/perms — every PermissionSet in the org"},
	{Key: "psgs", Default: 30 * time.Minute,
		Description: "/perms — every PermissionSetGroup"},
	{Key: "profiles", Default: 30 * time.Minute,
		Description: "/perms — every Profile"},
	{Key: "permsets", Default: 2 * time.Hour,
		Description: "the FLS scope picker (every assignable parent)"},
	{Key: "validation_rules", Default: 30 * time.Minute,
		Description: "per-sObject validation rule lists"},
	{Key: "record_types", Default: 30 * time.Minute,
		Description: "per-sObject RecordType lists (also drives the wizard's value picker)"},
	{Key: "triggers", Default: 30 * time.Minute,
		Description: "per-sObject Apex trigger lists"},
	{Key: "object_perms", Default: 30 * time.Minute,
		Description: "per-permset ObjectPermission grids"},
	{Key: "system_perms", Default: 30 * time.Minute,
		Description: "per-permset system permission grids"},
	{Key: "permset_users", Default: 30 * time.Minute,
		Description: "per-permset User assignment lists"},
	{Key: "validation_detail", Default: 30 * time.Minute,
		Description: "single validation rule's full Metadata XML"},
	{Key: "record_type_detail", Default: 30 * time.Minute,
		Description: "single record type's picklist + page-layout details"},
	{Key: "trigger_detail", Default: 30 * time.Minute,
		Description: "single trigger's source body"},
	{Key: "flow_versions", Default: 1 * time.Hour,
		Description: "per-flow version history"},
	{Key: "reports", Default: 1 * time.Hour,
		Description: "/reports — saved report catalogue"},
	{Key: "report_runs", Default: 24 * time.Hour,
		Description: "report preview runs (per report) · `r` to refresh"},
	{Key: "record_detail", Default: 24 * time.Hour,
		Description: "single record drill-in (per sobject+id) · `r` to refresh"},

	// Lazy per-(sobject, …) Resources. Long TTLs by default — first
	// fetch lands and stays for the session; user presses `r` to
	// force a manual refresh.
	{Key: "records", Default: 24 * time.Hour,
		Description: "records list (default 'recent') · `r` on the records subtab to refresh"},
	{Key: "chip_records", Default: 24 * time.Hour,
		Description: "chip-driven records (per chip+sObject) · `r` to refresh"},
	{Key: "list_views", Default: 24 * time.Hour,
		Description: "Salesforce list-view catalog per sObject"},
	{Key: "list_view_results", Default: 24 * time.Hour,
		Description: "running a Salesforce list view (rows + columns)"},
	{Key: "fls", Default: 1 * time.Hour,
		Description: "field-level security per (sobject, parent) combo"},
}

// cacheSettingsState is the live state of the modal.
type cacheSettingsState struct {
	rows   []cacheRow // computed at open time, frozen for the modal
	cursor int
}

type cacheRow struct {
	res       cacheResource
	override  string // settings override raw string (or "" if none)
	effective time.Duration
}

// openCacheSettingsModal opens the cache-settings overview. Each row
// is read-only here — pressing enter on one drills into a single-
// field edit modal that accepts a Go duration string.
func (m *Model) openCacheSettingsModal() tea.Cmd {
	rows := buildCacheRows(m.settings)
	state := &cacheSettingsState{rows: rows}
	m.cacheSettings = state
	return nil
}

// buildCacheRows snapshots the catalogue against the user's current
// settings. Sorted by key so the modal's row order is stable
// regardless of how the catalogue is declared.
func buildCacheRows(s interface {
	CacheTTL(string, time.Duration) time.Duration
	CacheTTLOverride(string) string
}) []cacheRow {
	out := make([]cacheRow, 0, len(cacheResourceCatalog))
	for _, r := range cacheResourceCatalog {
		row := cacheRow{res: r, effective: r.Default}
		if s != nil {
			row.effective = s.CacheTTL(r.Key, r.Default)
			row.override = s.CacheTTLOverride(r.Key)
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].res.Key < out[j].res.Key
	})
	return out
}

// renderCacheSettings draws the modal, or "" when not open.
func (m Model) renderCacheSettings() string {
	if m.cacheSettings == nil {
		return ""
	}
	st := m.cacheSettings
	w := modalWidth(m.width, 80, 120)
	inner := w - 4

	titleStyle := lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true)
	subStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	hilightStyle := lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true)
	boldStyle := lipgloss.NewStyle().Foreground(theme.Fg).Bold(true)
	barStyle := lipgloss.NewStyle().Foreground(theme.BorderHi)

	var lines []string
	lines = append(lines, titleStyle.Render("Cache & refresh policy"))
	lines = append(lines, subStyle.Render(
		"Each row shows one resource, its default TTL, and the effective TTL after your overrides. "+
			"Enter to edit · esc to close · r to reset to default · C to clear ALL cached data."))
	lines = append(lines, strings.Repeat("─", inner))

	const keyCol = 26
	const defCol = 12
	const effCol = 12

	header := padRight("KEY", keyCol) + padRight("DEFAULT", defCol) + padRight("EFFECTIVE", effCol) + "DESCRIPTION"
	lines = append(lines, subStyle.Render(header))

	for i, row := range st.rows {
		focused := i == st.cursor
		prefix := "  "
		if focused {
			prefix = barStyle.Render("▌") + " "
		}
		key := padRight(row.res.Key, keyCol-2)
		def := padRight(formatDuration(row.res.Default), defCol)
		effRaw := formatDuration(row.effective)
		eff := padRight(effRaw, effCol)
		if row.override != "" {
			eff = hilightStyle.Render(effRaw) + strings.Repeat(" ", effCol-len(effRaw))
		}
		desc := row.res.Description
		// Trim description if line is too wide.
		max := inner - keyCol - defCol - effCol - 4
		if max > 0 && len(desc) > max {
			desc = ansi.Truncate(desc, max, "…")
		}
		body := key + def + eff + subStyle.Render(desc)
		if focused {
			body = boldStyle.Render(key) + def + eff + subStyle.Render(desc)
		}
		lines = append(lines, prefix+body)
	}

	lines = append(lines, "")
	lines = append(lines, subStyle.Render(
		"j/k move · enter edit · r reset to default · C clear all cached data · esc close · TTL of 0 disables auto-refresh"))
	return modalBox(strings.Join(lines, "\n"), w)
}

// handleCacheSettingsKey is the reducer for the modal.
func (m Model) handleCacheSettingsKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.cacheSettings == nil {
		return m, nil
	}
	st := m.cacheSettings
	key := msg.String()
	switch key {
	case "esc", "ctrl+c":
		m.cacheSettings = nil
		return m, nil
	case "j", "down":
		if st.cursor < len(st.rows)-1 {
			st.cursor++
		}
		return m, nil
	case "k", "up":
		if st.cursor > 0 {
			st.cursor--
		}
		return m, nil
	case "g", "home":
		st.cursor = 0
		return m, nil
	case "G", "end":
		st.cursor = len(st.rows) - 1
		return m, nil
	case "enter":
		if st.cursor < 0 || st.cursor >= len(st.rows) {
			return m, nil
		}
		row := st.rows[st.cursor]
		return m, m.openCacheTTLEditor(row)
	case "C":
		// Clear all cached API data. Empties cache.db, drops every
		// org's in-memory Resource state, and re-fetches the current
		// view so the user sees data reload immediately. Other views
		// re-fetch lazily on next visit. TTL overrides (this modal's
		// rows) are settings, not cache — they survive.
		return m.clearAllCache()
	}
	if matches(key, Keys.CacheResetTTL) {
		if st.cursor < 0 || st.cursor >= len(st.rows) {
			return m, nil
		}
		row := st.rows[st.cursor]
		if m.settings != nil {
			m.settings.SetCacheTTLOverride(row.res.Key, "")
			if m.saveSettings("reset " + row.res.Key + " to default") {
				st.rows = buildCacheRows(m.settings)
			}
		}
		return m, nil
	}
	return m, nil
}

// openCacheTTLEditor opens a single-line edit modal for one row. The
// Save closure parses the input as a Go duration, persists, and
// re-snapshots the cache rows.
func (m *Model) openCacheTTLEditor(row cacheRow) tea.Cmd {
	initial := row.override
	if initial == "" {
		initial = formatDuration(row.res.Default)
	}
	state := editModalState{
		Title: "TTL · " + row.res.Key,
		Hint: fmt.Sprintf("default: %s · format: 5m / 1h / 30s / 0 to disable auto-refresh · blank to reset",
			formatDuration(row.res.Default)),
		InitialBody: initial,
		Multiline:   false,
		SuccessMsg:  "TTL saved",
		Save: func(val string, _ any) error {
			if m.settings == nil {
				return nil
			}
			val = strings.TrimSpace(val)
			if val == "" {
				m.settings.SetCacheTTLOverride(row.res.Key, "")
			} else {
				// Validate by parsing.
				if _, err := time.ParseDuration(val); err != nil {
					return fmt.Errorf("invalid duration %q: %w", val, err)
				}
				m.settings.SetCacheTTLOverride(row.res.Key, val)
			}
			if err := m.settings.Save(); err != nil {
				return err
			}
			// Re-snapshot the modal's rows so the new value lights
			// up immediately when the user comes back from the edit
			// modal.
			if m.cacheSettings != nil {
				m.cacheSettings.rows = buildCacheRows(m.settings)
			}
			return nil
		},
	}
	return m.openEditModal(state)
}

// clearAllCache empties the on-disk response cache and resets every
// org's in-memory Resource state, then re-fetches the current view so
// the reload is immediately visible. Other views re-fetch lazily when
// next visited (their resources are back to never-loaded, and the cache
// is now empty, so Ensure goes to the network). REST clients are
// invalidated too so a stale session token from before the clear can't
// linger. The cache-settings modal stays open with a result flash.
func (m Model) clearAllCache() (Model, tea.Cmd) {
	mm := &m
	if mm.cache == nil {
		mm.flash("no cache to clear")
		return *mm, nil
	}
	n, err := mm.cache.ClearAll()
	if err != nil {
		// ClearAll reports a partial success (rows gone, VACUUM failed)
		// as an error carrying the count — surface it honestly rather
		// than swallowing the "data did clear" fact.
		mm.flash("clear cache: " + err.Error())
	} else {
		mm.flash(fmt.Sprintf("cache cleared — %d entries dropped, refreshing…", n))
	}

	// Drop all in-memory org data so resources reset to never-loaded.
	// ensureOrgData rebuilds the active org's orgData on the next call.
	mm.data = map[string]*orgData{}
	sf.InvalidateRESTClients()

	// Re-fetch whatever the user is currently looking at so the clear
	// isn't a silent void — the active view reloads in front of them.
	return mm.refreshCurrent()
}

// formatDuration renders a duration as a compact human-friendly
// string ("5m", "1h", "30s") rather than time.Duration's default
// "1h30m0s" shape.
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0 (off)"
	}
	if d >= time.Hour && d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	if d >= time.Minute && d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	if d%time.Second == 0 {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	return d.String()
}
