package settings

import "testing"

func TestWelcomeSeenRoundTrip(t *testing.T) {
	var nilS *Settings
	if !nilS.WelcomeSeen() {
		t.Error("nil Settings should report WelcomeSeen=true (never nag on degraded start)")
	}
	s := &Settings{}
	if s.WelcomeSeen() {
		t.Error("fresh Settings should report WelcomeSeen=false")
	}
	s.SetWelcomeSeen(true)
	if !s.WelcomeSeen() {
		t.Error("SetWelcomeSeen(true) not reflected")
	}
	if s.DemoOrgImported() {
		t.Error("fresh Settings should report DemoOrgImported=false")
	}
	s.SetDemoOrgImported(true)
	if !s.DemoOrgImported() {
		t.Error("SetDemoOrgImported(true) not reflected")
	}
}
