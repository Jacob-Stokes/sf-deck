package ui

// Item notes — one free-text note per cursored item, stored locally in
// the devproject store (same (kind, ref, org) identity as tags, no
// project membership needed).
//
//   q-n         open the note modal (add / edit; save empty to remove)
//   sidebar     the cursored item's note renders in a NOTE box —
//               right ~1/4 of the panel (full height) in stacked mode;
//               content-sized at the bottom (10-row floor, 1/3 cap) in
//               beside mode — with a q-n hint when clipped.
//
// Lookup is per-frame (the sidebar renders on every tick), so the body
// read goes through a memo on orgData keyed by item identity + store
// generation: SQLite is only touched on cursor move or after a write.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// cursorNoteTarget resolves the cursored item to the (kind, ref, name)
// identity notes are keyed by. Same resolution order as collect:
// Openable first (carries the richer name), Identity fallback for
// kinds with no Lightning URL.
func (m Model) cursorNoteTarget() (kind devproject.ItemKind, ref, name string, ok bool) {
	if target := m.cursorOpenable(); target != nil {
		if k, r, _, nm, o := devproject.FromOpenable(target); o {
			return k, r, nm, true
		}
	}
	if id, o := m.resolveItemIdentity(); o && id.Kind != "" && id.Ref != "" {
		return id.Kind, id.Ref, id.Label, true
	}
	return "", "", "", false
}

// cursorNoteBody returns the note for the cursored item, or "" when
// there is none (or notes are unavailable). Memoised on orgData — see
// the package comment for why.
func (m Model) cursorNoteBody() string {
	if m.devProjects == nil || len(m.orgs) == 0 {
		return ""
	}
	kind, ref, _, ok := m.cursorNoteTarget()
	if !ok {
		return ""
	}
	user := m.orgs[m.selected].Username
	d := m.activeOrgData()
	if d == nil {
		return ""
	}
	key := string(kind) + "\x00" + ref + "\x00" + user
	gen := m.devProjects.Generation()
	if memo := d.noteMemo; memo != nil && memo.key == key && memo.generation == gen {
		return memo.body
	}
	body, err := m.devProjects.NoteFor(kind, ref, user)
	if err != nil {
		applog.Warn("note lookup failed", map[string]any{
			"kind": string(kind), "ref": ref, "err": err.Error(),
		})
		body = ""
	}
	d.noteMemo = &noteMemoEntry{key: key, generation: gen, body: body}
	return body
}

// openNoteModal opens the multiline note editor for the cursored item.
// Pre-filled with the existing note; ctrl+s saves; saving an empty
// buffer removes the note.
func (m *Model) openNoteModal() tea.Cmd {
	if m.devProjects == nil {
		m.flash("notes unavailable — local store failed to open")
		return nil
	}
	if len(m.orgs) == 0 {
		m.flash("no org selected")
		return nil
	}
	kind, ref, name, ok := m.cursorNoteTarget()
	if !ok {
		m.flash("nothing to note here — put the cursor on an item first")
		return nil
	}
	user := m.orgs[m.selected].Username
	current, err := m.devProjects.NoteFor(kind, ref, user)
	if err != nil {
		m.flash("note: " + err.Error())
		return nil
	}
	title := "Note — " + name
	hint := "one note per item · shows in the sidebar · save empty to remove"
	if current == "" {
		hint = "one note per item · shows in the sidebar"
	}
	store := m.devProjects
	removed := false
	return m.openEditModal(editModalState{
		Title:       title,
		Hint:        hint,
		InitialBody: current,
		Multiline:   true,
		Save: func(val string, _ any) error {
			removed = strings.TrimSpace(val) == ""
			return store.SetNote(kind, ref, user, val)
		},
		OnSuccess: func() tea.Cmd {
			return func() tea.Msg {
				if removed {
					return demoFlashMsg{text: "note removed"}
				}
				return demoFlashMsg{text: "note saved — shown in the sidebar"}
			}
		},
	})
}

// noteChord is the q-n chord: add / edit the cursored item's note.
func noteChord() chordSpec {
	return chordSpec{
		Letter: "n",
		Label:  "n note (add/edit)",
		Available: func(m Model) bool {
			if m.devProjects == nil || len(m.orgs) == 0 {
				return false
			}
			_, _, _, ok := m.cursorNoteTarget()
			return ok
		},
		Do: func(m Model) (Model, tea.Cmd) {
			mm := &m
			cmd := mm.openNoteModal()
			return *mm, cmd
		},
	}
}

// ----------------------------------------------------------------------
// Sidebar NOTE box rendering
// ----------------------------------------------------------------------

// Minimum space before the note box renders. Below these the sidebar
// is too cramped to split — the note stays reachable via q-n.
const (
	noteBoxMinStackedInner = 40 // stacked: total inner cols before the ~1/4 split
	noteBoxMinBesideInnerH = 20 // beside: total inner rows before carving the bottom box
	noteBoxBesideMinH      = 10 // beside: box floor — short notes still get a stable 10-row box
)

// noteBoxNeededHeight returns the box height (border included) that
// fits the whole note at box width w: wrapped body lines + title row +
// two border rows. The beside-mode sidebar sizes the box to this,
// capped at a fraction of the panel — see renderSidebar.
func noteBoxNeededHeight(w int, body string) int {
	innerW := w - 4
	if innerW < 8 {
		innerW = 8
	}
	lines := strings.Count(ansi.Wrap(strings.TrimRight(body, "\n"), innerW, ""), "\n") + 1
	return lines + 3
}

// renderNoteBox draws the bordered NOTE panel at exactly w cols ×
// h rows (border included). The body word-wraps; when it doesn't fit,
// the last line becomes a dim "q n → full note" hint — the modal is
// the expansion surface.
func renderNoteBox(w, h int, body string) string {
	if w < 12 {
		w = 12
	}
	if h < 3 {
		h = 3
	}
	innerW := w - 4 // border (2) + 1 padding each side
	innerH := h - 2 // border rows
	title := lipgloss.NewStyle().Foreground(theme.Cyan).Bold(true).Render("NOTE")
	editHint := theme.Subtle.Render("q n")
	titleLine := title
	if gap := innerW - ansi.StringWidth("NOTE") - ansi.StringWidth("q n"); gap >= 2 {
		titleLine = title + strings.Repeat(" ", gap) + editHint
	}
	bodyBudget := innerH - 1 // minus title row
	wrapped := strings.Split(ansi.Wrap(strings.TrimRight(body, "\n"), innerW, ""), "\n")
	if bodyBudget >= 1 && len(wrapped) > bodyBudget {
		keep := bodyBudget - 1
		if keep < 0 {
			keep = 0
		}
		wrapped = append(wrapped[:keep], theme.Subtle.Render("… q n → full note"))
	}
	lines := append([]string{titleLine}, wrapped...)
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Width(w). // measured: lipgloss Width here is the FINAL framed width (border included)
		MaxHeight(h).
		Render(strings.Join(lines, "\n"))
}

// padLinesTo pads s with empty lines to exactly n lines (no-op when
// already >= n). Keeps the beside-mode note box anchored to the panel
// bottom even when the content above is short.
func padLinesTo(s string, n int) string {
	got := strings.Count(s, "\n") + 1
	if s == "" {
		got = 0
	}
	if got >= n {
		return s
	}
	return s + strings.Repeat("\n", n-got)
}

// joinNoteBeside composes the stacked-mode sidebar row-by-row: content
// (already resolved at contentW) on the left, the note box on the
// right, starting at row 1 so the panel's title row stays full-width
// (the caller sizes the box to also clear the footer-button row).
func joinNoteBeside(content, box string, contentW, innerH int) string {
	cl := strings.Split(content, "\n")
	bl := strings.Split(box, "\n")
	out := make([]string, innerH)
	for i := 0; i < innerH; i++ {
		left := ""
		if i < len(cl) {
			left = cl[i]
		}
		right := ""
		if i >= 1 && i-1 < len(bl) {
			right = bl[i-1]
		}
		if right == "" {
			out[i] = left
			continue
		}
		if lw := ansi.StringWidth(left); lw > contentW {
			left = ansi.Truncate(left, contentW, "…")
		}
		pad := contentW + 2 - ansi.StringWidth(left)
		if pad < 1 {
			pad = 1
		}
		out[i] = left + strings.Repeat(" ", pad) + right
	}
	return strings.Join(out, "\n")
}
