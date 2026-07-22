package migrate

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

func TestRunLegacyLenses(t *testing.T) {
	s := &settings.Settings{}
	s.SetLenses([]settings.LensConfig{
		{
			ID: "open-cases", Label: "Open Cases", Scope: "Case",
			SOQLWhere: "IsClosed = false",
			OrderBy:   "LastModifiedDate DESC",
			Limit:     50,
			Origin:    "user",
		},
	})
	n := Run(s)
	if n != 1 {
		t.Fatalf("expected 1 migration, got %d", n)
	}
	chips := s.Chips()
	if len(chips) != 1 {
		t.Fatalf("expected 1 chip, got %d", len(chips))
	}
	c := chips[0]
	if c.Domain != "records" || c.ID != "open-cases" {
		t.Fatalf("bad chip: %#v", c)
	}
	q := qchip.QueryFromConfig(c.Query)
	if q.Limit != 50 {
		t.Fatalf("limit not preserved: %#v", q)
	}
	if q.Where == nil {
		t.Fatal("WHERE lost in migration")
	}
}

func TestRunLegacyFlowFilters(t *testing.T) {
	s := &settings.Settings{}
	s.SetFlowFilters([]settings.FilterConfig{
		{
			ID: "active-screen", Label: "Active screen flows", Scope: "*",
			Origin: "user",
			Spec: settings.FilterSpecYAML{
				StatusEquals:   "Active",
				CategoryEquals: "Flow",
			},
		},
	})
	if Run(s) != 1 {
		t.Fatal("expected 1 migration")
	}
	chips := s.Chips()
	if len(chips) != 1 {
		t.Fatal("missing chip")
	}
	q := qchip.QueryFromConfig(chips[0].Query)
	// Eval against a representative row.
	row := mapRow{
		"Status":      "Active",
		"ProcessType": "Flow",
	}
	if !query.Eval(q.Where, row) {
		t.Fatalf("expected migrated chip to match active screen flow row\nAST: %#v", q.Where)
	}
	row["Status"] = "Draft"
	if query.Eval(q.Where, row) {
		t.Fatal("expected draft flow to fail the migrated predicate")
	}
}

func TestRunIdempotent(t *testing.T) {
	s := &settings.Settings{}
	s.SetObjectFilters([]settings.FilterConfig{
		{ID: "custom", Label: "Custom", Scope: "*", Spec: settings.FilterSpecYAML{IsCustom: ptrBool(true)}},
	})
	Run(s)
	first := len(s.Chips())
	Run(s) // re-run; the "objects" domain is already populated
	if len(s.Chips()) != first {
		t.Fatalf("re-running migration should be a no-op once domain is populated")
	}
}

// mapRow is a minimal query.Row impl for tests that don't want to drag
// in the sf package types.
type mapRow map[string]any

func (m mapRow) Field(name string) (any, bool) {
	v, ok := m[name]
	return v, ok
}

func ptrBool(b bool) *bool { return &b }
