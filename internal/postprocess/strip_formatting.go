package postprocess

// "Strip formatting" transform — clears cell fills, fonts, borders,
// and number formats from every populated cell on every sheet, then
// resets row heights and column widths to defaults. The result is
// a workbook that looks like plain data when opened in Excel /
// Numbers / LibreOffice.
//
// SF's "Formatted Report" xlsx ships with branded cell colouring
// (green column headers, alternating row tints, bold group-leader
// rows). When the user wants Details-Only output, that styling is
// noise. excelize doesn't expose "delete style" directly, so we set
// every populated cell's style id to 0 (the workbook's default style
// — black text, no fill, no border).

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
	// Style 0 is the workbook's default style — guaranteed to exist
	// in every xlsx (Excel writes it automatically). Setting every
	// cell to style 0 is equivalent to "clear formatting".
	for r := 0; r < len(rows); r++ {
		for col := 0; col < len(rows[r]); col++ {
			cellName, err := excelize.CoordinatesToCellName(col+1, r+1)
			if err != nil {
				continue
			}
			_ = wb.SetCellStyle(sheet, cellName, cellName, 0)
		}
	}
	// Reset row heights to default (-1 = use default in excelize).
	for r := 0; r < len(rows); r++ {
		_ = wb.SetRowHeight(sheet, r+1, -1)
	}
	// Reset column widths. Defaults to 8.43 (Excel default); -1
	// would also work but excelize treats it as 0 on read-back. Use
	// the literal default so the file round-trips cleanly.
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
