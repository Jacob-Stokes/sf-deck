package ui

// Tag picker modal — multi-select list of all tags, used by the `t`
// keybind to apply / remove tags on the cursored item.
//
// Shape:
//
//   ╭─ Tags · Account.Phone ────────────────╮
//   │ + new tag…                            │
//   │ ☑ cleanup-q2                          │
//   │ ☐ tech-debt                           │
//   │ ☑ fragile                             │
//   │                                       │
//   │ space toggle · ↵ save · esc cancel    │
//   ╰───────────────────────────────────────╯
//
// Differs from choiceModal by being multi-select with a commit-on-
// enter (rather than commit-on-selection) flow. Reuses the modal
// box primitive + renderModalRows pattern so it visually matches.
//
// On save: computes the diff between the original tag set and the
// checked set, then calls store.SetTagsFor with the new full set.
// Atomic in the store layer (single transaction).

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// tagPickerState is the live state of the tag-picker modal.
type tagPickerState struct {
	// Target — the item the modal is editing tags for.
	Kind    devproject.ItemKind
	Ref     string
	OrgUser string
	Title   string // shown in the modal header

	// Tags is every defined tag (one row per tag). Order is by name.
	Tags []devproject.Tag
	// Selected mirrors the checkbox state — Selected[i] == true iff
	// Tags[i] is currently checked. Initialised from the store on
	// open and toggled by space.
	Selected []bool
	// Cursor is the highlighted row. Position 0 is the special
	// "+ new tag…" row; positions 1..len(Tags) map to Tags[i-1].
	Cursor int

	// NewTagInput holds the in-progress new-tag name when the user
	// is on the "+ new tag" row and has started typing. Empty string
	// = not in new-tag-input mode.
	NewTagInput string

	// Err carries any commit error so the user can retry without
	// losing their selection.
	Err error

	// BulkRefs, when non-empty, switches the picker into bulk mode:
	// the checkbox edits apply to EVERY ref (all of Kind, same org).
	// Checked-at-open = tags every item already shares (the
	// intersection); checking adds to all, unchecking a shared tag
	// removes from all, and partially-applied tags (unchecked at
	// open) are left alone unless explicitly checked.
	BulkRefs []string
	// bulkBaseline records which tag ids were checked at open so the
	// commit can diff added vs removed.
	bulkBaseline map[int64]bool
}

// openTagPicker is the canonical opener. Loads the full tag list
// from the store + the item's current bindings, pre-checks the
// applied tags, and pops the modal.
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

// openBulkTagPicker opens the picker over EVERY visible row of a
// surface — the "tag everything I'm looking at" path (T). Pre-checks
// the intersection of the rows' current tags.
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
	// Intersection: a tag is pre-checked only when EVERY ref has it.
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

// renderTagPicker draws the modal overlay. Returns "" when the
// modal isn't open. Layered into the overlay chain in render.go.
func (m Model) renderTagPicker() string {
	tp := m.tagPicker
	if tp == nil {
		return ""
	}
	header := lipgloss.NewStyle().Foreground(theme.Blue).Bold(true).
		Render("Tags · " + tp.Title)
	var rows []string
	rows = append(rows, header, "")

	// Row 0: "+ new tag" — switches to inline-input when user
	// presses enter on it. The visible label flips to a text-entry
	// hint while NewTagInput is active.
	newRowLabel := "+ new tag…"
	if tp.NewTagInput != "" || tp.Cursor == 0 && tp.NewTagInput == "" {
		// Highlight when cursor is here OR active typing.
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
		// Use the tag's color for the label so the user sees pill-
		// preview color even before saving.
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

// tagPickerRow formats one row with checkbox + label, applying a
// highlight when selected (cursor on this row).
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

// updateTagPicker handles key presses while the modal is open. Esc
// closes; space toggles; enter commits or creates-new; arrows move
// the cursor; printable chars feed the new-tag input when the user
// is on row 0 with inline-input active.
func (m Model) updateTagPicker(msg tea.KeyMsg) (Model, tea.Cmd) {
	tp := m.tagPicker
	if tp == nil {
		return m, nil
	}
	key := msg.String()

	// New-tag inline input mode: most keys feed the buffer.
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
				// First Enter on the "+ new tag" row: enter input mode.
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
			// Commit the new tag.
			name := strings.TrimSpace(tp.NewTagInput)
			if name == "" {
				tp.NewTagInput = ""
				return m, nil
			}
			if m.devProjects == nil {
				tp.Err = errors.New("store unavailable")
				return m, nil
			}
			// Rotate the default color through the palette so tags
			// created inline aren't all blue. Keyed off the existing tag
			// count → each new tag gets the next palette colour; recolour
			// later in /tags (tab to the colour field, ←/→).
			created, err := m.devProjects.CreateTag(name, nextRotatingTagColor(len(tp.Tags)), "")
			if err != nil {
				tp.Err = err
				return m, nil
			}
			// Insert the new tag into the modal's Tags slice in sorted
			// order, mark it selected, and exit input mode.
			tp.Tags = append(tp.Tags, created)
			tp.Selected = append(tp.Selected, true)
			sortTagsByName(tp.Tags, tp.Selected)
			tp.NewTagInput = ""
			tp.Err = nil
			// Find the new tag's index post-sort and move the cursor
			// onto it so the user can see it landed.
			for i, t := range tp.Tags {
				if t.ID == created.ID {
					tp.Cursor = i + 1
					break
				}
			}
			return m, nil
		case "backspace":
			if len(tp.NewTagInput) > 0 {
				// Trim trailing rune. Simple ASCII slice — names are
				// typically short and ASCII; an emoji prefix in a
				// name is unlikely (and our valid-name regex would
				// reject it later anyway).
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
		// Toggle the checkbox on the cursored row. Row 0 has no
		// checkbox so space is a no-op there.
		if tp.Cursor >= 1 && tp.Cursor-1 < len(tp.Selected) {
			tp.Selected[tp.Cursor-1] = !tp.Selected[tp.Cursor-1]
		}
		return m, nil
	case "enter":
		// Commit the diff.
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
			// Bulk: diff against the open-time intersection. Checked
			// and not in baseline -> add everywhere; in baseline and
			// now unchecked -> remove everywhere. Everything else is
			// untouched (partially-tagged rows keep their own tags).
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

// sortTagsByName sorts the (Tags, Selected) slice pair in tandem so
// the row order stays alphabetical after a new tag is created.
func sortTagsByName(tags []devproject.Tag, sel []bool) {
	// Insertion sort — n is small (typically <50 tags).
	for i := 1; i < len(tags); i++ {
		for j := i; j > 0 && strings.ToLower(tags[j-1].Name) > strings.ToLower(tags[j].Name); j-- {
			tags[j-1], tags[j] = tags[j], tags[j-1]
			sel[j-1], sel[j] = sel[j], sel[j-1]
		}
	}
}
