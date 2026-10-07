package qchip

import (
	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

// FromConfig rehydrates a persisted ChipConfig into a runtime Chip.
// Used by the load path on startup + after every settings.Save.
func FromConfig(c settings.ChipConfig) Chip {
	origin := OriginUser
	switch c.Origin {
	case "imported":
		origin = OriginImported
	case "user", "":
		origin = OriginUser
	}
	return Chip{
		ID:         c.ID,
		Label:      c.Label,
		Scope:      c.Scope,
		Origin:     origin,
		Query:      QueryFromConfig(c.Query),
		SourceID:   c.SourceID,
		SourceName: c.SourceName,
		ImportedAt: c.ImportedAt,
		Favourite:  c.Favourite,
		// OrgUser is preserved verbatim for the legacy persistence shape;
		// runtime filtering goes through Share, which migrates OrgUser
		// when Share is empty (EffectiveShare on the config side).
		OrgUser: c.OrgUser,
		Share:   c.EffectiveShare(),
	}
}

// ToConfig is the reverse — used when persisting a runtime chip.
// Domain is supplied separately because the runtime type is
// domain-agnostic (the same Chip might apply to records or flows
// depending on Scope), but the on-disk form needs to be sectioned
// for selective load.
func ToConfig(c Chip, domain string) settings.ChipConfig {
	originStr := "user"
	if c.Origin == OriginImported {
		originStr = "imported"
	}
	out := settings.ChipConfig{
		ID:         c.ID,
		Label:      c.Label,
		Scope:      c.Scope,
		Domain:     domain,
		Origin:     originStr,
		Query:      QueryToConfig(c.Query),
		SourceID:   c.SourceID,
		SourceName: c.SourceName,
		ImportedAt: c.ImportedAt,
		Favourite:  c.Favourite,
		OrgUser:    c.OrgUser, // preserved in case caller hasn't yet migrated
		Share:      c.Share,
	}
	// NormaliseShare collapses legacy OrgUser into Share, so what we write
	// is always the modern shape regardless of which fields the runtime
	// chip carried.
	out.NormaliseShare()
	return out
}

// QueryFromConfig / QueryToConfig are the settings.ChipQueryYAML ↔
// query.Query conversions. We can't import internal/query from the
// settings package (would couple persistence to the UI tree), so the
// translation lives here in qchip — it sees both packages.
//
// Exported so the migrate subpackage (internal/ui/qchip/migrate) can
// call these without re-implementing the AST conversion.
func QueryFromConfig(qy settings.ChipQueryYAML) query.Query {
	out := query.Query{
		Limit:   qy.Limit,
		Columns: append([]string(nil), qy.Columns...),
	}
	if qy.Where != nil {
		out.Where = NodeFromConfig(*qy.Where)
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

// QueryToConfig is the reverse.
func QueryToConfig(q query.Query) settings.ChipQueryYAML {
	out := settings.ChipQueryYAML{
		Limit:   q.Limit,
		Columns: append([]string(nil), q.Columns...),
	}
	if q.Where != nil {
		w := NodeToConfig(q.Where)
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

func NodeFromConfig(n settings.ChipNodeYAML) query.Node {
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
			out.Children[i] = NodeFromConfig(c)
		}
		return out
	case "or":
		out := query.OrNode{Children: make([]query.Node, len(n.Children))}
		for i, c := range n.Children {
			out.Children[i] = NodeFromConfig(c)
		}
		return out
	case "not":
		if n.Child == nil {
			return query.AndNode{}
		}
		return query.NotNode{Child: NodeFromConfig(*n.Child)}
	}
	return query.AndNode{}
}

func NodeToConfig(n query.Node) settings.ChipNodeYAML {
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
			out.Children = append(out.Children, NodeToConfig(c))
		}
		return out
	case query.OrNode:
		out := settings.ChipNodeYAML{Kind: "or"}
		for _, c := range x.Children {
			out.Children = append(out.Children, NodeToConfig(c))
		}
		return out
	case query.NotNode:
		c := NodeToConfig(x.Child)
		return settings.ChipNodeYAML{Kind: "not", Child: &c}
	}
	return settings.ChipNodeYAML{Kind: "and"}
}
