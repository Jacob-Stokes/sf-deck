package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/query"
)

// ---- splitForWizard ---------------------------------------------------

func TestSplitForWizardFlatPredicates(t *testing.T) {
	cases := []struct {
		name     string
		node     query.Node
		wantAdv  bool
		wantCmps int
	}{
		{"nil", nil, false, 0},
		{"single-compare", query.Cmp("a", query.OpEq, 1), false, 1},
		{"flat-and", query.And(
			query.Cmp("a", query.OpEq, 1),
			query.Cmp("b", query.OpContains, "x"),
		), false, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adv, cmps, reason := splitForWizard(tc.node)
			if adv != tc.wantAdv {
				t.Fatalf("advanced flag: got %v want %v (reason %q)", adv, tc.wantAdv, reason)
			}
			if len(cmps) != tc.wantCmps {
				t.Fatalf("compare count: got %d want %d", len(cmps), tc.wantCmps)
			}
			if !adv && reason != "" {
				t.Fatalf("flat predicate shouldn't carry a reason, got %q", reason)
			}
		})
	}
}

func TestSplitForWizardRichShapesNeedAdvanced(t *testing.T) {
	cases := []struct {
		name       string
		node       query.Node
		wantReason string
	}{
		{"or", query.Or(
			query.Cmp("a", query.OpEq, 1),
			query.Cmp("b", query.OpEq, 2),
		), "uses OR"},
		{"not", query.Not(query.Cmp("a", query.OpEq, 1)), "uses NOT"},
		{"and-with-or-child", query.And(
			query.Cmp("a", query.OpEq, 1),
			query.Or(
				query.Cmp("b", query.OpEq, 2),
				query.Cmp("c", query.OpEq, 3),
			),
		), "uses OR"},
		{"and-with-not-child", query.And(
			query.Cmp("a", query.OpEq, 1),
			query.Not(query.Cmp("b", query.OpEq, 2)),
		), "uses NOT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adv, cmps, reason := splitForWizard(tc.node)
			if !adv {
				t.Fatalf("expected advanced=true for %s", tc.name)
			}
			if reason != tc.wantReason {
				t.Fatalf("reason: got %q want %q", reason, tc.wantReason)
			}
			if len(cmps) != 0 {
				t.Fatalf("advanced result should return no compares, got %d", len(cmps))
			}
		})
	}
}

// ---- hasMeaningfulWhere ----------------------------------------------

func TestHasMeaningfulWhere(t *testing.T) {
	cases := []struct {
		name string
		node query.Node
		want bool
	}{
		{"nil", nil, false},
		{"empty-and", query.AndNode{}, false},
		{"empty-or", query.OrNode{}, false},
		{"single-compare", query.Cmp("a", query.OpEq, 1), true},
		{"non-empty-and", query.And(query.Cmp("a", query.OpEq, 1)), true},
		{"not-of-cmp", query.Not(query.Cmp("a", query.OpEq, 1)), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasMeaningfulWhere(tc.node); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

// ---- populateFromCompareNodes ---------------------------------------

func TestPopulateFromCompareNodesByFieldAndOp(t *testing.T) {
	// Catalogue rows: a string Name field on Contains, and a tristate
	// IsCustom on Eq. Same Field name on different Ops should match
	// independently, so we add two Name rows with different Ops to
	// stress the (Field, Op) keying.
	fields := []cwField{
		{Field: "Name", Op: query.OpContains, Kind: cwText, input: newWizardInput("")},
		{Field: "Name", Op: query.OpStartsWith, Kind: cwText, input: newWizardInput("")},
		{Field: "IsCustom", Op: query.OpEq, Kind: cwTri},
	}
	cmps := []query.CompareNode{
		{Field: "Name", Op: query.OpContains, Value: "lead"},
		{Field: "IsCustom", Op: query.OpEq, Value: true},
	}
	populateFromCompareNodes(fields, cmps)
	if got := fields[0].input.Value(); got != "lead" {
		t.Fatalf("Name contains row: got %q want %q", got, "lead")
	}
	if got := fields[1].input.Value(); got != "" {
		t.Fatalf("Name startsWith row should stay empty, got %q", got)
	}
	if fields[2].triValue == nil || !*fields[2].triValue {
		t.Fatalf("IsCustom tristate: got %v want true", fields[2].triValue)
	}
}

func TestPopulateFromCompareNodesIgnoresUnknown(t *testing.T) {
	fields := []cwField{
		{Field: "Name", Op: query.OpContains, Kind: cwText, input: newWizardInput("")},
	}
	cmps := []query.CompareNode{
		{Field: "Mystery", Op: query.OpEq, Value: "x"},
	}
	populateFromCompareNodes(fields, cmps)
	if got := fields[0].input.Value(); got != "" {
		t.Fatalf("unknown field shouldn't populate any row: got %q", got)
	}
}

// ---- buildSimpleQuery + round-trip ----------------------------------

func TestBuildSimpleQueryFromFilledRows(t *testing.T) {
	tri := true
	fields := []cwField{
		{Field: "Name", Op: query.OpContains, Kind: cwText, input: newWizardInput("acc")},
		{Field: "ApiVersion", Op: query.OpGTE, Kind: cwInt, input: newWizardInput("60")},
		{Field: "IsCustom", Op: query.OpEq, Kind: cwTri, triValue: &tri},
		// Empty rows should drop out.
		{Field: "Status", Op: query.OpEq, Kind: cwText, input: newWizardInput("")},
	}
	q := buildSimpleQuery(fields)
	and, ok := q.Where.(query.AndNode)
	if !ok {
		t.Fatalf("expected AndNode at top, got %T", q.Where)
	}
	if len(and.Children) != 3 {
		t.Fatalf("expected 3 non-empty rows in AND, got %d (%#v)", len(and.Children), and.Children)
	}
}

func TestSimpleRoundTripPreservesPredicate(t *testing.T) {
	// Build → split → repopulate → rebuild. The two builds should
	// produce equivalent ASTs.
	tri := true
	original := []cwField{
		{Field: "Name", Op: query.OpContains, Kind: cwText, input: newWizardInput("lead")},
		{Field: "ApiVersion", Op: query.OpGTE, Kind: cwInt, input: newWizardInput("60")},
		{Field: "IsCustom", Op: query.OpEq, Kind: cwTri, triValue: &tri},
	}
	q1 := buildSimpleQuery(original)

	advanced, cmps, _ := splitForWizard(q1.Where)
	if advanced {
		t.Fatal("flat AND should not require advanced mode")
	}

	repopulated := []cwField{
		{Field: "Name", Op: query.OpContains, Kind: cwText, input: newWizardInput("")},
		{Field: "ApiVersion", Op: query.OpGTE, Kind: cwInt, input: newWizardInput("")},
		{Field: "IsCustom", Op: query.OpEq, Kind: cwTri},
	}
	populateFromCompareNodes(repopulated, cmps)

	q2 := buildSimpleQuery(repopulated)

	// Compare emitted SOQL — easier than DeepEqual on the AST since
	// AndNode children may reorder during conversion.
	got := query.ToSOQLWhere(q2.Where)
	want := query.ToSOQLWhere(q1.Where)
	if got != want {
		t.Fatalf("round-trip mismatch\nwant %q\ngot  %q", want, got)
	}
}

// TestChipWizardTextInputFocused pins which focus states count as a
// text buffer — the gate that stops capital-S (and other single-letter
// wizard shortcuts) from hijacking a literal keystroke while typing a
// view name / SOQL. Regression for "typing S in the view editor opened
// the scope chooser".
func TestChipWizardTextInputFocused(t *testing.T) {
	manual := true
	notManual := false

	cases := []struct {
		name string
		st   chipWizardState
		want bool
	}{
		{"label focused", chipWizardState{Cursor: -1}, true},
		{"advanced SOQL editor", chipWizardState{Cursor: 0, Advanced: true, criteria: []cwField{{Kind: cwText}}}, true},
		{"text criterion", chipWizardState{Cursor: 0, criteria: []cwField{{Kind: cwText}}}, true},
		{"int criterion", chipWizardState{Cursor: 0, criteria: []cwField{{Kind: cwInt}}}, true},
		{"date criterion", chipWizardState{Cursor: 0, criteria: []cwField{{Kind: cwDate}}}, true},
		{"limit manual", chipWizardState{Cursor: 0, criteria: []cwField{{Kind: cwLimit, triValue: &manual}}}, true},
		{"limit auto", chipWizardState{Cursor: 0, criteria: []cwField{{Kind: cwLimit, triValue: &notManual}}}, false},
		{"tristate toggle", chipWizardState{Cursor: 0, criteria: []cwField{{Kind: cwTri}}}, false},
		{"add-criterion affordance", chipWizardState{Cursor: 0, criteria: nil}, false},
	}
	for _, c := range cases {
		st := c.st
		if got := st.textInputFocused(); got != c.want {
			t.Errorf("%s: textInputFocused() = %v, want %v", c.name, got, c.want)
		}
	}
}
