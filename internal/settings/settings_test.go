package settings

import (
	"testing"
	"time"
)

// --- SafetyLevel ----------------------------------------------------------

func TestSafetyLevel_String(t *testing.T) {
	cases := []struct {
		level SafetyLevel
		want  string
	}{
		{SafetyReadOnly, "read_only"},
		{SafetyRecords, "records"},
		{SafetyMetadata, "metadata"},
		{SafetyFull, "full"},
		{SafetyLevel(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.level.String(); got != c.want {
			t.Errorf("SafetyLevel(%d).String() = %q, want %q", c.level, got, c.want)
		}
	}
}

func TestSafetyLevel_Label(t *testing.T) {
	cases := []struct {
		level SafetyLevel
		want  string
	}{
		{SafetyReadOnly, "READ"},
		{SafetyRecords, "REC"},
		{SafetyMetadata, "META"},
		{SafetyFull, "FULL"},
		{SafetyLevel(99), "?"},
	}
	for _, c := range cases {
		if got := c.level.Label(); got != c.want {
			t.Errorf("SafetyLevel(%d).Label() = %q, want %q", c.level, got, c.want)
		}
	}
}

func TestParseSafetyLevel(t *testing.T) {
	cases := []struct {
		in   string
		want SafetyLevel
	}{
		{"read_only", SafetyReadOnly},
		{"readonly", SafetyReadOnly},
		{"RO", SafetyReadOnly},
		{"records", SafetyRecords},
		{"REC", SafetyRecords},
		{"metadata", SafetyMetadata},
		{"meta", SafetyMetadata},
		{"full", SafetyFull},
		{"FULL", SafetyFull},
		{"  records  ", SafetyRecords}, // whitespace tolerance
		// Unknown values fail closed to ReadOnly.
		{"yolo", SafetyReadOnly},
		{"", SafetyReadOnly},
	}
	for _, c := range cases {
		if got := ParseSafetyLevel(c.in); got != c.want {
			t.Errorf("ParseSafetyLevel(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestSafetyLevel_AllowsLadder confirms each level grants the
// minimum-required write-kind and nothing more.
func TestSafetyLevel_AllowsLadder(t *testing.T) {
	cases := []struct {
		level SafetyLevel
		rec   bool
		meta  bool
		anon  bool
	}{
		{SafetyReadOnly, false, false, false},
		{SafetyRecords, true, false, false},
		{SafetyMetadata, true, true, false},
		{SafetyFull, true, true, true},
	}
	for _, c := range cases {
		if got := c.level.Allows(WriteRecord); got != c.rec {
			t.Errorf("%s.Allows(WriteRecord) = %v, want %v", c.level, got, c.rec)
		}
		if got := c.level.Allows(WriteMetadata); got != c.meta {
			t.Errorf("%s.Allows(WriteMetadata) = %v, want %v", c.level, got, c.meta)
		}
		if got := c.level.Allows(WriteAnonymous); got != c.anon {
			t.Errorf("%s.Allows(WriteAnonymous) = %v, want %v", c.level, got, c.anon)
		}
	}
}

// --- DefaultChipLimit -----------------------------------------------------

func TestDefaultChipLimit_NilSettings(t *testing.T) {
	var s *Settings
	if got := s.DefaultChipLimit(); got != DefaultChipLimitFallback {
		t.Errorf("nil Settings: got %d, want fallback %d", got, DefaultChipLimitFallback)
	}
}

func TestDefaultChipLimit_Override(t *testing.T) {
	s := &Settings{}
	s.UI.ChipDefaults.DefaultLimit = 500
	if got := s.DefaultChipLimit(); got != 500 {
		t.Errorf("got %d, want 500", got)
	}
}

func TestDefaultChipLimit_ZeroFallsBackToFallback(t *testing.T) {
	s := &Settings{}
	s.UI.ChipDefaults.DefaultLimit = 0
	if got := s.DefaultChipLimit(); got != DefaultChipLimitFallback {
		t.Errorf("got %d, want %d", got, DefaultChipLimitFallback)
	}
}

func TestDefaultChipLimit_NegativeReturnsOne(t *testing.T) {
	s := &Settings{}
	s.UI.ChipDefaults.DefaultLimit = -5
	if got := s.DefaultChipLimit(); got != 1 {
		t.Errorf("got %d, want 1 (negative clamps to 1)", got)
	}
}

func TestSetDefaultChipLimit_RoundTrip(t *testing.T) {
	s := &Settings{}
	s.SetDefaultChipLimit(750)
	if got := s.DefaultChipLimit(); got != 750 {
		t.Errorf("got %d, want 750", got)
	}
	// 0 clears the override.
	s.SetDefaultChipLimit(0)
	if got := s.DefaultChipLimit(); got != DefaultChipLimitFallback {
		t.Errorf("after clear: got %d, want fallback", got)
	}
}

func TestSetDefaultChipLimit_NilSafe(t *testing.T) {
	var s *Settings
	// Should not panic.
	s.SetDefaultChipLimit(1000)
}

// --- CacheTTL ------------------------------------------------------------

func TestCacheTTL_NilSettingsUsesFallback(t *testing.T) {
	var s *Settings
	got := s.CacheTTL("records", 30*time.Minute)
	if got != 30*time.Minute {
		t.Errorf("got %v, want 30m fallback", got)
	}
}

func TestCacheTTL_OverrideBeatsDefault(t *testing.T) {
	s := &Settings{}
	s.UI.Cache.DefaultTTL = "1h"
	s.UI.Cache.TTL = map[string]string{"records": "5m"}
	got := s.CacheTTL("records", 30*time.Minute)
	if got != 5*time.Minute {
		t.Errorf("got %v, want 5m (override)", got)
	}
}

func TestCacheTTL_DefaultBeatsFallback(t *testing.T) {
	s := &Settings{}
	s.UI.Cache.DefaultTTL = "2h"
	got := s.CacheTTL("records", 30*time.Minute)
	if got != 2*time.Hour {
		t.Errorf("got %v, want 2h (default_ttl)", got)
	}
}

func TestCacheTTL_InvalidOverrideFallsThrough(t *testing.T) {
	s := &Settings{}
	s.UI.Cache.TTL = map[string]string{"records": "not-a-duration"}
	got := s.CacheTTL("records", 30*time.Minute)
	if got != 30*time.Minute {
		t.Errorf("got %v, want fallback when override unparseable", got)
	}
}

func TestSetCacheTTLOverride_RoundTrip(t *testing.T) {
	s := &Settings{}
	s.SetCacheTTLOverride("records", "10m")
	if got := s.CacheTTLOverride("records"); got != "10m" {
		t.Errorf("got %q, want %q", got, "10m")
	}
	// Empty value clears the entry.
	s.SetCacheTTLOverride("records", "")
	if got := s.CacheTTLOverride("records"); got != "" {
		t.Errorf("after clear: got %q, want empty", got)
	}
}

func TestCacheTTLOverride_NilSafe(t *testing.T) {
	var s *Settings
	if got := s.CacheTTLOverride("records"); got != "" {
		t.Errorf("nil Settings: got %q, want empty", got)
	}
}

// --- CompareConcurrency ---------------------------------------------------

func TestCompareConcurrency(t *testing.T) {
	cases := []struct {
		name string
		set  int
		want int
	}{
		{"unset uses default", 0, defaultCompareConcurrency},
		{"negative uses default", -3, defaultCompareConcurrency},
		{"in range honored", 10, 10},
		{"over ceiling clamps", 1000, 32},
	}
	for _, c := range cases {
		s := &Settings{}
		s.UI.Compare.Concurrency = c.set
		if got := s.CompareConcurrency(); got != c.want {
			t.Errorf("%s: CompareConcurrency(%d) = %d, want %d", c.name, c.set, got, c.want)
		}
	}
	// nil Settings is safe and yields the default.
	var s *Settings
	if got := s.CompareConcurrency(); got != defaultCompareConcurrency {
		t.Errorf("nil Settings: got %d, want %d", got, defaultCompareConcurrency)
	}
}

func TestCompareBodyCapBytes(t *testing.T) {
	cases := []struct {
		set, wantKB int
	}{
		{0, defaultCompareBodyCapKB}, // unset → default
		{-5, defaultCompareBodyCapKB},
		{256, 256},
		{2, 8},             // below floor → 8KB
		{1 << 30, 1 << 20}, // above ceiling → 1GB in KB
	}
	for _, c := range cases {
		s := &Settings{}
		s.UI.Compare.BodyCapKB = c.set
		if got := s.CompareBodyCapBytes(); got != c.wantKB*1024 {
			t.Errorf("BodyCapBytes(%d) = %d, want %d", c.set, got, c.wantKB*1024)
		}
	}
	var nilS *Settings
	if got := nilS.CompareBodyCapBytes(); got != defaultCompareBodyCapKB*1024 {
		t.Errorf("nil Settings cap = %d", got)
	}
}

func TestCompareRetainCeilingBytes(t *testing.T) {
	s := &Settings{}
	if got := s.CompareRetainCeilingBytes(); got != int64(defaultCompareRetainCeilingMB)*1024*1024 {
		t.Errorf("unset ceiling = %d", got)
	}
	s.UI.Compare.RetainCeilingMB = 300
	if got := s.CompareRetainCeilingBytes(); got != 300*1024*1024 {
		t.Errorf("ceiling(300) = %d", got)
	}
	s.UI.Compare.RetainCeilingMB = -1
	if got := s.CompareRetainCeilingBytes(); got != int64(defaultCompareRetainCeilingMB)*1024*1024 {
		t.Errorf("negative ceiling should fall back to default, got %d", got)
	}
	var nilS *Settings
	if got := nilS.CompareRetainCeilingBytes(); got != int64(defaultCompareRetainCeilingMB)*1024*1024 {
		t.Errorf("nil Settings ceiling = %d", got)
	}
}
