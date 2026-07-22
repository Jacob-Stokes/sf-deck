package ui

// update_orgs_keys.go — focus=orgs key intercept.
//
// Runs BEFORE the global keymap switch in handleKey when m.focus ==
// focusOrgs and the active utility is Orgs. The intercept owns the
// keys for org grouping (n, R, x, space, [, ], <, >, g) and auth
// lifecycle (A, D, *, =).
//
// Each handler returns (consumed, cmd). consumed=true means "this
// key was for the orgs panel and the global dispatcher should skip
// the keystroke." consumed=false falls through, e.g. j/k still
// invoke the global Move handlers (which call moveCursor, which
// branches on focus and walks the rail cursor).

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

// onOrgsKey is the focus=orgs intercept. Returns (true, cmd) when
// the key was consumed by an orgs-panel command; (false, nil) on
// fall-through.
//
// The rail intercept owns ONLY navigation aids (fold/expand) plus
// the trigger that opens the org-management modal. Every other
// edit action — create/rename/delete groups, move orgs, add org,
// logout, set default, rename alias — lives in the modal. The
// rail stays a quick-nav surface; the modal is where you go to
// reorganize.
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

// moveCursoredOrg moves the cursored org up (delta=-1) or down (+1)
// within its group, crossing into the next/prev group at the
// boundary. No-op when on a header.
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
		// From Ungrouped: delta<0 moves into the last group's bottom;
		// delta>0 into the first group's top. No-op when there are
		// zero groups.
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
		// In a real group. Find the position of username in members.
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
			// Within the same group — swap.
			srcGroup.Members[pos], srcGroup.Members[newPos] = srcGroup.Members[newPos], srcGroup.Members[pos]
			groups[srcIdx] = srcGroup
			m.settings.SetOrgGroups(groups)
		} else {
			// Cross a boundary. delta<0 from the top → previous group's
			// bottom; delta>0 from the bottom → next group's top. If no
			// adjacent group exists, fall to Ungrouped (delete from
			// current group's members, leave unassigned).
			members := append(srcGroup.Members[:pos], srcGroup.Members[pos+1:]...)
			srcGroup.Members = members
			groups[srcIdx] = srcGroup
			adj := srcIdx + delta
			if adj >= 0 && adj < len(groups) {
				if delta < 0 {
					// Append to bottom of prev group
					groups[adj].Members = append(groups[adj].Members, username)
				} else {
					// Prepend to top of next group
					groups[adj].Members = append([]string{username}, groups[adj].Members...)
				}
			}
			// If adj is out of range, the org now lives in Ungrouped —
			// no further action needed since we already removed it.
			m.settings.SetOrgGroups(groups)
		}
	default:
		return true, nil
	}
	m.saveSettings("")
	m.syncOrgRailCursorToOrg(username)
	return true, nil
}

// startCreateGroup opens the new-group prompt modal.
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

// syncOrgRailCursorToOrg positions the rail cursor on the row that
// owns the given username. Mirrors m.selected to that org's index
// so existing "current org" callers keep working.
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

// orgGroupPromptKind distinguishes create vs rename; both use the
// same single-line text input modal.
type orgGroupPromptKind int

const (
	orgGroupPromptCreate orgGroupPromptKind = iota
	orgGroupPromptRename
)

// applyOrgGroupPrompt commits the create/rename. Caller passes the
// trimmed user input. Empty input cancels.
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
