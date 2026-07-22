package instance

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func setupHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestRead_MissingFileReturnsEmpty(t *testing.T) {
	setupHome(t)
	f, err := Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(f.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(f.Entries))
	}
	if f.Version != schemaVersion {
		t.Errorf("expected version %d, got %d", schemaVersion, f.Version)
	}
}

func TestClaim_FirstInstanceIsNumber1(t *testing.T) {
	setupHome(t)
	e, err := Claim(os.Getpid(), "/tmp/sock", "")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if e.Number != 1 {
		t.Errorf("expected number 1, got %d", e.Number)
	}
	if e.PID != os.Getpid() {
		t.Errorf("PID mismatch: %d vs %d", e.PID, os.Getpid())
	}
	if e.Socket != "/tmp/sock" {
		t.Errorf("Socket = %q", e.Socket)
	}
}

func TestClaim_LowestFreeSlot(t *testing.T) {
	setupHome(t)
	// Seed with fake-alive entries at slots 1 and 3 (our PID is alive).
	myPID := os.Getpid()
	if _, err := Claim(myPID, "", ""); err != nil {
		t.Fatalf("seed 1: %v", err)
	}
	// Manually inject a second live entry at slot 3 using the same
	// PID — the package's de-dupe-by-PID logic would normally reject
	// this, so write the file directly to set up the scenario.
	path, _ := Path()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(path, []byte(`{"version":1,"entries":[
		{"number":1,"pid":`+itoa(myPID)+`,"started_at":"2026-01-01T00:00:00Z"},
		{"number":3,"pid":`+itoa(myPID)+`,"started_at":"2026-01-01T00:00:00Z"}
	]}`), 0o644))
	// New process (use 1 — init — almost always alive on every OS;
	// signal 0 to PID 1 returns EPERM on Linux + nil on macOS, both
	// of which prune() treats as "still alive").
	e, err := Claim(1, "", "")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if e.Number != 2 {
		t.Errorf("expected lowest-free=2, got %d", e.Number)
	}
}

func TestClaim_FillsHoleLeftByRelease(t *testing.T) {
	setupHome(t)
	myPID := os.Getpid()
	// Two claims would normally collide on PID; instead inject directly.
	path, _ := Path()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(`{"version":1,"entries":[
		{"number":2,"pid":`+itoa(myPID)+`,"started_at":"2026-01-01T00:00:00Z"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// New PID claims — slot 1 is free, slot 2 is taken.
	e, err := Claim(1, "", "")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if e.Number != 1 {
		t.Errorf("expected slot 1, got %d", e.Number)
	}
}

func TestRelease_RemovesEntry(t *testing.T) {
	setupHome(t)
	myPID := os.Getpid()
	if _, err := Claim(myPID, "", ""); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if err := Release(myPID); err != nil {
		t.Fatalf("Release: %v", err)
	}
	f, _ := Read()
	if len(f.Entries) != 0 {
		t.Errorf("expected 0 entries after release, got %d", len(f.Entries))
	}
}

func TestRelease_IdempotentWhenMissing(t *testing.T) {
	setupHome(t)
	if err := Release(999999); err != nil {
		t.Errorf("Release on missing entry should be a no-op, got %v", err)
	}
}

func TestPrune_DropsDeadPIDs(t *testing.T) {
	setupHome(t)
	path, _ := Path()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	// PID 0 is reserved and always not-alive; PID -1 too.
	if err := os.WriteFile(path, []byte(`{"version":1,"entries":[
		{"number":1,"pid":0,"started_at":"2026-01-01T00:00:00Z"},
		{"number":2,"pid":`+itoa(os.Getpid())+`,"started_at":"2026-01-01T00:00:00Z"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(f.Entries) != 1 {
		t.Fatalf("expected 1 surviving entry, got %d", len(f.Entries))
	}
	if f.Entries[0].PID != os.Getpid() {
		t.Errorf("wrong survivor: PID %d", f.Entries[0].PID)
	}
}

func TestClaim_ReplacesPriorEntryForSamePID(t *testing.T) {
	setupHome(t)
	myPID := os.Getpid()
	e1, _ := Claim(myPID, "/tmp/old.sock", "")
	if e1.Number != 1 {
		t.Fatalf("first claim should be 1, got %d", e1.Number)
	}
	// Re-claim with new socket; previous entry should be dropped, not
	// duplicated. The new claim still gets slot 1 because the prior
	// occupant of slot 1 was this same PID and was removed first.
	e2, _ := Claim(myPID, "/tmp/new.sock", "")
	if e2.Number != 1 {
		t.Errorf("re-claim same PID should reuse slot 1, got %d", e2.Number)
	}
	if e2.Socket != "/tmp/new.sock" {
		t.Errorf("socket not updated: %q", e2.Socket)
	}
	f, _ := Read()
	if len(f.Entries) != 1 {
		t.Errorf("expected exactly 1 entry, got %d", len(f.Entries))
	}
}

func TestWriteAtomic_NoPartialReadsUnderConcurrentClaim(t *testing.T) {
	setupHome(t)
	// Spawn N goroutines, each calling Read() while another goroutine
	// hammers Claim/Release. The Read calls must never fail to parse
	// (no partial-write window).
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			pid := os.Getpid()
			_, _ = Claim(pid, "/tmp/s", "")
			_ = Release(pid)
			select {
			case <-stop:
				return
			default:
			}
		}
	}()
	for i := 0; i < 200; i++ {
		_, err := Read()
		if err != nil {
			t.Fatalf("Read raced: %v", err)
		}
	}
	close(stop)
	wg.Wait()
}

func TestPath_UsesHomeDotSfDeck(t *testing.T) {
	home := setupHome(t)
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".sf-deck", "instances.json")
	if p != want {
		t.Errorf("Path() = %q, want %q", p, want)
	}
}

// itoa avoids importing strconv into the seed-file string-building.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf []byte
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		return "-" + string(buf)
	}
	return string(buf)
}
