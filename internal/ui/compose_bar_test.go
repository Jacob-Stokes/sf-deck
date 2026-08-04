package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The header's middle zone carries live status ("getting new token…").
// When a long breadcrumb (left) crowds the bar, the OLD behaviour dropped
// the middle entirely — so the token/sync note vanished on tabs with long
// breadcrumbs while showing on short ones. It must now survive by
// truncating the breadcrumb instead.
func TestComposeBarKeepsMiddleByTruncatingLeft(t *testing.T) {
	left := "records · Account · a-very-long-breadcrumb-that-would-crowd-the-bar-out"
	middle := "⟳ getting new token…"
	right := "API 412"
	// Width comfortably above the width-70 floor (caller handles that),
	// but not enough for the full breadcrumb + middle + right.
	out := composeBar(90, left, middle, right)

	if !strings.Contains(out, "getting new token…") {
		t.Fatalf("middle status was dropped when it should have survived by truncating left; bar:\n%q", out)
	}
	if !strings.Contains(out, "…") {
		t.Fatalf("expected the breadcrumb to be truncated (…) to make room for the status; bar:\n%q", out)
	}
	if !strings.Contains(out, "API 412") {
		t.Fatalf("right zone lost; bar:\n%q", out)
	}
}

// When even a stub of left cannot coexist with middle+right, middle is
// dropped rather than overflowing the bar — the graceful floor.
func TestComposeBarDropsMiddleWhenImpossible(t *testing.T) {
	left := "x"
	middle := "a-status-string-far-too-wide-to-ever-fit-in-this-narrow-bar-really"
	right := "API 999999"
	out := composeBar(40, left, middle, right)

	if ansi.StringWidth(out) != 40 {
		t.Fatalf("bar width = %d, want exactly 40 (no overflow); bar:\n%q", ansi.StringWidth(out), out)
	}
}

// The common wide-terminal case: everything fits, all three zones present,
// no truncation.
func TestComposeBarKeepsAllWhenWide(t *testing.T) {
	out := composeBar(120, "records · Account", "⟳ syncing records…", "API 12 / 15k")
	for _, want := range []string{"records", "syncing records…", "API 12"} {
		if !strings.Contains(out, want) {
			t.Fatalf("wide bar dropped %q; bar:\n%q", want, out)
		}
	}
	if strings.Contains(out, "…"+" ") && !strings.Contains(out, "syncing records…") {
		t.Fatalf("unexpected truncation on a wide bar; bar:\n%q", out)
	}
}
