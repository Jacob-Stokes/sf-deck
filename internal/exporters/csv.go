package exporters

// CSV writer. Standard library encoding/csv handles quoting + escaping
// for us; we just have to ensure column order is stable and missing
// columns render as empty cells.

import (
	"encoding/csv"
	"io"
)

func writeCSV(w io.Writer, headers []string, rows []ExportRow) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(headers); err != nil {
		return err
	}
	for _, r := range rows {
		record := make([]string, len(headers))
		for i, h := range headers {
			record[i] = NeutralizeFormula(r.Get(h))
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	// Flush BEFORE checking Error — a deferred Flush would run after the
	// return and its error would be lost, so a write that only fails at
	// flush would be reported as success.
	cw.Flush()
	return cw.Error()
}
