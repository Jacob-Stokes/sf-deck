package ui

// /dev-projects + drill-in renderers.
//
// /dev-projects        list of every DevProject. Each row shows
//                      aggregate counts (items, distinct orgs).
// /dev-project (detail) the items in one DevProject, filtered to
//                      the active org by default. Tab toggles the
//                      "all orgs" view so users can see the project's
//                      full reach across every org that contributed.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// renderDevProjects draws the cross-org dev-projects list.
func (m Model) renderDevProjects(w, innerH int) string {
	inner := w - 4
	if m.devProjects == nil {
		return theme.Subtle.Render("  dev-projects unavailable (store didn't open)")
	}
	subs := devProjectsSubtabs()
	sel := m.devProjectsSubtab()
	if sel < 0 || sel >= len(subs) {
		sel = 0
	}
	var header []string
	if strip := renderSubtabStrip(subs, sel, inner); strip != "" {
		header = append(header, strip)
	}
	body := innerH - len(header)
	if body < 5 {
		body = 5
	}
	if subs[sel].ID == SubtabDevProjectsBundles {
		return strings.Join(append(header,
			m.renderDevProjectsAllBundles(inner, body)...), "\n")
	}

	var lines []string
	lines = append(lines, header...)
	lines = append(lines,
		headerWithSearchPill(
			fmt.Sprintf("DEV PROJECTS · %d", m.devProjectList.Len()),
			m.devProjectList.Search))
	lines = append(lines, searchBar(m.devProjectList.Search, inner))

	items := m.devProjectList.Filtered()
	if len(items) == 0 {
		switch {
		case m.devProjectList.Search.Applied():
			lines = append(lines, theme.Subtle.Render("  no matches"))
		default:
			lines = append(lines, theme.Subtle.Render("  no dev projects yet"))
			lines = append(lines, dimLine(
				"  press "+firstPretty(Keys.NewProject)+" to create one — or "+firstPretty(Keys.CollectItemPick)+" from any record / sObject / flow to start one with that item",
				inner))
		}
		return strings.Join(lines, "\n")
	}
	rowSel := m.devProjectList.Cursor()
	if rowSel >= len(items) {
		rowSel = 0
	}

	cols := []tableColumn{
		{Header: "NAME", Width: -1, Style: lipgloss.NewStyle().Foreground(theme.Fg)},
		{Header: "ORGS", Width: 6, Style: lipgloss.NewStyle().Foreground(theme.Cyan)},
		{Header: "ITEMS", Width: 7, Style: lipgloss.NewStyle().Foreground(theme.Cyan)},
		{Header: "DESCRIPTION", Width: 40, Style: lipgloss.NewStyle().Foreground(theme.FgDim)},
		{Header: "TOUCHED", Width: 12, Style: lipgloss.NewStyle().Foreground(theme.Muted)},
	}
	lines = append(lines, renderTableHeader(cols, inner))
	lines = append(lines, renderRows(
		len(items), rowSel, innerH, len(lines), 2, inner,
		func(i int) string {
			p := items[i]
			rowCols := make([]tableColumn, len(cols))
			copy(rowCols, cols)
			counts, _ := m.devProjects.CountsForDev(p.ID)
			return renderInteractiveTableRow(rowCols, []string{
				p.Name,
				fmt.Sprintf("%d", counts.Orgs),
				fmt.Sprintf("%d", counts.Items),
				dashIfEmpty(p.Description),
				humanTimeAgo(p.TouchedAt),
			}, i == rowSel, m.focus == focusMain, inner)
		},
	)...)
	lines = append(lines, "", dimLine(
		"  ↵ open · "+firstPretty(Keys.NewProject)+" new · "+firstPretty(Keys.EditProject)+" edit · "+
			firstPretty(Keys.DeleteProject)+" delete · "+firstPretty(Keys.ExportProject)+" bundle · "+
			firstPretty(Keys.LoadOrgProject)+" load · "+firstPretty(Keys.SearchStart)+" search", inner))
	return strings.Join(lines, "\n")
}

// renderDevProjectsAllBundles renders the top-level /dev-projects →
// Bundles subtab. Lists every bundle across every DevProject with
// the parent project's name shown per row.
func (m Model) renderDevProjectsAllBundles(inner, body int) []string {
	bundles, err := m.devProjects.ListAllBundles()
	if err != nil {
		return []string{redLine("  bundles: " + err.Error())}
	}
	var lines []string
	lines = append(lines, sectionTitle("Bundles · "+fmt.Sprintf("%d total", len(bundles))))
	lines = append(lines, dimLine(
		"  every sfdx project directory linked to a DevProject — newest activity first",
		inner))

	if len(bundles) == 0 {
		lines = append(lines, "")
		lines = append(lines, theme.Subtle.Render("  no bundles yet."))
		lines = append(lines, dimLine(
			"  open a DevProject + press "+firstPretty(Keys.ExportProject)+" → \"Bundle: sfdx skeleton + retrieve from org\" to create one.",
			inner))
		return lines
	}

	d := m.activeOrgData()
	cursor := 0
	if d != nil {
		cursor = d.AllBundlesCursor
	}
	if cursor < 0 || cursor >= len(bundles) {
		cursor = 0
	}

	// Build a project-name lookup so each bundle row can label its parent.
	projects, _ := m.devProjects.ListDevProjects()
	nameByID := map[string]string{}
	for _, p := range projects {
		nameByID[p.ID] = p.Name
	}

	reserved := len(lines)
	const trailing = 2
	lines = append(lines, renderRows(
		len(bundles), cursor, body, reserved, trailing, inner,
		func(i int) string {
			return renderBundleRowWithProject(bundles[i],
				nameByID[bundles[i].DevProjectID],
				i == cursor, m.focus == focusMain, inner)
		},
	)...)
	hint := "  ↵ drill in · " + firstPretty(Keys.BundleRetrieve) + " retrieve · " +
		firstPretty(Keys.BundleDeploy) + " deploy · " + firstPretty(Keys.BundleOpen) + " reveal · " +
		firstPretty(Keys.BundleUnlink) + " unlink"
	lines = append(lines, "", dimLine(hint, inner))
	return lines
}

// renderDevProjectDetail draws the items in one dev project. Items
// are filtered to the active org by default; Tab toggles between
// "this org" and "all orgs" so the user can see the project's full
// cross-org reach when they want.
//
// Items group by parent sObject when one is in scope. So if the
// project contains "Account" + "Account.Phone" + "Account.IsActive
// validation rule", they collapse under one Account header that the
// user can expand/collapse. Items without a parent (records, flows,
// permsets, profiles, reports, orphan fields whose sObject isn't
// itself collected) sit in flat per-kind groups underneath. In "all
// orgs" mode, parents are keyed by (org, sObject) so the same
// sObject collected from two different orgs appears as two
// independent folders.
//
// Cursor walks every selectable row. Right / l / enter on a parent
// toggles expand. Left / h on a child jumps cursor up to its parent.
// d removes the cursored row's item from the project.
func (m Model) renderDevProjectDetail(w, innerH int) string {
	inner := w - 4
	if m.devProjects == nil {
		return theme.Subtle.Render("  dev-projects unavailable")
	}
	if m.devProjectCur == "" {
		return theme.Subtle.Render("  no dev project drilled in")
	}
	dp, ok := m.devProjectByID(m.devProjectCur)
	if !ok {
		return theme.Subtle.Render("  dev project not found (deleted?)")
	}

	subs := devProjectDetailSubtabs()
	sel := m.devProjectDetailSubtab()
	if sel < 0 || sel >= len(subs) {
		sel = 0
	}

	var header []string
	header = append(header, sectionTitle(dp.Name))
	if dp.Description != "" {
		header = append(header, dimLine("  "+dp.Description, inner))
	}
	if strip := renderSubtabStrip(subs, sel, inner); strip != "" {
		header = append(header, strip)
	}

	body := innerH - len(header)
	if body < 5 {
		body = 5
	}

	switch subs[sel].ID {
	case SubtabDevProjectBundles:
		return strings.Join(append(header, m.renderDevProjectDetailBundles(dp, inner, body)...), "\n")
	default:
		return strings.Join(append(header, m.renderDevProjectDetailItems(dp, inner, body)...), "\n")
	}
}

// renderDevProjectDetailItems renders the Items subtab — the
// existing per-project flat tree of collected sObjects, fields,
// flows, etc. Body of what used to be the whole tab renderer.
func (m Model) renderDevProjectDetailItems(dp devproject.DevProject, inner, body int) []string {
	var lines []string

	d := m.activeOrgData()
	if d == nil {
		return lines
	}

	// Apply the active kind-chip filter via the ListView Extra hook so
	// the unified list engine sees a single canonical set of rows.
	if m.devProjectKindChip != "" {
		active := m.devProjectKindChip
		d.DevProjectItems.SetExtra(func(it devproject.Item) bool {
			return it.Kind == active
		})
	} else {
		d.DevProjectItems.SetExtra(nil)
	}

	// Kind-filter chip strip. Built from the FULL item set so the
	// strip is stable across the active filter — switching from
	// "Flows" to "All" mustn't reset which chips exist. The strip
	// only renders when there's >1 distinct kind, since a single-
	// kind project doesn't need filtering.
	chips, chipSel := m.devProjectKindChips()
	if len(chips) > 2 {
		if strip := renderChipStrip(chips, chipSel, inner, ""); strip != "" {
			lines = append(lines, strip)
		}
	}

	scope := "this org"
	if m.devProjectShowAllOrgs {
		scope = "all orgs"
	}
	visible := d.DevProjectItems.Len()
	total := len(d.DevProjectItems.Items())
	header := fmt.Sprintf("  %d items · %s · touched %s", visible, scope, humanTimeAgo(dp.TouchedAt))
	if visible != total {
		header = fmt.Sprintf("  %d of %d items · %s · touched %s", visible, total, scope, humanTimeAgo(dp.TouchedAt))
	}
	lines = append(lines, dimLine(header, inner))

	if total == 0 {
		lines = append(lines, "")
		if m.devProjectShowAllOrgs {
			lines = append(lines, theme.Subtle.Render(
				"  no items in this dev project yet"))
		} else {
			lines = append(lines, theme.Subtle.Render(
				"  no items from this org yet"))
			lines = append(lines, dimLine(
				"  Tab to see items from other orgs · or navigate elsewhere + shift+K to add",
				inner))
		}
		return lines
	}
	if visible == 0 {
		lines = append(lines, "")
		lines = append(lines, theme.Subtle.Render(
			"  no items in this view · press [ or ] to switch filter"))
		return lines
	}

	// Delegate the actual table render to the shared listSurface
	// machinery — sort, scroll, search, persisted widths, everything
	// the other surfaces get.
	model, ok := devProjectItemsListSurface.BuildRenderModel(m, d)
	if !ok {
		return lines
	}
	usedAbove := len(lines)
	budget := body - usedAbove
	lines = append(lines, renderListModel(m, model, m.focus, inner, budget)...)

	viewKeys := firstPretty(Keys.PrevView) + " or " + firstPretty(Keys.NextView)
	hint := "  ↵ open · " + firstPretty(Keys.DeleteProject) + " remove · " +
		firstPretty(Keys.ExportProject) + " bundle · " + viewKeys + " filter · " +
		firstPretty(Keys.ToggleSidebar) + " toggle scope · esc back"
	// Surface a one-time hint about the managed badge when any item
	// in view has one.
	for _, it := range d.DevProjectItems.Items() {
		if it.Managed() {
			hint = "  ↵ open · " + firstPretty(Keys.DeleteProject) + " remove · " +
				firstPretty(Keys.ExportProject) + " bundle · " + viewKeys + " filter · " +
				firstPretty(Keys.ToggleSidebar) + " scope · " +
				lipgloss.NewStyle().Foreground(theme.Yellow).Render("[ns]") +
				" = managed package · esc back"
			break
		}
	}
	lines = append(lines, "", dimLine(hint, inner))
	return lines
}

// devProjectKindChipOrder is the fixed display order for the Items
// subtab's kind-filter chips. Items the project DOESN'T contain are
// dropped at chip-build time; this order just pins what's where when
// they ARE present, so the strip doesn't reorder on add/remove.
var devProjectKindChipOrder = []struct {
	Kind  devproject.ItemKind
	Label string
}{
	{devproject.KindSObject, "Objects"},
	{devproject.KindField, "Fields"},
	{devproject.KindValidationRule, "Validation rules"},
	{devproject.KindRecordType, "Record types"},
	{devproject.KindRecord, "Records"},
	{devproject.KindFlow, "Flows"},
	{devproject.KindApexClass, "Apex"},
	{devproject.KindApexTrigger, "Triggers"},
	{devproject.KindLWC, "LWC"},
	{devproject.KindAura, "Aura"},
	{devproject.KindReport, "Reports"},
	{devproject.KindPermissionSet, "Permsets"},
	{devproject.KindPermissionSetGroup, "Permset groups"},
	{devproject.KindProfile, "Profiles"},
	{devproject.KindQueue, "Queues"},
	{devproject.KindPublicGroup, "Public groups"},
	{devproject.KindSOQLQuery, "SOQL"},
	{devproject.KindApexSnippet, "Apex snippets"},
}

// devProjectKindChips builds the Items-subtab chip strip from the
// currently-loaded m.devProjectItems. Returns the chip rows + the
// cursor index to render. Kinds with zero items are omitted so the
// cursor space is dense; "All" is always at index 0.
//
// When m.devProjectKindChip has fallen off the strip (e.g. all items
// of that kind were just removed), the returned cursor snaps to 0.
func (m Model) devProjectKindChips() ([]chipRow, int) {
	items := m.devProjectItemsView()
	if len(items) == 0 {
		return nil, 0
	}
	counts := map[devproject.ItemKind]int{}
	for _, it := range items {
		counts[it.Kind]++
	}
	chips := []chipRow{{ID: "all", Label: "All", Count: len(items)}}
	cursor := 0
	for _, spec := range devProjectKindChipOrder {
		n := counts[spec.Kind]
		if n == 0 {
			continue
		}
		if m.devProjectKindChip == spec.Kind {
			cursor = len(chips)
		}
		chips = append(chips, chipRow{
			ID:    string(spec.Kind),
			Label: spec.Label,
			Count: n,
		})
	}
	return chips, cursor
}

// cycleDevProjectKindChip moves the kind-filter chip cursor on the
// Items subtab by delta and applies the resulting kind as the active
// filter. Wraps at both ends (matches the qchip cycler convention).
// Resets the item cursor so we land on a real row after the filter
// shrinks (or grows) the visible set.
func (m Model) cycleDevProjectKindChip(delta int) (Model, tea.Cmd) {
	chips, cur := m.devProjectKindChips()
	if len(chips) == 0 {
		return m, nil
	}
	next := (cur + delta) % len(chips)
	if next < 0 {
		next += len(chips)
	}
	kind, ok := m.devProjectKindChipFromIdx(next)
	if !ok {
		return m, nil
	}
	m.devProjectKindChip = kind
	m.devProjectKindChipCursor = next
	m.resetDevProjectItemCursor()
	return m, nil
}

// resetDevProjectItemCursor drops the item cursor for the active
// project back to 0. Called whenever the visible row set changes
// shape (kind-chip cycle, scope toggle, item removal) so the cursor
// doesn't point past the end.
func (m *Model) resetDevProjectItemCursor() {
	if len(m.orgs) == 0 || m.devProjectCur == "" {
		return
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil {
		return
	}
	d.Cursors.Set(cursorKindDevProjectItem, 0, 0, m.devProjectCur)
}

// devProjectKindChipFromIdx maps a chip-strip index back to the
// ItemKind that index represents. Index 0 → "" (All); higher indices
// look up the same chip list devProjectKindChips emitted. Returns
// ("", false) when idx is out of range.
func (m Model) devProjectKindChipFromIdx(idx int) (devproject.ItemKind, bool) {
	chips, _ := m.devProjectKindChips()
	if idx < 0 || idx >= len(chips) {
		return "", false
	}
	if idx == 0 {
		return "", true
	}
	return devproject.ItemKind(chips[idx].ID), true
}

// renderDevProjectDetailBundles renders the Bundles subtab — sfdx
// project directories linked to this DevProject. Reuses the
// /bundles list rendering by pulling the same bundle slice.
func (m Model) renderDevProjectDetailBundles(dp devproject.DevProject, inner, body int) []string {
	bundles, err := m.devProjects.ListBundlesFor(dp.ID)
	if err != nil {
		return []string{redLine("  bundles: " + err.Error())}
	}

	var lines []string
	lines = append(lines, dimLine(
		fmt.Sprintf("  %d bundle(s) on disk linked to this project", len(bundles)),
		inner))

	if len(bundles) == 0 {
		lines = append(lines, "")
		lines = append(lines, theme.Subtle.Render(
			"  no bundles yet."))
		lines = append(lines, dimLine(
			"  press "+firstPretty(Keys.ExportProject)+" and pick \"Bundle: sfdx skeleton + retrieve from org\" to create one.",
			inner))
		return lines
	}

	cursor := m.bundleCursor(len(bundles))
	if cursor < 0 || cursor >= len(bundles) {
		cursor = 0
	}

	reserved := len(lines)
	const trailing = 2
	lines = append(lines, renderRows(
		len(bundles), cursor, body, reserved, trailing, inner,
		func(i int) string {
			return renderBundleRow(bundles[i], i == cursor, m.focus == focusMain, inner)
		},
	)...)
	hint := "  ↵ drill in · " + firstPretty(Keys.BundleRetrieve) + " retrieve · " +
		firstPretty(Keys.BundleDeploy) + " deploy · " + firstPretty(Keys.BundleOpen) + " reveal · " +
		firstPretty(Keys.BundleUnlink) + " unlink"
	lines = append(lines, "", dimLine(hint, inner))
	return lines
}

// devProjectRowKind discriminates the row types in the items tree.
type devProjectRowKind int

const (
	rowKindParent        devProjectRowKind = iota // an sObject (or PSG) that has nested children
	rowKindChild                                  // an item nested under a parent (field/VR/RT/trigger/permset-in-PSG)
	rowKindLeaf                                   // an item with no parent in scope
	rowKindOrphanSObject                          // an sObject with no nested children — render as a plain leaf
	rowKindOrgHeader                              // "all orgs" mode only — section header per org
)

// devProjectRow is one row in the flattened items tree.
type devProjectRow struct {
	Kind     devProjectRowKind
	Item     devproject.Item // for leaves & children; the parent's own item for parent rows; empty for org headers
	Children int             // count under a parent (used by parent rows)
	Expanded bool            // parent rows only
	Parent   string          // cached parent sObject API name on a child row
	OrgUser  string          // populated on org-header rows in all-orgs mode
}

// expandCursoredDevProjectRow / collapseCursoredDevProjectRow are
// retained as stubs so update_keys.go's → / ← dispatch can keep
// calling them — both return handled=false which lets the default
// arrow-cursor handler take over. The previous hierarchical tree
// had per-parent expand state; the flat list-table view doesn't
// need it.
func (m Model) expandCursoredDevProjectRow() (Model, bool) {
	return m, false
}

func (m Model) collapseCursoredDevProjectRow() (Model, bool) {
	return m, false
}

// rowAtCursor returns the dev-project item under the cursor on the
// Items subtab. Backed by the unified ListView: cursor + filtered
// set both live in d.DevProjectItems, so this is just "look up the
// cursored Item and wrap it." The returned shape is the legacy
// devProjectRow so the existing dispatch sites (Enter handler, d
// remover, x exporter) don't need to change yet.
func (m Model) rowAtCursor() (devProjectRow, int, bool) {
	d := m.activeOrgData()
	if d == nil {
		return devProjectRow{}, 0, false
	}
	items := d.DevProjectItems.Filtered()
	if len(items) == 0 {
		return devProjectRow{}, 0, false
	}
	idx := d.DevProjectItems.Cursor()
	if idx < 0 || idx >= len(items) {
		return devProjectRow{}, 0, false
	}
	return devProjectRow{
		Kind: rowKindLeaf,
		Item: items[idx],
	}, idx, true
}
