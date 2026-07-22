package devproject

import (
	"testing"
	"time"
)

func newHistoryStore(t *testing.T) *Store {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// ----- LogApexHistory + ListApexHistory roundtrip ----------------

func TestApexHistory_RoundtripFields(t *testing.T) {
	s := newHistoryStore(t)
	now := time.Unix(time.Now().Unix(), 0) // truncate to seconds; SQLite stores Unix epoch
	in := ApexHistoryEntry{
		OrgUser:          "user@example.com",
		Body:             "System.debug('x');",
		ExecutedAt:       now,
		DurationMs:       42,
		Compiled:         true,
		Success:          true,
		CompileProblem:   "",
		ExceptionMessage: "",
		Line:             0,
		Column:           0,
		LogID:            "07L00000abc",
		LogBody:          "USER_DEBUG ...",
	}
	id, err := s.LogApexHistory(in)
	if err != nil {
		t.Fatalf("LogApexHistory: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero id")
	}
	rows, err := s.ListApexHistory("user@example.com", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("len = %d, want 1", len(rows))
	}
	got := rows[0]
	if got.Body != in.Body {
		t.Errorf("Body = %q", got.Body)
	}
	if !got.ExecutedAt.Equal(now) {
		t.Errorf("ExecutedAt = %v, want %v", got.ExecutedAt, now)
	}
	if got.DurationMs != 42 {
		t.Errorf("DurationMs = %d", got.DurationMs)
	}
	if !got.Compiled || !got.Success {
		t.Errorf("flags lost: %+v", got)
	}
	if got.LogID != "07L00000abc" {
		t.Errorf("LogID = %q", got.LogID)
	}
}

func TestApexHistory_PopulatesExecutedAtWhenZero(t *testing.T) {
	s := newHistoryStore(t)
	id, err := s.LogApexHistory(ApexHistoryEntry{
		OrgUser: "u@x.com",
		Body:    "System.debug('x');",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Error("zero id")
	}
	rows, _ := s.ListApexHistory("u@x.com", 1)
	if rows[0].ExecutedAt.IsZero() {
		t.Error("ExecutedAt left zero — handler should default to now()")
	}
}

func TestApexHistory_FilterByOrg(t *testing.T) {
	s := newHistoryStore(t)
	_, _ = s.LogApexHistory(ApexHistoryEntry{OrgUser: "a@x.com", Body: "a"})
	_, _ = s.LogApexHistory(ApexHistoryEntry{OrgUser: "b@x.com", Body: "b"})
	_, _ = s.LogApexHistory(ApexHistoryEntry{OrgUser: "a@x.com", Body: "a2"})

	rows, _ := s.ListApexHistory("a@x.com", 10)
	if len(rows) != 2 {
		t.Errorf("a@x.com filter: len = %d, want 2", len(rows))
	}
	for _, r := range rows {
		if r.OrgUser != "a@x.com" {
			t.Errorf("leaked org row: %v", r)
		}
	}

	// Empty orgUser returns everything.
	all, _ := s.ListApexHistory("", 10)
	if len(all) != 3 {
		t.Errorf("all-org: len = %d, want 3", len(all))
	}
}

func TestApexHistory_LimitRespected(t *testing.T) {
	s := newHistoryStore(t)
	for i := 0; i < 5; i++ {
		_, _ = s.LogApexHistory(ApexHistoryEntry{OrgUser: "u", Body: "x"})
	}
	rows, _ := s.ListApexHistory("u", 2)
	if len(rows) != 2 {
		t.Errorf("limit ignored: len = %d, want 2", len(rows))
	}
	// limit=0 means no limit
	rows, _ = s.ListApexHistory("u", 0)
	if len(rows) != 5 {
		t.Errorf("limit=0: len = %d, want 5", len(rows))
	}
}

// ----- TrimApexHistory -----------------------------------------

func TestTrimApexHistory_KeepsMostRecentPerOrg(t *testing.T) {
	s := newHistoryStore(t)
	for i := 0; i < 6; i++ {
		_, _ = s.LogApexHistory(ApexHistoryEntry{OrgUser: "u@x.com", Body: "x"})
	}
	for i := 0; i < 4; i++ {
		_, _ = s.LogApexHistory(ApexHistoryEntry{OrgUser: "u2@x.com", Body: "y"})
	}
	if err := s.TrimApexHistory(3); err != nil {
		t.Fatalf("TrimApexHistory: %v", err)
	}
	left1, _ := s.ListApexHistory("u@x.com", 100)
	left2, _ := s.ListApexHistory("u2@x.com", 100)
	if len(left1) != 3 {
		t.Errorf("u@x.com: kept %d, want 3", len(left1))
	}
	if len(left2) != 3 {
		// 4 rows total, keep=3 → 3 remaining
		t.Errorf("u2@x.com: kept %d, want 3", len(left2))
	}
}

func TestTrimApexHistory_ZeroKeepIsNoop(t *testing.T) {
	s := newHistoryStore(t)
	for i := 0; i < 5; i++ {
		_, _ = s.LogApexHistory(ApexHistoryEntry{OrgUser: "u", Body: "x"})
	}
	if err := s.TrimApexHistory(0); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.ListApexHistory("u", 100)
	if len(rows) != 5 {
		t.Errorf("rows = %d, want 5 (no-op trim)", len(rows))
	}
}

// ----- ApexHistoryEntry.Field --------------------------------

func TestApexHistoryEntry_FieldExtraction(t *testing.T) {
	e := ApexHistoryEntry{
		OrgUser:          "u@x.com",
		Body:             "System.debug('x');",
		ExecutedAt:       time.Unix(123456, 0),
		DurationMs:       42,
		Compiled:         true,
		Success:          true,
		CompileProblem:   "",
		ExceptionMessage: "",
		Line:             10,
		Column:           5,
		LogID:            "07Lxyz",
		LogBody:          "blob",
	}
	cases := []struct {
		name string
		want any
	}{
		{"OrgUser", "u@x.com"},
		{"Org", "u@x.com"},
		{"Body", "System.debug('x');"},
		{"DurationMs", 42},
		{"Duration", 42},
		{"Compiled", true},
		{"Success", true},
		{"Line", 10},
		{"Column", 5},
		{"LogID", "07Lxyz"},
		{"HasLog", true},
		{"Status", "ok"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := e.Field(c.name)
			if !ok {
				t.Fatalf("Field(%q) returned !ok", c.name)
			}
			if got != c.want {
				t.Errorf("Field(%q) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

func TestApexHistoryEntry_FieldStatus(t *testing.T) {
	cases := []struct {
		entry ApexHistoryEntry
		want  string
	}{
		{ApexHistoryEntry{Compiled: false}, "compile_error"},
		{ApexHistoryEntry{Compiled: true, Success: false}, "runtime_error"},
		{ApexHistoryEntry{Compiled: true, Success: true}, "ok"},
	}
	for _, c := range cases {
		got, _ := c.entry.Field("Status")
		if got != c.want {
			t.Errorf("entry %+v: Status = %v, want %v", c.entry, got, c.want)
		}
	}
}

func TestApexHistoryEntry_FieldHasLogFalse(t *testing.T) {
	e := ApexHistoryEntry{LogID: ""}
	got, _ := e.Field("HasLog")
	if got != false {
		t.Errorf("HasLog = %v, want false", got)
	}
}

func TestApexHistoryEntry_UnknownField(t *testing.T) {
	e := ApexHistoryEntry{}
	if _, ok := e.Field("NotARealField"); ok {
		t.Error("unknown field should return ok=false")
	}
}
