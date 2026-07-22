package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// The selected scope must remain visible even when the strip overflows —
// the bug: ansi.Truncate chopped a far-right selection off the edge.
func TestFLSScopeStripKeepsSelectedVisible(t *testing.T) {
	var perms []sf.FLSPickerEntry
	for i := 0; i < 40; i++ {
		perms = append(perms, sf.FLSPickerEntry{
			ID:    fmt.Sprintf("id_%d", i), // unique IDs
			Label: "PermissionSet_xxxxxx",
		})
	}
	perms[30].Label = "SELECTED_SCOPE_UNIQUE"
	sel := perms[30].ID

	out := ansi.Strip(renderFLSScopeStrip(perms, sel, 60))
	if !strings.Contains(out, "SELECTED_SCOPE_UNIQUE") {
		t.Fatalf("selected scope not visible in strip:\n%q", out)
	}
	// Overflow markers should show on both sides (30 is mid-list).
	if !strings.Contains(out, "…") {
		t.Errorf("expected an overflow marker for a windowed strip:\n%q", out)
	}
}

// A short list (fits) should show every scope, no markers.
func TestFLSScopeStripFitsNoMarkers(t *testing.T) {
	perms := []sf.FLSPickerEntry{
		{ID: "a", Label: "Admin"},
		{ID: "b", Label: "Sales"},
	}
	out := ansi.Strip(renderFLSScopeStrip(perms, "b", 80))
	if !strings.Contains(out, "Admin") || !strings.Contains(out, "Sales") {
		t.Fatalf("short list should show all scopes: %q", out)
	}
}
