package sf

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestDescribe_Singleflight confirms two concurrent Describe calls for
// the same (alias, sobject) share one underlying REST call rather than
// firing two. Regression guard for the duplicate-describe bug that
// turned up in the api-trace JSONL: EnsureDescribe and
// EnsureCustomObjectBaseline both call Describe on object drill-in.
func TestDescribe_Singleflight(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		// Simulate ~50ms describe latency so the two goroutines
		// actually overlap.
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"Account","label":"Account","fields":[]}`))
	}))
	defer srv.Close()

	// Pre-populate a Client so Describe's REST-path branch fires
	// directly without going through RESTClient bootstrap. Need to
	// reach the singleflight inside Describe, not test RESTClient.
	clientsMu.Lock()
	clients["test-alias"] = &clientEntry{
		client: &Client{
			alias:       "test-alias",
			accessToken: "token",
			instanceURL: srv.URL,
			apiVersion:  "62.0",
			http:        srv.Client(),
		},
	}
	// Mark the once as done so RESTClient returns immediately.
	clients["test-alias"].once.Do(func() {})
	clientsMu.Unlock()
	defer func() {
		clientsMu.Lock()
		delete(clients, "test-alias")
		clientsMu.Unlock()
	}()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := Describe("test-alias", "Account")
			if err != nil {
				t.Errorf("Describe: %v", err)
			}
		}()
	}
	wg.Wait()

	got := atomic.LoadInt32(&calls)
	if got != 1 {
		t.Errorf("expected 1 underlying REST describe call, got %d", got)
	}
}

// TestRESTClient_Singleflight confirms two concurrent RESTClient calls
// for the same alias share a single bootstrap. Regression guard for
// the duplicate `sf org display` bug observed in the api-trace.
//
// Note: this test can't actually run `sf org display`, so we verify
// the singleflight machinery's structure via the public surface — two
// concurrent calls return the same *Client pointer, with only one
// entry in the registry.
func TestRESTClient_SingleEntryPerAlias(t *testing.T) {
	// Reset registry so this test is isolated.
	InvalidateRESTClients()

	// Pre-seed an entry whose once has already fired with a valid
	// client. Two concurrent RESTClient calls should both receive it.
	expected := &Client{alias: "smoke", accessToken: "t", instanceURL: "http://x", apiVersion: "62.0"}
	clientsMu.Lock()
	entry := &clientEntry{client: expected}
	entry.once.Do(func() {}) // pre-fire so RESTClient skips bootstrap
	clients["smoke"] = entry
	clientsMu.Unlock()
	defer InvalidateRESTClients()

	var wg sync.WaitGroup
	results := make([]*Client, 4)
	errs := make([]error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = RESTClient("smoke")
		}(i)
	}
	wg.Wait()
	for i, c := range results {
		if errs[i] != nil {
			t.Fatalf("call %d: %v", i, errs[i])
		}
		if c != expected {
			t.Errorf("call %d: got client %p, want %p", i, c, expected)
		}
	}

	clientsMu.Lock()
	defer clientsMu.Unlock()
	if len(clients) != 1 {
		t.Errorf("registry has %d entries, want 1", len(clients))
	}
}
