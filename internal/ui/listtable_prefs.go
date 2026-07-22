package ui

import (
	"maps"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

const listTableWidthPrefsKey = "ui:listtable-widths:v1"

type listTableWidthPrefs struct {
	Version int                       `json:"version"`
	Scopes  map[string]map[string]int `json:"scopes"`
}

type listTableContext struct {
	State       *uilayout.ListTableState
	Cols        []uilayout.ListColumn
	Measure     func(col int) int
	OrgUsername string
	Scope       string
	// Cell / RowCount / RenderCols expose the active list's rendered
	// contents when the surface opts into the shared renderer
	// (BuildRenderModel). Used by list-wide yank chords (q-y table,
	// q-i id list). Nil / 0 on surfaces that haven't migrated.
	Cell       func(row, col int) string
	RowCount   int
	RenderCols []uilayout.ListColumn
}

func (c listTableContext) persistable() bool {
	return c.State != nil && c.OrgUsername != "" && c.Scope != ""
}

func (m *Model) applyListTableWidthPrefs(ctx listTableContext) {
	if !ctx.persistable() {
		return
	}
	prefs := m.ensureListTableWidthPrefs(ctx.OrgUsername)
	if prefs == nil {
		return
	}
	// Assign unconditionally: a scope with no saved widths CLEARS the
	// live state. Early-returning here left the previous scope's
	// widths in the shared state, and the next save then persisted
	// them under the new scope — how widths used to silently "copy"
	// from chip to chip.
	ctx.State.UserWidths = maps.Clone(prefs.Scopes[ctx.Scope])

	m.applyPerViewSort(ctx)
}

// sortPref is a stashed (column, direction) sort for one view.
type sortPref struct {
	Column string
	Desc   bool
}

// applyPerViewSort gives each view its own sort when [ui] sort_per_view
// is enabled. The ListTableState (sort/widths/scroll) is shared per
// surface; here we swap ONLY the sort fields in/out based on the active
// view so switching views restores that view's last sort. When the
// setting is off it's a no-op and sort stays shared — the default.
//
// Mechanism: track the view key the shared state currently reflects.
// When it changes, save the outgoing view's sort under the old key and
// load the incoming view's (or clear if it never had one).
func (m *Model) applyPerViewSort(ctx listTableContext) {
	if ctx.State == nil || m.settings == nil || !m.settings.SortPerView() {
		return
	}
	key := ctx.Scope + "|" + m.activeChipIDForRender()
	if key == m.perViewSortKey {
		return // same view — nothing to swap
	}
	if m.perViewSort == nil {
		m.perViewSort = map[string]sortPref{}
	}
	// Save the sort that belonged to the previously-active view.
	if m.perViewSortKey != "" {
		m.perViewSort[m.perViewSortKey] = sortPref{
			Column: ctx.State.SortColumn, Desc: ctx.State.SortDesc,
		}
	}
	// Load the incoming view's sort (zero value = no sort → cleared).
	pref := m.perViewSort[key]
	ctx.State.SortColumn = pref.Column
	ctx.State.SortDesc = pref.Desc
	m.perViewSortKey = key
}

func (m *Model) ensureListTableWidthPrefs(orgUsername string) *listTableWidthPrefs {
	if orgUsername == "" {
		return nil
	}
	if m.listTableWidthPrefs == nil {
		m.listTableWidthPrefs = map[string]*listTableWidthPrefs{}
	}
	if m.listTableWidthPrefsLoaded == nil {
		m.listTableWidthPrefsLoaded = map[string]bool{}
	}
	if m.listTableWidthPrefsLoaded[orgUsername] {
		return m.listTableWidthPrefs[orgUsername]
	}
	prefs := &listTableWidthPrefs{Version: 1, Scopes: map[string]map[string]int{}}
	if m.cache != nil {
		var loaded listTableWidthPrefs
		_, ok, err := m.cache.GetJSON(orgUsername, listTableWidthPrefsKey, &loaded)
		if err != nil {
			applog.Warn("listtable.width_prefs_load_failed", map[string]any{
				"org": orgUsername,
				"err": err.Error(),
			})
		} else if ok {
			prefs = normalizeListTableWidthPrefs(loaded)
		}
	}
	m.listTableWidthPrefs[orgUsername] = prefs
	m.listTableWidthPrefsLoaded[orgUsername] = true
	return prefs
}

// legacyChipScopeBases are the per-surface width scopes that older
// builds suffixed with ":<chipID>". Normalisation folds those back
// into the base (base entry wins; otherwise first variant seen).
// records:* scopes are intentionally NOT in this list — records keeps
// per-chip widths because its list-view chips change the columns.
var legacyChipScopeBases = []string{
	"objects", "flows", "apex:classes", "apex:triggers",
	"components:lwc", "components:aura", "perms:permsets", "perms:psgs",
	"perms:profiles", "perms:queues", "perms:public-groups",
	"users:recent", "users", "soql:saved", "soql:history", "recent",
}

func foldLegacyChipScope(scope string) string {
	if isLegacyChipScopeBase(scope) {
		return scope
	}
	// Longest matching base wins so "users:recent:<chip>" folds to
	// "users:recent", not "users".
	best := ""
	for _, base := range legacyChipScopeBases {
		if strings.HasPrefix(scope, base+":") && len(base) > len(best) {
			best = base
		}
	}
	if best != "" {
		return best
	}
	return scope
}

func isLegacyChipScopeBase(scope string) bool {
	for _, base := range legacyChipScopeBases {
		if scope == base {
			return true
		}
	}
	return false
}

func normalizeListTableWidthPrefs(p listTableWidthPrefs) *listTableWidthPrefs {
	out := &listTableWidthPrefs{Version: 1, Scopes: map[string]map[string]int{}}
	for scope, widths := range p.Scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" || len(widths) == 0 {
			continue
		}
		if folded := foldLegacyChipScope(scope); folded != scope {
			// Base entry wins; otherwise first folded variant lands.
			if _, exists := p.Scopes[folded]; exists {
				continue
			}
			if _, taken := out.Scopes[folded]; taken {
				continue
			}
			scope = folded
		}
		clean := map[string]int{}
		for name, width := range widths {
			if strings.TrimSpace(name) == "" || width <= 0 {
				continue
			}
			clean[name] = width
		}
		if len(clean) > 0 {
			out.Scopes[scope] = clean
		}
	}
	return out
}

func (m *Model) rememberListTableWidths(ctx listTableContext) listTableWidthPrefs {
	prefs := m.ensureListTableWidthPrefs(ctx.OrgUsername)
	if prefs == nil {
		return listTableWidthPrefs{Version: 1, Scopes: map[string]map[string]int{}}
	}
	if prefs.Scopes == nil {
		prefs.Scopes = map[string]map[string]int{}
	}
	if ctx.State == nil || len(ctx.State.UserWidths) == 0 {
		delete(prefs.Scopes, ctx.Scope)
	} else {
		prefs.Scopes[ctx.Scope] = maps.Clone(ctx.State.UserWidths)
	}
	return cloneListTableWidthPrefs(*prefs)
}

func cloneListTableWidthPrefs(p listTableWidthPrefs) listTableWidthPrefs {
	out := listTableWidthPrefs{Version: 1, Scopes: map[string]map[string]int{}}
	for scope, widths := range p.Scopes {
		if len(widths) > 0 {
			out.Scopes[scope] = maps.Clone(widths)
		}
	}
	return out
}

func (m Model) saveListTableWidthsCmd(ctx listTableContext) tea.Cmd {
	if !ctx.persistable() || m.cache == nil {
		return nil
	}
	prefs := (&m).rememberListTableWidths(ctx)
	return writeListTableWidthPrefsCmd(m.cache, ctx.OrgUsername, prefs)
}

func writeListTableWidthPrefsCmd(c *cache.Cache, orgUsername string, prefs listTableWidthPrefs) tea.Cmd {
	if c == nil || orgUsername == "" {
		return nil
	}
	return func() tea.Msg {
		if err := c.PutJSON(orgUsername, listTableWidthPrefsKey, prefs); err != nil {
			applog.Warn("listtable.width_prefs_save_failed", map[string]any{
				"org": orgUsername,
				"err": err.Error(),
			})
		}
		return nil
	}
}

// listSurfaceWidthScope is deliberately per-SURFACE, not per-chip:
// on metadata surfaces every chip renders the identical column set
// (chips are row filters), so a width tweak should follow the user
// across All/Active/custom views. Records is the exception — its SF
// list-view chips each carry their own columns — and keeps per-chip
// scoping via recordsWidthScope. Chipped scopes that older builds
// persisted are folded into the base by normalizeListTableWidthPrefs.
func (m Model) listSurfaceWidthScope(surf *listSurface, d *orgData) string {
	return listSurfaceBaseWidthScope(surf, d)
}

func listSurfaceBaseWidthScope(surf *listSurface, d *orgData) string {
	switch surf {
	case &objectsListSurface:
		return "objects"
	case &flowsListSurface:
		return "flows"
	case &apexLogsListSurface:
		return "apex-logs"
	case &setupAuditListSurface:
		return "setup-audit"
	case &flowInterviewsListSurface:
		return "flow-interviews"
	case &activeUsersListSurface:
		return "active-users"
	case &userSessionsListSurface:
		return "user-sessions"
	case &communitiesListSurface:
		return "communities"
	case &communityPagesListSurface:
		return "community-pages"
	case &deploysListSurface:
		return "deploys"
	case &packagesListSurface:
		return "packages"
	case &recentListSurface, &homeRecentListSurface:
		return "recent"
	case &homeNotificationsListSurface:
		return "home:notifications"
	case &homeLimitsListSurface:
		return "home:limits"
	case &homeLicensesListSurface:
		return "home:licenses"
	case &apexClassesListSurface:
		return "apex:classes"
	case &dashboardsListSurface:
		return "reports:dashboards"
	case &reportTypesListSurface:
		return "reports:types"
	case &metaTypesListSurface:
		return "meta:browse"
	case &metaTypeItemsListSurface:
		return "meta:type-items"
	case &cmtListSurface:
		return "meta:cmt"
	case &customLabelsListSurface:
		return "meta:labels"
	case &customSettingsListSurface:
		return "meta:custom-settings"
	case &staticResourcesListSurface:
		return "meta:static-resources"
	case &namedCredsListSurface:
		return "meta:named-creds"
	case &remoteSitesListSurface:
		return "meta:remote-sites"
	case &apexTriggersListSurface:
		return "apex:triggers"
	case &lwcListSurface:
		return "components:lwc"
	case &auraListSurface:
		return "components:aura"
	case &permsetsListSurface:
		return "perms:permsets"
	case &psgsListSurface:
		return "perms:psgs"
	case &profilesListSurface:
		return "perms:profiles"
	case &queuesListSurface:
		return "perms:queues"
	case &publicGroupsListSurface:
		return "perms:public-groups"
	case &recentUsersListSurface:
		return "users:recent"
	case &allUsersListSurface:
		return "users"
	case &soqlSavedListSurface:
		return "soql:saved"
	case &soqlHistoryListSurface:
		return "soql:history"
	case &execSavedListSurface:
		return "exec:saved"
	case &execHistoryListSurface:
		return "exec:history"
	}
	return ""
}

func recordsWidthScope(d *orgData, sobject string) string {
	if d == nil || sobject == "" {
		return ""
	}
	chipID := selectedRecordsChip(d, sobject)
	if chipID == "" {
		return ""
	}
	mode := "local"
	if currentChipMode(d, sobject) == ChipModeSalesforce {
		mode = "salesforce"
	}
	return "records:" + sobject + ":" + mode + ":" + chipID
}
