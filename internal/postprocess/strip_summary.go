package postprocess

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
	for i := len(rows) - 1; i >= 1; i-- { // skip header at 0
		if isAnySummaryRow(rows[i]) {
			if err := wb.RemoveRow(sheet, i+1); err != nil {
				return err
			}
		}
	}
	return nil
}

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
