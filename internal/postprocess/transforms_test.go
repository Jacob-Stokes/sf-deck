package postprocess

// Behaviour tests for the three real transforms — detailsify,
// strip-summary, and URL injection. These reshape user export files;
// the orchestration (Run/ToCSV) was tested but the transforms
// themselves were the June review's flagged gap: detailsify is
// self-described as heuristic, which is exactly where a weird report
// shape produces silent mangling.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// rowsFromXLSX re-reads transformed bytes for assertions.
func rowsFromXLSX(t *testing.T, b []byte) [][]string {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("reopen xlsx: %v", err)
	}
	defer f.Close()
	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	return rows
}

// sfFormattedReport mimics the shape SF's "Formatted Report" xlsx
// actually has: title + filter preamble (sparse rows, col A empty),
// a header row carrying a sort arrow, an indent-gutter column A,
// group-leader cells blank on continuation rows, subtotals, and a
// grand total.
func sfFormattedReport(t *testing.T) []byte {
	return makeXLSX(t, [][]string{
		{"", "My Accounts Report"},
		{"", "Filtered By: Owner equals Me"},
		{""},
		{"", "Owner ↑", "Account Name", "Amount"},
		{"", "Alice", "Acme", "100"},
		{"", "", "Globex", "200"},
		{"", "Subtotal", "", "300"},
		{"", "Bob", "Initech", "50"},
		{"", "Grand Total", "", "350"},
	})
}

func TestDetailsify_FullFormattedReport(t *testing.T) {
	out, err := Run(sfFormattedReport(t), []Transform{&DetailsifyTransform{}}, Context{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	rows := rowsFromXLSX(t, out)

	// Preamble gone: row 0 is the header, arrows stripped, gutter
	// column A removed.
	if len(rows) == 0 || rows[0][0] != "Owner" {
		t.Fatalf("header row = %v", rows)
	}
	for _, r := range rows {
		first := ""
		if len(r) > 0 {
			first = r[0]
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(first)), "subtotal") ||
			strings.HasPrefix(strings.ToLower(strings.TrimSpace(first)), "grand total") {
			t.Fatalf("summary row survived: %v", r)
		}
		if strings.ContainsAny(strings.Join(r, ""), "↑↓") {
			t.Fatalf("sort arrow survived: %v", r)
		}
	}
	// Forward-fill: Globex's continuation row inherits Alice.
	var globexOwner string
	for _, r := range rows {
		if len(r) > 1 && r[1] == "Globex" {
			globexOwner = r[0]
		}
	}
	if globexOwner != "Alice" {
		t.Fatalf("group-leader forward-fill failed: Globex owner = %q", globexOwner)
	}
	// Data rows intact: 3 data rows + header.
	if len(rows) != 4 {
		t.Fatalf("expected header + 3 data rows, got %d: %v", len(rows), rows)
	}
}

// TestDetailsify_DoesNotFabricateNullDataCells is the audit-7 must-fix:
// forward-fill must cascade group-leader (grouping) columns but must NOT
// copy a value into a genuinely-null DATA cell, which would silently
// report a value the record doesn't have. Here Owner is the grouping
// column (blank continuation row → should fill); Amount is a data column
// with a real null on row 2 → must stay blank.
func TestDetailsify_DoesNotFabricateNullDataCells(t *testing.T) {
	in := makeXLSX(t, [][]string{
		{"", "My Report"},
		{""},
		{"", "Owner ↑", "Account Name", "Amount"},
		{"", "Alice", "Acme", "100"},
		{"", "", "Globex", ""}, // same group as Alice; Globex has a NULL Amount
		{"", "Bob", "Initech", "50"},
	})
	out, err := Run(in, []Transform{&DetailsifyTransform{}}, Context{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	rows := rowsFromXLSX(t, out)

	var globex []string
	for _, r := range rows {
		if len(r) > 1 && r[1] == "Globex" {
			globex = r
		}
	}
	if globex == nil {
		t.Fatalf("Globex row missing: %v", rows)
	}
	// Owner (grouping col) SHOULD have cascaded.
	if globex[0] != "Alice" {
		t.Errorf("grouping column not filled: Globex owner = %q, want Alice", globex[0])
	}
	// Amount (data col) was null and MUST NOT be fabricated from row above (100).
	if len(globex) > 2 && strings.TrimSpace(globex[2]) != "" {
		t.Errorf("null data cell fabricated: Globex amount = %q, want empty", globex[2])
	}
}

func TestDetailsify_Idempotent(t *testing.T) {
	once, err := Run(sfFormattedReport(t), []Transform{&DetailsifyTransform{}}, Context{})
	if err != nil {
		t.Fatal(err)
	}
	twice, err := Run(once, []Transform{&DetailsifyTransform{}}, Context{})
	if err != nil {
		t.Fatal(err)
	}
	a, b := rowsFromXLSX(t, once), rowsFromXLSX(t, twice)
	if len(a) != len(b) {
		t.Fatalf("second run changed row count: %d -> %d", len(a), len(b))
	}
	for i := range a {
		if strings.Join(a[i], "|") != strings.Join(b[i], "|") {
			t.Fatalf("row %d changed on second run: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestDetailsify_BailsWithoutIdentifiableHeader(t *testing.T) {
	// Every row sparse (≤2 non-empty cells), no sort arrows — the
	// header heuristic must bail and leave the sheet alone rather
	// than mangle it.
	in := makeXLSX(t, [][]string{
		{"", "just"},
		{"", "two"},
		{"", "cells"},
		{"", "per row"},
	})
	out, err := Run(in, []Transform{&DetailsifyTransform{}}, Context{})
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsFromXLSX(t, out)
	if len(rows) != 4 || rows[0][1] != "just" {
		t.Fatalf("unidentifiable sheet was modified: %v", rows)
	}
}

func TestDetailsify_PreservesNonEmptyColumnA(t *testing.T) {
	// Some report types put the leftmost grouping IN column A — the
	// gutter strip must only fire when A is pure whitespace.
	in := makeXLSX(t, [][]string{
		{"Region ↓", "Account", "Amount"},
		{"EMEA", "Acme", "1"},
		{"APAC", "Globex", "2"},
	})
	out, err := Run(in, []Transform{&DetailsifyTransform{}}, Context{})
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsFromXLSX(t, out)
	if rows[0][0] != "Region" {
		t.Fatalf("column A was stripped despite holding data: %v", rows)
	}
}

func TestFindHeaderRow(t *testing.T) {
	cases := []struct {
		name string
		rows [][]string
		want int
	}{
		{"arrow wins", [][]string{{"", "title"}, {"a", "b ↑", "c"}, {"1", "2", "3"}}, 1},
		{"max-count fallback needs 3+", [][]string{{"", "t"}, {"a", "b", "c", "d"}, {"1", "2", "3", "4"}}, 1},
		{"nothing qualifies", [][]string{{"", "x"}, {"", "y"}}, -1},
	}
	for _, c := range cases {
		if got := findHeaderRow(c.rows); got != c.want {
			t.Errorf("%s: findHeaderRow = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestIsAnySummaryRow(t *testing.T) {
	yes := [][]string{
		{"Grand Total", "", "350"},
		{"Subtotal", "100"},
		{"", "", "Total Amount: 5"}, // leading empties skipped
		{"Sum of Amount"},
		{"Average of Score"},
		{"Unique Count of Id"},
		{"total"},
	}
	no := [][]string{
		{"Totally Normal Account", "1"}, // prefix "total " requires the space
		{"Subtotals R Us"},              // ...but "subtotal" prefix DOES match — see below
		{"Acme", "Total", "1"},          // summary label must be the FIRST non-empty cell
		{},
	}
	for _, r := range yes {
		if !isAnySummaryRow(r) {
			t.Errorf("expected summary: %v", r)
		}
	}
	// "Subtotals R Us" is a known false POSITIVE of the prefix
	// heuristic — pin the actual behaviour so a future fix is a
	// conscious change, not an accident.
	if !isAnySummaryRow(no[1]) {
		t.Errorf("heuristic changed: %v no longer matches subtotal prefix", no[1])
	}
	for _, r := range [][]string{no[0], no[2], no[3]} {
		if isAnySummaryRow(r) {
			t.Errorf("expected NOT summary: %v", r)
		}
	}
}

func TestStripSummary_RemovesRowsKeepsHeader(t *testing.T) {
	in := makeXLSX(t, [][]string{
		{"Name", "Amount"},
		{"Acme", "100"},
		{"Subtotal", "100"},
		{"Globex", "200"},
		{"Grand Total", "300"},
	})
	out, err := Run(in, []Transform{&StripSummaryTransform{}}, Context{})
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsFromXLSX(t, out)
	if len(rows) != 3 || rows[0][0] != "Name" || rows[1][0] != "Acme" || rows[2][0] != "Globex" {
		t.Fatalf("rows = %v", rows)
	}
}

func TestURLTransform_InsertsLinkColumn(t *testing.T) {
	in := makeXLSX(t, [][]string{
		{"AccountId", "Name"},
		{"001000000000001AAA", "Acme"},
		{"999000000000001AAA", "UnknownPrefix"}, // not in prefixMap → blank link
		{"not-an-id", "Garbage"},
	})
	ctx := Context{
		InstanceURL:     "https://example.my.salesforce.com",
		PrefixToSObject: map[string]string{"001": "Account"},
	}
	out, err := Run(in, []Transform{urlTransform{}}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	rows, _ := f.GetRows(sheet)
	if len(rows[0]) < 2 || rows[0][1] != "AccountId_link" {
		t.Fatalf("header = %v, want AccountId_link inserted after AccountId", rows[0])
	}
	formula, _ := f.GetCellFormula(sheet, "B2")
	if !strings.Contains(formula, "HYPERLINK") || !strings.Contains(formula, "001000000000001AAA") {
		t.Fatalf("B2 formula = %q", formula)
	}
	if got, _ := f.GetCellFormula(sheet, "B3"); got != "" {
		t.Fatalf("unknown-prefix row should be blank, got %q", got)
	}
	if got, _ := f.GetCellFormula(sheet, "B4"); got != "" {
		t.Fatalf("non-id row should be blank, got %q", got)
	}
}

func TestURLTransform_NoOpWithoutContext(t *testing.T) {
	in := makeXLSX(t, [][]string{
		{"AccountId"},
		{"001000000000001AAA"},
	})
	out, err := Run(in, []Transform{urlTransform{}}, Context{})
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsFromXLSX(t, out)
	if len(rows[0]) != 1 {
		t.Fatalf("link column inserted despite empty context: %v", rows[0])
	}
}

func TestLooksLikeSFID(t *testing.T) {
	cases := map[string]bool{
		"001000000000001AAA": true,  // 18
		"001000000000001":    true,  // 15
		"0010000000000":      false, // 13
		"001-00000000001AAA": false, // punctuation
		"":                   false,
	}
	for in, want := range cases {
		if got := looksLikeSFID(in); got != want {
			t.Errorf("looksLikeSFID(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestStripFormatting_PreservesContent(t *testing.T) {
	in := makeXLSX(t, [][]string{
		{"Name", "Amount"},
		{"Acme", "100"},
	})
	out, err := Run(in, []Transform{&StripFormattingTransform{}}, Context{})
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsFromXLSX(t, out)
	if len(rows) != 2 || rows[1][0] != "Acme" {
		t.Fatalf("content changed: %v", rows)
	}
}
