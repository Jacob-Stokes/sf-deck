package ui

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCompareRunSemaphoreBounds verifies the run-level semaphore caps how
// many API-call slots are in flight at once: many goroutines all
// acquire/work/release, and the observed peak never exceeds cap(sem).
func TestCompareRunSemaphoreBounds(t *testing.T) {
	const limit = 4
	const workers = 50
	run := &compareRun{sem: make(chan struct{}, limit)}

	var inFlight, peak int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run.acquire()
			defer run.release()
			n := atomic.AddInt64(&inFlight, 1)
			for { // record peak
				p := atomic.LoadInt64(&peak)
				if n <= p || atomic.CompareAndSwapInt64(&peak, p, n) {
					break
				}
			}
			time.Sleep(time.Millisecond) // hold the slot so contention is real
			atomic.AddInt64(&inFlight, -1)
		}()
	}
	wg.Wait()

	if peak > limit {
		t.Errorf("peak in-flight = %d, exceeds cap %d", peak, limit)
	}
	if peak == 0 {
		t.Error("no work observed; semaphore test did not run")
	}
}

// TestCompareRunSemaphoreNilUnbounded confirms a nil semaphore (Tooling
// path / tests) is a no-op rather than a panic or block.
func TestCompareRunSemaphoreNilUnbounded(t *testing.T) {
	run := &compareRun{} // sem nil
	done := make(chan struct{})
	go func() {
		run.acquire()
		run.release()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("nil-sem acquire/release blocked")
	}
	// Also safe on a nil *compareRun receiver.
	var nilRun *compareRun
	nilRun.acquire()
	nilRun.release()
}
