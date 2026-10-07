package soqlauto

import (
	"sort"
	"strings"
)

// Rank scores a candidate value against the search token. Higher is
// better. Bucket order matches Inspector Reloaded's sortRank:
//
//	4  exact case-insensitive match
//	3  startsWith
//	2  __c custom-suffix bucket (value ends in __c AND prefix matches token)
//	1  substring
//	0  no match
//
// Bucket 2 is only meaningfully different from bucket 3 when the
// token doesn't anchor at the start of the value; in practice it
// surfaces "Active__c" higher than "MyActiveFlag" when the user
// typed "Active" but neither startsWith.
func Rank(value, token string) int {
	if token == "" {
		// No token → everything ranks equal. Caller's stable sort
		// will fall back to alphabetical via Display.
		return 1
	}
	v := strings.ToLower(value)
	t := strings.ToLower(token)
	switch {
	case v == t:
		return 4
	case strings.HasPrefix(v, t):
		return 3
	case strings.HasSuffix(v, "__c") && strings.Contains(v, t):
		return 2
	case strings.Contains(v, t):
		return 1
	}
	return 0
}

// SortByRank stable-sorts suggestions by Rank descending, then by
// Display ascending. Drops any zero-rank entries.
func SortByRank(suggestions []Suggestion) []Suggestion {
	out := suggestions[:0]
	for _, s := range suggestions {
		if s.Rank > 0 {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Rank != out[j].Rank {
			return out[i].Rank > out[j].Rank
		}
		return out[i].Display < out[j].Display
	})
	return out
}
