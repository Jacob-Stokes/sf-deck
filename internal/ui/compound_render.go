package ui

// Cell-rendering for Salesforce compound + relationship struct values.

import (
	"strconv"
	"strings"
)

type compoundRenderer func(map[string]any) (string, bool)

// compoundRenderers is the ordered registry. Order matters when two
// shapes overlap (e.g. Geolocation's {latitude, longitude} is a subset
// of an Address's subfields — Address must win, so it goes first).
//
// Adding a new shape: write a recogniser, append it. Keep the more
// specific shapes earlier in the slice.
var compoundRenderers = []compoundRenderer{
	relationshipObjectRenderer, // {Name} → name, or {Id} → id
	addressRenderer,            // street/city/postalCode → flattened line
	personNameRenderer,         // FirstName+LastName → "Salutation First Mid Last Suffix"
	geolocationRenderer,        // {latitude, longitude} only → "lat, lng"
}

func renderCompound(x map[string]any) (string, bool) {
	for _, r := range compoundRenderers {
		if s, ok := r(x); ok {
			return s, true
		}
	}
	return "", false
}

func relationshipObjectRenderer(x map[string]any) (string, bool) {
	if name, ok := x["Name"].(string); ok && name != "" {
		return name, true
	}
	if id, ok := x["Id"].(string); ok && id != "" {
		return id, true
	}
	return "", false
}

// addressRenderer flattens a compound address into one readable line
// like "Street, City State Postal, Country" (skipping empty parts,
// collapsing newlines in multi-line street fields). Salesforce REST
// returns address subfields lowercase, so the keys match exactly.
//
// Heuristic gate: requires at least one of street / city / postalCode
// so we don't address-stringify any unfamiliar struct. country-only
// or state-only structs aren't "addresses enough" to claim.
func addressRenderer(x map[string]any) (string, bool) {
	street := mapStr(x, "street")
	city := mapStr(x, "city")
	state := mapStr(x, "state")
	postal := mapStr(x, "postalCode")
	country := mapStr(x, "country")
	if street == "" && city == "" && postal == "" {
		return "", false
	}
	var parts []string
	if street != "" {
		parts = append(parts, strings.ReplaceAll(street, "\n", ", "))
	}
	cityLine := city
	if state != "" {
		if cityLine != "" {
			cityLine += " " + state
		} else {
			cityLine = state
		}
	}
	if postal != "" {
		if cityLine != "" {
			cityLine += " " + postal
		} else {
			cityLine = postal
		}
	}
	if cityLine != "" {
		parts = append(parts, cityLine)
	}
	if country != "" {
		parts = append(parts, country)
	}
	return strings.Join(parts, ", "), true
}

// personNameRenderer flattens the compound Name struct returned for
// Contact/Lead/User: {Salutation, FirstName, MiddleName, LastName, Suffix}.
// Note: most objects' Name field is a plain string and never reaches
// this renderer.
//
// Gate: requires at least one of FirstName / LastName so we don't claim
// every map that happens to have a Salutation field.
func personNameRenderer(x map[string]any) (string, bool) {
	first := mapStr(x, "FirstName")
	last := mapStr(x, "LastName")
	if first == "" && last == "" {
		return "", false
	}
	var parts []string
	for _, k := range [...]string{"Salutation", "FirstName", "MiddleName", "LastName", "Suffix"} {
		if v := mapStr(x, k); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " "), true
}

func geolocationRenderer(x map[string]any) (string, bool) {
	lat, latOK := mapFloat(x, "latitude")
	lng, lngOK := mapFloat(x, "longitude")
	if !latOK || !lngOK {
		return "", false
	}
	// Be strict: claim only when this is JUST a geolocation, nothing
	// address-shaped alongside. Otherwise addressRenderer should have
	// won upstream, but the order-of-registration already ensures that
	// — this gate guards against odd custom shapes.
	for k := range x {
		switch k {
		case "latitude", "longitude":
		default:
			return "", false
		}
	}
	return formatLatLng(lat, lng), true
}

func mapStr(x map[string]any, key string) string {
	if v, ok := x[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func mapFloat(x map[string]any, key string) (float64, bool) {
	v, ok := x[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	}
	return 0, false
}

func formatLatLng(lat, lng float64) string {
	return trimFloat(lat) + ", " + trimFloat(lng)
}

func trimFloat(f float64) string {
	if f == 0 {
		f = 0 // normalize negative zero so a coordinate of -0.0 renders as "0"
	}
	s := strconv.FormatFloat(f, 'f', 4, 64)
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimRight(s, ".")
}
