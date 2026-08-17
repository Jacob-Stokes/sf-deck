package postprocess

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
	headerIdx := findHeaderRow(rows)
	if headerIdx < 0 {
		return nil
	}
	for i := headerIdx - 1; i >= 0; i-- {
		if err := wb.RemoveRow(sheet, i+1); err != nil {
			return err
		}
	}
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

func columnIsEmpty(rows [][]string, col int) bool {
	for r := 1; r < len(rows); r++ {
		if col < len(rows[r]) && strings.TrimSpace(rows[r][col]) != "" {
			return false
		}
	}
	return true
}

func stripSortArrows(s string) string {
	s = strings.ReplaceAll(s, "↑", "")
	s = strings.ReplaceAll(s, "↓", "")
	return s
}
