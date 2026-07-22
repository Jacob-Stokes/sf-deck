package settings

import (
	"bytes"
	"sync"
	"testing"

	"github.com/BurntSushi/toml"
)

// TestSaveEncodeRaceAgainstMutators reproduces the pre-fix crash: Save
// TOML-encodes the settings while a mutator writes one of the encoded
// maps from another goroutine → Go's fatal "concurrent map iteration and
// map write". The fix encodes a locked snapshot (cloned maps) and locks
// the map-mutators, so this must run clean under `go test -race`.
//
// Before the fix this panicked/aborted; after, it passes.
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

	// Mutator goroutines hammering the maps the encoder reads.
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
	// A handful of distinct keys so entries are both added and overwritten.
	return string(rune('a' + i%8))
}
