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
