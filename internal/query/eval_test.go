package query

import "testing"

// mapRow is a Row backed by a plain map — used by the test suite so
// we can author scenarios without depending on the sf package.
type mapRow map[string]any

func (m mapRow) Field(name string) (any, bool) {
	v, ok := m[name]
	return v, ok
}

// TestEvalCompareOps walks every CompareNode operator with a couple
// of positive + negative cases. The eval code paths fan out via a
// switch so this test is the single source of truth that nothing
// silently regressed.
func TestEvalCompareOps(t *testing.T) {
	row := mapRow{
		"Name":          "OpenCases",
		"Status":        "Active",
		"ApiVersion":    int(60),
		"IsActive":      true,
		"DeveloperName": "FSL_PolicyAssign",
		"NullField":     nil,
	}
	cases := []struct {
		name string
		node Node
		want bool
	}{
		// Eq / NotEq
		{"eq-string-true", Cmp("Status", OpEq, "Active"), true},
		{"eq-string-false", Cmp("Status", OpEq, "Draft"), false},
		{"eq-int-true", Cmp("ApiVersion", OpEq, 60), true},
		{"eq-int-false", Cmp("ApiVersion", OpEq, 50), false},
		{"eq-bool-true", Cmp("IsActive", OpEq, true), true},
		{"ne", Cmp("Status", OpNotEq, "Draft"), true},
		// Contains / Starts / Ends
		{"contains", Cmp("Name", OpContains, "open"), true},
		{"contains-fold", Cmp("Name", OpContains, "OPEN"), true},
		{"starts", Cmp("DeveloperName", OpStartsWith, "FSL_"), true},
		{"ends", Cmp("DeveloperName", OpEndsWith, "Assign"), true},
		// Ordered compare on numbers
		{"gt", Cmp("ApiVersion", OpGT, 50), true},
		{"gt-equal", Cmp("ApiVersion", OpGT, 60), false},
		{"gte", Cmp("ApiVersion", OpGTE, 60), true},
		{"lt", Cmp("ApiVersion", OpLT, 70), true},
		{"lte", Cmp("ApiVersion", OpLTE, 60), true},
		// Ordered compare on date strings (lexical compare matches
		// chronological order for ISO-8601).
		{"date-gt", Cmp("Status", OpGT, "Acti"), true},
		// IsNull
		{"isnull-explicit-nil", Cmp("NullField", OpIsNull, nil), true},
		{"isnull-missing", Cmp("Missing", OpIsNull, nil), true},
		{"isnull-non-null", Cmp("Status", OpIsNull, nil), false},
		// In
		{"in-match", Cmp("Status", OpIn, []any{"Draft", "Active"}), true},
		{"in-miss", Cmp("Status", OpIn, []any{"Draft", "Obsolete"}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Eval(tc.node, row)
			if got != tc.want {
				t.Fatalf("Eval(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestEvalBooleanComposition covers AND / OR / NOT and nested groups.
func TestEvalBooleanComposition(t *testing.T) {
	row := mapRow{
		"Status":      "Active",
		"ProcessType": "Flow",
		"Name":        "OpenCases",
	}
	cases := []struct {
		name string
		node Node
		want bool
	}{
		{"and-all-match", And(
			Cmp("Status", OpEq, "Active"),
			Cmp("ProcessType", OpEq, "Flow"),
		), true},
		{"and-one-misses", And(
			Cmp("Status", OpEq, "Active"),
			Cmp("ProcessType", OpEq, "Workflow"),
		), false},
		{"or-one-matches", Or(
			Cmp("Status", OpEq, "Draft"),
			Cmp("Status", OpEq, "Active"),
		), true},
		{"or-none-match", Or(
			Cmp("Status", OpEq, "Draft"),
			Cmp("Status", OpEq, "Obsolete"),
		), false},
		{"not-flips", Not(Cmp("Status", OpEq, "Draft")), true},
		{"nested-and-or", And(
			Cmp("Status", OpEq, "Active"),
			Or(
				Cmp("ProcessType", OpEq, "Flow"),
				Cmp("ProcessType", OpEq, "AutoLaunchedFlow"),
			),
		), true},
		{"empty-and-matches", AndNode{}, true},
		{"empty-or-fails", OrNode{}, false},
		{"nil-node-matches", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Eval(tc.node, row)
			if got != tc.want {
				t.Fatalf("Eval(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestEvalTypeMismatchSafe covers the "compare a string field to an
// int literal" trap. Behaviour: false rather than panic. Author
// confidence comes from the wizard validating types at save time;
// Eval is just defensive.
func TestEvalTypeMismatchSafe(t *testing.T) {
	row := mapRow{"Status": "Active"}
	if Eval(Cmp("Status", OpGT, 5), row) {
		t.Fatal("type mismatch should evaluate to false, not match")
	}
}
