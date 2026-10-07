package bulk

import (
	"os"
	"strings"
	"testing"
)

// TestExpandTilde covers the path-expansion helper. Real Bulk-export
// runs need a network + Salesforce org, so the harness is best done at
// the UI integration level; pure helpers like expandTilde can be
// unit-tested cheaply here.
func TestExpandTilde(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"~", "$HOME"},
		{"~/exports/file.csv", "$HOME/exports/file.csv"},
		{"/absolute/path.csv", "/absolute/path.csv"},
		{"relative/path.csv", "relative/path.csv"},
		{"", ""},
	}
	for _, c := range cases {
		got := expandTilde(c.in)
		if strings.Contains(c.want, "$HOME") {
			home, err := os.UserHomeDir()
			if err != nil || home == "" {
				continue // no HOME in this environment; nothing to assert
			}
			if want := strings.ReplaceAll(c.want, "$HOME", home); got != want {
				t.Errorf("expandTilde(%q) = %q, want %q", c.in, got, want)
			}
			continue
		}
		if got != c.want {
			t.Errorf("expandTilde(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestReadCmd_EmitsProgress confirms that the cmd reads a progress
// event from the channel and returns it as a ProgressMsg.
func TestReadCmd_EmitsProgress(t *testing.T) {
	ch := make(chan event, 1)
	ch <- event{
		progress: &ProgressMsg{
			Stage: "download",
			Rows:  100,
		},
	}
	cmd := ReadCmd(ch)
	msg := cmd()
	pm, ok := msg.(ProgressMsg)
	if !ok {
		t.Fatalf("expected ProgressMsg, got %T", msg)
	}
	if pm.Stage != "download" || pm.Rows != 100 {
		t.Errorf("payload mismatch: %+v", pm)
	}
}

// TestReadCmd_EmitsDone confirms Done events come through.
func TestReadCmd_EmitsDone(t *testing.T) {
	ch := make(chan event, 1)
	ch <- event{
		done: &DoneMsg{Path: "/tmp/out.csv", Rows: 42},
	}
	cmd := ReadCmd(ch)
	msg := cmd()
	dm, ok := msg.(DoneMsg)
	if !ok {
		t.Fatalf("expected DoneMsg, got %T", msg)
	}
	if dm.Path != "/tmp/out.csv" || dm.Rows != 42 {
		t.Errorf("payload mismatch: %+v", dm)
	}
}

// TestReadCmd_ClosedChannel confirms a closed channel returns nil msg.
func TestReadCmd_ClosedChannel(t *testing.T) {
	ch := make(chan event)
	close(ch)
	cmd := ReadCmd(ch)
	msg := cmd()
	if msg != nil {
		t.Errorf("expected nil from closed channel, got %T", msg)
	}
}
