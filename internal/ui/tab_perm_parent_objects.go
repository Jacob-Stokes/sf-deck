package ui

// Object-permissions subtab of TabPermParentDetail.
//
// 7-column grid: OBJECT · R · C · E · D · VA · MA.
// Read-only for PSGs (they have no direct permset; show a hint).
// Toggling is handled in perm_actions.go (Phase G).

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// renderPermParentObjects replaces the stub from views_perm_parent_detail.go.
func (m Model) renderPermParentObjects(w, inner, innerH int, o sf.Org) string {
	d := m.ensureOrgDataRef(o.Username)

	// PSGs have no direct permset — union of components. Show a hint.
	if d.PermParentPermSetID == "" {
		return psgNoDirectPermsNote(inner, "Object")
	}

	key := d.PermParentKind + ":" + d.PermParentPermSetID
	res := d.EnsureObjectPerms(targetArg(o), d.PermParentKind, d.PermParentPermSetID)

	if res.FetchedAt().IsZero() {
		var lines []string
		lines = append(lines, sectionTitle("OBJECT PERMISSIONS"))
		if res.Busy() {
			lines = append(lines, theme.Subtle.Render("  loading object permissions…"))
		} else if err := res.Err(); err != nil {
			lines = append(lines, redLine("  "+err.Error()))
		} else {
			lines = append(lines, theme.Subtle.Render("  fetching…"))
		}
		return strings.Join(lines, "\n")
	}

	allRows := res.Value()
	// Filter by search buffer (substring match against SobjectType).
	search := d.ObjPermSearch[key]
	q := ""
	if search != nil {
		q = strings.ToLower(search.Buffer())
	}
	var rows []sf.ObjectPermission
	if q == "" {
		rows = allRows
	} else {
		for _, r := range allRows {
			if strings.Contains(strings.ToLower(r.SObjectType), q) {
				rows = append(rows, r)
			}
		}
	}
	cur := d.Cursors.Get(cursorKindObjectPerms, len(rows), d.PermParentKind, d.PermParentPermSetID)

	age := humanAge(res.FetchedAt()) + stateSuffix(res.Busy(), res.Err())
	var headerText string
	if q == "" {
		headerText = fmt.Sprintf("OBJECT PERMISSIONS · %d · %s", len(allRows), age)
	} else {
		headerText = fmt.Sprintf("OBJECT PERMISSIONS · %d / %d · %s", len(rows), len(allRows), age)
	}

	var lines []string
	lines = append(lines, headerWithSearchPill(headerText, derefSearch(search)))
	lines = append(lines, searchBar(derefSearch(search), inner))
	if len(allRows) == 0 {
		lines = append(lines, theme.Subtle.Render("  no object permissions set"))
		return strings.Join(lines, "\n")
	}
	if len(rows) == 0 {
		lines = append(lines, theme.Subtle.Render("  no matches"))
		return strings.Join(lines, "\n")
	}

	// Column widths.
	nameW := inner * 40 / 100
	if nameW < 20 {
		nameW = 20
	}
	// 6 perm columns × 4 chars each + separators.
	permColW := 3

	lines = append(lines, renderObjPermHeader(nameW, permColW, inner))
	lines = append(lines, renderRows(
		len(rows), cur, innerH, len(lines), 0, inner,
		func(i int) string {
			return renderObjPermRow(rows[i], i == cur, m.focus == focusMain, nameW, permColW, inner)
		},
	)...)
	return strings.Join(lines, "\n")
}

// derefSearch returns a zero searchState if s is nil, so the
// headerWithSearchPill / searchBar helpers (which take a value,
// not a pointer) don't blow up before the user ever activates search.
func derefSearch(s *searchState) searchState {
	if s == nil {
		return searchState{}
	}
	return *s
}

func renderObjPermHeader(nameW, permColW, inner int) string {
	nameStyle := lipgloss.NewStyle().Foreground(theme.Muted).Width(nameW)
	colStyle := lipgloss.NewStyle().Foreground(theme.Muted).Width(permColW + 1)
	sep := lipgloss.NewStyle().Foreground(theme.Border).Render(" │ ")
	line := "  " +
		nameStyle.Render("OBJECT") + sep +
		colStyle.Render("R") + " " +
		colStyle.Render("C") + " " +
		colStyle.Render("E") + " " +
		colStyle.Render("D") + " " +
		colStyle.Render("VA") + " " +
		colStyle.Render("MA")
	return ansi.Truncate(line, inner, "…")
}

func renderObjPermRow(row sf.ObjectPermission, selected, mainFocused bool, nameW, permColW, inner int) string {
	fg := theme.Fg
	nameStyle := lipgloss.NewStyle().Foreground(fg).Width(nameW)
	if selected {
		nameStyle = nameStyle.Bold(true)
	}

	prefix := "  "
	if selected {
		barColor := theme.BorderHi
		if !mainFocused {
			barColor = theme.Muted
		}
		prefix = lipgloss.NewStyle().Foreground(barColor).Render("▌") + " "
	}

	sep := lipgloss.NewStyle().Foreground(theme.Border).Render(" │ ")
	name := nameStyle.Render(ansi.Truncate(row.SObjectType, nameW-1, "…"))
	return ansi.Truncate(
		prefix+name+sep+
			objPermCell(row.Read)+" "+
			objPermCell(row.Create)+" "+
			objPermCell(row.Edit)+" "+
			objPermCell(row.Delete)+" "+
			objPermCell(row.ViewAllRecords)+" "+
			objPermCell(row.ModifyAllRecords),
		inner, "…")
}

// objPermCell renders one boolean cell as ● (on) or ○ (off).
func objPermCell(on bool) string {
	if on {
		return lipgloss.NewStyle().Foreground(theme.Green).Bold(true).Render("●")
	}
	return lipgloss.NewStyle().Foreground(theme.FgDim).Render("○")
}

// psgNoDirectPermsNote returns the standard "PSG: no direct perms" message.
func psgNoDirectPermsNote(inner int, permKind string) string {
	var lines []string
	lines = append(lines, sectionTitle(strings.ToUpper(permKind)+" PERMISSIONS"))
	lines = append(lines, "")
	msg := fmt.Sprintf(
		"  %s permissions for a Permission Set Group are the union of its components.",
		permKind)
	lines = append(lines, dimLine(msg, inner))
	lines = append(lines, "")
	lines = append(lines, dimLine("  Drill into the Components subtab to view or edit individual permsets.", inner))
	return strings.Join(lines, "\n")
}
