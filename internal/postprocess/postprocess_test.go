package postprocess

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// makeXLSX builds a minimal xlsx with one sheet + the provided rows.
// Used to feed Run() / ToCSV() in tests.
func makeXLSX(t *testing.T, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)
	for r, row := range rows {
		for c, val := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
			if err := f.SetCellValue(sheet, cell, val); err != nil {
				t.Fatalf("SetCellValue: %v", err)
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}
	return buf.Bytes()
}

func TestToCSVNeutralizesSpreadsheetFormulas(t *testing.T) {
	in := makeXLSX(t, [][]string{
		{"Name", "Value"},
		{"Danger", `=HYPERLINK("https://evil.example","click")`},
		{"Also danger", "+1+1"},
	})
	out, err := ToCSV(in)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(out))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if got := rows[1][1]; !strings.HasPrefix(got, "'=") {
		t.Fatalf("formula cell = %q, want neutralized literal", got)
	}
	if got := rows[2][1]; got != "'+1+1" {
		t.Fatalf("plus cell = %q, want neutralized literal", got)
	}
}

// --- All() / ByIDs() ------------------------------------------------------

func TestAll_ReturnsKnownTransforms(t *testing.T) {
	got := All()
	if len(got) == 0 {
		t.Fatal("All() empty — should include at least url + detailsify")
	}
	// Spot-check: known ids must be present.
	ids := map[string]bool{}
	for _, tr := range got {
		ids[tr.ID()] = true
	}
	for _, want := range []string{"url", "detailsify", "strip-summary", "strip-formatting"} {
		if !ids[want] {
			t.Errorf("All() missing transform %q", want)
		}
	}
}

func TestByIDs_FiltersAndOrders(t *testing.T) {
	got := ByIDs([]string{"detailsify", "url"})
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	// Order should follow the request, not the All() order.
	if got[0].ID() != "detailsify" {
		t.Errorf("got[0].ID() = %q, want detailsify", got[0].ID())
	}
	if got[1].ID() != "url" {
		t.Errorf("got[1].ID() = %q, want url", got[1].ID())
	}
}

func TestByIDs_SkipsUnknown(t *testing.T) {
	got := ByIDs([]string{"detailsify", "nonexistent", "url"})
	if len(got) != 2 {
		t.Errorf("got %d, want 2 (unknown should be silently dropped)", len(got))
	}
}

func TestByIDs_EmptyReturnsEmpty(t *testing.T) {
	got := ByIDs(nil)
	if len(got) != 0 {
		t.Errorf("nil ids: got %d, want 0", len(got))
	}
}

// --- Run() ----------------------------------------------------------------

// fakeTransform is a controllable Transform for testing Run's pipeline
// behaviour without depending on real transforms' workbook mutations.
type fakeTransform struct {
	id      string
	err     error
	applied *bool
}

func (f *fakeTransform) ID() string    { return f.id }
func (f *fakeTransform) Label() string { return f.id }
func (f *fakeTransform) Apply(wb *excelize.File, ctx Context) error {
	if f.applied != nil {
		*f.applied = true
	}
	return f.err
}

func TestRun_ZeroTransformsRoundTrips(t *testing.T) {
	in := makeXLSX(t, [][]string{{"A", "B"}, {"1", "2"}})
	out, err := Run(in, nil, Context{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("Run with 0 transforms returned empty bytes")
	}
	// Round-tripped bytes won't byte-equal the original (excelize rewrites
	// metadata), but they should re-open as a valid xlsx.
	if _, err := excelize.OpenReader(bytes.NewReader(out)); err != nil {
		t.Errorf("Run output isn't valid xlsx: %v", err)
	}
}

func TestRun_AppliesTransformsInOrder(t *testing.T) {
	in := makeXLSX(t, [][]string{{"A"}})
	var aRan, bRan bool
	transforms := []Transform{
		&fakeTransform{id: "a", applied: &aRan},
		&fakeTransform{id: "b", applied: &bRan},
	}
	if _, err := Run(in, transforms, Context{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !aRan || !bRan {
		t.Errorf("transforms didn't all run: a=%v b=%v", aRan, bRan)
	}
}

func TestRun_StopsOnTransformError(t *testing.T) {
	in := makeXLSX(t, [][]string{{"A"}})
	boom := errors.New("kaboom")
	var bRan bool
	transforms := []Transform{
		&fakeTransform{id: "a", err: boom},
		&fakeTransform{id: "b", applied: &bRan},
	}
	_, err := Run(in, transforms, Context{})
	if err == nil {
		t.Fatal("Run should propagate transform error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error %q should wrap original boom", err)
	}
	if bRan {
		t.Error("b ran after a errored — pipeline should stop on first error")
	}
}

func TestRun_BadXLSXReturnsError(t *testing.T) {
	_, err := Run([]byte("not an xlsx"), nil, Context{})
	if err == nil {
		t.Fatal("Run should reject non-xlsx bytes")
	}
}

// --- ToCSV() --------------------------------------------------------------

func TestToCSV_RoundTrip(t *testing.T) {
	in := makeXLSX(t, [][]string{
		{"Name", "Industry", "Revenue"},
		{"Acme", "Tech", "100"},
		{"Globex", "Finance", "200"},
	})
	out, err := ToCSV(in)
	if err != nil {
		t.Fatalf("ToCSV: %v", err)
	}
	got := string(out)
	for _, want := range []string{"Name,Industry,Revenue", "Acme,Tech,100", "Globex,Finance,200"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("CSV missing %q\nfull output:\n%s", want, got)
		}
	}
}

func TestToCSV_RaggedRowsPadded(t *testing.T) {
	// Row 2 is shorter than row 1 — the CSV writer pads to the widest.
	in := makeXLSX(t, [][]string{
		{"A", "B", "C"},
		{"x"},
	})
	out, err := ToCSV(in)
	if err != nil {
		t.Fatalf("ToCSV: %v", err)
	}
	// Short row should pad: "x,," — two trailing empty cells.
	if !bytes.Contains(out, []byte("x,,")) {
		t.Errorf("ragged row not padded:\n%s", out)
	}
}

func TestToCSV_BadXLSXReturnsError(t *testing.T) {
	_, err := ToCSV([]byte("not xlsx"))
	if err == nil {
		t.Fatal("ToCSV should reject non-xlsx bytes")
	}
}
