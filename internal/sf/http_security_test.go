package sf

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAuthenticatedHTTPClientRejectsUnsafeInstanceURLs(t *testing.T) {
	for _, raw := range []string{
		"http://example.test",
		"https://user:password@example.test",
		"https://example.test/services/data",
		"https://example.test?target=other",
		"https://example.test#fragment",
		"not-a-url",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := authenticatedHTTPClient(&http.Client{}, raw); err == nil {
				t.Fatalf("unsafe instance URL accepted: %q", raw)
			}
		})
	}
	if _, err := authenticatedHTTPClient(&http.Client{}, "https://example.test/"); err != nil {
		t.Fatalf("valid instance URL rejected: %v", err)
	}
}

func TestAuthenticatedHTTPClientRefusesCrossOriginRedirectWithBody(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL+"/capture")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	client, err := authenticatedHTTPClient(origin.Client(), origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, origin.URL+"/soap", strings.NewReader("SESSION_TOKEN"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(req)
	if err == nil || !strings.Contains(err.Error(), "refusing authenticated redirect") {
		t.Fatalf("cross-origin redirect err = %v", err)
	}
	if got := targetCalls.Load(); got != 0 {
		t.Fatalf("redirect target received %d requests", got)
	}
}

func TestAuthenticatedHTTPClientAllowsSameOriginRedirect(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/done", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := authenticatedHTTPClient(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(server.URL + "/start")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
