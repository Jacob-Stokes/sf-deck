package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/diff"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/tablemodel"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func (m *Model) reloadCompareSaved(d *orgData) {
	if d == nil {
		return
	}
	var rows []CompareDefRow
	if m.devProjects != nil {
		if saved, err := m.devProjects.ListSavedComparisons(); err == nil {
			for _, sc := range saved {
				rows = append(rows, CompareDefRow{
					Kind:   savedRowComparison,
					ID:     sc.ID,
					Name:   sc.Name,
					Source: sc.Source,
					Target: sc.Target,
					Scope:  sc.Scope,
					Saved:  "saved " + humanTimeAgo(sc.UpdatedAt),
				})
			}
		}
	}
	if m.settings != nil {
		for _, def := range m.settings.CompareDefs() {
			rows = append(rows, CompareDefRow{
				Kind:   savedRowTemplate,
				Name:   def.Name,
				Source: def.Source,
				Target: def.Target,
				Scope:  scopeLabel(def.Scope),
			})
		}
	}
	if !d.SavedList.HasMatch() {
		d.SavedList.SetMatch(func(r CompareDefRow, q string) bool {
			return strings.Contains(strings.ToLower(r.Name+" "+r.Source+" "+r.Target+" "+r.KindLabel()), q)
		})
	}
	d.SavedList.Set(rows)
	d.SavedLoaded = true
}

func compareSavedColumnSchema() tablemodel.Schema[CompareDefRow] {
	return tablemodel.Schema[CompareDefRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Kind", "Name", "Source", "Target", "Scope", "Saved"}
		},
		Columns: map[string]tablemodel.ColumnDef[CompareDefRow]{
			"Kind":   textColumnDef[CompareDefRow]("KIND", tablemodel.Width{Min: 10, Ideal: 11, Max: 11}, func(r CompareDefRow) string { return r.KindLabel() }),
			"Name":   textColumnDef[CompareDefRow]("NAME", tablemodel.Width{Min: 14, Ideal: 24}, func(r CompareDefRow) string { return r.Name }),
			"Source": textColumnDef[CompareDefRow]("SOURCE", tablemodel.Width{Min: 12, Ideal: 20}, func(r CompareDefRow) string { return r.Source }),
			"Target": textColumnDef[CompareDefRow]("TARGET", tablemodel.Width{Min: 12, Ideal: 20}, func(r CompareDefRow) string { return r.Target }),
			"Scope":  textColumnDef[CompareDefRow]("SCOPE", tablemodel.Width{Min: 12, Ideal: 26}, func(r CompareDefRow) string { return r.Scope }),
			"Saved":  textColumnDef[CompareDefRow]("SAVED", tablemodel.Width{Min: 8, Ideal: 14}, func(r CompareDefRow) string { return r.Saved }),
		},
	}
}

var compareSavedListSurface = listSurface{
	State:       func(d *orgData) *uilayout.ListTableState { return &d.SavedTable },
	Cols:        func() []uilayout.ListColumn { return schemaListColumns(compareSavedColumnSchema()) },
	SearchPtr:   func(d *orgData) *searchState { return d.SavedList.SearchPtr() },
	MoveCursor:  func(d *orgData, n int) { d.SavedList.MoveBy(n) },
	ResetCursor: func(d *orgData) { d.SavedList.ResetCursor() },
	BuildRenderModel: func(m Model, d *orgData) (listRenderModel, bool) {
		if d == nil {
			return listRenderModel{}, false
		}
		resolved := mustResolveColumns(compareSavedColumnSchema())
		cols := resolved.ListColumns()
		installListViewOrderRows(&d.SavedList, &d.SavedTable, cols,
			func(items []CompareDefRow, row, col int) string {
				if row < 0 || row >= len(items) || col < 0 || col >= len(cols) {
					return ""
				}
				return resolvedSortCellByID(resolved, items[row], cols[col].Name)
			})
		items := d.SavedList.Filtered()
		return listRenderModel{
			Title:  fmt.Sprintf("SAVED · %d", len(items)),
			State:  &d.SavedTable,
			Search: d.SavedList.SearchPtr(),
			Cols:   cols,
			N:      len(items),
			Cursor: d.SavedList.Cursor(),
			Cell: func(row, col int) string {
				if row < 0 || row >= len(items) || col < 0 || col >= len(cols) {
					return ""
				}
				return resolvedCellByID(resolved, items[row], cols[col].Name)
			},
			Empty:        "  nothing saved — run a comparison in New, then ctrl+s to save it",
			FooterExtras: "↵ open · e rerun/edit · d delete · t →template · " + firstPretty(Keys.SOQLRename) + " rename",
		}, true
	},
}

func (m Model) renderCompareSavedSubtab(w, innerH int) string {
	d, ok := m.activeOrgState()
	if !ok {
		return noOrgPlaceholder()
	}
	if !d.SavedLoaded {
		mm := m
		(&mm).reloadCompareSaved(d)
	}
	inner := w - 4
	model, ok := compareSavedListSurface.BuildRenderModel(m, d)
	if !ok {
		return theme.Subtle.Render("  saved comparisons unavailable")
	}
	return strings.Join(renderListModel(m, model, m.focus, inner, innerH), "\n")
}

func (m *Model) reloadCompareHistory(d *orgData) {
	if d == nil {
		return
	}
	if !d.HistoryList.HasMatch() {
		d.HistoryList.SetMatch(func(r CompareHistoryRow, q string) bool {
			return strings.Contains(strings.ToLower(r.Name+" "+r.Source+" "+r.Target), q)
		})
	}
	d.HistoryLoaded = true
}

func compareHistoryColumnSchema() tablemodel.Schema[CompareHistoryRow] {
	return tablemodel.Schema[CompareHistoryRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Source", "Target", "Diff", "Ran"}
		},
		Columns: map[string]tablemodel.ColumnDef[CompareHistoryRow]{
			"Name":   textColumnDef[CompareHistoryRow]("NAME", tablemodel.Width{Min: 12, Ideal: 20}, func(r CompareHistoryRow) string { return r.Name }),
			"Source": textColumnDef[CompareHistoryRow]("SOURCE", tablemodel.Width{Min: 12, Ideal: 20}, func(r CompareHistoryRow) string { return r.Source }),
			"Target": textColumnDef[CompareHistoryRow]("TARGET", tablemodel.Width{Min: 12, Ideal: 20}, func(r CompareHistoryRow) string { return r.Target }),
			"Diff": textColumnDef[CompareHistoryRow]("DIFF", tablemodel.Width{Min: 18, Ideal: 24}, func(r CompareHistoryRow) string {
				return fmt.Sprintf("%d≠ %d← %d→", r.Different, r.AOnly, r.BOnly)
			}),
			"Ran": textColumnDef[CompareHistoryRow]("RAN", tablemodel.Width{Min: 10, Ideal: 14}, func(r CompareHistoryRow) string { return r.RanAt }),
		},
	}
}

var compareHistoryListSurface = listSurface{
	State:       func(d *orgData) *uilayout.ListTableState { return &d.HistoryTable },
	Cols:        func() []uilayout.ListColumn { return schemaListColumns(compareHistoryColumnSchema()) },
	SearchPtr:   func(d *orgData) *searchState { return d.HistoryList.SearchPtr() },
	MoveCursor:  func(d *orgData, n int) { d.HistoryList.MoveBy(n) },
	ResetCursor: func(d *orgData) { d.HistoryList.ResetCursor() },
	BuildRenderModel: func(m Model, d *orgData) (listRenderModel, bool) {
		if d == nil {
			return listRenderModel{}, false
		}
		resolved := mustResolveColumns(compareHistoryColumnSchema())
		cols := resolved.ListColumns()
		installListViewOrderRows(&d.HistoryList, &d.HistoryTable, cols,
			func(items []CompareHistoryRow, row, col int) string {
				if row < 0 || row >= len(items) || col < 0 || col >= len(cols) {
					return ""
				}
				return resolvedSortCellByID(resolved, items[row], cols[col].Name)
			})
		items := d.HistoryList.Filtered()
		return listRenderModel{
			Title:  fmt.Sprintf("HISTORY · %d", len(items)),
			State:  &d.HistoryTable,
			Search: d.HistoryList.SearchPtr(),
			Cols:   cols,
			N:      len(items),
			Cursor: d.HistoryList.Cursor(),
			Cell: func(row, col int) string {
				if row < 0 || row >= len(items) || col < 0 || col >= len(cols) {
					return ""
				}
				return resolvedCellByID(resolved, items[row], cols[col].Name)
			},
			Empty: "  no runs yet this session",
		}, true
	},
}

func (m Model) renderCompareHistorySubtab(w, innerH int) string {
	d, ok := m.activeOrgState()
	if !ok {
		return noOrgPlaceholder()
	}
	if !d.HistoryLoaded {
		mm := m
		(&mm).reloadCompareHistory(d)
	}
	inner := w - 4
	model, ok := compareHistoryListSurface.BuildRenderModel(m, d)
	if !ok {
		return theme.Subtle.Render("  history unavailable")
	}
	return strings.Join(renderListModel(m, model, m.focus, inner, innerH), "\n")
}

func (d *orgData) recordCompareRun(run *compareRun) {
	_, different, aOnly, bOnly := run.Inv.Summary()
	row := CompareHistoryRow{
		Name:      "(ad-hoc)",
		Source:    run.Source.Ref,
		Target:    run.Target.Ref,
		RanAt:     time.Now().Format("15:04:05"),
		Different: different,
		AOnly:     aOnly,
		BOnly:     bOnly,
	}
	hist := append([]CompareHistoryRow{row}, d.HistoryList.Items()...)
	d.HistoryList.Set(hist)
	d.HistoryLoaded = true
}

func (m *Model) activateCompare() tea.Cmd {
	d, ok := m.activeOrgState()
	if !ok {
		return nil
	}
	switch m.currentSubtab() {
	case SubtabCompareSaved:
		return m.activateSavedRow(d)
	case SubtabCompareNew:
		return m.activateCompareNew(d)
	case SubtabCompareResult:
		return m.activateCompareResult(d)
	}
	return nil
}

func (m *Model) activateCompareNew(d *orgData) tea.Cmd {
	if d.Run == nil {
		d.Run = m.newCompareRun()
	}
	if d.Run.Phase != comparePhaseSetup {
		return nil // a result is in progress/shown; that lives on Result
	}
	rows := compareSetupRowsFor(d)
	if d.SetupCursor < 0 || d.SetupCursor >= len(rows) {
		return nil
	}
	switch rows[d.SetupCursor] {
	case setupRowSource:
		return m.openCompareOrgPicker(d, true)
	case setupRowTarget:
		return m.openCompareOrgPicker(d, false)
	case setupRowScope:
		return m.openCompareScopePicker(d)
	case setupRowMethod:
		return m.openCompareMethodPicker(d)
	case setupRowCompare:
		if d.Run.Target.IsZero() {
			d.Run.Err = fmt.Errorf("choose a target org first")
			return nil
		}
		if d.Run.Target.Equal(d.Run.Source) {
			d.Run.Err = fmt.Errorf("source and target are the same")
			return nil
		}
		if len(d.Run.Scope) == 0 {
			d.Run.Err = fmt.Errorf("pick at least one metadata type (Scope)")
			return nil
		}
		return m.startCompare(d)
	}
	return nil
}

func (m *Model) activateCompareResult(d *orgData) tea.Cmd {
	if d.Run == nil || d.Run.Phase != comparePhaseInventory || d.Diff != nil {
		return nil
	}
	return m.openCompareDiff(d)
}

func (m *Model) newCompareRun() *compareRun {
	var source endpoint
	if len(m.orgs) > 0 {
		source = orgEndpoint(m.orgs[m.selected].Username)
	}
	return &compareRun{
		Source: source,
		Scope:  nil, // unticked by default — user picks types in the scope modal
		Method: compareMethodAuto,
		Phase:  comparePhaseSetup,
	}
}

func (m *Model) activateSavedRow(d *orgData) tea.Cmd {
	row, ok := d.SavedList.Selected()
	if !ok {
		return nil
	}
	if row.Kind == savedRowComparison {
		return m.openSavedComparison(d, row.ID)
	}
	return m.rerunEditSelected(d)
}

func (m *Model) openSavedComparison(d *orgData, id string) tea.Cmd {
	if m.devProjects == nil {
		return nil
	}
	sc, err := m.devProjects.GetSavedComparison(id)
	if err != nil {
		m.flash("open: " + err.Error())
		return nil
	}
	run, err := deserializeCompareRun(sc)
	if err != nil {
		m.flash("open: " + err.Error())
		return nil
	}
	run.OriginSavedID = sc.ID
	run.OriginSavedName = sc.Name
	run.OpenedSavedAt = sc.UpdatedAt
	if run.OpenedSavedAt.IsZero() {
		run.OpenedSavedAt = sc.CreatedAt
	}
	d.Run = run
	d.syncInventoryList()
	d.InventoryList.ResetCursor()
	m.compareSubtabIdx = compareSubtabResultIdx
	return nil
}

func (m *Model) rerunEditSelected(d *orgData) tea.Cmd {
	row, ok := d.SavedList.Selected()
	if !ok {
		return nil
	}
	if row.Kind == savedRowComparison && m.devProjects != nil {
		sc, err := m.devProjects.GetSavedComparison(row.ID)
		if err != nil {
			m.flash("edit: " + err.Error())
			return nil
		}
		m.openCompareEditModal(compareEditSeed{
			OriginID:   sc.ID,
			OriginName: sc.Name,
			Source:     sc.Source,
			Target:     sc.Target,
			Scope:      splitScope(sc.Scope),
			Method:     parseCompareMethod(sc.Method),
		})
		return nil
	}
	if m.settings != nil {
		for _, def := range m.settings.CompareDefs() {
			if def.Name != row.Name {
				continue
			}
			d.Run = &compareRun{
				Source: orgEndpoint(def.Source),
				Target: orgEndpoint(def.Target),
				Scope:  def.Scope,
				Method: parseCompareMethod(def.Method),
				Phase:  comparePhaseSetup,
			}
			d.SetupCursor = len(compareSetupRowsFor(d)) - 1 // land on Compare
			m.compareSubtabIdx = compareSubtabNewIdx
			return nil
		}
	}
	return nil
}

func (m *Model) editCurrentCompareInSetup(d *orgData) tea.Cmd {
	if d.Run == nil {
		return nil
	}
	if d.Run.OriginSavedID != "" {
		m.openCompareEditModal(compareEditSeed{
			OriginID:   d.Run.OriginSavedID,
			OriginName: d.Run.OriginSavedName,
			Source:     d.Run.Source.OrgRef(),
			Target:     d.Run.Target.OrgRef(),
			Scope:      d.Run.Scope,
			Method:     d.Run.Method,
		})
		return nil
	}
	d.Run.Phase = comparePhaseSetup
	d.Run.Err = nil
	d.Diff = nil
	d.SetupCursor = len(compareSetupRowsFor(d)) - 1
	m.compareSubtabIdx = compareSubtabNewIdx
	return nil
}

func (m *Model) saveSelectedAsTemplate(d *orgData) tea.Cmd {
	row, ok := d.SavedList.Selected()
	if !ok || row.Kind != savedRowComparison || m.settings == nil {
		return nil
	}
	defs := m.settings.CompareDefs()
	defs = append(defs, settingsCompareDefFromRow(row))
	m.settings.SetCompareDefs(defs)
	if m.saveSettings("saved as template: " + row.Name) {
		d.SavedLoaded = false
	}
	return nil
}

func (m *Model) deleteSelectedSaved(d *orgData) tea.Cmd {
	row, ok := d.SavedList.Selected()
	if !ok {
		return nil
	}
	if row.Kind == savedRowComparison && m.devProjects != nil {
		if err := m.devProjects.DeleteSavedComparison(row.ID); err != nil {
			m.flash("delete: " + err.Error())
			return nil
		}
		m.flash("deleted " + row.Name)
		d.SavedLoaded = false
		return nil
	}
	if m.settings != nil {
		var keep []settings.CompareDef
		for _, def := range m.settings.CompareDefs() {
			if def.Name != row.Name {
				keep = append(keep, def)
			}
		}
		m.settings.SetCompareDefs(keep)
		if m.saveSettings("deleted template: " + row.Name) {
			d.SavedLoaded = false
		}
	}
	return nil
}

func settingsCompareDefFromRow(row CompareDefRow) settings.CompareDef {
	return settings.CompareDef{
		Name:   row.Name + " (template)",
		Source: row.Source,
		Target: row.Target,
		Scope:  splitScope(row.Scope),
	}
}

func (m *Model) renameSelectedSaved(d *orgData) tea.Cmd {
	row, ok := d.SavedList.Selected()
	if !ok || row.Kind != savedRowComparison || m.devProjects == nil {
		return nil
	}
	id := row.ID
	state := editModalState{
		Title:       "Rename comparison",
		Hint:        "new name",
		InitialBody: row.Name,
		SuccessMsg:  "renamed",
		Save: func(name string, _ any) error {
			if err := m.devProjects.RenameSavedComparison(id, name); err != nil {
				return err
			}
			d.SavedLoaded = false
			return nil
		},
	}
	return m.openEditModal(state)
}

func settingsCompareDef(name string, run *compareRun) settings.CompareDef {
	return settings.CompareDef{
		Name:   name,
		Source: run.Source.OrgRef(),
		Target: run.Target.OrgRef(),
		Scope:  append([]string(nil), run.Scope...),
		Method: run.Method.String(),
	}
}

func parseCompareMethod(s string) compareMethod {
	switch s {
	case "Tooling":
		return compareMethodTooling
	case "Metadata API":
		return compareMethodMetadataAPI
	default:
		return compareMethodAuto
	}
}

var _ = diff.StatusSame // keep diff import referenced if trimmed later
