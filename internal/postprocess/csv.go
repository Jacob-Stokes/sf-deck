package postprocess

// xlsx → csv conversion. Used when the user picked "Details Only · csv"
// and we've already detail-ified the SF xlsx — we just need to dump
// the first sheet as csv.

import (
	"bytes"
	"encoding/csv"
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/exporters"
)

// ToCSV reads the first sheet of an in-memory xlsx and serialises it
// as csv bytes. Empty trailing cells per row are kept so the column
// count stays uniform across rows.
func ToCSV(in []byte) ([]byte, error) {
	wb, err := excelize.OpenReader(bytesReader(in))
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer wb.Close()
	sheets := wb.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("xlsx has no sheets")
	}
	rows, err := wb.GetRows(sheets[0])
	if err != nil {
		return nil, err
	}
	// Pad every row to the widest row's length so a csv.Writer doesn't
	// emit ragged rows.
	width := 0
	for _, r := range rows {
		if len(r) > width {
			width = len(r)
		}
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	for _, r := range rows {
		row := make([]string, width)
		for i := range row {
			if i < len(r) {
				row[i] = exporters.NeutralizeFormula(r[i])
			}
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
