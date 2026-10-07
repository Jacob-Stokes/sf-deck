package sf

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRESTUsageTracksRequestErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`[{"message":"boom","errorCode":"SERVER_ERROR"}]`))
	}))
	defer server.Close()

	old := OnCall
	defer func() { OnCall = old }()

	var gotErr error
	var gotDur time.Duration
	OnCall = func(alias string, args []string, err error, dur time.Duration) {
		gotErr = err
		gotDur = dur
	}

	c := &Client{
		alias:       "dev",
		accessToken: "token",
		instanceURL: server.URL,
		apiVersion:  "62.0",
		http:        server.Client(),
	}
	if _, err := c.doOnce("GET", "/services/data/v62.0/limits", nil, nil); err == nil {
		t.Fatal("doOnce returned nil error")
	}
	if gotErr == nil {
		t.Fatal("usage hook saw nil error for failed REST request")
	}
	if gotDur <= 0 {
		t.Fatalf("usage hook saw zero/negative duration: %v", gotDur)
	}
}
