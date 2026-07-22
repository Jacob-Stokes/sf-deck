package usage

import "testing"

// TestTagOf_FiltersNoise locks in the noise filter rules so the API
// audit modal keeps showing the right attribution. The actual
// stack-walking in captureCaller is essentially "find first frame
// where tagOf != """ — so testing tagOf in isolation covers the
// interesting logic. The walk itself is exercised end-to-end any time
// the binary makes an API call.
func TestTagOf_FiltersNoise(t *testing.T) {
	cases := []struct {
		in   string
		want string // "" means filtered out as noise
	}{
		// Standard-library noise — always filtered.
		{"runtime.goexit", ""},
		{"net/http.(*Client).Do", ""},
		{"testing.tRunner", ""},

		// main package: the OnCall closure lives in cmd/sf-deck/main.go
		// as "main.main.func1". Filtering "main." surfaces the real
		// fetcher above it.
		{"main.main.func1", ""},
		{"main.main", ""},

		// usage package self-frames — always filtered.
		{"github.com/Jacob-Stokes/sf-deck/internal/usage.(*Tracker).Bump", ""},
		{"github.com/Jacob-Stokes/sf-deck/internal/usage.captureCaller", ""},

		// sf REST transport plumbing — filtered, since these are the
		// generic verb helpers every call passes through.
		{"github.com/Jacob-Stokes/sf-deck/internal/sf.(*Client).doOnce", ""},
		{"github.com/Jacob-Stokes/sf-deck/internal/sf.(*Client).get", ""},
		{"github.com/Jacob-Stokes/sf-deck/internal/sf.(*Client).post", ""},
		{"github.com/Jacob-Stokes/sf-deck/internal/sf.QueryREST", ""},
		{"github.com/Jacob-Stokes/sf-deck/internal/sf.fireOnCall", ""},

		// Real fetchers / UI helpers — survive as attribution tags.
		{"github.com/Jacob-Stokes/sf-deck/internal/sf.fetchHome", "sf.fetchHome"},
		{"github.com/Jacob-Stokes/sf-deck/internal/sf.fetchFlowVersions", "sf.fetchFlowVersions"},
		{"github.com/Jacob-Stokes/sf-deck/internal/ui.ensureActiveUsersChip", "ui.ensureActiveUsersChip"},
		{"github.com/Jacob-Stokes/sf-deck/internal/ui.(*Model).Update", "ui.Model.Update"},
	}
	for _, c := range cases {
		got := tagOf(c.in)
		if got != c.want {
			t.Errorf("tagOf(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
