package ui

// Chip overflow modal — the "+ N more…" picker. Opened when the user
// activates the overflow sentinel at the end of the chip strip.
// Lists every non-favourite chip for the current scope; picking one
// sets it as the active chip on the strip without adding it to the
// favourites list (a one-off "I want this view right now" action).
//
// Re-uses internal/ui/picker.go — generic anchored dropdown over
// qchip.Chip. Same widget the field picker + value picker use.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

// openChipOverflowFor opens the overflow picker for the given domain
// and scope. Resolves the registry, gathers EVERY chip applicable to
// the scope (favourites + others), and opens an anchored picker. On
// pick, the chosen chip becomes the active selection on the strip
// (writes the cursor index for /objects + /flows, or
// ListViewCur[sobj] for the records subtab).
//
// Why every chip and not just non-favourites: when many favourites
// are pinned the strip itself truncates with `…`, so a favourite the
// user pinned may not be reachable from the visible strip. The modal
// is therefore the canonical "every applicable view" picker, not
// just "the overflow set" — same UX as a command palette.
func (m *Model) openChipOverflowFor(domain chipDomain, scope string) tea.Cmd {
	reg := m.registryFor(domain)
	if reg == nil {
		return nil
	}
	all := reg.ChipsFor(scope)
	if len(all) == 0 {
		m.flash("no views for this surface")
		return nil
	}

	wW := modalWidth(m.width, 56, 90)
	wX := (m.width - wW) / 2
	pickerW := wW * 2 / 3
	if pickerW < 48 {
		pickerW = 48
	}
	if pickerW > m.width-4 {
		pickerW = m.width - 4
	}
	anchorX := wX + 4
	// Anchor below the chip strip with a one-row gap, not glued to
	// the top edge of the main pane. The header + tab bar + strip
	// take ~6 rows; sit one row below that so the picker visibly
	// belongs to the strip without flush-bumping the panel border.
	anchorY := 7

	// Take most of the terminal height — the picker default is 12
	// rows, which truncates real chip catalogs (records can hit 20+
	// list views). Floor at 12 so a tiny terminal still works; cap at
	// terminal-height-minus-chrome so the picker never overlaps the
	// status bar.
	maxRows := m.height - anchorY - 6
	if maxRows < 12 {
		maxRows = 12
	}

	return openPicker(m, pickerSpec[qchip.Chip]{
		Title:       "All views · " + scope,
		Items:       all,
		Width:       pickerW,
		MaxRows:     maxRows,
		AnchorX:     anchorX,
		AnchorY:     anchorY,
		Placeholder: "type to filter…",
		Match: func(c qchip.Chip, q string) bool {
			lq := strings.ToLower(q)
			return strings.Contains(strings.ToLower(c.Label), lq) ||
				strings.Contains(strings.ToLower(c.ID), lq)
		},
		RenderRow: func(c qchip.Chip, focused bool) string {
			label := c.Origin.Glyph() + c.Label
			line := "  " + label
			if focused {
				line = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " " +
					lipgloss.NewStyle().Foreground(theme.Fg).Bold(true).Render(label)
			}
			return line
		},
		OnPick: func(c qchip.Chip) tea.Cmd {
			return func() tea.Msg {
				return chipOverflowPickedMsg{domain: domain, scope: scope, chipID: c.ID}
			}
		},
	})
}

// chipOverflowPickedMsg lands on the main loop after the user picks a
// chip from the overflow modal.
type chipOverflowPickedMsg struct {
	domain chipDomain
	scope  string
	chipID string
}

// applyChipOverflowPicked sets the picked chip as the active strip
// selection AND populates the transient slot for that surface. The
// chip stays on the strip in a distinct style (transient, not
// favourite) until either:
//   - the user picks a different chip from M (replaces this one)
//   - the user pins it via 'f' (transient slot clears, chip moves
//     to the favourites group)
func (m Model) applyChipOverflowPicked(msg chipOverflowPickedMsg) (Model, tea.Cmd) {
	cmd := (&m).applyChipSelection(msg.domain, msg.scope, msg.chipID)
	return m, cmd
}

// applyChipSelection makes chipID the active view for (domain, scope):
// if the chip is already on the strip (a favourite) the cursor just
// moves to it; otherwise it lands in the transient slot, exactly like
// an M-overflow pick. Shared by the overflow modal and the V manager's
// Enter-to-apply.
//
// Records is the bespoke exception — its "active chip" lives on
// d.ListViewCur[sobj] (per-sObject) not on a numeric cursor, so the
// chipSurface SetChipIdx contract doesn't fit. Every other surface
// flows through chipSurfaceForDomain so the active cursor actually
// advances.
func (m *Model) applyChipSelection(domain chipDomain, scope, chipID string) tea.Cmd {
	if domain == domainRecords {
		_, sobj := m.activeRecordsSObject()
		if sobj != "" {
			d := m.data[m.orgs[m.selected].Username]
			if d != nil {
				d.ListViewCur[sobj] = chipID
			}
		}
		return m.onTabChanged()
	}

	// Favourite already on the strip? Just move the cursor onto it —
	// no transient slot needed (a transient copy would render the
	// chip twice).
	surf := chipSurfaceForDomain(domain)
	if surf != nil && surf.SetChipIdx != nil {
		for i, row := range m.stripRows(domain, "*") {
			if row.ID == chipID {
				surf.SetChipIdx(m, i)
				return m.onTabChanged()
			}
		}
	}

	// Not on the strip: occupy the transient slot, then move the
	// cursor onto the freshly-added row.
	if m.activeTransient == nil {
		m.activeTransient = map[string]string{}
	}
	key := transientSlotKey(string(domain), scope)
	m.activeTransient[key] = chipID
	if surf != nil && surf.SetChipIdx != nil {
		for i, row := range m.stripRows(domain, "*") {
			if row.ID == chipID {
				surf.SetChipIdx(m, i)
				break
			}
		}
	}
	return m.onTabChanged()
}
