package postprocess

// "Strip summary" transform — removes Subtotal / Total / Grand Total
// rows that SF leaves embedded in grouped reports. detailsify trims
// the trailing Grand Total only; this catches subtotals that appear
// between groups in summary / matrix reports.
//
// Heuristic: a row is a summary row when its first non-empty cell
// matches one of the SF summary labels (Total, Subtotal, Grand Total,
// Sum of …, Avg of …, Count …, Min …, Max …). Case-insensitive,
// prefix-match.

import (
	"strings"

	"github.com/xuri/excelize/v2"
)

type StripSummaryTransform struct{}

func (StripSummaryTransform) ID() string    { return "strip-summary" }
func (StripSummaryTransform) Label() string { return "Drop subtotal / total rows" }

func (t StripSummaryTransform) Apply(wb *excelize.File, ctx Context) error {
	for _, sheet := range wb.GetSheetList() {
		if err := stripSummarySheet(wb, sheet); err != nil {
			return err
		}
	}
	return nil
}

func stripSummarySheet(wb *excelize.File, sheet string) error {
	rows, err := wb.GetRows(sheet)
	if err != nil {
		return err
	}
	// Walk bottom-up so the indices we still need don't shift on delete.
	for i := len(rows) - 1; i >= 1; i-- { // skip header at 0
		if isAnySummaryRow(rows[i]) {
			if err := wb.RemoveRow(sheet, i+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// isAnySummaryRow returns true when the first non-empty cell looks
// like a summary label. Broader than the stricter check inside
// detailsify (which only handles the trailing Grand Total).
func isAnySummaryRow(row []string) bool {
	for _, c := range row {
		t := strings.TrimSpace(c)
		if t == "" {
			continue
		}
		lower := strings.ToLower(t)
		switch {
		case strings.HasPrefix(lower, "grand total"),
			strings.HasPrefix(lower, "subtotal"),
			strings.HasPrefix(lower, "total "),
			strings.HasPrefix(lower, "total\t"),
			lower == "total",
			strings.HasPrefix(lower, "sum of "),
			strings.HasPrefix(lower, "avg of "),
			strings.HasPrefix(lower, "average of "),
			strings.HasPrefix(lower, "count of "),
			strings.HasPrefix(lower, "min of "),
			strings.HasPrefix(lower, "max of "),
			strings.HasPrefix(lower, "median of "),
			strings.HasPrefix(lower, "unique count of "):
			return true
		}
		return false
	}
	return false
}
