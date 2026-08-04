package ui

// /soql Saved + History subtabs.
//
// Both subtabs are backed by ListView[T] living on orgData, just
// like every other list surface in sf-deck. That's the contract
// the chip system + listSurface registry threads through; living
// outside it (which we tried first by parking snapshots on Model)
// makes V / N / M / project chips fight the dispatcher.
//
// Saved is the curated library: named queries the user authored,
// taggable, pinnable to DevProjects, fully managed (rename / edit /
// duplicate / delete). The data is org-agnostic but the snapshot
// is reloaded fresh per-org from the same SQLite store.
//
// History is the read-only execution log: every SOQL run lands
// here scoped to the org it ran against. The useful gesture is
// "load this body back into the editor."

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// ----- Saved subtab -----------------------------------------------

// reloadSOQLSaved refreshes d.SOQLSavedList from the devproject
// store. Idempotent — Match is set lazily on first call. The
// store read is the same regardless of which org is active; we
// keep the snapshot per-org so the chip + listSurface contracts
// (which all thread *orgData through callbacks) work without
// special casing.
func (m *Model) reloadSOQLSaved(d *orgData) {
	if d == nil {
		return
	}
	if m.devProjects == nil {
		d.SOQLSavedList.Set(nil)
		d.SOQLSavedLoaded = true
		return
	}
	saved, err := m.devProjects.ListSavedQueries()
	if err != nil {
		saved = nil
	}
	if !d.SOQLSavedList.HasMatch() {
		installSearch(&d.SOQLSavedList, uilayout.MatchSpec[devproject.SavedQuery]{
			Any: func(q devproject.SavedQuery) string {
				return strings.ToLower(q.Name + " " + q.Description + " " + q.Body)
			},
			Field: func(q devproject.SavedQuery, field string) string {
				switch field {
				case "Name":
					return strings.ToLower(q.Name)
				case "Description":
					return strings.ToLower(q.Description)
				case "Body":
					return strings.ToLower(q.Body)
				}
				return ""
			},
			Fields:  []string{"Name", "Description", "Body"},
			Primary: "Name",
		})
	}
	d.SOQLSavedList.Set(saved)
	d.SOQLSavedLoaded = true
}

// soqlSavedCols defines the Saved subtab's columns.
func soqlSavedCols() []uilayout.ListColumn {
	return schemaListColumns(soqlSavedColumnSchema())
}

// soqlSavedListSurface renders the Saved-queries grid. Standard
// listSurface shape with State/Search/Match/Cursor all reading
// from orgData — same contract as every other list surface.
var soqlSavedListSurface = listSurface{
	State:       func(d *orgData) *uilayout.ListTableState { return &d.SOQLSavedTable },
	Cols:        soqlSavedCols,
	SearchPtr:   func(d *orgData) *searchState { return d.SOQLSavedList.SearchPtr() },
	MoveCursor:  func(d *orgData, n int) { d.SOQLSavedList.MoveBy(n) },
	ResetCursor: func(d *orgData) { d.SOQLSavedList.ResetCursor() },
	BuildRenderModel: func(m Model, d *orgData) (listRenderModel, bool) {
		if d == nil {
			return listRenderModel{}, false
		}
		resolved := mustResolveColumns(soqlSavedColumnSchema())
		cols := resolved.ListColumns()
		installListViewOrderRows(&d.SOQLSavedList, &d.SOQLSavedTable, cols,
			func(items []devproject.SavedQuery, row, col int) string {
				return resolvedSortCellForListColumn(resolved, items, cols, row, col)
			})
		entries := d.SOQLSavedList.Filtered()
		tagsByID := bulkTagsForSavedQueries(m, entries)
		return listRenderModel{
			Title:  fmt.Sprintf("SAVED · %d", len(entries)),
			State:  &d.SOQLSavedTable,
			Search: d.SOQLSavedList.SearchPtr(),
			Cols:   cols,
			N:      len(entries),
			Cursor: d.SOQLSavedList.Cursor(),
			Cell: func(row, col int) string {
				if row < 0 || row >= len(entries) {
					return ""
				}
				if col < 0 || col >= len(cols) {
					return ""
				}
				if cols[col].Name == "Tags" {
					return tagsByID[entries[row].ID]
				}
				return resolvedCellByID(resolved, entries[row], cols[col].Name)
			},
			Recolor: func(row, col int, base lipgloss.Style) lipgloss.Style {
				if col == 0 {
					return base.Foreground(theme.Cyan)
				}
				if col == 4 {
					return base.Foreground(theme.Muted)
				}
				return base
			},
			Empty: "  no saved queries — press " + firstPretty(Keys.SOQLSave) + " on the Editor",
			FooterExtras: firstPretty(Keys.OpenDefault) + " load · " +
				firstPretty(Keys.SOQLRename) + " rename · " +
				firstPretty(Keys.SOQLDuplicate) + " duplicate · " +
				firstPretty(Keys.SOQLDelete) + " delete",
			DataVersion: listVersionWithStore(d.SOQLSavedList.Version(), m),
		}, true
	},
}

// identityFromSOQLSaved exposes the cursored saved row for the
// unified tag picker / collect / openable lookup machinery.
func identityFromSOQLSaved(m Model) (ItemIdentity, bool) {
	d, ok := m.activeOrgState()
	if !ok {
		return ItemIdentity{}, false
	}
	q, ok := d.SOQLSavedList.Selected()
	if !ok {
		return ItemIdentity{}, false
	}
	return ItemIdentity{
		Kind:  devproject.KindSOQLQuery,
		Ref:   q.ID,
		Label: q.Name,
	}, true
}

// ----- History subtab ---------------------------------------------

func (m *Model) reloadSOQLHistory(d *orgData) {
	if d == nil {
		return
	}
	if m.devProjects == nil {
		d.SOQLHistoryList.Set(nil)
		d.SOQLHistoryLoaded = true
		return
	}
	hist, err := m.devProjects.ListSOQLHistory(d.username, 200)
	if err != nil {
		hist = nil
	}
	if !d.SOQLHistoryList.HasMatch() {
		installSearch(&d.SOQLHistoryList, uilayout.MatchSpec[devproject.SOQLHistoryEntry]{
			Any: func(e devproject.SOQLHistoryEntry) string {
				return strings.ToLower(e.Body + " " + e.Error)
			},
			Field: func(e devproject.SOQLHistoryEntry, field string) string {
				switch field {
				case "Body":
					return strings.ToLower(e.Body)
				case "Status":
					if e.Error != "" {
						return "error"
					}
					return "ok"
				}
				return ""
			},
			Fields:  []string{"Body", "Status"},
			Primary: "Body",
		})
	}
	d.SOQLHistoryList.Set(hist)
	d.SOQLHistoryLoaded = true
}

func soqlHistoryCols() []uilayout.ListColumn {
	return schemaListColumns(soqlHistoryColumnSchema())
}

var soqlHistoryListSurface = listSurface{
	State:       func(d *orgData) *uilayout.ListTableState { return &d.SOQLHistoryTable },
	Cols:        soqlHistoryCols,
	SearchPtr:   func(d *orgData) *searchState { return d.SOQLHistoryList.SearchPtr() },
	MoveCursor:  func(d *orgData, n int) { d.SOQLHistoryList.MoveBy(n) },
	ResetCursor: func(d *orgData) { d.SOQLHistoryList.ResetCursor() },
	BuildRenderModel: func(m Model, d *orgData) (listRenderModel, bool) {
		if d == nil {
			return listRenderModel{}, false
		}
		resolved := mustResolveColumns(soqlHistoryColumnSchema())
		cols := resolved.ListColumns()
		installListViewOrderRows(&d.SOQLHistoryList, &d.SOQLHistoryTable, cols,
			func(items []devproject.SOQLHistoryEntry, row, col int) string {
				return resolvedSortCellForListColumn(resolved, items, cols, row, col)
			})
		entries := d.SOQLHistoryList.Filtered()
		title := fmt.Sprintf("HISTORY · %d", len(entries))
		if d.username != "" {
			title += " · " + d.username
		}
		return listRenderModel{
			Title:  title,
			State:  &d.SOQLHistoryTable,
			Search: d.SOQLHistoryList.SearchPtr(),
			Cols:   cols,
			N:      len(entries),
			Cursor: d.SOQLHistoryList.Cursor(),
			Cell: func(row, col int) string {
				if row < 0 || row >= len(entries) {
					return ""
				}
				return resolvedCellForListColumn(resolved, entries, cols, row, col)
			},
			Recolor: func(row, col int, base lipgloss.Style) lipgloss.Style {
				if row < 0 || row >= len(entries) {
					return base
				}
				if entries[row].Error != "" {
					if col == 4 {
						return base.Foreground(theme.Red)
					}
					return base.Foreground(theme.Muted)
				}
				if col == 0 {
					return base.Foreground(theme.Muted)
				}
				return base
			},
			Empty: "  no executions yet on this org — run something on the Editor",
			FooterExtras: firstPretty(Keys.OpenDefault) + " load · " +
				firstPretty(Keys.SOQLSave) + " save as",
			DataVersion: listVersionWithStore(d.SOQLHistoryList.Version(), m),
		}, true
	},
}

// ----- Renderers --------------------------------------------------

func (m Model) renderSOQLSaved(w, innerH int) string {
	d, ok := m.activeOrgState()
	if !ok {
		return noOrgPlaceholder()
	}
	if !d.SOQLSavedLoaded {
		return theme.Subtle.Render("  loading saved queries…")
	}
	inner := w - 4
	chips := m.stripRows(domainSOQLSaved, "*")
	dash := m.renderDashboard("VIEWS", chips, m.soqlSavedChipIdx(), inner)
	var lines []string
	if dash != "" {
		lines = append(lines, dash)
	}
	model, ok := soqlSavedListSurface.BuildRenderModel(m, d)
	if !ok {
		return theme.Subtle.Render("  saved queries unavailable")
	}
	usedAbove := usedLines(lines)
	budget := innerH - usedAbove
	lines = append(lines, renderListModel(m, model, m.focus, inner, budget)...)
	return strings.Join(lines, "\n")
}

func (m Model) renderSOQLHistory(w, innerH int) string {
	d, ok := m.activeOrgState()
	if !ok {
		return noOrgPlaceholder()
	}
	if !d.SOQLHistoryLoaded {
		return theme.Subtle.Render("  loading history…")
	}
	inner := w - 4
	chips := m.stripRows(domainSOQLHistory, "*")
	dash := m.renderDashboard("VIEWS", chips, m.soqlHistoryChipIdx(), inner)
	var lines []string
	if dash != "" {
		lines = append(lines, dash)
	}
	model, ok := soqlHistoryListSurface.BuildRenderModel(m, d)
	if !ok {
		return theme.Subtle.Render("  history unavailable")
	}
	usedAbove := usedLines(lines)
	budget := innerH - usedAbove
	lines = append(lines, renderListModel(m, model, m.focus, inner, budget)...)
	return strings.Join(lines, "\n")
}

// ----- Activate (Enter) handlers ----------------------------------

func (m *Model) loadCursoredSavedEntry() {
	d, ok := m.activeOrgState()
	if !ok {
		return
	}
	q, ok := d.SOQLSavedList.Selected()
	if !ok {
		return
	}
	if m.devProjects != nil {
		_ = m.devProjects.TouchSavedQuery(q.ID)
	}
	resolved := substituteSOQL(q.Body, m.substitutionsFor(d))
	m.soqlInput.SetValue(resolved)
	// Loading a different query: drop the previous query's results so
	// the editor doesn't sit above stale rows (or a stale error) until
	// the user re-runs. Search + column sort stay — they're sticky per
	// session by design, same as across manual re-runs.
	m.soqlResult = sf.QueryResult{}
	m.soqlErr = nil
	m.soqlRowCur = 0
	m.soqlEditing = true
	m.soqlInput.Focus()
	m.soqlSubtabIdx = 0
	m.soqlEditingSavedID = q.ID
	d.SOQLSavedLoaded = false
	if resolved != q.Body {
		m.flash("loaded with substitutions: $ME / $ORG / $TODAY etc resolved for this org")
	}
}

func (m *Model) loadCursoredHistoryEntry() {
	d, ok := m.activeOrgState()
	if !ok {
		return
	}
	e, ok := d.SOQLHistoryList.Selected()
	if !ok {
		return
	}
	// History rows ran with their values already resolved (the
	// query that hit Salesforce was the substituted form), but
	// re-applying is a no-op for those — the key reason to call
	// substitute here is for rows that originated as saved
	// queries but were captured with the raw token string
	// somehow. Cheap; keeps the contract uniform.
	resolved := substituteSOQL(e.Body, m.substitutionsFor(d))
	m.soqlInput.SetValue(resolved)
	// Loading a different query: drop the previous query's results so
	// the editor doesn't sit above stale rows (or a stale error) until
	// the user re-runs. Search + column sort stay — they're sticky per
	// session by design, same as across manual re-runs.
	m.soqlResult = sf.QueryResult{}
	m.soqlErr = nil
	m.soqlRowCur = 0
	m.soqlEditing = true
	m.soqlInput.Focus()
	m.soqlSubtabIdx = 0
	m.soqlEditingSavedID = ""
}

// ----- Save / delete / rename / duplicate -------------------------

func (m Model) handleSOQLSave() (Model, tea.Cmd) {
	body := strings.TrimSpace(m.soqlInput.Value())
	if body == "" {
		m.flash("nothing to save — type a query first")
		return m, nil
	}
	if m.devProjects == nil {
		m.flash("dev-projects store unavailable — can't save query")
		return m, nil
	}
	if m.soqlEditingSavedID != "" {
		existing, err := m.devProjects.GetSavedQuery(m.soqlEditingSavedID)
		if err != nil {
			m.soqlEditingSavedID = ""
		} else {
			if err := m.devProjects.UpdateSavedQuery(
				existing.ID, existing.Name, existing.Description, body,
			); err != nil {
				m.flash("update failed: " + err.Error())
				return m, nil
			}
			m.invalidateSOQLSaved()
			m.flash("updated: " + existing.Name)
			return m, nil
		}
	}
	name := defaultSavedQueryName(body)
	q, err := m.devProjects.CreateSavedQuery(name, "", body)
	if err != nil {
		m.flash("save failed: " + err.Error())
		return m, nil
	}
	m.soqlEditingSavedID = q.ID
	m.invalidateSOQLSaved()
	m.flash("saved query: " + q.Name)
	return m, nil
}

func (m Model) handleSOQLSaveAs() (Model, tea.Cmd) {
	body := strings.TrimSpace(m.soqlInput.Value())
	if body == "" {
		m.flash("nothing to save — type a query first")
		return m, nil
	}
	if m.devProjects == nil {
		return m, nil
	}
	name := defaultSavedQueryName(body)
	q, err := m.devProjects.CreateSavedQuery(name, "", body)
	if err != nil {
		m.flash("save failed: " + err.Error())
		return m, nil
	}
	m.soqlEditingSavedID = q.ID
	m.invalidateSOQLSaved()
	m.flash("saved as new: " + q.Name)
	return m, nil
}

func (m Model) handleSOQLLibraryDelete() (Model, tea.Cmd) {
	if m.currentSubtab() != SubtabSOQLSaved {
		return m, nil
	}
	d, ok := m.activeOrgState()
	if !ok {
		return m, nil
	}
	q, ok := d.SOQLSavedList.Selected()
	if !ok {
		return m, nil
	}
	if m.devProjects == nil {
		return m, nil
	}
	if err := m.devProjects.DeleteSavedQuery(q.ID); err != nil {
		m.flash("delete failed: " + err.Error())
		return m, nil
	}
	if m.soqlEditingSavedID == q.ID {
		m.soqlEditingSavedID = ""
	}
	m.invalidateSOQLSaved()
	m.flash("deleted: " + q.Name)
	return m, nil
}

func (m Model) handleSOQLDuplicate() (Model, tea.Cmd) {
	if m.currentSubtab() != SubtabSOQLSaved {
		return m, nil
	}
	d, ok := m.activeOrgState()
	if !ok {
		return m, nil
	}
	src, ok := d.SOQLSavedList.Selected()
	if !ok {
		return m, nil
	}
	if m.devProjects == nil {
		return m, nil
	}
	q, err := m.devProjects.CreateSavedQuery(
		"Copy of "+src.Name, src.Description, src.Body,
	)
	if err != nil {
		m.flash("duplicate failed: " + err.Error())
		return m, nil
	}
	m.invalidateSOQLSaved()
	m.flash("duplicated: " + q.Name)
	return m, nil
}

func (m Model) handleSOQLRename() (Model, tea.Cmd) {
	if m.currentSubtab() != SubtabSOQLSaved {
		return m, nil
	}
	d, ok := m.activeOrgState()
	if !ok {
		return m, nil
	}
	q, ok := d.SOQLSavedList.Selected()
	if !ok {
		return m, nil
	}
	if m.devProjects == nil {
		return m, nil
	}
	initial := q.Name
	if q.Description != "" {
		initial += "\n" + q.Description
	}
	cmd := m.openEditModal(editModalState{
		Title:       "Rename saved query",
		Hint:        "First line is the name; the rest is description. Enter to save.",
		InitialBody: initial,
		Multiline:   true,
		Save: func(val string, _ any) error {
			name, desc := splitNameDescription(val)
			if name == "" {
				return fmt.Errorf("name required")
			}
			return m.devProjects.UpdateSavedQuery(q.ID, name, desc, q.Body)
		},
		OnSuccess: func() tea.Cmd {
			return func() tea.Msg { return soqlSavedChangedMsg{} }
		},
	})
	return m, cmd
}

// soqlSavedChangedMsg forces a Saved snapshot reload across all
// orgs (the data is shared in the SQLite store). Captured rather
// than reusing devProjectsChangedMsg because saved queries aren't
// strictly DevProjects.
type soqlSavedChangedMsg struct{}

// invalidateSOQLSaved marks every org's saved-queries snapshot as
// stale so the next render reloads. Used after any mutation
// (create / update / delete / duplicate) since the same query can
// surface under multiple orgs in cached snapshots.
func (m *Model) invalidateSOQLSaved() {
	for _, d := range m.data {
		if d != nil {
			d.SOQLSavedLoaded = false
		}
	}
}

// invalidateSOQLHistory marks every org's history snapshot as
// stale. Called after a SOQL run so the next History view reflects
// the new row.
func (m *Model) invalidateSOQLHistory() {
	for _, d := range m.data {
		if d != nil {
			d.SOQLHistoryLoaded = false
		}
	}
}

// ----- Persist execution log --------------------------------------

func (m *Model) persistSOQLHistory(msg soqlResultMsg) {
	if m.devProjects == nil || msg.orgUser == "" {
		return
	}
	errMsg := ""
	if msg.err != nil {
		errMsg = msg.err.Error()
	}
	rowCount := 0
	if msg.err == nil {
		rowCount = msg.data.TotalSize
	}
	if _, err := m.devProjects.LogSOQLHistory(
		msg.orgUser, msg.soql, msg.tookMs, rowCount, errMsg,
	); err != nil {
		return
	}
	m.invalidateSOQLHistory()
}

// ----- Helpers ----------------------------------------------------

func collapseSOQL(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	out := strings.Builder{}
	out.Grow(len(body))
	prevSpace := false
	for _, r := range body {
		if r == '\n' || r == '\r' || r == '\t' {
			r = ' '
		}
		if r == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		out.WriteRune(r)
	}
	return out.String()
}

func defaultSavedQueryName(body string) string {
	collapsed := collapseSOQL(body)
	const cap = 50
	if len(collapsed) > cap {
		return ansi.Truncate(collapsed, cap+1, "…")
	}
	return collapsed
}

func bulkTagsForSavedQueries(m Model, entries []devproject.SavedQuery) map[string]string {
	if m.devProjects == nil || len(entries) == 0 {
		return nil
	}
	keys := make([]devproject.TagLookupKey, 0, len(entries))
	for _, q := range entries {
		keys = append(keys, devproject.TagLookupKey{
			Kind: devproject.KindSOQLQuery,
			Ref:  q.ID,
		})
	}
	m2, err := m.devProjects.TagsForItems("", keys)
	if err != nil {
		return nil
	}
	prefix := string(devproject.KindSOQLQuery) + ":"
	out := make(map[string]string, len(m2))
	for key, tags := range m2 {
		ref := strings.TrimPrefix(key, prefix)
		names := make([]string, len(tags))
		for i, t := range tags {
			names[i] = t.Name
		}
		out[ref] = strings.Join(names, ", ")
	}
	return out
}
