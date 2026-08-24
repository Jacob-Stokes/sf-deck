package query

import "testing"

// TestToSOQLWhereCompareOps mirrors the eval test for ToSOQL — every
// op gets at least one expected emission.
func TestToSOQLWhereCompareOps(t *testing.T) {
	cases := []struct {
		name string
		node Node
		want string
	}{
		{"eq-string", Cmp("Status", OpEq, "Active"), "Status = 'Active'"},
		{"eq-int", Cmp("ApiVersion", OpEq, 60), "ApiVersion = 60"},
		{"eq-bool", Cmp("IsActive", OpEq, true), "IsActive = true"},
		{"ne", Cmp("Status", OpNotEq, "Draft"), "Status != 'Draft'"},
		{"contains", Cmp("Name", OpContains, "lead"), "Name LIKE '%lead%'"},
		{"starts", Cmp("DeveloperName", OpStartsWith, "FSL_"), "DeveloperName LIKE 'FSL_%'"},
		{"ends", Cmp("DeveloperName", OpEndsWith, "_c"), "DeveloperName LIKE '%_c'"},
		{"gt-int", Cmp("ApiVersion", OpGT, 50), "ApiVersion > 50"},
		{"gte-int", Cmp("ApiVersion", OpGTE, 60), "ApiVersion >= 60"},
		{"lt-int", Cmp("ApiVersion", OpLT, 70), "ApiVersion < 70"},
		{"lte-int", Cmp("ApiVersion", OpLTE, 60), "ApiVersion <= 60"},
		{"isnull", Cmp("ActiveVersionId", OpIsNull, nil), "ActiveVersionId = null"},
		{"in", Cmp("ProcessType", OpIn, []any{"Flow", "AutoLaunchedFlow"}),
			"ProcessType IN ('Flow', 'AutoLaunchedFlow')"},
		{"date-bare", Cmp("LastModifiedDate", OpGT, "2025-01-01"),
			"LastModifiedDate > 2025-01-01"},
		{"date-bare-tz", Cmp("LastModifiedDate", OpGT, "2025-01-01T00:00:00Z"),
			"LastModifiedDate > 2025-01-01T00:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ToSOQLWhere(tc.node)
			if got != tc.want {
				t.Fatalf("ToSOQLWhere mismatch\nwant %q\ngot  %q", tc.want, got)
			}
		})
	}
}

// TestToSOQLBooleanComposition makes sure precedence + parenthesisation
// stay correct across nested groups.
func TestToSOQLBooleanComposition(t *testing.T) {
	cases := []struct {
		name string
		node Node
		want string
	}{
		{"and-flat",
			And(
				Cmp("Status", OpEq, "Active"),
				Cmp("ProcessType", OpEq, "Flow"),
			),
			"Status = 'Active' AND ProcessType = 'Flow'",
		},
		{"or-flat",
			Or(
				Cmp("Status", OpEq, "Active"),
				Cmp("Status", OpEq, "Draft"),
			),
			"Status = 'Active' OR Status = 'Draft'",
		},
		{"or-inside-and-parenthesised",
			And(
				Cmp("Status", OpEq, "Active"),
				Or(
					Cmp("ProcessType", OpEq, "Flow"),
					Cmp("ProcessType", OpEq, "AutoLaunchedFlow"),
				),
			),
			"Status = 'Active' AND (ProcessType = 'Flow' OR ProcessType = 'AutoLaunchedFlow')",
		},
		{"not-wraps",
			Not(Cmp("DeveloperName", OpContains, "test")),
			"(NOT DeveloperName LIKE '%test%')",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ToSOQLWhere(tc.node)
			if got != tc.want {
				t.Fatalf("ToSOQLWhere mismatch\nwant %q\ngot  %q", tc.want, got)
			}
		})
	}
}

// TestToSOQLEscaping is the airtight-string-quoting check. Everything
// that goes through emitStringLiteral must round-trip safely against
// hostile inputs (apostrophes, percent signs already in the data).
func TestToSOQLEscaping(t *testing.T) {
	cases := []struct {
		name string
		node Node
		want string
	}{
		{"apostrophe-eq",
			Cmp("Name", OpEq, "O'Reilly"),
			"Name = 'O\\'Reilly'",
		},
		{"apostrophe-contains",
			Cmp("Name", OpContains, "O'Reilly"),
			"Name LIKE '%O\\'Reilly%'",
		},
		// Backslash must be escaped BEFORE the quote, or a trailing
		// backslash escapes the closing quote and the literal runs on
		// into the next clause (a break-out). See emitStringLiteral.
		{"trailing-backslash",
			Cmp("Name", OpEq, `abc\`),
			`Name = 'abc\\'`,
		},
		{"embedded-backslash-and-quote",
			Cmp("Name", OpEq, `a\b'c`),
			`Name = 'a\\b\'c'`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ToSOQLWhere(tc.node)
			if got != tc.want {
				t.Fatalf("escape mismatch\nwant %q\ngot  %q", tc.want, got)
			}
		})
	}
}

func TestToSOQLRejectsInjectedIdentifiersAndDateLiterals(t *testing.T) {
	if got := ToSOQL(Query{Columns: []string{"Id"}, Where: Cmp("Name OR Id", OpEq, "x")}, "Account"); got != "" {
		t.Fatalf("injected field emitted as SOQL: %q", got)
	}
	if got := ToSOQL(Query{Columns: []string{"Id"}}, "Account WHERE Name != null"); got != "" {
		t.Fatalf("injected FROM emitted as SOQL: %q", got)
	}
	if got := ToSOQLWhere(Cmp("CreatedDate", OpDateLiteral, "TODAY OR Name != null")); got != "" {
		t.Fatalf("injected raw date emitted as SOQL: %q", got)
	}
	if got := ToSOQL(Query{Where: Cmp("Name", Op("unknown"), "x")}, "Account"); got != "" {
		t.Fatalf("unknown operator broadened query: %q", got)
	}
	want := "CreatedDate = '2026-01-01T00:00:00Z OR Name != null'"
	if got := ToSOQLWhere(Cmp("CreatedDate", OpEq, "2026-01-01T00:00:00Z OR Name != null")); got != want {
		t.Fatalf("malformed date-like value was not quoted\nwant %q\ngot  %q", want, got)
	}
}

// TestToSOQLFull builds full SELECT statements end-to-end.
func TestToSOQLFull(t *testing.T) {
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
	want := "SELECT Id, DeveloperName, Status FROM Flow " +
		"WHERE Status = 'Active' AND ApiVersion >= 60 " +
		"ORDER BY LastModifiedDate DESC NULLS LAST, DeveloperName ASC " +
		"LIMIT 50"
	got := ToSOQL(q, "Flow")
	if got != want {
		t.Fatalf("ToSOQL mismatch\nwant %q\ngot  %q", want, got)
	}
}

// TestEvalAndToSOQLLockstep — for every test case in the eval suite,
// ToSOQL of the same expression should be a non-empty string. Doesn't
// validate semantic equivalence (we'd need Salesforce in the loop for
// that) but does catch "added an Op to Eval but not ToSOQL" drift.
func TestEvalAndToSOQLLockstep(t *testing.T) {
	cases := []Node{
		Cmp("Status", OpEq, "Active"),
		Cmp("ApiVersion", OpGTE, 60),
		Cmp("ProcessType", OpIn, []any{"Flow", "AutoLaunchedFlow"}),
		And(Cmp("a", OpEq, 1), Cmp("b", OpEq, 2)),
		Or(Cmp("a", OpEq, 1), Cmp("b", OpEq, 2)),
		Not(Cmp("a", OpContains, "x")),
	}
	for i, c := range cases {
		got := ToSOQLWhere(c)
		if got == "" {
			t.Fatalf("case %d: ToSOQLWhere returned empty for %#v", i, c)
		}
	}
}

func TestToSOQLClausesAndLiteralEdges(t *testing.T) {
	q := Query{
		Where:   Cmp("Active", OpEq, false),
		OrderBy: []OrderBy{{Field: "Name", Direction: Ascending, NullsLast: true}},
		Limit:   5,
	}
	if got, want := ToSOQLClauses(q), "WHERE Active = false ORDER BY Name ASC NULLS LAST LIMIT 5"; got != want {
		t.Fatalf("ToSOQLClauses() = %q, want %q", got, want)
	}
	if got := ToSOQLClauses(Query{OrderBy: []OrderBy{{Field: "Name", Direction: Descending}}}); got != "ORDER BY Name DESC" {
		t.Fatalf("descending clauses = %q", got)
	}
	if got := ToSOQLWhere(Cmp("Count__c", OpIn, nil)); got != "Count__c IN ('')" {
		t.Fatalf("empty IN = %q", got)
	}
	for value, want := range map[any]string{
		int64(7): "Count__c = 7",
		2.5:      "Count__c = 2.500000",
		nil:      "Count__c = null",
	} {
		if got := ToSOQLWhere(Cmp("Count__c", OpEq, value)); got != want {
			t.Errorf("literal %#v = %q, want %q", value, got, want)
		}
	}
	if got := ToSOQLWhere(And()); got != "Id != null" {
		t.Fatalf("empty AND = %q", got)
	}
	if got := ToSOQLWhere(Or()); got != "Id = null" {
		t.Fatalf("empty OR = %q", got)
	}
}

func TestSOQLValidationEdges(t *testing.T) {
	for _, name := range []string{"", ".Name", "Account.", "Account..Name", "1Name", "Name-Other"} {
		if validSOQLIdentifier(name) {
			t.Errorf("invalid identifier accepted: %q", name)
		}
	}
	for _, name := range []string{"Name", "Account.Owner.Name", "Field_2__c"} {
		if !validSOQLIdentifier(name) {
			t.Errorf("valid identifier rejected: %q", name)
		}
	}
	for _, value := range []string{"", ":1", "TODAY:", "TODAY:1:2", "TODAY:x", "TODAY-1"} {
		if validDateLiteral(value) {
			t.Errorf("invalid date literal accepted: %q", value)
		}
	}
	for _, value := range []string{"TODAY", "LAST_N_DAYS:30"} {
		if !validDateLiteral(value) {
			t.Errorf("valid date literal rejected: %q", value)
		}
	}
	invalidQueries := []Query{
		{Limit: -1},
		{OrderBy: []OrderBy{{Field: "Name DESC LIMIT 1"}}},
		{Columns: []string{"Id, Password__c"}},
		{Where: NotNode{}},
		{Where: AndNode{Children: []Node{Cmp("bad field", OpEq, 1)}}},
		{Where: OrNode{Children: []Node{Cmp("bad field", OpEq, 1)}}},
	}
	for i, q := range invalidQueries {
		if got := ToSOQL(q, "Account"); got != "" {
			t.Errorf("invalid query %d emitted %q", i, got)
		}
	}
}
