package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// renderNoteBox must produce EXACTLY w×h — the sidebar composition
// places it by arithmetic, so an off-by-two (lipgloss border/padding
// semantics) would overflow the panel or leave a seam.
func TestRenderNoteBoxDimensions(t *testing.T) {
	for _, tc := range []struct{ w, h int }{{20, 6}, {34, 10}, {16, 3}} {
		box := renderNoteBox(tc.w, tc.h, "short note")
		lines := strings.Split(box, "\n")
		if len(lines) != tc.h {
			t.Errorf("w=%d h=%d: got %d lines, want %d", tc.w, tc.h, len(lines), tc.h)
		}
		if got := lipgloss.Width(box); got != tc.w {
			t.Errorf("w=%d h=%d: rendered width %d, want %d", tc.w, tc.h, got, tc.w)
		}
	}
}

// A body taller than the box swaps its last line for the q-n hint —
// the modal is the expansion surface, so the hint must survive.
func TestRenderNoteBoxTruncationHint(t *testing.T) {
	long := strings.Repeat("line of note text\n", 20)
	box := renderNoteBox(30, 6, long)
	if !strings.Contains(ansi.Strip(box), "q n → full note") {
		t.Errorf("clipped note must carry the expand hint; got:\n%s", ansi.Strip(box))
	}
	// And a fitting body must NOT show it.
	small := renderNoteBox(30, 6, "fits")
	if strings.Contains(ansi.Strip(small), "full note") {
		t.Error("un-clipped note must not show the expand hint")
	}
}

// Stacked composition: box spans rows 1..innerH-2 (title + footer rows
// stay note-free), and every composed row stays within the panel width.
func TestJoinNoteBesideGeometry(t *testing.T) {
	const contentW, innerH = 20, 8
	content := strings.Join([]string{"TITLE", "row1", "row2"}, "\n")
	box := renderNoteBox(14, innerH-2, "note body")
	out := strings.Split(joinNoteBeside(content, box, contentW, innerH), "\n")
	if len(out) != innerH {
		t.Fatalf("composed %d rows, want %d", len(out), innerH)
	}
	// Row 0 (panel title) and last row (footer) carry no box border.
	for _, i := range []int{0, innerH - 1} {
		if strings.ContainsAny(ansi.Strip(out[i]), "╭╰│╮╯") {
			t.Errorf("row %d must stay box-free: %q", i, ansi.Strip(out[i]))
		}
	}
	// Rows 1..innerH-2 all carry the box; total width == contentW+2+boxW.
	want := contentW + 2 + 14
	for i := 1; i < innerH-1; i++ {
		if !strings.ContainsAny(ansi.Strip(out[i]), "╭╰│╮╯") {
			t.Errorf("row %d missing the note box: %q", i, ansi.Strip(out[i]))
		}
		if got := ansi.StringWidth(out[i]); got != want {
			t.Errorf("row %d width %d, want %d", i, got, want)
		}
	}
}

// q-n is the note chord (view-contingent), and the name sort it
// displaced lives on q-l. Both must exist — losing either silently
// would strand a workflow.
func TestNoteChordRegistered(t *testing.T) {
	var note, nameSort *chordSpec
	for _, c := range chordRegistry() {
		c := c
		switch c.Letter {
		case "n":
			note = &c
		case "l":
			nameSort = &c
		}
	}
	if note == nil {
		t.Fatal("q-n note chord missing from registry")
	}
	if !strings.Contains(note.Label, "note") {
		t.Errorf("q-n label should mention notes: %q", note.Label)
	}
	if note.Available == nil {
		t.Error("q-n must be view-contingent (needs a cursored item)")
	}
	if note.Available != nil && note.Available(Model{}) {
		t.Error("q-n must be unavailable on a bare model (no store, no orgs)")
	}
	if nameSort == nil {
		t.Fatal("q-l name sort missing (displaced from q-n)")
	}
	if !strings.Contains(nameSort.Label, "sort") {
		t.Errorf("q-l should be the name sort: %q", nameSort.Label)
	}
}

// padLinesTo pads short content and leaves tall content alone.
func TestPadLinesTo(t *testing.T) {
	if got := strings.Count(padLinesTo("a\nb", 5), "\n") + 1; got != 5 {
		t.Errorf("padded to %d lines, want 5", got)
	}
	tall := "1\n2\n3\n4\n5\n6"
	if padLinesTo(tall, 3) != tall {
		t.Error("padLinesTo must not clip content taller than n")
	}
	if got := strings.Count(padLinesTo("", 3), "\n"); got != 3 {
		t.Errorf("empty input: %d newlines, want 3", got)
	}
}

// End-to-end: with a note on the cursored flow, the sidebar renders
// the NOTE box in BOTH layout modes without disturbing frame geometry.
// This exercises the real path — cursorOpenable → FromOpenable →
// store lookup → renderSidebar composition — not just the helpers.
func TestSidebarNoteBoxBothModes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c, err := cache.Open()
	if err != nil {
		t.Fatalf("cache open: %v", err)
	}
	defer c.Close()
	store, err := devproject.Open()
	if err != nil {
		t.Fatalf("devproject open: %v", err)
	}
	defer store.Close()
	m := New(c).WithDevProjects(store)
	m.width, m.height = 160, 50
	m.focus = focusMain
	m.orgs = []sf.Org{{Alias: "noted", Username: "note@test", Status: "Connected"}}
	m.selected = 0
	d := m.ensureOrgData("note@test")
	d.Flows.Set([]sf.Flow{{
		DefinitionID: "300TESTDEF", DeveloperName: "Noted_Flow",
		MasterLabel: "Noted Flow", ActiveVersionID: "301X",
	}})
	d.SyncFlowsList()
	m.setTab(TabFlows)
	if err := m.devProjects.SetNote(devproject.KindFlow, "300TESTDEF", "note@test", "remember the batch size"); err != nil {
		t.Fatal(err)
	}
	m.sidebarOpen = true

	for _, stacked := range []bool{false, true} {
		m.sidebarStacked = stacked
		frame := ansi.Strip(m.viewImpl().Content)
		mode := "beside"
		if stacked {
			mode = "stacked"
		}
		if !strings.Contains(frame, "NOTE") || !strings.Contains(frame, "remember the batch size") {
			t.Errorf("%s mode: NOTE box missing from frame", mode)
		}
		// The box must be CLOSED — top and bottom borders both present.
		// Regression: the footer-button stamp (row innerH-2) used to
		// overwrite the bottom border in both modes. Height is measured
		// at the box's own left-border column (the outer panels also
		// use rounded corners, so a naive corner scan finds the frame).
		boxH := measureBoxAround(frame, "NOTE")
		if boxH < 0 {
			t.Errorf("%s mode: note box is not a closed border box", mode)
		}
		// Beside mode: a one-line note still gets the 10-row floor.
		if !stacked && boxH >= 0 && boxH != noteBoxBesideMinH {
			t.Errorf("beside mode: box height %d rows, want floor %d", boxH, noteBoxBesideMinH)
		}
		// Frame must stay rectangular — a composition overflow would
		// push some rows wider than the terminal.
		for i, line := range strings.Split(frame, "\n") {
			if w := ansi.StringWidth(line); w > 160 {
				t.Errorf("%s mode: row %d overflows terminal (%d > 160)", mode, i, w)
			}
		}
	}

	// Stacked-mode header spans the FULL panel width: the project pill
	// right-aligns past the note box's left edge (the box starts one
	// row below the title). Regression: the pill used to stop at the
	// narrowed content column.
	if err := m.devProjects.CreateDevProject(devproject.DevProject{ID: "np", Name: "Note Proj"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.devProjects.AddItem(devproject.Item{
		DevProjectID: "np", OrgUser: "note@test", Kind: devproject.KindFlow,
		Ref: "300TESTDEF", Type: "Flow", Name: "Noted Flow",
	}); err != nil {
		t.Fatal(err)
	}
	m.sidebarStacked = true
	frameRows := strings.Split(ansi.Strip(m.viewImpl().Content), "\n")
	pillCol, noteCol := -1, -1
	for _, row := range frameRows {
		if i := strings.Index(row, "Note Proj"); i >= 0 && pillCol < i {
			pillCol = i
		}
		if i := strings.Index(row, "NOTE"); i >= 0 {
			noteCol = i
		}
	}
	if pillCol < 0 || noteCol < 0 {
		t.Fatalf("missing project pill (%d) or NOTE box (%d) in stacked frame", pillCol, noteCol)
	}
	if pillCol <= noteCol {
		t.Errorf("header must span the full width: project pill at col %d should sit past the note box edge (col %d)", pillCol, noteCol)
	}

	// Without a note the box must vanish again.
	if err := m.devProjects.SetNote(devproject.KindFlow, "300TESTDEF", "note@test", ""); err != nil {
		t.Fatal(err)
	}
	frame := ansi.Strip(m.viewImpl().Content)
	if strings.Contains(frame, "remember the batch size") {
		t.Error("removed note still rendering")
	}
}

// measureBoxAround finds the row containing marker, walks up/down the
// box's left-border rune column (marker col - 2: one padding + the
// border) to its rounded corners, and returns the border-inclusive
// height. Returns -1 when the box can't be located.
func measureBoxAround(frame, marker string) int {
	rows := strings.Split(frame, "\n")
	runeRows := make([][]rune, len(rows))
	for i, r := range rows {
		runeRows[i] = []rune(r)
	}
	markerRunes := []rune(marker)
	for r, rr := range runeRows {
		col := runeIndex(rr, markerRunes)
		if col < 2 {
			continue
		}
		border := col - 2
		top, bottom := -1, -1
		for i := r - 1; i >= 0; i-- {
			if border >= len(runeRows[i]) || runeRows[i][border] == ' ' {
				break
			}
			if runeRows[i][border] == '\u256d' {
				top = i
				break
			}
		}
		for i := r + 1; i < len(runeRows); i++ {
			if border >= len(runeRows[i]) || runeRows[i][border] == ' ' {
				break
			}
			if runeRows[i][border] == '\u2570' {
				bottom = i
				break
			}
		}
		if top >= 0 && bottom >= 0 {
			return bottom - top + 1
		}
	}
	return -1
}

// runeIndex returns the rune-column of needle inside haystack, or -1.
func runeIndex(haystack, needle []rune) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
