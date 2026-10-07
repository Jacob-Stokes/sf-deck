package ui

func (m Model) currentOrgRailRows() []orgRailRow {
	return buildRailRows(m.orgs, m.settings.OrgGroups())
}

func (m Model) orgRailCursorOnHeader() bool {
	rows := m.currentOrgRailRows()
	if len(rows) == 0 {
		return false
	}
	c := m.orgRailCursor
	if c < 0 || c >= len(rows) {
		return false
	}
	return rows[c].Kind == railRowGroupHeader
}

func (m Model) cursoredGroupID() string {
	rows := m.currentOrgRailRows()
	if len(rows) == 0 {
		return ""
	}
	c := m.orgRailCursor
	if c < 0 || c >= len(rows) {
		return ""
	}
	return rows[c].GroupID
}

// clampOrgRailCursor keeps m.orgRailCursor inside the row list and
// guarantees it lands on an org row (never a header). Mirrors the
// underlying org index into m.selected. No-op when the row list is
// empty.
func (m *Model) clampOrgRailCursor() {
	rows := m.currentOrgRailRows()
	if len(rows) == 0 {
		m.orgRailCursor = 0
		return
	}
	if m.orgRailCursor < 0 {
		m.orgRailCursor = 0
	}
	if m.orgRailCursor >= len(rows) {
		m.orgRailCursor = len(rows) - 1
	}
	if rows[m.orgRailCursor].Kind != railRowOrg {
		m.orgRailCursor = nearestOrgRow(rows, m.orgRailCursor, 1)
	}
	row := rows[m.orgRailCursor]
	if row.Kind == railRowOrg {
		m.setSelectedOrg(row.OrgIdx)
	}
}

// syncOrgRailCursorToSelected positions the rail cursor on the row
// that owns m.selected. Called whenever an external code path
// (quick-jump, keymap shortcuts that call ensureOrgData) shifts the
// selected org out from under the rail cursor. Always lands on an
// org row — clampOrgRailCursor handles the case where m.selected
// points at an org that isn't currently rendered (collapsed group).
func (m *Model) syncOrgRailCursorToSelected() {
	rows := m.currentOrgRailRows()
	if len(rows) == 0 {
		m.orgRailCursor = 0
		return
	}
	for i, r := range rows {
		if r.Kind == railRowOrg && r.OrgIdx == m.selected {
			m.orgRailCursor = i
			return
		}
	}
	m.clampOrgRailCursor()
}

func (m *Model) stepOrgRailCursor(delta int) bool {
	rows := m.currentOrgRailRows()
	if len(rows) == 0 {
		return false
	}
	prev := m.selected

	step := 1
	if delta < 0 {
		step = -1
		delta = -delta
	}
	target := m.orgRailCursor
	for delta > 0 {
		next := target + step
		if next < 0 || next >= len(rows) {
			break
		}
		target = next
		if rows[target].Kind == railRowOrg {
			delta--
		}
	}
	// If we ended on a header (e.g. cursor started on one and we
	// couldn't move at all), nudge to the nearest org row in our
	// direction; otherwise fall back to the nearest in the opposite.
	if rows[target].Kind != railRowOrg {
		target = nearestOrgRow(rows, target, step)
	}
	m.orgRailCursor = target
	if rows[target].Kind == railRowOrg {
		m.setSelectedOrg(rows[target].OrgIdx)
	}
	return m.selected != prev
}

func nearestOrgRow(rows []orgRailRow, from, dir int) int {
	if dir == 0 {
		dir = 1
	}
	for _, d := range []int{dir, -dir} {
		i := from
		for i >= 0 && i < len(rows) {
			if rows[i].Kind == railRowOrg {
				return i
			}
			i += d
		}
	}
	return from
}
