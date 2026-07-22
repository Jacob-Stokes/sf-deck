package sf

import (
	"testing"
	"time"
)

func TestApplyConfig_OverridesAndZeroIgnored(t *testing.T) {
	// Snapshot + restore so we don't leak state into other tests.
	cfgMu.RLock()
	orig := cfg
	cfgMu.RUnlock()
	defer func() {
		cfgMu.Lock()
		cfg = orig
		cfgMu.Unlock()
	}()

	// Set a known baseline.
	ApplyConfig(Config{
		HTTPTimeout: 11 * time.Second,
		CLITimeout:  12 * time.Second,
		BulkPoll:    3 * time.Second,
		APIVersion:  "60.0",
	})
	if got := cfgHTTPTimeout(); got != 11*time.Second {
		t.Errorf("HTTPTimeout = %v, want 11s", got)
	}
	if got := cfgAPIVersion(); got != "60.0" {
		t.Errorf("APIVersion = %q, want 60.0", got)
	}

	// A second call with a zero HTTPTimeout must NOT clobber the
	// previously-set value (zero durations are ignored)...
	ApplyConfig(Config{CLITimeout: 99 * time.Second, APIVersion: ""})
	if got := cfgHTTPTimeout(); got != 11*time.Second {
		t.Errorf("HTTPTimeout after zero-field apply = %v, want preserved 11s", got)
	}
	if got := cfgCLITimeout(); got != 99*time.Second {
		t.Errorf("CLITimeout = %v, want 99s", got)
	}
	// ...but APIVersion is always applied, so "" clears the forced version.
	if got := cfgAPIVersion(); got != "" {
		t.Errorf("APIVersion after empty apply = %q, want cleared", got)
	}
}
