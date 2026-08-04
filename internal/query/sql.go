package query

import (
	"strings"
)

// ToSOQLWhere walks a predicate tree and emits the SOQL fragment that
// goes after WHERE. A nil node returns the empty string — callers
// concatenate with " WHERE " only when the result is non-empty.
//
// String literals are single-quote-escaped. SOQL has no parameter
// binding so escaping has to be airtight; we double up apostrophes
// (the only escaping SOQL accepts inside a string literal). Other
// literal types — int, bool, ISO date — render verbatim without
// quoting since their grammar is unambiguous.
//
// AND has higher precedence than OR in SOQL, so OrNode children get
// wrapped in parentheses when they're under an outer And. We always
// paren around mixed-precedence groups to keep the emitted SQL
// readable on disk + safe across SOQL parser quirks.
func ToSOQLWhere(node Node) string {
	if node == nil {
		return ""
	}
	return emitNode(node, 0)
}

// ToSOQLClauses emits everything AFTER the FROM clause — the
// WHERE, ORDER BY, and LIMIT segments separated by spaces. Used by
// the chip wizard's advanced mode where the textarea expresses just
// the filtering clauses (no SELECT / FROM, since those are
// determined by the chip's runtime context).
//
// Returns "" when the Query carries no clauses. Each present clause
// includes its keyword ("WHERE", "ORDER BY", "LIMIT").
func ToSOQLClauses(q Query) string {
	var parts []string
	if where := ToSOQLWhere(q.Where); where != "" {
		parts = append(parts, "WHERE "+where)
	}
	if len(q.OrderBy) > 0 {
		var ob strings.Builder
		ob.WriteString("ORDER BY ")
		for i, o := range q.OrderBy {
			if i > 0 {
				ob.WriteString(", ")
			}
			ob.WriteString(o.Field)
			switch o.Direction {
			case Descending:
				ob.WriteString(" DESC")
			default:
				ob.WriteString(" ASC")
			}
			if o.NullsLast {
				ob.WriteString(" NULLS LAST")
			}
		}
		parts = append(parts, ob.String())
	}
	if q.Limit > 0 {
		parts = append(parts, "LIMIT "+itoa64(int64(q.Limit)))
	}
	return strings.Join(parts, " ")
}

// ToSOQL builds the full query string: SELECT … FROM <object> WHERE …
// ORDER BY … LIMIT N. The caller supplies the FROM target (sObject
// name) since the AST itself is FROM-agnostic. Returns "" when the
// query has no SELECT columns + no FROM target — signals "let the
// caller decide".
//
// The output is well-formed SOQL the runtime can hand straight to
// `Query` — round-tripping through the parser (parse.go) returns the
// same QueryExpr we started with.
func ToSOQL(q Query, fromSObject string) string {
	var b strings.Builder
	b.WriteString("SELECT ")
	if len(q.Columns) == 0 {
		b.WriteString("Id")
	} else {
		b.WriteString(strings.Join(q.Columns, ", "))
	}
	b.WriteString(" FROM ")
	b.WriteString(fromSObject)
	if where := ToSOQLWhere(q.Where); where != "" {
		b.WriteString(" WHERE ")
		b.WriteString(where)
	}
	if len(q.OrderBy) > 0 {
		b.WriteString(" ORDER BY ")
		for i, ob := range q.OrderBy {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(ob.Field)
			switch ob.Direction {
			case Descending:
				b.WriteString(" DESC")
			default:
				b.WriteString(" ASC")
			}
			if ob.NullsLast {
				b.WriteString(" NULLS LAST")
			}
		}
	}
	if q.Limit > 0 {
		b.WriteString(" LIMIT ")
		b.WriteString(itoa64(int64(q.Limit)))
	}
	return b.String()
}

// emitNode is the recursive worker. parentPrecedence tracks the
// surrounding operator so we know when to add parens.
//
//	0 — top-level / inside parens (no surrounding op)
//	1 — under an OR
//	2 — under an AND
//
// AND binds tighter than OR. An OR child of an AND must be parenthesised.
func emitNode(node Node, parentPrec int) string {
	switch n := node.(type) {
	case CompareNode:
		return emitCompare(n)
	case AndNode:
		if len(n.Children) == 0 {
			// Vacuous AND: SOQL has no "TRUE" so we use a tautology.
			// Callers that recognise IsEmpty() typically skip the
			// WHERE clause entirely so this is rarely emitted.
			return "Id != null"
		}
		parts := make([]string, len(n.Children))
		for i, c := range n.Children {
			parts[i] = emitNode(c, 2)
		}
		out := strings.Join(parts, " AND ")
		if parentPrec == 0 {
			return out
		}
		return "(" + out + ")"
	case OrNode:
		if len(n.Children) == 0 {
			// Vacuous OR — never matches anything. Salesforce SOQL
			// doesn't have a literal "FALSE" so we use an unsatisfiable
			// comparison.
			return "Id = null"
		}
		parts := make([]string, len(n.Children))
		for i, c := range n.Children {
			parts[i] = emitNode(c, 1)
		}
		out := strings.Join(parts, " OR ")
		if parentPrec >= 2 {
			// Under an AND — paren to preserve precedence.
			return "(" + out + ")"
		}
		return out
	case NotNode:
		return "(NOT " + emitNode(n.Child, 0) + ")"
	}
	return ""
}

// emitCompare renders one CompareNode. Per-operator literal handling
// lives here because IN-lists, IS NULL, and LIKE all break the
// "field op literal" mould.
func emitCompare(c CompareNode) string {
	switch c.Op {
	case OpDateLiteral:
		// Salesforce date literals (TODAY, LAST_N_DAYS:30, …) emit
		// verbatim with the standard `=` operator. Match what the
		// list-view describe API gives us so re-importing is lossless.
		return c.Field + " = " + toString(c.Value)
	case OpIsNull:
		return c.Field + " = null"
	case OpEq:
		return c.Field + " = " + emitLiteral(c.Value)
	case OpNotEq:
		return c.Field + " != " + emitLiteral(c.Value)
	case OpGT:
		return c.Field + " > " + emitLiteral(c.Value)
	case OpGTE:
		return c.Field + " >= " + emitLiteral(c.Value)
	case OpLT:
		return c.Field + " < " + emitLiteral(c.Value)
	case OpLTE:
		return c.Field + " <= " + emitLiteral(c.Value)
	case OpContains:
		return c.Field + " LIKE " + emitStringLiteral("%"+toString(c.Value)+"%")
	case OpStartsWith:
		return c.Field + " LIKE " + emitStringLiteral(toString(c.Value)+"%")
	case OpEndsWith:
		return c.Field + " LIKE " + emitStringLiteral("%"+toString(c.Value))
	case OpIn:
		list, ok := c.Value.([]any)
		if !ok || len(list) == 0 {
			// Empty IN — render as something that can never match so
			// the query stays well-formed.
			return c.Field + " IN ('')"
		}
		parts := make([]string, len(list))
		for i, e := range list {
			parts[i] = emitLiteral(e)
		}
		return c.Field + " IN (" + strings.Join(parts, ", ") + ")"
	}
	return ""
}

// emitLiteral renders a CompareNode value as SOQL. Strings are
// quoted; numbers + booleans render verbatim.
func emitLiteral(v any) string {
	switch x := v.(type) {
	case string:
		// Heuristic: Salesforce date / datetime / id literals don't
		// take quotes (e.g. `LastModifiedDate > 2025-01-01T00:00:00Z`).
		// We sniff for ISO-8601-shaped strings and emit them bare.
		// Plain strings get quoted normally.
		if looksLikeDate(x) {
			return x
		}
		return emitStringLiteral(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return itoa64(int64(x))
	case int64:
		return itoa64(x)
	case float64:
		if x == float64(int64(x)) {
			return itoa64(int64(x))
		}
		return ftoa(x)
	case nil:
		return "null"
	}
	return "''"
}

// emitStringLiteral wraps a string in single quotes for a SOQL string
// literal. Backslash MUST be escaped first — otherwise a value ending
// in `\` turns the appended closing quote into an escaped quote (`\'`),
// letting the literal run on into the next clause. That's a real
// break-out here because chip WHERE values come from the chip wizard
// and from imported/shared chip definitions (org + cross-user data).
func emitStringLiteral(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "'", `\'`)
	return "'" + s + "'"
}

// looksLikeDate is the cheap "should I quote this?" heuristic.
// Matches `YYYY-MM-DD` and `YYYY-MM-DDTHH:MM:SS(Z)` shapes — anything
// else is treated as a regular string and quoted.
func looksLikeDate(s string) bool {
	if len(s) < 10 || len(s) > 30 {
		return false
	}
	if s[4] != '-' || s[7] != '-' {
		return false
	}
	for i, r := range s[:10] {
		if i == 4 || i == 7 {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	if len(s) == 10 {
		return true
	}
	if s[10] != 'T' {
		return false
	}
	return true
}
