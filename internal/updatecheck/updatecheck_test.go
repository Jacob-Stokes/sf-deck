package updatecheck

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestEvaluateUpdateKinds(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name, current, latest, kind string
		available                   bool
	}{
		{"same", "0.1.0", "v0.1.0", "", false},
		{"older latest", "v0.2.0", "v0.1.9", "", false},
		{"patch", "v0.1.0", "v0.1.1", "patch", true},
		{"minor", "v0.1.9", "v0.2.0", "minor", true},
		{"major", "v0.9.9", "v1.0.0", "major", true},
		{"stable after prerelease", "v0.2.0-beta.1", "v0.2.0", "patch", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evaluate(tc.current, release{TagName: tc.latest}, now, false)
			if err != nil {
				t.Fatal(err)
			}
			if got.UpdateAvailable != tc.available || got.Kind != tc.kind {
				t.Fatalf("got available=%v kind=%q", got.UpdateAvailable, got.Kind)
			}
		})
	}
}

func TestCheckerCachesForTwentyFourHours(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if got := r.Header.Get("User-Agent"); got != "sf-deck-update-check" {
			t.Errorf("User-Agent = %q", got)
		}
		fmt.Fprint(w, `{"tag_name":"v0.2.0","html_url":"https://example.test/v0.2.0","published_at":"2026-07-23T10:00:00Z"}`)
	}))
	defer srv.Close()

	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	statePath := filepath.Join(t.TempDir(), "update-state.json")
	checker := &Checker{
		Client:    srv.Client(),
		URL:       srv.URL,
		StatePath: statePath,
		Now:       func() time.Time { return now },
	}
	first, err := checker.Check(context.Background(), "v0.1.0", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !first.UpdateAvailable || first.FromCache {
		t.Fatalf("first = %+v", first)
	}
	second, err := checker.Check(context.Background(), "v0.1.0", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !second.FromCache || calls.Load() != 1 {
		t.Fatalf("second cache=%v calls=%d", second.FromCache, calls.Load())
	}
	if fi, err := os.Stat(statePath); err != nil {
		t.Fatal(err)
	} else if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("state mode = %o", got)
	}
	_, err = checker.Check(context.Background(), "v0.1.0", Options{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("forced calls=%d", calls.Load())
	}
}

func TestCheckerNoStableReleaseAndDevelopmentBuild(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	checker := &Checker{
		Client:    srv.Client(),
		URL:       srv.URL,
		StatePath: filepath.Join(t.TempDir(), "state.json"),
	}
	got, err := checker.Check(context.Background(), "dev", Options{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if !got.DevelopmentBuild || !got.NoStableRelease || got.UpdateAvailable {
		t.Fatalf("got %+v", got)
	}
}

func TestCheckerIgnoresPrereleaseDefensively(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v0.2.0-beta.1","prerelease":true}`)
	}))
	defer srv.Close()
	checker := &Checker{
		Client:    srv.Client(),
		URL:       srv.URL,
		StatePath: filepath.Join(t.TempDir(), "state.json"),
	}
	got, err := checker.Check(context.Background(), "v0.1.0", Options{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if !got.NoStableRelease || got.UpdateAvailable {
		t.Fatalf("got %+v", got)
	}
}

func TestCheckerRejectsMalformedLatestVersion(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"tag_name":"release-next"}`)
	}))
	defer srv.Close()
	checker := &Checker{
		Client:    srv.Client(),
		URL:       srv.URL,
		StatePath: filepath.Join(t.TempDir(), "state.json"),
	}
	if _, err := checker.Check(context.Background(), "v0.1.0", Options{Force: true}); err == nil {
		t.Fatal("expected malformed version error")
	}
}
