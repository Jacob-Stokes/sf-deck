package ui

// modal_tab_overflow.go — slot 9 ("More…") opens a choice modal
// listing every tab not currently pinned to slots 1-8. Selecting
// jumps to that tab.
//
// Future: per-row pin/unpin toggle so users can curate the bar
// from inside the modal. For now it's navigation only — pin
// management lives in the Settings → Misc tab.

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// openTabOverflowModal lists every top-level tab the user could
// jump to that ISN'T currently visible in the strip. That's:
//   - pinned tabs the fit logic dropped on a narrow window
//     (listed first, in strip order — so the first tab that fell
//     off the right edge appears at the top of the modal, which
//     is what users reach for when they shrink the window), then
//   - non-pinned tabs (their permanent home).
//
// Pinned-and-visible tabs are excluded (strip already shows them).
// The active tab is excluded (no point jumping to where you are).
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
	// 1) Dropped-pinned, in strip order — the most useful: "tabs
	// that fell off the right edge of my strip just now."
	for _, t := range pinned {
		if t == active || visible[t] {
			continue
		}
		add(t)
	}
	// 2) Non-pinned tabs, in allPinnableTabs() declaration order.
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
		// No-op Save — the actual tab switch happens in Update via
		// the synthetic msg below. Mutating the captured *Model from
		// inside Save wouldn't stick: the pointer is taken from the
		// transient Model copy at modal-open time, which goes out of
		// scope before bubbletea applies the result message. Emit a
		// msg instead and let Update mutate the live model.
		Save: func(val any) error { return nil },
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
