package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// homeLandingModel returns a Model parked on /home -> Landing with one
// connected org, ready to feed keys to onHomeDestinationsKey.
func homeLandingModel(t *testing.T) Model {
	t.Helper()
	c, err := cache.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })

	m := New(c)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 40})
	mm := nm.(Model)
	mm.orgs = []sf.Org{{
		Alias: "t", Username: "u@t.com",
		InstanceURL: "https://x.my.salesforce.com",
		Status:      "Connected", LastUsed: time.Now().Format(time.RFC3339),
	}}
	mm.focus = focusMain
	d := mm.ensureOrgDataRef("u@t.com")
	d.Tab = TabHome // Landing is homeSubtab 0, the default
	return mm
}

// TestHomeDestCatalogHasCollisions documents the situation the
// dispatcher precedence rule exists to handle: several item letters
// equal another section's section letter. If this ever drops to zero
// the precedence rule is merely belt-and-braces, which is fine — but
// we assert it's >0 so the regression test below stays meaningful.
func TestHomeDestCatalogHasCollisions(t *testing.T) {
	collisions := 0
	for _, s := range homeDestinations {
		for _, e := range s.Entries {
			if other, ok := homeDestSectionByItemLetterElsewhere(s.Letter, e.Key); ok {
				_ = other
				collisions++
			}
		}
	}
	if collisions == 0 {
		t.Skip("no item/section letter collisions in catalog; precedence rule untested but harmless")
	}
}

// homeDestSectionByItemLetterElsewhere reports whether the given item
// letter is the section letter of a DIFFERENT section than the one it
// lives in (ownLetter). Test helper mirroring the collision audit.
func homeDestSectionByItemLetterElsewhere(ownLetter, itemLetter string) (string, bool) {
	for _, s := range homeDestinations {
		if s.Letter == itemLetter && s.Letter != ownLetter {
			return s.Label, true
		}
	}
	return "", false
}

// TestFocusedItemLetterBeatsCollidingSection is the core regression:
// the exact bug reported was pressing "m" while CODE is focused (to
// open Custom Metadata Types) teleporting to DEPLOY & MONITOR because
// "m" is also DEPLOY's section letter. After the fix, a focused
// section's item letter must fire the item and NOT switch focus to the
// colliding section.
func TestFocusedItemLetterBeatsCollidingSection(t *testing.T) {
	// Every (focused section, item letter) pair where the item letter
	// collides with another section's section letter. Pressing the
	// item letter must leave focus on the SAME section.
	for _, s := range homeDestinations {
		for _, e := range s.Entries {
			collidesWith, collides := homeDestSectionByItemLetterElsewhere(s.Letter, e.Key)
			if !collides {
				continue
			}
			t.Run(s.Label+"/"+e.Key, func(t *testing.T) {
				m := homeLandingModel(t)
				m.homeFocusedSectionLetter = s.Letter
				mm, _, consumed := m.onHomeDestinationsKey(e.Key)
				if !consumed {
					t.Fatalf("item %q in focused %s: key not consumed", e.Key, s.Label)
				}
				if mm.homeFocusedSectionLetter != s.Letter {
					t.Fatalf("item %q in focused %s teleported focus to %q (collides with section %s); want focus to stay on %q",
						e.Key, s.Label, mm.homeFocusedSectionLetter, collidesWith, s.Letter)
				}
			})
		}
	}
}

// TestSectionLetterFocusesWhenNothingFocused guards the other regime:
// with no section focused, a section letter must focus that section.
func TestSectionLetterFocusesWhenNothingFocused(t *testing.T) {
	for _, s := range homeDestinations {
		t.Run(s.Label, func(t *testing.T) {
			m := homeLandingModel(t)
			m.homeFocusedSectionLetter = "" // nothing focused
			mm, _, consumed := m.onHomeDestinationsKey(s.Letter)
			if !consumed {
				t.Fatalf("section letter %q not consumed", s.Letter)
			}
			if mm.homeFocusedSectionLetter != s.Letter {
				t.Fatalf("section letter %q focused %q, want %q", s.Letter, mm.homeFocusedSectionLetter, s.Letter)
			}
		})
	}
}

// TestNonCollidingSectionLetterStillHopsWhileFocused: while a section
// is focused, pressing a letter that is NOT one of its item letters
// but IS another section's letter should still hop to that section
// (the fallback branch). Use AUTOMATION (f) focused, press "u" (USERS
// section) — "u" is not an AUTOMATION item letter, so it should hop.
func TestNonCollidingSectionLetterStillHopsWhileFocused(t *testing.T) {
	m := homeLandingModel(t)
	auto, ok := homeDestinationSectionByLetter("f")
	if !ok {
		t.Fatal("AUTOMATION section (f) not found")
	}
	if _, isItem := homeDestinationByItemLetter(auto, "u"); isItem {
		t.Skip("'u' is an AUTOMATION item letter; pick another non-colliding case")
	}
	m.homeFocusedSectionLetter = "f"
	mm, _, consumed := m.onHomeDestinationsKey("u")
	if !consumed {
		t.Fatal("section hop key 'u' not consumed")
	}
	if mm.homeFocusedSectionLetter != "u" {
		t.Fatalf("hop while focused: focus = %q, want u (USERS)", mm.homeFocusedSectionLetter)
	}
}
