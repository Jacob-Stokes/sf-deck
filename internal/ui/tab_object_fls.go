package ui

// Field-Level Security (FLS) subtab of TabObjectDetail.

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

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
	parent := d.FLSParentID
	if parent == "" {
		parent = permsets[0].ID
	}

	return m.renderFLSGrid(w, inner, innerH, o, sobj, parent, true)
}

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

func flsCell(letter string, on bool) string {
	if on {
		return lipgloss.NewStyle().Foreground(theme.Green).Bold(true).Render("[" + letter + "]")
	}
	return lipgloss.NewStyle().Foreground(theme.FgDim).Render("[·]")
}

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

func renderFLSScopeStrip(perms []sf.FLSPickerEntry, selectedID string, width int) string {
	selStyle := lipgloss.NewStyle().Foreground(theme.Cyan).Bold(true).Underline(true)
	normStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	sep := lipgloss.NewStyle().Foreground(theme.Border).Render(" · ")

	hint := lipgloss.NewStyle().Foreground(theme.FgDim).Render("  ← / → cycle scope")
	ell := lipgloss.NewStyle().Foreground(theme.Border).Render("…")

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

	budget := width - lipgloss.Width(hint)
	if budget < 12 {
		budget = width // too narrow for the hint — drop it
		hint = ""
	}

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
