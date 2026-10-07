package ui

// Subtab dispatch helper.

import (
	"strings"
)

type subtabBranch struct {
	Render func(w, innerH int) string

	Placeholder *subtabPlaceholder
}

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
	return ""
}
