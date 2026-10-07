package soqlauto

import (
	"fmt"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// Suggest generates the ranked list of completions for the given
// classification. Dispatches per ContextKind to the matching
// generator. Callers should already have called Classify to build
// the classification.
//
// The returned slice is already rank-sorted; zero-rank entries are
// dropped. Empty slice = nothing matches (popup should close).
//
// LoadingFor is propagated from describe-walk into the
// Classification by side-effect — callers can read it from `c`
// after this call.
func Suggest(snap Snapshot, c *Classification) []Suggestion {
	switch c.Context {
	case ContextAfterFromKeyword:
		return suggestSObjects(snap, c)
	case ContextAfterSelectKeyword,
		ContextWhereField,
		ContextOrderByField,
		ContextGroupByField:
		return suggestFields(snap, c)
	case ContextWhereValue:
		return suggestValues(snap, c)
	case ContextInWithValues:
		return suggestValues(snap, c)
	case ContextTopLevel:
		return suggestKeywords(snap, c)
	case ContextNumericLiteral:
		return suggestNumericLiterals(c)
	}
	return nil
}

func suggestNumericLiterals(c *Classification) []Suggestion {
	values := []string{"10", "20", "50", "100", "200", "500", "1000", "2000"}
	out := make([]Suggestion, 0, len(values))
	for _, v := range values {
		if r := Rank(v, c.SearchToken); r > 0 {
			out = append(out, Suggestion{
				Value:   v,
				Display: v,
				Detail:  "row count",
				Kind:    KindLiteral,
				Rank:    r,
			})
		}
	}
	return SortByRank(out)
}

// suggestSObjects emits the catalog of API names + queryable
// flag. Filters by token, ranks, sorts.
func suggestSObjects(snap Snapshot, c *Classification) []Suggestion {
	out := make([]Suggestion, 0, len(snap.SObjects))
	for _, name := range snap.SObjects {
		r := Rank(name, c.SearchToken)
		if r == 0 {
			continue
		}
		out = append(out, Suggestion{
			Value:   name,
			Display: name,
			Detail:  "sObject",
			Suffix:  " ",
			Kind:    KindSObject,
			Rank:    r,
		})
	}
	return SortByRank(out)
}

func suggestFields(snap Snapshot, c *Classification) []Suggestion {
	if c.Sobject == "" {
		// No FROM yet — can't suggest fields without a target.
		// Fall back to suggesting "FROM" keyword.
		return suggestKeywords(snap, c)
	}

	hops := HopsBeforeToken(c.ContextPath)
	describes, loading := WalkHops(snap, c.Sobject, hops)
	c.LoadingFor = loading

	if c.InSubquery && len(describes) == 0 && len(loading) > 0 {
		// Best-effort: WalkHops failed because "Contacts" isn't a
		// real sObject. Find the outer FROM, resolve child relname.
		if parentName := resolveSObject(snap.Query, len(snap.Query)); parentName != "" && parentName != c.Sobject {
			parentRef := snap.Describes(parentName)
			if parentRef.Status == StatusLoaded && parentRef.Describe != nil {
				if childName := ResolveChildSObject(parentRef.Describe, c.Sobject); childName != "" {
					describes, loading = WalkHops(snap, childName, hops)
					c.LoadingFor = loading
					c.Sobject = childName
				}
			}
		}
	}

	out := make([]Suggestion, 0, 64)
	seen := map[string]bool{}
	for _, desc := range describes {
		for _, f := range desc.Fields {
			if !contextAllowsField(c.Context, f) {
				continue
			}
			if !seen[f.Name] {
				seen[f.Name] = true
				if r := Rank(f.Name, c.SearchToken); r > 0 {
					out = append(out, Suggestion{
						Value:    f.Name,
						Display:  f.Name,
						Detail:   fieldDetail(f),
						Suffix:   fieldSuffix(c.Context, hops),
						Kind:     KindField,
						DataType: f.Type,
						Rank:     r,
					})
				}
			}
			if f.RelationshipName != "" {
				relKey := f.RelationshipName + "."
				if !seen[relKey] {
					seen[relKey] = true
					if r := Rank(f.RelationshipName, c.SearchToken); r > 0 {
						out = append(out, Suggestion{
							Value:   relKey,
							Display: relKey,
							Detail:  "→ " + strings.Join(f.ReferenceTo, " | "),
							Suffix:  "",
							Kind:    KindRelationship,
							Rank:    r,
						})
					}
				}
			}
		}
	}

	if c.Context == ContextAfterSelectKeyword && len(hops) == 0 {
		for _, fn := range soqlFunctions {
			if r := Rank(fn.Value, c.SearchToken); r > 0 {
				out = append(out, Suggestion{
					Value:   fn.Value,
					Display: fn.Value,
					Detail:  fn.Detail,
					Suffix:  ", ",
					Kind:    KindFunction,
					Rank:    r,
				})
			}
		}
	}

	return SortByRank(out)
}

func suggestValues(snap Snapshot, c *Classification) []Suggestion {
	field, ok := resolveWhereField(snap, c)
	if !ok {
		return fallbackValueSuggestions(c)
	}

	out := make([]Suggestion, 0, 32)
	switch field.Type {
	case "picklist", "multipicklist":
		for _, p := range field.PicklistValues {
			if !p.Active {
				continue
			}
			val := "'" + escapeSOQL(p.Value) + "'"
			if r := Rank(p.Value, c.SearchToken); r > 0 {
				out = append(out, Suggestion{
					Value:   val,
					Display: p.Value,
					Detail:  p.Label,
					Kind:    KindPicklist,
					Rank:    r,
				})
			}
		}
	case "boolean":
		for _, b := range []string{"true", "false"} {
			if r := Rank(b, c.SearchToken); r > 0 {
				out = append(out, Suggestion{
					Value:   b,
					Display: b,
					Kind:    KindBoolean,
					Rank:    r,
				})
			}
		}
	case "date", "datetime":
		for _, lit := range soqlDateLiterals {
			if r := Rank(lit.Value, c.SearchToken); r > 0 {
				out = append(out, Suggestion{
					Value:   lit.Value,
					Display: lit.Value,
					Detail:  "SOQL date literal",
					Kind:    KindDateLiteral,
					Rank:    r,
				})
			}
		}
	case "string", "email", "phone", "url", "textarea", "reference", "id":
	}

	if field.Nillable {
		if r := Rank("null", c.SearchToken); r > 0 {
			out = append(out, Suggestion{
				Value:   "null",
				Display: "null",
				Detail:  "field is nillable",
				Kind:    KindNull,
				Rank:    r,
			})
		}
	}
	return SortByRank(out)
}

func suggestKeywords(snap Snapshot, c *Classification) []Suggestion {
	out := make([]Suggestion, 0, len(soqlKeywords)+len(snap.SObjects))
	for _, kw := range soqlKeywords {
		if r := Rank(kw.Value, c.SearchToken); r > 0 {
			out = append(out, Suggestion{
				Value:   kw.Value,
				Display: kw.Value,
				Detail:  kw.Detail,
				Suffix:  " ",
				Kind:    KindKeyword,
				Rank:    r,
			})
		}
	}
	if c.Sobject == "" {
		for _, name := range snap.SObjects {
			if r := Rank(name, c.SearchToken); r > 0 {
				out = append(out, Suggestion{
					Value:   "SELECT Id, " + NameFieldHint(name) + " FROM " + name + " LIMIT 20",
					Display: name,
					Detail:  "sObject · query it",
					Suffix:  "",
					Kind:    KindSObject,
					Rank:    r,
				})
			}
		}
	}
	return SortByRank(out)
}

// NameFieldHint is a thin re-export so suggestKeywords can pick a
// sensible Name-column for the starter query without re-importing
// sf inside this file. Falls back to "Name" for unknown sObjects.
//
// Kept as a free function (not a Snapshot method) so callers can
// override later when describes are loaded — we can swap to the
// describe's actual NameField flag.
var NameFieldHint = func(sobject string) string {
	return "Name"
}

func fallbackValueSuggestions(c *Classification) []Suggestion {
	out := []Suggestion{}
	for _, b := range []string{"null", "true", "false"} {
		if r := Rank(b, c.SearchToken); r > 0 {
			kind := KindLiteral
			switch b {
			case "null":
				kind = KindNull
			case "true", "false":
				kind = KindBoolean
			}
			out = append(out, Suggestion{
				Value: b, Display: b, Kind: kind, Rank: r,
			})
		}
	}
	return SortByRank(out)
}

func resolveWhereField(snap Snapshot, c *Classification) (sf.Field, bool) {
	if c.WhereField == "" || c.Sobject == "" {
		return sf.Field{}, false
	}
	parts := strings.Split(c.WhereField, ".")
	terminal := parts[len(parts)-1]
	hops := parts[:len(parts)-1]
	describes, _ := WalkHops(snap, c.Sobject, hops)
	for _, desc := range describes {
		for _, f := range desc.Fields {
			if strings.EqualFold(f.Name, terminal) {
				return f, true
			}
		}
	}
	return sf.Field{}, false
}

func contextAllowsField(ctx ContextKind, f sf.Field) bool {
	switch ctx {
	case ContextWhereField:
		return f.Filterable
	case ContextOrderByField:
		return f.Sortable
	case ContextGroupByField:
		return f.Groupable
	}
	return true
}

func fieldDetail(f sf.Field) string {
	if f.Label != "" && f.Label != f.Name {
		return f.Label + " · " + f.Type
	}
	return f.Type
}

func fieldSuffix(ctx ContextKind, hops []string) string {
	if len(hops) > 0 {
		return ""
	}
	switch ctx {
	case ContextAfterSelectKeyword:
		return ", "
	default:
		return " "
	}
}

// escapeSOQL escapes single-quote and backslash for safe inlining
// into a SOQL string literal.
func escapeSOQL(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

var _ = fmt.Sprintf
