package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) openTabOverflowModal() tea.Cmd {
	active := m.tab()
	visible := m.visiblePinnedTabs()
	pinned := TabsForNumbers()
	pinnedSet := map[Tab]bool{}
	for _, t := range pinned {
		pinnedSet[t] = true
	}
	opts := make([]choiceOption, 0, len(pinned)+8)
	add := func(t Tab) {
		opts = append(opts, choiceOption{
			Label: fmt.Sprintf("/%s", t.String()),
			Hint:  tabOverflowHint(t),
			Value: t.String(),
		})
	}
	for _, t := range pinned {
		if t == active || visible[t] {
			continue
		}
		add(t)
	}
	for _, t := range allPinnableTabs() {
		if t == active || pinnedSet[t] {
			continue
		}
		add(t)
	}
	if len(opts) == 0 {
		m.flash("no other tabs to jump to")
		return nil
	}
	opts = append(opts, choiceOption{Label: "Cancel", Cancel: true})

	return m.openChoiceModal(choiceModalState{
		Title:      "More tabs",
		Hint:       "Enter to jump · Esc to cancel",
		Options:    opts,
		Cursor:     0,
		Searchable: len(opts) > 6,
		Save:       func(val any) error { return nil },
		OnSuccessTyped: func(val any) tea.Cmd {
			id, _ := val.(string)
			if id == "" {
				return nil
			}
			return func() tea.Msg { return tabOverflowPickedMsg{ID: id} }
		},
	})
}

// tabOverflowPickedMsg carries the picked overflow tab's string
// ID. Update handles it by switching to that tab on the live
// model — which is what gets the slot 0 pill to appear.
type tabOverflowPickedMsg struct {
	ID string
}

// tabOverflowHint returns a short description shown next to each
// overflow tab in the picker. Helps users recognize what each tab
// IS without having to navigate to it first. Hints live on each
// registry entry's OverflowHint field; blank falls back to the slug.
func tabOverflowHint(t Tab) string {
	if spec := lookupTabSpec(t); spec != nil && spec.OverflowHint != "" {
		return spec.OverflowHint
	}
	return t.String()
}
