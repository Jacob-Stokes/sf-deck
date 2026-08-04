package ui

// modal_subtab_overflow.go — slot 0 ("More…") on the subtab strip
// opens a choice modal listing every subtab that doesn't fit on the
// pinned strip. Selecting jumps to that subtab via the standard
// SetSubtabIdx / setObjectSubtab dispatch path, just like a
// keyboard shift+N press would.
//
// Mirrors modal_tab_overflow.go for the top-level "0 Tabs" overflow.
// Shape is identical so the user gets the same gesture across both
// hierarchies.

import (
	tea "charm.land/bubbletea/v2"
)

// openSubtabOverflowModal lists the active tab's overflow subtabs in
// the standard choice modal. Active when the tab has more subtabs
// than fit on the strip; for /objects · object detail today that's
// Triggers / Layouts / Flows.
func (m *Model) openSubtabOverflowModal() tea.Cmd {
	overflow := m.tabSubtabsOverflow()
	if len(overflow) == 0 {
		return nil
	}
	pinned, _ := m.subtabPinSplit()
	opts := make([]choiceOption, 0, len(overflow))
	for i, sub := range overflow {
		// Value = the FULL-list index (pinned + offset), so the
		// subtab dispatcher can route through the standard
		// SetSubtabIdx contract without overflow-aware logic.
		opts = append(opts, choiceOption{
			Label: sub.Label,
			Value: pinned + i,
		})
	}
	opts = append(opts, choiceOption{Label: "Cancel", Cancel: true})
	return m.openChoiceModal(choiceModalState{
		Title:      "More subtabs",
		Hint:       "Enter to jump · Esc to cancel",
		Options:    opts,
		Cursor:     0,
		Searchable: len(opts) > 6,
		// Save is a no-op — actual switch happens via the message
		// emitted in OnSuccessTyped so it lands on the live model
		// rather than the modal's captured copy.
		Save: func(val any) error { return nil },
		OnSuccessTyped: func(val any) tea.Cmd {
			idx, _ := val.(int)
			return func() tea.Msg { return subtabOverflowPickedMsg{Index: idx} }
		},
	})
}

// subtabOverflowPickedMsg carries the picked overflow subtab's
// FULL-list index. Update applies it to the live model via
// switchToSubtabIndex (same path shift+N takes).
type subtabOverflowPickedMsg struct {
	Index int
}
