package postprocess

// URL post-processor — adds hyperlink columns next to every Salesforce-Id
// column in the workbook.
//
// Detection is conservative: a column qualifies when (1) the header name
// ends in "Id" / "ID", and (2) its first non-empty data row matches the
// Salesforce Id shape (15 or 18 alphanumeric chars) and the 3-char prefix
// maps to a known sObject. This excludes external Ids (custom text fields
// that happen to live in an "*Id" column) cleanly because their prefixes
// won't match any KeyPrefix.
//
// Polymorphic columns (Task.WhatId → Account/Opp/Lead) are handled by
// looking up each row's prefix individually; rows whose prefix doesn't
// match get a blank link cell.

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

type urlTransform struct{}

func (urlTransform) ID() string    { return "url" }
func (urlTransform) Label() string { return "Add hyperlink columns for Salesforce IDs" }

func (t urlTransform) Apply(wb *excelize.File, ctx Context) error {
	// Bail when the runner didn't supply the bits we need — be a no-op
	// rather than an error so a partially-configured pipeline still
	// produces a usable file.
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

// annotateSheet walks one sheet, finds qualifying Id columns, and
// inserts a sibling hyperlink column for each. Iterates from the right
// so column-index math doesn't shift under us as we insert.
func annotateSheet(wb *excelize.File, sheet, instance string, prefixMap map[string]string) error {
	rows, err := wb.GetRows(sheet)
	if err != nil {
		return err
	}
	if len(rows) < 2 {
		return nil // no data — nothing to annotate
	}
	header := rows[0]
	// Identify candidate columns first (left → right) so when we insert
	// from the right we can still use the original indices.
	candidates := []idColumn{}
	for i, h := range header {
		hn := strings.TrimSpace(h)
		if !strings.HasSuffix(hn, "Id") && !strings.HasSuffix(hn, "ID") {
			continue
		}
		// Find the first non-empty value below this header.
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

	// Insert from rightmost candidate to leftmost so each insertion
	// doesn't shift the indices we're about to act on.
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

// insertLinkColumn adds a "<header>_link" column immediately after the
// Id column at idx. Each row gets a HYPERLINK(...) formula pointing at
// /<instanceURL>/<recordId>; rows whose prefix doesn't match a known
// sObject get a blank cell (handles polymorphic columns gracefully).
func insertLinkColumn(wb *excelize.File, sheet string, c idColumn, rows [][]string, instance string, prefixMap map[string]string) error {
	// excelize column numbers are 1-indexed; c.Index is 0-indexed, so
	// +2 places the new column right after it.
	insertAt := c.Index + 2
	insertColName, err := excelize.ColumnNumberToName(insertAt)
	if err != nil {
		return err
	}
	if err := wb.InsertCols(sheet, insertColName, 1); err != nil {
		return err
	}
	// Header.
	headerCell, err := excelize.CoordinatesToCellName(insertAt, 1)
	if err != nil {
		return err
	}
	if err := wb.SetCellStr(sheet, headerCell, c.Header+"_link"); err != nil {
		return err
	}
	// Rows.
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

// looksLikeSFID returns true for a 15- or 18-char alphanumeric string —
// the Salesforce Id shape. Doesn't validate the case-folding checksum
// for 18-char IDs; pairing this with prefixMap membership is enough to
// keep external IDs out.
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
