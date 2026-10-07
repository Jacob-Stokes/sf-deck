package ui

// Tab-bar slot editor — the settings-modal path for choosing which
// tabs occupy number slots 1-8 (the same set the "0 More…" overflow
// picker could already reorder, now editable from Settings → Startup
// & defaults → Tab bar slots).

import (
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"
)

const tabBarSlots = 8

func tabBarPinnedIDs() []string {
	out := make([]string, tabBarSlots)
	cur := TabsForNumbers()
	for i := 0; i < tabBarSlots && i < len(cur); i++ {
		out[i] = cur[i].String()
	}
	return out
}

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
			return func() tea.Msg { return openSettingsSubmenuMsg{pick: "startup.tab_bar"} }
		},
		OnCancel: func() tea.Cmd {
			return func() tea.Msg { return openSettingsSubmenuMsg{pick: "startup.tab_bar"} }
		},
	}
	return m.openChoiceModal(state)
}

func (m *Model) applyTabBarSlot(slot int, id string) {
	if m.settings == nil {
		return
	}
	ids := tabBarPinnedIDs()
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

func (m *Model) applyTabBarReset() {
	if m.settings == nil {
		return
	}
	m.settings.SetPinnedTabs(defaultPinnedTabIDs())
	m.persistTabBar("tab bar reset to defaults")
}

func (m *Model) persistTabBar(msg string) {
	RebuildTabsForNumbers(m.settings.PinnedTabs())
	_ = m.settings.Save()
	m.flash(msg)
}

func compactTabIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func defaultPinnedTabIDs() []string {
	def := defaultPinnedTabs()
	out := make([]string, 0, len(def))
	for _, t := range def {
		out = append(out, t.String())
	}
	return out
}
