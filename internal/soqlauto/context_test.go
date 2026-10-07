package soqlauto

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		cursor    int // -1 means end of string
		wantCtx   ContextKind
		wantTok   string
		wantPath  string
		wantSobj  string
		wantSub   bool
		wantField string // for WhereValue / InWithValues
	}{
		{
			name:    "empty",
			query:   "",
			cursor:  -1,
			wantCtx: ContextTopLevel,
		},
		{
			name:    "after-select",
			query:   "SELECT ",
			cursor:  -1,
			wantCtx: ContextAfterSelectKeyword,
		},
		{
			name:    "after-from-empty",
			query:   "SELECT Id FROM ",
			cursor:  -1,
			wantCtx: ContextAfterFromKeyword,
		},
		{
			name:     "after-from-with-name",
			query:    "SELECT Id FROM Account",
			cursor:   -1,
			wantCtx:  ContextAfterFromKeyword, // typing the sObject name
			wantTok:  "Account",
			wantSobj: "Account",
		},
		{
			name:     "where-field-slot",
			query:    "SELECT Id FROM Account WHERE ",
			cursor:   -1,
			wantCtx:  ContextWhereField,
			wantSobj: "Account",
		},
		{
			name:      "where-value-empty-after-eq",
			query:     "SELECT Id FROM Account WHERE Name = ",
			cursor:    -1,
			wantCtx:   ContextWhereValue,
			wantSobj:  "Account",
			wantField: "Name",
		},
		{
			name:      "where-value-typing-quoted",
			query:     "SELECT Id FROM Account WHERE Name = 'A",
			cursor:    -1,
			wantCtx:   ContextWhereValue,
			wantSobj:  "Account",
			wantField: "Name",
			wantTok:   "A",
		},
		{
			name:      "where-value-relationship-path",
			query:     "SELECT Id FROM Contact WHERE Account.Name = '",
			cursor:    -1,
			wantCtx:   ContextWhereValue,
			wantSobj:  "Contact",
			wantField: "Account.Name",
		},
		{
			name:      "in-values-first",
			query:     "SELECT Id FROM Account WHERE Industry IN (",
			cursor:    -1,
			wantCtx:   ContextInWithValues,
			wantSobj:  "Account",
			wantField: "Industry",
		},
		{
			name:      "in-values-after-comma",
			query:     "SELECT Id FROM Account WHERE Industry IN ('Tech', '",
			cursor:    -1,
			wantCtx:   ContextInWithValues,
			wantSobj:  "Account",
			wantField: "Industry",
		},
		{
			name:    "order-by-empty",
			query:   "SELECT Id FROM Account ORDER BY ",
			cursor:  -1,
			wantCtx: ContextOrderByField,
		},
		{
			name:    "group-by-empty",
			query:   "SELECT Count(Id) FROM Account GROUP BY ",
			cursor:  -1,
			wantCtx: ContextGroupByField,
		},
		{
			name:    "subquery-field-slot",
			query:   "SELECT Id, (SELECT Id, ",
			cursor:  -1,
			wantCtx: ContextAfterSelectKeyword,
			wantSub: true,
		},
		{
			name:     "subquery-with-from-and-where",
			query:    "SELECT Id, (SELECT Id FROM Contacts WHERE ",
			cursor:   -1,
			wantCtx:  ContextWhereField,
			wantSub:  true,
			wantSobj: "Contacts",
		},
		{
			name:     "mixed-case-keywords",
			query:    "select Id from Account wHeRe ",
			cursor:   -1,
			wantCtx:  ContextWhereField,
			wantSobj: "Account",
		},
		{
			name:     "dotted-path-relationship",
			query:    "SELECT Owner. FROM Account",
			cursor:   13, // right after the "."
			wantCtx:  ContextAfterSelectKeyword,
			wantPath: "Owner.",
			wantSobj: "Account",
		},
		{
			name:     "dotted-path-typing-field",
			query:    "SELECT Owner.Em FROM Account",
			cursor:   15,
			wantCtx:  ContextAfterSelectKeyword,
			wantTok:  "Em",
			wantPath: "Owner.Em",
			wantSobj: "Account",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cursor := tc.cursor
			if cursor < 0 {
				cursor = len(tc.query)
			}
			snap := Snapshot{Query: tc.query, CursorPos: cursor, SelEnd: cursor}
			got := Classify(snap)
			if got.Context != tc.wantCtx {
				t.Errorf("Context = %v, want %v", got.Context, tc.wantCtx)
			}
			if tc.wantTok != "" && got.SearchToken != tc.wantTok {
				t.Errorf("SearchToken = %q, want %q", got.SearchToken, tc.wantTok)
			}
			if tc.wantPath != "" && got.ContextPath != tc.wantPath {
				t.Errorf("ContextPath = %q, want %q", got.ContextPath, tc.wantPath)
			}
			if tc.wantSobj != "" && got.Sobject != tc.wantSobj {
				t.Errorf("Sobject = %q, want %q", got.Sobject, tc.wantSobj)
			}
			if got.InSubquery != tc.wantSub {
				t.Errorf("InSubquery = %v, want %v", got.InSubquery, tc.wantSub)
			}
			if tc.wantField != "" && got.WhereField != tc.wantField {
				t.Errorf("WhereField = %q, want %q", got.WhereField, tc.wantField)
			}
		})
	}
}

func TestHopsBeforeToken(t *testing.T) {
	cases := []struct {
		path string
		want []string
	}{
		{"", nil},
		{"Name", nil}, // bare token, no hops
		{"Account.", []string{"Account"}},
		{"Account.Name", []string{"Account"}},
		{"Account.Owner.", []string{"Account", "Owner"}},
		{"Account.Owner.Manager.Email", []string{"Account", "Owner", "Manager"}},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := HopsBeforeToken(tc.path)
			if len(got) != len(tc.want) {
				t.Fatalf("hops = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("hops[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
