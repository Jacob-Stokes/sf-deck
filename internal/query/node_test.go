package query

import (
	"reflect"
	"testing"
)

// TestRoundTrip exercises the YAMLFrom/From conversion across every
// node variant. If you add a new Op or Node kind and forget to wire it
// into the persistence layer, this test catches it before settings.toml
// silently corrupts on a write.
func TestRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		node Node
	}{
		{"compare-eq", Cmp("Status", OpEq, "Active")},
		{"compare-contains", Cmp("MasterLabel", OpContains, "lead")},
		{"compare-int", Cmp("ApiVersion", OpGTE, 60)},
		{"compare-bool", Cmp("IsActive", OpEq, true)},
		{"compare-isnull", Cmp("ActiveVersionId", OpIsNull, nil)},
		{"compare-in", Cmp("ProcessType", OpIn, []any{"Flow", "AutoLaunchedFlow"})},
		{"and", And(
			Cmp("Status", OpEq, "Active"),
			Cmp("ProcessType", OpEq, "Flow"),
		)},
		{"or", Or(
			Cmp("Status", OpEq, "Active"),
			Cmp("Status", OpEq, "Draft"),
		)},
		{"not", Not(Cmp("DeveloperName", OpStartsWith, "FSL_"))},
		{"nested", And(
			Cmp("Status", OpEq, "Active"),
			Or(
				Cmp("ProcessType", OpEq, "Flow"),
				Cmp("ProcessType", OpEq, "AutoLaunchedFlow"),
			),
			Not(Cmp("DeveloperName", OpContains, "test")),
		)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			y := YAMLFromNode(tc.node)
			got := NodeFromYAML(y)
			if !reflect.DeepEqual(tc.node, got) {
				t.Fatalf("round-trip mismatch\nwant %#v\ngot  %#v", tc.node, got)
			}
		})
	}
}

// TestQueryRoundTrip extends the round-trip to Query (the top-level
// type carrying ORDER BY + LIMIT + Columns).
func TestQueryRoundTrip(t *testing.T) {
	q := Query{
		Where: And(
			Cmp("Status", OpEq, "Active"),
			Cmp("ApiVersion", OpGTE, 60),
		),
		OrderBy: []OrderBy{
			{Field: "LastModifiedDate", Direction: Descending, NullsLast: true},
			{Field: "DeveloperName", Direction: Ascending},
		},
		Limit:   50,
		Columns: []string{"Id", "DeveloperName", "Status"},
	}
	y := YAMLFromQuery(q)
	got := QueryFromYAML(y)
	if !reflect.DeepEqual(q, got) {
		t.Fatalf("query round-trip mismatch\nwant %#v\ngot  %#v", q, got)
	}
}

// TestEmptyDefaults makes sure a zero Query stays zero through the
// round-trip — IsEmpty must remain true so callers can rely on it as
// the "match everything" sentinel.
func TestEmptyDefaults(t *testing.T) {
	q := Query{}
	if !q.IsEmpty() {
		t.Fatal("expected zero Query to be empty")
	}
	y := YAMLFromQuery(q)
	got := QueryFromYAML(y)
	if !got.IsEmpty() {
		t.Fatal("expected round-tripped zero Query to remain empty")
	}
}

// TestAndOrFlatten checks the helper-side flattening so the resulting
// tree is shallow regardless of how it's constructed.
func TestAndOrFlatten(t *testing.T) {
	a := Cmp("a", OpEq, 1)
	b := Cmp("b", OpEq, 2)
	c := Cmp("c", OpEq, 3)
	got := And(a, And(b, c))
	want := AndNode{Children: []Node{a, b, c}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("flatten mismatch\nwant %#v\ngot  %#v", want, got)
	}
}

// TestNotDoubleNegationCollapses verifies Not(Not(x)) → x.
func TestNotDoubleNegationCollapses(t *testing.T) {
	x := Cmp("a", OpEq, 1)
	got := Not(Not(x))
	if !reflect.DeepEqual(got, x) {
		t.Fatalf("Not(Not) should collapse\nwant %#v\ngot  %#v", x, got)
	}
}

// TestUnknownKindDecaysToAnd — invalid persisted data must NOT crash;
// it converts to an empty And so the rest of the config can still load.
func TestUnknownKindDecaysToAnd(t *testing.T) {
	got := NodeFromYAML(NodeYAML{Kind: "garbage"})
	want := AndNode{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unknown kind should decay to empty And\nwant %#v\ngot  %#v", want, got)
	}
}
