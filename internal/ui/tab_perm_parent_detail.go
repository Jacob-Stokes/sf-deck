package ui

// /perms parent-detail — the drill-in when the user presses Enter on
// one permset, PSG, or profile. Shares one Tab (TabPermParentDetail)
// across all three kinds because the subtab axis is identical —
// Overview / Objects / Fields / System / Users (+ Components for
// PSGs). Which parent is in focus is stored on orgData:
// PermParentKind ∈ {"permset","psg","profile"} + PermParentID +
// (for profiles) PermParentPermSetID = the implicit permset Id used
// as ParentId when writing ObjectPermissions / FieldPermissions.

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// currentPermParent returns (kind, id, displayName, ok) for the
// drilled-in parent on TabPermParentDetail.
func (m Model) currentPermParent() (kind, id, displayName string, ok bool) {
	if len(m.orgs) == 0 {
		return "", "", "", false
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil || d.PermParentID == "" {
		return "", "", "", false
	}
	switch d.PermParentKind {
	case "permset":
		for _, p := range d.PermSets.Value() {
			if p.ID == d.PermParentID {
				lbl := p.Label
				if lbl == "" {
					lbl = p.Name
				}
				return "permset", p.ID, lbl, true
			}
		}
	case "psg":
		for _, g := range d.PSGs.Value() {
			if g.ID == d.PermParentID {
				lbl := g.MasterLabel
				if lbl == "" {
					lbl = g.DeveloperName
				}
				return "psg", g.ID, lbl, true
			}
		}
	case "profile":
		for _, p := range d.Profiles.Value() {
			if p.ID == d.PermParentID {
				return "profile", p.ID, p.Name, true
			}
		}
	}
	return d.PermParentKind, d.PermParentID, d.PermParentID, true
}

// renderPermParentDetail is the top-level dispatcher for the drill-in.
// Layout: breadcrumb + subtab strip at the top, then the subtab body.
func (m Model) renderPermParentDetail(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	if !canUseOrg(o) {
		return theme.Subtle.Render("  org disconnected")
	}

	kind, _, name, ok := m.currentPermParent()
	if !ok {
		return theme.Subtle.Render("  no permission parent selected — press enter on a row in /perms first")
	}

	subs := permParentDetailSubtabs(kind)
	sel := m.permParentSubtab()
	if sel < 0 || sel >= len(subs) {
		sel = 0
	}
	strip := renderSubtabStrip(subs, sel, w-4)
	body := innerH - subtabReserve(strip)

	// Fields are a drill-down from the Objects subtab, not a peer
	// subtab. When the user has drilled in (PermFieldsSObject set),
	// show the FLS grid for that object instead of the Objects list.
	d := m.ensureOrgDataRef(o.Username)
	isObjectsSubtab := subs[sel].ID == SubtabParentObjects
	drilledToFields := isObjectsSubtab && d.PermFieldsSObject != ""

	var content string
	switch {
	case drilledToFields:
		content = m.renderPermParentFields(w, inner, body, o)
	default:
		switch subs[sel].ID {
		case SubtabParentOverview:
			content = m.renderPermParentOverview(w, inner, body, o, kind, name)
		case SubtabParentObjects:
			content = m.renderPermParentObjects(w, inner, body, o)
		case SubtabParentSystem:
			content = m.renderPermParentSystem(w, inner, body, o)
		case SubtabParentUsers:
			content = m.renderPermParentUsers(w, inner, body, o)
		case SubtabParentComponents:
			content = m.renderPermParentComponentsStub(inner, name)
		default:
			content = theme.Subtle.Render("  (no subtab)")
		}
	}
	if strip == "" {
		return content
	}
	return strings.Join([]string{strip, content}, "\n")
}

// renderPermParentOverview — the identity + metadata section. Works
// for all three kinds; picks the right source struct via PermParentKind.
func (m Model) renderPermParentOverview(w, inner, innerH int, o sf.Org, kind, name string) string {
	d := m.ensureOrgDataRef(o.Username)

	var lines []string
	lines = append(lines, sectionTitle(name))

	switch kind {
	case "permset":
		var found *sf.PermissionSet
		for i, p := range d.PermSets.Value() {
			if p.ID == d.PermParentID {
				found = &d.PermSets.Value()[i]
				_ = p
				break
			}
		}
		if found == nil {
			lines = append(lines, theme.Subtle.Render("  permset not loaded"))
			return strings.Join(lines, "\n")
		}
		lines = append(lines, dimLine("  "+found.Name, inner))
		if found.NamespacePrefix != "" {
			lines = append(lines, kvLine("namespace", found.NamespacePrefix, inner))
		}
		lines = append(lines, kvLine("type", dashIfEmpty(found.Type), inner))
		lines = append(lines, kvLine("custom", boolLabel(found.IsCustom), inner))
		lines = append(lines, kvLine("license", dashIfEmpty(found.LicenseName), inner))
		if found.Description != "" {
			lines = append(lines, "")
			lines = append(lines, dimLine("  "+found.Description, inner))
		}
		if found.LastModifiedDate != "" {
			lines = append(lines, "")
			lines = append(lines, dimLine("  modified "+prettyDate(found.LastModifiedDate), inner))
		}
	case "psg":
		var found *sf.PermissionSetGroup
		for i := range d.PSGs.Value() {
			if d.PSGs.Value()[i].ID == d.PermParentID {
				g := d.PSGs.Value()[i]
				found = &g
				break
			}
		}
		if found == nil {
			lines = append(lines, theme.Subtle.Render("  psg not loaded"))
			return strings.Join(lines, "\n")
		}
		lines = append(lines, dimLine("  "+found.DeveloperName, inner))
		if found.NamespacePrefix != "" {
			lines = append(lines, kvLine("namespace", found.NamespacePrefix, inner))
		}
		lines = append(lines, kvLine("status", dashIfEmpty(found.Status), inner))
		if found.Description != "" {
			lines = append(lines, "")
			lines = append(lines, dimLine("  "+found.Description, inner))
		}
		if found.LastModifiedDate != "" {
			lines = append(lines, "")
			lines = append(lines, dimLine("  modified "+prettyDate(found.LastModifiedDate), inner))
		}
	case "profile":
		var found *sf.Profile
		for i := range d.Profiles.Value() {
			if d.Profiles.Value()[i].ID == d.PermParentID {
				p := d.Profiles.Value()[i]
				found = &p
				break
			}
		}
		if found == nil {
			lines = append(lines, theme.Subtle.Render("  profile not loaded"))
			return strings.Join(lines, "\n")
		}
		lines = append(lines, kvLine("user type", dashIfEmpty(found.UserType), inner))
		lines = append(lines, kvLine("license", dashIfEmpty(found.UserLicenseName), inner))
		lines = append(lines, kvLine("implicit permset", dashIfEmpty(found.PermissionSetID), inner))
		if found.Description != "" {
			lines = append(lines, "")
			lines = append(lines, dimLine("  "+found.Description, inner))
		}
		if found.LastModifiedDate != "" {
			lines = append(lines, "")
			lines = append(lines, dimLine("  modified "+prettyDate(found.LastModifiedDate), inner))
		}
	}
	_ = innerH
	_ = w
	return strings.Join(lines, "\n")
}

func (m Model) renderPermParentComponentsStub(inner int, name string) string {
	return stubPane(inner, name, "Components",
		"Permission sets that make up this PSG",
		"Coming next phase — drill-through to each component.")
}

func stubPane(inner int, name, subtab, desc, note string) string {
	var lines []string
	lines = append(lines, sectionTitle(name+" — "+subtab))
	lines = append(lines, "")
	lines = append(lines, dimLine("  "+desc, inner))
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(theme.FgDim).Render(
		ansi.Truncate("  "+note, inner, "…")))
	return strings.Join(lines, "\n")
}

// helpers
func boolLabel(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
