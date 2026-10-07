package ui

// /users exposes user lists and detail navigation.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func usersListCols() []uilayout.ListColumn {
	return schemaListColumns(userColumnSchema())
}

var recentUsersListSurface = listSurface{
	State:       func(d *orgData) *uilayout.ListTableState { return &d.HomeUserTableState },
	Cols:        usersListCols,
	SearchPtr:   func(d *orgData) *searchState { return d.HomeUserList.SearchPtr() },
	MoveCursor:  func(d *orgData, n int) { d.HomeUserList.MoveBy(n) },
	ResetCursor: func(d *orgData) { d.HomeUserList.ResetCursor() },
	BuildRenderModel: func(m Model, d *orgData) (listRenderModel, bool) {
		if d == nil {
			return listRenderModel{}, false
		}
		resolved := mustResolveColumns(userColumnSchema())
		cols := resolved.ListColumns()
		installListViewOrderRows(&d.HomeUserList, &d.HomeUserTableState, cols,
			func(items []sf.UserRow, row, col int) string {
				return resolvedSortCellForListColumn(resolved, items, cols, row, col)
			})
		items := d.HomeUserList.Filtered()
		h := d.Home.Value()
		title := fmt.Sprintf("RECENT LOGINS · %d shown · %d active total · %s",
			d.HomeUserList.Len(), h.Users.TotalActive,
			humanAge(d.Home.FetchedAt())+stateSuffix(d.Home.Busy(), d.Home.Err()))
		return listRenderModel{
			Title:  title,
			State:  &d.HomeUserTableState,
			Search: d.HomeUserList.SearchPtr(),
			Cols:   cols,
			N:      len(items),
			Cursor: d.HomeUserList.Cursor(),
			Cell: func(row, col int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				return resolvedCellForListColumn(resolved, items, cols, row, col)
			},
			Empty:       "  no recent logins",
			DataVersion: listVersionWithStore(d.HomeUserList.Version(), m),
		}, true
	},
}

var allUsersListSurface = listSurface{
	State: func(d *orgData) *uilayout.ListTableState {
		return d.UsersTableStatePtr(activeUsersChipID(d))
	},
	Cols: usersListCols,
	SearchPtr: func(d *orgData) *searchState {
		return d.UsersListPtr(activeUsersChipID(d)).SearchPtr()
	},
	MoveCursor: func(d *orgData, n int) {
		d.UsersListPtr(activeUsersChipID(d)).MoveBy(n)
	},
	ResetCursor: func(d *orgData) {
		d.UsersListPtr(activeUsersChipID(d)).ResetCursor()
	},
	BuildRenderModel: func(m Model, d *orgData) (listRenderModel, bool) {
		if d == nil {
			return listRenderModel{}, false
		}
		chipID := activeUsersChipID(d)
		d.SyncChipUsers(chipID)
		lv := d.UsersListPtr(chipID)
		state := d.UsersTableStatePtr(chipID)
		resolved := mustResolveColumns(userColumnSchema())
		cols := resolved.ListColumns()
		installListViewOrderRows(lv, state, cols,
			func(items []sf.UserRow, row, col int) string {
				return resolvedSortCellForListColumn(resolved, items, cols, row, col)
			})
		items := lv.Filtered()
		// Hard-capped: the chip's SOQL pinned a LIMIT, so lv.Len() is
		// at most the cap. Title is just "visible / fetched" — no
		// "of N" suffix because LIMIT collapses totalSize to the cap.
		var title string
		if r, ok := d.ChipUsers[chipID]; ok && r != nil {
			title = fmt.Sprintf("USERS · %s · %d / %d · %s",
				usersChipLabel(m, chipID),
				len(items), lv.Len(),
				humanAge(r.FetchedAt())+stateSuffix(r.Busy(), r.Err()))
		} else {
			title = fmt.Sprintf("USERS · %d / %d", len(items), lv.Len())
		}
		return listRenderModel{
			Title:  title,
			State:  state,
			Search: lv.SearchPtr(),
			Cols:   cols,
			N:      len(items),
			Cursor: lv.Cursor(),
			Cell: func(row, col int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				return resolvedCellForListColumn(resolved, items, cols, row, col)
			},
			Empty:       "  no users match this chip",
			DataVersion: listVersionWithStore(lv.Version(), m),
		}, true
	},
}

func activeUsersChipID(d *orgData) string {
	if d == nil || d.ActiveUsersChipID == "" {
		if len(qchip.UserBuiltins) == 0 {
			return "all"
		}
		return qchip.UserBuiltins[0].ID
	}
	return d.ActiveUsersChipID
}

func usersChipLabel(m Model, chipID string) string {
	if m.chipRegistry(domainUsers) == nil {
		return chipID
	}
	for _, c := range m.chipRegistry(domainUsers).ChipsFor("*") {
		if c.ID == chipID {
			return c.Label
		}
	}
	return chipID
}

// activeUsersChip looks up the qchip.Chip struct for the chip-id
// currently selected on /users · All users. Returns the first
// built-in when the registry can't resolve the id (e.g. the user's
// settings.toml referenced a stale id).
func activeUsersChip(m Model, d *orgData) (qchip.Chip, bool) {
	chipID := activeUsersChipID(d)
	if m.chipRegistry(domainUsers) != nil {
		for _, c := range m.chipRegistry(domainUsers).ChipsFor("*") {
			if c.ID == chipID {
				return c, true
			}
		}
	}
	if len(qchip.UserBuiltins) > 0 {
		return qchip.UserBuiltins[0], true
	}
	return qchip.Chip{}, false
}

func ensureActiveUsersChip(m *Model, d *orgData) tea.Cmd {
	if m == nil || d == nil || len(m.orgs) == 0 {
		return nil
	}
	c, ok := activeUsersChip(*m, d)
	if !ok {
		return nil
	}
	subs := qchip.Substitutions{UserID: d.Home.Value().UserID}
	r := d.EnsureChipUsers(m.orgs[m.selected].Alias, c, subs)
	return r.Ensure(m.cache)
}

func refreshActiveUsersChip(m Model, d *orgData) tea.Cmd {
	if d == nil || len(m.orgs) == 0 {
		return nil
	}
	c, ok := activeUsersChip(m, d)
	if !ok {
		return nil
	}
	subs := qchip.Substitutions{UserID: d.Home.Value().UserID}
	r := d.EnsureChipUsers(m.orgs[m.selected].Alias, c, subs)
	return r.Refresh(m.cache)
}

// allUsersOpenSurface mirrors usersOpenSurface but reads from the
// AllUsers list — keeps the cursored row's Lightning URL accurate
// when the user is on the All users subtab. Assigned in init() to
// avoid a package-init cycle (Drill closure pulls in resolveOpenSurface
// which transitively references the tab registry).
var allUsersOpenSurface openSurface

func init() {
	allUsersOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			d := m.activeOrgData()
			if d == nil {
				return nil
			}
			lv := d.UsersListPtr(activeUsersChipID(d))
			if v, ok := lv.Selected(); ok {
				return m.enrichUserRowTargets(v)
			}
			return nil
		},
		Drill: func(m *Model) (tea.Cmd, bool) {
			d := m.activeOrgData()
			if d == nil {
				return nil, false
			}
			lv := d.UsersListPtr(activeUsersChipID(d))
			u, ok := lv.Selected()
			if !ok {
				return nil, false
			}
			if s := lv.SearchPtr(); s.Active {
				s.Active = false
				s.Committed = s.Buffer() != ""
			}
			return m.triggerOpenUser(u.ID), true
		},
	}
}

func (m Model) renderUsers(w, innerH int) string {
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	if !canUseOrg(o) {
		return theme.Subtle.Render("  org disconnected")
	}
	return m.dispatchSubtab(w, innerH, m.tabSubtabs(), m.usersSubtab(),
		map[Subtab]subtabBranch{
			SubtabUsersRecent: {Render: m.renderUsersRecent},
			SubtabUsersAll:    {Render: m.renderUsersAll},
			SubtabUsersActive: {Render: m.renderActiveUsers},
		},
		subtabBranch{Render: m.renderUsersRecent},
	)
}

func (m Model) renderUsersRecent(w, innerH int) string {
	d := m.activeOrgData()
	if d == nil {
		return theme.Subtle.Render("  loading…")
	}
	inner := w - 4
	if d.Home.FetchedAt().IsZero() {
		if d.Home.Busy() {
			return dimLine("  loading recent logins…", inner)
		}
		return dimLine("  press "+firstPretty(Keys.Refresh)+" to load recent logins", inner)
	}
	model, ok := recentUsersListSurface.BuildRenderModel(m, d)
	if !ok {
		return dimLine("  loading…", inner)
	}
	return strings.Join(renderListModel(m, model, m.focus, inner, innerH), "\n")
}

func (m Model) renderUsersAll(w, innerH int) string {
	d := m.activeOrgData()
	if d == nil {
		return theme.Subtle.Render("  loading…")
	}
	inner := w - 4

	chips := m.stripRows(domainUsers, "*")
	chipSel := m.allUsersChipIdx()
	if chipSel < 0 || chipSel >= len(chips) {
		chipSel = 0
	}
	dash := m.renderDashboard("VIEWS", chips, chipSel, inner)

	var lines []string
	if dash != "" {
		lines = append(lines, dash)
	}

	chipID := activeUsersChipID(d)
	r := d.ChipUsers[chipID]
	if r == nil || r.FetchedAt().IsZero() {
		if r != nil && r.Busy() {
			lines = append(lines, dimLine("  loading users…", inner))
		} else {
			lines = append(lines, dimLine("  press "+firstPretty(Keys.Refresh)+" to load users", inner))
		}
		return strings.Join(lines, "\n")
	}

	model, ok := allUsersListSurface.BuildRenderModel(m, d)
	if !ok {
		lines = append(lines, dimLine("  loading…", inner))
		return strings.Join(lines, "\n")
	}
	budget := innerH - usedLines(lines)
	lines = append(lines, renderListModel(m, model, m.focus, inner, budget)...)
	return strings.Join(lines, "\n")
}
