// Package bulk runs Salesforce Bulk API 2.0 full-dataset CSV exports
// as a self-contained UI feature. It owns the in-flight goroutine,
// the channel-based progress event stream, the per-stage flash
// messages, and the tea.Msg types used to thread events back through
// the Bubble Tea Update loop.
//
// Lives under internal/exporters/ alongside soql/ and devproject/ —
// one subpackage per export source. Format converters (csv/json/xlsx)
// live in the parent exporters/ package.
//
// Architecture: the package depends on a small Host interface that
// the UI shell implements. That keeps bulk free of any ui-package
// coupling — it only sees flash banners, modal openings, and an
// in-flight handle slot.
//
// Submitted Bulk API jobs are routed through sf.BulkQuery (defined in
// internal/sf), which owns the REST plumbing + polling cadence.
package bulk

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/securefile"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// --- Public types ---------------------------------------------------------

// OpenPathMsg lands after the records-export scope picker selects
// Full. Carries the unbounded SOQL and a label for the filename.
type OpenPathMsg struct {
	Label string
	SOQL  string
}

// StartMsg kicks off the Bulk API call once the user has confirmed
// the destination path.
type StartMsg struct {
	Label string
	SOQL  string
	Path  string
	Alias string
}

// ProgressMsg conveys an intermediate stage from the bulk goroutine
// to the UI thread so the flash banner can update.
type ProgressMsg struct {
	Label   string
	Stage   string
	State   string
	Rows    int
	Chunks  int
	Polls   int
	Elapsed time.Duration
}

// DoneMsg lands when the export finishes (success, error, or cancel).
type DoneMsg struct {
	Path    string
	Rows    int
	Polls   int
	Chunks  int
	Elapsed time.Duration
	Err     error
}

// CancelMsg is fired by ctrl+c during a running bulk export.
type CancelMsg struct{}

// Flight holds the in-flight bulk goroutine's handles so the host can
// read its events and cancel it. Hosts store one per session.
type Flight struct {
	events chan event
	cancel context.CancelFunc
	label  string
}

// Events returns the channel from which the host's read-cmd pulls.
// Used by the dispatcher to re-arm reads.
func (f *Flight) Events() <-chan event { return f.events }

// Cancel aborts the in-flight bulk job. The goroutine emits a Done
// with context.Canceled error after this call.
func (f *Flight) Cancel() { f.cancel() }

// Label returns the user-visible label for the in-flight export
// (typically the chip name). Used by host flash messages.
func (f *Flight) Label() string { return f.label }

// event is the internal channel payload. Unifies progress + done so
// the host's read-cmd has one source to select from.
type event struct {
	progress *ProgressMsg
	done     *DoneMsg
}

// --- Host interface -------------------------------------------------------

// Host is the surface this package requires from its UI shell. Keep
// it narrow — every method here is one the export flow actually calls.
type Host interface {
	// Flash + FlashFor surface user-visible status messages on the
	// banner zone. FlashFor lets long-running stages override the
	// default fade timing so progress doesn't disappear mid-job.
	Flash(msg string)
	FlashFor(msg string, d time.Duration)

	// OpenPathPicker opens the path-edit modal pre-populated with
	// defaultPath. onConfirm is invoked with the user-entered path
	// when they accept; the returned tea.Msg should typically be a
	// StartMsg.
	OpenPathPicker(title, hint, defaultPath string, onConfirm func(path string) tea.Msg) tea.Cmd

	// DefaultPath builds the default save path for a CSV export
	// labelled `label` — honours the user's export-dir setting.
	DefaultPath(label string) string

	// ActiveUsername returns the currently-selected org's username,
	// or "" when no org is selected.
	ActiveUsername() string

	// Flight + SetFlight expose the in-flight Flight slot so the
	// host can route ctrl+c to the right cancel func and the
	// dispatcher can re-arm channel reads.
	Flight() *Flight
	SetFlight(f *Flight)
}

// --- Flow functions -------------------------------------------------------

// OpenPathPicker prompts the host's path-edit modal pre-populated
// with the default CSV path.
func OpenPathPicker(host Host, msg OpenPathMsg) tea.Cmd {
	defaultPath := host.DefaultPath(msg.Label)
	label := msg.Label
	soql := msg.SOQL
	alias := host.ActiveUsername()
	return host.OpenPathPicker(
		"Save full export · "+label,
		"Path / filename for full dataset (CSV)  ·  Enter to start  ·  Esc cancel",
		defaultPath,
		func(savedPath string) tea.Msg {
			return StartMsg{
				Label: label,
				SOQL:  soql,
				Path:  strings.TrimSpace(savedPath),
				Alias: alias,
			}
		},
	)
}

// Start opens the destination file, kicks the Bulk job in a goroutine,
// and registers the in-flight handles on the host so the UI dispatcher
// can read progress events + ctrl+c can cancel.
//
// ctx.WithCancel (not WithTimeout) so the user controls run length;
// the host invokes Flight.Cancel() to abort, which propagates to
// sf.BulkQuery and best-effort PATCHes state=Aborted on SF.
func Start(host Host, msg StartMsg) tea.Cmd {
	savePath := expandTilde(msg.Path)
	label := msg.Label
	soql := msg.SOQL
	alias := msg.Alias

	f, err := securefile.New(savePath, false)
	if err != nil {
		return func() tea.Msg {
			return DoneMsg{Err: fmt.Errorf("create %s: %w", savePath, err)}
		}
	}

	// Capacity 32 because progress can burst (every 200ms during
	// chunk download). Backpressure on the goroutine when the UI is
	// slow is fine — sends are non-blocking inside sf.BulkQuery.
	events := make(chan event, 32)
	progress := make(chan sf.BulkQueryProgress, 32)
	ctx, cancel := context.WithCancel(context.Background())
	host.SetFlight(&Flight{events: events, cancel: cancel, label: label})
	host.Flash("submitting bulk job…  (ctrl+c to cancel)")

	// forwarderDone is closed by the forwarder when its drain loop
	// exits. The main bulk goroutine waits on this before closing
	// events — without it, the main goroutine could close(events)
	// while the forwarder still holds a buffered progress value
	// mid-send, panicking with "send on closed channel".
	forwarderDone := make(chan struct{})

	// Forwarder: copies sf.BulkQueryProgress → event until the
	// progress channel closes. Owns no other channels; only writes
	// to events.
	go func() {
		defer close(forwarderDone)
		for p := range progress {
			events <- event{
				progress: &ProgressMsg{
					Label:   label,
					Stage:   p.Stage,
					State:   p.State,
					Rows:    p.Rows,
					Chunks:  p.Chunks,
					Polls:   p.Polls,
					Elapsed: p.Elapsed,
				},
			}
		}
	}()

	// Main bulk goroutine. Lifecycle is strict so the forwarder
	// never races with the close(events):
	//
	//   1. sf.BulkQueryAlias runs to completion (success/error/cancel).
	//   2. close(progress) → forwarder's range loop exits.
	//   3. <-forwarderDone — wait for the forwarder to fully return
	//      (drained any in-flight send to events).
	//   4. Send final Done event.
	//   5. close(events) — safe now because we own events exclusively.
	go func() {
		res, err := sf.BulkQueryAlias(ctx, alias, soql, f, progress)
		close(progress)
		<-forwarderDone

		done := DoneMsg{
			Path:    savePath,
			Rows:    res.RowCount,
			Polls:   res.Polls,
			Chunks:  res.Chunks,
			Elapsed: res.Elapsed,
		}
		if err != nil {
			done.Err = err
			if errors.Is(err, context.Canceled) {
				// Preserve the prior UX: a cancelled export publishes the
				// captured rows, but only after the writer has closed cleanly.
				if commitErr := f.Commit(); commitErr != nil {
					done.Err = fmt.Errorf("publish partial export: %w", commitErr)
				}
			} else {
				_ = f.Abort()
			}
		} else if commitErr := f.Commit(); commitErr != nil {
			done.Err = commitErr
		}
		events <- event{done: &done}
		close(events)
	}()

	return ReadCmd(events)
}

// ReadCmd is the channel-reader cmd. Pulls one event, returns the
// matching tea.Msg. The msg handler in the host's dispatcher re-arms
// with another ReadCmd until the channel closes.
func ReadCmd(events <-chan event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return nil
		}
		if ev.progress != nil {
			return *ev.progress
		}
		if ev.done != nil {
			return *ev.done
		}
		return nil
	}
}

// --- Apply* handlers ------------------------------------------------------

// ApplyProgress updates the flash banner with the current stage.
func ApplyProgress(host Host, msg ProgressMsg) {
	elapsed := msg.Elapsed.Round(time.Second)
	var body string
	switch msg.Stage {
	case "submit":
		body = "submitting bulk job…  (ctrl+c to cancel)"
	case "poll":
		state := msg.State
		if state == "" {
			state = "polling"
		}
		body = fmt.Sprintf("bulk job: %s · poll %d · %s  (ctrl+c to cancel)",
			state, msg.Polls, elapsed)
	case "download":
		body = fmt.Sprintf("downloading %d rows · chunk %d · %s  (ctrl+c to cancel)",
			msg.Rows, msg.Chunks, elapsed)
	default:
		body = fmt.Sprintf("bulk: %s · %s", msg.Stage, elapsed)
	}
	host.FlashFor(body, 60*time.Second)
}

// ApplyCancel fires when the user hits ctrl+c during a bulk export.
// Best-effort: calls the stored cancel func via Flight.Cancel.
func ApplyCancel(host Host) {
	flight := host.Flight()
	if flight == nil {
		return
	}
	flight.Cancel()
	host.Flash("cancelling bulk export…")
}

// ApplyDone folds the result into a flash + log line.
func ApplyDone(host Host, msg DoneMsg) {
	host.SetFlight(nil)

	if errors.Is(msg.Err, context.Canceled) {
		host.FlashFor(fmt.Sprintf("bulk export cancelled (%d rows captured)", msg.Rows),
			8*time.Second)
		applog.Info("records.export.bulk.cancelled", map[string]any{
			"rows": msg.Rows, "polls": msg.Polls,
		})
		return
	}
	if msg.Err != nil {
		host.FlashFor("bulk export failed: "+msg.Err.Error(), 8*time.Second)
		applog.Error("records.export.bulk.failed", map[string]any{"err": msg.Err.Error()})
		return
	}
	host.FlashFor(fmt.Sprintf("saved %d rows → %s (%d polls · %s)",
		msg.Rows, filepath.Base(msg.Path), msg.Polls, msg.Elapsed.Round(time.Second)),
		8*time.Second)
	applog.Info("records.export.bulk.saved", map[string]any{
		"path":   msg.Path,
		"rows":   msg.Rows,
		"polls":  msg.Polls,
		"chunks": msg.Chunks,
	})
}

// --- helpers -------------------------------------------------------------

// expandTilde resolves a leading ~ in a path against $HOME. Hand-rolled
// to avoid importing the (much larger) os/user package just for this.
func expandTilde(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}
