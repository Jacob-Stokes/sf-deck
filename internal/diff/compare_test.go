package diff

import (
	"errors"
	"testing"
)

// fakeProvider is an in-memory Provider for testing the compare engine
// without any Salesforce calls.
type fakeProvider struct {
	label   string
	byAlias map[string][]Component // alias -> components
	bodies  map[string]string      // id -> body
	listErr map[string]error       // alias -> error
}

func (f fakeProvider) TypeLabel() string { return f.label }
func (f fakeProvider) List(alias string) ([]Component, error) {
	if e := f.listErr[alias]; e != nil {
		return nil, e
	}
	return f.byAlias[alias], nil
}
func (f fakeProvider) Body(alias, id string) (string, error) { return f.bodies[id], nil }

func TestCompareInventoryClassifies(t *testing.T) {
	p := fakeProvider{
		label: "ApexClass",
		byAlias: map[string][]Component{
			"src": {
				{Type: "ApexClass", Key: "Same", ID: "a1", Summary: "v61"},
				{Type: "ApexClass", Key: "Diff", ID: "a2", Summary: "v60"},
				{Type: "ApexClass", Key: "OnlyA", ID: "a3", Summary: "v61"},
			},
			"tgt": {
				{Type: "ApexClass", Key: "Same", ID: "b1", Summary: "v61"},
				{Type: "ApexClass", Key: "Diff", ID: "b2", Summary: "v61"}, // summary differs
				{Type: "ApexClass", Key: "OnlyB", ID: "b4", Summary: "v61"},
			},
		},
	}
	inv := CompareInventory("src", "tgt", []Provider{p})

	byKey := map[string]Row{}
	for _, r := range inv.Rows {
		byKey[r.Key] = r
	}
	if got := byKey["Same"].Status; got != StatusSame {
		t.Errorf("Same status = %v, want SAME", got)
	}
	if got := byKey["Diff"].Status; got != StatusDifferent {
		t.Errorf("Diff status = %v, want DIFFERENT (summary mismatch)", got)
	}
	if got := byKey["OnlyA"].Status; got != StatusAOnly {
		t.Errorf("OnlyA status = %v, want A ONLY", got)
	}
	if got := byKey["OnlyB"].Status; got != StatusBOnly {
		t.Errorf("OnlyB status = %v, want B ONLY", got)
	}

	same, diff, aOnly, bOnly := inv.Summary()
	if same != 1 || diff != 1 || aOnly != 1 || bOnly != 1 {
		t.Errorf("summary = same%d diff%d a%d b%d, want 1/1/1/1", same, diff, aOnly, bOnly)
	}
}

func TestCompareInventorySurfacesListErrors(t *testing.T) {
	p := fakeProvider{
		label:   "ApexClass",
		byAlias: map[string][]Component{"tgt": nil},
		listErr: map[string]error{"src": errors.New("boom")},
	}
	inv := CompareInventory("src", "tgt", []Provider{p})
	if len(inv.Errors) != 1 || inv.Errors[0].Side != "source" {
		t.Fatalf("expected one source-side error, got %+v", inv.Errors)
	}
}

func TestCompareInventorySortedByTypeThenKey(t *testing.T) {
	p := fakeProvider{
		label: "ApexClass",
		byAlias: map[string][]Component{
			"src": {
				{Type: "ApexClass", Key: "Zebra", ID: "1"},
				{Type: "ApexClass", Key: "Apple", ID: "2"},
			},
			"tgt": {},
		},
	}
	inv := CompareInventory("src", "tgt", []Provider{p})
	if inv.Rows[0].Key != "Apple" || inv.Rows[1].Key != "Zebra" {
		t.Errorf("rows not sorted by key: %v, %v", inv.Rows[0].Key, inv.Rows[1].Key)
	}
}

func TestCompareSnapshots(t *testing.T) {
	a := Snapshot{
		"ApexClass": {
			"Same":  "line1\nline2",
			"Diff":  "line1\nold",
			"OnlyA": "x",
		},
	}
	b := Snapshot{
		"ApexClass": {
			"Same":  "line1\nline2",
			"Diff":  "line1\nnew",
			"OnlyB": "y",
		},
	}
	inv := CompareSnapshots(a, b)
	byKey := map[string]Row{}
	for _, r := range inv.Rows {
		byKey[r.Key] = r
	}
	if byKey["Same"].Status != StatusSame {
		t.Errorf("Same = %v", byKey["Same"].Status)
	}
	if byKey["Diff"].Status != StatusDifferent {
		t.Errorf("Diff = %v", byKey["Diff"].Status)
	}
	if byKey["OnlyA"].Status != StatusAOnly {
		t.Errorf("OnlyA = %v", byKey["OnlyA"].Status)
	}
	if byKey["OnlyB"].Status != StatusBOnly {
		t.Errorf("OnlyB = %v", byKey["OnlyB"].Status)
	}

	// Body diff straight from snapshots, no fetch.
	res := BodyDiffFromSnapshots(byKey["Diff"], a, b)
	if res.Added != 1 || res.Removed != 1 {
		t.Errorf("snapshot body diff added=%d removed=%d, want 1/1", res.Added, res.Removed)
	}
}

// TestCompareSnapshotsSurfacesFailedTypes covers audit-7 #18/#19: a type
// that failed to retrieve on one side must be surfaced on Inv.Errors and
// its rows suppressed — NOT emitted as phantom one-sided drift.
func TestCompareSnapshotsSurfacesFailedTypes(t *testing.T) {
	// "Layout" failed on the source side, so a only has it empty while b is
	// full. Without the error it would emit every Layout as B-only drift.
	a := Snapshot{"ApexClass": {"Keep": "x"}}
	b := Snapshot{
		"ApexClass": {"Keep": "x"},
		"Layout":    {"L1": "y", "L2": "z"},
	}
	failed := TypeError{Type: "Layout", Side: "source", Err: errors.New("timeout")}
	inv := CompareSnapshots(a, b, failed)

	if len(inv.Errors) != 1 || inv.Errors[0].Type != "Layout" {
		t.Fatalf("failed type not surfaced on Inv.Errors: %+v", inv.Errors)
	}
	for _, r := range inv.Rows {
		if r.Type == "Layout" {
			t.Errorf("failed type emitted phantom drift row: %+v", r)
		}
	}
	// The healthy type still diffs.
	var sawApex bool
	for _, r := range inv.Rows {
		if r.Type == "ApexClass" && r.Key == "Keep" {
			sawApex = true
			if r.Status != StatusSame {
				t.Errorf("healthy type misclassified: %v", r.Status)
			}
		}
	}
	if !sawApex {
		t.Error("healthy ApexClass row missing")
	}
}

func TestNormalizeBodyIgnoresTrailingWhitespace(t *testing.T) {
	// Trailing whitespace + trailing blank lines must not count as a diff.
	a := Snapshot{"T": {"X": "line1  \nline2\t\n\n"}}
	b := Snapshot{"T": {"X": "line1\nline2\n"}}
	inv := CompareSnapshots(a, b)
	if len(inv.Rows) != 1 || inv.Rows[0].Status != StatusSame {
		t.Errorf("cosmetic whitespace diff reported as change: %+v", inv.Rows)
	}
}

func TestBodyDiffMatchedPair(t *testing.T) {
	p := fakeProvider{
		label:  "ApexClass",
		bodies: map[string]string{"a": "line1\nold\nline3", "b": "line1\nnew\nline3"},
	}
	row := Row{Type: "ApexClass", Key: "X", Status: StatusDifferent, AID: "a", BID: "b"}
	res, err := BodyDiff(row, "src", "tgt", p)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 || res.Removed != 1 {
		t.Errorf("body diff added=%d removed=%d, want 1/1", res.Added, res.Removed)
	}
}

// BenchmarkCompareSnapshotsScale approximates the production worst case:
// ~80k components per side, most identical (cheap path), with large XML
// bodies. Proves the diff no longer does PrettyXML on every matched row
// (the cause of the 99% UI freeze).
func BenchmarkCompareSnapshotsScale(b *testing.B) {
	bigXML := ""
	for i := 0; i < 200; i++ {
		bigXML += "<field><fullName>F" + itoaB(i) + "__c</fullName><type>Text</type></field>"
	}
	a := Snapshot{}
	bb := Snapshot{}
	const types, perType = 20, 500 // 10k components (scaled down; runs fast)
	for ti := 0; ti < types; ti++ {
		tn := "Type" + itoaB(ti)
		a[tn] = map[string]string{}
		bb[tn] = map[string]string{}
		for ci := 0; ci < perType; ci++ {
			key := "C" + itoaB(ci)
			a[tn][key] = bigXML
			// 1 in 100 differs; the rest are identical (cheap path).
			if ci%100 == 0 {
				bb[tn][key] = bigXML + "<extra/>"
			} else {
				bb[tn][key] = bigXML
			}
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CompareSnapshots(a, bb)
	}
}

func itoaB(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
