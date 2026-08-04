package ui

// Tab-bar slot editor — the settings-modal path for choosing which
// tabs occupy number slots 1-8 (the same set the "0 More…" overflow
// picker could already reorder, now editable from Settings → Startup
// & defaults → Tab bar slots).
//
// UX: a submenu lists slots 1-8, each showing its current tab. Picking
// a slot opens a tab picker (every pinnable tab + "empty"); the pick
// replaces that slot, persists, and rebuilds the live number bar so
// the strip updates immediately. A "Reset to defaults" row clears the
// override.
//
// Slots are edited in place rather than via drag-reorder because the
// choiceModal primitive is a flat picker — a slot-at-a-time model maps
// onto it cleanly and needs no new widget.

import (
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"
)

const tabBarSlots = 8

// tabBarPinnedIDs returns the current slot assignment as a fixed-length
// (tabBarSlots) slice of tab id strings, padding trailing empties with
// "". Resolves from the live TabsForNumbers() so it reflects defaults
// when the user hasn't set an override yet.
func tabBarPinnedIDs() []string {
	out := make([]string, tabBarSlots)
	cur := TabsForNumbers()
	for i := 0; i < tabBarSlots && i < len(cur); i++ {
		out[i] = cur[i].String()
	}
	return out
}

// openTabBarModal lists the 8 slots plus reset. Each slot row shows the
// tab currently assigned (or "empty").
func (m *Model) openTabBarModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	ids := tabBarPinnedIDs()
	opts := make([]choiceOption, 0, tabBarSlots+2)
	for i := 0; i < tabBarSlots; i++ {
		label := fmt.Sprintf("Slot %d", i+1)
		hint := "empty — press to assign"
		if ids[i] != "" {
			if t, ok := tabByID(ids[i]); ok {
				hint = "/" + t.String() + " · " + tabOverflowHint(t)
			} else {
				hint = ids[i]
			}
		}
		opts = append(opts, choiceOption{Label: label, Hint: hint, Value: "slot." + strconv.Itoa(i)})
	}
	opts = append(opts, choiceOption{
		Label: "Reset to defaults",
		Hint:  "home · soql · objects · flows · apex · users · perms · system",
		Value: "reset",
	})
	return m.settingsSubmenu("Tab bar slots (1-8)", "startup.tab_bar", opts)
}

// openTabBarSlotPicker assigns one slot. Lists every pinnable tab plus
// an "empty" row (which shortens the bar by clearing that slot). Picking
// a tab that already occupies another slot moves it here (the duplicate
// is stripped on save so a tab never claims two number keys).
func (m *Model) openTabBarSlotPicker(slot int) tea.Cmd {
	if m.settings == nil || slot < 0 || slot >= tabBarSlots {
		return nil
	}
	ids := tabBarPinnedIDs()
	current := ""
	if slot < len(ids) {
		current = ids[slot]
	}

	opts := make([]choiceOption, 0, len(allPinnableTabs())+2)
	opts = append(opts, choiceOption{
		Label: "Empty (remove from bar)",
		Hint:  "leave this slot unassigned",
		Value: "",
	})
	for _, t := range allPinnableTabs() {
		hint := tabOverflowHint(t)
		if t.String() == current {
			hint = "current · " + hint
		}
		opts = append(opts, choiceOption{Label: "/" + t.String(), Hint: hint, Value: t.String()})
	}

	cursor := 0
	for i, o := range opts {
		if v, _ := o.Value.(string); v == current {
			cursor = i
			break
		}
	}

	state := choiceModalState{
		Title:      fmt.Sprintf("Slot %d — pick a tab", slot+1),
		Hint:       "Enter to assign  ·  Esc to go back",
		Options:    opts,
		Cursor:     cursor,
		Searchable: true,
		OnSuccessTyped: func(val any) tea.Cmd {
			id, _ := val.(string)
			m.applyTabBarSlot(slot, id)
			// Re-open the slot list so the user can keep editing.
			return func() tea.Msg { return openSettingsSubmenuMsg{pick: "startup.tab_bar"} }
		},
		// Esc returns to the slot list rather than closing settings.
		OnCancel: func() tea.Cmd {
			return func() tea.Msg { return openSettingsSubmenuMsg{pick: "startup.tab_bar"} }
		},
	}
	return m.openChoiceModal(state)
}

// applyTabBarSlot sets slot to id (empty id clears it), strips any
// duplicate of id from the other slots (a tab occupies at most one
// number key), drops trailing empties, persists, and rebuilds the live
// number bar.
func (m *Model) applyTabBarSlot(slot int, id string) {
	if m.settings == nil {
		return
	}
	ids := tabBarPinnedIDs()
	// Remove id from any slot it already holds so assigning it here
	// moves it rather than duplicating.
	if id != "" {
		for i := range ids {
			if i != slot && ids[i] == id {
				ids[i] = ""
			}
		}
	}
	if slot < len(ids) {
		ids[slot] = id
	}
	m.settings.SetPinnedTabs(compactTabIDs(ids))
	m.persistTabBar("tab bar updated")
}

// applyTabBarReset clears the user override so the bar falls back to
// the default 8, and rebuilds.
func (m *Model) applyTabBarReset() {
	if m.settings == nil {
		return
	}
	// Clearing the stored slice with UserSetPinned=false would be ideal,
	// but SetPinnedTabs always marks it user-set; writing the default
	// ids explicitly gives the same visible result and is simplest.
	m.settings.SetPinnedTabs(defaultPinnedTabIDs())
	m.persistTabBar("tab bar reset to defaults")
}

// persistTabBar saves settings and rebuilds the number-bar cache so the
// strip reflects the new slots without a restart.
func (m *Model) persistTabBar(msg string) {
	RebuildTabsForNumbers(m.settings.PinnedTabs())
	_ = m.settings.Save()
	m.flash(msg)
}

// compactTabIDs drops empty ("") entries, preserving order — so a
// cleared middle slot closes the gap rather than leaving a hole that
// would shift the number keys confusingly.
func compactTabIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// defaultPinnedTabIDs is the id-string form of defaultPinnedTabs, used
// by the reset path.
func defaultPinnedTabIDs() []string {
	def := defaultPinnedTabs()
	out := make([]string, 0, len(def))
	for _, t := range def {
		out = append(out, t.String())
	}
	return out
}
