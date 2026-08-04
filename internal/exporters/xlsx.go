package exporters

// XLSX writer — uses xuri/excelize/v2 (already a dep via the report
// post-processor pipeline). Header row is bold; data rows are plain
// text so users can sort/filter without inheriting any styling that
// would interfere with their own conditional formatting.
//
// Single sheet per export; sheet name is caller-supplied so it
// reads as the export's subject ("Q2 migration" rather than the
// default "Sheet1").

import (
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

func writeXLSX(w io.Writer, headers []string, rows []ExportRow, sheetName string) error {
	sheetName = sanitizeSheetName(sheetName)
	f := excelize.NewFile()
	defer f.Close()
	// excelize creates "Sheet1" by default. Rename it to our chosen
	// sheet name so the file reads sensibly when opened.
	if err := f.SetSheetName("Sheet1", sheetName); err != nil {
		return fmt.Errorf("rename sheet: %w", err)
	}

	// Header row, bold. Data rows are plain.
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
	})
	if err != nil {
		return fmt.Errorf("header style: %w", err)
	}

	// Write headers (row 1).
	headerRow := make([]any, len(headers))
	for i, h := range headers {
		headerRow[i] = h
	}
	if err := f.SetSheetRow(sheetName, "A1", &headerRow); err != nil {
		return fmt.Errorf("write headers: %w", err)
	}
	if len(headers) > 0 {
		startCell := "A1"
		endCell, err := excelize.CoordinatesToCellName(len(headers), 1)
		if err == nil {
			_ = f.SetCellStyle(sheetName, startCell, endCell, headerStyle)
		}
	}

	// Data rows (row 2 onwards).
	for i, r := range rows {
		dataRow := make([]any, len(headers))
		for j, h := range headers {
			dataRow[j] = NeutralizeFormula(r.Get(h))
		}
		cell := fmt.Sprintf("A%d", i+2)
		if err := f.SetSheetRow(sheetName, cell, &dataRow); err != nil {
			return fmt.Errorf("write row %d: %w", i, err)
		}
	}

	// Auto-fit-ish column widths: cap at 60 cols so a long URL or
	// description doesn't blow up the layout. Excel users can resize
	// as needed.
	for i := range headers {
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			continue
		}
		// Estimate by max content width, capped.
		maxW := len(headers[i])
		for _, r := range rows {
			if l := len(r.Get(headers[i])); l > maxW {
				maxW = l
			}
		}
		if maxW > 60 {
			maxW = 60
		}
		if maxW < 8 {
			maxW = 8
		}
		_ = f.SetColWidth(sheetName, col, col, float64(maxW)+2)
	}

	if err := f.Write(w); err != nil {
		return fmt.Errorf("flush xlsx: %w", err)
	}
	return nil
}

// sanitizeSheetName clamps the caller-supplied sheet name to the
// constraints Excel imposes:
//
//   - 1..31 characters (longer names are silently truncated to 31)
//   - no /, \, ?, *, [, ], : (replaced with - so the result is still
//     legible — Excel rejects the file otherwise)
//   - cannot start or end with an apostrophe (rare, defensive)
//   - empty string falls back to "Export"
//
// Centralised here so every caller is protected; callers should
// still feed their natural label and let this function do the
// flattening.
func sanitizeSheetName(name string) string {
	const max = 31
	const fallback = "Export"
	if name == "" {
		return fallback
	}
	// Replace each forbidden character. Excel rejects the file
	// outright if any of these appear, so substitute rather than
	// strip — preserves visible word boundaries.
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		"?", "-",
		"*", "-",
		"[", "(",
		"]", ")",
		":", "-",
	)
	out := replacer.Replace(name)
	out = strings.TrimSpace(out)
	out = strings.Trim(out, "'")
	if out == "" {
		return fallback
	}
	if len(out) > max {
		// Truncate by rune to avoid splitting a multi-byte glyph.
		runes := []rune(out)
		if len(runes) > max {
			runes = runes[:max]
		}
		out = string(runes)
	}
	return out
}
