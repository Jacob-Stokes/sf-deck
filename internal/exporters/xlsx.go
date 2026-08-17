package exporters

// XLSX writer — uses xuri/excelize/v2 (already a dep via the report
// post-processor pipeline). Header row is bold; data rows are plain
// text so users can sort/filter without inheriting any styling that
// would interfere with their own conditional formatting.

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
	if err := f.SetSheetName("Sheet1", sheetName); err != nil {
		return fmt.Errorf("rename sheet: %w", err)
	}

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
	})
	if err != nil {
		return fmt.Errorf("header style: %w", err)
	}

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

	for i := range headers {
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			continue
		}
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
