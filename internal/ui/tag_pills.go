package ui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func tagColorFor(name string) color.Color {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "blue":
		return theme.Blue
	case "cyan":
		return theme.Cyan
	case "green":
		return theme.Green
	case "yellow":
		return theme.Yellow
	case "red":
		return theme.Red
	case "magenta", "purple", "pink":
		return theme.Magenta
	case "orange":
		return theme.Orange
	}
	return theme.Border
}

var tagPalette = []string{"blue", "cyan", "green", "yellow", "red", "purple", "orange"}

func nextRotatingTagColor(n int) string {
	if len(tagPalette) == 0 {
		return "blue"
	}
	if n < 0 {
		n = 0
	}
	return tagPalette[n%len(tagPalette)]
}

// renderTagPill produces a single tag pill — block-coloured
// background, white-on-color text, with the optional icon prefix.
// Used both inline (sidebar) and standalone (chip strip).
//
// The pill is one terminal cell taller than text-only because of the
// background bar, so callers join with " " separator and don't pad.
func renderTagPill(t devproject.Tag) string {
	bg := tagColorFor(t.Color)
	style := lipgloss.NewStyle().
		Background(bg).
		Foreground(theme.Bg).
		Bold(true).
		Padding(0, 1)
	label := t.Name
	if t.Icon != "" {
		label = t.Icon + " " + label
	}
	return style.Render(label)
}

func renderTagPills(tags []devproject.Tag) string {
	if len(tags) == 0 {
		return ""
	}
	pills := make([]string, 0, len(tags))
	for _, t := range tags {
		pills = append(pills, renderTagPill(t))
	}
	return strings.Join(pills, " ")
}

func (m Model) sidebarTagSection(kind devproject.ItemKind, ref, orgUser string, inner int) string {
	if m.devProjects == nil || ref == "" {
		return ""
	}
	tags, err := m.devProjects.TagsFor(kind, ref, orgUser)
	if err != nil || len(tags) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(sideSection("tags"))
	b.WriteString("\n  ")
	b.WriteString(renderTagPills(tags))
	return b.String()
}

// TagGutterWidth is the screen width reserved for the synthetic tag
// gutter in "compact" mode (coloured dots). Wide enough for the
// header label "TAGS" (4 cells) and the maximum body content
// "●●●+" (4 cells). Fixed width so columns to the right line up
// across every list.
const TagGutterWidth = 5

// TagGutterExpandedWidth is the width used in "expanded" mode where
// the gutter renders full tag pills (Name + colour-block padding)
// instead of dots. Wide enough for one short pill ("[bug]" ≈ 5
// cells) plus a "+N" suffix when the row carries multiple tags.
const TagGutterExpandedWidth = 28

func (m Model) tagGutterWidth() int {
	if m.settings == nil {
		return TagGutterWidth
	}
	switch m.settings.TagColumnDisplayMode() {
	case settings.TagColumnModeHidden:
		return 0
	case settings.TagColumnModeExpanded:
		return TagGutterExpandedWidth
	}
	return TagGutterWidth
}

func (m Model) tagColumnExpanded() bool {
	return m.settings != nil &&
		m.settings.TagColumnDisplayMode() == settings.TagColumnModeExpanded
}

func (m Model) listGutters(rowTag, rowProject func(row int) string) (left, right []uilayout.GutterSpec) {
	if w := m.tagGutterWidth(); w > 0 && rowTag != nil {
		left = append(left, uilayout.GutterSpec{
			Width: w, Header: "TAGS", Cell: rowTag,
		})
	}
	if w := m.projectGutterWidth(); w > 0 && rowProject != nil {
		right = append(right, uilayout.GutterSpec{
			Width: w, Header: "PROJECTS", Cell: rowProject,
		})
	}
	return left, right
}

func (m Model) kindRefGutters(kind devproject.ItemKind, n int, refOf func(row int) string) (left, right []uilayout.GutterSpec) {
	if m.devProjects == nil || n == 0 {
		return m.listGutters(nil, nil)
	}
	o, ok := m.currentOrg()
	if !ok {
		return m.listGutters(nil, nil)
	}
	keys := make([]devproject.TagLookupKey, 0, n)
	for i := 0; i < n; i++ {
		ref := refOf(i)
		if ref == "" {
			continue
		}
		keys = append(keys, devproject.TagLookupKey{Kind: kind, Ref: ref})
	}
	if len(keys) == 0 {
		return m.listGutters(nil, nil)
	}
	tagMap, _ := m.devProjects.TagsForItems(o.Username, keys)
	projMap, _ := m.devProjects.ProjectsForItems(o.Username, keys)
	return m.listGutters(
		func(row int) string {
			ref := refOf(row)
			if ref == "" {
				return ""
			}
			return m.resolveTagGutterCell(kind, ref, tagMap)
		},
		func(row int) string {
			ref := refOf(row)
			if ref == "" {
				return ""
			}
			return rowProjectGutterFromMap(kind, ref, projMap)
		},
	)
}

func rowTagGutterFromMap(kind devproject.ItemKind, ref string, tags map[string][]devproject.Tag) string {
	if ref == "" || len(tags) == 0 {
		return ""
	}
	bound, ok := tags[string(kind)+":"+ref]
	if !ok || len(bound) == 0 {
		return ""
	}
	const maxDots = 3
	dots := make([]string, 0, maxDots+1)
	for i, t := range bound {
		if i >= maxDots {
			break
		}
		dots = append(dots,
			lipgloss.NewStyle().Foreground(tagColorFor(t.Color)).Render("●"))
	}
	if len(bound) > maxDots {
		dots = append(dots,
			lipgloss.NewStyle().Foreground(theme.Muted).Render("+"))
	}
	return strings.Join(dots, "")
}

func rowTagGutterPillFromMap(kind devproject.ItemKind, ref string, tags map[string][]devproject.Tag) string {
	if ref == "" || len(tags) == 0 {
		return ""
	}
	bound, ok := tags[string(kind)+":"+ref]
	if !ok || len(bound) == 0 {
		return ""
	}
	suffix := ""
	budget := TagGutterExpandedWidth - 2 // pill padding
	if len(bound) > 1 {
		suffix = " +" + intStr(len(bound)-1)
		budget -= ansi.StringWidth(suffix)
	}
	if budget < 1 {
		budget = 1
	}
	first := bound[0]
	label := first.Name
	if first.Icon != "" {
		label = first.Icon + " " + label
	}
	label = ansi.Truncate(label, budget, "…")
	pill := lipgloss.NewStyle().
		Background(tagColorFor(first.Color)).
		Foreground(theme.Bg).
		Bold(true).
		Padding(0, 1).
		Render(label)
	if suffix == "" {
		return pill
	}
	return pill + lipgloss.NewStyle().Foreground(theme.Muted).Render(suffix)
}

func (m Model) resolveTagGutterCell(kind devproject.ItemKind, ref string, tagMap map[string][]devproject.Tag) string {
	if m.tagColumnExpanded() {
		return rowTagGutterPillFromMap(kind, ref, tagMap)
	}
	return rowTagGutterFromMap(kind, ref, tagMap)
}

// intStr is a tiny strconv.Itoa to avoid importing strconv just for
// this. Caller never passes negative values (suffix is "+N" with
// N>=1) so we don't bother handling the sign.
func intStr(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// openTagPickerForCursored is the `t` keybind dispatcher. Resolves
// the cursored item's (kind, ref, title) for the active tab and
// opens the tag picker. Returns the modal-open Cmd (currently nil
// since openTagPicker doesn't kick off async work) or nil when the
// active tab doesn't support tagging.
//
// New tabs that should support tagging extend the switch below.
// Each branch resolves its own state (cursor, list, active org)
// because the existing per-surface state is the source of truth —
// duplicating it into a shared "tag target" interface would just
// add an indirection layer.
func (m *Model) openTagPickerForCursored() tea.Cmd {
	o, ok := m.currentOrg()
	if !ok {
		return nil
	}
	id, ok := m.resolveItemIdentity()
	if !ok {
		return nil
	}
	return m.openTagPicker(id.Kind, id.Ref, o.Username, id.Label)
}

func (m Model) openBulkTagPickerForVisible() (Model, tea.Cmd) {
	o, ok := m.currentOrg()
	if !ok {
		m.flash("no org selected")
		return m, nil
	}
	surf := m.resolveListSurface()
	if surf == nil || surf.BulkTagTargets == nil {
		m.flash("bulk tagging isn't available on this view")
		return m, nil
	}
	d := m.activeOrgData()
	if d == nil {
		return m, nil
	}
	kind, refs, ok := surf.BulkTagTargets(d)
	if !ok {
		m.flash("nothing visible to tag")
		return m, nil
	}
	title := fmt.Sprintf("Tag %d visible %s row(s)", len(refs), kind)
	cmd := (&m).openBulkTagPicker(kind, refs, o.Username, title)
	return m, cmd
}
