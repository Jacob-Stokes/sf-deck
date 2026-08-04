package sf

import (
	"errors"
	"strings"
	"testing"
)

// R1: the rate-limit / daily-limit classifiers drive whether a failed
// REST call is retried (429) and how a daily-limit error is surfaced.
func TestRateLimitClassifiers(t *testing.T) {
	rate := &sfHTTPError{Status: 429, Body: []byte(`[{"message":"too many"}]`)}
	daily := &sfHTTPError{Status: 403, Body: []byte(`[{"errorCode":"REQUEST_LIMIT_EXCEEDED","message":"TotalRequests Limit exceeded."}]`)}
	other := &sfHTTPError{Status: 400, Body: []byte(`[{"errorCode":"MALFORMED_QUERY"}]`)}
	plain := errors.New("network down")

	if !isRateLimited(rate) {
		t.Error("429 should be rate-limited")
	}
	if isRateLimited(daily) || isRateLimited(other) || isRateLimited(plain) {
		t.Error("non-429 classified as rate-limited")
	}
	if !isDailyLimitExceeded(daily) {
		t.Error("REQUEST_LIMIT_EXCEEDED should be daily-limit")
	}
	if isDailyLimitExceeded(rate) || isDailyLimitExceeded(other) || isDailyLimitExceeded(plain) {
		t.Error("non-daily-limit classified as daily-limit")
	}

	// classifyQueryErr wraps only the daily-limit case with a clear message,
	// preserving the original via errors.Is/unwrap.
	wrapped := classifyQueryErr(daily)
	if !strings.Contains(wrapped.Error(), "daily API request limit") {
		t.Errorf("daily-limit error not clarified: %v", wrapped)
	}
	if !errors.Is(wrapped, error(daily)) {
		t.Error("classifyQueryErr should wrap (not replace) the original error")
	}
	if got := classifyQueryErr(other); got != error(other) {
		t.Error("non-daily-limit error should pass through unchanged")
	}
}
