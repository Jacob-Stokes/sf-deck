package ui

// Subtab dispatch helper.
//
// Multiple tabs (Apex, Components, Reports, SOQL, Meta, Home) share
// the same shape: render the subtab strip, branch on the active
// subtab ID, fall through to a default. Each used to hand-roll
// this; the result was that adding a subtab meant remembering to
// (a) declare the subtab ID, (b) add a case in the renderer, and
// (c) prepend the strip. The Reports default-branch shipped
// without (c) and the strip vanished until you cycled away and
// back — exactly the bug class this helper kills.
//
// Use it like this:
//
//   func (m Model) renderFoo(w, innerH int) string {
//       return m.dispatchSubtab(w, innerH, fooSubtabs(), m.fooSubtab(),
//           map[Subtab]subtabBranch{
//               SubtabFooBlue:  {Render: m.renderFooBlue},
//               SubtabFooGreen: {Render: m.renderFooGreen},
//           },
//           subtabBranch{Render: m.renderFooDefault},
//       )
//   }
//
// Each subtabBranch carries either a Render closure (rendered with
// the subtab strip prepended automatically) or a Placeholder
// (renders the standard "coming soon" placeholder). The dispatcher
// clamps the cursor, draws the strip once, dispatches, joins.
//
// Tabs whose subtab list is dynamic (LWC bundle files, PermParent
// kind-dependent subtabs) don't fit and stay bespoke.

import (
	"strings"
)

// subtabBranch describes one subtab's render path. Exactly one of
// Render or Placeholder is set; Placeholder is the convenience for
// "coming soon"-style stubs that all look the same today.
type subtabBranch struct {
	// Render is called with the inner pane width and the budget
	// REMAINING after the subtab strip is drawn. The closure should
	// not draw the strip itself — dispatchSubtab handles that.
	Render func(w, innerH int) string

	// Placeholder, when set, takes precedence over Render. Wraps
	// the standard joinPlaceholder helper so coming-soon stubs are
	// one struct literal in the dispatch map rather than three
	// repeated lines per subtab.
	Placeholder *subtabPlaceholder
}

// subtabPlaceholder describes a "coming soon" stub. Header is the
// section title (uppercase shown in the placeholder); Description
// is the muted body text; SetupURL is the Lightning URL the
// placeholder's "open in SF" affordance points at. Empty fields
// fall through to sane defaults.
type subtabPlaceholder struct {
	Header      string
	Description string
	SetupURL    string
}

// dispatchSubtab is the standard subtab renderer:
//
//  1. Clamp `selected` to a valid index.
//  2. Render the subtab strip (one line, hidden when ≤1 subtab).
//  3. Look up the branch for the active subtab; fall through to
//     `defaultBranch` if no branch matches.
//  4. Call the branch's Render (or render its Placeholder), with
//     the budget reduced by however many lines the strip ate.
//  5. Join strip + body with newlines.
//
// The contract is the same as renderSubtabStrip + a hand-rolled
// switch — just centralised so the strip can't be silently dropped
// by a forgotten `lines = append(lines, strip)` in some default
// branch.
func (m Model) dispatchSubtab(
	w, innerH int,
	subs []subtabInfo,
	selected int,
	branches map[Subtab]subtabBranch,
	defaultBranch subtabBranch,
) string {
	inner := w - 4
	if selected < 0 || selected >= len(subs) {
		selected = 0
	}
	stripLines := []string{}
	if strip := renderSubtabStrip(subs, selected, inner); strip != "" {
		stripLines = append(stripLines, strip)
	}
	// Count ACTUAL terminal lines in the strip — pills with rounded
	// borders render multi-line but live as one slice element. Using
	// len(stripLines) undercounted by ~2, so the body computed its
	// budget against a too-large innerH and ran past the pane bottom
	// (visible as the hint disappearing on subtab surfaces).
	budget := innerH - usedLines(stripLines)
	if budget < 5 {
		budget = 5
	}

	branch := defaultBranch
	if len(subs) > 0 {
		if b, ok := branches[subs[selected].ID]; ok {
			branch = b
		}
	}

	body := renderSubtabBranch(branch, w, budget, inner)
	if len(stripLines) == 0 {
		return body
	}
	return strings.Join(append(stripLines, body), "\n")
}

// renderSubtabBranch routes through Placeholder or Render.
// Placeholder takes precedence so a branch can declare both
// (useful for migration: drop the Render closure to disable a
// half-built subtab without losing the explicit placeholder).
func renderSubtabBranch(b subtabBranch, w, budget, inner int) string {
	if b.Placeholder != nil {
		return joinPlaceholder("", b.Placeholder.Header, b.Placeholder.Description, b.Placeholder.SetupURL, inner)
	}
	if b.Render != nil {
		return b.Render(w, budget)
	}
	// No branch declared and no default — render an empty body
	// rather than crashing. Should be unreachable in practice.
	return ""
}
