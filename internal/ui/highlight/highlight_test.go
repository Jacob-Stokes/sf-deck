package highlight

import (
	"strings"
	"testing"
)

// TestHighlightApex covers the key invariant: an Apex body produces
// one styled line per source line, and the styled output contains
// ANSI escape sequences (so the highlighter is actually running, not
// silently falling through to plain).
func TestHighlightApex(t *testing.T) {
	body := `trigger MyTrigger on Account (after update) {
    if (Trigger.new.size() > 0) {
        System.debug('hello');
    }
}`
	out := Highlight(body, LangApex)
	if got, want := len(out), 5; got != want {
		t.Fatalf("got %d lines, want %d", got, want)
	}
	// Every keyword/string/etc is wrapped in ANSI escapes — at least
	// one line MUST contain an escape sequence (\x1b[) for the
	// highlighter to be doing its job.
	combined := strings.Join(out, "\n")
	if !strings.Contains(combined, "\x1b[") {
		t.Fatalf("expected ANSI escape sequences in highlighted output, got plain text")
	}
	// Sanity: source content is preserved (escapes added, characters
	// not lost). Strip ANSI then compare.
	stripped := stripANSI(combined)
	if !strings.Contains(stripped, "trigger MyTrigger on Account") {
		t.Fatalf("source text lost during highlighting; stripped output:\n%s", stripped)
	}
}

// TestHighlightPlain confirms LangPlain bypasses chroma entirely.
// No ANSI escapes should appear; the output should be a plain split.
func TestHighlightPlain(t *testing.T) {
	body := "line one\nline two\nline three"
	out := Highlight(body, LangPlain)
	if len(out) != 3 {
		t.Fatalf("got %d lines, want 3", len(out))
	}
	for i, ln := range out {
		if strings.Contains(ln, "\x1b[") {
			t.Fatalf("line %d has ANSI escape on LangPlain: %q", i, ln)
		}
	}
}

// TestLanguageForFilename covers the dispatch table. Only the most
// common cases — exhaustive coverage isn't worth the maintenance
// burden, but we should pin the LWC/Aura surface paths.
func TestLanguageForFilename(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"myCmp/myCmp.html", LangHTML},
		{"myCmp/myCmp.js", LangJavaScript},
		{"myCmp/myCmp.css", LangCSS},
		{"myCmp/myCmp.xml", LangXML},
		{"AuraThing.cmp", LangXML},
		{"AuraThing.evt", LangXML},
		{"unknown.weird", LangPlain},
		{"noextension", LangPlain},
		{"", LangPlain},
	}
	for _, tc := range cases {
		if got := LanguageForFilename(tc.name); got != tc.want {
			t.Errorf("LanguageForFilename(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestHighlightCaches ensures the second call for the same input
// hits the cache. We can't observe the cache directly, but a stable
// pointer-equal slice for repeated calls confirms it.
func TestHighlightCaches(t *testing.T) {
	body := "trigger T on Account (after update) {}"
	a := Highlight(body, LangApex)
	b := Highlight(body, LangApex)
	if len(a) != len(b) {
		t.Fatalf("cached output mismatch: %d vs %d", len(a), len(b))
	}
	// The cached slice is shared — &a[0] should equal &b[0]. If the
	// cache ever stops working, the slices will be re-allocated and
	// the addresses differ.
	if &a[0] != &b[0] {
		t.Fatalf("cache miss on repeat call — got fresh slice each time")
	}
}

// stripANSI is a tiny helper for tests; not exported. Removes all
// CSI sequences (escape + bracket + … + final byte in @-~).
func stripANSI(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != 0x1b || i+1 >= len(s) || s[i+1] != '[' {
			out.WriteByte(s[i])
			i++
			continue
		}
		// Skip until final byte (0x40-0x7E).
		j := i + 2
		for j < len(s) {
			b := s[j]
			j++
			if b >= 0x40 && b <= 0x7e {
				break
			}
		}
		i = j
	}
	return out.String()
}
