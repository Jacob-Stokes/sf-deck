package settings

import "testing"

func TestAutomaticUpdateChecksDefaultsOnAndCanBeDisabled(t *testing.T) {
	t.Setenv("SF_DECK_NO_UPDATE_CHECK", "")
	s := &Settings{}
	if !s.AutomaticUpdateChecks() {
		t.Fatal("automatic update checks should default on")
	}
	s.SetAutomaticUpdateChecks(false)
	if s.AutomaticUpdateChecks() {
		t.Fatal("explicit false ignored")
	}
	s.SetAutomaticUpdateChecks(true)
	if !s.AutomaticUpdateChecks() {
		t.Fatal("explicit true ignored")
	}
}

func TestAutomaticUpdateChecksEnvironmentOverride(t *testing.T) {
	t.Setenv("SF_DECK_NO_UPDATE_CHECK", "true")
	s := &Settings{}
	s.SetAutomaticUpdateChecks(true)
	if s.AutomaticUpdateChecks() {
		t.Fatal("environment override should disable automatic checks")
	}
	if !UpdateChecksDisabledByEnv() {
		t.Fatal("environment override not reported")
	}
}
