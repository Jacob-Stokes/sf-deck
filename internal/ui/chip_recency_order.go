package ui

import (
	"sort"
	"strconv"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/resource"
)

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

func clearVisitedListOrder[T any](lv *resource.ListView[T]) {
	if lv == nil {
		return
	}
	lv.SetDefaultOrder(nil, "")
}
