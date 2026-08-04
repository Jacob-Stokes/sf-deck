package ui

// orggroups_cursor.go — rail cursor helpers + moveOrgRailCursor.
//
// The cursor (m.orgRailCursor) addresses the row list returned by
// buildRailRows (see orggroups.go). Headers and orgs share one
// flat space; m.selected stays in lockstep with the cursor's
// underlying org index so existing "current org" callers don't
// need to change.

// currentOrgRailRows rebuilds the rail's row list from the current
// orgs + persisted groups. Cheap — used by every cursor handler so
// header insertions/removals are reflected on the next keystroke.
func (m Model) currentOrgRailRows() []orgRailRow {
	return buildRailRows(m.orgs, m.settings.OrgGroups())
}

// orgRailCursorOnHeader reports whether the rail cursor currently
// addresses a group header row.
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

// cursoredGroupID returns the group id the cursor is "in" — when on
// a header, that header's id; when on an org, the containing group's
// id (or ungroupedID). Returns "" when the rail is empty.
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
	// m.selected isn't currently rendered (collapsed group). Snap
	// to the nearest org row so subsequent j/k feels sane.
	m.clampOrgRailCursor()
}

// stepOrgRailCursor advances the rail cursor by `delta` org rows,
// skipping group headers entirely. Returns true when the
// underlying selected org changed (caller fires onOrgChanged).
//
// Skipping headers in the rail is a UX choice — they're visual
// dividers only at this surface; every editing action lives in
// the org-manage modal which uses its own cursor that DOES land
// on headers. j/k in the rail therefore behaves like an org-only
// list.
func (m *Model) stepOrgRailCursor(delta int) bool {
	rows := m.currentOrgRailRows()
	if len(rows) == 0 {
		return false
	}
	prev := m.selected

	// Walk |delta| org rows in the right direction, hopping over
	// any header rows we encounter.
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

// nearestOrgRow returns the index of the closest org row to `from`
// in the row list. Searches in `dir` first (1 forward, -1 back);
// falls back to the opposite direction when the first one runs out.
// Returns `from` unchanged when no org row exists at all.
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
