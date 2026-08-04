package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

// TestActiveUsersChipPredicates verifies the built-in session lenses
// filter correctly through the real query.Eval path — the same code
// the chip strip uses at runtime. (The numeric "recent" lens is
// covered in the sf package, where freshnessMinutes is constructable.)
func TestActiveUsersChipPredicates(t *testing.T) {
	byID := map[string]qchip.Chip{}
	for _, c := range qchip.ActiveUsersBuiltins {
		byID[c.ID] = c
	}
	match := func(id string, r sf.ActiveUserRow) bool {
		return query.Eval(byID[id].Query.Where, r)
	}

	if !match("nomfa", sf.ActiveUserRow{AnyLowMFA: true}) {
		t.Error("nomfa should match a LOW-security row")
	}
	if match("nomfa", sf.ActiveUserRow{AnyLowMFA: false}) {
		t.Error("nomfa should not match a high-assurance row")
	}
	if !match("api", sf.ActiveUserRow{IsAPI: true}) {
		t.Error("api should match an integration row")
	}
	if match("api", sf.ActiveUserRow{IsAPI: false}) {
		t.Error("api should not match a UI row")
	}
	// "all" has an empty Where — matches everything.
	if !match("all", sf.ActiveUserRow{}) {
		t.Error("all should match any row")
	}
}
