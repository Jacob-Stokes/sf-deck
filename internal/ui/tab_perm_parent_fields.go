package ui

import (
	"fmt"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func (m Model) renderPermParentFields(w, inner, innerH int, o sf.Org) string {
	d := m.ensureOrgDataRef(o.Username)

	if d.PermParentPermSetID == "" {
		return psgNoDirectPermsNote(inner, "Field")
	}

	if d.PermFieldsSObject == "" {
		return theme.Subtle.Render("  press enter on an object in the Objects subtab first")
	}

	return m.renderFLSGrid(w, inner, innerH, o, d.PermFieldsSObject, d.PermParentPermSetID, false)
}

func (m Model) renderFLSGrid(w, inner, innerH int, o sf.Org, sobj, parentID string, showScopePicker bool) string {
	d := m.ensureOrgDataRef(o.Username)

	if showScopePicker {
		if d.PermissionSets.Busy() || d.PermissionSets.FetchedAt().IsZero() {
			if err := d.PermissionSets.Err(); err != nil {
				return redLine("  " + err.Error())
			}
			return theme.Subtle.Render("  loading permission sets…")
		}
	}

	flsRes := d.EnsureFLS(targetArg(o), sobj, parentID)
	if flsRes.FetchedAt().IsZero() {
		var lines []string
		title := fmt.Sprintf("FIELD PERMISSIONS · %s", sobj)
		lines = append(lines, sectionTitle(title))
		if showScopePicker {
			permsets := d.PermissionSets.Value()
			strip := renderFLSScopeStrip(permsets, parentID, inner)
			lines = append(lines, strip, "")
		}
		if flsRes.Busy() {
			lines = append(lines, theme.Subtle.Render("  loading field permissions…"))
		} else if err := flsRes.Err(); err != nil {
			lines = append(lines, redLine("  "+err.Error()))
		} else {
			lines = append(lines, theme.Subtle.Render("  fetching…"))
		}
		return strings.Join(lines, "\n")
	}

	byField := map[string]sf.FieldPermissionRow{}
	for _, fp := range flsRes.Value() {
		name := fp.Field
		if i := strings.IndexByte(name, '.'); i >= 0 {
			name = name[i+1:]
		}
		byField[name] = fp
	}

	dr, ok := d.Describes[sobj]
	if !ok || dr.FetchedAt().IsZero() {
		return theme.Subtle.Render("  loading describe…")
	}
	fields := dr.Value().Fields

	totalExplicit := len(flsRes.Value())
	var lines []string
	title := fmt.Sprintf("FIELD PERMISSIONS · %s · %d fields · %d explicit · %s",
		sobj, len(fields), totalExplicit,
		humanAge(flsRes.FetchedAt())+stateSuffix(flsRes.Busy(), flsRes.Err()))
	lines = append(lines, sectionTitle(title))
	if showScopePicker {
		permsets := d.PermissionSets.Value()
		strip := renderFLSScopeStrip(permsets, parentID, inner)
		lines = append(lines, strip, "")
	} else {
		lines = append(lines, "")
	}

	nameW := inner / 3
	if nameW < 22 {
		nameW = 22
	}
	// Fixed row overhead: prefix(2) + two column gaps(4) + "[R]"(3) +
	// space(1) + "[E]"(3) = 13. The previous budget subtracted 10, so
	// EVERY row overflowed `inner` by 3 columns and ansi.Truncate ate
	// the trailing [E] cell — the Edit indicator was invisible at all
	// widths (field report 2026-06-13). One spare column on top for
	// wide-glyph safety.
	labelW := inner - nameW - 14
	if labelW < 12 {
		labelW = 12
	}

	sel := d.Cursors.Get(cursorKindFLS, len(fields), sobj, parentID)
	lines = append(lines, renderRows(
		len(fields), sel, innerH, len(lines), 0, inner,
		func(i int) string {
			return renderFLSRow(fields[i], byField, i == sel, m.focus == focusMain,
				nameW, labelW, inner)
		},
	)...)

	return strings.Join(lines, "\n")
}
