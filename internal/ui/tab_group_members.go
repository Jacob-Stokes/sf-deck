package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func (m Model) renderQueueDetail(w, innerH int) string {
	return m.renderGroupMembersDetail(w, innerH, "Queue")
}

func (m Model) renderPublicGroupDetail(w, innerH int) string {
	return m.renderGroupMembersDetail(w, innerH, "Public Group")
}

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

	parentName := groupParentName(d, d.GroupMemberKind, d.GroupMemberID)
	title := parentLabel + " · " + parentName
	if parentName == "" {
		title = parentLabel + " · " + d.GroupMemberID
	}

	var lines []string
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

func groupMembersFetchedAt(m Model, d *orgData) time.Time {
	if r, ok := d.GroupMembers[d.GroupMemberID]; ok && r != nil {
		return r.FetchedAt()
	}
	return time.Time{}
}
