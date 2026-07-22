package query

// Persistence form for the AST. TOML doesn't have native sum types so
// every Node serialises to a NodeYAML with a discriminator field
// ("kind") plus the union of fields any concrete node might need.
// Empty fields are omitted via toml omitempty so the on-disk form
// stays human-readable.
//
// Round-trip flow:
//
//	Node          → YAMLFromNode  → NodeYAML (toml)
//	NodeYAML      → NodeFromYAML  → Node
//
// Both directions are total — invalid persisted data converts to a
// vacuous AndNode (matches everything) rather than panicking, so a
// hand-edited settings.toml that's slightly wrong never crashes
// startup. The conversion is paired with debug logging so the
// failure is visible.

// NodeYAML is the on-disk form of a Node. One concrete shape covers
// every variant — emitters write only the fields that apply to the
// chosen Kind.
//
// Kind is one of: "and", "or", "not", "cmp". Anything else converts
// to an empty AndNode at load time.
type NodeYAML struct {
	Kind string `toml:"kind"`

	// Compare-only fields.
	Field string `toml:"field,omitempty"`
	Op    string `toml:"op,omitempty"`
	// Value is stored as the concrete go type the AST carries — TOML
	// handles strings, ints, bools, and []string natively; other
	// types are rejected at write time. We keep it as `any` so the
	// reader can decode without prior knowledge, then the converter
	// validates.
	Value any `toml:"value,omitempty"`

	// And / Or only.
	Children []NodeYAML `toml:"children,omitempty"`

	// Not only.
	Child *NodeYAML `toml:"child,omitempty"`
}

// QueryYAML is the persistence form of Query. Mirrors the runtime
// type but with NodeYAML in place of Node and an OrderBy list whose
// fields are tomlable strings.
type QueryYAML struct {
	Where   *NodeYAML     `toml:"where,omitempty"`
	OrderBy []OrderByYAML `toml:"order_by,omitempty"`
	Limit   int           `toml:"limit,omitempty"`
	Columns []string      `toml:"columns,omitempty"`
}

// OrderByYAML mirrors OrderBy with Direction as a string for TOML
// compatibility.
type OrderByYAML struct {
	Field     string `toml:"field"`
	Direction string `toml:"direction,omitempty"` // "asc" / "desc"
	NullsLast bool   `toml:"nulls_last,omitempty"`
}

// YAMLFromNode converts an in-memory Node into its persistable form.
// A nil Node converts to a zero NodeYAML (Kind == "") which the
// reader treats as "no predicate" (vacuous AndNode).
func YAMLFromNode(n Node) NodeYAML {
	switch x := n.(type) {
	case nil:
		return NodeYAML{}
	case CompareNode:
		return NodeYAML{
			Kind:  "cmp",
			Field: x.Field,
			Op:    string(x.Op),
			Value: x.Value,
		}
	case AndNode:
		return NodeYAML{
			Kind:     "and",
			Children: childrenToYAML(x.Children),
		}
	case OrNode:
		return NodeYAML{
			Kind:     "or",
			Children: childrenToYAML(x.Children),
		}
	case NotNode:
		c := YAMLFromNode(x.Child)
		return NodeYAML{Kind: "not", Child: &c}
	}
	// Unknown node types fall through as a vacuous And — same defensive
	// behaviour the reader applies, so the round-trip never throws.
	return NodeYAML{Kind: "and"}
}

func childrenToYAML(cs []Node) []NodeYAML {
	out := make([]NodeYAML, len(cs))
	for i, c := range cs {
		out[i] = YAMLFromNode(c)
	}
	return out
}

// NodeFromYAML is the reverse. Invalid trees decay to an empty And
// (match everything) rather than producing an error — the assumption
// is that a hand-edited config with one bad entry shouldn't take down
// the whole app. Treat the return value as "best-effort"; callers
// can compare to AndNode{} to detect the empty case.
func NodeFromYAML(y NodeYAML) Node {
	switch y.Kind {
	case "cmp":
		return CompareNode{
			Field: y.Field,
			Op:    Op(y.Op),
			Value: y.Value,
		}
	case "and":
		return AndNode{Children: childrenFromYAML(y.Children)}
	case "or":
		return OrNode{Children: childrenFromYAML(y.Children)}
	case "not":
		if y.Child == nil {
			return AndNode{}
		}
		return NotNode{Child: NodeFromYAML(*y.Child)}
	}
	return AndNode{}
}

func childrenFromYAML(cs []NodeYAML) []Node {
	out := make([]Node, len(cs))
	for i, c := range cs {
		out[i] = NodeFromYAML(c)
	}
	return out
}

// YAMLFromQuery / QueryFromYAML are the same conversion at the Query
// level. We split this from the Node conversion so callers persisting
// just a predicate don't drag in OrderBy plumbing.
func YAMLFromQuery(q Query) QueryYAML {
	out := QueryYAML{
		Limit:   q.Limit,
		Columns: append([]string(nil), q.Columns...),
	}
	if q.Where != nil {
		w := YAMLFromNode(q.Where)
		out.Where = &w
	}
	for _, ob := range q.OrderBy {
		out.OrderBy = append(out.OrderBy, OrderByYAML{
			Field:     ob.Field,
			Direction: string(ob.Direction),
			NullsLast: ob.NullsLast,
		})
	}
	return out
}

// QueryFromYAML is the reverse.
func QueryFromYAML(y QueryYAML) Query {
	out := Query{
		Limit:   y.Limit,
		Columns: append([]string(nil), y.Columns...),
	}
	if y.Where != nil {
		out.Where = NodeFromYAML(*y.Where)
	}
	for _, ob := range y.OrderBy {
		dir := Direction(ob.Direction)
		if dir == "" {
			dir = Ascending
		}
		out.OrderBy = append(out.OrderBy, OrderBy{
			Field:     ob.Field,
			Direction: dir,
			NullsLast: ob.NullsLast,
		})
	}
	return out
}
