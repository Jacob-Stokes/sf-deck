// Package soqlfmt is a small text-pass SOQL formatter — turns
// "select id,name from account where industry='Tech' order by
// name limit 50" into the clause-per-line shape every SOQL author
// already writes by hand:
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

func emit(tokens []token) string {
	var b strings.Builder
	indent := ""
	atLineStart := true
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
			if t.Text == "," {
				if atLineStart {
					b.WriteString(indent)
					atLineStart = false
				}
				trimTrailingSpace(&b)
				b.WriteString(", ")
				i = skipSpaces(i+1) - 1 // -1 because loop ++s
				continue
			}
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
			if upper == "ORDER" || upper == "GROUP" {
				j := skipSpaces(i + 1)
				if j < len(tokens) && tokens[j].Kind == kindWord && strings.EqualFold(tokens[j].Text, "BY") {
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

func isClauseStarter(upper string) bool {
	switch upper {
	case "SELECT", "FROM", "WHERE", "HAVING",
		"LIMIT", "OFFSET", "FOR", "WITH", "USING":
		return true
	}
	return false
}

func trimTrailingSpace(b *strings.Builder) {
	s := b.String()
	trimmed := strings.TrimRightFunc(s, unicode.IsSpace)
	if len(trimmed) == len(s) {
		return
	}
	b.Reset()
	b.WriteString(trimmed)
}
