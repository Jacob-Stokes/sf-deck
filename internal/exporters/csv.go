package exporters

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
	cw.Flush()
	return cw.Error()
}
