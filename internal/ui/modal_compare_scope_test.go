package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	case " ", "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "} // .String() == "space"
	default:
		r := []rune(s)
		return tea.KeyPressMsg{Code: r[0], Text: s}
	}
}

func loadScope(m *Model, current, types []string, onConfirm func([]string)) {
	m.openCompareScopeModal(current, onConfirm)
	m.applyCompareTypesLoaded(compareTypesLoadedMsg{Alias: "", Types: types})
}

func TestCompareScopeModalToggleAndConfirm(t *testing.T) {
	types := []string{"ApexClass", "ApexTrigger", "Flow", "Layout"}
	var got []string
	m := Model{}
	loadScope(&m, []string{"ApexClass", "Flow"}, types, func(sel []string) { got = sel })
	st := m.compareScope
	if st == nil {
		t.Fatal("scope modal not opened")
	}
	if st.Loading {
		t.Fatal("modal should be loaded after applyCompareTypesLoaded")
	}
	if !st.Checked["ApexClass"] || !st.Checked["Flow"] {
		t.Errorf("pre-tick wrong: %+v", st.Checked)
	}
	if st.countChecked() != 2 {
		t.Errorf("countChecked = %d, want 2", st.countChecked())
	}

	m, _ = m.handleCompareScopeKey(keyMsg("a"))
	if m.compareScope.countChecked() != len(types) {
		t.Errorf("after 'a' = %d, want all %d", m.compareScope.countChecked(), len(types))
	}
	m, _ = m.handleCompareScopeKey(keyMsg("a"))
	if m.compareScope.countChecked() != 0 {
		t.Errorf("after second 'a' = %d, want 0", m.compareScope.countChecked())
	}

	// Enter with none selected → refuses (modal stays, no confirm).
	m, _ = m.handleCompareScopeKey(keyMsg("enter"))
	if m.compareScope == nil {
		t.Error("enter with empty selection should keep modal open")
	}
	if got != nil {
		t.Error("onConfirm should not fire with empty selection")
	}

	m, _ = m.handleCompareScopeKey(keyMsg("down"))
	m, _ = m.handleCompareScopeKey(keyMsg(" "))
	first := m.compareScope.Types[0]
	m, _ = m.handleCompareScopeKey(keyMsg("enter"))
	if m.compareScope != nil {
		t.Error("enter with a selection should close the modal")
	}
	if len(got) != 1 || got[0] != first {
		t.Errorf("confirmed selection = %v, want [%s]", got, first)
	}
}

func TestCompareScopeModalNoDefaultAll(t *testing.T) {
	types := []string{"ApexClass", "Flow", "Layout"}
	// Opening with empty current scope must NOT pre-tick everything.
	m := Model{}
	loadScope(&m, nil, types, func([]string) {})
	if n := m.compareScope.countChecked(); n != 0 {
		t.Errorf("empty-open ticked %d types, want 0 (unticked by default)", n)
	}
	m.compareScope.Cursor = scopeAllRowIdx
	m.compareScope.toggleAtCursor()
	if n := m.compareScope.countChecked(); n != len(types) {
		t.Errorf("All-row toggle checked %d, want all %d", n, len(types))
	}
}

func TestCompareScopeModalOpensLoading(t *testing.T) {
	m := Model{}
	cmd := (&m).openCompareScopeModal([]string{"Flow"}, func([]string) {})
	if cmd == nil {
		t.Fatal("openCompareScopeModal should return a load cmd")
	}
	if m.compareScope == nil || !m.compareScope.Loading {
		t.Fatal("modal should open in Loading state")
	}
	if len(m.compareScope.Types) != 0 {
		t.Errorf("Types should be empty until load lands, got %v", m.compareScope.Types)
	}
	m, _ = m.handleCompareScopeKey(keyMsg("esc"))
	if m.compareScope != nil {
		t.Error("esc during load should close the modal")
	}
}

func TestCompareScopeModalStaleLoadIgnored(t *testing.T) {
	m := Model{}
	(&m).openCompareScopeModal(nil, func([]string) {})
	m.compareScope.Alias = "orgA"
	// A result for a different org must not populate.
	(&m).applyCompareTypesLoaded(compareTypesLoadedMsg{Alias: "orgB", Types: []string{"Flow"}})
	if !m.compareScope.Loading {
		t.Error("stale load (wrong alias) should leave modal still loading")
	}
	(&m).applyCompareTypesLoaded(compareTypesLoadedMsg{Alias: "orgA", Types: []string{"Flow"}})
	if m.compareScope.Loading || len(m.compareScope.Types) != 1 {
		t.Errorf("matching load should populate: loading=%v types=%v", m.compareScope.Loading, m.compareScope.Types)
	}
}

func TestCompareScopeModalSearchFilterAndConfirm(t *testing.T) {
	types := []string{"ApexClass", "ApexTrigger", "Flow", "FlexiPage", "Layout"}
	var got []string
	m := Model{}
	loadScope(&m, nil, types, func(sel []string) { got = sel })

	m, _ = m.handleCompareScopeKey(keyMsg("/"))
	if !m.compareScope.searchActive {
		t.Fatal("/ should enter search mode")
	}
	for _, ch := range []string{"f", "l", "o", "w"} {
		m, _ = m.handleCompareScopeKey(keyMsg(ch))
	}
	if got := m.compareScope.filtered(); len(got) != 1 || got[0] != "Flow" {
		t.Fatalf("filter 'flow' = %v, want [Flow]", got)
	}
	m, _ = m.handleCompareScopeKey(keyMsg("enter"))
	if m.compareScope.searchActive {
		t.Error("enter should leave search-entry mode")
	}
	if m.compareScope.Search != "flow" {
		t.Errorf("applied search = %q, want flow", m.compareScope.Search)
	}
	m.compareScope.Cursor = scopeAllRowIdx
	m, _ = m.handleCompareScopeKey(keyMsg(" "))
	m, _ = m.handleCompareScopeKey(keyMsg("enter"))
	if len(got) != 1 || got[0] != "Flow" {
		t.Errorf("confirmed = %v, want [Flow]", got)
	}
}

func TestCompareScopeModalSearchToggleOnlyFiltered(t *testing.T) {
	types := []string{"ApexClass", "ApexTrigger", "Flow"}
	m := Model{}
	loadScope(&m, nil, types, func([]string) {})
	m, _ = m.handleCompareScopeKey(keyMsg("/"))
	for _, ch := range []string{"a", "p", "e", "x"} {
		m, _ = m.handleCompareScopeKey(keyMsg(ch))
	}
	m, _ = m.handleCompareScopeKey(keyMsg("enter")) // apply
	m, _ = m.handleCompareScopeKey(keyMsg("a"))     // toggle-all (filtered)
	if !m.compareScope.Checked["ApexClass"] || !m.compareScope.Checked["ApexTrigger"] {
		t.Error("'a' under filter should tick the Apex types")
	}
	if m.compareScope.Checked["Flow"] {
		t.Error("'a' under filter must NOT tick the non-matching Flow")
	}
}

func TestCompareScopeModalScrollWindowFollowsCursor(t *testing.T) {
	var types []string
	for i := 0; i < scopeVisibleRows+5; i++ {
		types = append(types, "Type"+itoa(i))
	}
	m := Model{}
	loadScope(&m, nil, types, func([]string) {})
	// Walk the cursor to the last type; offset must have advanced so the
	// cursored type stays inside the window.
	for i := 0; i <= len(types); i++ {
		m, _ = m.handleCompareScopeKey(keyMsg("down"))
	}
	st := m.compareScope
	if st.Cursor != len(types) {
		t.Fatalf("cursor = %d, want %d (last type)", st.Cursor, len(types))
	}
	lastTypeIdx := st.Cursor - 1
	if lastTypeIdx < st.Offset || lastTypeIdx >= st.Offset+scopeVisibleRows {
		t.Errorf("cursored type %d not visible in window [%d,%d)", lastTypeIdx, st.Offset, st.Offset+scopeVisibleRows)
	}
}

func TestCompareScopeModalEscClearsSearchThenCloses(t *testing.T) {
	types := []string{"Flow", "Layout"}
	m := Model{}
	loadScope(&m, nil, types, func([]string) {})
	m, _ = m.handleCompareScopeKey(keyMsg("/"))
	m, _ = m.handleCompareScopeKey(keyMsg("f"))
	m, _ = m.handleCompareScopeKey(keyMsg("enter"))
	m, _ = m.handleCompareScopeKey(keyMsg("esc"))
	if m.compareScope == nil {
		t.Fatal("first esc (with search) should keep modal open")
	}
	if m.compareScope.Search != "" {
		t.Errorf("first esc should clear search, got %q", m.compareScope.Search)
	}
	m, _ = m.handleCompareScopeKey(keyMsg("esc"))
	if m.compareScope != nil {
		t.Error("second esc should close the modal")
	}
}
