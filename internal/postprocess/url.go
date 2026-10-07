package postprocess

// URL post-processor — adds hyperlink columns next to every Salesforce-Id
// column in the workbook.

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

type urlTransform struct{}

func (urlTransform) ID() string    { return "url" }
func (urlTransform) Label() string { return "Add hyperlink columns for Salesforce IDs" }

func (t urlTransform) Apply(wb *excelize.File, ctx Context) error {
	if ctx.InstanceURL == "" || len(ctx.PrefixToSObject) == 0 {
		return nil
	}
	instance := strings.TrimRight(ctx.InstanceURL, "/")
	for _, sheet := range wb.GetSheetList() {
		if err := annotateSheet(wb, sheet, instance, ctx.PrefixToSObject); err != nil {
			return err
		}
	}
	return nil
}

func annotateSheet(wb *excelize.File, sheet, instance string, prefixMap map[string]string) error {
	rows, err := wb.GetRows(sheet)
	if err != nil {
		return err
	}
	if len(rows) < 2 {
		return nil // no data — nothing to annotate
	}
	header := rows[0]
	candidates := []idColumn{}
	for i, h := range header {
		hn := strings.TrimSpace(h)
		if !strings.HasSuffix(hn, "Id") && !strings.HasSuffix(hn, "ID") {
			continue
		}
		var sample string
		for r := 1; r < len(rows); r++ {
			if i < len(rows[r]) {
				v := strings.TrimSpace(rows[r][i])
				if v != "" {
					sample = v
					break
				}
			}
		}
		if !looksLikeSFID(sample) {
			continue
		}
		prefix := sample[:3]
		if _, ok := prefixMap[prefix]; !ok {
			continue
		}
		candidates = append(candidates, idColumn{
			Index:  i,
			Header: hn,
		})
	}

	for k := len(candidates) - 1; k >= 0; k-- {
		c := candidates[k]
		if err := insertLinkColumn(wb, sheet, c, rows, instance, prefixMap); err != nil {
			return err
		}
	}
	return nil
}

type idColumn struct {
	Index  int
	Header string
}

func insertLinkColumn(wb *excelize.File, sheet string, c idColumn, rows [][]string, instance string, prefixMap map[string]string) error {
	insertAt := c.Index + 2
	insertColName, err := excelize.ColumnNumberToName(insertAt)
	if err != nil {
		return err
	}
	if err := wb.InsertCols(sheet, insertColName, 1); err != nil {
		return err
	}
	headerCell, err := excelize.CoordinatesToCellName(insertAt, 1)
	if err != nil {
		return err
	}
	if err := wb.SetCellStr(sheet, headerCell, c.Header+"_link"); err != nil {
		return err
	}
	for r := 1; r < len(rows); r++ {
		var raw string
		if c.Index < len(rows[r]) {
			raw = strings.TrimSpace(rows[r][c.Index])
		}
		if !looksLikeSFID(raw) {
			continue
		}
		if _, ok := prefixMap[raw[:3]]; !ok {
			continue
		}
		formula := fmt.Sprintf(`HYPERLINK("%s/%s","%s")`, instance, raw, raw)
		cell, err := excelize.CoordinatesToCellName(insertAt, r+1)
		if err != nil {
			return err
		}
		if err := wb.SetCellFormula(sheet, cell, formula); err != nil {
			return err
		}
	}
	return nil
}

func looksLikeSFID(s string) bool {
	if len(s) != 15 && len(s) != 18 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		default:
			return false
		}
	}
	return true
}
