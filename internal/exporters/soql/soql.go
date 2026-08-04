// Package soql shapes SOQL result rows into the shared
// exporters.ExportRow form so the standard CSV / XLSX / JSON writers
// can serialise them without further per-format work.
//
// SOQL projections are dynamic — the column set comes from the query,
// not from a fixed schema. The shape function takes a list of records
// (map[string]any from sf.QueryResult) and the column order observed
// at render time, and walks each record producing one ExportRow per.
//
// Cell formatting matches what the TUI shows: nil → "", numbers via
// %v, strings preserved, nested maps (relationship traversal) flattened
// to dotted keys (e.g. Owner.Name).
package soql

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/exporters"
)

// Shape turns a record set + observed column order into the headers
// + rows pair the standard exporters.Write expects. Columns the
// caller didn't list are surfaced via discovery; the discovered
// columns appear after the listed ones, alphabetically, so the
// caller's preferred order wins for the columns it knows about.
//
// records is the sf.QueryResult.Records slice; columns is the user's
// or renderer's preferred column order.
func Shape(records []map[string]any, columns []string) (headers []string, rows []exporters.ExportRow) {
	flat := flattenRecords(records)
	headers = mergeColumns(columns, discoverColumns(flat))
	rows = make([]exporters.ExportRow, 0, len(flat))
	for _, rec := range flat {
		out := make(map[string]string, len(headers))
		for _, h := range headers {
			out[h] = formatCell(rec[h])
		}
		rows = append(rows, exporters.ExportRow{Columns: out})
	}
	return headers, rows
}

// flattenRecords walks each record and folds nested maps into dotted
// keys. SF subqueries surface as nested maps under
// "attributes.type"-bearing values; this recursion turns those into
// "Account.Owner.Name"-style keys, matching how the TUI renders them.
//
// Skips Salesforce's "attributes" envelope at every level — it's
// metadata, not user data.
func flattenRecords(records []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, r := range records {
		out = append(out, flatten("", r))
	}
	return out
}

func flatten(prefix string, in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		if k == "attributes" {
			continue
		}
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if nested, ok := v.(map[string]any); ok {
			for nk, nv := range flatten(key, nested) {
				out[nk] = nv
			}
			continue
		}
		out[key] = v
	}
	return out
}

// discoverColumns returns the union of keys across records, sorted
// alphabetically. Used to fill in columns the caller didn't pre-list.
func discoverColumns(records []map[string]any) []string {
	seen := map[string]struct{}{}
	for _, r := range records {
		for k := range r {
			seen[k] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// mergeColumns returns the user-supplied order followed by any
// discovered columns the user didn't already list. Preserves order
// of the first list, dedupes against it.
func mergeColumns(preferred, discovered []string) []string {
	seen := make(map[string]struct{}, len(preferred))
	out := make([]string, 0, len(preferred)+len(discovered))
	for _, c := range preferred {
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	for _, c := range discovered {
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

// formatCell stringifies a SOQL cell value for export. nil → empty
// string; everything else routes through fmt.Sprintf("%v"). Lists
// emit "[a, b, c]" — better than "[]any{a b c}" — and other
// composite types fall through to %v so the column doesn't crash on
// unexpected shapes.
func formatCell(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = formatCell(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	return fmt.Sprintf("%v", v)
}
