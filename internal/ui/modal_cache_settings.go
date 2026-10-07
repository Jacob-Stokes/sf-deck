package ui

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

type cacheResource struct {
	Key         string
	Default     time.Duration
	Description string
}

var cacheResourceCatalog = []cacheResource{
	{Key: "orgs", Default: time.Minute,
		Description: "the org list shown in the left rail (sf org list)"},
	{Key: "projects", Default: 10 * time.Minute,
		Description: "local sfdx project directories discovered for /bundles"},
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

type cacheSettingsState struct {
	rows   []cacheRow // computed at open time, frozen for the modal
	cursor int
}

type cacheRow struct {
	res       cacheResource
	override  string // settings override raw string (or "" if none)
	effective time.Duration
}

func (m *Model) openCacheSettingsModal() tea.Cmd {
	rows := buildCacheRows(m.settings)
	state := &cacheSettingsState{rows: rows}
	m.cacheSettings = state
	return nil
}

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
				if _, err := time.ParseDuration(val); err != nil {
					return fmt.Errorf("invalid duration %q: %w", val, err)
				}
				m.settings.SetCacheTTLOverride(row.res.Key, val)
			}
			if err := m.settings.Save(); err != nil {
				return err
			}
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
		mm.flash("clear cache: " + err.Error())
	} else {
		mm.flash(fmt.Sprintf("cache cleared — %d entries dropped, refreshing…", n))
	}

	// Drop all in-memory org data so resources reset to never-loaded.
	// ensureOrgData rebuilds the active org's orgData on the next call.
	mm.data = map[string]*orgData{}
	sf.InvalidateRESTClients()

	return mm.refreshCurrent()
}

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
