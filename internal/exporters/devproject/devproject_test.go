package devproject

import (
	"strings"
	"testing"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/exporters"
)

func TestRowsShape(t *testing.T) {
	added := time.Date(2026, 4, 29, 10, 30, 0, 0, time.UTC)
	items := []devproject.Item{
		{
			DevProjectID: "dp1", OrgUser: "dev@example.com",
			Kind: devproject.KindSObject, Ref: "Account", Type: "",
			Name: "Account", AddedAt: added,
		},
		{
			DevProjectID: "dp1", OrgUser: "dev@example.com",
			Kind: devproject.KindField, Ref: "Account.Phone", Type: "Account",
			Name: "Phone", AddedAt: added,
		},
		{
			DevProjectID: "dp1", OrgUser: "prod@example.com",
			Kind: devproject.KindApexClass, Ref: "01p4I0000000abc", Type: "ApexClass",
			Name: "AccountUtil", AddedAt: added, Notes: "needed for Q2",
		},
	}

	resolver := func(it devproject.Item) string {
		return "https://example.lightning.force.com/" + it.Ref
	}

	rows := Rows(items, resolver)
	if got, want := len(rows), 3; got != want {
		t.Fatalf("rows: got %d, want %d", got, want)
	}

	// Spot-check the first row
	r := rows[0]
	if r.Get("Name") != "Account" {
		t.Errorf("Name: got %q, want %q", r.Get("Name"), "Account")
	}
	if r.Get("Kind") != "sObject" {
		t.Errorf("Kind: got %q, want %q", r.Get("Kind"), "sObject")
	}
	if r.Get("Org") != "dev@example.com" {
		t.Errorf("Org: got %q", r.Get("Org"))
	}
	if !strings.HasPrefix(r.Get("URL"), "https://example.lightning") {
		t.Errorf("URL not resolved: got %q", r.Get("URL"))
	}
	if r.Get("Added") != "2026-04-29T10:30:00Z" {
		t.Errorf("Added timestamp: got %q", r.Get("Added"))
	}

	// Notes column should preserve the user's note
	if rows[2].Get("Notes") != "needed for Q2" {
		t.Errorf("Notes: got %q", rows[2].Get("Notes"))
	}

	// Missing column reads as empty rather than panicking
	if rows[0].Get("Notes") != "" {
		t.Errorf("expected empty Notes for first row, got %q", rows[0].Get("Notes"))
	}
}

// TestRowsNilResolver — URLResolver=nil should produce empty URL
// strings rather than panicking. Keeps the export usable from
// contexts that don't have org instance URLs handy.
func TestRowsNilResolver(t *testing.T) {
	items := []devproject.Item{
		{Kind: devproject.KindFlow, Ref: "0F1", Name: "MyFlow"},
	}
	rows := Rows(items, nil)
	if len(rows) != 1 {
		t.Fatalf("rows: got %d", len(rows))
	}
	if rows[0].Get("URL") != "" {
		t.Errorf("URL should be empty with nil resolver, got %q", rows[0].Get("URL"))
	}
}

// TestSuggestedFilename — the slug helper used to default the
// export filename. Don't go overboard with edge cases; just enough
// to pin behaviour for the common shapes.
func TestSuggestedFilename(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Q2 migration", "q2-migration"},
		{"Customer-360 / cleanup", "customer-360-cleanup"},
		{"  trim me  ", "trim-me"},
		{"!!!!", "dev-project"},
		{"", "dev-project"},
		{"AB CD", "ab-cd"},
	}
	for _, tc := range cases {
		if got := SuggestedFilename(tc.in); got != tc.want {
			t.Errorf("SuggestedFilename(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestWriteCSV — round-trip the CSV output to confirm column order
// + escaping work. Spot-check, not exhaustive.
func TestWriteCSV(t *testing.T) {
	items := []devproject.Item{
		{Kind: devproject.KindSObject, Ref: "Account", Name: "Acc, with comma"},
	}
	rows := Rows(items, nil)
	var buf strings.Builder
	if err := exporters.Write(&buf, exporters.FormatCSV, Headers, rows, "Test"); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := buf.String()
	// Header row first
	if !strings.HasPrefix(got, "Name,Kind,Ref,Parent,Org,URL,Added,Notes\n") {
		t.Errorf("header row missing or wrong:\n%s", got)
	}
	// Comma-containing value should be quoted by encoding/csv
	if !strings.Contains(got, `"Acc, with comma"`) {
		t.Errorf("comma value not quoted in CSV:\n%s", got)
	}
}

// TestWriteJSON — confirm key order + escaping. JSON spec doesn't
// require key order, but our writer preserves Headers order
// deliberately so scripts can rely on it.
func TestWriteJSON(t *testing.T) {
	items := []devproject.Item{
		{Kind: devproject.KindFlow, Ref: "0F1", Name: `Flow "with quotes"`},
	}
	rows := Rows(items, nil)
	var buf strings.Builder
	if err := exporters.Write(&buf, exporters.FormatJSON, Headers, rows, ""); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := buf.String()
	// First key in the object should be "Name" (matches Headers[0])
	if !strings.Contains(got, `"Name":"Flow \"with quotes\""`) {
		t.Errorf("JSON output mis-shaped:\n%s", got)
	}
	// Trailing newline so scripts can append
	if !strings.HasSuffix(got, "]\n") {
		t.Errorf("expected trailing newline+]: %q", got[len(got)-3:])
	}
}
