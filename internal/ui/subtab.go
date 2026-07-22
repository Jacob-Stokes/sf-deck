package ui

// Subtab machinery.
//
// A Tab can contain multiple Subtabs. The Objects tab's drill-in state
// has (eventually) many: Schema, Records, Flows, Triggers, Validation
// Rules, Apex, Layouts, Record Types, etc. Each Subtab owns its own
// view chips + list + render function.
//
// Principles:
//   - When a context has ≥2 subtabs, we show a pill-shaped strip below
//     the breadcrumb. Selected one has a blue underline.
//   - When a context has only 1 subtab, the strip is hidden entirely.
//   - '[' / ']' cycle subtabs; '←/→' cycle views within the selected
//     subtab.
//   - Every tab+drill combo declares its subtabs statically (for now —
//     future plugins will register them dynamically).

import (
	"time"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// Subtab identifies one mode within a tab. IDs are stable across
// renames and map to config / keymap / breadcrumb labels.
type Subtab string

const (
	SubtabDetails     Subtab = "details"
	SubtabSchema      Subtab = "schema"
	SubtabRecords     Subtab = "records"
	SubtabFlows       Subtab = "flows"
	SubtabTriggers    Subtab = "triggers"
	SubtabValidation  Subtab = "validation"
	SubtabApex        Subtab = "apex"
	SubtabLayouts     Subtab = "layouts"
	SubtabRecordTypes Subtab = "recordtypes"
	SubtabPermissions Subtab = "permissions"
	SubtabFLS         Subtab = "fls"

	// Home subtabs. Each slots into the "details box" on /home while
	// ORG + API LIMITS stay pinned above.
	SubtabHomeUsers    Subtab = "home-users"
	SubtabHomeLicenses Subtab = "home-licenses"
	SubtabHomeJobs     Subtab = "home-jobs"
	SubtabHomeDeploys  Subtab = "home-deploys"
	SubtabHomePackages Subtab = "home-packages"

	// /perms dashboard subtabs — each lists one kind of parent.
	SubtabPermSets Subtab = "permsets"
	SubtabPSGs     Subtab = "psgs"
	SubtabProfiles Subtab = "profiles"

	// /perms parent-detail subtabs — the view axes inside one
	// permset / PSG / profile.
	SubtabParentOverview   Subtab = "parent-overview"
	SubtabParentObjects    Subtab = "parent-objects"
	SubtabParentFields     Subtab = "parent-fields"
	SubtabParentSystem     Subtab = "parent-system"
	SubtabParentUsers      Subtab = "parent-users"
	SubtabParentComponents Subtab = "parent-components" // PSG only

	// /system subtabs — unified observability surface bundling
	// previously-top-level Logs + Deploys + the API usage modal.
	SubtabSystemLogs       Subtab = "system-logs"
	SubtabSystemDeploys    Subtab = "system-deploys"
	SubtabSystemAudit      Subtab = "system-audit"
	SubtabSystemInterviews Subtab = "system-interviews"
	SubtabSystemAsyncJobs  Subtab = "system-async-jobs"
	SubtabSystemScheduled  Subtab = "system-scheduled"
	SubtabSystemAPI        Subtab = "system-api"

	// /apex subtabs — code-shaped surfaces.
	SubtabApexClasses  Subtab = "apex-classes"
	SubtabApexTriggers Subtab = "apex-triggers"

	// /components subtabs — UI-shaped surfaces.
	SubtabComponentsLWC  Subtab = "components-lwc"
	SubtabComponentsAura Subtab = "components-aura"

	// /home subtabs — Recent moves here from the standalone /recent
	// tab; Notifications surfaces the bell stream; Limits renders
	// the full /services/data/vNN/limits payload as a sortable
	// list-table.
	SubtabHomeLanding       Subtab = "home-landing"
	SubtabHomeRecent        Subtab = "home-recent"
	SubtabHomeNotifications Subtab = "home-notifications"
	SubtabHomeLimits        Subtab = "home-limits"
	SubtabHomeDownloads     Subtab = "home-downloads"
	// Legacy subtab IDs — kept as constants so persisted user state
	// referencing them doesn't crash. They no longer appear in
	// homeSubtabs() and the registry doesn't render them.
	SubtabHomeLogs  Subtab = "home-logs"
	SubtabHomeAPI   Subtab = "home-api"
	SubtabHomeAudit Subtab = "home-audit"

	// /perms subtabs — existing PermSets/PSGs/Profiles + routing &
	// sharing additions.
	SubtabPermsQueues       Subtab = "perms-queues"
	SubtabPermsPublicGroups Subtab = "perms-public-groups"
	SubtabPermsSharingRules Subtab = "perms-sharing-rules"

	// /reports subtabs — the folder browser is the Reports subtab;
	// Dashboards and Report Types are full list-engine surfaces.
	SubtabReportsReports     Subtab = "reports-reports"
	SubtabReportsDashboards  Subtab = "reports-dashboards"
	SubtabReportsReportTypes Subtab = "reports-types"

	// /meta subtabs — admin-metadata catch-all hub. Most are
	// placeholders pending dedicated renderers.
	SubtabMetaBrowse             Subtab = "meta-browse"
	SubtabMetaCustomMetadata     Subtab = "meta-cmt"
	SubtabMetaCustomLabels       Subtab = "meta-labels"
	SubtabMetaCustomSettings     Subtab = "meta-custom-settings"
	SubtabMetaStaticResources    Subtab = "meta-static"
	SubtabMetaNamedCredentials   Subtab = "meta-namedcreds"
	SubtabMetaRemoteSiteSettings Subtab = "meta-rss"

	// /objects subtab additions.
	SubtabObjectLayouts Subtab = "object-layouts"
	SubtabObjectFlows   Subtab = "object-flows"

	// /dev-project-detail subtabs — split the per-project drill-in
	// into Items (the existing flat tree) and Bundles (sfdx project
	// directories linked to this DevProject).
	SubtabDevProjectItems   Subtab = "devproject-items"
	SubtabDevProjectBundles Subtab = "devproject-bundles"

	// /dev-projects (top-level) subtabs — Projects (existing list)
	// and Bundles (all bundles across every DevProject so the user
	// can scan their on-disk artifacts without drilling into each
	// project first).
	SubtabDevProjectsList    Subtab = "devprojects-list"
	SubtabDevProjectsBundles Subtab = "devprojects-bundles"

	// /soql subtabs — Editor (query+results workspace), Saved (the
	// curated library of named queries the user manages), and
	// History (read-only log of every execution per org). The split
	// keeps gestures clean: Saved supports rename / delete / pin /
	// tag; History is consult-only.
	//
	// Editor stays the default landing experience so users with
	// muscle memory aren't forced through a list before typing.
	SubtabSOQLEditor  Subtab = "soql-editor"
	SubtabSOQLSaved   Subtab = "soql-saved"
	SubtabSOQLHistory Subtab = "soql-history"

	// /users subtabs.
	SubtabUsersRecent Subtab = "users-recent"
	SubtabUsersAll    Subtab = "users-all"
	SubtabUsersActive Subtab = "users-active"

	// /exec subtabs — anonymous Apex editor, debug-log output of the
	// most recent run, saved snippets library, and execution history.
	SubtabExecEditor  Subtab = "exec-editor"
	SubtabExecOutput  Subtab = "exec-output"
	SubtabExecSaved   Subtab = "exec-saved"
	SubtabExecHistory Subtab = "exec-history"

	// /compare subtabs — New (setup form only), Result (the active/opened
	// comparison: retrieving → inventory → drill-in diff), Saved
	// (templates + saved results), History (past runs).
	SubtabCompareNew     Subtab = "compare-new"
	SubtabCompareResult  Subtab = "compare-result"
	SubtabCompareSaved   Subtab = "compare-saved"
	SubtabCompareHistory Subtab = "compare-history"
)

// subtabInfo holds per-subtab display metadata.
type subtabInfo struct {
	ID    Subtab
	Label string
}

// subtabInfosFor projects one tab's registry-declared Subtabs into
// the strip shape. THE single source for static subtab lists — the
// per-tab helper funcs below are thin delegations kept only for
// call-site readability. (Before 2026-06-13 each helper duplicated
// its registry list as a literal; the two could silently drift —
// e.g. the meta stub cut had to edit both. Now the registry is the
// only place a subtab set is written down. objectDrillSubtabs is
// the one inversion: the registry derives from IT via
// objectDrillSubtabSpecs, which is the same single-source property
// from the other direction.)
func subtabInfosFor(t Tab) []subtabInfo {
	spec := lookupTabSpec(t)
	if spec == nil || len(spec.Subtabs) == 0 {
		return nil
	}
	out := make([]subtabInfo, len(spec.Subtabs))
	for i, sub := range spec.Subtabs {
		out[i] = subtabInfo{ID: sub.ID, Label: sub.Label}
	}
	return out
}

// objectDrillSubtabs returns every subtab available at the
// TabObjectDetail level. The first objectDrillPinnedCount entries
// are pinned to the strip; the remainder live in the More… modal
// (slot 0 of the strip). Pinned ordering matches admin frequency:
// Details (landing), Schema (fields), Records (data), FLS (per-
// profile read/edit), Validation (rules), Record Types.
func objectDrillSubtabs() []subtabInfo {
	return []subtabInfo{
		{ID: SubtabDetails, Label: "Details"},
		{ID: SubtabSchema, Label: "Schema"},
		{ID: SubtabRecords, Label: "Records"},
		{ID: SubtabFLS, Label: "FLS"},
		{ID: SubtabValidation, Label: "Validation"},
		{ID: SubtabRecordTypes, Label: "Record Types"},
		// Below this line: overflow subtabs surfaced via the
		// More… modal. objectDrillPinnedCount marks the split.
		{ID: SubtabTriggers, Label: "Triggers"},
		{ID: SubtabObjectLayouts, Label: "Layouts"},
		{ID: SubtabObjectFlows, Label: "Flows"},
	}
}

func objectDrillSubtabSpecs() []SubtabSpec {
	subs := objectDrillSubtabs()
	specs := make([]SubtabSpec, 0, len(subs))
	for _, sub := range subs {
		spec := SubtabSpec{ID: sub.ID, Label: sub.Label}
		// Wire per-subtab PrimaryFetchedAt so the header age-stamp
		// reflects each subtab's data source — Validation/RecordTypes/
		// Triggers point at their list resource, Records at the chip
		// records, others fall through to the tab-level describe.
		// Help is wired in the same loop for symmetry — each subtab
		// of /object knows its own ? modal.
		switch sub.ID {
		case SubtabSchema:
			spec.Help = func(m Model) infoModalState { return helpFieldsTable() }
		case SubtabDetails:
			spec.Help = func(m Model) infoModalState { return helpObjectDetails() }
		case SubtabFLS:
			spec.Help = func(m Model) infoModalState { return helpObjectFLS() }
		case SubtabRecords:
			spec.Help = func(m Model) infoModalState { return helpRecordsLenses() }
			spec.Open = &objectRecordsOpenSurface
			spec.PrimaryFetchedAt = func(m Model, d *orgData) time.Time {
				if r := currentRecordsResource(d, d.DescribeCur); r != nil {
					return r.FetchedAt()
				}
				return time.Time{}
			}
		case SubtabValidation:
			spec.PrimaryFetchedAt = func(m Model, d *orgData) time.Time {
				if r, ok := d.ValidationRules.Lists[d.DescribeCur]; ok {
					return r.FetchedAt()
				}
				return time.Time{}
			}
		case SubtabRecordTypes:
			spec.PrimaryFetchedAt = func(m Model, d *orgData) time.Time {
				if r, ok := d.RecordTypes.Lists[d.DescribeCur]; ok {
					return r.FetchedAt()
				}
				return time.Time{}
			}
		case SubtabTriggers:
			spec.PrimaryFetchedAt = func(m Model, d *orgData) time.Time {
				if r, ok := d.Triggers.Lists[d.DescribeCur]; ok {
					return r.FetchedAt()
				}
				return time.Time{}
			}
		}
		specs = append(specs, spec)
	}
	return specs
}

// objectDrillPinnedCount is how many entries from objectDrillSubtabs
// occupy fixed strip slots; the rest are reachable through the
// More… modal. Pinned set was chosen by admin-task frequency:
// landing/fields/data/permissions/rules/types are daily-use, the
// remainder (triggers/layouts/flows) are debugging surfaces and
// fine to dig for.
const objectDrillPinnedCount = 6

// homeSubtabs returns the subtab set for the Home tab's details box.
// ORG + API LIMITS are pinned above the box; these populate the box.
//
// Recent is the first subtab — it absorbs the old standalone /recent
// tab. The remaining subtabs (Users, Licenses, Jobs, Deploys, Packages)
// keep their order so users with muscle memory only have to relearn
// one position.
func homeSubtabs() []subtabInfo { return subtabInfosFor(TabHome) }

// apexSubtabs returns the subtab set for /apex — code-shaped
// surfaces. Classes is the default; the rest cover triggers and
// the Visualforce holdovers.
func apexSubtabs() []subtabInfo { return subtabInfosFor(TabApex) }

// componentsSubtabs returns the subtab set for /components — LWC
// (default), Aura, plus UI surfaces that pair with components.
func componentsSubtabs() []subtabInfo { return subtabInfosFor(TabLWC) }

// metaSubtabs returns the subtab set for /meta — admin-metadata hub.
// Most are placeholders today; renderers land per-subtab when needed.
// Order matches the rough mental grouping: data → comms → static →
// auth/integration → other.
// devProjectsSubtabs returns the top-level /dev-projects tab strip:
// Projects (the existing master list) and Bundles (all bundles
// across every DevProject so users can scan disk artifacts without
// drilling into each project first).
func devProjectsSubtabs() []subtabInfo { return subtabInfosFor(TabDevProjects) }

// devProjectDetailSubtabs returns the per-project drill-in tab
// strip: Items (the existing flat tree) and Bundles (sfdx project
// directories linked to this DevProject).
func devProjectDetailSubtabs() []subtabInfo { return subtabInfosFor(TabDevProjectDetail) }

func metaSubtabs() []subtabInfo { return subtabInfosFor(TabMeta) }

// reportsSubtabs returns the subtab set for /reports — Reports
// (folder browser) plus the Dashboards / Report Types list surfaces.
func reportsSubtabs() []subtabInfo { return subtabInfosFor(TabReports) }

// systemSubtabs returns the subtab set for the unified /system tab.
// Logs + Deploys come from existing top-level renderers; API surfaces
// the daily call counter that was previously a Ctrl+A-only modal.
//
// New subtabs (Limits, Async Jobs, etc.) drop in here as their
// renderers land — extension is one entry per subtab.
func systemSubtabs() []subtabInfo { return subtabInfosFor(TabSystem) }

// permsDashboardSubtabs returns the subtab set for the /perms top tab
// — permset/PSG/profile management plus routing/sharing surfaces
// (queues, public groups, sharing rules) that admins reach for in
// the same context.
func permsDashboardSubtabs() []subtabInfo { return subtabInfosFor(TabPerms) }

// permParentDetailSubtabs returns the subtab axes inside a single
// permset / PSG / profile drill-in. kind determines which axes apply:
// "psg" gets an extra Components subtab; non-PSG hides it.
//
// Fields is deliberately NOT a top-level subtab — admins think about
// FLS as "this object's field perms for this permset", not "this
// permset's fields across every object". The Objects subtab drills
// (Enter) into a per-sobject FLS view instead.
func permParentDetailSubtabs(kind string) []subtabInfo {
	subs := []subtabInfo{
		{ID: SubtabParentOverview, Label: "Overview"},
		{ID: SubtabParentObjects, Label: "Objects"},
		{ID: SubtabParentSystem, Label: "System"},
		{ID: SubtabParentUsers, Label: "Users"},
	}
	if kind == "psg" {
		subs = append(subs, subtabInfo{ID: SubtabParentComponents, Label: "Components"})
	}
	return subs
}

// SubtabMoreSentinelID identifies the strip's "0 More…" slot in
// the rendered subtab list. The strip renderer + click resolver
// both special-case this id: the subtab dispatcher never lands on
// it (it opens the overflow modal instead).
const SubtabMoreSentinelID Subtab = "$more"

// subtabPinSplit returns (pinnedCount, totalCount) for the active
// tab. pinnedCount is how many leading subtabs fit on the strip;
// the remainder go into the overflow modal. Most tabs have no
// overflow (pinnedCount == totalCount). The cap lives on
// TabSpec.SubtabPinned — TabObjectDetail's 9 → 6 split is currently
// the only opt-in.
func (m Model) subtabPinSplit() (pinned, total int) {
	all := m.tabSubtabs()
	total = len(all)
	if spec := lookupTabSpec(m.tab()); spec != nil && spec.SubtabPinned > 0 {
		// The user can override how many object-drill subtabs are
		// pinned vs relegated to the More… modal ([ui.layout]
		// object_pinned_subtabs); the spec value is the built-in default.
		pinCap := spec.SubtabPinned
		if m.tab() == TabObjectDetail {
			pinCap = m.settings.LayoutObjectPinnedSubtabs()
		}
		if total >= pinCap {
			return pinCap, total
		}
	}
	return total, total
}

// hasSubtabOverflow reports whether the active tab has subtabs that
// don't fit on the strip and need the More… modal.
func (m Model) hasSubtabOverflow() bool {
	pinned, total := m.subtabPinSplit()
	return total > pinned
}

// tabSubtabsForStrip returns the strip-shaped subtab list: the
// pinned subset followed by a synthetic More… sentinel when
// overflow exists. Callers that render the strip use this; every
// other consumer (cursor dispatch, jump-key table, drill
// resolution) keeps using tabSubtabs() which returns the full
// merged list.
func (m Model) tabSubtabsForStrip() []subtabInfo {
	all := m.tabSubtabs()
	pinned, total := m.subtabPinSplit()
	if pinned >= total {
		return all
	}
	out := make([]subtabInfo, 0, pinned+1)
	out = append(out, all[:pinned]...)
	out = append(out, subtabInfo{ID: SubtabMoreSentinelID, Label: "More…"})
	return out
}

// stripSelectedFor maps a full-list cursor index to the strip's
// equivalent index. Pinned indices map 1:1; an overflow index
// collapses onto the More… slot (last position) so the strip
// highlights "More…" when the user is on an overflow subtab.
func stripSelectedFor(fullIdx int, m Model) int {
	pinned, total := m.subtabPinSplit()
	if pinned >= total {
		return fullIdx
	}
	if fullIdx < pinned {
		return fullIdx
	}
	// More… slot is at position pinned (right after the pinned set).
	return pinned
}

// tabSubtabsOverflow returns the subtabs that don't fit on the
// strip — i.e. tabSubtabs()[pinned:]. Empty for tabs with no
// overflow. Used by the More… modal to populate its picker.
func (m Model) tabSubtabsOverflow() []subtabInfo {
	pinned, total := m.subtabPinSplit()
	if total <= pinned {
		return nil
	}
	all := m.tabSubtabs()
	out := make([]subtabInfo, total-pinned)
	copy(out, all[pinned:])
	return out
}

// tabSubtabs returns the canonical full subtab list for the active
// tab — pinned + overflow merged. Callers that render the strip
// should use tabSubtabsForStrip instead (which collapses overflow
// into a More… sentinel). Cursor dispatch, jump-key table, drill
// resolution all consume the full list as the source of truth.
func (m Model) tabSubtabs() []subtabInfo {
	return m.tabSubtabsRaw()
}

// tabSubtabsRaw is the inner full-list resolver. Three-stage lookup:
//
//  1. TabSpec.SubtabsResolver — for dynamic subtabs (per-bundle,
//     per-perm-parent-kind). Highest precedence so dynamic shapes
//     beat a stale static list.
//  2. TabSpec.Subtabs — static-subtab tabs (most of them).
//  3. Empty single-element fallback for tabs with no subtab story.
//
// Stays private; tabSubtabs is the named entry.
func (m Model) tabSubtabsRaw() []subtabInfo {
	spec := lookupTabSpec(m.tab())
	if spec != nil && spec.SubtabsResolver != nil {
		return spec.SubtabsResolver(m)
	}
	if spec != nil && len(spec.Subtabs) > 0 {
		out := make([]subtabInfo, len(spec.Subtabs))
		for i, s := range spec.Subtabs {
			out[i] = subtabInfo{ID: s.ID, Label: s.Label}
		}
		return out
	}
	return []subtabInfo{{ID: "", Label: ""}}
}

// currentSubtab returns the selected Subtab ID for the current Tab
// (defaults to "" for tabs without subtabs).
//
// Resolution: walk TabSpec.SubtabsResolver / Subtabs to get the list,
// then TabSpec.GetSubtabIdx (required when subtabs exist) for the
// cursor index. The dynamic-shape tabs (TabLWCDetail, TabPermParentDetail)
// register both a SubtabsResolver and a GetSubtabIdx that reads from
// per-tab state — no special-casing needed here.
func (m Model) currentSubtab() Subtab {
	spec := lookupTabSpec(m.tab())
	if spec == nil {
		return ""
	}
	var subs []subtabInfo
	if spec.SubtabsResolver != nil {
		subs = spec.SubtabsResolver(m)
	} else if len(spec.Subtabs) > 0 {
		subs = make([]subtabInfo, len(spec.Subtabs))
		for i, s := range spec.Subtabs {
			subs[i] = subtabInfo{ID: s.ID, Label: s.Label}
		}
	}
	if len(subs) == 0 || spec.GetSubtabIdx == nil {
		return ""
	}
	i := spec.GetSubtabIdx(m)
	if i < 0 || i >= len(subs) {
		i = 0
	}
	return subs[i].ID
}

// renderSubtabStrip draws a horizontal tab-style strip for subtabs.
// Mirrors the top tab bar's rounded-border pills (renderTabPill) so
// the visual language is consistent — "tabs within a tab." Hidden
// when ≤1 subtab is declared.
//
// Pills are joined with lipgloss.JoinHorizontal so multi-row borders
// align cleanly on a single visual row.
func renderSubtabStrip(subs []subtabInfo, selectedIdx, width int) string {
	out, _ := renderSubtabStripLayers(subs, selectedIdx, width)
	return out
}

func renderSubtabStripLayers(subs []subtabInfo, selectedIdx, width int) (string, []*lipgloss.Layer) {
	if len(subs) <= 1 {
		return "", nil
	}
	// When subtabs are visible the number row "shifts down" to them —
	// each pill is labelled with its 1..9 shortcut digit so the user
	// can hit shift+N to jump directly. Top-tab pills lose their digit
	// in renderTabBar to keep the meaning unambiguous.
	//
	// The synthetic "More…" sentinel (last pill on tabs with subtab
	// overflow) gets a 0 prefix to mirror the top-level "0 Tabs"
	// overflow gesture, and a distinct zone id so a click opens the
	// modal instead of selecting it as a real subtab.
	pills := make([]renderedPill, len(subs))
	for i, s := range subs {
		label := s.Label
		switch {
		case s.ID == SubtabMoreSentinelID:
			label = "⇧0 " + s.Label
			pills[i] = renderedPill{
				text: renderSubtabPill(label, i == selectedIdx),
				id:   zoneSubtabOverflow,
			}
			continue
		case i < 9:
			label = subtabDigit(i) + " " + s.Label
		}
		pills[i] = renderedPill{
			text: renderSubtabPill(label, i == selectedIdx),
			id:   zoneSubtabID(i),
		}
	}
	return fitTabRowLayers(pills, selectedIdx, width)
}

// subtabDigit returns the display label for subtab index i. Prefixed
// with the upward-arrow glyph "⇧" so users see at a glance that the
// shortcut requires shift. Single digit since we only address 1..9.
func subtabDigit(i int) string {
	digits := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}
	if i < 0 || i >= len(digits) {
		return " "
	}
	return "⇧" + digits[i]
}

// renderSubtabPill draws one subtab pill. Same shape as renderTabPill
// (rounded border, 0/1 padding) but with a thinner border style and
// muted accent so it reads as subordinate to the top tab bar.
func renderSubtabPill(label string, active bool) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Foreground(theme.Muted)
	if active {
		style = style.
			BorderForeground(theme.Blue).
			Foreground(theme.Fg).
			Bold(true)
	}
	return style.Render(label)
}

// permParentSubtabsResolver feeds TabPermParentDetail's dynamic
// subtab strip: permset / PSG / profile drills expose different
// subtab sets, so the resolver reads the active kind off orgData
// each render rather than freezing one list on the spec (extracted
// in the registry-purity pass).
func permParentSubtabsResolver(m Model) []subtabInfo {
	kind := ""
	if len(m.orgs) > 0 {
		if d, ok := m.data[m.orgs[m.selected].Username]; ok && d != nil {
			kind = d.PermParentKind
		}
	}
	return permParentDetailSubtabs(kind)
}
