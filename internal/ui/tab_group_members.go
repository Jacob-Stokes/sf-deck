package ui

// tab_group_members.go — TabQueueDetail + TabPublicGroupDetail.
//
// Both surfaces show the resolved member list of a Group sObject
// (Queues are stored as Group rows with Type='Queue'; Public
// Groups are Group rows with Type='Regular'). The render shape is
// identical so they share a single body — the only difference is
// the parent kind label in the title and which list-resource is
// fed into the renderer.
//
// Drill on a member row opens that User or nested Group's
// Lightning record (handled by GroupMemberRow.Targets).

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// renderQueueDetail draws the queue-members detail. Wraps the
// shared body with a queue-flavoured title.
func (m Model) renderQueueDetail(w, innerH int) string {
	return m.renderGroupMembersDetail(w, innerH, "Queue")
}

// renderPublicGroupDetail draws the public-group-members detail.
func (m Model) renderPublicGroupDetail(w, innerH int) string {
	return m.renderGroupMembersDetail(w, innerH, "Public Group")
}

// renderGroupMembersDetail is the shared body used by both queue
// and public-group detail surfaces.
func (m Model) renderGroupMembersDetail(w, innerH int, parentLabel string) string {
	inner := w - 4
	d := m.activeOrgData()
	if d == nil {
		return noOrgPlaceholder()
	}
	if d.GroupMemberID == "" {
		return theme.Subtle.Render("  no group drilled in")
	}
	res := d.GroupMembers[d.GroupMemberID]
	if res == nil {
		return theme.Subtle.Render("  members not loaded — press r")
	}

	// Title + parent metadata (resolved from the parent list cache,
	// since member rows themselves don't carry the parent's name).
	parentName := groupParentName(d, d.GroupMemberKind, d.GroupMemberID)
	title := parentLabel + " · " + parentName
	if parentName == "" {
		title = parentLabel + " · " + d.GroupMemberID
	}

	var lines []string
	// Blank top line — the global subtab-strip hit-layer paints
	// over Y=1 on every tab whose tabSubtabs() returns >1 entries.
	// We don't have subtabs here, but the layer skips when
	// len(subs)<=1 so the blank line stays available without
	// double-up risk.
	lines = append(lines, "")
	lines = append(lines, sectionTitle(title))

	if res.FetchedAt().IsZero() {
		if res.Busy() {
			lines = append(lines, dimLine("  loading members…", inner))
		} else {
			lines = append(lines, dimLine("  press "+firstPretty(Keys.Refresh)+" to load", inner))
		}
		return strings.Join(lines, "\n")
	}
	if err := res.Err(); err != nil {
		lines = append(lines, redLine("  members query failed: "+err.Error()))
		return strings.Join(lines, "\n")
	}

	// Sync the ListView with the latest payload. Set every render
	// is fine here (cursor is owned by the ListView; the data
	// rarely changes).
	lv, ok := d.GroupMemberList[d.GroupMemberID]
	if !ok {
		lv = ListView[sf.GroupMemberRow]{}
		lv.SetMatch(makeGroupMemberMatcher())
	}
	lv.Set(res.Value())
	d.GroupMemberList[d.GroupMemberID] = lv

	state, ok := d.GroupMemberState[d.GroupMemberID]
	if !ok {
		state = &uilayout.ListTableState{}
		d.GroupMemberState[d.GroupMemberID] = state
	}

	resolved := mustResolveColumns(groupMemberColumnSchema())
	cols := resolved.ListColumns()
	installListViewOrderRows(&lv, state, cols,
		func(items []sf.GroupMemberRow, row, col int) string {
			return resolvedSortCellForListColumn(resolved, items, cols, row, col)
		})
	items := lv.Filtered()
	d.GroupMemberList[d.GroupMemberID] = lv
	spec := uilayout.ListTableSpec{
		Cols: cols,
		N:    len(items),
		Cell: func(row, col int) string {
			if row < 0 || row >= len(items) {
				return ""
			}
			return resolvedCellForListColumn(resolved, items, cols, row, col)
		},
	}

	model := listRenderModel{
		Title:       fmt.Sprintf("MEMBERS · %d", lv.Len()),
		State:       state,
		Search:      lv.SearchPtr(),
		Cols:        cols,
		N:           spec.N,
		Cursor:      lv.Cursor(),
		Cell:        spec.Cell,
		Empty:       "  no members on this " + strings.ToLower(parentLabel),
		DataVersion: listVersionWithStore(lv.Version(), m),
	}
	usedAbove := usedLines(lines)
	budget := innerH - usedAbove
	rest := renderListModel(m, model, m.focus, inner, budget)
	lines = append(lines, rest...)
	return strings.Join(lines, "\n")
}

func makeGroupMemberMatcher() func(sf.GroupMemberRow, string) bool {
	return uilayout.MakeMatcher(uilayout.MatchSpec[sf.GroupMemberRow]{
		Any: func(r sf.GroupMemberRow) string {
			return strings.ToLower(r.Name + " " + r.Email + " " + r.ID + " " + r.Kind)
		},
		Field: func(r sf.GroupMemberRow, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(r.Name)
			case "Email":
				return strings.ToLower(r.Email)
			case "Id":
				return strings.ToLower(r.ID)
			case "Kind":
				return strings.ToLower(r.Kind)
			}
			return ""
		},
		Fields: []string{"Name", "Email", "Id", "Kind"},
	})
}

// groupParentName resolves the parent Queue / PublicGroup's display
// name from the cached list on /perms. Falls back to "" when the
// list hasn't been fetched yet (the title renderer falls back to
// the id in that case).
func groupParentName(d *orgData, kind, id string) string {
	switch kind {
	case "queue":
		for _, q := range d.QueueList.Items() {
			if q.ID == id {
				return q.Name
			}
		}
	case "public_group":
		for _, g := range d.PublicGroupList.Items() {
			if g.ID == id {
				return g.Name
			}
		}
	}
	return ""
}

// activateQueue is the drill handler for /perms Queues. Sets the
// drill-state and switches to TabQueueDetail.
func (m *Model) activateQueue() tea.Cmd {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	q, ok := d.QueueList.Selected()
	if !ok {
		return nil
	}
	d.GroupMemberKind = "queue"
	d.GroupMemberID = q.ID
	if s := d.QueueList.SearchPtr(); s.Active {
		s.Active = false
		s.Committed = s.Buffer() != ""
	}
	m.setTab(TabQueueDetail)
	return m.onTabChanged()
}

// activatePublicGroup mirrors activateQueue for public groups.
func (m *Model) activatePublicGroup() tea.Cmd {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	g, ok := d.PublicGroupList.Selected()
	if !ok {
		return nil
	}
	d.GroupMemberKind = "public_group"
	d.GroupMemberID = g.ID
	if s := d.PublicGroupList.SearchPtr(); s.Active {
		s.Active = false
		s.Committed = s.Buffer() != ""
	}
	m.setTab(TabPublicGroupDetail)
	return m.onTabChanged()
}

// ensureGroupMembersData / refreshGroupMembersData serve BOTH
// TabQueueDetail and TabPublicGroupDetail — the drills share
// d.GroupMemberID and the members fetch (extracted in the
// registry-purity pass; the two inline pairs were identical).
func (m *Model) ensureGroupMembersData(d *orgData, o sf.Org) tea.Cmd {
	if d.GroupMemberID == "" {
		return nil
	}
	return d.EnsureGroupMembers(targetArg(o), d.GroupMemberID).Ensure(m.cache)
}

func (m Model) refreshGroupMembersData(d *orgData) tea.Cmd {
	if d.GroupMemberID == "" || len(m.orgs) == 0 {
		return nil
	}
	return d.EnsureGroupMembers(targetArg(m.orgs[m.selected]), d.GroupMemberID).Refresh(m.cache)
}

// groupMembersFetchedAt is the header-freshness hook shared by the
// queue + public-group detail tabs — both render the same per-group
// members resource.
func groupMembersFetchedAt(m Model, d *orgData) time.Time {
	if r, ok := d.GroupMembers[d.GroupMemberID]; ok && r != nil {
		return r.FetchedAt()
	}
	return time.Time{}
}
