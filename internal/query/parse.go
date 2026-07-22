package query

import (
	"fmt"
	"strconv"
	"strings"
)

// Parse takes a full SOQL SELECT statement and returns the equivalent
// Query plus the FROM target. The WHERE clause is parsed recursively
// into the AST; ORDER BY / LIMIT / SELECT lists come along for the
// ride.
//
// Supported subset:
//   - SELECT a, b, c
//   - FROM <sObject>
//   - WHERE expr  with comparisons, AND / OR / NOT, parens, IN, LIKE,
//     IS NULL / IS NOT NULL, ordered comparisons
//   - ORDER BY field [ASC|DESC] [NULLS FIRST|LAST] (multiple, comma-sep)
//   - LIMIT n
//
// Not supported (these clauses cause Parse to return ErrUnsupported,
// preserving what we did parse so the caller can still save the
// partial chip):
//   - subqueries (parenthesised SELECTs)
//   - GROUP BY / HAVING / OFFSET / FOR UPDATE / TYPEOF / USING SCOPE
//   - date literals like LAST_N_DAYS:30 (use absolute dates instead)
//
// On a parse error the function returns the partial Query alongside
// the error so the import flow can still surface what was understood.
func Parse(soql string) (Query, string, error) {
	parts, err := splitSOQL(soql)
	if err != nil {
		return Query{}, "", err
	}
	q := Query{
		Limit:   parts.limit,
		Columns: parts.selectCols,
	}
	if parts.where != "" {
		w, perr := parseWhere(parts.where)
		if perr != nil {
			// Partial result: keep SELECT / FROM / ORDER BY / LIMIT,
			// drop the WHERE we couldn't parse. Caller decides whether
			// to surface the warning.
			return q, parts.from, perr
		}
		q.Where = w
	}
	if parts.orderBy != "" {
		obs, perr := parseOrderBy(parts.orderBy)
		if perr != nil {
			return q, parts.from, perr
		}
		q.OrderBy = obs
	}
	return q, parts.from, nil
}

// soqlSplit is the structural break-up done before tokenisation —
// finds clause boundaries respecting parens + string literals so that
// e.g. an apostrophe inside a string literal doesn't hide a later WHERE.
type soqlSplit struct {
	selectCols []string
	from       string
	where      string
	orderBy    string
	limit      int
}

func splitSOQL(s string) (soqlSplit, error) {
	s = strings.TrimSpace(s)
	upper := strings.ToUpper(s)
	out := soqlSplit{}

	// Walk the string char-by-char to find the keyword boundaries that
	// aren't inside a string literal or paren group. This is the crucial
	// difference vs a plain strings.Index — `Name = 'WHERE'` shouldn't
	// terminate the SELECT.
	// marker.pos is the index of the *first* char of the keyword
	// (e.g. the W of WHERE), so start = pos + len(kw) skips it cleanly.
	type marker struct {
		kw  string
		pos int
	}
	var markers []marker
	depth := 0
	inStr := false
	clauseKWs := []string{"SELECT", "FROM", "WHERE", "ORDER BY", "LIMIT", "GROUP BY", "HAVING", "OFFSET"}

	tryMatch := func(at int) {
		// Must be at the start of the string, or preceded by whitespace.
		if at != 0 && upper[at-1] != ' ' && upper[at-1] != '\t' && upper[at-1] != '\n' {
			return
		}
		for _, kw := range clauseKWs {
			end := at + len(kw)
			if end > len(upper) {
				continue
			}
			if upper[at:end] != kw {
				continue
			}
			// Followed by whitespace or EOF.
			if end < len(upper) && upper[end] != ' ' && upper[end] != '\t' && upper[end] != '\n' {
				continue
			}
			markers = append(markers, marker{kw: kw, pos: at})
			return
		}
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == '\'' {
				inStr = false
			}
			continue
		}
		switch c {
		case '\'':
			inStr = true
		case '(':
			depth++
		case ')':
			depth--
		default:
			if depth == 0 {
				tryMatch(i)
			}
		}
	}

	if len(markers) == 0 {
		return out, fmt.Errorf("query: no SELECT keyword found")
	}

	// Reject unsupported clauses early.
	for _, m := range markers {
		switch m.kw {
		case "GROUP BY", "HAVING", "OFFSET":
			return out, fmt.Errorf("query: unsupported clause %q", m.kw)
		}
	}

	// Build the section map.
	get := func(kw string) (start, end int, ok bool) {
		for i, m := range markers {
			if m.kw != kw {
				continue
			}
			start = m.pos + len(kw)
			if i+1 < len(markers) {
				end = markers[i+1].pos
			} else {
				end = len(s)
			}
			return start, end, true
		}
		return 0, 0, false
	}

	if start, end, ok := get("SELECT"); ok {
		body := strings.TrimSpace(s[start:end])
		for _, c := range strings.Split(body, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				out.selectCols = append(out.selectCols, c)
			}
		}
	}
	if start, end, ok := get("FROM"); ok {
		out.from = strings.TrimSpace(s[start:end])
	}
	if start, end, ok := get("WHERE"); ok {
		out.where = strings.TrimSpace(s[start:end])
	}
	if start, end, ok := get("ORDER BY"); ok {
		out.orderBy = strings.TrimSpace(s[start:end])
	}
	if start, end, ok := get("LIMIT"); ok {
		body := strings.TrimSpace(s[start:end])
		// Find the first non-digit so we ignore trailing tokens.
		i := 0
		for i < len(body) && body[i] >= '0' && body[i] <= '9' {
			i++
		}
		if i > 0 {
			n, _ := strconv.Atoi(body[:i])
			out.limit = n
		}
	}
	return out, nil
}

// ---- WHERE parser ------------------------------------------------------

// parseWhere is a recursive-descent parser over the WHERE body.
//
// Grammar (informal):
//
//	expr     := term ( OR term )*
//	term     := factor ( AND factor )*
//	factor   := NOT factor | "(" expr ")" | compare
//	compare  := IDENT op_or_pred
//	op_or_pred := EQ literal | NE literal | GT literal | ...
//	             | LIKE string
//	             | IN "(" literal ("," literal)* ")"
//	             | IS [NOT] NULL
//
// The tokeniser handles whitespace, identifiers, numeric / string
// literals, parens, and the comparison operators.
func parseWhere(s string) (Node, error) {
	p := &parser{src: s, tokens: tokenise(s)}
	expr, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.tokens) {
		return expr, fmt.Errorf("query: trailing tokens after WHERE: %q", p.remaining())
	}
	return expr, nil
}

// token kinds for the recursive-descent parser.
type tokKind int

const (
	tkEOF tokKind = iota
	tkIdent
	tkString
	tkNumber
	tkLParen
	tkRParen
	tkComma
	tkOp // = != < <= > >=
	tkKW // AND / OR / NOT / LIKE / IN / IS / NULL / NOT-NULL / TRUE / FALSE
)

type token struct {
	kind tokKind
	text string // canonical: keywords uppercased, idents preserved
	raw  string // original (used by error messages)
}

func tokenise(s string) []token {
	var out []token
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			out = append(out, token{kind: tkLParen, text: "(", raw: "("})
			i++
		case c == ')':
			out = append(out, token{kind: tkRParen, text: ")", raw: ")"})
			i++
		case c == ',':
			out = append(out, token{kind: tkComma, text: ",", raw: ","})
			i++
		case c == '\'':
			j := i + 1
			for j < len(s) {
				if s[j] == '\\' && j+1 < len(s) {
					j += 2
					continue
				}
				if s[j] == '\'' {
					break
				}
				j++
			}
			body := s[i+1 : j]
			// Unescape \' → '
			body = strings.ReplaceAll(body, "\\'", "'")
			// Clamp the closing index: an UNTERMINATED literal (no closing
			// quote) leaves j == len(s), so s[i : j+1] would slice one past
			// the end and panic. The tokeniser must never panic — it runs
			// synchronously inside Update on raw user input (e.g. typing
			// `WHERE Name = 'foo` in the chip wizard), and only View() is
			// recover()-guarded. Treat the rest of the input as the string.
			end := j + 1
			if end > len(s) {
				end = len(s)
			}
			out = append(out, token{kind: tkString, text: body, raw: s[i:end]})
			i = end
		case c == '=':
			out = append(out, token{kind: tkOp, text: "=", raw: "="})
			i++
		case c == '!' && i+1 < len(s) && s[i+1] == '=':
			out = append(out, token{kind: tkOp, text: "!=", raw: "!="})
			i += 2
		case c == '<' || c == '>':
			tok := string(c)
			i++
			if i < len(s) && s[i] == '=' {
				tok += "="
				i++
			}
			out = append(out, token{kind: tkOp, text: tok, raw: tok})
		case c >= '0' && c <= '9':
			// Number — but watch for a date-shaped literal:
			// YYYY-MM-DD or YYYY-MM-DDTHH:MM:SS(Z). If we see four
			// digits followed by a hyphen, swallow the rest of the
			// date as one token (kind: tkIdent, since it carries no
			// quotes and the value-coercion step keeps it as a
			// string).
			if i+4 < len(s) && s[i+4] == '-' && allDigits(s[i:i+4]) {
				j := i + 4
				for j < len(s) {
					ch := s[j]
					if (ch >= '0' && ch <= '9') || ch == '-' || ch == ':' || ch == 'T' || ch == 'Z' || ch == '+' || ch == '.' {
						j++
						continue
					}
					break
				}
				out = append(out, token{kind: tkIdent, text: s[i:j], raw: s[i:j]})
				i = j
				continue
			}
			j := i
			for j < len(s) && ((s[j] >= '0' && s[j] <= '9') || s[j] == '.') {
				j++
			}
			out = append(out, token{kind: tkNumber, text: s[i:j], raw: s[i:j]})
			i = j
		default:
			// Identifier or keyword. Salesforce ids are alnum + underscore + dot
			// (relationship traversals like Account.Owner.Name). Date/datetime
			// literals like 2025-01-01T00:00:00Z aren't quoted in SOQL — we
			// treat them as identifiers and the value-coercion step decides.
			j := i
			for j < len(s) {
				ch := s[j]
				if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
					(ch >= '0' && ch <= '9') || ch == '_' || ch == '.' {
					j++
					continue
				}
				break
			}
			if j == i {
				// The byte at i isn't alnum/_/. and started no other token
				// kind — e.g. `-`, `*`, `&`, `|`, `@`, `/`. Without this the
				// inner loop never advances and the outer loop spins forever,
				// wedging the single-threaded Update loop (Parse runs there
				// on every chip-wizard submit). Emit the unknown byte as a
				// one-char token and move on; the parser then rejects it with
				// a normal "unexpected token" error instead of hanging.
				out = append(out, token{kind: tkIdent, text: s[i : i+1], raw: s[i : i+1]})
				i++
				continue
			}
			text := s[i:j]
			upper := strings.ToUpper(text)
			switch upper {
			case "AND", "OR", "NOT", "LIKE", "IN", "IS", "NULL", "TRUE", "FALSE":
				out = append(out, token{kind: tkKW, text: upper, raw: text})
			default:
				out = append(out, token{kind: tkIdent, text: text, raw: text})
			}
			i = j
		}
	}
	return out
}

// allDigits reports whether every byte of s is a decimal digit. Used by
// the tokeniser to detect the YYYY-MM-DD prefix of a date literal.
func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

type parser struct {
	src    string
	tokens []token
	pos    int
}

func (p *parser) peek() token {
	if p.pos >= len(p.tokens) {
		return token{kind: tkEOF}
	}
	return p.tokens[p.pos]
}

func (p *parser) advance() token {
	t := p.peek()
	p.pos++
	return t
}

func (p *parser) remaining() string {
	if p.pos >= len(p.tokens) {
		return ""
	}
	var b strings.Builder
	for _, t := range p.tokens[p.pos:] {
		b.WriteString(t.raw)
		b.WriteByte(' ')
	}
	return strings.TrimSpace(b.String())
}

func (p *parser) parseOr() (Node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tkKW && p.peek().text == "OR" {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = Or(left, right)
	}
	return left, nil
}

func (p *parser) parseAnd() (Node, error) {
	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tkKW && p.peek().text == "AND" {
		p.advance()
		right, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		left = And(left, right)
	}
	return left, nil
}

func (p *parser) parseFactor() (Node, error) {
	t := p.peek()
	if t.kind == tkKW && t.text == "NOT" {
		p.advance()
		child, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		return Not(child), nil
	}
	if t.kind == tkLParen {
		p.advance()
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tkRParen {
			return nil, fmt.Errorf("query: expected ')' got %q", p.peek().raw)
		}
		p.advance()
		return expr, nil
	}
	return p.parseCompare()
}

func (p *parser) parseCompare() (Node, error) {
	t := p.advance()
	if t.kind != tkIdent {
		return nil, fmt.Errorf("query: expected field name got %q", t.raw)
	}
	field := t.text

	op := p.advance()
	switch {
	case op.kind == tkOp:
		val, err := p.parseLiteral()
		if err != nil {
			return nil, err
		}
		switch op.text {
		case "=":
			if isNullLit(val) {
				return Cmp(field, OpIsNull, nil), nil
			}
			if isDateLiteral(val) {
				return Cmp(field, OpDateLiteral, val), nil
			}
			return Cmp(field, OpEq, val), nil
		case "!=":
			if isNullLit(val) {
				return Not(Cmp(field, OpIsNull, nil)), nil
			}
			if isDateLiteral(val) {
				return Not(Cmp(field, OpDateLiteral, val)), nil
			}
			return Cmp(field, OpNotEq, val), nil
		case ">":
			return Cmp(field, OpGT, val), nil
		case ">=":
			return Cmp(field, OpGTE, val), nil
		case "<":
			return Cmp(field, OpLT, val), nil
		case "<=":
			return Cmp(field, OpLTE, val), nil
		}
	case op.kind == tkKW && op.text == "LIKE":
		val, err := p.parseLiteral()
		if err != nil {
			return nil, err
		}
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("query: LIKE expects a string literal")
		}
		return likeToCompare(field, s), nil
	case op.kind == tkKW && op.text == "IN":
		if p.peek().kind != tkLParen {
			return nil, fmt.Errorf("query: IN expects '(' got %q", p.peek().raw)
		}
		p.advance()
		var list []any
		for {
			val, err := p.parseLiteral()
			if err != nil {
				return nil, err
			}
			list = append(list, val)
			if p.peek().kind == tkComma {
				p.advance()
				continue
			}
			break
		}
		if p.peek().kind != tkRParen {
			return nil, fmt.Errorf("query: IN expects ')' got %q", p.peek().raw)
		}
		p.advance()
		return Cmp(field, OpIn, list), nil
	case op.kind == tkKW && op.text == "IS":
		negate := false
		if p.peek().kind == tkKW && p.peek().text == "NOT" {
			p.advance()
			negate = true
		}
		nxt := p.advance()
		if nxt.kind != tkKW || nxt.text != "NULL" {
			return nil, fmt.Errorf("query: expected NULL after IS, got %q", nxt.raw)
		}
		if negate {
			return Not(Cmp(field, OpIsNull, nil)), nil
		}
		return Cmp(field, OpIsNull, nil), nil
	}
	return nil, fmt.Errorf("query: unexpected token after %q: %q", field, op.raw)
}

// parseLiteral consumes one literal token and returns its go value.
func (p *parser) parseLiteral() (any, error) {
	t := p.advance()
	switch t.kind {
	case tkString:
		return t.text, nil
	case tkNumber:
		if i, err := strconv.Atoi(t.text); err == nil {
			return i, nil
		}
		f, err := strconv.ParseFloat(t.text, 64)
		if err != nil {
			return nil, fmt.Errorf("query: bad numeric literal %q", t.raw)
		}
		return f, nil
	case tkKW:
		switch t.text {
		case "TRUE":
			return true, nil
		case "FALSE":
			return false, nil
		case "NULL":
			return nil, nil
		}
	case tkIdent:
		// Bare-token literal — usually a date/datetime, an enum-shaped
		// id, or a Salesforce ID. We pass it through as-is; the
		// emitter detects date-shaped strings and re-emits them
		// without quotes on the way out.
		return t.text, nil
	}
	return nil, fmt.Errorf("query: expected literal, got %q", t.raw)
}

// likeToCompare maps a SOQL LIKE pattern to one of OpStartsWith /
// OpEndsWith / OpContains / OpEq. Only `%` wildcards are supported;
// `_` (single-char wildcard) is rare in SOQL and we treat it
// literally — round-trip-safe for the common case.
func likeToCompare(field, pattern string) Node {
	startsAny := strings.HasPrefix(pattern, "%")
	endsAny := strings.HasSuffix(pattern, "%")
	body := pattern
	if startsAny {
		body = body[1:]
	}
	if endsAny {
		body = body[:len(body)-1]
	}
	switch {
	case startsAny && endsAny:
		return Cmp(field, OpContains, body)
	case startsAny:
		return Cmp(field, OpEndsWith, body)
	case endsAny:
		return Cmp(field, OpStartsWith, body)
	}
	// No wildcard at all — treat as equality (this is what SOQL does
	// in practice, give or take collation).
	return Cmp(field, OpEq, body)
}

// isNullLit reports whether the literal value should be treated as
// NULL — used to fold `field = null` into OpIsNull.
func isNullLit(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return strings.EqualFold(s, "null")
	}
	return false
}

// isDateLiteral reports whether a string is one of Salesforce's
// date-literal tokens. Used by the parser to fold `field = TODAY`
// into OpDateLiteral so Eval can resolve the window at apply time
// (rather than treating the literal as a plain string).
func isDateLiteral(v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	upper := strings.ToUpper(s)
	switch upper {
	case "TODAY", "YESTERDAY", "TOMORROW",
		"THIS_WEEK", "LAST_WEEK", "NEXT_WEEK",
		"THIS_MONTH", "LAST_MONTH", "NEXT_MONTH",
		"THIS_QUARTER", "LAST_QUARTER", "NEXT_QUARTER",
		"THIS_YEAR", "LAST_YEAR", "NEXT_YEAR":
		return true
	}
	return strings.HasPrefix(upper, "LAST_N_DAYS:") ||
		strings.HasPrefix(upper, "NEXT_N_DAYS:") ||
		strings.HasPrefix(upper, "LAST_N_WEEKS:") ||
		strings.HasPrefix(upper, "NEXT_N_WEEKS:") ||
		strings.HasPrefix(upper, "LAST_N_MONTHS:") ||
		strings.HasPrefix(upper, "NEXT_N_MONTHS:")
}

// ---- ORDER BY parser ---------------------------------------------------

// parseOrderBy splits an ORDER BY body into typed entries. SOQL grammar:
//
//	field [ASC|DESC] [NULLS FIRST|LAST] (, field [ASC|DESC] ...)*
func parseOrderBy(s string) ([]OrderBy, error) {
	var out []OrderBy
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		ob := OrderBy{Field: fields[0], Direction: Ascending}
		i := 1
		for i < len(fields) {
			tok := strings.ToUpper(fields[i])
			switch tok {
			case "ASC":
				ob.Direction = Ascending
				i++
			case "DESC":
				ob.Direction = Descending
				i++
			case "NULLS":
				if i+1 < len(fields) && strings.EqualFold(fields[i+1], "LAST") {
					ob.NullsLast = true
				}
				i += 2
			default:
				i++
			}
		}
		out = append(out, ob)
	}
	return out, nil
}
