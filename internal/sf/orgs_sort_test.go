package sf

import "testing"

// TestOrgSortKey covers the alphabetical-stable sort key. Sort is
// case-insensitive so "ACME-PROD" and "acme-test" cluster, and alias
// beats username when present so the display name drives the rail.
func TestOrgSortKey(t *testing.T) {
	cases := []struct {
		name string
		org  Org
		want string
	}{
		{"alias wins over username",
			Org{Alias: "ACME-PROD", Username: "j.foo@x"}, "acme-prod"},
		{"username used when no alias",
			Org{Username: "j.foo@x"}, "j.foo@x"},
		{"casing normalised",
			Org{Alias: "Phd"}, "phd"},
		{"empty",
			Org{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := orgSortKey(c.org); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}
