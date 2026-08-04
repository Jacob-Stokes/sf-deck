package ui

// Field-Level Security (FLS) subtab of TabObjectDetail.
//
// One 2D grid: rows = fields on the current sobject, columns = Read
// + Edit for the selected scope (a Profile or PermissionSet). A
// chip strip at the top picks the scope; ← / → cycles it. r toggles
// Read on the cursored field; e toggles Edit. Edit=true implies
// Read=true (Salesforce invariant — enforced in the sf layer,
// reflected live in the UI on save).
//
// NOTE: r is "toggle Read" on this grid, which shadows the global
// r=refresh. Use ctrl+r to refresh the FLS data here.
//
// FieldPermissions rows are sparse: when a field has never had its
// FLS explicitly set, there's no row. We render those as R=off /
// E=off because that's the effective permission anyway.
//
// Writes are POST+PATCH through FieldPermissions. Both operations
// are fast (single sobject write, no deploy cycle) so the grid
// reacts instantly on toggle — no "saving…" modal needed for a
// single cell, we just flash on error.

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// renderObjectFLS is the main-pane renderer for the FLS subtab.
func (m Model) renderObjectFLS(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	d := m.ensureOrgDataRef(o.Username)
	sobj := d.DescribeCur
	if sobj == "" {
		return theme.Subtle.Render("  press enter on an object in /objects first")
	}

	if d.PermissionSets.Busy() || d.PermissionSets.FetchedAt().IsZero() {
		if err := d.PermissionSets.Err(); err != nil {
			return redLine("  " + err.Error())
		}
		return theme.Subtle.Render("  loading permission sets…")
	}
	permsets := d.PermissionSets.Value()
	if len(permsets) == 0 {
		return theme.Subtle.Render("  no permission sets / profiles visible to this user")
	}
	// Default-parent selection happens in applyResourceMsg's "permsets"
	// branch — the moment the org-wide list lands, we pin the first
	// permset and fire the FLS ensure. Render here is read-only; if
	// FLSParentID is still empty we just show a hint while the data
	// loads (rare; only briefly visible after a permsets cache miss).
	parent := d.FLSParentID
	if parent == "" {
		parent = permsets[0].ID
	}

	return m.renderFLSGrid(w, inner, innerH, o, sobj, parent, true)
}

// renderFLSRow is one field's row in the grid.
func renderFLSRow(f sf.Field, byField map[string]sf.FieldPermissionRow, selected, mainFocused bool, nameW, labelW, inner int) string {
	fp, hasRow := byField[f.Name]
	read := false
	edit := false
	if hasRow {
		read = fp.Read
		edit = fp.Edit
	}
	permissionable := f.Permissionable

	nameStyle := lipgloss.NewStyle().Foreground(theme.Fg).Width(nameW)
	labelStyle := lipgloss.NewStyle().Foreground(theme.Muted).Width(labelW)
	if selected {
		nameStyle = nameStyle.Bold(true)
	}

	name := nameStyle.Render(ansi.Truncate(f.Name, nameW-1, "…"))
	label := labelStyle.Render(ansi.Truncate(dashIfEmpty(f.Label), labelW-1, "…"))
	r := flsCell("R", read)
	e := flsCell("E", edit)
	if !permissionable {
		// FLS doesn't apply (Id, audit stamps, compound fields) —
		// a [·] here reads as "denied", which is wrong: these are
		// always visible. Dim dashes, same cell width.
		dash := lipgloss.NewStyle().Foreground(theme.FgDim).Render(" — ")
		r, e = dash, dash
	}

	prefix := "  "
	if selected {
		barColor := theme.BorderHi
		if !mainFocused {
			barColor = theme.Muted
		}
		prefix = lipgloss.NewStyle().Foreground(barColor).Render("▌") + " "
	}
	return ansi.Truncate(prefix+name+"  "+label+"  "+r+" "+e, inner, "…")
}

// flsCell renders one on/off indicator.
func flsCell(letter string, on bool) string {
	if on {
		return lipgloss.NewStyle().Foreground(theme.Green).Bold(true).Render("[" + letter + "]")
	}
	return lipgloss.NewStyle().Foreground(theme.FgDim).Render("[·]")
}

// sidebarFLS renders the FLS subtab's right-side sidebar: current
// scope label, cursored field's metadata, and the keyboard hints
// for toggling Read/Edit. Complements the grid in the main pane.
func (m Model) sidebarFLS(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil || d.DescribeCur == "" {
		return sideEmpty("—")
	}
	dr, ok := d.Describes[d.DescribeCur]
	if !ok || dr.FetchedAt().IsZero() {
		return sideEmpty("loading…")
	}
	fields := dr.Value().Fields
	if len(fields) == 0 {
		return sideEmpty("no fields")
	}
	idx := d.Cursors.Get(cursorKindFLS, len(fields), d.DescribeCur, d.FLSParentID)
	f := fields[idx]

	// Resolve scope label.
	scopeLabel := "—"
	for _, p := range d.PermissionSets.Value() {
		if p.ID == d.FLSParentID {
			scopeLabel = p.Label
			if p.IsPermSet {
				scopeLabel = "⌘ " + scopeLabel
			}
			break
		}
	}

	// Current row R/E state.
	read, edit := false, false
	flsKey := d.DescribeCur + ":" + d.FLSParentID
	if flsRes, ok := d.FLS[flsKey]; ok && flsRes != nil {
		for _, fp := range flsRes.Value() {
			name := fp.Field
			if i := strings.IndexByte(name, '.'); i >= 0 {
				name = name[i+1:]
			}
			if name == f.Name {
				read = fp.Read
				edit = fp.Edit
				break
			}
		}
	}

	rows := []kv{
		{"scope", scopeLabel},
		{"field", f.Name},
		{"label", dashIfEmpty(f.Label)},
		{"type", sidebarFieldTypeDisplay(f)},
		{"custom", yesNo(f.Custom)},
		{"read", yesNo(read)},
		{"edit", yesNo(edit)},
	}
	extra := []string{
		"", sideSection("keys"),
		sideDim("  ← / → cycle scope", inner),
		sideDim("  "+firstPretty(Keys.FLSToggleRead)+" toggle Read", inner),
		sideDim("  "+firstPretty(Keys.FLSToggleEdit)+" toggle Edit", inner),
		sideDim("  "+firstPretty(Keys.GlobalRefresh)+" refresh ("+firstPretty(Keys.FLSToggleRead)+" is taken)", inner),
		"", sideDim("  Edit on implies Read on.", inner),
		sideDim("  Both off deletes the row.", inner),
	}
	return renderKVPanel(inner, f.Name, rows, extra...)
}

// renderFLSScopeStrip draws the chip strip of profiles + permsets.
func renderFLSScopeStrip(perms []sf.FLSPickerEntry, selectedID string, width int) string {
	selStyle := lipgloss.NewStyle().Foreground(theme.Cyan).Bold(true).Underline(true)
	normStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	sep := lipgloss.NewStyle().Foreground(theme.Border).Render(" · ")

	hint := lipgloss.NewStyle().Foreground(theme.FgDim).Render("  ← / → cycle scope")
	ell := lipgloss.NewStyle().Foreground(theme.Border).Render("…")

	// Render each scope label + find the selected index.
	labels := make([]string, len(perms))
	selectedIdx := 0
	for i, p := range perms {
		label := p.Label
		if p.IsPermSet {
			label = "⌘ " + label
		}
		if p.ID == selectedID {
			selectedIdx = i
			labels[i] = selStyle.Render(label)
		} else {
			labels[i] = normStyle.Render(label)
		}
	}

	// Budget for the scope pills (leave room for the hint).
	budget := width - lipgloss.Width(hint)
	if budget < 12 {
		budget = width // too narrow for the hint — drop it
		hint = ""
	}

	// Window the pills around the SELECTED index so the current scope is
	// always visible, with a "…" marker on whichever side has hidden
	// scopes. The old code joined ALL scopes then ansi.Truncate'd to
	// width, so a selection past the right edge (common with many
	// permsets) scrolled off entirely — you couldn't tell which scope you
	// were editing. Grow outward from the selection until the budget runs
	// out, alternating right then left so context appears on both sides.
	lo, hi := selectedIdx, selectedIdx // [lo, hi] inclusive kept window
	used := lipgloss.Width(labels[selectedIdx])
	for {
		grew := false
		if hi+1 < len(perms) {
			w := lipgloss.Width(sep) + lipgloss.Width(labels[hi+1])
			if used+w <= budget {
				used += w
				hi++
				grew = true
			}
		}
		if lo-1 >= 0 {
			w := lipgloss.Width(sep) + lipgloss.Width(labels[lo-1])
			if used+w <= budget {
				used += w
				lo--
				grew = true
			}
		}
		if !grew {
			break
		}
	}

	parts := make([]string, 0, hi-lo+1)
	if lo > 0 {
		parts = append(parts, ell)
	}
	parts = append(parts, labels[lo:hi+1]...)
	if hi < len(perms)-1 {
		parts = append(parts, ell)
	}
	return ansi.Truncate(strings.Join(parts, sep), budget, "…") + hint
}
