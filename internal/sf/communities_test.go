package sf

import "testing"

func TestCommunityRowFieldTargetsYank(t *testing.T) {
	r := CommunityRow{
		ID: "0DB1", Name: "Northwind Partner Portal", URLPathPrefix: "partners",
		Status: "Live", Members: 111, SelfReg: true,
	}

	for _, n := range []string{"Name", "Status", "Members", "SelfReg", "UrlPathPrefix"} {
		if _, ok := r.Field(n); !ok {
			t.Errorf("Field(%q) not resolvable", n)
		}
	}

	ts := r.Targets()
	if len(ts) == 0 || ts[0].ID != "live" {
		t.Errorf("Targets should lead with the live site, got %+v", ts)
	}
	bare := CommunityRow{ID: "0DB2", Name: "Default"}.Targets()
	if bare[0].ID != "allsites" {
		t.Errorf("no-prefix community should lead with All Sites, got %+v", bare)
	}

	ys := r.YankTargets()
	if len(ys) != 3 || ys[0].Value != "Northwind Partner Portal" {
		t.Fatalf("yank targets = %+v, want name-first (3)", ys)
	}
}
