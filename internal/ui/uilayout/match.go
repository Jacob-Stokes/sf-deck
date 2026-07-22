package uilayout

// Generic field-aware substring matcher for list-view searches.
//
// Each ListView[T].Match closure used to be a hand-written
// `strings.Contains(strings.ToLower(s.X), q) || ...` chain. This
// helper centralises the pattern AND adds the fielded shorthand
// (field:value) we ship on the records subtab — so every list view
// gains the same syntax without per-surface code:
//
//	`acme`            substring match across every searchable field
//	`acme inc`        AND of two substrings (each free term)
//	`name:acme`       substring scoped to the Name field
//	`label:acc inc`   "label contains acc" AND "any field contains inc"
//
// The matcher is built from a small per-row spec that names which
// fields the surface considers "searchable" and how to extract them.
// Surfaces still own the spec (different lists have different
// columns) but the parsing/AND-ing/case-folding live here.

import (
	"strings"
)

// FieldExtractor returns the lower-cased value of a named field on
// a row. Empty string means "no such field on this row" — fielded
// queries that target a missing field never match. Non-string values
// should be stringified by the caller (this matcher is text-only).
type FieldExtractor[T any] func(row T, field string) string

// AnyExtractor returns the concatenation of every searchable field on
// a row, lowercased, used for the bare-term "search any field" path.
// Surfaces typically build it by joining the same field set used by
// FieldExtractor with newlines or spaces.
type AnyExtractor[T any] func(row T) string

// MatchSpec wires the two extractors plus the field-name resolver:
// what a user-typed prefix like "lbl" should resolve to ("Label").
// The resolver is case-insensitive and supports HasPrefix matching
// so `desc:` lands on `Description` etc.
type MatchSpec[T any] struct {
	Any    AnyExtractor[T]
	Field  FieldExtractor[T]
	Fields []string // canonical field names; used for prefix-resolve
	// Primary names the field whose match should rank highest in
	// Score() — typically "Name" or whatever stable identifier the
	// list shows in column 0. When non-empty, exact / prefix /
	// token-prefix matches against this field bubble to the top of
	// search results. When empty, Score returns 1 for every match
	// (no ranking; the original behaviour).
	Primary string
}

// MakeMatcher returns a `func(T, string) bool` suitable for assigning
// directly to a ListView[T].Match. The query syntax is the records
// subtab's shape — see package doc above.
func MakeMatcher[T any](spec MatchSpec[T]) func(T, string) bool {
	return func(row T, q string) bool {
		q = strings.TrimSpace(q)
		if q == "" {
			return true
		}
		for _, term := range strings.Fields(q) {
			if !termMatches(spec, row, term) {
				return false
			}
		}
		return true
	}
}

// termMatches resolves one space-separated term against the row.
// Fielded terms (`field:value`) scope to the named field; bare
// terms scan the Any extractor.
func termMatches[T any](spec MatchSpec[T], row T, term string) bool {
	if i := strings.IndexByte(term, ':'); i > 0 && i < len(term)-1 {
		field := resolveField(spec.Fields, term[:i])
		value := strings.ToLower(term[i+1:])
		if field == "" || spec.Field == nil {
			return false
		}
		return strings.Contains(spec.Field(row, field), value)
	}
	if spec.Any == nil {
		return false
	}
	return strings.Contains(spec.Any(row), strings.ToLower(term))
}

// MakeScorer returns a `func(T, string) int` that ranks how well a
// row matches the query. Higher is better; 0 means "no ranking
// signal" (caller should fall back to original order).
//
// Bands (highest to lowest):
//
//	1000 — exact match on Primary field (Request__c == "Request__c")
//	 900 — exact match on any other indexed field (Label, etc.)
//	 800 — Primary field starts with query
//	 700 — any underscore-delimited token of Primary starts with query
//	 600 — any field's value starts with query
//	 500 — substring of Primary
//	 400 — substring of any other field
//
// Within a band the length-ratio of (query / Primary) is folded in as
// a tiebreaker — shorter Primary fields score higher because they're
// "more dominantly" the query (Request__c beats
// bt_base__SCH_Schedule_Delta_Request__c). Returns 0 when the row
// doesn't match at all (caller should have already filtered with the
// matcher; this is a "given a match, how good is it" signal).
//
// Falls back to the simple "1 if matches" behaviour when spec.Primary
// is empty — surfaces that don't care about ranking just don't set it.
func MakeScorer[T any](spec MatchSpec[T]) func(row T, q string) int {
	return func(row T, q string) int {
		q = strings.TrimSpace(strings.ToLower(q))
		if q == "" {
			return 1
		}
		if spec.Primary == "" || spec.Field == nil {
			// No ranking signal available; just confirm a match.
			if spec.Any != nil && strings.Contains(spec.Any(row), q) {
				return 1
			}
			return 0
		}

		primary := spec.Field(row, spec.Primary)
		// Length-ratio tiebreaker: 0..99. Q same length as primary
		// → 99. Q tiny relative to primary → small. Folded into
		// each band so within a band, shorter-primary rows rank
		// higher.
		tieBreak := 0
		if len(primary) > 0 {
			ratio := (len(q) * 99) / len(primary)
			if ratio > 99 {
				ratio = 99
			}
			tieBreak = ratio
		}

		// Band 1000: exact Primary match.
		if primary == q {
			return 1000 + tieBreak
		}
		// Band 900: exact match on any other indexed field.
		for _, f := range spec.Fields {
			if f == spec.Primary {
				continue
			}
			if v := spec.Field(row, f); v == q {
				return 900 + tieBreak
			}
		}
		// Band 800: Primary starts with query.
		if strings.HasPrefix(primary, q) {
			return 800 + tieBreak
		}
		// Band 700: a `_`-delimited token of Primary starts with
		// query (handles "Request" → Request__c because
		// underscores split into Request | c).
		if tokenStartsWith(primary, q) {
			return 700 + tieBreak
		}
		// Band 600: any other field starts with query.
		for _, f := range spec.Fields {
			if f == spec.Primary {
				continue
			}
			if strings.HasPrefix(spec.Field(row, f), q) {
				return 600 + tieBreak
			}
		}
		// Band 500: substring of Primary.
		if strings.Contains(primary, q) {
			return 500 + tieBreak
		}
		// Band 400: any field has query as substring (the original
		// behaviour, last-resort).
		if spec.Any != nil && strings.Contains(spec.Any(row), q) {
			return 400 + tieBreak
		}
		return 0
	}
}

// tokenStartsWith reports whether any `_`-delimited token of s
// starts with prefix. Helps "Request" rank Request__c high since
// "Request" is a token (not just a substring) of the name.
func tokenStartsWith(s, prefix string) bool {
	if prefix == "" {
		return false
	}
	for _, tok := range strings.Split(s, "_") {
		if strings.HasPrefix(tok, prefix) {
			return true
		}
	}
	return false
}

// resolveField maps a user-typed prefix (e.g. "lbl") to its canonical
// field name from the spec's list ("Label"). Exact case-insensitive
// match wins; falls back to a case-insensitive HasPrefix match. Empty
// when no field qualifies.
func resolveField(fields []string, prefix string) string {
	lp := strings.ToLower(prefix)
	for _, f := range fields {
		if strings.EqualFold(f, prefix) {
			return f
		}
	}
	for _, f := range fields {
		if strings.HasPrefix(strings.ToLower(f), lp) {
			return f
		}
	}
	return ""
}
