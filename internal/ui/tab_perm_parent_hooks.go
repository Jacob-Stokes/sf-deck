package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// permParentFetchedAt is TabPermParentDetail's header-freshness hook.
// The drill-in shows a different resource per subtab, so the age
// follows the active one; Overview (and PSG Components) rides the
// parent list the row came from.
func permParentFetchedAt(m Model, d *orgData) time.Time {
	switch m.currentSubtab() {
	case SubtabParentObjects:
		key := d.PermParentKind + ":" + d.PermParentPermSetID
		if r, ok := d.ObjectPerms[key]; ok && r != nil {
			return r.FetchedAt()
		}
	case SubtabParentSystem:
		if r, ok := d.SystemPerms[d.PermParentPermSetID]; ok && r != nil {
			return r.FetchedAt()
		}
	case SubtabParentUsers:
		if r, ok := d.AssignedUsers[d.PermParentPermSetID]; ok && r != nil {
			return r.FetchedAt()
		}
	}
	switch d.PermParentKind {
	case "psg":
		return d.PSGs.FetchedAt()
	case "profile":
		return d.Profiles.FetchedAt()
	}
	return d.PermSets.FetchedAt()
}

func (m *Model) cyclePermParentChip(delta int) tea.Cmd {
	if m.currentSubtab() != SubtabParentObjects || len(m.orgs) == 0 {
		return nil
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	if d.PermParentPermSetID == "" || d.PermFieldsSObject == "" {
		return nil
	}
	key := d.PermParentKind + ":" + d.PermParentPermSetID
	res := d.ObjectPerms[key]
	if res == nil || res.FetchedAt().IsZero() {
		return nil
	}
	rows := res.Value()
	if len(rows) == 0 {
		return nil
	}
	cur := 0
	for i, r := range rows {
		if r.SObjectType == d.PermFieldsSObject {
			cur = i
			break
		}
	}
	cur = wrapIdx(cur+delta, len(rows))
	d.PermFieldsSObject = rows[cur].SObjectType
	return m.onTabChanged()
}

func (m *Model) movePermParentCursor(delta int) {
	if len(m.orgs) == 0 {
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	switch m.currentSubtab() {
	case SubtabParentObjects:
		if d.PermParentPermSetID != "" && d.PermFieldsSObject != "" {
			desc := d.Describes[d.PermFieldsSObject]
			if desc != nil && !desc.FetchedAt().IsZero() {
				n := len(desc.Value().Fields)
				d.Cursors.Move(cursorKindFLS, delta, n, d.PermFieldsSObject, d.PermParentPermSetID)
				return
			}
		}
		if d.PermParentPermSetID != "" {
			key := d.PermParentKind + ":" + d.PermParentPermSetID
			if r, ok := d.ObjectPerms[key]; ok {
				n := len(r.Value())
				d.Cursors.Move(cursorKindObjectPerms, delta, n, d.PermParentKind, d.PermParentPermSetID)
			}
		}
	case SubtabParentSystem:
		if d.PermParentPermSetID != "" {
			if r, ok := d.SystemPerms[d.PermParentPermSetID]; ok {
				n := len(r.Value())
				d.Cursors.Move(cursorKindSystemPerms, delta, n, d.PermParentPermSetID)
			}
		}
	case SubtabParentUsers:
		if d.PermParentPermSetID != "" {
			if r, ok := d.AssignedUsers[d.PermParentPermSetID]; ok {
				n := len(r.Value())
				d.Cursors.Move(cursorKindAssignedUsers, delta, n, d.PermParentPermSetID)
			}
		}
	}
}

func (m Model) permParentSearchPtr() *searchState {
	if len(m.orgs) == 0 {
		return nil
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil {
		return nil
	}
	key := d.PermParentKind + ":" + d.PermParentPermSetID
	switch m.currentSubtab() {
	case SubtabParentObjects:
		if d.ObjPermSearch == nil {
			d.ObjPermSearch = map[string]*searchState{}
		}
		if s, ok := d.ObjPermSearch[key]; ok {
			return s
		}
		s := &searchState{}
		d.ObjPermSearch[key] = s
		return s
	case SubtabParentSystem:
		if d.SysPermSearch == nil {
			d.SysPermSearch = map[string]*searchState{}
		}
		if s, ok := d.SysPermSearch[d.PermParentPermSetID]; ok {
			return s
		}
		s := &searchState{}
		d.SysPermSearch[d.PermParentPermSetID] = s
		return s
	}
	return nil
}

func (m *Model) resetPermParentCursor() {
	if len(m.orgs) == 0 {
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	switch m.currentSubtab() {
	case SubtabParentObjects:
		d.Cursors.Reset(cursorKindObjectPerms, d.PermParentKind, d.PermParentPermSetID)
	case SubtabParentSystem:
		d.Cursors.Reset(cursorKindSystemPerms, d.PermParentPermSetID)
	}
}

func (m *Model) ensurePermParentData(d *orgData, o sf.Org) tea.Cmd {
	cmds := []tea.Cmd{
		d.PermSets.Ensure(m.cache),
		d.PSGs.Ensure(m.cache),
		d.Profiles.Ensure(m.cache),
	}
	switch m.currentSubtab() {
	case SubtabParentObjects:
		if d.PermParentPermSetID != "" {
			r := d.EnsureObjectPerms(targetArg(o), d.PermParentKind, d.PermParentPermSetID)
			cmds = append(cmds, r.Ensure(m.cache))
			if d.PermFieldsSObject != "" {
				desc := d.EnsureDescribe(targetArg(o), d.PermFieldsSObject)
				cmds = append(cmds, desc.Ensure(m.cache))
				fls := d.EnsureFLS(targetArg(o), d.PermFieldsSObject, d.PermParentPermSetID)
				cmds = append(cmds, fls.Ensure(m.cache))
			}
		}
	case SubtabParentSystem:
		if d.PermParentPermSetID != "" {
			r := d.EnsureSystemPerms(targetArg(o), d.PermParentPermSetID)
			cmds = append(cmds, r.Ensure(m.cache))
		}
	case SubtabParentUsers:
		if d.PermParentPermSetID != "" {
			r := d.EnsureAssignedUsers(targetArg(o), d.PermParentPermSetID)
			cmds = append(cmds, r.Ensure(m.cache))
		}
	}
	return tea.Batch(cmds...)
}

// refreshPermParentData is the r-key refresh for TabPermParentDetail:
// force-refresh the resource behind the ACTIVE subtab (object perms /
// system perms / assigned users) plus the parent lists. Without this r
// was a no-op on a permset/PSG/profile drill — only ctrl+r reached it.
func (m Model) refreshPermParentData(d *orgData) tea.Cmd {
	if d == nil || len(m.orgs) == 0 {
		return nil
	}
	o := m.orgs[m.selected]
	cmds := []tea.Cmd{
		d.PermSets.Refresh(m.cache),
		d.PSGs.Refresh(m.cache),
		d.Profiles.Refresh(m.cache),
	}
	if d.PermParentPermSetID != "" {
		switch m.currentSubtab() {
		case SubtabParentObjects:
			cmds = append(cmds, d.EnsureObjectPerms(targetArg(o), d.PermParentKind, d.PermParentPermSetID).Refresh(m.cache))
			if d.PermFieldsSObject != "" {
				cmds = append(cmds, d.EnsureFLS(targetArg(o), d.PermFieldsSObject, d.PermParentPermSetID).Refresh(m.cache))
			}
		case SubtabParentSystem:
			cmds = append(cmds, d.EnsureSystemPerms(targetArg(o), d.PermParentPermSetID).Refresh(m.cache))
		case SubtabParentUsers:
			cmds = append(cmds, d.EnsureAssignedUsers(targetArg(o), d.PermParentPermSetID).Refresh(m.cache))
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) activatePermParent() tea.Cmd {
	if m.currentSubtab() != SubtabParentObjects || len(m.orgs) == 0 {
		return nil
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	if d.PermParentPermSetID == "" {
		return nil
	}
	key := d.PermParentKind + ":" + d.PermParentPermSetID
	res := d.ObjectPerms[key]
	if res == nil {
		return nil
	}
	rows := filteredObjectPermRows(res.Value(), "")
	if s := d.ObjPermSearch[key]; s != nil {
		rows = filteredObjectPermRows(res.Value(), s.Buffer())
	}
	cur := d.Cursors.Get(cursorKindObjectPerms, len(rows), d.PermParentKind, d.PermParentPermSetID)
	if len(rows) == 0 {
		return nil
	}
	d.PermFieldsSObject = rows[cur].SObjectType
	return m.onTabChanged()
}

func filteredObjectPermRows(allRows []sf.ObjectPermission, q string) []sf.ObjectPermission {
	q = strings.ToLower(q)
	if q == "" {
		return allRows
	}
	rows := []sf.ObjectPermission{}
	for _, r := range allRows {
		if strings.Contains(strings.ToLower(r.SObjectType), q) {
			rows = append(rows, r)
		}
	}
	return rows
}
