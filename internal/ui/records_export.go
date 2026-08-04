package ui

// Records-list export.
//
// Pressing x on a records-shaped surface (TabRecords, /objects ·
// records subtab, /users · All users) opens the same two-step
// flow as SOQL export: format picker (xlsx / csv / json) → path
// picker (pre-populated with <export-dir>/<context>-<timestamp>.<ext>).
//
// Records-list data is already in memory — we don't re-run the
// SOQL. Same exporters package reused so a CSV from /records and a
// CSV from /soql share their row shape + header style.
//
// Why a separate module: the source data shape differs slightly.
// SOQL results are []map[string]any with arbitrary columns; records
// lists carry a typed RecordsList that already projects the chip's
// SELECT columns in order.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/exporters"
	"github.com/Jacob-Stokes/sf-deck/internal/exporters/bulk"
	exsoql "github.com/Jacob-Stokes/sf-deck/internal/exporters/soql"
	"github.com/Jacob-Stokes/sf-deck/internal/securefile"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// recordsExportSurfaceActive reports whether the user is on a
// surface that the export gesture should apply to. Lets the
// global key dispatcher pre-filter so a stray x on /home or /apex
// doesn't accidentally fire — and so other tabs are free to bind
// x to their own actions.
func (m Model) recordsExportSurfaceActive() bool {
	switch m.tab() {
	case TabRecords:
		_, sobj := m.activeRecordsSObject()
		return sobj != ""
	case TabObjectDetail:
		return m.currentSubtab() == SubtabRecords
	case TabUsers:
		return m.currentSubtab() == SubtabUsersAll
	}
	return false
}

// triggerRecordsExport opens the format picker for the active
// records-shaped surface. Resolves the source — /records,
// /objects · records subtab, /users · All users — and falls
// through with a flash when there's nothing exportable.
//
// When the active /records chip is a *preview* (list.Done=false and
// TotalSize > rows-in-memory), a scope picker runs first:
//
//	Visible — write the in-memory rows. Same as today.
//	Full    — Bulk API export of the chip's SOQL with no LIMIT.
//	          Streams direct-to-disk as CSV (forced format), bypasses
//	          the format picker since Bulk's wire format IS CSV.
//
// /users export keeps the legacy in-view-only flow for now — its
// SOQL plumbing isn't exposed the same way as /records chips.
func (m Model) triggerRecordsExport() (Model, tea.Cmd) {
	rows, label, ok := m.recordsExportSource()
	if !ok {
		m.flash("nothing to export — load some records first")
		return m, nil
	}
	if len(rows) == 0 {
		m.flash("no rows to export")
		return m, nil
	}

	// Scope picker for /records-style surfaces (where we have the
	// underlying SOQL) WHENEVER the chip's SOQL carries a LIMIT
	// clause. /users surface returns nil here.
	//
	// Salesforce's /query response doesn't tell us how many rows
	// match the WHERE clause unbounded — totalSize is just the count
	// returned in the response, which equals len(Records) when a
	// LIMIT was in play. So we can't *detect* "more rows on the
	// server" from the response shape. Instead: assume any LIMIT'd
	// SOQL might be hiding rows, and let the user decide. They picked
	// LIMIT 2000 to keep first-paint fast; whether to pay the cost of
	// the full set is a *separate* choice.
	if list := m.activeRecordsListForExport(); list != nil && hasLimitClause(list.Query) {
		cmd := m.openRecordsExportScopePicker(label, list, len(rows))
		return m, cmd
	}

	cmd := m.openRecordsExportFormatPicker(label, len(rows))
	return m, cmd
}

// hasLimitClause reports whether soql contains a trailing LIMIT clause.
// Matches the same shape stripTrailingLimit removes — case-insensitive,
// integer after " LIMIT " up to end-of-string.
func hasLimitClause(soql string) bool {
	trimmed := strings.TrimRight(soql, " \t\r\n")
	low := strings.ToLower(trimmed)
	i := strings.LastIndex(low, " limit ")
	if i < 0 {
		return false
	}
	tail := strings.TrimSpace(trimmed[i+len(" limit "):])
	if tail == "" {
		return false
	}
	for _, r := range tail {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// openRecordsExportFormatPicker is the original format-picker step,
// factored out so the scope picker can route into it for "Visible"
// and the bulk path can skip it for "Full".
//
// Pointer receiver so the modal-state mutation made by openChoiceModal
// (which itself takes *Model) sticks through to the caller's Model. A
// value receiver here would silently drop the modal-opened state on
// the floor because openChoiceModal would write to a local copy.
func (m *Model) openRecordsExportFormatPicker(label string, rowCount int) tea.Cmd {
	opts := []choiceOption{
		{Label: "Excel (xlsx)", Hint: "spreadsheet, headers + values", Value: string(exporters.FormatXLSX)},
		{Label: "CSV", Hint: "comma-separated text, no formatting", Value: string(exporters.FormatCSV)},
		{Label: "JSON", Hint: "array of objects, keys preserved", Value: string(exporters.FormatJSON)},
	}
	state := choiceModalState{
		Title:   "Export records · " + label,
		Hint:    fmt.Sprintf("%d rows  ·  Enter to pick format  ·  Esc to cancel", rowCount),
		Options: opts,
		Cursor:  0,
		OnSuccessTyped: func(val any) tea.Cmd {
			pick, _ := val.(string)
			return func() tea.Msg {
				return openRecordsExportPathMsg{
					Format: exporters.Format(pick),
					Label:  label,
				}
			}
		},
	}
	return m.openChoiceModal(state)
}

// openRecordsExportScopePicker is the first stage shown when the
// chip's SOQL carries a LIMIT clause — the in-view result is a slice
// by construction. Choosing Full routes into the Bulk path; choosing
// Visible routes into the existing format picker.
//
// Pointer receiver — same reason as openRecordsExportFormatPicker:
// openChoiceModal mutates the model to display the modal, and a
// value receiver would discard that mutation.
func (m *Model) openRecordsExportScopePicker(label string, list *sf.RecordsList, visible int) tea.Cmd {
	opts := []choiceOption{
		{
			Label: fmt.Sprintf("Visible (%d rows)", visible),
			Hint:  "what's in view · xlsx / csv / json",
			Value: "visible",
		},
		{
			Label: "Full dataset (Bulk API)",
			Hint:  "all WHERE-matched rows · CSV · streams direct to disk",
			Value: "full",
		},
	}
	soql := stripTrailingLimit(list.Query)
	state := choiceModalState{
		Title:   "Export records · " + label,
		Hint:    "preview chip — pick scope · Esc to cancel",
		Options: opts,
		Cursor:  0,
		OnSuccessTyped: func(val any) tea.Cmd {
			pick, _ := val.(string)
			switch pick {
			case "full":
				return func() tea.Msg {
					return bulk.OpenPathMsg{
						Label: label,
						SOQL:  soql,
					}
				}
			default:
				return func() tea.Msg {
					return openRecordsExportFormatMsg{
						Label:    label,
						RowCount: visible,
					}
				}
			}
		},
	}
	return m.openChoiceModal(state)
}

// openRecordsExportFormatMsg is the post-scope-picker message that
// re-enters the format picker once the user has confirmed Visible.
// Lets the scope picker stay a single-purpose modal instead of
// re-opening a choice modal from inside another choice modal's
// success callback.
type openRecordsExportFormatMsg struct {
	Label    string
	RowCount int
}

// stripTrailingLimit removes a trailing `LIMIT <n>` clause from a SOQL
// string. Bulk API jobs reject result-set caps — the job pulls the
// full WHERE-matched set — and the user picked Full precisely
// because they want everything. Case-insensitive, conservative: only
// strips when LIMIT is the last clause.
func stripTrailingLimit(soql string) string {
	trimmed := strings.TrimRight(soql, " \t\r\n")
	low := strings.ToLower(trimmed)
	i := strings.LastIndex(low, " limit ")
	if i < 0 {
		return trimmed
	}
	// Sanity-check that what follows " limit " is a bare integer +
	// trailing whitespace; otherwise leave the SOQL untouched so we
	// don't mangle a subquery that happens to contain LIMIT.
	tail := strings.TrimSpace(trimmed[i+len(" limit "):])
	for _, r := range tail {
		if r < '0' || r > '9' {
			return trimmed
		}
	}
	return strings.TrimRight(trimmed[:i], " \t\r\n")
}

// activeRecordsListForExport returns a pointer to the in-memory
// RecordsList for the active /records-style surface, or nil for
// non-records surfaces (e.g. /users). The pointer is read-only; we
// only need Done / TotalSize / Query off it.
func (m Model) activeRecordsListForExport() *sf.RecordsList {
	d, sobj := m.activeRecordsSObject()
	if sobj == "" || d == nil {
		return nil
	}
	r := currentRecordsResource(d, sobj)
	if r == nil || r.FetchedAt().IsZero() {
		return nil
	}
	v := r.Value()
	return &v
}

// openRecordsExportPathMsg arrives after the user picks a format.
type openRecordsExportPathMsg struct {
	Format exporters.Format
	Label  string // used as the filename prefix
}

// openRecordsExportPathPicker opens the path-edit modal, mirrors
// SOQL export's shape — same edit modal, same expand-tilde,
// same export directory setting from settings.toml.
func (m *Model) openRecordsExportPathPicker(msg openRecordsExportPathMsg) tea.Cmd {
	defaultPath := m.defaultRecordsExportPath(msg.Label, msg.Format)
	format := msg.Format
	exportLabel := msg.Label
	var savedPath string
	state := editModalState{
		Title:       "Save records · " + msg.Format.Label(),
		Hint:        "Edit path / filename, Enter to save  ·  Esc to cancel",
		InitialBody: defaultPath,
		Save: func(val string, _ any) error {
			savedPath = strings.TrimSpace(val)
			if savedPath == "" {
				return fmt.Errorf("path required")
			}
			if _, err := os.Lstat(expandTilde(savedPath)); err == nil {
				return fmt.Errorf("file already exists; choose a new path")
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("check path: %w", err)
			}
			return nil
		},
		OnSuccess: func() tea.Cmd {
			return func() tea.Msg {
				return startRecordsExportMsg{
					Format: format,
					Path:   savedPath,
					Label:  exportLabel,
				}
			}
		},
	}
	return m.openEditModal(state)
}

// startRecordsExportMsg lands once both format + path are confirmed.
type startRecordsExportMsg struct {
	Format exporters.Format
	Path   string
	Label  string
}

// startRecordsExport resolves the source rows + columns and writes
// the file. Synchronous-feeling but goroutine-backed via tea.Cmd —
// the data is already in memory so writes are typically fast.
func (m *Model) startRecordsExport(msg startRecordsExportMsg) tea.Cmd {
	rows, _, ok := m.recordsExportSource()
	if !ok || len(rows) == 0 {
		return func() tea.Msg {
			return recordsExportDoneMsg{Err: fmt.Errorf("nothing to export — source went stale")}
		}
	}
	cols := m.recordsExportColumns()
	if len(cols) == 0 {
		cols = collectColumns(rows, "") // fallback: derive from row keys (records mode has no SELECT to honour)
	}
	format := msg.Format
	label := msg.Label
	savePath := expandTilde(msg.Path)

	return func() tea.Msg {
		headers, dataRows := exsoql.Shape(rows, cols)
		// Generic sheet name — Excel caps sheet names at 31 chars
		// and forbids /\?*[]:; passing a record-shaped label
		// here either gets silently truncated or rejected. The
		// filename already encodes the context (object + chip +
		// timestamp), so a flat "Export" inside the workbook is
		// the right level of detail.
		_ = label
		if err := securefile.Write(savePath, false, func(w io.Writer) error {
			return exporters.Write(w, format, headers, dataRows, "Export")
		}); err != nil {
			return recordsExportDoneMsg{Err: fmt.Errorf("write %s: %w", format, err)}
		}
		return recordsExportDoneMsg{Path: savePath}
	}
}

// recordsExportDoneMsg lands after the export attempt.
type recordsExportDoneMsg struct {
	Path string
	Err  error
}

// applyRecordsExportDone folds the result into a flash banner.
func (m *Model) applyRecordsExportDone(msg recordsExportDoneMsg) {
	if msg.Err != nil {
		m.flashFor("export failed: "+msg.Err.Error(), 8*time.Second)
		applog.Error("records.export.failed", map[string]any{"err": msg.Err.Error()})
		return
	}
	m.flash("saved → " + filepath.Base(msg.Path))
	applog.Info("records.export.saved", map[string]any{"path": msg.Path})
}

// recordsExportSource resolves (rows, label, ok) for the current
// surface. label feeds the filename / modal title. Returns ok=false
// when no records-shaped surface is active.
//
//   - /records · object-detail-records-subtab → ChipRecords for
//     the active sObject + chip
//   - /users · All users → ChipUsers for the active chip,
//     converted from typed UserRow back to map[string]any so the
//     exporters get a uniform shape
func (m Model) recordsExportSource() ([]map[string]any, string, bool) {
	if d, sobj := m.activeRecordsSObject(); sobj != "" {
		if r := currentRecordsResource(d, sobj); r != nil && !r.FetchedAt().IsZero() {
			list := r.Value()
			label := sobj
			chipID := selectedRecordsChip(d, sobj)
			if chipID != "" && chipID != syntheticRecentID {
				label = sobj + "-" + slugify(chipID)
			}
			return list.Records, label, true
		}
	}
	if m.tab() == TabUsers && m.currentSubtab() == SubtabUsersAll {
		d := m.activeOrgData()
		if d == nil {
			return nil, "", false
		}
		chipID := activeUsersChipID(d)
		lv := d.UsersListPtr(chipID)
		items := lv.Items()
		if len(items) == 0 {
			return nil, "", false
		}
		out := make([]map[string]any, 0, len(items))
		for _, u := range items {
			out = append(out, userRowAsMap(u))
		}
		label := "users-" + slugify(chipID)
		return out, label, true
	}
	return nil, "", false
}

// recordsExportColumns picks the column order for the active
// surface. Records lists already carry list.Columns in SELECT
// order; users surface uses a fixed projection so the file matches
// what's on screen.
func (m Model) recordsExportColumns() []string {
	if d, sobj := m.activeRecordsSObject(); sobj != "" {
		if r := currentRecordsResource(d, sobj); r != nil && !r.FetchedAt().IsZero() {
			cols := r.Value().Columns
			if len(cols) > 0 {
				return cols
			}
		}
	}
	if m.tab() == TabUsers && m.currentSubtab() == SubtabUsersAll {
		return []string{"Id", "Name", "Username", "Profile.Name", "UserRole.Name", "LastLoginDate", "IsActive"}
	}
	return nil
}

// userRowAsMap flattens a typed UserRow into the map[string]any
// shape the exporters consume. Mirrors the Profile.Name /
// UserRole.Name keys SF returns natively so the JSON export looks
// like a real Salesforce row.
func userRowAsMap(u sf.UserRow) map[string]any {
	row := map[string]any{
		"Id":            u.ID,
		"Name":          u.Name,
		"Username":      u.Username,
		"LastLoginDate": u.LastLoginDate,
		"IsActive":      u.IsActive,
	}
	row["Profile.Name"] = u.ProfileName
	row["UserRole.Name"] = u.UserRoleName
	return row
}

// defaultRecordsExportPath reuses the same export-dir setting as
// reports + SOQL so users only configure it once.
func (m Model) defaultRecordsExportPath(label string, format exporters.Format) string {
	dir := expandTilde(m.settings.ReportExportDir())
	ts := time.Now().Format("20060102-150405")
	prefix := slugify(label)
	if prefix == "" {
		prefix = "records"
	}
	fname := prefix + "-" + ts + format.Extension()
	return filepath.Join(dir, fname)
}
