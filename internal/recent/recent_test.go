package recent

import (
	"testing"
	"time"
)

// --- Merge ----------------------------------------------------------------

func TestMerge_LocalOnly(t *testing.T) {
	now := time.Now()
	local := []Entry{
		{Kind: KindRecord, ID: "001A", Name: "Acme", Type: "Account", VisitedAt: now},
	}
	out := Merge(local, nil)
	if len(out) != 1 {
		t.Fatalf("got %d, want 1", len(out))
	}
	if out[0].Origin != OriginDeck {
		t.Errorf("got origin %q, want %q", out[0].Origin, OriginDeck)
	}
}

func TestMerge_SFOnly(t *testing.T) {
	out := Merge(nil, []SFRow{
		{ID: "001A", Name: "Acme", SObjectType: "Account", LastViewedDate: time.Now()},
	})
	if len(out) != 1 {
		t.Fatalf("got %d, want 1", len(out))
	}
	if out[0].Origin != OriginSF {
		t.Errorf("got origin %q, want %q", out[0].Origin, OriginSF)
	}
	if out[0].Kind != KindRecord {
		t.Errorf("got kind %q, want record", out[0].Kind)
	}
}

func TestMerge_DedupesBoth(t *testing.T) {
	now := time.Now()
	local := []Entry{
		{Kind: KindRecord, ID: "001A", Name: "Acme (local)", Type: "Account", VisitedAt: now},
	}
	sf := []SFRow{
		{ID: "001A", Name: "Acme (SF)", SObjectType: "Account", LastViewedDate: now.Add(-time.Hour)},
	}
	out := Merge(local, sf)
	if len(out) != 1 {
		t.Fatalf("dedupe failed: got %d, want 1", len(out))
	}
	// Local timestamp wins; entry tagged "both".
	if out[0].Origin != OriginBoth {
		t.Errorf("got origin %q, want %q", out[0].Origin, OriginBoth)
	}
	if out[0].Name != "Acme (local)" {
		t.Errorf("expected local name to win: got %q", out[0].Name)
	}
}

func TestMerge_SortMRU(t *testing.T) {
	now := time.Now()
	out := Merge(
		[]Entry{
			{Kind: KindRecord, ID: "a", VisitedAt: now.Add(-3 * time.Hour)},
			{Kind: KindRecord, ID: "b", VisitedAt: now.Add(-1 * time.Hour)},
			{Kind: KindRecord, ID: "c", VisitedAt: now.Add(-2 * time.Hour)},
		},
		nil,
	)
	if len(out) != 3 {
		t.Fatalf("got %d, want 3", len(out))
	}
	if out[0].ID != "b" || out[1].ID != "c" || out[2].ID != "a" {
		t.Errorf("MRU order wrong: %s,%s,%s want b,c,a", out[0].ID, out[1].ID, out[2].ID)
	}
}

func TestMerge_SkipsBlankSFRows(t *testing.T) {
	out := Merge(nil, []SFRow{
		{ID: "", Name: "no-id"},
		{ID: "001A", SObjectType: "", Name: "no-type"},
		{ID: "001B", SObjectType: "Account", Name: "valid"},
	})
	if len(out) != 1 {
		t.Errorf("got %d, want 1 (valid only)", len(out))
	}
}

// --- FilterByKinds --------------------------------------------------------

func TestFilterByKinds(t *testing.T) {
	in := []Entry{
		{Kind: KindRecord},
		{Kind: KindReport},
		{Kind: KindFlow},
		{Kind: KindRecord},
	}
	out := FilterByKinds(in, []string{KindReport, KindFlow})
	if len(out) != 2 {
		t.Fatalf("got %d, want 2", len(out))
	}
	for _, e := range out {
		if e.Kind != KindRecord {
			t.Errorf("leak: %+v", e)
		}
	}
}

func TestFilterByKinds_EmptyExcludedReturnsInput(t *testing.T) {
	in := []Entry{{Kind: KindRecord}}
	out := FilterByKinds(in, nil)
	if len(out) != 1 {
		t.Errorf("got %d, want 1", len(out))
	}
}

// --- Upsert ---------------------------------------------------------------

func TestUpsert_NewEntryFirst(t *testing.T) {
	now := time.Now()
	list := []Entry{
		{Kind: KindRecord, ID: "a", VisitedAt: now.Add(-time.Hour)},
	}
	out := Upsert(list, Entry{Kind: KindRecord, ID: "b", VisitedAt: now}, 50)
	if out[0].ID != "b" {
		t.Errorf("new entry should be first, got %s", out[0].ID)
	}
	if len(out) != 2 {
		t.Errorf("got %d, want 2", len(out))
	}
}

func TestUpsert_DedupesOnKindID(t *testing.T) {
	now := time.Now()
	list := []Entry{
		{Kind: KindRecord, ID: "a", VisitedAt: now.Add(-time.Hour)},
		{Kind: KindRecord, ID: "b", VisitedAt: now.Add(-2 * time.Hour)},
	}
	out := Upsert(list, Entry{Kind: KindRecord, ID: "a", VisitedAt: now}, 50)
	if len(out) != 2 {
		t.Errorf("got %d, want 2 (dedupe failed)", len(out))
	}
	if out[0].ID != "a" || out[1].ID != "b" {
		t.Errorf("dedupe order wrong: %v", out)
	}
}

func TestUpsert_CapsList(t *testing.T) {
	now := time.Now()
	list := []Entry{
		{Kind: KindRecord, ID: "a", VisitedAt: now.Add(-time.Hour)},
		{Kind: KindRecord, ID: "b", VisitedAt: now.Add(-2 * time.Hour)},
		{Kind: KindRecord, ID: "c", VisitedAt: now.Add(-3 * time.Hour)},
	}
	out := Upsert(list, Entry{Kind: KindRecord, ID: "d", VisitedAt: now}, 2)
	if len(out) != 2 {
		t.Errorf("got %d, want 2 (cap=2)", len(out))
	}
	if out[0].ID != "d" {
		t.Errorf("new entry should be first")
	}
}

// --- Formatters -----------------------------------------------------------

func TestNameForRow_FallsBackToTruncatedID(t *testing.T) {
	got := NameForRow(Entry{ID: "001A0000005xYZk"})
	if got != "001A…" {
		t.Errorf("got %q, want 001A…", got)
	}
}

func TestNameForRow_PreservesShortID(t *testing.T) {
	got := NameForRow(Entry{ID: "abc"})
	if got != "abc" {
		t.Errorf("got %q, want abc", got)
	}
}

func TestKindForSFType(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ListView", KindListView},
		{"User", KindUser},
		{"Report", KindReport},
		{"Account", KindRecord}, // unknown → record
		{"ApexTrigger", KindApexClass},
		{"LightningComponentBundle", KindLWC},
	}
	for _, c := range cases {
		if got := KindForSFType(c.in); got != c.want {
			t.Errorf("KindForSFType(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestKindLabel_UnknownPassThrough(t *testing.T) {
	if got := KindLabel("never-heard-of-it"); got != "never-heard-of-it" {
		t.Errorf("unknown kind should pass through: got %q", got)
	}
}

// --- Field --------------------------------------------------------------

func TestField(t *testing.T) {
	e := Entry{
		Kind: KindRecord, ID: "001A", Name: "Acme", Type: "Account", Origin: OriginBoth,
	}
	cases := []struct {
		field  string
		want   string
		wantOK bool
	}{
		{"Kind", KindRecord, true},
		{"Id", "001A", true},
		{"ID", "001A", true},
		{"Name", "Acme", true},
		{"Type", "Account", true},
		{"Origin", OriginBoth, true},
		{"unknown", "", false},
	}
	for _, c := range cases {
		v, ok := e.Field(c.field)
		if ok != c.wantOK {
			t.Errorf("Field(%q) ok = %v, want %v", c.field, ok, c.wantOK)
		}
		if ok && v != c.want {
			t.Errorf("Field(%q) = %v, want %v", c.field, v, c.want)
		}
	}
}

func TestField_OriginDefault(t *testing.T) {
	// Empty Origin → "deck" default.
	e := Entry{Kind: KindRecord, Origin: ""}
	v, ok := e.Field("Origin")
	if !ok || v != OriginDeck {
		t.Errorf("default origin: got (%v, %v), want (deck, true)", v, ok)
	}
}
