package query

import (
	"reflect"
	"testing"
	"time"
)

// TestParseRoundTripWithToSOQL is the headline test: every shape we
// claim to support should parse → AST → ToSOQL back to a SOQL string
// that's semantically equivalent. We don't require byte-identical
// output (the parser canonicalises whitespace etc.) but we do require
// a re-parse of the emitted SOQL to yield the same AST.
func TestParseRoundTripWithToSOQL(t *testing.T) {
	cases := []struct {
		name string
		soql string
	}{
		{"simple-eq", "SELECT Id FROM Flow WHERE Status = 'Active'"},
		{"and", "SELECT Id FROM Flow WHERE Status = 'Active' AND ProcessType = 'Flow'"},
		{"or", "SELECT Id FROM Flow WHERE Status = 'Active' OR Status = 'Draft'"},
		{"nested-or-in-and",
			"SELECT Id FROM Flow WHERE Status = 'Active' AND " +
				"(ProcessType = 'Flow' OR ProcessType = 'AutoLaunchedFlow')"},
		{"like-contains", "SELECT Id FROM Flow WHERE DeveloperName LIKE '%lead%'"},
		{"like-starts", "SELECT Id FROM Flow WHERE DeveloperName LIKE 'FSL_%'"},
		{"like-ends", "SELECT Id FROM Flow WHERE DeveloperName LIKE '%__c'"},
		{"in", "SELECT Id FROM Flow WHERE ProcessType IN ('Flow', 'AutoLaunchedFlow')"},
		{"is-null", "SELECT Id FROM Flow WHERE ActiveVersionId = null"},
		{"is-not-null", "SELECT Id FROM Flow WHERE (NOT ActiveVersionId = null)"},
		{"date", "SELECT Id FROM Flow WHERE LastModifiedDate > 2025-01-01T00:00:00Z"},
		{"order-limit",
			"SELECT Id, Name FROM Account WHERE Industry = 'Tech' ORDER BY Name ASC LIMIT 50"},
		{"escaped-quote",
			"SELECT Id FROM Account WHERE Name = 'O\\'Reilly'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q1, from1, err := Parse(tc.soql)
			if err != nil {
				t.Fatalf("first parse failed: %v", err)
			}
			emitted := ToSOQL(q1, from1)
			q2, from2, err := Parse(emitted)
			if err != nil {
				t.Fatalf("re-parse failed for %q: %v", emitted, err)
			}
			if from1 != from2 {
				t.Fatalf("FROM mismatch: %q vs %q", from1, from2)
			}
			if !reflect.DeepEqual(q1, q2) {
				t.Fatalf("AST drift\nfirst: %#v\nemitted: %s\nsecond: %#v", q1, emitted, q2)
			}
		})
	}
}

// TestParseEvaluatesCorrectly drives the parser → AST → Eval pipeline
// against representative rows. This is what proves the imported chip
// behaves consistently with what the SOQL would have returned server-side.
func TestParseEvaluatesCorrectly(t *testing.T) {
	type tc struct {
		name string
		soql string
		row  mapRow
		want bool
	}
	cases := []tc{
		{"active-flow",
			"SELECT Id FROM Flow WHERE Status = 'Active'",
			mapRow{"Status": "Active"},
			true,
		},
		{"draft-not-active",
			"SELECT Id FROM Flow WHERE Status = 'Active'",
			mapRow{"Status": "Draft"},
			false,
		},
		{"and",
			"SELECT Id FROM Flow WHERE Status = 'Active' AND ApiVersion >= 60",
			mapRow{"Status": "Active", "ApiVersion": int(60)},
			true,
		},
		{"and-fails",
			"SELECT Id FROM Flow WHERE Status = 'Active' AND ApiVersion >= 60",
			mapRow{"Status": "Active", "ApiVersion": int(50)},
			false,
		},
		{"in-list",
			"SELECT Id FROM Flow WHERE ProcessType IN ('Flow', 'AutoLaunchedFlow')",
			mapRow{"ProcessType": "Flow"},
			true,
		},
		{"like-prefix",
			"SELECT Id FROM Flow WHERE DeveloperName LIKE 'FSL_%'",
			mapRow{"DeveloperName": "FSL_PolicyAssign"},
			true,
		},
		{"like-prefix-miss",
			"SELECT Id FROM Flow WHERE DeveloperName LIKE 'FSL_%'",
			mapRow{"DeveloperName": "OpenCases"},
			false,
		},
		{"is-null",
			"SELECT Id FROM Flow WHERE ActiveVersionId = null",
			mapRow{"ActiveVersionId": ""},
			true,
		},
		{"is-not-null-via-not",
			"SELECT Id FROM Flow WHERE NOT ActiveVersionId = null",
			mapRow{"ActiveVersionId": "30150000000Abcd"},
			true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q, _, err := Parse(c.soql)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := Eval(q.Where, c.row)
			if got != c.want {
				t.Fatalf("Eval(%q) = %v, want %v\nAST: %#v", c.soql, got, c.want, q.Where)
			}
		})
	}
}

// TestParseOrderByVariants checks the ORDER BY parser's edge cases:
// multi-key, ASC/DESC mix, NULLS LAST.
func TestParseOrderByVariants(t *testing.T) {
	q, _, err := Parse(
		"SELECT Id FROM Flow ORDER BY LastModifiedDate DESC NULLS LAST, DeveloperName ASC")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []OrderBy{
		{Field: "LastModifiedDate", Direction: Descending, NullsLast: true},
		{Field: "DeveloperName", Direction: Ascending},
	}
	if !reflect.DeepEqual(q.OrderBy, want) {
		t.Fatalf("ORDER BY mismatch\nwant %#v\ngot  %#v", want, q.OrderBy)
	}
}

// TestParseRejectsUnsupportedClauses guards against silent acceptance
// of GROUP BY / OFFSET / HAVING — the import flow should warn the user.
func TestParseRejectsUnsupportedClauses(t *testing.T) {
	cases := []string{
		"SELECT Id FROM Account GROUP BY Industry",
		"SELECT Id FROM Account HAVING COUNT(Id) > 5",
		"SELECT Id FROM Account ORDER BY Name OFFSET 10",
	}
	for _, soql := range cases {
		_, _, err := Parse(soql)
		if err == nil {
			t.Errorf("expected error parsing %q, got nil", soql)
		}
	}
}

// TestTokeniseUnterminatedQuoteNoPanic locks in the must-fix from the
// pre-release audit: an unterminated single-quoted literal made the
// tokeniser slice one past end-of-string and panic. Parse runs
// synchronously in the TUI's Update loop on raw user input (e.g. typing
// `WHERE Name = 'foo` in the chip wizard), where only View() is
// recover()-guarded — so a tokeniser panic crashed the whole program.
// These must return (possibly with a parse error) but never panic.
func TestTokeniseUnterminatedQuoteNoPanic(t *testing.T) {
	cases := []string{
		"SELECT Id FROM Account WHERE Name = 'foo",
		"SELECT Id FROM Account WHERE Name = '",
		"SELECT Id FROM Account WHERE Name = 'a\\'",
		"SELECT Id FROM Account WHERE Name = 'unterminated AND X = 1",
	}
	for _, soql := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Parse(%q) panicked: %v", soql, r)
				}
			}()
			_, _, _ = Parse(soql) // error is fine; a panic is not
		}()
	}
}

// TestTokeniseNoInfiniteLoopOnUnknownByte locks in the must-fix from
// audit 7: the tokeniser's default branch only advanced over
// [A-Za-z0-9_.], so any other byte (a negative literal `-5`, `*`, `&`,
// etc.) left the index unmoved and spun forever, wedging the
// single-threaded Update loop. Each case must return promptly (the
// timeout guard fails the test instead of hanging the whole suite).
func TestTokeniseNoInfiniteLoopOnUnknownByte(t *testing.T) {
	cases := []string{
		"SELECT Id FROM Account WHERE Amount > -5",
		"SELECT Id FROM Account WHERE Lat > -90.5",
		"SELECT Id FROM Account WHERE X = a*b",
		"SELECT Id FROM Account WHERE A & B",
		"SELECT Id FROM Account WHERE A | B",
		"SELECT Id FROM Account WHERE x @ y",
		"SELECT Id FROM Account WHERE a/b = 1",
	}
	for _, soql := range cases {
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _, _ = Parse(soql) // error is fine; a hang is not
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("Parse(%q) did not return within 2s — tokeniser likely looping", soql)
		}
	}
}
