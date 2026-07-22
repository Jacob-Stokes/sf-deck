package sf

import "testing"

func TestCommunityRowFieldTargetsYank(t *testing.T) {
	r := CommunityRow{
		ID: "0DB1", Name: "Summer School", URLPathPrefix: "summer",
		Status: "Live", Members: 111, SelfReg: true,
	}

	for _, n := range []string{"Name", "Status", "Members", "SelfReg", "UrlPathPrefix"} {
		if _, ok := r.Field(n); !ok {
			t.Errorf("Field(%q) not resolvable", n)
		}
	}

	// o menu leads with the live site when a URL prefix is set.
	ts := r.Targets()
	if len(ts) == 0 || ts[0].ID != "live" {
		t.Errorf("Targets should lead with the live site, got %+v", ts)
	}
	// A community with no URL prefix falls back to All Sites (Setup).
	bare := CommunityRow{ID: "0DB2", Name: "Default"}.Targets()
	if bare[0].ID != "allsites" {
		t.Errorf("no-prefix community should lead with All Sites, got %+v", bare)
	}

	// Yank exposes name + url + id.
	ys := r.YankTargets()
	if len(ys) != 3 || ys[0].Value != "Summer School" {
		t.Fatalf("yank targets = %+v, want name-first (3)", ys)
	}
}
