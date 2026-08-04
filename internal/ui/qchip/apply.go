package qchip

import (
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/query"
)

// Substitutions are runtime-only values resolved before a chip's
// Query is applied. The most common is $userId — built-in chips
// reference the current user without baking an id into settings.toml.
//
// UserName mirrors UserID for client-side matchers (e.g. the "Mine"
// chip on /flows that filters on CreatedBy.Name = $userName). The
// records-shaped SOQL chips don't tend to need it — they filter on
// OwnerId / CreatedById — but client-side rows like sf.Flow only
// expose the human display name on LastModifiedBy / CreatedBy, so
// the substitution has to be a name string.
type Substitutions struct {
	UserID   string
	UserName string
}

// ApplyToSOQL renders a chip's Query as a SOQL statement against
// fromSObject, substituting Subs into any $userId tokens. Used by the
// records-shaped surfaces where chips run server-side.
func ApplyToSOQL(c Chip, fromSObject string, subs Substitutions) string {
	q := substitute(c.Query, subs)
	return query.ToSOQL(q, fromSObject)
}

// ApplyToRow runs a chip's predicate against a single row. Used by
// client-filtering surfaces (objects, flows). Returns true when the
// row satisfies the predicate.
func ApplyToRow(c Chip, row query.Row, subs Substitutions) bool {
	q := substitute(c.Query, subs)
	return query.Eval(q.Where, row)
}

// SubstituteWhere is the exported single-node entry point used by
// client-side matchers (chipMatcherFor) that don't need OrderBy /
// Limit / Columns. Returns a deep-copied tree so the original chip
// AST stays immutable for repeated apply-to-different-orgs calls.
// Pass-through nil for nil so callers can use it on any chip.
func SubstituteWhere(n query.Node, subs Substitutions) query.Node {
	if n == nil {
		return nil
	}
	return substituteNode(n, subs)
}

// substitute walks the AST and replaces $userId / $userid tokens in
// CompareNode values with the runtime value. Other nodes pass through
// unchanged. Returns a *new* Query — the input AST is left intact so
// the same chip can be applied repeatedly to different orgs.
func substitute(q query.Query, subs Substitutions) query.Query {
	out := query.Query{
		OrderBy: append([]query.OrderBy(nil), q.OrderBy...),
		Limit:   q.Limit,
		Columns: append([]string(nil), q.Columns...),
	}
	if q.Where != nil {
		out.Where = substituteNode(q.Where, subs)
	}
	return out
}

func substituteNode(n query.Node, subs Substitutions) query.Node {
	switch x := n.(type) {
	case query.CompareNode:
		x.Value = substituteValue(x.Value, subs)
		return x
	case query.AndNode:
		out := query.AndNode{Children: make([]query.Node, len(x.Children))}
		for i, c := range x.Children {
			out.Children[i] = substituteNode(c, subs)
		}
		return out
	case query.OrNode:
		out := query.OrNode{Children: make([]query.Node, len(x.Children))}
		for i, c := range x.Children {
			out.Children[i] = substituteNode(c, subs)
		}
		return out
	case query.NotNode:
		return query.NotNode{Child: substituteNode(x.Child, subs)}
	}
	return n
}

func substituteValue(v any, subs Substitutions) any {
	switch x := v.(type) {
	case string:
		if strings.EqualFold(x, "$userId") {
			return subs.UserID
		}
		if strings.EqualFold(x, "$userName") {
			return subs.UserName
		}
		return x
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = substituteValue(e, subs)
		}
		return out
	}
	return v
}
