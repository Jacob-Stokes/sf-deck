package settings

import "testing"

func TestPinDefault_SetAndRead(t *testing.T) {
	s := &Settings{}
	if !s.PinDefault("alice@example.com") {
		t.Error("PinDefault returned changed=false on first set")
	}
	if got := s.DefaultOrgUsername(); got != "alice@example.com" {
		t.Errorf("DefaultOrgUsername = %q, want alice@example.com", got)
	}
}

func TestPinDefault_IsExclusive(t *testing.T) {
	// Set alice → bob. Only one default at a time.
	s := &Settings{}
	s.PinDefault("alice@x")
	if !s.PinDefault("bob@x") {
		t.Error("PinDefault returned changed=false when re-pinning")
	}
	if got := s.DefaultOrgUsername(); got != "bob@x" {
		t.Errorf("after re-pin, default = %q", got)
	}
	// Alice's entry should still exist only if she has a Safety
	// override; in this test she has none, so her entry must have
	// been dropped from the map to keep TOML tidy.
	if _, ok := s.Orgs["alice@x"]; ok {
		t.Errorf("alice's entry should be removed (no other config); got %+v", s.Orgs["alice@x"])
	}
	// Bob has Default=true.
	if !s.Orgs["bob@x"].Default {
		t.Errorf("bob.Default = false; want true")
	}
}

func TestPinDefault_PreservesSafetyOnOtherEntries(t *testing.T) {
	// Alice has Safety=records + Default=true. Re-pinning to bob
	// must clear alice.Default but keep alice.Safety.
	s := &Settings{Orgs: map[string]OrgConfig{
		"alice@x": {Safety: "records", Default: true},
	}}
	s.PinDefault("bob@x")
	if cfg, ok := s.Orgs["alice@x"]; !ok || cfg.Safety != "records" || cfg.Default {
		t.Errorf("alice = %+v, want {Safety:records, Default:false}", cfg)
	}
}

func TestPinDefault_Clear(t *testing.T) {
	s := &Settings{}
	s.PinDefault("alice@x")
	if !s.PinDefault("") {
		t.Error("PinDefault(\"\") returned changed=false when there was a pin")
	}
	if got := s.DefaultOrgUsername(); got != "" {
		t.Errorf("after clear, default = %q, want empty", got)
	}
	// Repeat clear → no change.
	if s.PinDefault("") {
		t.Error("PinDefault(\"\") on already-clear state returned changed=true")
	}
}

func TestPinDefault_Idempotent(t *testing.T) {
	s := &Settings{}
	s.PinDefault("alice@x")
	if s.PinDefault("alice@x") {
		t.Error("re-pinning same org reported changed=true")
	}
}

func TestSetOrg_PreservesDefaultPin(t *testing.T) {
	// Regression: changing safety must not wipe Default.
	s := &Settings{}
	s.PinDefault("alice@x")
	s.SetOrg("alice@x", SafetyRecords, false)
	if !s.Orgs["alice@x"].Default {
		t.Errorf("Default cleared by SetOrg; got %+v", s.Orgs["alice@x"])
	}
	if s.Orgs["alice@x"].Safety != "records" {
		t.Errorf("Safety = %q", s.Orgs["alice@x"].Safety)
	}
}

func TestSetOrg_ClearPreservesDefaultPin(t *testing.T) {
	// Clearing safety should keep the pin, since the user may want
	// "pinned at startup, but default safety".
	s := &Settings{}
	s.PinDefault("alice@x")
	s.SetOrg("alice@x", SafetyRecords, false)
	s.SetOrg("alice@x", SafetyReadOnly, true) // clear=true
	cfg, ok := s.Orgs["alice@x"]
	if !ok {
		t.Fatal("entry dropped despite still having Default=true")
	}
	if cfg.Safety != "" {
		t.Errorf("Safety should be cleared, got %q", cfg.Safety)
	}
	if !cfg.Default {
		t.Errorf("Default should be preserved")
	}
}

func TestSetOrg_ClearDropsEntryWithNoDefault(t *testing.T) {
	// Same flow but without a pin: clear should remove the entry
	// entirely so TOML stays tidy.
	s := &Settings{}
	s.SetOrg("alice@x", SafetyRecords, false)
	s.SetOrg("alice@x", SafetyReadOnly, true)
	if _, ok := s.Orgs["alice@x"]; ok {
		t.Errorf("entry should have been dropped; got %+v", s.Orgs["alice@x"])
	}
}

func TestDefaultOrgUsername_NilSafe(t *testing.T) {
	var s *Settings
	if got := s.DefaultOrgUsername(); got != "" {
		t.Errorf("nil receiver returned %q", got)
	}
}
