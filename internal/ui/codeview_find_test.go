package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

const findTestBody = "public class Foo {\n" +
	"    private String query;\n" +
	"    public void runQuery() {\n" +
	"        // query the query cache\n" +
	"    }\n" +
	"}"

// newCodeFindModel builds a model whose active org is on TabApexDetail
// with a painted code body, so the find/hscroll key gates open.
func newCodeFindModel(t *testing.T) (Model, *orgData) {
	t.Helper()
	d := &orgData{}
	d.Tab = TabApexDetail
	m := Model{
		modelOrgs: modelOrgs{
			orgs:     []sf.Org{{Username: "find@test"}},
			selected: 0,
			data:     map[string]*orgData{"find@test": d},
		},
	}
	// Simulate the paint that records what's on screen.
	m.renderCodeView(d, codeViewSpec{
		BodyID: "apex:test", Body: findTestBody, Lang: "",
		Inner: 60, Height: 10, Focused: true,
	})
	if d.CodeViewLast.BodyID != "apex:test" {
		t.Fatal("renderCodeView must record the last-painted body")
	}
	return m, d
}

func TestCodeFindMatchScan(t *testing.T) {
	st := &codeFindState{Buffer: "query"}
	ms := codeFindMatchesFor(st, findTestBody)
	// "query" (line 1), "runQuery" (line 2, case-insensitive),
	// "query the query" (line 3, twice) = 4 matches.
	if len(ms) != 4 {
		t.Fatalf("got %d matches, want 4: %+v", len(ms), ms)
	}
	if ms[0].Line != 1 || ms[1].Line != 2 || ms[2].Line != 3 || ms[3].Line != 3 {
		t.Errorf("match lines wrong: %+v", ms)
	}
	// Memo: same query + body returns the identical slice.
	again := codeFindMatchesFor(st, findTestBody)
	if len(again) != 4 || &again[0] != &ms[0] {
		t.Error("memo miss on unchanged query+body")
	}
	// Query change invalidates.
	st.Buffer = "class"
	if got := codeFindMatchesFor(st, findTestBody); len(got) != 1 || got[0].Line != 0 {
		t.Errorf("after query change: %+v", got)
	}
	// A same-length source refresh must also invalidate. Body length alone is
	// not an identity: Apex/LWC edits commonly replace text in place.
	st.Buffer = "query"
	refreshed := strings.NewReplacer("query", "other", "Query", "Other").Replace(findTestBody)
	if len(refreshed) != len(findTestBody) {
		t.Fatal("test fixture must preserve byte length")
	}
	if got := codeFindMatchesFor(st, refreshed); len(got) != 0 {
		t.Errorf("same-length body refresh reused stale matches: %+v", got)
	}
}

func TestCodeFindTypingCyclingAndCounter(t *testing.T) {
	m, d := newCodeFindModel(t)

	// "/" opens the bar.
	mm, _, ok := m.onCodeViewKey("/")
	if !ok {
		t.Fatal("/ must open find on a code surface")
	}
	st := codeFindStateFor(d, "apex:test", false)
	if st == nil || !st.Active {
		t.Fatal("find bar should be active after /")
	}

	// Typing appends + live-jumps to the first match.
	for _, r := range "query" {
		var handled bool
		mm, _, handled = mm.handleCodeFindInput(tea.KeyPressMsg{Code: r, Text: string(r)})
		if !handled {
			t.Fatalf("printable %q not consumed by find input", r)
		}
	}
	if st.Buffer != "query" {
		t.Fatalf("buffer = %q, want \"query\"", st.Buffer)
	}
	if d.BodyCursor["apex:test"] != 1 {
		t.Errorf("live-jump cursor = %d, want line 1", d.BodyCursor["apex:test"])
	}

	// Enter cycles forward; shift+enter back (wrapping).
	mm, _, _ = mm.handleCodeFindInput(keyMsgFromString("enter"))
	if st.Idx != 1 || d.BodyCursor["apex:test"] != 2 {
		t.Errorf("enter: idx=%d cursor=%d, want 1 / line 2", st.Idx, d.BodyCursor["apex:test"])
	}
	mm, _, _ = mm.handleCodeFindInput(keyMsgFromString("shift+enter"))
	if st.Idx != 0 {
		t.Errorf("shift+enter: idx=%d, want 0", st.Idx)
	}
	mm, _, _ = mm.handleCodeFindInput(keyMsgFromString("shift+enter"))
	if st.Idx != 3 {
		t.Errorf("shift+enter wrap: idx=%d, want 3", st.Idx)
	}

	// Counter renders "x of y" in the bar.
	out := mm.renderCodeView(d, codeViewSpec{
		BodyID: "apex:test", Body: findTestBody, Lang: "",
		Inner: 60, Height: 10, Focused: true,
	})
	if len(out) == 0 || !strings.Contains(ansi.Strip(out[0]), "4 of 4") {
		t.Errorf("bar should show \"4 of 4\", got %q", ansi.Strip(out[0]))
	}

	// Esc keeps the query; n keeps cycling from idle state.
	mm, _, _ = mm.handleCodeFindInput(keyMsgFromString("esc"))
	if st.Active {
		t.Fatal("esc must close the bar")
	}
	if st.Buffer != "query" {
		t.Fatal("esc must preserve the query")
	}
	mm, _, ok = mm.onCodeViewKey("n")
	if !ok || st.Idx != 0 {
		t.Errorf("idle n: handled=%v idx=%d, want cycle to 0", ok, st.Idx)
	}
	// C clears everything.
	if _, _, ok = mm.onCodeViewKey("C"); !ok || st.Buffer != "" {
		t.Errorf("idle C should clear the query (buffer=%q)", st.Buffer)
	}
}

func TestCodeFindHighlightAndBarConsumeHeight(t *testing.T) {
	m, d := newCodeFindModel(t)
	st := codeFindStateFor(d, "apex:test", true)
	st.Buffer = "query"
	st.Active = true

	out := m.renderCodeView(d, codeViewSpec{
		BodyID: "apex:test", Body: findTestBody, Lang: "",
		Inner: 60, Height: 4, Focused: true,
	})
	// Bar + at most Height-1 body rows.
	if len(out) > 4 {
		t.Fatalf("output %d rows, budget 4 (bar consumes one)", len(out))
	}
	if !strings.Contains(ansi.Strip(out[0]), "/query") {
		t.Errorf("first row should be the find bar, got %q", ansi.Strip(out[0]))
	}
	// A matched line carries background styling (SGR 48; codes).
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "\x1b[") {
		t.Error("matched lines should carry ANSI match styling")
	}
	// Content is intact after styling.
	if !strings.Contains(ansi.Strip(joined), "private String query;") {
		t.Errorf("matched line content mangled:\n%s", ansi.Strip(joined))
	}
}

func TestCodeViewHorizontalScroll(t *testing.T) {
	m, d := newCodeFindModel(t)

	// Right scrolls; the body loses leading text and gains the "…" marker.
	mm, _, ok := m.onCodeViewKey("right")
	if !ok {
		t.Fatal("right arrow must be consumed on a code surface")
	}
	if d.BodyHScroll["apex:test"] != codeFindHScrollStep {
		t.Fatalf("hscroll = %d, want %d", d.BodyHScroll["apex:test"], codeFindHScrollStep)
	}
	out := mm.renderCodeView(d, codeViewSpec{
		BodyID: "apex:test", Body: findTestBody, Lang: "",
		Inner: 60, Height: 10, Focused: true,
	})
	plain := ansi.Strip(strings.Join(out, "\n"))
	if !strings.Contains(plain, "…") {
		t.Error("hscrolled view should show the … marker")
	}
	if strings.Contains(plain, "public class Foo") {
		t.Error("hscroll should shift the body text left")
	}
	// Gutter stays put: line numbers still visible.
	if !strings.Contains(plain, " 1 ") {
		t.Error("gutter must not scroll horizontally")
	}

	// Left scrolls back and clamps at 0.
	mm, _, _ = mm.onCodeViewKey("left")
	_, _, _ = mm.onCodeViewKey("left")
	if d.BodyHScroll["apex:test"] != 0 {
		t.Errorf("hscroll should clamp at 0, got %d", d.BodyHScroll["apex:test"])
	}
}

// The gates must NOT fire once the user navigates away from the
// painted code surface — left/right belong to other surfaces there.
func TestCodeViewKeysGatedByTab(t *testing.T) {
	m, d := newCodeFindModel(t)
	d.Tab = TabFlows // navigated away; CodeViewLast still points at apex
	if _, _, ok := m.onCodeViewKey("right"); ok {
		t.Error("hscroll must not fire off the code surface")
	}
	if _, _, ok := m.onCodeViewKey("/"); ok {
		t.Error("find must not open off the code surface")
	}
	if m.codeFindInputActive() {
		t.Error("input gate must close off the code surface")
	}
}

// keyMsgFromString builds a KeyPressMsg for special keys by name.
func keyMsgFromString(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "shift+enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	}
	return tea.KeyPressMsg{}
}
