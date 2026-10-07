package ui

// /tag-detail — drilled-in tag view. Lists every item carrying the
// drilled tag across every org, with an auto-generated kind chip
// strip (same shape as /dev-project-detail). Enter on an item drills
// into that item; esc backs out to /tags.

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func (m *Model) triggerTagDrill() tea.Cmd {
	if m.devProjects == nil {
		return nil
	}
	// Auto-reconcile tag bindings on drill-in: prune bindings on
	// resources deleted in their org, and normalise any non-canonical
	// refs. Safe no-op on loaded/clean data (same oracle + safety rule
	// as the dev-project reconcile).
	m.reconcileTags()
	tags, err := m.devProjects.ListTagsWithUsage()
	if err != nil || len(tags) == 0 {
		return nil
	}
	idx := m.tagsCursor
	if idx >= len(tags) {
		return nil
	}
	t := tags[idx]
	bindings, err := m.devProjects.ItemsWithTag(t.ID, "")
	if err != nil {
		m.flash("load tag bindings: " + err.Error())
		return nil
	}
	items := bindingsToItems(*m, bindings)
	m.tagItems.Set(items)
	m.tagItems.SetMatch(matchItemNameOrRef)
	m.tagItems.SetExtra(nil)
	m.tagItems.SetCursor(0)
	m.tagCur = t.ID
	m.tagKindChip = ""
	m.tagKindChipCursor = 0
	m.setTab(TabTagDetail)
	return m.ensureNamesForTagItems(items)
}

func (m *Model) ensureNamesForTagItems(items []devproject.Item) tea.Cmd {
	if m.cache == nil {
		return nil
	}
	seen := map[string]bool{}
	var cmds []tea.Cmd
	for _, it := range items {
		if it.Name != "" || it.OrgUser == "" {
			continue
		}
		key := it.OrgUser + "|" + string(it.Kind)
		if seen[key] {
			continue
		}
		seen[key] = true
		// ensureOrgData lazily allocates the per-org state for orgs
		// the user hasn't visited this session. Without this, tags
		// pointing at items in unvisited orgs never get their
		// resources fetched and stay as raw ids.
		d := m.ensureOrgData(it.OrgUser)
		if d == nil {
			continue
		}
		switch it.Kind {
		case devproject.KindFlow:
			cmds = append(cmds, d.Flows.Ensure(m.cache))
		case devproject.KindApexClass:
			cmds = append(cmds, d.ApexClasses.Ensure(m.cache))
		case devproject.KindApexTrigger:
			cmds = append(cmds, d.ApexTriggersFlat.Ensure(m.cache))
		case devproject.KindLWC:
			cmds = append(cmds, d.LWCBundles.Ensure(m.cache))
		case devproject.KindAura:
			cmds = append(cmds, d.AuraBundles.Ensure(m.cache))
		case devproject.KindSObject:
			cmds = append(cmds, d.SObjects.Ensure(m.cache))
		case devproject.KindReport:
			cmds = append(cmds, d.Reports.Ensure(m.cache))
		case devproject.KindPermissionSet:
			cmds = append(cmds, d.PermSets.Ensure(m.cache))
		case devproject.KindProfile:
			cmds = append(cmds, d.Profiles.Ensure(m.cache))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func bindingsToItems(m Model, bs []devproject.Binding) []devproject.Item {
	out := make([]devproject.Item, 0, len(bs))
	for _, b := range bs {
		name, parent := lookupItemDisplay(m, b.ItemKind, b.ItemRef, b.OrgUser)
		out = append(out, devproject.Item{
			OrgUser: b.OrgUser,
			Kind:    b.ItemKind,
			Ref:     b.ItemRef,
			Name:    name,
			Type:    parent,
			AddedAt: b.CreatedAt,
		})
	}
	return out
}

func lookupItemDisplay(m Model, kind devproject.ItemKind, ref, orgUser string) (string, string) {
	if orgUser == "" {
		return "", ""
	}
	d, ok := m.data[orgUser]
	if !ok || d == nil {
		return "", ""
	}
	switch kind {
	case devproject.KindFlow:
		for _, f := range d.Flows.Value() {
			if f.DefinitionID == ref {
				if f.MasterLabel != "" {
					return f.MasterLabel, ""
				}
				return f.DeveloperName, ""
			}
		}
	case devproject.KindApexClass:
		for _, c := range d.ApexClasses.Value() {
			if c.ID == ref {
				return c.Name, ""
			}
		}
	case devproject.KindApexTrigger:
		for _, t := range d.ApexTriggersFlat.Value() {
			if t.ID == ref {
				return t.Name, t.Table
			}
		}
	case devproject.KindLWC:
		for _, l := range d.LWCBundles.Value() {
			if l.ID == ref {
				if l.MasterLabel != "" {
					return l.MasterLabel, ""
				}
				return l.DeveloperName, ""
			}
		}
	case devproject.KindSObject:
		for _, s := range d.SObjects.Value() {
			if s.Name == ref {
				return s.Label, ""
			}
		}
	case devproject.KindField:
		sobj, _ := splitSObjectField(ref)
		return "", sobj
	}
	return "", ""
}

func matchItemNameOrRef(it devproject.Item, q string) bool {
	q = strings.ToLower(q)
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(it.Ref), q) {
		return true
	}
	if strings.Contains(strings.ToLower(it.Name), q) {
		return true
	}
	return false
}

func (m *Model) moveTagDetailCursor(delta int) {
	m.tagItems.MoveBy(delta)
}

func (m *Model) activateTagDetailItem() tea.Cmd {
	rows := m.tagItems.Filtered()
	if len(rows) == 0 {
		return nil
	}
	cur := m.tagItems.Cursor()
	if cur >= len(rows) {
		return nil
	}
	return m.openItemForOrigin(rows[cur], TabTagDetail)
}

// orgDisplayForUsername resolves a stored org_user (the Salesforce
// username) into the user-facing label — alias when one exists,
// username when not. Mirrors what the left rail + safety pill use,
// so the same org reads the same on every surface. Falls back to
// the raw username for orgs we don't currently know about (logged
// out, never authed from this machine).
func (m Model) orgDisplayForUsername(username string) string {
	if username == "" {
		return ""
	}
	for _, o := range m.orgs {
		if o.Username == username {
			if label := o.Display(); label != "" {
				return label
			}
			return username
		}
	}
	return username
}

func (m Model) tagByID(id int64) (devproject.Tag, bool) {
	if m.devProjects == nil {
		return devproject.Tag{}, false
	}
	tags, err := m.devProjects.ListTags()
	if err != nil {
		return devproject.Tag{}, false
	}
	for _, t := range tags {
		if t.ID == id {
			return t, true
		}
	}
	return devproject.Tag{}, false
}

func (m Model) renderTagDetail(w, innerH int) string {
	inner := w - 4
	if m.devProjects == nil || m.tagCur == 0 {
		return theme.Subtle.Render("  no tag drilled in")
	}
	tag, ok := m.tagByID(m.tagCur)
	if !ok {
		return theme.Subtle.Render("  tag no longer exists — esc back")
	}

	var lines []string
	title := "TAG · " + tag.Name
	if tag.Icon != "" {
		title = "TAG · " + tag.Icon + " " + tag.Name
	}
	lines = append(lines, sectionTitle(title))

	body := innerH - len(lines)
	if body < 5 {
		body = 5
	}
	lines = append(lines, m.renderTagDetailItems(tag, inner, body)...)
	return strings.Join(lines, "\n")
}

// renderTagDetailItems is the kind-chip + table block for TabTagDetail.
// Mirrors renderDevProjectDetailItems intentionally so users get the
// same affordances on both surfaces.
func (m Model) renderTagDetailItems(tag devproject.Tag, inner, body int) []string {
	var lines []string

	chips, chipSel := m.tagKindChips()
	if len(chips) > 2 {
		if strip := renderChipStrip(chips, chipSel, inner, ""); strip != "" {
			lines = append(lines, strip)
		}
	}

	visible := m.tagItems.Len()
	total := len(m.tagItems.Items())
	header := fmt.Sprintf("  %d items · across all orgs", visible)
	if visible != total {
		header = fmt.Sprintf("  %d of %d items · across all orgs", visible, total)
	}
	lines = append(lines, dimLine(header, inner))

	if total == 0 {
		lines = append(lines, "")
		lines = append(lines, theme.Subtle.Render(
			"  no items carry this tag yet — press "+firstPretty(Keys.Tag)+" on any item to tag it"))
		return lines
	}
	if visible == 0 {
		lines = append(lines, "")
		lines = append(lines, theme.Subtle.Render(
			"  no items in this view · press [ or ] to switch filter"))
		return lines
	}

	rows := m.tagItems.Filtered()
	cursor := m.tagItems.Cursor()
	if cursor >= len(rows) {
		cursor = 0
	}

	orgLabels := make([]string, len(rows))
	orgW := lipgloss.Width("ORG")
	for i, it := range rows {
		orgLabels[i] = m.orgDisplayForUsername(it.OrgUser)
		if w := lipgloss.Width(orgLabels[i]); w > orgW {
			orgW = w
		}
	}

	cols := []tableColumn{
		{Header: "KIND", Width: 14, Style: lipgloss.NewStyle().Foreground(theme.Cyan)},
		{Header: "NAME / REF", Width: -1, Style: lipgloss.NewStyle().Foreground(theme.Fg)},
		{Header: "ORG", Width: orgW, Style: lipgloss.NewStyle().Foreground(theme.Muted)},
		{Header: "ADDED", Width: 12, Style: lipgloss.NewStyle().Foreground(theme.FgDim)},
	}
	lines = append(lines, renderTableHeader(cols, inner))

	headerUsed := len(lines)
	budget := body - headerUsed
	if budget < 1 {
		budget = 1
	}
	lines = append(lines, renderRows(
		len(rows), cursor, budget, 0, 2, inner,
		func(i int) string {
			it := rows[i]
			name := it.Name
			if name == "" {
				if n, _ := lookupItemDisplay(m, it.Kind, it.Ref, it.OrgUser); n != "" {
					name = n
				}
			}
			if name == "" {
				name = it.Ref
			}
			cells := []string{
				string(it.Kind),
				name,
				orgLabels[i],
				humanAge(it.AddedAt),
			}
			return renderInteractiveTableRow(cols, cells, i == cursor, m.focus == focusMain, inner)
		},
	)...)

	lines = append(lines, "", dimLine(
		"  ↵ open · [ or ] filter kind · esc back", inner))
	_ = tag
	return lines
}

func (m Model) cycleTagKindChip(delta int) (Model, tea.Cmd) {
	chips, cur := m.tagKindChips()
	if len(chips) == 0 {
		return m, nil
	}
	next := (cur + delta) % len(chips)
	if next < 0 {
		next += len(chips)
	}
	if next == 0 {
		m.tagKindChip = ""
	} else if next-1 < len(devProjectKindChipOrder) {
		items := m.tagItems.Items()
		counts := map[devproject.ItemKind]int{}
		for _, it := range items {
			counts[it.Kind]++
		}
		visIdx := 0
		for _, ord := range devProjectKindChipOrder {
			if counts[ord.Kind] == 0 {
				continue
			}
			visIdx++
			if visIdx == next {
				m.tagKindChip = ord.Kind
				break
			}
		}
	}
	m.tagKindChipCursor = next
	m.applyTagKindFilter()
	m.tagItems.SetCursor(0)
	return m, nil
}

// applyTagKindFilter installs (or clears) the kind-filter predicate on the
// real m.tagItems ListView. This must run on Update because mutations to a
// value-receiver render copy are discarded when View returns.
func (m *Model) applyTagKindFilter() {
	if m.tagKindChip == "" {
		m.tagItems.SetExtra(nil)
		return
	}
	active := m.tagKindChip
	m.tagItems.SetExtra(func(it devproject.Item) bool {
		return it.Kind == active
	})
}

func (m Model) tagKindChips() ([]chipRow, int) {
	items := m.tagItems.Items()
	counts := map[devproject.ItemKind]int{}
	for _, it := range items {
		counts[it.Kind]++
	}
	chips := []chipRow{
		{ID: "__all__", Label: "All", Count: len(items)},
	}
	// sel is the index into the VISIBLE chips slice, not into
	// devProjectKindChipOrder. Track the visible position as we
	// append; the order-index isn't usable because zero-count kinds
	// are skipped, so order-index and visible-index diverge as soon
	// as the loaded items don't cover every kind.
	sel := 0
	for _, ord := range devProjectKindChipOrder {
		if c := counts[ord.Kind]; c > 0 {
			chips = append(chips, chipRow{
				ID:    string(ord.Kind),
				Label: ord.Label,
				Count: c,
			})
			if m.tagKindChip == ord.Kind {
				sel = len(chips) - 1
			}
		}
	}
	return chips, sel
}

func (m Model) sidebarTagDetail(inner int) string {
	if m.devProjects == nil || m.tagCur == 0 {
		return sideEmpty("no tag")
	}
	tag, ok := m.tagByID(m.tagCur)
	if !ok {
		return sideEmpty("tag gone")
	}

	items := m.tagItems.Items()
	byKind := map[devproject.ItemKind]int{}
	byOrg := map[string]int{}
	for _, it := range items {
		byKind[it.Kind]++
		if it.OrgUser != "" {
			byOrg[it.OrgUser]++
		}
	}

	rows := []kv{
		{"name", tag.Name},
		{"color", dashIfEmpty(tag.Color)},
		{"icon", dashIfEmpty(tag.Icon)},
		{"items", fmt.Sprintf("%d", len(items))},
		{"orgs", fmt.Sprintf("%d", len(byOrg))},
	}

	var extra []string
	if len(byKind) > 0 {
		kinds := make([]string, 0, len(byKind))
		for k := range byKind {
			kinds = append(kinds, string(k))
		}
		sort.Strings(kinds)
		extra = append(extra, "", sideSection("by kind"))
		for _, k := range kinds {
			extra = append(extra, sideKV(k, fmt.Sprintf("%d", byKind[devproject.ItemKind(k)]), inner))
		}
	}
	extra = append(extra, "", sideDim("  ↵ open · esc back", inner))

	title := tag.Name
	if tag.Icon != "" {
		title = tag.Icon + " " + title
	}
	return renderKVPanel(inner, title, rows, extra...)
}
