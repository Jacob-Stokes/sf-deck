package sf

import "testing"

// TestSQLEscape locks in that sqlEscape neutralizes BOTH the single
// quote and the backslash. Escaping only the quote (the old behavior)
// let a value ending in `\` turn the appended closing quote into an
// escaped quote, breaking out of the SOQL string literal.
func TestSQLEscape(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", "Account", "Account"},
		{"single quote", "O'Brien", `O\'Brien`},
		{"trailing backslash", `abc\`, `abc\\`},
		{"backslash then quote attempt", `\'`, `\\\'`},
		{"embedded backslash", `a\b`, `a\\b`},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sqlEscape(c.in); got != c.want {
				t.Errorf("sqlEscape(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}

	// The concrete break-out the fix prevents: a trailing backslash must
	// not escape the closing quote of `'<value>'`.
	q := "'" + sqlEscape(`x\`) + "'"
	if q != `'x\\'` {
		t.Errorf("quoted literal = %q, want %q (closing quote must stay literal)", q, `'x\\'`)
	}
}
