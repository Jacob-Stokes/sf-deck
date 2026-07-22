// Package exporters is the shared "shape an internal data structure
// into a portable file" toolbox.
//
// The package decomposes into:
//
//   - exporters.go (this file)  — shared types: ExportRow, Format,
//     Write dispatch
//   - csv.go / xlsx.go / json.go — per-format writers; each takes a
//     []ExportRow + io.Writer and emits
//     the appropriate bytes
//   - <subject>/                 — per-subject row builders that turn
//     domain objects into []ExportRow.
//     Today: devproject/. Future: recent/,
//     soql/, snapshot/, … any data the
//     TUI accumulates that a user might
//     want as a file.
//
// Why a generic ExportRow rather than each subject having its own
// writers: every subject's export ends up looking like a tabular
// reference list ("here's what's in this collection, with stable
// identifiers + URLs"). Centralising the row shape means a new export
// subject is one row-builder file + zero writer changes; a new format
// (markdown? html?) is one writer file + zero subject changes.
//
// Counter-arguments that lost: per-subject ExportRow types could
// carry richer typed columns (e.g. "ItemKind" as a typed enum rather
// than a string). True, but the gain is small and the cost is N×M
// implementations. The shared row uses string columns; subjects pre-
// format their values when shaping rows. The output is destined for
// spreadsheets either way — strings all the way down is fine.
package exporters

import (
	"fmt"
	"io"
	"strings"
)

// ExportRow is one tabular row in any export. Columns is keyed by
// the column header; the writer determines the column order from
// the caller-supplied Headers slice (so column order is stable
// across writers and a missing column reads as empty rather than
// crashing).
//
// Columns map values are strings — domain types pre-format (dates as
// ISO-8601, booleans as "true"/"false", ints as decimal). Spreadsheet
// consumers don't benefit from typed values and the JSON exporter
// emits the strings as-is for stable round-tripping.
type ExportRow struct {
	Columns map[string]string
}

// Get returns the column value or "" when the column isn't set.
// Defensive against subjects that don't populate every header for
// every row (e.g. a project with no records won't fill the
// "Records-only" columns).
func (r ExportRow) Get(col string) string {
	if r.Columns == nil {
		return ""
	}
	return r.Columns[col]
}

// NeutralizeFormula defuses CSV/spreadsheet formula injection: a cell
// whose first character is one a spreadsheet treats as a formula
// (= + - @, or a leading tab/CR that some parsers strip first) is
// prefixed with a single quote so the app renders it as literal text
// rather than evaluating it. Salesforce record/org data is written
// verbatim into exports, so a malicious field value (e.g. a Name of
// `=HYPERLINK(...)`) would otherwise become a live formula when the
// file is opened. The leading-quote convention is the standard,
// app-agnostic mitigation.
func NeutralizeFormula(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// Format is the chosen output format. Values match the file
// extension (lowercased) so callers can derive a default filename
// directly from the format.
type Format string

const (
	FormatCSV  Format = "csv"
	FormatXLSX Format = "xlsx"
	FormatJSON Format = "json"
	// FormatPackageXML emits a minimal manifest bundle: just
	// package.xml + optional records.csv + README.md. Right answer
	// for users who already have an sfdx project (drop package.xml
	// into the existing manifest/ dir) or for non-sfdx tools (Gearset,
	// Workbench, ANT).
	FormatPackageXML Format = "package-xml"
	// FormatSfdxProject emits a complete, self-contained sfdx project:
	// package.xml + sfdx-project.json + force-app/ skeleton + README.
	// Users can `cd` into the bundle and run `sf project retrieve`
	// immediately. Right answer for users starting from scratch.
	FormatSfdxProject Format = "sfdx-project"
	// FormatSfdxProjectRetrieve is FormatSfdxProject + an automatic
	// `sf project retrieve` after the bundle is written. Yields a
	// project tree with the actual XML/source files populated, ready
	// to commit / open in VS Code. Saves the user the cd + retrieve
	// step at the cost of needing org auth + one extra round-trip.
	FormatSfdxProjectRetrieve Format = "sfdx-project-retrieve"
)

// AllFormats returns every supported format in display order. Used
// by the "format picker" modal so adding a new format auto-shows up
// in the UI.
func AllFormats() []Format {
	return []Format{FormatCSV, FormatXLSX, FormatJSON, FormatPackageXML, FormatSfdxProject, FormatSfdxProjectRetrieve}
}

// IsBundle reports whether the format produces a folder of files
// (true) rather than a single file (false). Bundle formats route
// through their subject-specific writer instead of Write() since
// the standard ExportRow shape doesn't apply.
func (f Format) IsBundle() bool {
	return f == FormatPackageXML || f == FormatSfdxProject || f == FormatSfdxProjectRetrieve
}

// RunsRetrieve reports whether the format kicks off a follow-up
// `sf project retrieve` after the bundle is written. Only
// FormatSfdxProjectRetrieve does today; kept as a method so callers
// don't grow a switch on every place that asks "should I retrieve".
func (f Format) RunsRetrieve() bool {
	return f == FormatSfdxProjectRetrieve
}

// Label is the human-readable name shown in pickers.
func (f Format) Label() string {
	switch f {
	case FormatCSV:
		return "CSV"
	case FormatXLSX:
		return "Excel (xlsx)"
	case FormatJSON:
		return "JSON"
	case FormatPackageXML:
		return "Bundle: manifest only (package.xml)"
	case FormatSfdxProject:
		return "Bundle: sfdx skeleton (package.xml + force-app/)"
	case FormatSfdxProjectRetrieve:
		return "Bundle: sfdx skeleton + retrieve from org"
	}
	return strings.ToUpper(string(f))
}

// Extension is the dotted file extension for the format (".csv",
// ".xlsx", ".json"). Bundle formats return "" — they emit a
// directory, not a single file.
func (f Format) Extension() string {
	switch f {
	case FormatCSV:
		return ".csv"
	case FormatXLSX:
		return ".xlsx"
	case FormatJSON:
		return ".json"
	case FormatPackageXML, FormatSfdxProject, FormatSfdxProjectRetrieve:
		return ""
	}
	return "." + strings.ToLower(string(f))
}

// FormatFromExtension reverses Extension. Returns ("", false) for
// unknown extensions; callers fall through to a default or refuse
// the export.
func FormatFromExtension(ext string) (Format, bool) {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	switch ext {
	case "csv":
		return FormatCSV, true
	case "xlsx":
		return FormatXLSX, true
	case "json":
		return FormatJSON, true
	}
	return "", false
}

// Write dispatches to the format-specific writer. Headers fixes the
// column order so writers across formats produce consistent layouts;
// SheetName is used by xlsx (ignored by csv/json — kept on the
// signature so the dispatch surface stays uniform).
//
// The CSV writer emits a header row matching Headers, then one data
// row per ExportRow with empty cells for missing columns. The XLSX
// writer mirrors that with a styled header row. The JSON writer
// emits an array of objects; each object's keys come from Headers in
// declared order.
func Write(w io.Writer, format Format, headers []string, rows []ExportRow, sheetName string) error {
	switch format {
	case FormatCSV:
		return writeCSV(w, headers, rows)
	case FormatXLSX:
		return writeXLSX(w, headers, rows, sheetName)
	case FormatJSON:
		return writeJSON(w, headers, rows)
	}
	return fmt.Errorf("exporters: unknown format %q", format)
}
