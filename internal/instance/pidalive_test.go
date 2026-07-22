package instance

import (
	"os"
	"testing"
)

// TestPidAlive covers pidAlive's branches — the liveness probe behind
// the instance-number-climbing fix (a dead PID must read as NOT alive
// so its slot is pruned and reused rather than accumulating).
func TestPidAlive(t *testing.T) {
	// Our own PID is alive.
	if !pidAlive(os.Getpid()) {
		t.Error("current process should be alive")
	}
	// Non-positive PIDs are never alive.
	if pidAlive(0) {
		t.Error("pid 0 should not be alive")
	}
	if pidAlive(-1) {
		t.Error("negative pid should not be alive")
	}
	// A PID that is almost certainly not running. On Unix a huge PID
	// yields ESRCH → dead; the key contract is it does NOT report alive
	// (which is what caused duplicate instance numbers when the ESRCH/
	// EPERM distinction was wrong).
	if pidAlive(0x7ffffff0) {
		t.Error("an unused huge PID should read as dead on this platform")
	}
}
