package settings

import "testing"

func TestDebugForceWelcome(t *testing.T) {
	s := &Settings{}
	if s.DebugForceWelcome() {
		t.Error("default should be false")
	}
	s.SetDebugForceWelcome(true)
	if !s.DebugForceWelcome() {
		t.Error("SetDebugForceWelcome(true) not reflected")
	}
	// nil-safe
	var n *Settings
	if n.DebugForceWelcome() {
		t.Error("nil should be false")
	}
}
