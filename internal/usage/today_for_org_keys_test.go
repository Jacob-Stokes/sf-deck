package usage

import (
	"path/filepath"
	"testing"
	"time"
)

// TestTodayForOrgKeysReconcilesAliasAndUsername proves the header-counter
// fix: the same org's calls recorded under BOTH its short alias and its
// username are summed (and each row counted once), instead of the header
// seeing only one key. This is what made the /compare counter look flat.
func TestTodayForOrgKeysReconcilesAliasAndUsername(t *testing.T) {
	tr, err := openAt(filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatalf("openAt: %v", err)
	}
	defer tr.db.Close()

	const alias = "acme-test"
	const username = "user@acme.test"

	// Simulate: a few calls recorded under the short alias, many under the
	// username (as /compare does).
	for i := 0; i < 3; i++ {
		tr.Bump(alias, []string{"GET", "/services/data/limits"}, nil, time.Millisecond)
	}
	for i := 0; i < 20; i++ {
		tr.Bump(username, []string{"POST", "/services/Soap/m readMetadata Flow"}, nil, time.Millisecond)
	}

	// Old behaviour: looking up only one key misses the other.
	if got := tr.TodayForOrg(alias); got != 3 {
		t.Errorf("TodayForOrg(alias) = %d, want 3 (the alias-only rows)", got)
	}
	if got := tr.TodayForOrg(username); got != 20 {
		t.Errorf("TodayForOrg(username) = %d, want 20", got)
	}

	// New behaviour: both keys reconcile to the full total.
	if got := tr.TodayForOrgKeys(alias, username); got != 23 {
		t.Errorf("TodayForOrgKeys(alias, username) = %d, want 23", got)
	}

	// Duplicate / empty keys are ignored, not double-counted.
	if got := tr.TodayForOrgKeys(alias, alias, "", username); got != 23 {
		t.Errorf("TodayForOrgKeys with dupes/empty = %d, want 23", got)
	}
	if got := tr.TodayForOrgKeys(); got != 0 {
		t.Errorf("TodayForOrgKeys() with no keys = %d, want 0", got)
	}
}
