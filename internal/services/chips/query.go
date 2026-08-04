package chips

import (
	"fmt"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

const clauseParseObject = "SfDeckChip__c"

// ParseClauses converts the chip wizard's advanced-mode text into the
// persisted query shape. clauses is everything after FROM: WHERE,
// ORDER BY, and LIMIT. Columns remain a separate chip display concern.
func ParseClauses(clauses string) (settings.ChipQueryYAML, error) {
	q, _, err := query.Parse("SELECT Id FROM " + clauseParseObject + " " + strings.TrimSpace(clauses))
	if err != nil {
		return settings.ChipQueryYAML{}, fmt.Errorf("parse clauses: %w", err)
	}
	q.Columns = nil
	return queryToConfig(q), nil
}

func queryFromConfig(qy settings.ChipQueryYAML) query.Query {
	out := query.Query{
		Limit:   qy.Limit,
		Columns: append([]string(nil), qy.Columns...),
	}
	if qy.Where != nil {
		out.Where = nodeFromConfig(*qy.Where)
	}
	for _, ob := range qy.OrderBy {
		dir := query.Direction(ob.Direction)
		if dir == "" {
			dir = query.Ascending
		}
		out.OrderBy = append(out.OrderBy, query.OrderBy{
			Field:     ob.Field,
			Direction: dir,
			NullsLast: ob.NullsLast,
		})
	}
	return out
}

func queryToConfig(q query.Query) settings.ChipQueryYAML {
	out := settings.ChipQueryYAML{
		Limit:   q.Limit,
		Columns: append([]string(nil), q.Columns...),
	}
	if q.Where != nil {
		w := nodeToConfig(q.Where)
		out.Where = &w
	}
	for _, ob := range q.OrderBy {
		out.OrderBy = append(out.OrderBy, settings.ChipOrderByYAML{
			Field:     ob.Field,
			Direction: string(ob.Direction),
			NullsLast: ob.NullsLast,
		})
	}
	return out
}

func nodeFromConfig(n settings.ChipNodeYAML) query.Node {
	switch n.Kind {
	case "cmp":
		return query.CompareNode{
			Field: n.Field,
			Op:    query.Op(n.Op),
			Value: n.Value,
		}
	case "and":
		out := query.AndNode{Children: make([]query.Node, len(n.Children))}
		for i, c := range n.Children {
			out.Children[i] = nodeFromConfig(c)
		}
		return out
	case "or":
		out := query.OrNode{Children: make([]query.Node, len(n.Children))}
		for i, c := range n.Children {
			out.Children[i] = nodeFromConfig(c)
		}
		return out
	case "not":
		if n.Child == nil {
			return query.AndNode{}
		}
		return query.NotNode{Child: nodeFromConfig(*n.Child)}
	}
	return query.AndNode{}
}

func nodeToConfig(n query.Node) settings.ChipNodeYAML {
	switch x := n.(type) {
	case query.CompareNode:
		return settings.ChipNodeYAML{
			Kind:  "cmp",
			Field: x.Field,
			Op:    string(x.Op),
			Value: x.Value,
		}
	case query.AndNode:
		out := settings.ChipNodeYAML{Kind: "and"}
		for _, c := range x.Children {
			out.Children = append(out.Children, nodeToConfig(c))
		}
		return out
	case query.OrNode:
		out := settings.ChipNodeYAML{Kind: "or"}
		for _, c := range x.Children {
			out.Children = append(out.Children, nodeToConfig(c))
		}
		return out
	case query.NotNode:
		c := nodeToConfig(x.Child)
		return settings.ChipNodeYAML{Kind: "not", Child: &c}
	}
	return settings.ChipNodeYAML{Kind: "and"}
}
