package settings

import (
	"bytes"
	"sync"
	"testing"

	"github.com/BurntSushi/toml"
)

// TestSaveEncodeRaceAgainstMutators verifies that Save encodes a locked
// snapshot while concurrent mutators update the live settings. This must run
// cleanly under `go test -race`.
func TestSaveEncodeRaceAgainstMutators(t *testing.T) {
	s := &Settings{
		Orgs: map[string]OrgConfig{},
	}

	const iters = 300
	var wg sync.WaitGroup

	// Encoder goroutine — exactly what Save does after taking s.mu:
	// snapshot under the lock, then encode the snapshot.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			s.mu.Lock()
			snap := s.snapshotLocked()
			s.mu.Unlock()
			var buf bytes.Buffer
			if err := toml.NewEncoder(&buf).Encode(snap); err != nil {
				t.Errorf("encode snapshot: %v", err)
				return
			}
		}
	}()

	mutators := []func(i int){
		func(i int) { s.SetOrg(orgKey(i), SafetyRecords, false) },
		func(i int) { s.SetRecentForOrg(orgKey(i), []RecentConfig{{}}) },
		func(i int) { s.SetCacheTTLOverride(orgKey(i), "4h") },
		func(i int) { s.SetLoadedDevProjectForOrg(orgKey(i), "p1") },
		func(i int) { s.UpsertChip(ChipConfig{ID: orgKey(i), Domain: "records"}) },
	}
	for _, m := range mutators {
		wg.Add(1)
		go func(mut func(int)) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				mut(i)
			}
		}(m)
	}

	wg.Wait()
}

func orgKey(i int) string {
	return string(rune('a' + i%8))
}
