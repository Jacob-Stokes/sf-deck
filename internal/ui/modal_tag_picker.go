package ui

// Tag picker modal — multi-select list of all tags, used by the `t`
// keybind to apply / remove tags on the cursored item.

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

type tagPickerState struct {
	Kind    devproject.ItemKind
	Ref     string
	OrgUser string
	Title   string // shown in the modal header

	Tags     []devproject.Tag
	Selected []bool
	Cursor   int

	NewTagInput string

	Err error

	BulkRefs     []string
	bulkBaseline map[int64]bool
}

func (m *Model) openTagPicker(kind devproject.ItemKind, ref, orgUser, title string) tea.Cmd {
	if m.devProjects == nil {
		m.flash("tags unavailable: devproject store not loaded")
		return nil
	}
	if ref == "" {
		m.flash("nothing to tag here")
		return nil
	}
	all, err := m.devProjects.ListTags()
	if err != nil {
		m.flash("list tags: " + err.Error())
		return nil
	}
	current, err := m.devProjects.TagsFor(kind, ref, orgUser)
	if err != nil {
		m.flash("load tags for item: " + err.Error())
		return nil
	}
	currentSet := map[int64]bool{}
	for _, t := range current {
		currentSet[t.ID] = true
	}
	selected := make([]bool, len(all))
	for i, t := range all {
		selected[i] = currentSet[t.ID]
	}
	m.tagPicker = &tagPickerState{
		Kind:     kind,
		Ref:      ref,
		OrgUser:  orgUser,
		Title:    title,
		Tags:     all,
		Selected: selected,
		Cursor:   0,
	}
	return nil
}

func (m *Model) openBulkTagPicker(kind devproject.ItemKind, refs []string, orgUser, title string) tea.Cmd {
	if m.devProjects == nil {
		m.flash("tags unavailable: devproject store not loaded")
		return nil
	}
	if len(refs) == 0 {
		m.flash("nothing visible to tag")
		return nil
	}
	all, err := m.devProjects.ListTags()
	if err != nil {
		m.flash("list tags: " + err.Error())
		return nil
	}
	keys := make([]devproject.TagLookupKey, len(refs))
	for i, r := range refs {
		keys[i] = devproject.TagLookupKey{Kind: kind, Ref: r}
	}
	byItem, err := m.devProjects.TagsForItems(orgUser, keys)
	if err != nil {
		m.flash("load tags: " + err.Error())
		return nil
	}
	counts := map[int64]int{}
	for _, r := range refs {
		for _, t := range byItem[string(kind)+":"+r] {
			counts[t.ID]++
		}
	}
	baseline := map[int64]bool{}
	selected := make([]bool, len(all))
	for i, t := range all {
		if counts[t.ID] == len(refs) {
			selected[i] = true
			baseline[t.ID] = true
		}
	}
	m.tagPicker = &tagPickerState{
		Kind:         kind,
		OrgUser:      orgUser,
		Title:        title,
		Tags:         all,
		Selected:     selected,
		Cursor:       0,
		BulkRefs:     refs,
		bulkBaseline: baseline,
	}
	return nil
}

func (m Model) renderTagPicker() string {
	tp := m.tagPicker
	if tp == nil {
		return ""
	}
	header := lipgloss.NewStyle().Foreground(theme.Blue).Bold(true).
		Render("Tags · " + tp.Title)
	var rows []string
	rows = append(rows, header, "")

	newRowLabel := "+ new tag…"
	if tp.NewTagInput != "" || tp.Cursor == 0 && tp.NewTagInput == "" {
	}
	if tp.NewTagInput != "" {
		newRowLabel = "+ " + tp.NewTagInput + "▏"
	}
	rows = append(rows, tagPickerRow(newRowLabel, false, tp.Cursor == 0, false))

	for i, t := range tp.Tags {
		label := t.Name
		if t.Icon != "" {
			label = t.Icon + " " + label
		}
		colored := lipgloss.NewStyle().Foreground(tagColorFor(t.Color)).Render(label)
		rows = append(rows, tagPickerRow(colored, tp.Selected[i], tp.Cursor == i+1, true))
	}

	rows = append(rows, "")
	hintText := "space toggle  ·  ↵ save  ·  esc cancel"
	if tp.NewTagInput != "" {
		hintText = "type tag name  ·  ↵ create  ·  esc cancel"
	} else if tp.Cursor == 0 {
		hintText = "↵ new tag  ·  ↓ existing tags"
	}
	rows = append(rows, lipgloss.NewStyle().Foreground(theme.Muted).Render(hintText))
	if tp.Err != nil {
		rows = append(rows,
			lipgloss.NewStyle().Foreground(theme.Red).Render("error: "+tp.Err.Error()))
	}

	width := modalWidth(m.width, 40, 70)
	return modalBox(strings.Join(rows, "\n"), width)
}

func tagPickerRow(label string, checked, highlighted, hasCheckbox bool) string {
	box := "  "
	if hasCheckbox {
		if checked {
			box = "☑ "
		} else {
			box = "☐ "
		}
	}
	prefix := "  "
	style := lipgloss.NewStyle().Foreground(theme.Fg)
	if highlighted {
		prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌ ")
		style = style.Bold(true)
	}
	return prefix + style.Render(box+label)
}

func (m Model) updateTagPicker(msg tea.KeyMsg) (Model, tea.Cmd) {
	tp := m.tagPicker
	if tp == nil {
		return m, nil
	}
	key := msg.String()

	if tp.NewTagInput != "" || (tp.Cursor == 0 && key == "enter") {
		switch key {
		case "esc":
			if tp.NewTagInput != "" {
				tp.NewTagInput = ""
				return m, nil
			}
			m.tagPicker = nil
			return m, nil
		case "enter":
			if tp.NewTagInput == "" {
				tp.NewTagInput = ""
				// We use an empty buffer + a marker (cursor==0 + Enter
				// pressed) — bump to a single space stand-in that we'll
				// strip later. Simpler: just toggle into input mode by
				// setting NewTagInput to a non-empty placeholder which
				// will get backspaced out by the user. Actually best:
				// set a sentinel that the renderer reads as "input
				// mode."
				tp.NewTagInput = " " // sentinel; backspaced before commit
				return m, nil
			}
			name := strings.TrimSpace(tp.NewTagInput)
			if name == "" {
				tp.NewTagInput = ""
				return m, nil
			}
			if m.devProjects == nil {
				tp.Err = errors.New("store unavailable")
				return m, nil
			}
			created, err := m.devProjects.CreateTag(name, nextRotatingTagColor(len(tp.Tags)), "")
			if err != nil {
				tp.Err = err
				return m, nil
			}
			tp.Tags = append(tp.Tags, created)
			tp.Selected = append(tp.Selected, true)
			sortTagsByName(tp.Tags, tp.Selected)
			tp.NewTagInput = ""
			tp.Err = nil
			for i, t := range tp.Tags {
				if t.ID == created.ID {
					tp.Cursor = i + 1
					break
				}
			}
			return m, nil
		case "backspace":
			if len(tp.NewTagInput) > 0 {
				tp.NewTagInput = tp.NewTagInput[:len(tp.NewTagInput)-1]
				if tp.NewTagInput == "" {
					tp.NewTagInput = " " // keep sentinel until esc
				}
			}
			return m, nil
		}
		// Printable chars append. Treat the sentinel space as "empty"
		// so the first letter doesn't carry a leading space.
		if len(key) == 1 && key[0] >= 0x20 && key[0] < 0x7f {
			if tp.NewTagInput == " " {
				tp.NewTagInput = key
			} else {
				tp.NewTagInput += key
			}
		}
		return m, nil
	}

	switch key {
	case "esc":
		m.tagPicker = nil
		return m, nil
	case "up", "k":
		if tp.Cursor > 0 {
			tp.Cursor--
		}
		return m, nil
	case "down", "j":
		if tp.Cursor < len(tp.Tags) {
			tp.Cursor++
		}
		return m, nil
	case " ", "space":
		if tp.Cursor >= 1 && tp.Cursor-1 < len(tp.Selected) {
			tp.Selected[tp.Cursor-1] = !tp.Selected[tp.Cursor-1]
		}
		return m, nil
	case "enter":
		if m.devProjects == nil {
			tp.Err = errors.New("store unavailable")
			return m, nil
		}
		ids := make([]int64, 0, len(tp.Tags))
		for i, t := range tp.Tags {
			if tp.Selected[i] {
				ids = append(ids, t.ID)
			}
		}
		if len(tp.BulkRefs) > 0 {
			var add, remove []int64
			checked := map[int64]bool{}
			for i, t := range tp.Tags {
				if tp.Selected[i] {
					checked[t.ID] = true
					if !tp.bulkBaseline[t.ID] {
						add = append(add, t.ID)
					}
				}
			}
			for id := range tp.bulkBaseline {
				if !checked[id] {
					remove = append(remove, id)
				}
			}
			if len(add) == 0 && len(remove) == 0 {
				m.tagPicker = nil
				return m, nil
			}
			if err := m.devProjects.BulkApplyRemoveTags(tp.Kind, tp.OrgUser, tp.BulkRefs, add, remove); err != nil {
				tp.Err = err
				return m, nil
			}
			n := len(tp.BulkRefs)
			m.tagPicker = nil
			m.flash(fmt.Sprintf("%d tag change(s) applied to %d rows", len(add)+len(remove), n))
			return m, nil
		}
		if err := m.devProjects.SetTagsFor(tp.Kind, tp.Ref, tp.OrgUser, ids); err != nil {
			tp.Err = err
			return m, nil
		}
		count := len(ids)
		m.tagPicker = nil
		m.flash(fmt.Sprintf("tags applied: %d", count))
		return m, nil
	}
	return m, nil
}

func sortTagsByName(tags []devproject.Tag, sel []bool) {
	for i := 1; i < len(tags); i++ {
		for j := i; j > 0 && strings.ToLower(tags[j-1].Name) > strings.ToLower(tags[j].Name); j-- {
			tags[j-1], tags[j] = tags[j], tags[j-1]
			sel[j-1], sel[j] = sel[j], sel[j-1]
		}
	}
}
