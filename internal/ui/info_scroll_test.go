package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestInfoModalScrollClamp(t *testing.T) {
	// Everything fits (n <= visible): always 0.
	if got := infoModalScroll(5, 3, 10); got != 0 {
		t.Errorf("fits: got %d, want 0", got)
	}
	// Overflow: visible=10 -> windowH=8, n=20 -> maxOffset=12.
	if got := infoModalScroll(100, 20, 10); got != 12 {
		t.Errorf("over max: got %d, want 12 (clamped)", got)
	}
	if got := infoModalScroll(-5, 20, 10); got != 0 {
		t.Errorf("negative: got %d, want 0", got)
	}
	if got := infoModalScroll(5, 20, 10); got != 5 {
		t.Errorf("in range: got %d, want 5", got)
	}
}

func TestRecordFieldDisplayValue(t *testing.T) {
	if got := recordFieldDisplayValue(nil); got != "" {
		t.Errorf("nil -> %q, want empty", got)
	}
	if got := recordFieldDisplayValue("hello"); got != "hello" {
		t.Errorf("string -> %q", got)
	}
	// A nested value gets JSON-rendered (multi-line, readable).
	got := recordFieldDisplayValue(map[string]any{"a": 1})
	if got == "" || got[0] != '{' {
		t.Errorf("object should JSON-render, got %q", got)
	}
}

func TestRecordFieldJSONDetection(t *testing.T) {
	// A JSON-object string gets pretty-printed (multi-line, indented).
	got := recordFieldDisplayValue(`{"group":"Name","showLabel":true}`)
	if !strings.Contains(got, "\"group\": \"Name\"") {
		t.Errorf("JSON object not pretty-printed:\n%q", got)
	}
	// A JSON array string too.
	got = recordFieldDisplayValue(`[{"a":1},{"b":2}]`)
	if got == "" || got[0] != '[' || !strings.Contains(got, "\"a\": 1") {
		t.Errorf("JSON array not pretty-printed:\n%q", got)
	}
	// Ordinary CSV / text (not JSON) is returned verbatim.
	csv := "FirstName,LastName,Forename2__pc"
	if recordFieldDisplayValue(csv) != csv {
		t.Errorf("non-JSON text should be verbatim")
	}
	// A bare number-looking string must NOT be reformatted.
	if recordFieldDisplayValue("123") != "123" {
		t.Errorf("bare number string should be verbatim")
	}
	// Malformed JSON-ish text returns verbatim (not swallowed).
	bad := `{not valid json`
	if recordFieldDisplayValue(bad) != bad {
		t.Errorf("malformed JSON should be verbatim")
	}
}

// Reproduces the "scroll silently continues past the end, so reversing
// lags" bug: the stored Scroll must be clamped at mutation time, not
// just at render time.
func TestInfoModalScrollNoOverrun(t *testing.T) {
	// 100-line body; a terminal tall enough to show ~12 lines.
	body := strings.Repeat("x\n", 99) + "x" // 100 lines
	m := Model{modelRuntime: modelRuntime{height: 20}}
	m.infoModal = &infoModalState{PreRendered: body}
	maxS := m.infoModalMaxScroll()
	if maxS <= 0 {
		t.Fatalf("expected a positive max scroll, got %d", maxS)
	}

	down := tea.KeyPressMsg{Code: 'j', Text: "j"}
	// Press down far more times than there are lines.
	for i := 0; i < 500; i++ {
		m, _ = m.handleInfoModalKey(down)
	}
	if m.infoModal.Scroll != maxS {
		t.Fatalf("stored scroll = %d, want clamped to max %d (over-run bug)", m.infoModal.Scroll, maxS)
	}
	// One press up must IMMEDIATELY move (not unwind phantom scroll).
	up := tea.KeyPressMsg{Code: 'k', Text: "k"}
	m, _ = m.handleInfoModalKey(up)
	if m.infoModal.Scroll != maxS-1 {
		t.Fatalf("after 1 up: scroll = %d, want %d (reversal lag)", m.infoModal.Scroll, maxS-1)
	}
}

// wrapPreserving must keep JSON indentation intact — the old prose wrap()
// collapsed whitespace and reflowed words, destroying the indent.
func TestWrapPreservingKeepsIndent(t *testing.T) {
	pretty, ok := prettyJSONString(`{"group":"Name","fields":["A","B"]}`)
	if !ok {
		t.Fatal("expected valid JSON")
	}
	// The pretty JSON has indented lines like `  "group": "Name",`.
	out := wrapPreserving(pretty, 80)
	if !strings.Contains(out, "\n  \"group\": \"Name\"") {
		t.Errorf("indentation not preserved:\n%s", out)
	}
	// A line under width is untouched (no spurious breaks).
	if strings.Contains(out, "\n\n") {
		t.Errorf("introduced blank lines:\n%s", out)
	}
}
