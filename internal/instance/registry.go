// Package instance manages the per-PID identity sf-deck instances
// expose to the outside world.
package instance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"syscall"
	"time"
)

const fileName = "instances.json"

// Entry is one running sf-deck instance.
type Entry struct {
	Number    int    `json:"number"`
	PID       int    `json:"pid"`
	StartedAt string `json:"started_at"`
	Socket    string `json:"socket,omitempty"`
	Label     string `json:"label,omitempty"`
}

// File is the on-disk shape — just a list of entries plus a schema
// version that lets us evolve the format without breaking older
// instances reading it.
type File struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

const schemaVersion = 1

// Path returns the absolute path to the registry file. Visible so
// tests can inject HOME via t.Setenv("HOME", ...) and pull the
// resulting path back out.
func Path() (string, error) {
	dir, err := defaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

func defaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sf-deck"), nil
}

// fileLock serialises concurrent Claim/Release calls inside one
// process. Cross-process serialisation is a SEPARATE concern handled
// by an OS advisory file lock (see withFileLock): atomic-rename alone
// prevents a torn file but NOT a lost update — two processes claiming
// from the same starting state both compute the same lowest-free slot
// and each overwrites the other, ending up on the same number and the
// same socket path (one then unlinks the other's live socket). The
// flock makes read-allocate-write one critical section across
// processes.
var fileLock sync.Mutex

// withFileLock runs fn while holding an exclusive advisory lock on
// ~/.sf-deck/instances.lock, serialising the read-allocate-write
// transaction across processes. The lock file is separate from the
// registry file so we never flock the file we atomically rename over.
// Best-effort on lock-file setup: if the lock can't be acquired (e.g.
// an exotic filesystem without flock), fn still runs — we degrade to
// the old process-local-only behaviour rather than refusing to launch.
func withFileLock(fn func() error) error {
	path, err := Path()
	if err != nil {
		return fn()
	}
	lockPath := path + ".lock"
	lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fn()
	}
	defer lf.Close()
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		return fn()
	}
	defer syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)
	return fn()
}

// Read returns the current registry contents, pruned of any entry
// whose PID is no longer alive. Returns an empty File (not an
// error) when the file doesn't exist yet.
func Read() (File, error) {
	path, err := Path()
	if err != nil {
		return File{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{Version: schemaVersion}, nil
		}
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		// Corrupt file — treat as empty rather than refusing to start.
		// A new Claim will overwrite it cleanly.
		return File{Version: schemaVersion}, nil
	}
	f.Entries = prune(f.Entries)
	return f, nil
}

// prune drops entries whose PID has died. See pidAlive for the
// per-platform liveness rules (Unix: alive on nil-or-EPERM, dead on
// ESRCH / wrapped-finished; Windows: conservative keep-on-ambiguous).
func prune(entries []Entry) []Entry {
	out := entries[:0:0]
	for _, e := range entries {
		if !pidAlive(e.PID) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// Windows: Signal(0) is NOT a liveness probe — os.Process.Signal
	// returns "not supported by windows" for LIVE processes too. The
	// Unix EPERM logic below would then classify every live Windows
	// instance as dead and prune it on read (duplicate instance
	// numbers, lost socket discovery). Fall back to the conservative
	// "keep on ambiguous error" there — a Windows dead PID whose slot
	// lingers is the lesser evil, and FindProcess already fails for a
	// truly-gone PID on Windows so most dead entries still drop.
	if runtime.GOOS == "windows" {
		return true
	}
	// Unix: Go wraps the syscall error into a generic "os: process
	// already finished" for ESRCH, so errors.Is(err, syscall.ESRCH)
	// silently returns false. The old "treat anything that isn't
	// ESRCH as alive" branch left every dead PID in the registry
	// forever — the instance counter climbed by one on every relaunch
	// (#41 after a day of dev) and never reset.
	//
	// Treat as alive only when the kernel says EPERM — the one case
	// where the process really does exist but we don't own it
	// (cross-user, recycled root PID). Every other error (ESRCH, the
	// wrapped "process already finished", anything weird) → dead.
	return errors.Is(err, syscall.EPERM)
}

// Claim allocates a new instance number for this process and writes
// the registry to disk. socket may be "" when the listener isn't
// enabled. Returns the claimed Entry so callers can render the
// instance number / socket path.
//
// Allocation policy: lowest free Number ≥ 1 across the surviving
// entries (after pruning dead PIDs).
func Claim(pid int, socket, label string) (Entry, error) {
	fileLock.Lock()
	defer fileLock.Unlock()

	var entry Entry
	err := withFileLock(func() error {
		f, err := Read()
		if err != nil {
			return err
		}
		filtered := f.Entries[:0:0]
		for _, e := range f.Entries {
			if e.PID == pid {
				continue
			}
			filtered = append(filtered, e)
		}
		number := lowestFree(filtered)
		entry = Entry{
			Number:    number,
			PID:       pid,
			StartedAt: time.Now().UTC().Format(time.RFC3339),
			Socket:    socket,
			Label:     label,
		}
		f.Entries = append(filtered, entry)
		return writeAtomic(f)
	})
	if err != nil {
		return Entry{}, err
	}
	return entry, nil
}

// Release removes this PID's entry from the registry. Idempotent —
// "entry not found" is success, since a missing entry means the goal
// state is already reached. Call on clean shutdown.
func Release(pid int) error {
	fileLock.Lock()
	defer fileLock.Unlock()

	return withFileLock(func() error {
		f, err := Read()
		if err != nil {
			return err
		}
		filtered := f.Entries[:0:0]
		for _, e := range f.Entries {
			if e.PID == pid {
				continue
			}
			filtered = append(filtered, e)
		}
		if len(filtered) == len(f.Entries) {
			return nil // nothing to do
		}
		f.Entries = filtered
		return writeAtomic(f)
	})
}

func lowestFree(entries []Entry) int {
	taken := make(map[int]bool, len(entries))
	for _, e := range entries {
		taken[e.Number] = true
	}
	for i := 1; ; i++ {
		if !taken[i] {
			return i
		}
	}
}

// writeAtomic dumps the file to a sibling temp file and renames it
// over the real path. Rename is atomic on POSIX so concurrent
// readers never see a partial write.
func writeAtomic(f File) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f.Version = schemaVersion
	sort.Slice(f.Entries, func(i, j int) bool {
		return f.Entries[i].Number < f.Entries[j].Number
	})
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	// Suffix with the PID so concurrent claimers don't fight over the
	// same temp path. Rename is what makes the final file appear all
	// at once.
	tmp := fmt.Sprintf("%s.tmp.%d", path, os.Getpid())
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
