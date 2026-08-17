package ui

// update_orgs_keys.go — focus=orgs key intercept.

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

func (m *Model) onOrgsKey(key string) (bool, tea.Cmd) {
	if m.focus != focusOrgs || m.currentUtility().ID != utilityOrgs {
		return false, nil
	}

	switch {
	case matches(key, Keys.OrgGroupToggle):
		return m.toggleCursoredGroup()
	case matches(key, Keys.OrgManageOpen):
		return m.openOrgManageModal()
	}
	return false, nil
}

// toggleCursoredGroup flips the collapsed flag on the group that
// contains the cursored org. Since the rail cursor never lands on
// header rows (stepOrgRailCursor skips them), space always means
// "fold the group I'm currently inside." Synthetic Ungrouped is
// never collapsible (no-op there).
//
// After toggling, snap the cursor to the group's header position
// when it just collapsed — the org under the cursor is no longer
// rendered, so the next visible row is the header itself; nudging
// to the next visible org keeps j/k feeling sane.
func (m *Model) toggleCursoredGroup() (bool, tea.Cmd) {
	gid := m.cursoredGroupID()
	if gid == "" || gid == ungroupedID {
		return true, nil
	}
	groups := m.settings.OrgGroups()
	idx, _ := findGroupByID(groups, gid)
	if idx < 0 {
		return true, nil
	}
	groups[idx].Collapsed = !groups[idx].Collapsed
	m.settings.SetOrgGroups(groups)
	m.saveSettings("")
	m.clampOrgRailCursor()
	return true, nil
}

func (m *Model) moveCursoredOrg(delta int) (bool, tea.Cmd) {
	if m.orgRailCursorOnHeader() {
		return true, nil
	}
	rows := m.currentOrgRailRows()
	cur := m.orgRailCursor
	if cur < 0 || cur >= len(rows) {
		return true, nil
	}
	row := rows[cur]
	if row.Kind != railRowOrg {
		return true, nil
	}
	username := row.Org.Username
	groups := m.settings.OrgGroups()
	srcGroupID := row.GroupID
	srcIdx, srcGroup := findGroupByID(groups, srcGroupID)

	// Remove the username from its current group (or note that it's
	// in the synthetic Ungrouped bucket — i.e. not in any group's
	// members). reattach() puts it back at a target position.
	switch {
	case srcGroupID == ungroupedID:
		if len(groups) == 0 {
			return true, nil
		}
		var dstIdx int
		if delta < 0 {
			dstIdx = len(groups) - 1
			groups[dstIdx].Members = append(groups[dstIdx].Members, username)
		} else {
			dstIdx = 0
			groups[dstIdx].Members = append([]string{username}, groups[dstIdx].Members...)
		}
		m.settings.SetOrgGroups(groups)
	case srcIdx >= 0:
		pos := -1
		for i, mu := range srcGroup.Members {
			if mu == username {
				pos = i
				break
			}
		}
		if pos < 0 {
			return true, nil
		}
		newPos := pos + delta
		if newPos >= 0 && newPos < len(srcGroup.Members) {
			srcGroup.Members[pos], srcGroup.Members[newPos] = srcGroup.Members[newPos], srcGroup.Members[pos]
			groups[srcIdx] = srcGroup
			m.settings.SetOrgGroups(groups)
		} else {
			members := append(srcGroup.Members[:pos], srcGroup.Members[pos+1:]...)
			srcGroup.Members = members
			groups[srcIdx] = srcGroup
			adj := srcIdx + delta
			if adj >= 0 && adj < len(groups) {
				if delta < 0 {
					groups[adj].Members = append(groups[adj].Members, username)
				} else {
					groups[adj].Members = append([]string{username}, groups[adj].Members...)
				}
			}
			m.settings.SetOrgGroups(groups)
		}
	default:
		return true, nil
	}
	m.saveSettings("")
	m.syncOrgRailCursorToOrg(username)
	return true, nil
}

func (m *Model) startCreateGroup() (bool, tea.Cmd) {
	m.openOrgGroupPrompt(orgGroupPromptCreate, "", "")
	return true, nil
}

// startAddOrg, startDisconnectOrg, setDefaultCursoredOrg,
// startRenameCursoredAlias are stubs filled in by Step 5 + Step 6.
// They consume the key so the global dispatcher doesn't fall through
// (e.g. global `r` for refresh would otherwise run when we want
// alias rename).
func (m *Model) startAddOrg() (bool, tea.Cmd) {
	m.openAddOrgChoice()
	return true, nil
}

func (m *Model) syncOrgRailCursorToOrg(username string) {
	rows := m.currentOrgRailRows()
	for i, r := range rows {
		if r.Kind == railRowOrg && r.Org.Username == username {
			m.orgRailCursor = i
			m.setSelectedOrg(r.OrgIdx)
			return
		}
	}
}

type orgGroupPromptKind int

const (
	orgGroupPromptCreate orgGroupPromptKind = iota
	orgGroupPromptRename
)

func (m *Model) applyOrgGroupPrompt(kind orgGroupPromptKind, targetID, name string) {
	if name == "" {
		return
	}
	groups := m.settings.OrgGroups()
	switch kind {
	case orgGroupPromptCreate:
		id := uniqueGroupID(name, groups)
		groups = append(groups, settings.OrgGroupConfig{ID: id, Name: name})
		m.settings.SetOrgGroups(groups)
	case orgGroupPromptRename:
		idx, _ := findGroupByID(groups, targetID)
		if idx < 0 {
			return
		}
		groups[idx].Name = name
		m.settings.SetOrgGroups(groups)
	}
	m.saveSettings("")
}
