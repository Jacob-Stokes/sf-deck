// Package soqlfmt is a small text-pass SOQL formatter — turns
// "select id,name from account where industry='Tech' order by
// name limit 50" into the clause-per-line shape every SOQL author
// already writes by hand:
//
//	SELECT Id, Name
//	FROM Account
//	WHERE Industry = 'Tech'
//	ORDER BY Name
//	LIMIT 50
//
// Strategy: tokenize the query at whitespace + punctuation boundaries
// (single-quoted string literals are atomic), upper-case clause
// keywords, lower-case function calls and field references, then
// re-emit with line breaks before each top-level clause keyword.
//
// Not a parser. We don't validate structure — we just shuffle
// whitespace and case. Embedded subqueries stay on one line (they
// rarely benefit from breaking, and they confuse the simple "is
// this an outer-level clause keyword?" check). Aliases and
// relationship paths are preserved verbatim.
package soqlfmt

import (
	"strings"
	"unicode"
)

// Format reflows the query. Idempotent — formatting an already-
// formatted query returns the same string.
func Format(query string) string {
	tokens := tokenize(query)
	if len(tokens) == 0 {
		return strings.TrimSpace(query)
	}
	return emit(tokens)
}

type tokenKind int

const (
	kindWord tokenKind = iota
	kindString
	kindPunct
	kindSpace
)

type token struct {
	Text string
	Kind tokenKind
}

// tokenize splits the query into atomic tokens. Single-quoted
// strings stay whole (so `'don\'t'` doesn't get cut). Multi-char
// punctuation like `>=`, `<=`, `!=`, `<>` clump together. Words
// are letter/digit/underscore/dot runs.
func tokenize(s string) []token {
	var out []token
	i := 0
	for i < len(s) {
		ch := s[i]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r':
			j := i
			for j < len(s) {
				c := s[j]
				if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
					break
				}
				j++
			}
			out = append(out, token{Text: " ", Kind: kindSpace})
			i = j
		case ch == '\'':
			// Quoted literal. Walk to the matching ' (handling \\').
			j := i + 1
			for j < len(s) {
				if s[j] == '\\' && j+1 < len(s) {
					j += 2
					continue
				}
				if s[j] == '\'' {
					j++
					break
				}
				j++
			}
			out = append(out, token{Text: s[i:j], Kind: kindString})
			i = j
		case isWordChar(ch):
			j := i
			for j < len(s) && isWordChar(s[j]) {
				j++
			}
			out = append(out, token{Text: s[i:j], Kind: kindWord})
			i = j
		default:
			// Multi-char operators: >=, <=, !=, <>.
			if i+1 < len(s) {
				two := s[i : i+2]
				if two == ">=" || two == "<=" || two == "!=" || two == "<>" {
					out = append(out, token{Text: two, Kind: kindPunct})
					i += 2
					continue
				}
			}
			out = append(out, token{Text: string(ch), Kind: kindPunct})
			i++
		}
	}
	return out
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '_' || b == '.'
}

// upperKeywords are recognised SOQL words we always uppercase
// regardless of position. Includes operators, qualifiers,
// directionals.
var upperKeywords = map[string]bool{
	"SELECT": true, "FROM": true, "WHERE": true, "AND": true, "OR": true,
	"NOT": true, "LIKE": true, "IN": true, "INCLUDES": true, "EXCLUDES": true,
	"ORDER": true, "BY": true, "GROUP": true, "HAVING": true, "LIMIT": true,
	"OFFSET": true, "ASC": true, "DESC": true, "NULLS": true, "FIRST": true,
	"LAST": true, "FOR": true, "VIEW": true, "REFERENCE": true, "UPDATE": true,
	"WITH": true, "SECURITY_ENFORCED": true, "USER_MODE": true, "SYSTEM_MODE": true,
	"USING": true, "SCOPE": true, "TRUE": true, "FALSE": true, "NULL": true,
	"COUNT": true, "COUNT_DISTINCT": true, "SUM": true, "AVG": true, "MIN": true,
	"MAX": true, "FIELDS": true,
}

// emit re-assembles tokens into a formatted string. Logic:
//  1. Walk tokens, building lines. Each clause keyword starts a
//     new line at indent 0.
//  2. AND/OR inside a WHERE/HAVING clause start a new line at
//     indent 2 (continuation).
//  3. Commas in SELECT projections get a space after but stay on
//     the same line — projection lists are usually short enough
//     to fit one line, and breaking on every comma reads worse.
//  4. Strings + non-keyword words: uppercase the keyword set,
//     preserve casing of everything else (field names like
//     `Account.Name` stay as typed).
func emit(tokens []token) string {
	var b strings.Builder
	indent := ""
	atLineStart := true
	// Track whether we're past the WHERE so AND/OR get the
	// continuation indent rather than being treated as bare words.
	inWhereOrHaving := false

	skipSpaces := func(i int) int {
		j := i
		for j < len(tokens) && tokens[j].Kind == kindSpace {
			j++
		}
		return j
	}

	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		switch t.Kind {
		case kindSpace:
			if !atLineStart {
				b.WriteByte(' ')
			}
			continue
		case kindString:
			if atLineStart {
				b.WriteString(indent)
				atLineStart = false
			}
			b.WriteString(t.Text)
			continue
		case kindPunct:
			// Comma → emit then ensure exactly one space follows.
			if t.Text == "," {
				if atLineStart {
					b.WriteString(indent)
					atLineStart = false
				}
				// Strip trailing space from previous chars.
				trimTrailingSpace(&b)
				b.WriteString(", ")
				i = skipSpaces(i+1) - 1 // -1 because loop ++s
				continue
			}
			// Comparison operators: pad with spaces.
			if t.Text == "=" || t.Text == "<" || t.Text == ">" ||
				t.Text == ">=" || t.Text == "<=" || t.Text == "!=" || t.Text == "<>" {
				if atLineStart {
					b.WriteString(indent)
					atLineStart = false
				}
				trimTrailingSpace(&b)
				b.WriteString(" " + t.Text + " ")
				i = skipSpaces(i+1) - 1
				continue
			}
			if atLineStart {
				b.WriteString(indent)
				atLineStart = false
			}
			b.WriteString(t.Text)
			continue
		case kindWord:
			upper := strings.ToUpper(t.Text)
			// Multi-word clause keyword? Peek ahead for "ORDER BY",
			// "GROUP BY".
			if upper == "ORDER" || upper == "GROUP" {
				j := skipSpaces(i + 1)
				if j < len(tokens) && tokens[j].Kind == kindWord && strings.EqualFold(tokens[j].Text, "BY") {
					// Emit "ORDER BY" / "GROUP BY" on a new line.
					if b.Len() > 0 {
						trimTrailingSpace(&b)
						b.WriteByte('\n')
					}
					b.WriteString(upper + " BY")
					indent = ""
					atLineStart = false
					inWhereOrHaving = false
					i = j
					continue
				}
			}
			// Single-word clause keyword on a new line.
			if isClauseStarter(upper) {
				if b.Len() > 0 {
					trimTrailingSpace(&b)
					b.WriteByte('\n')
				}
				b.WriteString(upper)
				indent = ""
				atLineStart = false
				if upper == "WHERE" || upper == "HAVING" {
					inWhereOrHaving = true
				} else {
					inWhereOrHaving = false
				}
				continue
			}
			// Continuation keyword AND/OR inside WHERE/HAVING.
			if inWhereOrHaving && (upper == "AND" || upper == "OR") {
				trimTrailingSpace(&b)
				b.WriteByte('\n')
				b.WriteString("  " + upper)
				atLineStart = false
				continue
			}
			// Other recognised keyword — uppercase, otherwise
			// preserve as-typed.
			if atLineStart {
				b.WriteString(indent)
				atLineStart = false
			}
			if upperKeywords[upper] {
				b.WriteString(upper)
			} else {
				b.WriteString(t.Text)
			}
			continue
		}
	}
	out := strings.TrimRight(b.String(), " \n\t")
	return out
}

// isClauseStarter is true for single-word top-level keywords that
// start a new line. "ORDER" / "GROUP" are handled separately
// (multi-word).
func isClauseStarter(upper string) bool {
	switch upper {
	case "SELECT", "FROM", "WHERE", "HAVING",
		"LIMIT", "OFFSET", "FOR", "WITH", "USING":
		return true
	}
	return false
}

// trimTrailingSpace removes trailing space characters from a
// strings.Builder by re-allocating. Cheap for query-sized strings;
// the builder doesn't expose Truncate so we have to round-trip.
func trimTrailingSpace(b *strings.Builder) {
	s := b.String()
	trimmed := strings.TrimRightFunc(s, unicode.IsSpace)
	if len(trimmed) == len(s) {
		return
	}
	b.Reset()
	b.WriteString(trimmed)
}
