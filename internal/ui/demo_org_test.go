package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func TestMergeDemoOrgs(t *testing.T) {
	real := demoOrgs()[:0] // empty real list is fine; use a fake real org
	real = append(real, sfOrgFake("real@acme.com"))

	// Not imported: unchanged.
	got := mergeDemoOrgs(real, false)
	if len(got) != 1 {
		t.Fatalf("not-imported: expected 1 org, got %d", len(got))
	}

	// Imported: demo orgs appended.
	got = mergeDemoOrgs(real, true)
	if len(got) != 1+len(demoOrgs()) {
		t.Fatalf("imported: expected %d orgs, got %d", 1+len(demoOrgs()), len(got))
	}

	// Idempotent: merging an already-merged list doesn't duplicate.
	got2 := mergeDemoOrgs(got, true)
	if len(got2) != len(got) {
		t.Fatalf("merge should be idempotent: %d -> %d", len(got), len(got2))
	}
}

func sfOrgFake(username string) sf.Org { return sf.Org{Username: username, Alias: username} }
