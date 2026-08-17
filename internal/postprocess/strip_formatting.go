package postprocess

import (
	"github.com/xuri/excelize/v2"
)

type StripFormattingTransform struct{}

func (StripFormattingTransform) ID() string    { return "strip-formatting" }
func (StripFormattingTransform) Label() string { return "Strip cell colours / borders / fonts" }

func (t StripFormattingTransform) Apply(wb *excelize.File, ctx Context) error {
	for _, sheet := range wb.GetSheetList() {
		if err := stripFormattingSheet(wb, sheet); err != nil {
			return err
		}
	}
	return nil
}

func stripFormattingSheet(wb *excelize.File, sheet string) error {
	rows, err := wb.GetRows(sheet)
	if err != nil {
		return err
	}
	for r := 0; r < len(rows); r++ {
		for col := 0; col < len(rows[r]); col++ {
			cellName, err := excelize.CoordinatesToCellName(col+1, r+1)
			if err != nil {
				continue
			}
			_ = wb.SetCellStyle(sheet, cellName, cellName, 0)
		}
	}
	for r := 0; r < len(rows); r++ {
		_ = wb.SetRowHeight(sheet, r+1, -1)
	}
	if len(rows) > 0 {
		maxCol := 0
		for _, r := range rows {
			if len(r) > maxCol {
				maxCol = len(r)
			}
		}
		if maxCol > 0 {
			startCol, err := excelize.ColumnNumberToName(1)
			if err == nil {
				endCol, err := excelize.ColumnNumberToName(maxCol)
				if err == nil {
					_ = wb.SetColWidth(sheet, startCol, endCol, 12.0)
				}
			}
		}
	}
	return nil
}
