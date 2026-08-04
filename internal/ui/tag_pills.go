package ui

// Tag pill rendering — sidebar + chip-strip visual surface for tags.
//
// Tags are theme-colour-named ("blue", "purple", "red", …) rather
// than hex so they re-tint when the user switches themes. Names map
// to the seven palette accent colors (Blue/Cyan/Green/Yellow/Red/
// Magenta/Orange); unknown / empty colors fall through to Border.
//
// A pill is a block-coloured background + the tag name (with optional
// unicode icon prefix). Pills sit in the sidebar TAGS section and
// appear inline in the chip strip when a tag is filtered on.

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

// tagColorFor maps a tag's stored color name to a theme palette
// entry. Empty / unknown names default to Border so unstyled tags
// still render cleanly.
//
// The palette names match the public theme color vars so the
// management modal can let the user pick from a small fixed list
// without us having to maintain a parallel registry.
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

// tagPalette is the ordered list of color names exposed by the tag
// editor. Used by the tag-management modal so users have a fixed,
// theme-friendly palette to choose from rather than typing hex.
//
// "magenta" / "purple" / "pink" are aliases for the same theme color
// — the editor exposes "purple" as the user-facing label since it's
// the most natural reading of the slot.
var tagPalette = []string{"blue", "cyan", "green", "yellow", "red", "purple", "orange"}

// nextRotatingTagColor picks a default palette colour by position, so
// tags created inline (where the user doesn't pick a colour) cycle
// through the palette instead of all being blue. Wraps at the palette
// length.
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

// renderTagPills joins a slice of tags into a single line of pills
// separated by spaces. Empty input → empty string so callers can
// emit-or-skip without a length check.
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

// sidebarTagSection returns the rendered TAGS block for an item, or
// "" when the item has no tags / the store is unavailable.
//
// inner is the available width; the section wraps pills onto multi-
// ple lines if a single line wouldn't fit. The header line uses
// sideSection so it visually matches every other sidebar section.
//
// orgUser is the originating org username — passed through directly
// to the store's per-org binding lookup. Empty string is fine for
// surfaces that aren't org-scoped (none today, but keeps the API
// uniform).
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

// tagGutterWidth returns the effective gutter width for the active
// 3-state tag-column mode (Ctrl+T cycles): hidden=0, compact=dots,
// expanded=pills. Width 0 makes the listtable layout skip the
// gutter entirely.
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

// tagColumnExpanded reports whether the gutter should render full
// pills rather than coloured dots. Used by the row-cell renderer to
// pick the right shape.
func (m Model) tagColumnExpanded() bool {
	return m.settings != nil &&
		m.settings.TagColumnDisplayMode() == settings.TagColumnModeExpanded
}

// listGutters builds the per-render gutter slices for any list-table
// surface. Returns two slices: the LEFT gutters render before the
// regular columns (cursor-bar adjacent), the RIGHT gutters render
// after.
//
// Layout policy: TAGS go left (user-curated identity, anchored next
// to the row name), PROJECTS go right (system-derived membership,
// less actionable per-row, OK to be the first thing elided when the
// terminal narrows). Marks live as a regular column inside the
// surface's own Cols definition — see e.g. sobjectListCols.
//
// rowTag and rowProject closures may be nil (passed nil when the
// list doesn't support that gutter). Width-0 gutters (toggled off)
// are filtered out so the listtable layout sees clean slices.
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

// kindRefGutters builds the standard TAGS + PROJECTS gutter pair
// for a generic list of items where each item has a stable (kind,
// ref) key. The closure converts a row index into its ref string;
// kind is the same for every row in the list (e.g. KindPermissionSet
// for the perms surface).
//
// Pre-fetches both maps in a single round-trip to TagsForItems and
// ProjectsForItems so the gutter render is map-lookup, not store-
// query, per row.
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

// rowTagGutterFromMap renders one row's tag-gutter cell in COMPACT
// mode — up to three coloured dots followed by a "+" if the row
// carries more tags than fit. Returns empty string when the row has
// no tags so the gutter cell renders blank (still occupying
// TagGutterWidth so columns to the right stay aligned).
//
// Each dot uses the corresponding tag's colour. Multi-tag rows show
// the first three tags in the order TagsFor returns them
// (alphabetical) so the same row always shows the same dots across
// renders.
//
// For the expanded (pill) variant, see rowTagGutterPillFromMap. The
// model-level resolveTagGutterCell picks between the two based on
// the user's Ctrl+T cycle state.
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

// rowTagGutterPillFromMap is the EXPANDED-mode variant — renders
// the first tag as a full coloured pill (Name with background tint),
// with a "+N" suffix when the row carries more than one tag. Picked
// by resolveTagGutterCell when the user has cycled the column to
// expanded via Ctrl+T.
//
// Truncates the first tag's name to fit the gutter width minus pill
// padding (2 cells) minus the worst-case "+N" suffix (3 cells when
// N ≤ 9, 4 when N ≤ 99). Returns "" for untagged rows so the cell
// renders blank.
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

// resolveTagGutterCell renders one row's tag cell in whichever mode
// the user has cycled the gutter to. Each surface's BuildRenderModel
// closure delegates here so the per-surface code is mode-agnostic.
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

// openBulkTagPickerForVisible opens the tag picker over every row of
// the active surface's CURRENT filtered view (the T keybind). The
// workflow is filter-then-tag: narrow the list with / or a chip, hit
// T, tick tags, enter.
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
