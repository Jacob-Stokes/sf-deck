package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

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

func (m Model) renderDevProjectDetailItems(dp devproject.DevProject, inner, body int) []string {
	var lines []string

	d := m.activeOrgData()
	if d == nil {
		return lines
	}

	if m.devProjectKindChip != "" {
		active := m.devProjectKindChip
		d.DevProjectItems.SetExtra(func(it devproject.Item) bool {
			return it.Kind == active
		})
	} else {
		d.DevProjectItems.SetExtra(nil)
	}

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

type devProjectRowKind int

const (
	rowKindParent        devProjectRowKind = iota // an sObject (or PSG) that has nested children
	rowKindChild                                  // an item nested under a parent (field/VR/RT/trigger/permset-in-PSG)
	rowKindLeaf                                   // an item with no parent in scope
	rowKindOrphanSObject                          // an sObject with no nested children — render as a plain leaf
	rowKindOrgHeader                              // "all orgs" mode only — section header per org
)

type devProjectRow struct {
	Kind     devProjectRowKind
	Item     devproject.Item // for leaves & children; the parent's own item for parent rows; empty for org headers
	Children int             // count under a parent (used by parent rows)
	Expanded bool            // parent rows only
	Parent   string          // cached parent sObject API name on a child row
	OrgUser  string          // populated on org-header rows in all-orgs mode
}

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
