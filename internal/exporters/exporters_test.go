package exporters

import "testing"

// TestNeutralizeFormula covers the CSV/XLSX formula-injection guard:
// Salesforce field values are written verbatim, so a value starting with
// a spreadsheet formula trigger must be prefixed with a quote to render
// as literal text rather than evaluate.
func TestNeutralizeFormula(t *testing.T) {
	cases := []struct{ in, want string }{
		{"=HYPERLINK(\"http://evil\")", "'=HYPERLINK(\"http://evil\")"},
		{"+1234", "'+1234"},
		{"-1+2", "'-1+2"},
		{"@SUM(A1)", "'@SUM(A1)"},
		{"\tleading-tab", "'\tleading-tab"},
		{"\rleading-cr", "'\rleading-cr"},
		{"Acme Corp", "Acme Corp"}, // ordinary value untouched
		{"001ABC", "001ABC"},       // record id untouched
		{"", ""},                   // empty untouched
		{"a=b", "a=b"},             // = not in first position untouched
	}
	for _, c := range cases {
		if got := NeutralizeFormula(c.in); got != c.want {
			t.Errorf("NeutralizeFormula(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
