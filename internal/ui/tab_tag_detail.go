package ui

// /tag-detail — drilled-in tag view. Lists every item carrying the
// drilled tag across every org, with an auto-generated kind chip
// strip (same shape as /dev-project-detail). Enter on an item drills
// into that item; esc backs out to /tags.
//
// Why this reuses the dev-project plumbing: dev-project items and
// tag bindings are both shaped like devproject.Item — same
// (kind, ref, org_user) key plus optional name/type. So the
// rendering function, the kind chip generator, and the item-open
// path all work as-is. The only thing tag-detail owns is loading
// the right items into m.tagItems.

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// triggerTagDrill is the Activate for /tags. Loads bindings for the
// cursored tag, maps each binding into a devproject.Item, populates
// m.tagItems, sets m.tagCur, and switches to TabTagDetail.
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
	// Proactively kick off resource fetches for any (kind, org) pair
	// whose name lookup missed. The renderer re-resolves names per
	// frame, so as these land the labels light up without needing
	// the user to re-drill.
	return m.ensureNamesForTagItems(items)
}

// ensureNamesForTagItems triggers the per-kind resource fetches
// needed to resolve display names for the loaded tag items. Best-
// effort: returns a batched tea.Cmd of all the .Ensure() calls;
// the renderer's per-frame lookup picks up names as the resources
// land. Skips org/kind combos already in cache.
func (m *Model) ensureNamesForTagItems(items []devproject.Item) tea.Cmd {
	if m.cache == nil {
		return nil
	}
	// Track (orgUser, kind) pairs we've already queued so we don't
	// fan out duplicate fetches for, say, 50 flows on the same org.
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

// bindingsToItems maps tag_bindings rows into the devproject.Item
// shape the renderer expects. tag_bindings don't persist the
// human-readable name (only kind + ref + org_user), so we look it
// up at render time from the org's cached resources where
// available — flows from d.FlowList, apex classes from
// d.ApexClassList, etc. When no cache hit is found (org not loaded
// this session, item not in the cached list), Name stays empty and
// the renderer's standard fallback shows Ref.
//
// Best-effort: if the cache lookup misses, the user sees the id
// instead of the label. They can navigate to the kind's tab to
// load the data, then come back. Future v0.2 work: persist the
// name in tag_bindings at apply time (mirrors how dev-project
// items capture the name on shift+K) so the lookup isn't needed.
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

// lookupItemDisplay returns the human-readable (name, parent) pair
// for an item by walking the org's cached resource lists. orgUser
// scopes the lookup — for cross-org tags this means we only find
// names for items belonging to orgs loaded this session.
//
// Returns ("", "") when nothing matches; the renderer falls back to
// Ref in that case.
func lookupItemDisplay(m Model, kind devproject.ItemKind, ref, orgUser string) (string, string) {
	if orgUser == "" {
		return "", ""
	}
	// Read-only lookup: prefer the existing orgData when present;
	// don't ensure (that mutates). The drill-time ensure took care
	// of allocating + kicking off the fetch.
	d, ok := m.data[orgUser]
	if !ok || d == nil {
		return "", ""
	}
	switch kind {
	case devproject.KindFlow:
		// Pull from the raw Resource not the ListView — the ListView
		// is a sync'd view that may not have fired yet on first
		// load, but the Resource holds the data the moment the
		// fetch lands.
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
		// Ref is the API name; the SObjects list carries Label.
		for _, s := range d.SObjects.Value() {
			if s.Name == ref {
				return s.Label, ""
			}
		}
	case devproject.KindField:
		// Ref is "<sobject>.<field>". Parent is the sobject; the
		// field label needs a describe lookup that may not be
		// loaded, so we just return the parent.
		sobj, _ := splitSObjectField(ref)
		return "", sobj
	}
	return "", ""
}

// matchItemNameOrRef is the ListView.SetMatch comparator. Searches
// by ref + name (when present) — same logic the dev-project items
// view uses.
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

// moveTagDetailCursor is the MoveCursor registry hook. Delegates to
// the ListView so cursor + filter remain consistent across renders.
func (m *Model) moveTagDetailCursor(delta int) {
	m.tagItems.MoveBy(delta)
}

// activateTagDetailItem is the Activate hook for TabTagDetail — Enter
// on a row drills into the item the same way Enter on a dev-project
// item does (switches org if needed + opens the per-kind detail).
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

// tagByID looks up a tag in the store by id. No dedicated DB
// method exists, so we walk ListTags(). Tag rows are few; this is
// cheap.
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

// renderTagDetail is the main-pane renderer for TabTagDetail.
// Composes: heading (tag name + count) → kind chip strip → item
// table → footer hint.
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

	// The kind-chip filter is installed on m.tagItems by
	// applyTagKindFilter on the Update path (cycleTagKindChip /
	// triggerTagDrill) — NOT here. This renderer is a value receiver,
	// so any SetExtra it did would mutate a throwaway copy and the
	// activate / open / yank paths (which read the real ListView)
	// would act on the unfiltered rows. Render just consumes the
	// already-filtered view.

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

	// Org column sizes to the widest displayed label (alias when one
	// exists, username when not) so a couple of long usernames don't
	// stay truncated when most rows are short aliases. Header acts as
	// the floor.
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
			// Re-resolve from org cache per render — the items were
			// populated at drill time when not every org's resource
			// was loaded. Browsing to /flows etc. later loads more,
			// and the name appears on the next paint without
			// needing a re-drill.
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

// cycleTagKindChip moves the kind-filter chip cursor on TabTagDetail
// by delta and applies the resulting kind as the active filter. Wraps
// at both ends, mirrors cycleDevProjectKindChip exactly.
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
		// Walk the order, accounting for chips that were skipped due
		// to zero counts — mirror the dev-project cursor→kind
		// resolver exactly.
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

// applyTagKindFilter installs (or clears) the kind-filter predicate on
// the REAL m.tagItems ListView. This MUST run on the Update path, not
// in the value-receiver renderer — mutating the ListView Extra from a
// render copy is discarded when View returns, which used to leave the
// activate / open / yank paths (all reading the unfiltered Filtered())
// acting on a different row than the highlighted one whenever a kind
// chip was active.
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

// tagKindChips builds the auto-generated kind-filter chip strip.
// Same shape + ordering as devProjectKindChips so the surfaces feel
// identical — only chips with non-zero counts appear, plus a
// leading "All" chip.
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

// sidebarTagDetail renders the right-rail info panel for TabTagDetail.
// Shows the tag's pill, count of items, distinct orgs, and a per-kind
// breakdown — same shape as the /dev-project sidebar.
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
