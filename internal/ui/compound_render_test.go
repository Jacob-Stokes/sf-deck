package ui

import (
	"math"
	"testing"
)

// TestAddressRenderer covers the address-flattening + the gating
// heuristic (only claims when street/city/postalCode is present).
func TestAddressRenderer(t *testing.T) {
	cases := []struct {
		name    string
		in      map[string]any
		want    string
		claimed bool
	}{
		{"full address",
			map[string]any{"street": "221B Baker St", "city": "London", "state": "Greater London", "postalCode": "NW1 6XE", "country": "United Kingdom"},
			"221B Baker St, London Greater London NW1 6XE, United Kingdom", true},
		{"city + postal only",
			map[string]any{"city": "Edinburgh", "postalCode": "EH8 9YL"},
			"Edinburgh EH8 9YL", true},
		{"street only",
			map[string]any{"street": "1 Infinite Loop"},
			"1 Infinite Loop", true},
		{"multi-line street collapses",
			map[string]any{"street": "Building A\nWing 3", "city": "Cupertino"},
			"Building A, Wing 3, Cupertino", true},
		{"empty map - not an address",
			map[string]any{}, "", false},
		{"country-only - not enough to claim",
			map[string]any{"country": "France"}, "", false},
		{"non-string subfield ignored",
			map[string]any{"city": "Paris", "postalCode": 75001},
			"Paris", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := addressRenderer(c.in)
			if ok != c.claimed {
				t.Errorf("claimed = %v, want %v", ok, c.claimed)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestPersonNameRenderer covers the compound Name struct shape used by
// Contact/Lead/User. Gated on FirstName or LastName so it doesn't claim
// every map that happens to carry a Salutation field.
func TestPersonNameRenderer(t *testing.T) {
	cases := []struct {
		name    string
		in      map[string]any
		want    string
		claimed bool
	}{
		{"first + last", map[string]any{"FirstName": "Ada", "LastName": "Lovelace"},
			"Ada Lovelace", true},
		{"full set", map[string]any{
			"Salutation": "Dr.", "FirstName": "Ada", "MiddleName": "King",
			"LastName": "Lovelace", "Suffix": "II",
		}, "Dr. Ada King Lovelace II", true},
		{"last name only", map[string]any{"LastName": "Smith"}, "Smith", true},
		{"first name only", map[string]any{"FirstName": "Cher"}, "Cher", true},
		{"salutation alone is not enough",
			map[string]any{"Salutation": "Mr."}, "", false},
		{"empty map", map[string]any{}, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := personNameRenderer(c.in)
			if ok != c.claimed {
				t.Errorf("claimed = %v, want %v", ok, c.claimed)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestGeolocationRenderer covers the pure {latitude, longitude} pair.
// Strict: claims ONLY when those two are the only keys, so addresses
// (which also carry lat/lng) fall through to the address renderer.
func TestGeolocationRenderer(t *testing.T) {
	cases := []struct {
		name    string
		in      map[string]any
		want    string
		claimed bool
	}{
		{"pure geolocation",
			map[string]any{"latitude": 51.5074, "longitude": -0.1278},
			"51.5074, -0.1278", true},
		{"trims trailing zeros",
			map[string]any{"latitude": 51.5, "longitude": math.Copysign(0, -1)},
			"51.5, 0", true},
		{"missing longitude → not claimed",
			map[string]any{"latitude": 51.5}, "", false},
		{"address-shaped (extra keys) → not claimed",
			map[string]any{"latitude": 51.5, "longitude": -0.1, "city": "London"},
			"", false},
		{"non-numeric lat → not claimed",
			map[string]any{"latitude": "north", "longitude": -0.1}, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := geolocationRenderer(c.in)
			if ok != c.claimed {
				t.Errorf("claimed = %v, want %v", ok, c.claimed)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestRelationshipObjectRenderer covers the existing Name/Id shortcut.
// Lifted into the registry but the behaviour is unchanged.
func TestRelationshipObjectRenderer(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want string
	}{
		{"name wins", map[string]any{"Id": "001x", "Name": "Acme"}, "Acme"},
		{"id fallback", map[string]any{"Id": "001x"}, "001x"},
		{"empty name falls through to id", map[string]any{"Id": "001x", "Name": ""}, "001x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := relationshipObjectRenderer(c.in)
			if !ok || got != c.want {
				t.Errorf("got (%q, %v), want (%q, true)", got, ok, c.want)
			}
		})
	}
}

// TestRenderCompoundRegistryOrder covers ordering effects: an address
// must beat a pure-geolocation interpretation when both could plausibly
// match. (Geolocation is gated to {latitude, longitude} only, so this
// is also belt-and-braces but worth pinning.)
func TestRenderCompoundRegistryOrder(t *testing.T) {
	addr := map[string]any{
		"street": "1 Mission St", "city": "SF",
		"latitude": 37.79, "longitude": -122.39,
	}
	got, ok := renderCompound(addr)
	if !ok {
		t.Fatal("address with lat/lng should be claimed")
	}
	if got != "1 Mission St, SF" {
		t.Errorf("got %q, want address rendering not geolocation", got)
	}
}

// TestFormatCellEndToEnd asserts the cell formatter no longer says
// "{…}" for any of the documented compound shapes.
func TestFormatCellEndToEnd(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"address", map[string]any{"city": "London", "postalCode": "SW1A 1AA"},
			"London SW1A 1AA"},
		{"person name", map[string]any{"FirstName": "Grace", "LastName": "Hopper"},
			"Grace Hopper"},
		{"geolocation", map[string]any{"latitude": 0.0, "longitude": 0.0},
			"0, 0"},
		{"relationship name", map[string]any{"Id": "001x", "Name": "Acme"},
			"Acme"},
		{"unknown struct still {…}",
			map[string]any{"weird": "shape"}, "{…}"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatCell(c.in); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
