// Package query defines a small AST that expresses sf-deck's chip
// queries. Every chip — whether it ends up running server-side as
// SOQL or client-side as a predicate — is the same shape: a tree of
// comparisons combined with AND/OR/NOT, plus optional ORDER BY,
// LIMIT, and column projection.
//
// The AST has two execution modes implemented as independent walkers
// over the same tree:
//
//	ToSOQL(expr) string     → server execution (sql.go)
//	Eval(expr, row) bool    → client execution (eval.go)
//
// Lockstep: both walkers are tested side-by-side in sql_test.go so a
// new operator can't land in one without showing up in the other.
// That's the substitute for a shared compile-and-emit pass — each
// walker is small enough that branching the same tree twice is
// cleaner than synthesising an intermediate form.
//
// Persistence: the AST round-trips through a tagged-union form
// (see node.go's NodeYAML) so settings.toml stays human-readable
// while still encoding the tree structure.
package query

// Op enumerates the comparison operators a Compare node can carry.
// New operators land here and require updates in three places:
// Eval (eval.go), ToSOQL (sql.go), and the parser (parse.go).
//
// The string values are stable on disk — never rename them, only
// add new ones at the bottom.
type Op string

const (
	// OpEq matches when the field equals the literal exactly.
	// Comparison is case-sensitive against strings — for a
	// case-insensitive match use OpContains with the full value or
	// downcase both sides at the row layer.
	OpEq Op = "eq"

	// OpNotEq is the inverse of OpEq.
	OpNotEq Op = "ne"

	// OpContains is a case-insensitive substring match. Strings only;
	// numeric / boolean values fall through as false.
	OpContains Op = "contains"

	// OpStartsWith / OpEndsWith are case-insensitive prefix/suffix
	// matches against the field.
	OpStartsWith Op = "starts"
	OpEndsWith   Op = "ends"

	// OpIn matches when the field equals any of a list of literals.
	// Sibling to a chain of OpEq + OpOr, but cheaper to read on
	// disk and to render in the wizard.
	OpIn Op = "in"

	// OpGT / OpGTE / OpLT / OpLTE are ordered comparisons. Apply to
	// numbers and to ISO-8601 date / datetime strings (lexical
	// compare matches chronological order for that format).
	OpGT  Op = "gt"
	OpGTE Op = "gte"
	OpLT  Op = "lt"
	OpLTE Op = "lte"

	// OpIsNull matches rows where the field has no value. Handy for
	// "Active version exists" / "no description" filters that both
	// the wizard and SOQL should round-trip identically.
	OpIsNull Op = "isnull"

	// OpDateLiteral matches a Salesforce date-literal token like
	// TODAY / YESTERDAY / THIS_WEEK / LAST_N_DAYS:30. SOQL emits the
	// literal verbatim; Eval maps it to a date-window check the
	// runtime evaluator resolves at apply time. Value is the literal
	// string ("TODAY", "LAST_N_DAYS:30", etc).
	OpDateLiteral Op = "date_literal"
)

// String returns the underlying string for the operator. Used by the
// persistence layer + a few error paths.
func (o Op) String() string { return string(o) }

// Node is the AST root type. Implementations are AndNode, OrNode,
// NotNode, and CompareNode. Use a sealed interface (unexported tag
// method) so future variants are an additive change in this package
// only — emitters break loudly if someone adds a new Node without
// updating them.
type Node interface {
	// node is unexported so this interface can only be implemented
	// inside this package. See https://go.dev/wiki/CodeReviewComments
	// "Pseudo-Sealed Interfaces" for the pattern.
	node()
}

// CompareNode is a single field-vs-literal comparison.
//
// Field is a logical column name — the Row interface (see eval.go)
// translates this into a struct field at evaluation time. SOQL
// emission uses Field verbatim, so callers should pass the SOQL
// column name (e.g. "DeveloperName" not "name").
//
// Value is one of: string, int, int64, float64, bool, time.Time, or
// []any for OpIn. Other types panic in Eval / ToSOQL.
type CompareNode struct {
	Field string
	Op    Op
	Value any
}

func (CompareNode) node() {}

// AndNode is the conjunction of every child. An empty children list
// matches every row (vacuous truth) so a chip with no constraints
// behaves like the "All" built-in.
type AndNode struct {
	Children []Node
}

func (AndNode) node() {}

// OrNode is the disjunction of every child. An empty children list
// matches no rows.
type OrNode struct {
	Children []Node
}

func (OrNode) node() {}

// NotNode negates its child. Wrapping a CompareNode in NotNode
// produces "not contains", "not equals", etc. without doubling the
// operator vocabulary.
type NotNode struct {
	Child Node
}

func (NotNode) node() {}

// Direction is the ORDER BY direction.
type Direction string

const (
	Ascending  Direction = "asc"
	Descending Direction = "desc"
)

// OrderBy is one ORDER BY clause entry. SOQL allows multiple, so
// callers carry a slice. Field is the same logical name CompareNode
// uses; emitters / evaluators must agree.
type OrderBy struct {
	Field     string
	Direction Direction
	NullsLast bool // SOQL: NULLS LAST
}

// Query is the top-level expression: the predicate tree plus the
// projection / ordering / limit. A nil Where means "match everything"
// — equivalent to AndNode{} but cheaper to construct.
//
// Columns is the SELECT list in SOQL terms. Used by lens-shaped
// surfaces (records); client-side surfaces ignore Columns and load
// every field they have available.
type Query struct {
	Where   Node
	OrderBy []OrderBy
	Limit   int      // 0 = no limit
	Columns []string // empty = "default columns for this surface"
}

// IsEmpty reports whether the query has no constraints at all — used
// by callers that want a fast "All" check before walking the tree.
func (q Query) IsEmpty() bool {
	if q.Where == nil {
		return len(q.OrderBy) == 0 && q.Limit == 0 && len(q.Columns) == 0
	}
	switch n := q.Where.(type) {
	case AndNode:
		return len(n.Children) == 0
	case OrNode:
		return len(n.Children) == 0
	}
	return false
}

// And is a small helper that flattens trivially-nestable trees as it
// builds them — `And(a, And(b, c))` becomes `And(a, b, c)`. Saves
// callers from caring about the shape and helps emitters produce
// fewer parens.
func And(children ...Node) Node {
	flat := make([]Node, 0, len(children))
	for _, c := range children {
		if c == nil {
			continue
		}
		if a, ok := c.(AndNode); ok {
			flat = append(flat, a.Children...)
			continue
		}
		flat = append(flat, c)
	}
	switch len(flat) {
	case 0:
		return AndNode{}
	case 1:
		return flat[0]
	}
	return AndNode{Children: flat}
}

// Or mirrors And.
func Or(children ...Node) Node {
	flat := make([]Node, 0, len(children))
	for _, c := range children {
		if c == nil {
			continue
		}
		if o, ok := c.(OrNode); ok {
			flat = append(flat, o.Children...)
			continue
		}
		flat = append(flat, c)
	}
	switch len(flat) {
	case 0:
		return OrNode{}
	case 1:
		return flat[0]
	}
	return OrNode{Children: flat}
}

// Not wraps a child in negation, collapsing double negation as it
// goes (`Not(Not(x))` → `x`). The compiler-friendly identity costs
// nothing and keeps the tree shallow.
func Not(child Node) Node {
	if n, ok := child.(NotNode); ok {
		return n.Child
	}
	return NotNode{Child: child}
}

// Cmp is the canonical CompareNode constructor — strictly nicer to
// read at call sites than struct literals.
func Cmp(field string, op Op, value any) Node {
	return CompareNode{Field: field, Op: op, Value: value}
}
