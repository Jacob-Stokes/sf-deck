package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// --- generic KV panel ----------------------------------------------------
//
// Most sidebars are just "title + list of k:v rows + optional extra
// sections." renderKVPanel gives every one of them the same shape:
//
//   renderKVPanel(inner, "Account", []kv{{"label", "Account"}, ...})
//
// Extra sections (picklist values, package dirs, limits list) are still
// bespoke at the end of each view's renderer.

type kv struct{ K, V string }

func renderKVPanel(inner int, title string, rows []kv, extra ...string) string {
	return renderKVPanelWithPills(inner, title, nil, rows, extra...)
}

// renderKVPanelWithPills is renderKVPanel + an optional pill row
// rendered just under the title. Each pill is a small bracketed
// label tinted by its color (managed/custom/invalid markers); same
// vocabulary as the list-table row marks. Empty pill slice → no
// extra row, output identical to renderKVPanel.
// compactSidebarPills reports whether flag pills should sit INLINE on
// the title line (saving a vertical row) rather than on their own row
// below it. True only in stacked mode (sidebar below main, where
// vertical space is scarce) and never while building the inspect modal
// (which has room for the full layout). The RHS-beside sidebar keeps
// the separate pill row.
func (m Model) compactSidebarPills() bool {
	return m.sidebarStacked && !m.sidebarForModal
}

// kvPanelPills renders a KV panel, placing the flag pills inline on the
// title line when compactSidebarPills() (stacked mode) or on a separate
// row otherwise.
func (m Model) kvPanelPills(inner int, title string, pills []markPill, rows []kv, extra ...string) string {
	if m.compactSidebarPills() {
		if pillRow := renderMarkPills(pills); pillRow != "" {
			// Inline: "<title>  <pills>" on one line, no separate row.
			return renderKVPanelTitled(inner, sideTitle(title)+"  "+pillRow, rows, extra...)
		}
	}
	return renderKVPanelWithPills(inner, title, pills, rows, extra...)
}

// kvPanelTagged is kvPanelPills plus tag/project surfacing. In STACKED
// mode the tags are appended to the title line (as pills) and the
// projects are right-aligned on that same line — moving them OUT of the
// scrollable body, where a narrow stacked column truncated them and
// forced the user into the inspect (i) modal to read them. In the
// RHS-beside sidebar (roomy) the header stays clean and tags/projects
// keep their normal body sections (the caller still appends
// sidebarTagsProjectsSection to extra as before).
//
// kind/ref/orgUser identify the item for the tag + project lookups.
func (m Model) kvPanelTagged(inner int, title string, pills []markPill, kind devproject.ItemKind, ref, orgUser string, rows []kv, extra ...string) string {
	if !m.compactSidebarPills() {
		// RHS mode — unchanged: flag pills via kvPanelPills, tags/projects
		// live in the body (already appended to extra by the caller).
		return m.kvPanelPills(inner, title, pills, rows, extra...)
	}

	// Stacked mode: fold tags into the title line and right-align
	// projects, moving both out of the truncation-prone body.
	titleLine := m.stackedTitleWithTagsProjects(sideTitle(title),
		renderMarkPills(pills), kind, ref, orgUser, inner)
	return renderKVPanelTitled(inner, titleLine, rows, extra...)
}

// stackedTitleWithTagsProjects composes the stacked-mode sidebar title
// line: "<styledTitle> <flagPills> <tagPills> .... <projectPills>", with
// projects right-aligned to inner. flagPills may be "" (no mark pills).
// Projects that don't fit on the line fall to their own line beneath it,
// so they're never lost. Shared by kvPanelTagged and the bespoke field
// sidebar (which emits its own title).
func (m Model) stackedTitleWithTagsProjects(styledTitle, flagPills string, kind devproject.ItemKind, ref, orgUser string, inner int) string {
	left := styledTitle
	if flagPills != "" {
		left += "  " + flagPills
	}
	tagPills, projectPills := m.tagsProjectsHeaderPills(kind, ref, orgUser)
	if tagPills != "" {
		left += "  " + tagPills
	}
	// During a NOTE-box split the body column is narrowed but the title
	// row still owns the full panel width (the box starts a row below),
	// so right-align the projects to the full width when it's set.
	if m.sidebarTitleW > inner {
		inner = m.sidebarTitleW
	}
	return placeTitleAndProjects(left, projectPills, inner)
}

// placeTitleAndProjects right-aligns projectPills after the left segment
// on a single line when there's room (>=2 cells of gap), otherwise drops
// projectPills onto its own indented line so it's never lost. Empty
// projectPills returns left unchanged. Pure layout math — unit-tested.
func placeTitleAndProjects(left, projectPills string, inner int) string {
	if projectPills == "" {
		return left
	}
	gap := inner - ansi.StringWidth(left) - ansi.StringWidth(projectPills)
	if gap >= 2 {
		return left + strings.Repeat(" ", gap) + projectPills
	}
	// No room to right-align — tuck projects onto their own line.
	return left + "\n  " + projectPills
}

// tagsProjectsHeaderPills returns the inline pill strings for an item's
// tags and projects, for the stacked-mode header. Either may be "".
// Projects render as their sidebar pills (single name, or "N projects");
// tags render as their normal coloured pills.
func (m Model) tagsProjectsHeaderPills(kind devproject.ItemKind, ref, orgUser string) (tagPills, projectPills string) {
	if m.devProjects == nil || ref == "" {
		return "", ""
	}
	if tags, err := m.devProjects.TagsFor(kind, ref, orgUser); err == nil && len(tags) > 0 {
		tagPills = renderTagPills(tags)
	}
	if projects, err := m.devProjects.ProjectsForItem(kind, ref, orgUser); err == nil && len(projects) > 0 {
		projectPills = renderProjectPills(projects)
	}
	return tagPills, projectPills
}

// renderKVPanelTitled is renderKVPanel with a pre-styled title line (so
// callers can append inline pills); rows + extra render as usual.
func renderKVPanelTitled(inner int, titleLine string, rows []kv, extra ...string) string {
	var out []string
	out = append(out, titleLine)
	out = append(out, sideSeparator(inner))
	for _, r := range rows {
		if r.V == "" {
			continue
		}
		out = append(out, sideKV(r.K, r.V, inner))
	}
	if len(extra) > 0 {
		out = append(out, extra...)
	}
	return strings.Join(out, "\n")
}

func renderKVPanelWithPills(inner int, title string, pills []markPill, rows []kv, extra ...string) string {
	var out []string
	out = append(out, sideTitle(title))
	if pillRow := renderMarkPills(pills); pillRow != "" {
		out = append(out, "  "+pillRow)
	}
	out = append(out, sideSeparator(inner))
	for _, r := range rows {
		if r.V == "" {
			continue
		}
		out = append(out, sideKV(r.K, r.V, inner))
	}
	if len(extra) > 0 {
		out = append(out, extra...)
	}
	return strings.Join(out, "\n")
}

// --- per-view dispatcher ------------------------------------------------

func (m Model) renderSidebar(w, h, innerH int) string {
	inner := w - 4
	if inner < 10 {
		inner = 10
	}
	// Cursored item's note (if any) carves out a NOTE box: stacked
	// mode splits the width (note ~1/4, full height bar the title and
	// footer-button rows); beside mode carves a content-sized box off
	// the bottom (10-row floor, 1/3 cap, above the footer-button row).
	// Skipped when the panel is too small to split — q-n still shows
	// the full note.
	note := m.cursorNoteBody()
	stackedNote := note != "" && m.sidebarStacked && inner >= noteBoxMinStackedInner && innerH >= 6
	besideNote := note != "" && !m.sidebarStacked && innerH >= noteBoxMinBesideInnerH
	contentInner := inner
	if stackedNote {
		boxW := inner / 4
		if boxW < 12 {
			boxW = 12
		}
		contentInner = inner - boxW - 2
		// Title rows keep the full panel width (see sidebarTitleW) —
		// only the body narrows beside the note box.
		m.sidebarTitleW = inner
	}
	content := ""
	if m.disconnectedOrgNotice(contentInner) != "" {
		content = sideEmpty("org disconnected")
	} else {
		content = m.resolveSidebar(contentInner)
	}
	// Universal truncation guard: ANY sidebar panel whose content is
	// clipped — vertically (taller than innerH) or horizontally (a line
	// truncated with an ellipsis) — gets a red warning stamped on its
	// last visible row pointing at the inspect shortcut. This lives here
	// (the one choke point every RHS panel flows through) so it works
	// generically, not per-panel. The inspect modal works for any non-
	// empty sidebar, so the shortcut is always offered.
	switch {
	case stackedNote:
		content = stampSidebarTruncation(content, contentInner, innerH, content != "")
		// Height innerH-3: row 0 is the panel title, innerH-2 is the
		// footer-button row (stampSidebarFooterButtons writes there —
		// a taller box loses its bottom border to the buttons), and
		// innerH-1 stays empty for breathing room.
		box := renderNoteBox(inner-contentInner-2, innerH-3, note)
		content = joinNoteBeside(clipLines(content, innerH), box, contentInner, innerH)
	case besideNote:
		// Content-sized between a 10-row floor and a 1/3-of-panel cap:
		// short notes get a stable 10-row box (room to grow into
		// without the layout jumping), long ones stop at 1/3 so they
		// never crowd out the panel. When the panel is so short that
		// 1/3 < 10, the floor wins — the min-height gate above keeps
		// that from starving the content column.
		noteH := noteBoxNeededHeight(inner, note)
		if maxH := innerH / 3; noteH > maxH {
			noteH = maxH
		}
		if noteH < noteBoxBesideMinH {
			noteH = noteBoxBesideMinH
		}
		// -2: the footer-button row (innerH-2, where the stamp writes —
		// any overlap eats the box's bottom border) plus the final
		// breathing-room row both stay free below the box.
		contentBudget := innerH - noteH - 2
		content = stampSidebarTruncation(content, inner, contentBudget, content != "")
		box := renderNoteBox(inner, noteH, note)
		content = padLinesTo(clipLines(content, contentBudget), contentBudget) + "\n" + box
	default:
		content = stampSidebarTruncation(content, inner, innerH, content != "")
	}
	// Stamp the hide+stack icon buttons on the bottom-right row. The
	// click hit-layers are registered separately in render.go (they
	// need absolute coordinates the sidebar render can't compute).
	content = stampSidebarFooterButtons(content, inner, innerH)
	// Bright border when the sidebar is the active pane —
	// SidebarFocusable tab + bodyFocus=false. Mirrors the main pane's
	// PanelledFocus styling so the user can see at a glance which
	// pane Tab landed on.
	style := theme.Panelled
	if m.focus == focusMain {
		if spec := lookupTabSpec(m.tab()); spec != nil && spec.SidebarFocusable && !m.bodyFocus {
			style = theme.PanelledFocus
		}
	}
	return style.Width(w).Height(h).MaxHeight(h).Render(clipLines(content, innerH))
}

// sidebarFooterButtons returns the two button strings rendered at
// the bottom-right of the sidebar. Each button carries its full
// hint text (label + the actual bound key, surfaced via firstPretty)
// so the user can read what the button does without hovering. The
// click hit-layer over each button (see sidebarFooterHitLayers)
// makes the whole button area mouse-targetable.
//
// The returned `stackedBtn` renders RIGHTMOST; `hideBtn` is just
// to its left with one space between. theme.Muted matches the
// footer-hint styling already used elsewhere in the sidebar.
func sidebarFooterButtons() (hideBtn, stackedBtn string, totalWidth int) {
	style := lipgloss.NewStyle().Foreground(theme.Muted)
	hideKey := firstPretty(Keys.ToggleSidebar)
	stackKey := firstPretty(Keys.ToggleSidebarStacked)
	if hideKey == "" {
		hideKey = `\`
	}
	if stackKey == "" {
		stackKey = `^\`
	}
	hideBtn = style.Render("[ " + hideKey + " hide ]")
	stackedBtn = style.Render("[ " + stackKey + " stack ]")
	totalWidth = lipgloss.Width(hideBtn) + 1 + lipgloss.Width(stackedBtn)
	return
}

// sidebarFooterHitLayers returns invisible click-target layers
// positioned over the two icon buttons stamped in
// stampSidebarFooterButtons. Anchored relative to the sidebar
// layer's own origin — caller is render.go which already places
// the sidebar layer at (bodyX, panelY), so these compose to
// absolute hit boxes via lipgloss layer nesting.
//
// w / h are the sidebar's outer dimensions (including its border).
// Buttons sit one row above the inner bottom so the global footer
// hint row has breathing room — outer-Y = h - 3 (top border + the
// (innerH - 2) row we stamp to).
func sidebarFooterHitLayers(w, h int) []*lipgloss.Layer {
	if h < 5 {
		return nil
	}
	hide, stack, btnW := sidebarFooterButtons()
	// Register hit zones ONLY when the buttons are actually stamped.
	// stampSidebarFooterButtons bails when inner (= w-4) < btnW+2, so
	// mirror that exact threshold here — otherwise (at sideW==28, the
	// common narrow-terminal case) the invisible click layers were
	// live over the sidebar's bottom-right while no buttons were drawn,
	// so a click on blank space silently hid the sidebar / toggled
	// stacked mode.
	if w-4 < btnW+2 {
		return nil
	}
	hideW := lipgloss.Width(hide)
	stackW := lipgloss.Width(stack)
	// X positions (sidebar-local): one cell of right padding from
	// the border, then [stack][space][hide] reading right-to-left.
	stackX := w - 2 - stackW
	hideX := stackX - 1 - hideW
	if hideX < 2 {
		return nil
	}
	y := h - 3
	if y < 1 {
		return nil
	}
	return []*lipgloss.Layer{
		lipgloss.NewLayer(strings.Repeat(" ", hideW)).
			X(hideX).Y(y).Z(2).ID(zoneSidebarHide),
		lipgloss.NewLayer(strings.Repeat(" ", stackW)).
			X(stackX).Y(y).Z(2).ID(zoneSidebarStack),
	}
}

// stampSidebarFooterButtons replaces the last visible row of the
// sidebar content with the original line plus the two right-aligned
// icon buttons. Returns the updated multiline string. When the
// sidebar is too narrow to fit both buttons + at least 2 chars of
// the original last line, returns content unchanged.
func stampSidebarFooterButtons(content string, inner, innerH int) string {
	if content == "" || innerH < 1 {
		return content
	}
	hide, stack, btnW := sidebarFooterButtons()
	// Bail out when the sidebar can't even fit the buttons by
	// themselves — narrow stacked layouts on tiny terminals.
	if inner < btnW+2 {
		return content
	}
	lines := strings.Split(content, "\n")
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	// Ensure we have innerH lines so the buttons sit at a stable Y
	// regardless of content height — otherwise short sidebars would
	// float the buttons up beside the content, looking out of place.
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	// Buttons sit one row ABOVE the bottom so the global footer hint
	// row that sf-deck draws under every pane has breathing room
	// (avoids the buttons abutting "r refresh · \ side · ?  help …").
	// Falls back to innerH-1 only when the sidebar is too short to
	// give up a row.
	btnRow := innerH - 2
	if btnRow < 0 {
		btnRow = innerH - 1
	}
	last := lines[btnRow]
	lastW := lipgloss.Width(last)
	if lastW+1+btnW > inner {
		// Not enough room — drop the original last line entirely
		// and right-align the buttons on a blank row.
		last = ""
		lastW = 0
	}
	padding := inner - lastW - btnW
	if padding < 0 {
		padding = 0
	}
	lines[btnRow] = last + strings.Repeat(" ", padding) + hide + " " + stack
	return strings.Join(lines, "\n")
}

// truncSentinel is a zero-width marker a panel embeds in its content to
// say "I lost data to horizontal truncation / column reflow — flag it."
// We use an explicit sentinel rather than scanning for "…" because a
// bare ellipsis is ambiguous: hint/footer lines and long values are
// truncated BY DESIGN and must NOT trigger the warning. The sentinel is
// stripped before display.
const truncSentinel = "\x00TRUNC\x00"

// stampSidebarTruncation detects whether content is clipped and, if so,
// replaces its last visible row with a red truncation warning. inner is
// the panel content width, innerH the visible height. inspectable picks
// the hint text — when the current view has a full-info modal we point
// at the inspect shortcut; otherwise we just flag "(truncated)".
//
// Detection is precise: VERTICAL clipping (more lines than fit) always
// flags; HORIZONTAL truncation flags ONLY when a panel embedded
// truncSentinel (the column-reflow panels do this when a cell is cut).
// A bare "…" never triggers it — too many hint/value lines truncate by
// design.
func stampSidebarTruncation(content string, inner, innerH int, inspectable bool) string {
	if content == "" || innerH <= 1 {
		return stripTruncSentinel(content)
	}
	hadSentinel := strings.Contains(content, truncSentinel)
	content = stripTruncSentinel(content)
	lines := strings.Split(content, "\n")
	clippedV := len(lines) > innerH
	clippedH := inspectable && hadSentinel
	if !clippedV && !clippedH {
		return content
	}
	warn := sidebarTruncationWarning(inner, inspectable)
	if clippedV {
		// Vertical clip: keep the first innerH-1 lines, warning last.
		keep := innerH - 1
		if keep < 0 {
			keep = 0
		}
		if keep > len(lines) {
			keep = len(lines)
		}
		lines = append(lines[:keep], warn)
		return strings.Join(lines, "\n")
	}
	// Horizontal-only clip: content fits height-wise, so append the
	// warning if there's a spare row; otherwise overwrite the last.
	if len(lines) < innerH {
		lines = append(lines, warn)
	} else {
		lines[len(lines)-1] = warn
	}
	return strings.Join(lines, "\n")
}

// stripTruncSentinel removes the zero-width truncation marker(s) so
// they never reach the screen.
func stripTruncSentinel(s string) string {
	if !strings.Contains(s, truncSentinel) {
		return s
	}
	return strings.ReplaceAll(s, truncSentinel, "")
}

// sidebarTruncationWarning is the red line shown when a sidebar panel
// is clipped. Points at the inspect shortcut when one is available.
func sidebarTruncationWarning(inner int, inspectable bool) string {
	msg := "⚠ truncated"
	if inspectable {
		msg += " — " + firstPretty(Keys.InspectPanel) + " for full"
	}
	return lipgloss.NewStyle().Foreground(theme.Red).Bold(true).
		Render(ansi.Truncate("  "+msg, inner, "…"))
}

// activeListPaginated reports whether the surface the user is
// currently looking at has its list-table state in paginated mode.
// Used by the wheel-event dispatcher to pick the per-mode handler.
func (m Model) activeListPaginated() bool {
	mp := &m
	state := mp.activeListTableState()
	if state == nil {
		return false
	}
	return state.Paginated
}

// sidebarSystemAPI is the placeholder right-pane for the /system
// API subtab — until that view gets a real list-table-shaped
// payload, the sidebar just shows a static hint.

// sidebarObjectDetailDispatch routes to the right per-subtab sidebar
// for ObjectDetail, whose subtabs aren't declared in TabSpec.Subtabs
// (they fan out from a render-time switch instead). Mirror of the
// Identity dispatcher; explicit branches per subtab — no default
// fallback so a future SubtabXX without an entry no-ops cleanly.

// resolveSidebar walks the spec resolver chain (subtab → tab) and
// returns the first non-nil Sidebar closure's output. Empty string
// when no resolver applies.
func (m Model) resolveSidebar(inner int) string {
	spec, sub := m.activeSpec()
	if sub != nil && sub.Sidebar != nil {
		return sub.Sidebar(m, inner)
	}
	if spec != nil && spec.Sidebar != nil {
		return spec.Sidebar(m, inner)
	}
	return ""
}

// sidebarObjectDetailRecord is the Object-drill Records-subtab sidebar.
// Shows the selected row's full KV. Source depends on the active chip:
// synthetic "recent" pulls from d.Records; a Salesforce list view
// pulls from d.ListViewResults.

// sidebarRecords shows the full selected record as KV when in record-
// list mode, or the selected sObject's summary when in picker mode.

// Picker mode → reuse the sObject sidebar verbatim.

// Record-list mode → show the selected record as a KV dump.
// Route via currentRecordAt so the row reflects the active search
// filter (and the chip-records path, not just bare Records).

// Id first, then every other field sorted.

// Deterministic order for display.

// sortStrings is a local shim so we don't pull `sort` into sidebar.go
// just for the one call site.
func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j-1] > xs[j]; j-- {
			xs[j-1], xs[j] = xs[j], xs[j-1]
		}
	}
}

// --- per-view renderers -------------------------------------------------

// Per-row metadata available straight from the EntityDefinition
// list query — no describe fetch required, so these always show.

// If we have a cached describe for this one, show the deeper
// caps split into individual rows (the "QCUD" mnemonic is opaque
// without context). Each cap also implies what actions are safe
// to wire up on this sObject.

// sidebarApex routes the right-pane render for /apex by active
// subtab. Classes + Triggers each get their own renderer; VF Pages
// + Components fall through to an empty placeholder until they
// have detail data.

// sidebarApexClass renders the cursored Apex class's metadata in
// the right pane. Pills above the kv rows show managed-package
// status + IsValid so they're scannable without parsing column
// values.

// Character count (body minus comments), not lines — see the
// Apex SIZE column note in list_column_schemas.go.

// sidebarApexTrigger is sidebarApexClass's twin for the Triggers
// subtab. Same shape; trigger-specific fields swap in (parent
// sObject, events).

// sidebarField is the full "everything about this field" detail view.
// Structured as Object-Manager-style sections (IDENTITY · CONSTRAINTS ·
// REFERENCE · PICKLIST · FORMULA · SECURITY · SOQL) so admins don't
// have to scan a single flat kv dump.
//
// Each section is optional — reference-less fields omit REFERENCE,
// non-picklists omit PICKLIST, and so on.

// A describe that ERRORED never sets FetchedAt, so without the
// Err check this sidebar spins "loading describe…" forever while
// the main pane already shows the failure (the inaccessible
// managed-object NOT_FOUND case). Surface it instead of hanging.

// Each section builds a []kv (or plain strings) and is joined at
// the end. Sections separate with a blank line + dim title.

// --- IDENTITY ---

// --- CONSTRAINTS ---

// --- REFERENCE ---

// --- PICKLIST ---

// --- FORMULA / DEFAULT ---

// --- SECURITY / BEHAVIOR ---

// --- SOQL CAPABILITIES ---

// --- HELP ---

// --- TAGS / PROJECTS ---

// In a short slot (stacked layout) the flat list overflows + gets
// clipped — losing CONSTRAINTS / SOQL at the bottom. Reflow the
// already-short kv lines into columns to fit the height. The
// generic truncation guard in renderSidebar stamps the warning if
// anything still clips.

// sidebarFieldBudget is the content-height the schema-subtab field
// sidebar may use before it should reflow into columns. The title +
// separator that renderKVPanel-style headers add aren't present here
// (sidebarField emits its own title), so the budget is the full
// available sidebar height.

// reflowLinesToBudget packs an already-rendered, mostly-short line list
// into up to 3 balanced columns so it fits budget rows. Lines are NOT
// re-wrapped (callers pass short kv rows); over-long lines truncate to
// the column width. Returns reflowed=true when it had to column-pack
// (so the caller can warn that info is denser/clipped). When the list
// already fits, or budget is unknown, it's returned unchanged.
func reflowLinesToBudget(lines []string, inner, budget int) (out []string, reflowed bool) {
	if budget <= 0 || len(lines) <= budget {
		return lines, false
	}
	for cols := 2; cols <= 3; cols++ {
		perCol := (len(lines) + cols - 1) / cols
		if perCol > budget {
			continue
		}
		gutter := 2
		colW := (inner - gutter*(cols-1)) / cols
		if colW < 10 {
			break
		}
		columns := make([][]string, cols)
		for c := 0; c < cols; c++ {
			start := c * perCol
			end := start + perCol
			if start > len(lines) {
				start = len(lines)
			}
			if end > len(lines) {
				end = len(lines)
			}
			col := make([]string, perCol)
			for i := 0; i < perCol; i++ {
				if start+i < end {
					col[i] = padRight(ansi.Truncate(lines[start+i], colW, "…"), colW)
				} else {
					col[i] = strings.Repeat(" ", colW)
				}
			}
			columns[c] = col
		}
		packed := make([]string, perCol)
		gap := strings.Repeat(" ", gutter)
		for r := 0; r < perCol; r++ {
			parts := make([]string, cols)
			for c := 0; c < cols; c++ {
				parts[c] = columns[c][r]
			}
			packed[r] = strings.Join(parts, gap)
		}
		if len(packed) <= budget {
			// Reflow happened — the card is denser than its natural
			// single column (and cells may be ellipsis-cut). Mark it so
			// the generic guard offers the full-info modal.
			if len(packed) > 0 {
				packed[len(packed)-1] += truncSentinel
			}
			return packed, true
		}
	}
	return lines, true
}

// sidebarFieldTypeDisplay matches the TYPE column's virtual-type
// expansion so the sidebar is consistent with the table.

// yesNo is a tiny formatter for the boolean kv rows in the sidebar.
// Renders "yes" in the default fg, "no" dimmed so a column of booleans
// reads as a visual pattern (active rows stand out).
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return lipgloss.NewStyle().Foreground(theme.FgDim).Render("no")
}

func stringish(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	switch x := v.(type) {
	case string:
		return x, true
	case bool:
		if x {
			return "true", true
		}
		return "false", true
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x)), true
		}
		return fmt.Sprintf("%g", x), true
	}
	return "", false
}

// CreatedBy / CreatedDate live in the sidebar only — the row
// stays compact with no dedicated CREATED column. Chip predicates
// (e.g. the "Created by me" built-in) can still filter on these
// fields via sf.Flow.Field().

// Active marker.

// itoaFn avoids name collision with the update.go private itoa used by
// activate(); both do the same thing, but keeping them local avoids
// introducing a shared helper just for this.
func itoaFn(n int) string { return fmt.Sprintf("%d", n) }

// humanBytes formats a byte count with a unit suffix. Apex log sizes
// run from a few KB to several MB; raw byte counts are unreadable.
func humanBytes(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
	}
}

// compactChars formats a raw character count into a short, column-
// friendly form: "847", "3.2K", "992K", "1.4M". Decimal (1000-based)
// units — this is a code-size figure, not a disk allocation, so plain
// thousands read more naturally than KiB. Used for the Apex "SIZE"
// column (ApexClass.LengthWithoutComments, a char count — NOT lines).
func compactChars(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 1000*1000:
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/(1000*1000))
	}
}

// While the user is actively editing the query, the most useful
// sidebar content is the FROM sObject's schema — field list with
// types + capability flags. Mirrors what Inspector Reloaded shows
// in its right-hand pane while building a query.

// sidebarSOQLSchema renders the describe of the FROM sObject (when
// resolvable) as a scannable field reference. Returns "" when the
// query has no FROM yet OR the describe isn't loaded yet — caller
// falls through to the result-row sidebar in that case.
//
// Layout: header with sObject name + label, then per-field rows
// showing `apiName  type  badges`. Reference fields show their
// target sObject; picklist fields show value count; required
// fields get a `*` marker.

// Trigger a fetch if not cached. The render returns "loading…"
// and re-runs naturally when the describe lands.

// Describe not yet cached. The autocomplete engine's
// EnsureDescribe path (fired on every editor keystroke)
// kicks the fetch, so the next render after the user
// types ANY key will land the describe and we'll render
// the schema. Until then, just show "loading…".

// Build the field list. Sort: NameField first, then Id, then
// rest alphabetical — easier to scan.

// extractFromSObject returns the FROM target of a SOQL query. Uses
// the same FROM regex parseSelectColumns relies on, but pulled out
// to a one-shot helper so the sidebar doesn't need to walk the
// projection.
func extractFromSObject(query string) string {
	m := fromSObjectRe.FindStringSubmatch(query)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// renderSchemaFieldRow formats one field row in the schema sidebar.
// Layout: "  fieldName  type · flags". Reference fields append
// "→ TargetSObject". Picklists append the value count. Wider
// sidebar widths reveal more detail; narrow widths truncate.

// Capability badges: only show the ones that DIFFER from the
// usual defaults (filterable+sortable are common; non-
// filterable is the notable case).

// Hard-truncate to the sidebar inner width so long custom
// API names don't wrap.

// fromSObjectRe matches "FROM <name>" anywhere in the query.
// Mirrors parseSelectColumns' approach but doesn't need paren
// awareness because the FROM keyword inside a subquery would have
// a different parent sObject — and the schema sidebar only cares
// about the outer FROM.
var fromSObjectRe = mustCompileFromRe()

func mustCompileFromRe() *fromRe {
	return &fromRe{}
}

// fromRe is a tiny case-insensitive matcher; using a custom impl
// rather than `regexp` because we want a stable callsite without
// pulling regexp into sidebar.go's import set.
type fromRe struct{}

func (r *fromRe) FindStringSubmatch(s string) []string {
	// Strip parens to skip subquery FROMs.
	depth := 0
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '(' {
			depth++
			continue
		}
		if c == ')' {
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth == 0 {
			b.WriteByte(c)
		}
	}
	flat := b.String()
	lower := strings.ToLower(flat)
	idx := strings.Index(lower, "from ")
	if idx < 0 {
		// Try a tab-separated FROM as well.
		idx = strings.Index(lower, "from\t")
		if idx < 0 {
			idx = strings.Index(lower, "from\n")
		}
		if idx < 0 {
			return nil
		}
	}
	// Word-boundary check on the left.
	if idx > 0 {
		prev := flat[idx-1]
		if prev != ' ' && prev != '\t' && prev != '\n' && prev != '\r' {
			return nil
		}
	}
	start := idx + 5
	// Skip whitespace after FROM.
	for start < len(flat) && (flat[start] == ' ' || flat[start] == '\t' || flat[start] == '\n') {
		start++
	}
	end := start
	for end < len(flat) {
		c := flat[end]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' {
			end++
			continue
		}
		break
	}
	if end == start {
		return nil
	}
	return []string{flat[idx:end], flat[start:end]}
}

// 1. Cloud banner — animates while the user is on /home unless
//    disabled (static) or hidden (skipped entirely). Falls back to
//    the local alias when OrgInfo hasn't landed yet.

// 2. ORG details — moved out of the main pane, condensed to a
//    KV list. Long values wrap onto a second line via sideKV.

// Limits used to render here as a flat KV block, but they now
// live in the dedicated /home → Limits subtab where they get
// sort / search / column-mode / full-row highlight / Lightning
// open via `o`. Sidebar stays focused on the org-identity card.

// --- sidebar visual helpers ---------------------------------------------

func sideTitle(s string) string {
	return lipgloss.NewStyle().Foreground(theme.Blue).Bold(true).Render(s)
}

func sideSection(s string) string {
	return lipgloss.NewStyle().Foreground(theme.Muted).Bold(true).Render(strings.ToUpper(s))
}

func sideSeparator(w int) string {
	if w < 4 {
		w = 4
	}
	return lipgloss.NewStyle().Foreground(theme.Border).Render(strings.Repeat("─", w))
}

func sideKV(k, v string, width int) string {
	keyStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	valStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	if ansi.StringWidth(k+"  "+v) <= width {
		return keyStyle.Render(k) + "  " +
			valStyle.Render(ansi.Truncate(v, width-ansi.StringWidth(k)-2, "…"))
	}
	return keyStyle.Render(k) + "\n  " + valStyle.Render(wrap(v, width-2))
}

// sideDim renders dim sidebar text, truncating PER LINE. The
// previous version ansi.Truncate'd the whole string as one line —
// so every multi-line block (sideDim(wrap(description))) collapsed
// to its first line + "…" with acres of empty panel below it
// (field report 2026-06-13). Vertical overflow is the panel
// clipper's job (stampSidebarTruncation + the i inspect modal),
// not this function's.
func sideDim(s string, width int) string {
	style := lipgloss.NewStyle().Foreground(theme.FgDim)
	if !strings.Contains(s, "\n") {
		return style.Render(ansi.Truncate(s, width, "…"))
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, width, "…")
	}
	return style.Render(strings.Join(lines, "\n"))
}

func sideEmpty(s string) string {
	return lipgloss.NewStyle().Foreground(theme.Muted).Italic(true).Render("  " + s)
}

// sidebarPerms shows the cursored permset/PSG/profile's metadata on
// the /perms top tab. Keeps the body tight — overview-shape, the full
// drill-in view already has a dedicated overview subtab.

// sidebarPermParent shows the current drilled-in parent's identity
// on every TabPermParentDetail subtab. Per-subtab sidebars (e.g. the
// cursored field on the Fields subtab) land in later phases.

// actionRow is the sidebar-facing flattening of any per-entity
// action (field, object, validation-rule, …). The action registries
// live in their own files; they adapt to this shape when rendering
// the shared sidebar menu so the row-level layout stays in one place.
type actionRow struct {
	Label   string
	Hint    string // the "what this does" line shown when allowed
	Allowed bool   // false → row is dimmed, Reason is shown instead
	Reason  string // why it's disabled (e.g. "custom objects only")
	// Separator marks the row as a visual divider rather than a real
	// action. Rendered as a thin rule and skipped by cursor movement.
	// Label / Hint / Allowed / Reason are ignored when set.
	Separator bool
}

// rowContext is the per-cursored-row info the detail sidebars now
// render instead of mirroring the main-pane action list. It answers
// "what is this row and what happens if I act on it" — full help,
// current value, how an edit ships, and the consequence — so the
// sidebar complements the main pane rather than duplicating its
// cursor. Surfaces populate it from their cursored row; an empty
// Heading means "cursor on a non-actionable row" (ReadOnlyMsg shows).
type rowContext struct {
	Heading     string   // e.g. "Toggle Allow Reports", "Edit label"
	Current     string   // current value, rendered after "now:"
	Help        string   // the action's full one-line hint
	Routing     string   // how a write ships (e.g. "Metadata API · CustomObject deploy")
	Affects     string   // consequence of the edit/toggle/delete
	Blocked     string   // disabled reason, when the action is gated
	Danger      bool     // destructive — render heading red
	ReadOnlyMsg string   // shown when Heading == "" (cursor on a read-only row)
	Hints       []string // bottom hint-bar items (e.g. "↵ open", "o Lightning")
}

// sidebarRowContext renders the context panel for the cursored row.
// title is the panel header (e.g. "OBJECT · CONTEXT").
//
// Layout: a title, then the context body (heading + now/help/ships/
// affects), then a reserved full-width hint bar (the org target + any
// nav hints). When the body is taller than the panel, it reflows into
// up to three columns so it fits without clipping; the hint bar always
// stays full-width below the columns, wrapping to extra lines if the
// panel is too narrow for one line.
func (m Model) sidebarRowContext(title string, inner int, ctx rowContext) string {
	// --- body as logical items (each wraps within its column) ---
	var items []ctxItem
	if ctx.Heading == "" {
		msg := ctx.ReadOnlyMsg
		if msg == "" {
			msg = "read-only row — nothing to edit here."
		}
		items = append(items, ctxItem{text: msg, style: ctxDim})
	} else {
		hs := ctxHead
		if ctx.Danger {
			hs = ctxDanger
		}
		items = append(items, ctxItem{text: ctx.Heading, style: hs})
		if ctx.Current != "" {
			items = append(items, ctxItem{label: "now ", text: ctx.Current, style: ctxKV})
		}
		if ctx.Help != "" {
			items = append(items, ctxItem{text: ctx.Help, style: ctxDim})
		}
		if ctx.Routing != "" {
			items = append(items, ctxItem{label: "ships: ", text: ctx.Routing, style: ctxDim})
		}
		if ctx.Affects != "" {
			items = append(items, ctxItem{label: "affects: ", text: ctx.Affects, style: ctxDim})
		}
		if ctx.Blocked != "" {
			items = append(items, ctxItem{label: "blocked: ", text: ctx.Blocked, style: ctxRed})
		}
	}

	// --- hint bar (always full-width, reserved at the bottom) ---
	var hintBar []string
	if o, ok := m.currentOrg(); ok {
		lvl := m.safetyFor(o)
		hintBar = append(hintBar, sideDim("  target: "+o.Display()+" ("+lvl.String()+")", inner))
	}
	if len(ctx.Hints) > 0 {
		hintBar = append(hintBar, sideHintBar(ctx.Hints, inner)...)
	}

	title0 := []string{sideTitle(title), sideSeparator(inner)}
	avail := m.sidebarInnerH
	// Reserve room for title + a separator above the hint bar + the
	// hint bar itself when deciding whether the body must reflow.
	reserved := len(title0) + len(hintBar)
	if len(hintBar) > 0 {
		reserved++ // separator line above the hint bar
	}
	bodyBudget := avail - reserved

	// Reflow the body into columns when a single column would overflow
	// the height — buys vertical room. The generic truncation guard in
	// renderSidebar stamps the warning if anything still clips, so no
	// per-panel warning logic is needed here.
	body, _ := layoutContextItems(items, inner, bodyBudget)

	out := append([]string(nil), title0...)
	out = append(out, body...)
	if len(hintBar) > 0 {
		out = append(out, sideSeparator(inner))
		out = append(out, hintBar...)
	}
	return strings.Join(out, "\n")
}

// ctxItem is one logical chunk of the context body. label is an
// optional inline prefix ("ships: ", "now "); text is the value/prose
// that word-wraps to the available column width. style picks the
// foreground treatment.
type ctxItem struct {
	label string
	text  string
	style ctxStyle
}

type ctxStyle int

const (
	ctxDim    ctxStyle = iota // dim prose (help / affects)
	ctxHead                   // bold heading
	ctxDanger                 // bold red heading
	ctxKV                     // muted label + fg value
	ctxRed                    // red prose (blocked)
)

// renderCtxItem word-wraps one item to width w, returning its lines.
// The first line carries the label; continuation lines indent under
// it. Wrapping is rune/word-safe so em-dashes + bullets survive — the
// whole point of columns is to gain vertical room, so items wrap
// within their column rather than truncate.
func renderCtxItem(it ctxItem, w int) []string {
	switch it.style {
	case ctxHead:
		return wrapStyled(lipgloss.NewStyle().Foreground(theme.Fg).Bold(true), "", it.text, w)
	case ctxDanger:
		return wrapStyled(lipgloss.NewStyle().Foreground(theme.Red).Bold(true), "", it.text, w)
	case ctxKV:
		// label muted, value fg — render as one wrapped block; keep it
		// simple by styling the whole line muted-then-value via prefix.
		lines := wrapPlain(it.label, it.text, w)
		st := lipgloss.NewStyle().Foreground(theme.Fg)
		out := make([]string, len(lines))
		for i, ln := range lines {
			out[i] = st.Render(ln)
		}
		return out
	case ctxRed:
		return wrapStyled(lipgloss.NewStyle().Foreground(theme.Red), it.label, it.text, w)
	default:
		return wrapStyled(lipgloss.NewStyle().Foreground(theme.FgDim), it.label, it.text, w)
	}
}

// wrapStyled word-wraps label+text to width w and applies style to
// every line. Continuation lines indent to align under the label.
func wrapStyled(style lipgloss.Style, label, text string, w int) []string {
	lines := wrapPlain(label, text, w)
	out := make([]string, len(lines))
	for i, ln := range lines {
		out[i] = style.Render(ln)
	}
	return out
}

// wrapPlain word-wraps "label+text" to width w (unstyled). The first
// line is "label"+first words; continuation lines indent by the label
// width so the text block aligns. Returns at least one line.
func wrapPlain(label, text string, w int) []string {
	if w < 4 {
		w = 4
	}
	indent := strings.Repeat(" ", ansi.StringWidth(label))
	avail := w - ansi.StringWidth(label)
	if avail < 3 {
		// label itself eats the width — just truncate the combined.
		return []string{ansi.Truncate(label+text, w, "…")}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{label}
	}
	var lines []string
	cur := ""
	for _, word := range words {
		cand := word
		if cur != "" {
			cand = cur + " " + word
		}
		if ansi.StringWidth(cand) > avail && cur != "" {
			lines = append(lines, cur)
			cur = word
			continue
		}
		cur = cand
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	out := make([]string, len(lines))
	for i, ln := range lines {
		if i == 0 {
			out[i] = label + ln
		} else {
			out[i] = indent + ln
		}
	}
	return out
}

// layoutContextItems renders the body items into as many lines as
// needed, reflowing into 2 then 3 columns when a single column would
// overflow budget. Each item word-wraps WITHIN its column (no
// truncation) — columns buy vertical room by using the panel width.
// A 2-space gutter prefixes every line so the body indents under the
// title like the rest of the sidebar.
// reflowed reports HOW the body was laid out, so the caller can decide
// whether to show the "see full info" warning. Anything other than a
// clean single column means the user is reading a denser, narrower
// rendering — worth offering the modal.
func layoutContextItems(items []ctxItem, inner, budget int) (lines []string, reflowed bool) {
	// Single column first.
	single := renderItemColumn(items, inner-2)
	single = indentLines(single, "  ")
	if budget <= 0 || len(single) <= budget {
		return single, false
	}
	// Try 2 then 3 columns; pick the first that fits the budget.
	for cols := 2; cols <= 3; cols++ {
		laid, ok := renderItemColumns(items, cols, inner, budget)
		if ok {
			return laid, true
		}
	}
	// Nothing fits even at 3 columns (too few items to split, panel too
	// narrow, or genuinely too much content). Prefer the densest column
	// layout that actually produced output; otherwise fall back to the
	// single column. The caller clips + shows the truncation warning.
	for cols := 3; cols >= 2; cols-- {
		if laid, _ := renderItemColumns(items, cols, inner, 1<<30); len(laid) > 0 {
			return laid, true
		}
	}
	return single, true
}

// renderItemColumn renders items top-to-bottom into a flat []string at
// content width w (each item wraps, blank line between items).
func renderItemColumn(items []ctxItem, w int) []string {
	var out []string
	for i, it := range items {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, renderCtxItem(it, w)...)
	}
	return out
}

// renderItemColumns distributes items across cols balanced columns,
// each wrapping within its width, then joins them side-by-side. Returns
// (lines, true) when the result fits budget rows. Items are kept whole
// within a column (never split across columns), assigned greedily to
// balance height.
func renderItemColumns(items []ctxItem, cols, inner, budget int) ([]string, bool) {
	if cols < 2 || len(items) < cols {
		return nil, false
	}
	gutter := 2
	colW := (inner - 2 - gutter*(cols-1)) / cols
	if colW < 10 {
		return nil, false // too narrow to wrap meaningfully
	}
	// Pre-render each item to its wrapped block at colW.
	blocks := make([][]string, len(items))
	for i, it := range items {
		blocks[i] = renderCtxItem(it, colW)
	}
	// Greedy balanced assignment: walk items, push onto the shortest
	// column so far (respecting order within a column).
	colLines := make([][]string, cols)
	colHeight := make([]int, cols)
	target := 0
	for _, b := range blocks {
		target += len(b) + 1
	}
	target = (target + cols - 1) / cols
	c := 0
	for _, b := range blocks {
		if c < cols-1 && colHeight[c] > 0 && colHeight[c]+len(b) > target {
			c++
		}
		if len(colLines[c]) > 0 {
			colLines[c] = append(colLines[c], "")
			colHeight[c]++
		}
		colLines[c] = append(colLines[c], b...)
		colHeight[c] += len(b)
	}
	// Pad columns to equal height + width, stitch row-by-row.
	maxH := 0
	for _, cl := range colLines {
		if len(cl) > maxH {
			maxH = len(cl)
		}
	}
	if maxH > budget {
		return nil, false
	}
	out := make([]string, maxH)
	gap := strings.Repeat(" ", gutter)
	for r := 0; r < maxH; r++ {
		parts := make([]string, cols)
		for ci := 0; ci < cols; ci++ {
			cell := ""
			if r < len(colLines[ci]) {
				cell = colLines[ci][r]
			}
			parts[ci] = padRight(cell, colW)
		}
		out[r] = "  " + strings.Join(parts, gap)
	}
	return out, true
}

// indentLines prefixes every line with pad.
func indentLines(lines []string, pad string) []string {
	out := make([]string, len(lines))
	for i, ln := range lines {
		out[i] = pad + ln
	}
	return out
}

// detailNavHints is the standard hint-bar item set for the drill-
// detail context panels: back, refresh, and (when lightning) the
// Lightning open. Kept here so all the detail surfaces read the same.
func detailNavHints(lightning bool) []string {
	hints := []string{
		firstPretty(Keys.Back) + " back",
		firstPretty(Keys.Refresh) + " refresh",
	}
	if lightning {
		hints = append(hints, firstPretty(Keys.OpenDefault)+" Lightning")
	}
	return hints
}

// sideHintBar lays the hint items onto as few full-width lines as
// possible, separated by " · ", wrapping to the next line when the
// next item wouldn't fit. Always full-width (never columned) so the
// nav/launcher hints read as one bar.
func sideHintBar(items []string, inner int) []string {
	style := lipgloss.NewStyle().Foreground(theme.FgDim)
	var lines []string
	cur := ""
	for _, it := range items {
		cand := it
		if cur != "" {
			cand = cur + " · " + it
		}
		if ansi.StringWidth("  "+cand) > inner && cur != "" {
			lines = append(lines, style.Render("  "+cur))
			cur = it
			continue
		}
		cur = cand
	}
	if cur != "" {
		lines = append(lines, style.Render("  "+cur))
	}
	return lines
}

// inspectModalForCurrentView builds the full-info modal for the active
// RHS panel — GENERIC: it re-renders whatever sidebar the current view
// shows, at the modal's full width and with no height clip, so every
// panel (not just the detail/context ones) can be inspected in full.
// Returns (_, false) only when there is no sidebar at all.
func (m Model) inspectModalForCurrentView() (infoModalState, bool) {
	full, ok := m.fullSidebarContent()
	if !ok {
		return infoModalState{}, false
	}
	return infoModalState{Title: m.inspectModalTitle(), PreRendered: full}, true
}

// fullSidebarContent re-renders the active sidebar at a wide width with
// the column-reflow / height-clip pressure removed, yielding the full
// untruncated panel. Returns (_, false) when the view has no sidebar.
func (m Model) fullSidebarContent() (string, bool) {
	// Wide inner width so values don't truncate horizontally; huge
	// sidebarInnerH so the context panels stay single-column (no reflow)
	// and nothing self-clips. modalWidth caps the eventual box.
	w := modalWidth(m.width, 44, 80)
	inner := w - 4
	if inner < 20 {
		inner = 20
	}
	mm := m
	mm.sidebarInnerH = 1 << 20 // effectively unbounded — disables reflow
	mm.sidebarForModal = true  // full roomy layout — never compact pills inline
	content := stripTruncSentinel(mm.resolveSidebar(inner))
	if strings.TrimSpace(content) == "" {
		return "", false
	}
	return content, true
}

// inspectModalTitle is the heading for the generic inspect modal —
// names the entity under inspection when we can identify it.
func (m Model) inspectModalTitle() string {
	subj := m.inspectSubject()
	if subj == "" {
		return "Panel · full info"
	}
	return subj + " · full info"
}

// inspectSubject is the short identity label for the inspect modal
// title (sObject / field / drilled entity / user).
func (m Model) inspectSubject() string {
	o, ok := m.currentOrg()
	if !ok {
		return ""
	}
	d := m.data[o.Username]
	if d == nil {
		return ""
	}
	switch m.tab() {
	case TabFieldDetail:
		if d.DescribeCur != "" && d.FieldCur != "" {
			return d.DescribeCur + "." + d.FieldCur
		}
	case TabUserDetail:
		if d.UserCur != "" {
			return m.cursoredUserRow(d, d.UserCur).Username
		}
	case TabObjectDetail:
		if m.currentSubtab() == SubtabSchema && d.FieldCur != "" {
			return d.DescribeCur + "." + d.FieldCur
		}
	}
	return d.DescribeCur
}

// sidebarFieldActions is the TabFieldDetail right sidebar — a context
// panel for the cursored field-detail row.

// fieldRowContext builds the context panel for the cursored field
// row. The field actions write via the Tooling CustomField API (not
// the Metadata deploy that object edits use).

// Current value from the cursored field.

// fieldActionCurrentValue returns the "now:" value for a field action.

// sidebarObjectActions is the Details-subtab right sidebar. It is now
// INFO-ONLY: the action menu lives in the main pane (arrow keys walk
// the editable rows; Enter / ctrl+e fires them). This sidebar just
// mirrors the catalog of available actions and highlights whichever
// one the cursored main-pane row maps to, so the user can safely hide
// it without losing the ability to act.

// Details subtab — surface the subtab nav + Lightning open.

// objectRowContext builds the context panel for the cursored Details
// row. Read-only rows explain themselves; editable rows carry the
// deploy routing + consequence so the user knows what a write ships.

// Current value for the cursored row, pulled from describe/baseline.

// objectActionCurrentValue returns the current value string for an
// object action index, used as the "now:" line in the context panel.

// sidebarRecordType renders a compact summary of the currently-
// selected record type on the Record Types subtab. Drill into a
// record type (enter) to open TabRecordTypeDetail for the full
// Metadata + action menu.

// sidebarTrigger renders a compact summary of the currently-selected
// trigger on the Triggers subtab. Drill (enter) to open
// TabTriggerDetail for the body + action menu.

// sidebarValidationRule renders a compact summary of the currently-
// selected rule on the Validation subtab. Drill into a rule (enter)
// to open TabValidationDetail for the full formula body + action
// menu.

// wrap word-wraps s to width, joining continuation lines with a
// two-space hanging indent (callers prefix the first line with "  "
// themselves). Rune-safe — the previous version byte-sliced, which
// split multi-byte characters mid-rune — and word-aware, breaking
// only inside words longer than a whole line. Embedded newlines are
// preserved as paragraph breaks.
func wrap(s string, width int) string {
	if width <= 0 || len(s) == 0 {
		return s
	}
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			switch {
			case line == "":
				line = word
			case lipgloss.Width(line)+1+lipgloss.Width(word) <= width:
				line += " " + word
			default:
				lines = append(lines, line)
				line = word
			}
			// Hard-break words longer than a full line (URLs, ids).
			for lipgloss.Width(line) > width {
				r := []rune(line)
				if len(r) <= width {
					break
				}
				lines = append(lines, string(r[:width]))
				line = string(r[width:])
			}
		}
		lines = append(lines, line)
	}
	// Trim a trailing empty produced by an empty final paragraph.
	for len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n  ")
}

// Focused-item details. When a row is selected on the Items
// subtab the sidebar surfaces the canonical id (where one
// exists), the kind label, and any captured context. Kinds
// without an Id (sObject, field) just show what they DO carry —
// the ref still answers "what is this?" cleanly.

// devProjectKindHasID reports whether the given kind's Ref slot
// holds a true Salesforce Id (or local sf-deck id) versus a
// compound / api-name-only reference. Drives whether the sidebar
// labels the value as "id" (canonical) or "ref" (composite).
