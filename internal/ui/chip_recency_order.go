package ui

import (
	"sort"
	"strconv"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/resource"
)

// applyVisitedListOrder wires the Recently Viewed chip's recency order
// onto a ListView. Rank-0 (most recent) lands first; items not in the
// rank map sort to the end. Installed via SetDefaultOrder so the
// column-sort path (which owns the explicit order slot) still wins
// when the user presses a sort key.
//
// `idOf` extracts the rank-map key from a row. `tag` disambiguates the
// orderKey across surfaces sharing the same kind. `gen` is the
// per-org recentGen counter — folded into the orderKey so re-MRU and
// at-cap-shift events (which don't change len(rank)) still invalidate
// the Filtered() cache.
func applyVisitedListOrder[T any](
	lv *resource.ListView[T],
	rank map[string]int,
	idOf func(T) string,
	tag string,
	gen uint64,
) {
	if lv == nil || idOf == nil {
		return
	}
	if len(rank) == 0 {
		lv.SetDefaultOrder(nil, "")
		return
	}
	key := "visited:" + tag + ":" + strconv.FormatUint(gen, 10)
	lv.SetDefaultOrder(func(items []T) []int {
		out := make([]int, len(items))
		ranks := make([]int, len(items))
		const unranked = 1 << 30
		for i := range items {
			out[i] = i
			r, ok := rank[idOf(items[i])]
			if !ok {
				ranks[i] = unranked
				continue
			}
			ranks[i] = r
		}
		sort.SliceStable(out, func(i, j int) bool {
			return ranks[out[i]] < ranks[out[j]]
		})
		return out
	}, key)
}

// clearVisitedListOrder removes any recency order installed by
// applyVisitedListOrder. Called by the regular ApplyChip /
// ApplyProjectChip branches so swapping away from the Recently Viewed
// chip returns the list to its natural source order.
func clearVisitedListOrder[T any](lv *resource.ListView[T]) {
	if lv == nil {
		return
	}
	lv.SetDefaultOrder(nil, "")
}
