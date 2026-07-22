package ui

// Shared chip-manager modal infrastructure. One opener serves
// /records, /objects, and /flows; this file owns the menu shape so
// every surface gets consistent UX. Per-surface specifics
// (registry lookup, scope rules, import flow) live in chip_manager.go.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

// chipManagerSpec describes one invocation of the manager menu —
// what title to show, which chips are listed, which kind of surface
// (records-lens, sobject-filter, flow-filter) to dispatch to on
// Save. Doesn't carry closures: dispatch happens by kind on the
// live Model in Update so callbacks resolve against fresh state.
type chipManagerSpec struct {
	Kind        string            // "lens" / "objects" / "flows" — domain key (legacy name)
	Title       string            // e.g. "Views · Account"
	Scope       string            // sObject name or "*"
	Chips       []chipMenuRow     // one row per visible (active-org) view
	OtherOrgs   []otherOrgChipRow // chips owned by OTHER orgs that match this (domain, scope) — listed in a separate bottom section so users can preview them or widen scope
	Ephemerals  []chipMenuRow     // IPC-spawned session-only chips at this (domain, scope) — listed in their own section so users can promote (Save) or Dismiss them via the action sub-modal
	NewLabel    string            // "+ New view…"
	ImportLabel string            // empty → no import row
}

// chipMenuRow is one chip's entry in the manager menu.
type chipMenuRow struct {
	ID              string
	Label           string
	Hint            string
	Origin          qchip.Origin
	Share           settings.ChipShare // drives the cross-org "⇄" marker on the row
	Favourite       bool
	LockedFavourite bool // built-ins like Recent / All can't be unpinned
}

// openChipManagerMenu opens the top-level chip manager modal driven
// by the given spec. Returns the tea.Cmd to fire (the choiceModal).
//
// One row per chip. Enter on a chip opens an action sub-modal
// (Pin/Unpin · Edit · Delete · Cancel) so the main list stays
// scannable. Sectioned: top-of-list actions (+ New, ↓ Import),
// then Pinned, then Available (user-defined that aren't pinned),
// then Built-ins.
func (m *Model) openChipManagerMenu(spec chipManagerSpec) tea.Cmd {
	opts := []choiceOption{
		{Label: "+ " + spec.NewLabel, Hint: "create a new entry", Value: "new"},
	}
	if spec.ImportLabel != "" {
		opts = append(opts, choiceOption{
			Label: "↓ " + spec.ImportLabel,
			Hint:  "copy from an external source",
			Value: "import",
		})
	}

	// Three buckets so the modal reads top-down:
	//   1. Pinned     — chips on the strip; user's daily drivers
	//   2. Available  — user-defined, not pinned (overflow); enter to pin
	//   3. Built-ins  — read-only; pin/unpin only when LockedFavourite=false
	var pinned, available, builtins []chipMenuRow
	for _, r := range spec.Chips {
		switch {
		case r.Origin == qchip.OriginBuiltIn:
			builtins = append(builtins, r)
		case r.Favourite:
			pinned = append(pinned, r)
		default:
			available = append(available, r)
		}
	}

	addSection := func(title string, rows []chipMenuRow) {
		if len(rows) == 0 {
			return
		}
		opts = append(opts, choiceOption{
			Label:   "── " + title + " ──",
			Value:   "_sep_" + title,
			Heading: true,
		})
		for _, r := range rows {
			opts = append(opts, choiceOption{
				Label: chipManagerRowLabel(r),
				Hint:  r.Hint,
				Value: "apply:" + r.ID,
			})
		}
	}
	addSection("pinned", pinned)
	addSection("available", available)
	addSection("built-ins", builtins)

	// Ephemerals section: IPC-spawned session-only chips. Listed
	// after the persisted chips so the user can see what's been
	// spun up by an external controller this session, then either
	// Save (promote to persisted via the action sub-modal) or
	// Dismiss (drop the entry from m.chipPreviews). Hidden when
	// empty — most sessions never see this.
	if len(spec.Ephemerals) > 0 {
		opts = append(opts, choiceOption{
			Label:   "── session-only ──",
			Value:   "_sep_ephemerals",
			Heading: true,
		})
		for _, r := range spec.Ephemerals {
			opts = append(opts, choiceOption{
				// Tilde glyph matches the strip render so the user
				// recognises the same entry at a glance.
				Label: chipEphemeralGlyph + " " + r.Label,
				Hint:  r.Hint,
				// "eph:" value-prefix routes the e (actions) sub-modal
				// to the Save/Dismiss action set instead of the
				// usual Rename/Delete options.
				Value: "eph:" + r.ID,
			})
		}
	}

	// Other-orgs section: chips that exist in your settings but belong
	// to a different org. Enter on one opens a sub-modal with
	// Preview/Add-to-scope actions; no destructive defaults — the
	// user has to deliberately pick "Preview here" or widen scope.
	if len(spec.OtherOrgs) > 0 {
		opts = append(opts, choiceOption{
			Label:   "── chips from your other orgs ──",
			Value:   "_sep_other_orgs",
			Heading: true,
		})
		for _, r := range spec.OtherOrgs {
			// Enter previews the chip here (session-only) and makes it
			// the active view — the non-destructive equivalent of
			// "apply" for a chip another org owns. e opens the
			// Preview/Add-to-scope actions sub-modal.
			opts = append(opts, choiceOption{
				Label: "  ☐ " + r.Label,
				Hint:  r.Hint,
				Value: "otherpreview:" + r.ID,
			})
		}
	}

	kind := spec.Kind
	scope := spec.Scope
	state := choiceModalState{
		Title:      spec.Title,
		Hint:       "Enter to apply  ·  e for actions  ·  / to filter  ·  Esc to cancel",
		Options:    opts,
		Cursor:     0,
		Searchable: true,
		Wide:       true,
		// Both kind and the picked id thread through the typed
		// channel: kind via the closure capture (it's static for
		// this modal instance), pick via val. Enter applies the
		// chip as the active view; e opens the per-chip actions
		// sub-modal (edit / pin / delete / scope).
		OnSuccessTyped: func(val any) tea.Cmd {
			pick, _ := val.(string)
			return func() tea.Msg {
				return chipManagerInvokeMsg{kind: kind, scope: scope, pick: pick}
			}
		},
		AltKeys: "e",
		OnAltTyped: func(_ string, val any) tea.Cmd {
			pick, _ := val.(string)
			switch {
			case strings.HasPrefix(pick, "apply:"):
				pick = "actions:" + strings.TrimPrefix(pick, "apply:")
			case strings.HasPrefix(pick, "otherpreview:"):
				pick = "otherorg:" + strings.TrimPrefix(pick, "otherpreview:")
			case strings.HasPrefix(pick, "eph:"):
				pick = "ephactions:" + strings.TrimPrefix(pick, "eph:")
			default:
				return nil // e only means "actions" on chip rows
			}
			return func() tea.Msg {
				return chipManagerInvokeMsg{kind: kind, scope: scope, pick: pick}
			}
		},
	}
	return m.openChoiceModal(state)
}

// chipManagerRowLabel renders one chip as a single line for the
// manager modal. Glyphs encode state inline:
//
//	★ / ☆   pinned to strip / available
//	·       user-authored (origin glyph)
//	↓       imported from a Salesforce list view
//	⇄       shared with other orgs (global / group / multi-org list)
//	(blt)   built-in chip (suffix; read-only body)
func chipManagerRowLabel(r chipMenuRow) string {
	parts := []string{"  ", favStar(r.Favourite), " "}
	if g := r.Origin.Glyph(); g != "" {
		parts = append(parts, g)
	}
	if r.Share.IsShared() {
		parts = append(parts, qchip.SharedGlyph)
	}
	parts = append(parts, r.Label)
	if r.Origin == qchip.OriginBuiltIn {
		parts = append(parts, " (built-in)")
	}
	return strings.Join(parts, "")
}

// openChipActionsModal is the per-chip action picker shown when the
// user hits Enter on a chip in the manager. Built-ins offer a
// reduced set (no edit, no delete).
func (m *Model) openChipActionsModal(d chipDomain, id string) tea.Cmd {
	reg := m.registryFor(d)
	if reg == nil {
		return nil
	}
	c, ok := reg.FindByID(id)
	if !ok {
		return nil
	}
	domain := d
	chipID := id

	pinLabel := "Pin to strip"
	pinHint := "show on the strip without opening this menu each time"
	if c.Favourite {
		pinLabel = "Unpin from strip"
		pinHint = "hide from the strip; still reachable via the More… modal"
	}
	var opts []choiceOption
	if !c.LockedFavourite {
		opts = append(opts, choiceOption{
			Label: pinLabel, Hint: pinHint, Value: "fav",
		})
	}
	if c.Origin != qchip.OriginBuiltIn {
		opts = append(opts,
			choiceOption{Label: "Edit", Hint: "open in chip wizard", Value: "edit"},
			choiceOption{Label: "Delete", Hint: "remove this chip", Value: "delete"},
		)
	}
	opts = append(opts, choiceOption{Label: "Cancel", Cancel: true})

	state := choiceModalState{
		Title:   c.Label,
		Hint:    "Enter to apply  ·  Esc to cancel",
		Options: opts,
		Cursor:  0,
		OnSuccessTyped: func(val any) tea.Cmd {
			action, _ := val.(string)
			pick := action + ":" + chipID
			if action == "fav" {
				pick = "fav:" + chipID
			}
			return func() tea.Msg {
				return chipManagerInvokeMsg{kind: string(domain), pick: pick}
			}
		},
	}
	return m.openChoiceModal(state)
}

// chipManagerInvokeMsg arrives on the main loop after the menu is
// dismissed; Update calls applyChipManagerInvoke to route to the
// right per-surface handler.
type chipManagerInvokeMsg struct {
	kind  string // chip domain, e.g. "apex" / "objects" / "flows"
	scope string // chip applicability ("*" or an sObject API name)
	pick  string // "new" / "import" / "apply:<id>" / "actions:<id>" / ...
}

// applyChipManagerInvoke is the Update-side handler. All three
// surfaces route through the unified dispatchChipManagerAction now —
// per-domain lookup happens inside.
func (m *Model) applyChipManagerInvoke(msg chipManagerInvokeMsg) tea.Cmd {
	d := chipDomain(msg.kind)
	return m.dispatchChipManagerAction(d, msg.scope, msg.pick)
}

// favStar renders the favourite-on-strip flag for chip manager rows.
// Star = on the strip; outline = lives only in the overflow modal.
// Toggling is via the F keybind on the active chip, not via this
// modal — keeps the manager focused on edit / delete / import.
func favStar(favourite bool) string {
	if favourite {
		return "★"
	}
	return "☆"
}

// helper to silence "imported var used" linter on tea import in
// trivial tests; keeps the import unconditional.
var _ tea.Msg = chipManagerInvokeMsg{}
var _ = fmt.Sprint
