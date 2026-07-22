package ui

// /exec Saved + History subtabs. Mirrors tab_soql_library.go's shape
// (per-org ListViews of devproject store snapshots, listSurface
// wiring, load-into-editor + save/duplicate/delete/rename handlers).
// Differs only in storage table (saved_apex / apex_history) and the
// load-target (editor textarea instead of single-line input).

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// ----- Saved subtab -----------------------------------------------

// reloadExecSaved refreshes d.ExecSavedList from the devproject store.
func (m *Model) reloadExecSaved(d *orgData) {
	if d == nil {
		return
	}
	if m.devProjects == nil {
		d.ExecSavedList.Set(nil)
		d.ExecSavedLoaded = true
		return
	}
	saved, err := m.devProjects.ListSavedApex()
	if err != nil {
		saved = nil
	}
	if !d.ExecSavedList.HasMatch() {
		installSearch(&d.ExecSavedList, uilayout.MatchSpec[devproject.SavedApex]{
			Any: func(a devproject.SavedApex) string {
				return strings.ToLower(a.Name + " " + a.Description + " " + a.Body)
			},
			Field: func(a devproject.SavedApex, field string) string {
				switch field {
				case "Name":
					return strings.ToLower(a.Name)
				case "Description":
					return strings.ToLower(a.Description)
				case "Body":
					return strings.ToLower(a.Body)
				}
				return ""
			},
			Fields:  []string{"Name", "Description", "Body"},
			Primary: "Name",
		})
	}
	d.ExecSavedList.Set(saved)
	d.ExecSavedLoaded = true
}

func execSavedCols() []uilayout.ListColumn {
	return schemaListColumns(execSavedColumnSchema())
}

var execSavedListSurface = listSurface{
	State:       func(d *orgData) *uilayout.ListTableState { return &d.ExecSavedTable },
	Cols:        execSavedCols,
	SearchPtr:   func(d *orgData) *searchState { return d.ExecSavedList.SearchPtr() },
	MoveCursor:  func(d *orgData, n int) { d.ExecSavedList.MoveBy(n) },
	ResetCursor: func(d *orgData) { d.ExecSavedList.ResetCursor() },
	BuildRenderModel: func(m Model, d *orgData) (listRenderModel, bool) {
		if d == nil {
			return listRenderModel{}, false
		}
		resolved := mustResolveColumns(execSavedColumnSchema())
		cols := resolved.ListColumns()
		installListViewOrderRows(&d.ExecSavedList, &d.ExecSavedTable, cols,
			func(items []devproject.SavedApex, row, col int) string {
				return resolvedSortCellForListColumn(resolved, items, cols, row, col)
			})
		entries := d.ExecSavedList.Filtered()
		return listRenderModel{
			Title:  fmt.Sprintf("SAVED APEX · %d", len(entries)),
			State:  &d.ExecSavedTable,
			Search: d.ExecSavedList.SearchPtr(),
			Cols:   cols,
			N:      len(entries),
			Cursor: d.ExecSavedList.Cursor(),
			Cell: func(row, col int) string {
				if row < 0 || row >= len(entries) {
					return ""
				}
				return resolvedCellForListColumn(resolved, entries, cols, row, col)
			},
			Recolor: func(row, col int, base lipgloss.Style) lipgloss.Style {
				if col == 0 {
					return base.Foreground(theme.Cyan)
				}
				if col == 3 {
					return base.Foreground(theme.Muted)
				}
				return base
			},
			Empty: "  no saved snippets — press " + firstPretty(Keys.ExecSave) + " on the Editor",
			FooterExtras: firstPretty(Keys.OpenDefault) + " load · " +
				firstPretty(Keys.ExecRename) + " rename · " +
				firstPretty(Keys.ExecDuplicate) + " duplicate · " +
				firstPretty(Keys.ExecDelete) + " delete",
			DataVersion: listVersionWithStore(d.ExecSavedList.Version(), m),
		}, true
	},
}

// ----- History subtab ---------------------------------------------

func (m *Model) reloadExecHistory(d *orgData) {
	if d == nil {
		return
	}
	if m.devProjects == nil {
		d.ExecHistoryList.Set(nil)
		d.ExecHistoryLoaded = true
		return
	}
	hist, err := m.devProjects.ListApexHistory(d.username, 200)
	if err != nil {
		hist = nil
	}
	if !d.ExecHistoryList.HasMatch() {
		installSearch(&d.ExecHistoryList, uilayout.MatchSpec[devproject.ApexHistoryEntry]{
			Any: func(e devproject.ApexHistoryEntry) string {
				return strings.ToLower(e.Body + " " + e.CompileProblem + " " + e.ExceptionMessage)
			},
			Field: func(e devproject.ApexHistoryEntry, field string) string {
				switch field {
				case "Body":
					return strings.ToLower(e.Body)
				case "Status":
					switch {
					case !e.Compiled:
						return "compile_error"
					case !e.Success:
						return "runtime_error"
					}
					return "ok"
				}
				return ""
			},
			Fields:  []string{"Body", "Status"},
			Primary: "Body",
		})
	}
	d.ExecHistoryList.Set(hist)
	d.ExecHistoryLoaded = true
}

func execHistoryCols() []uilayout.ListColumn {
	return schemaListColumns(execHistoryColumnSchema())
}

var execHistoryListSurface = listSurface{
	State:       func(d *orgData) *uilayout.ListTableState { return &d.ExecHistoryTable },
	Cols:        execHistoryCols,
	SearchPtr:   func(d *orgData) *searchState { return d.ExecHistoryList.SearchPtr() },
	MoveCursor:  func(d *orgData, n int) { d.ExecHistoryList.MoveBy(n) },
	ResetCursor: func(d *orgData) { d.ExecHistoryList.ResetCursor() },
	BuildRenderModel: func(m Model, d *orgData) (listRenderModel, bool) {
		if d == nil {
			return listRenderModel{}, false
		}
		resolved := mustResolveColumns(execHistoryColumnSchema())
		cols := resolved.ListColumns()
		installListViewOrderRows(&d.ExecHistoryList, &d.ExecHistoryTable, cols,
			func(items []devproject.ApexHistoryEntry, row, col int) string {
				return resolvedSortCellForListColumn(resolved, items, cols, row, col)
			})
		entries := d.ExecHistoryList.Filtered()
		title := fmt.Sprintf("EXEC HISTORY · %d", len(entries))
		if d.username != "" {
			title += " · " + d.username
		}
		return listRenderModel{
			Title:  title,
			State:  &d.ExecHistoryTable,
			Search: d.ExecHistoryList.SearchPtr(),
			Cols:   cols,
			N:      len(entries),
			Cursor: d.ExecHistoryList.Cursor(),
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
				e := entries[row]
				if !e.Compiled || !e.Success {
					if col == 3 {
						return base.Foreground(theme.Red)
					}
					return base.Foreground(theme.Muted)
				}
				if col == 0 {
					return base.Foreground(theme.Muted)
				}
				return base
			},
			Empty: "  no runs yet — run something on the Editor",
			FooterExtras: firstPretty(Keys.OpenDefault) + " load · " +
				firstPretty(Keys.ExecSave) + " save as",
			DataVersion: listVersionWithStore(d.ExecHistoryList.Version(), m),
		}, true
	},
}

// ----- Renderers (replacements for stage 3 stubs) ----------------

func (m Model) renderExecSavedSubtab(w, innerH int) string {
	d, ok := m.activeOrgState()
	if !ok {
		return noOrgPlaceholder()
	}
	// Lazy reload on render so post-mutation invalidations
	// (rename / delete / duplicate / save) repopulate the list
	// without needing the user to leave + re-enter the subtab.
	// reloadExecSaved is cheap (one local SQLite query) so doing
	// it inline on a single render after mutation is fine.
	if !d.ExecSavedLoaded {
		mm := m
		(&mm).reloadExecSaved(d)
	}
	inner := w - 4
	model, ok := execSavedListSurface.BuildRenderModel(m, d)
	if !ok {
		return theme.Subtle.Render("  saved snippets unavailable")
	}
	lines := renderListModel(m, model, m.focus, inner, innerH)
	return strings.Join(lines, "\n")
}

func (m Model) renderExecHistorySubtab(w, innerH int) string {
	d, ok := m.activeOrgState()
	if !ok {
		return noOrgPlaceholder()
	}
	if !d.ExecHistoryLoaded {
		mm := m
		(&mm).reloadExecHistory(d)
	}
	inner := w - 4
	model, ok := execHistoryListSurface.BuildRenderModel(m, d)
	if !ok {
		return theme.Subtle.Render("  history unavailable")
	}
	lines := renderListModel(m, model, m.focus, inner, innerH)
	return strings.Join(lines, "\n")
}

// ----- Activate (Enter) handlers ---------------------------------

// loadCursoredExecSavedEntry — Enter on a Saved row loads it into
// the editor and flips back to the Editor subtab. Same shape as
// loadCursoredSavedEntry on /soql.
func (m *Model) loadCursoredExecSavedEntryReal() {
	d, ok := m.activeOrgState()
	if !ok {
		return
	}
	a, ok := d.ExecSavedList.Selected()
	if !ok {
		return
	}
	if m.devProjects != nil {
		_ = m.devProjects.TouchSavedApex(a.ID)
	}
	m.execInput.SetValue(a.Body)
	m.execEditing = false
	m.execSubtabIdx = execSubtabIndex(SubtabExecEditor)
	m.execEditingSavedID = a.ID
	d.ExecSavedLoaded = false
}

// loadCursoredExecHistoryEntry — Enter on a History row loads the
// body back into the editor.
func (m *Model) loadCursoredExecHistoryEntryReal() {
	d, ok := m.activeOrgState()
	if !ok {
		return
	}
	e, ok := d.ExecHistoryList.Selected()
	if !ok {
		return
	}
	m.execInput.SetValue(e.Body)
	m.execEditing = false
	m.execSubtabIdx = execSubtabIndex(SubtabExecEditor)
	m.execEditingSavedID = ""
}

// ----- Save / delete / rename / duplicate ------------------------

// handleExecSave saves the current editor body — updating the
// existing snippet when execEditingSavedID is set, otherwise
// creating a fresh one. Uses the first non-empty line as the
// default name; the user can rename via R from the Saved subtab.
func (m Model) handleExecSave() (tea.Model, tea.Cmd) {
	if m.devProjects == nil {
		m.flash("dev-projects store unavailable")
		return m, nil
	}
	body := strings.TrimSpace(m.execInput.Value())
	if body == "" {
		m.flash("nothing to save — write some Apex first")
		return m, nil
	}
	name := firstNonEmptyLine(body)
	if name == "" {
		name = "untitled"
	}
	if m.execEditingSavedID != "" {
		if err := m.devProjects.UpdateSavedApex(m.execEditingSavedID, name, "", body); err != nil {
			m.flash("save: " + err.Error())
			return m, nil
		}
		m.flash("updated " + m.execEditingSavedID)
	} else {
		a, err := m.devProjects.CreateSavedApex(name, "", body)
		if err != nil {
			m.flash("save: " + err.Error())
			return m, nil
		}
		m.execEditingSavedID = a.ID
		m.flash("saved as " + a.ID)
	}
	if d, ok := m.activeOrgState(); ok {
		d.ExecSavedLoaded = false
	}
	return m, nil
}

// handleExecLibraryDelete deletes the cursored Saved snippet.
func (m Model) handleExecLibraryDelete() (tea.Model, tea.Cmd) {
	if m.devProjects == nil {
		return m, nil
	}
	d, ok := m.activeOrgState()
	if !ok {
		return m, nil
	}
	a, ok := d.ExecSavedList.Selected()
	if !ok {
		return m, nil
	}
	if err := m.devProjects.DeleteSavedApex(a.ID); err != nil {
		m.flash("delete: " + err.Error())
		return m, nil
	}
	if m.execEditingSavedID == a.ID {
		m.execEditingSavedID = ""
	}
	d.ExecSavedLoaded = false
	m.flash("deleted " + a.Name)
	return m, nil
}

// handleExecRename opens an edit modal pre-populated with the
// cursored snippet's name + description.
func (m Model) handleExecRename() (tea.Model, tea.Cmd) {
	if m.currentSubtab() != SubtabExecSaved {
		return m, nil
	}
	d, ok := m.activeOrgState()
	if !ok {
		return m, nil
	}
	a, ok := d.ExecSavedList.Selected()
	if !ok {
		return m, nil
	}
	if m.devProjects == nil {
		return m, nil
	}
	initial := a.Name
	if a.Description != "" {
		initial += "\n" + a.Description
	}
	cmd := m.openEditModal(editModalState{
		Title:       "Rename saved snippet",
		Hint:        "First line is the name; the rest is description. Enter to save.",
		InitialBody: initial,
		Multiline:   true,
		Save: func(val string, _ any) error {
			name, desc := splitNameDescription(val)
			if name == "" {
				return fmt.Errorf("name required")
			}
			return m.devProjects.UpdateSavedApex(a.ID, name, desc, a.Body)
		},
		OnSuccess: func() tea.Cmd {
			if dd, ok := m.activeOrgState(); ok {
				dd.ExecSavedLoaded = false
			}
			return nil
		},
	})
	return m, cmd
}

// handleExecDuplicate creates "Copy of X" from the cursored Saved
// snippet.
func (m Model) handleExecDuplicate() (tea.Model, tea.Cmd) {
	if m.devProjects == nil {
		return m, nil
	}
	d, ok := m.activeOrgState()
	if !ok {
		return m, nil
	}
	src, ok := d.ExecSavedList.Selected()
	if !ok {
		return m, nil
	}
	if _, err := m.devProjects.CreateSavedApex("Copy of "+src.Name, src.Description, src.Body); err != nil {
		m.flash("duplicate: " + err.Error())
		return m, nil
	}
	d.ExecSavedLoaded = false
	m.flash("duplicated " + src.Name)
	return m, nil
}

// ----- Helpers ----------------------------------------------------

// collapseApex flattens a multi-line Apex body to a single-line
// preview for the Saved + History tables. Mirrors collapseSOQL.
func collapseApex(body string) string {
	out := strings.ReplaceAll(body, "\r", " ")
	out = strings.ReplaceAll(out, "\n", " ")
	out = strings.ReplaceAll(out, "\t", " ")
	for strings.Contains(out, "  ") {
		out = strings.ReplaceAll(out, "  ", " ")
	}
	return strings.TrimSpace(out)
}

// firstNonEmptyLine returns the first line of s that isn't blank,
// truncated to 60 chars. Used as a default snippet name when the
// user saves without explicitly naming.
func firstNonEmptyLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if len(ln) > 60 {
			ln = ln[:60]
		}
		return ln
	}
	return ""
}
