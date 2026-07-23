package buildinfo

import "testing"

func TestInfoDisplayHelpers(t *testing.T) {
	t.Cleanup(func() { Set("dev", "none", "unknown") })

	Set("0.2.1", "0123456789abcdef", "2026-07-23T12:00:00Z")
	got := Current()
	if got.DisplayVersion() != "v0.2.1" {
		t.Fatalf("DisplayVersion = %q", got.DisplayVersion())
	}
	if got.ShortCommit() != "0123456789ab" {
		t.Fatalf("ShortCommit = %q", got.ShortCommit())
	}
	if got.IsDevelopment() {
		t.Fatal("release build reported as development")
	}

	Set("dev", "none", "unknown")
	if !Current().IsDevelopment() {
		t.Fatal("dev build not recognised")
	}
}
