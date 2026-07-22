package sf

import "testing"

func TestSetupAuditRowFieldAndYank(t *testing.T) {
	r := SetupAuditRow{
		ID:          "0Ym1",
		Action:      "PermSetGroupFlsChanged",
		Section:     "Permission Set Group",
		Display:     "Changed FLS for X",
		CreatedByID: "005xx",
		CreatedBy:   "Gar Mun Hui",
	}

	// Field() feeds the generic sort/search engine — the columns the
	// audit surface renders must resolve.
	for _, name := range []string{"Action", "Section", "Display", "CreatedBy.Name", "CreatedDate"} {
		if _, ok := r.Field(name); !ok {
			t.Errorf("Field(%q) not resolvable", name)
		}
	}

	// The change sentence is the primary yank (paste into a ticket).
	ys := r.YankTargets()
	if len(ys) == 0 || ys[0].Value != "Changed FLS for X" {
		t.Fatalf("first yank should be the change description, got %+v", ys)
	}

	// o opens the actor's user record first when the actor Id is known.
	ts := r.Targets()
	if len(ts) == 0 || ts[0].ID != "user" {
		t.Errorf("Targets should lead with the actor user link when CreatedById is set, got %+v", ts)
	}

	// With no actor Id, o falls back to the audit-trail Setup page only.
	bare := SetupAuditRow{ID: "0Ym2", Display: "x"}.Targets()
	if len(bare) != 1 || bare[0].ID != "audit" {
		t.Errorf("no-actor row should offer only the audit Setup target, got %+v", bare)
	}
}
