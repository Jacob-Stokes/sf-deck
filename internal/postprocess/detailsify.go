package postprocess

// "Details-ify" transform — converts SF's "Formatted Report" xlsx into
// a Details-Only-shaped layout client-side.
//
// SF's REST Analytics endpoint only emits xlsx in Formatted layout
// (the documented endpoint is literally titled "Download Formatted
// Excel"). The Details-Only-xlsx variant is gated by the UI's classic
// export servlet which is session-cookie-only.
//
// Luckily, the formatted xlsx is structurally close to details-only:
//   - rows 0..N: title + filter-list preamble (no cell in column A;
//     report title in column B)
//   - row N+1: the column header row, marked by the sort-arrow glyph
//     "↑" / "↓" appearing on at least one header cell, AND a
//     significantly higher non-empty cell count than the preamble
//     rows
//   - rows N+2..end: data rows, with column A always blank (an
//     indent gutter) and "group leader" values left blank on
//     subsequent rows that share the same group
//
// To detail-ify:
//   1. find the header row by scanning for max-non-empty-cells in the
//      first ~30 rows
//   2. delete every row above it
//   3. delete column A (the indent gutter)
//   4. forward-fill blank cells in every column (group-leader cells
//      cascade downwards visually in SF; we make that explicit)
//   5. trim aggregate / total rows at the bottom (rows whose first
//      cell is "Grand Total", "Subtotal", etc. — only when the user
//      asked for a clean detail-only view)

import (
	"strings"

	"github.com/xuri/excelize/v2"
)

// DetailsifyTransform flattens SF's formatted xlsx into a details-only
// shape. Idempotent: running it on an already-detailsified workbook
// is a no-op (no header sigil left to find).
type DetailsifyTransform struct{}

func (DetailsifyTransform) ID() string    { return "detailsify" }
func (DetailsifyTransform) Label() string { return "Strip SF preamble + groupings (Details Only)" }

func (t DetailsifyTransform) Apply(wb *excelize.File, ctx Context) error {
	for _, sheet := range wb.GetSheetList() {
		if err := detailsifySheet(wb, sheet); err != nil {
			return err
		}
	}
	return nil
}

func detailsifySheet(wb *excelize.File, sheet string) error {
	// Note: SF's grouped reports contain merged cells (e.g. group-leader
	// names spanning B:C). We don't unmerge them — excelize's GetRows
	// already returns the merge value at its top-left coordinate and
	// empty for the rest of the range. Forward-fill below populates
	// the empties correctly. Calling UnmergeCell in a loop is O(n²)
	// and on a 7700-row report takes >2 minutes — not worth it.
	rows, err := wb.GetRows(sheet)
	if err != nil {
		return err
	}
	if len(rows) < 3 {
		return nil // nothing useful to do on a tiny sheet
	}
	// Step 1: find the header row by looking for the sort-arrow
	// glyph or the row with the most non-empty cells in the first 30.
	headerIdx := findHeaderRow(rows)
	if headerIdx < 0 {
		// Couldn't identify a header — bail rather than mangle.
		return nil
	}
	// Step 2: delete rows above the header (working bottom-up so the
	// indices we still need don't shift). excelize is 1-indexed.
	for i := headerIdx - 1; i >= 0; i-- {
		if err := wb.RemoveRow(sheet, i+1); err != nil {
			return err
		}
	}
	// Step 3: delete column A (the indent gutter) if it's actually
	// empty across the data block. Some report types put the leftmost
	// grouping in column A — we only strip when it's pure whitespace.
	rows, err = wb.GetRows(sheet)
	if err != nil {
		return err
	}
	if columnIsEmpty(rows, 0) {
		if err := wb.RemoveCol(sheet, "A"); err != nil {
			return err
		}
		rows, err = wb.GetRows(sheet)
		if err != nil {
			return err
		}
	}
	// Step 4a: scrub sort-arrow glyphs ("↑", "↓") from header cells —
	// they're SF UI cosmetics, not data.
	if len(rows) > 0 {
		for col, h := range rows[0] {
			cleaned := strings.TrimSpace(stripSortArrows(h))
			if cleaned != h {
				cellName, err := excelize.CoordinatesToCellName(col+1, 1)
				if err == nil {
					_ = wb.SetCellStr(sheet, cellName, cleaned)
				}
			}
		}
	}
	// Step 5: drop summary rows BEFORE forward-fill — otherwise a
	// row like ["Subtotal", "Count", "3"] would seed forward-fill
	// with "Count" and propagate it down every row below. Walk
	// bottom-up so indices don't shift on delete.
	rows, err = wb.GetRows(sheet)
	if err != nil {
		return err
	}
	for i := len(rows) - 1; i >= 1; i-- {
		if isAnySummaryRow(rows[i]) {
			if err := wb.RemoveRow(sheet, i+1); err != nil {
				return err
			}
		}
	}
	// Step 6: forward-fill group-leader values so they cascade onto every
	// row that shares the group. CRITICAL: fill ONLY grouping columns, not
	// every column. A grouping column is blank on continuation rows by
	// design (the leader value shows once per group); a regular data
	// column is blank only when the field is genuinely null. Filling the
	// latter fabricates data — a null Amount would become the row-above's
	// Amount, silently, in a file Salesforce never validates. We restrict
	// the fill to the leftmost contiguous block of grouping columns,
	// identified structurally below.
	rows, err = wb.GetRows(sheet)
	if err != nil {
		return err
	}
	if len(rows) < 2 {
		return nil
	}
	headerLen := len(rows[0])
	groupCols := groupingColumns(rows, headerLen)
	for col := 0; col < headerLen; col++ {
		if !groupCols[col] {
			continue
		}
		var last string
		for r := 1; r < len(rows); r++ {
			var cell string
			if col < len(rows[r]) {
				cell = rows[r][col]
			}
			if strings.TrimSpace(cell) == "" && last != "" {
				cellName, err := excelize.CoordinatesToCellName(col+1, r+1)
				if err != nil {
					continue
				}
				_ = wb.SetCellStr(sheet, cellName, last)
			} else if cell != "" {
				last = cell
			}
		}
	}
	return nil
}

// groupingColumns identifies which columns are grouping columns safe to
// forward-fill. SF places grouping columns leftmost, and the structural
// tell is: a grouping column has at least one blank cell (the leader
// value is suppressed on continuation rows), whereas a data column is
// fully populated unless a field is genuinely null. We return the
// LEFTMOST CONTIGUOUS run of columns that contain a blank, and stop at
// the first fully-populated column — so the fill stays in the leftmost
// grouping block and can never reach a data column where a blank means a
// real null (the case that would fabricate data). Conservative: a data
// column to the right of a grouping column is protected even if it has
// nulls, because the run stops at the first dense column between them.
func groupingColumns(rows [][]string, headerLen int) map[int]bool {
	out := map[int]bool{}
	if len(rows) < 2 {
		return out
	}
	for col := 0; col < headerLen; col++ {
		hasBlank := false
		for r := 1; r < len(rows); r++ {
			if col >= len(rows[r]) || strings.TrimSpace(rows[r][col]) == "" {
				hasBlank = true
				break
			}
		}
		if !hasBlank {
			// Fully-populated column = data column. Stop: everything to the
			// right is beyond the leftmost grouping block.
			break
		}
		out[col] = true
	}
	return out
}

// findHeaderRow scans the first 30 rows and returns the index of the
// row that most plausibly is the column-header row. Heuristic: the
// row with a "↑" or "↓" sort arrow wins; otherwise the first row
// where non-empty-cell count is materially higher than the rows
// above it (SF preamble rows have 1-2 non-empty cells; header rows
// have N).
func findHeaderRow(rows [][]string) int {
	limit := 30
	if limit > len(rows) {
		limit = len(rows)
	}
	for i := 0; i < limit; i++ {
		for _, c := range rows[i] {
			if strings.ContainsAny(c, "↑↓") {
				return i
			}
		}
	}
	// Fallback: row with max non-empty count in the first 30 — but
	// require at least 3 non-empty cells to avoid latching onto a
	// stray title row. Tie-break: earliest such row wins (the header
	// is always above the data).
	bestIdx, bestCount := -1, 2
	for i := 0; i < limit; i++ {
		n := 0
		for _, c := range rows[i] {
			if strings.TrimSpace(c) != "" {
				n++
			}
		}
		if n > bestCount {
			bestIdx, bestCount = i, n
		}
	}
	return bestIdx
}

// columnIsEmpty returns true if every row's `col`th cell is whitespace.
// Only checks data rows (skips the header at index 0).
func columnIsEmpty(rows [][]string, col int) bool {
	for r := 1; r < len(rows); r++ {
		if col < len(rows[r]) && strings.TrimSpace(rows[r][col]) != "" {
			return false
		}
	}
	return true
}

// stripSortArrows removes the unicode sort-arrow glyphs SF appends
// to grouped/sorted header cells.
func stripSortArrows(s string) string {
	s = strings.ReplaceAll(s, "↑", "")
	s = strings.ReplaceAll(s, "↓", "")
	return s
}
