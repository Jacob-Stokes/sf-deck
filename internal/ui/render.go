package ui

// Top-level frame layout.
//
// View() is the entry point Bubble Tea calls every frame. It returns
// a tea.View carrying the rendered string + altscreen / cursor flags
// the runtime reads.
//
// Layout:
//
//   renderHeader()       — top chrome bar (render_header.go)
//   renderTabBar()       — tab row (render_tabs.go)
//   body (3 panes)       — left rail + main + sidebar
//   renderStatusBar()    — bottom chrome bar (render_status.go)
//
// Modal overlays (open menu, edit modal, choice modal, global search,
// info modal) used to take over the whole frame on a blank
// background. We now compose them as positioned layers over the base
// view using lipgloss v2's NewLayer / NewCompositor — so the modal
// floats over the real UI instead of hiding it.

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
)

// View is Bubble Tea's per-frame entry point. We wrap the real
// renderer (viewImpl) in a deferred recover so a panic anywhere in
// the render tree writes a stack trace to the session log + a
// dedicated panic.log instead of taking down the TTY with no
// diagnostic. The fallback frame keeps the alt-screen alive long
// enough for the user to read the message and quit cleanly.
func (m Model) View() (out tea.View) {
	defer func() {
		if r := recover(); r != nil {
			out = renderPanicFrame(r)
		}
	}()
	// Publish the latest snapshot to any IPC subscribers. Cheap
	// (single-map copy + RWMutex) and runs once per render — keeps
	// the control channel honest without the publisher needing to
	// know which messages changed which fields.
	m.PublishControlSnapshot()
	return m.viewImpl()
}

// renderPanicFrame is the safe fallback we render after a panic in
// the View pipeline. Logs the panic + stack to applog AND writes a
// standalone panic.log next to the session log so the trace
// survives even if the TTY ate the rest. Returns a minimal frame
// that says "render panicked" + the first line of the recovered
// value so the user knows to quit + check the log.
func renderPanicFrame(r any) tea.View {
	stack := string(debug.Stack())
	msg := fmt.Sprintf("%v", r)
	applog.Error("render.panic", map[string]any{
		"recovered": msg,
		"stack":     stack,
	})
	body := "render panicked — see ~/.sf-deck/log for stack trace.\n" +
		"recovered: " + msg + "\n\nq to quit"
	v := tea.NewView(body)
	v.AltScreen = true
	return v
}

func (m Model) viewImpl() tea.View {
	trace := m.beginRenderTrace()
	if trace != nil {
		defer func() { trace.finish(m) }()
	}
	if m.width == 0 {
		trace.setPath("starting")
		m.stashCompositor(lipgloss.NewCompositor())
		v := tea.NewView("starting…")
		trace.setOutput("starting…")
		return v
	}
	if v, ok := m.cachedFrameView(); ok {
		trace.setPath("cached")
		trace.markCached()
		if m.renderCache != nil {
			trace.setOutput(m.renderCache.lastFrame)
		}
		return v
	}

	// Zen mode: list-table surfaces remember their own zen flag (so
	// switching tabs preserves whether each list was zen'd); other
	// surfaces fall back to a global m.zenMode toggle. Either flips
	// the entire layout to a single pane — no header, no tab bars, no
	// sidebar, no left rail, no status bar. Press z again (or Esc)
	// to return.
	zenActive := m.zenMode
	phaseStart := time.Now()
	if state := (&m).activeListTableState(); state != nil && state.Zen {
		zenActive = true
	}
	trace.phase("zen_check", phaseStart)
	// Modals must layer over zen — the open menu, edit modal,
	// command palette, etc. are useless if they're invisible. When
	// a modal/overlay is active, fall through to the regular render
	// path which composes overlays on top of the body. The zen
	// short-circuit only fires when nothing's overlaid, so the
	// chrome (header / tabs / status / sidebar) is still hidden in
	// the common case but the user can still hit `o`, `?`, etc.
	if zenActive && !m.anyModalActive() {
		trace.markZen()
		trace.setPath("zen_direct")
		phaseStart = time.Now()
		body := m.renderMain(m.width-2, m.height-2, m.height-4)
		trace.phase("main", phaseStart)
		phaseStart = time.Now()
		m.stashCompositor(lipgloss.NewCompositor(
			lipgloss.NewLayer(body).ID("main"),
		))
		trace.phase("compositor_setup", phaseStart)
		m.rememberFrame(body)
		v := tea.NewView(body)
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		trace.setOutput(body)
		return v
	}

	phaseStart = time.Now()
	header := m.renderHeader()
	trace.phase("header", phaseStart)
	phaseStart = time.Now()
	status := m.cachedStatusBar()
	trace.phase("status", phaseStart)

	// Left rail: single widget pane hosting utilities (Orgs,
	// Bookmarks, …). Toggled by `|`. Right rail: context sidebar,
	// toggled by `\`.
	//
	// Pane sizing: each pane renderer takes a width and produces a
	// string EXACTLY that wide (border included — lipgloss v2's
	// Width counts the border in the total). The three rendered
	// panes plus a symmetric 1-cell gutter on each terminal edge
	// sum to m.width: leftGutter + widgetW + mainW + sideW +
	// rightGutter == m.width. The gutter just gives the rounded
	// borders breathing room from the terminal edge so the layout
	// doesn't look like it's bleeding off the screen.
	const edgeGutter = 1
	innerWidth := m.width - 2*edgeGutter
	widgetW := 0
	if m.leftOpen {
		widgetW = clamp(innerWidth/5, 24, 34)
	}
	// Sidebar width is 0 when either closed OR stacked-below-main —
	// in stacked mode the sidebar shares the main column's
	// horizontal real-estate.
	sideW := 0
	if m.sidebarOpen && !m.sidebarStacked {
		sideW = clamp(innerWidth/4, 28, 48)
	}
	mainW := innerWidth - widgetW - sideW
	if mainW < 20 {
		mainW = 20
	}
	// Old aliases kept around because trace.setLayout + the rest of
	// the pipeline (paneH math, hit-layer width) refer to "totals"
	// for legacy reasons. Totals == rendered widths now that
	// padding lives inside each pane's Width call.
	widgetTotal := widgetW
	sideTotal := sideW
	mainTotal := mainW

	// Two tab bars sit on the same row, each scoped to its surface.
	// Left bar = "0 Orgs" pill above the rail; main bar spans main +
	// sidebar. When the left rail is hidden the whole row is just
	// the main bar. The width comes from mainTabBarWidth() — the
	// SINGLE source visiblePinnedTabs shares, so the More… modal's
	// idea of "which pinned tabs fit" can never drift from what this
	// render actually shows. (mainTotal+sideTotal equals it by
	// construction; asserting via the helper keeps it that way.)
	phaseStart = time.Now()
	mainTabBar, mainTabLayers := m.cachedTabBar(m.mainTabBarWidth())
	var leftTabBar string
	var leftTabLayers []*lipgloss.Layer
	if m.leftOpen {
		leftTabBar, leftTabLayers = m.cachedLeftTabBar(widgetTotal)
	}
	trace.phase("tab_bars", phaseStart)
	tabsHeight := lipgloss.Height(mainTabBar)
	if lh := lipgloss.Height(leftTabBar); lh > tabsHeight {
		tabsHeight = lh
	}

	// Lipgloss v2: Style.Width/Height include the border in the block
	// size — Height(h) renders exactly h rows total (content + border).
	// So paneH is the FULL slot height. innerH (what renderers get to
	// fill) is paneH minus the 2 border rows.
	bodyTotalH := m.height - lipgloss.Height(header) - tabsHeight - lipgloss.Height(status)
	paneH := bodyTotalH
	if paneH < 5 {
		paneH = 5
	}
	innerH := paneH - 2
	if innerH < 3 {
		innerH = 3
	}
	// Beside-the-main sidebar gets the full pane height; the stacked
	// branch below overrides this with its shorter slot. Sidebar
	// renderers read m.sidebarInnerH for column-reflow decisions.
	m.sidebarInnerH = innerH
	trace.setLayout(widgetTotal, mainTotal, sideTotal, paneH, innerH, m.leftOpen, m.sidebarOpen)

	// Each pane renders at exactly its rendered width (border
	// included). The compositor places the next pane at the running
	// X cursor; bodyX starts at edgeGutter so a symmetric gutter
	// flanks the layout on both terminal edges. The string-concat
	// path (bodyParts → bodyRow) prepends a left-pad string of the
	// same width so the two render paths produce identical output.
	// gutter is a paneH-tall column of edgeGutter spaces — the
	// joinRenderedColumns path concatenates row-by-row, so it
	// needs the gutter at the same line count as the panes for
	// every row to stay aligned.
	gutter := buildGutterColumn(edgeGutter, paneH)
	var bodyLayers []*lipgloss.Layer
	var bodyParts []string
	if edgeGutter > 0 {
		bodyParts = append(bodyParts, gutter)
	}
	bodyX := edgeGutter
	if m.leftOpen {
		phaseStart = time.Now()
		leftWidget := m.renderLeftWidget(widgetW, paneH, innerH)
		trace.phase("left_widget", phaseStart)
		bodyParts = append(bodyParts, leftWidget)
		bodyLayers = append(bodyLayers,
			lipgloss.NewLayer(leftWidget).X(bodyX).Y(0).ID("left-rail"),
		)
		bodyX += widgetW
	}
	// Stacked mode: sidebar sits BELOW the main pane instead of beside
	// it. Main gets 2/3 of the body height, sidebar gets the
	// remainder. Both panes use mainW (the full width-after-rail)
	// since the sidebar's old horizontal slot was returned to mainW
	// when sideW was zeroed above.
	mainPaneH := paneH
	stackedSideH := 0
	if m.sidebarStacked && m.sidebarOpen {
		// 2/3 main, 1/3 sidebar. Floor at 5 each so neither pane
		// collapses on a short terminal.
		stackedSideH = paneH / 3
		if stackedSideH < 5 {
			stackedSideH = 5
		}
		mainPaneH = paneH - stackedSideH
		if mainPaneH < 5 {
			mainPaneH = 5
			stackedSideH = paneH - mainPaneH
		}
	}
	mainInnerH := mainPaneH - 2
	if mainInnerH < 3 {
		mainInnerH = 3
	}
	phaseStart = time.Now()
	mainStr := m.renderMain(mainW, mainPaneH, mainInnerH)
	trace.phase("main", phaseStart)
	mainLayer := lipgloss.NewLayer(mainStr).X(bodyX).Y(0).ID("main")
	phaseStart = time.Now()
	mainLayer.AddLayers(m.renderMainHitLayers(mainW)...)
	trace.phase("hit_layers", phaseStart)
	bodyLayers = append(bodyLayers, mainLayer)

	switch {
	case m.sidebarStacked && m.sidebarOpen:
		// Stacked: build a column of [mainStr \n sidebar] occupying
		// mainW wide × paneH tall. bodyParts stays row-aligned by
		// joining vertically into a single column string.
		stackedInnerH := stackedSideH - 2
		if stackedInnerH < 3 {
			stackedInnerH = 3
		}
		m.sidebarInnerH = stackedInnerH
		phaseStart = time.Now()
		sidebar := m.renderSidebar(mainW, stackedSideH, stackedInnerH)
		trace.phase("sidebar", phaseStart)
		stack := lipgloss.JoinVertical(lipgloss.Left, mainStr, sidebar)
		bodyParts = append(bodyParts, stack)
		stackedSidebarLayer := lipgloss.NewLayer(sidebar).X(bodyX).Y(mainPaneH).ID("sidebar")
		stackedSidebarLayer.AddLayers(sidebarFooterHitLayers(mainW, stackedSideH)...)
		bodyLayers = append(bodyLayers, stackedSidebarLayer)
		bodyX += mainW
	default:
		bodyParts = append(bodyParts, mainStr)
		bodyX += mainW
		if m.sidebarOpen {
			phaseStart = time.Now()
			sidebar := m.renderSidebar(sideW, paneH, innerH)
			trace.phase("sidebar", phaseStart)
			bodyParts = append(bodyParts, sidebar)
			sidebarLayer := lipgloss.NewLayer(sidebar).X(bodyX).Y(0).ID("sidebar")
			sidebarLayer.AddLayers(sidebarFooterHitLayers(sideW, paneH)...)
			bodyLayers = append(bodyLayers, sidebarLayer)
		}
	}
	// Right gutter (string-concat path only — compositor stops
	// drawing past the rightmost layer X coord, which is fine).
	if edgeGutter > 0 {
		bodyParts = append(bodyParts, gutter)
	}

	var tabRow string
	var tabLayers []*lipgloss.Layer
	phaseStart = time.Now()
	if m.leftOpen {
		tabRow = lipgloss.JoinHorizontal(lipgloss.Top, leftTabBar, mainTabBar)
		tabLayers = append(tabLayers, leftTabLayers...)
		mainOffset := lipgloss.Width(leftTabBar)
		for _, layer := range mainTabLayers {
			layer.X(layer.GetX() + mainOffset)
			tabLayers = append(tabLayers, layer)
		}
	} else {
		tabRow = mainTabBar
		tabLayers = append(tabLayers, mainTabLayers...)
	}
	// Align the tab row's LEFT edge with the body panel below it. The
	// panel sits at column edgeGutter (1); the tab row is rendered at
	// innerWidth (m.width - 2*edgeGutter). Previously padTabRowToWidth
	// right-aligned it to the full m.width, prepending 2 spaces → the
	// tabs started at column 2 while the panel starts at column 1 (a
	// visible 1-column misalignment). Instead: prepend exactly edgeGutter
	// on the left, then pad the remainder on the right so the row still
	// fills m.width. Tab click hit-layers shift by edgeGutter to match.
	if edgeGutter > 0 {
		tabRow = indentTabRow(tabRow, edgeGutter)
	}
	tabRow = rightPadTabRowToWidth(tabRow, m.width)
	for _, layer := range tabLayers {
		layer.X(layer.GetX() + edgeGutter)
	}
	trace.phase("tab_row_join", phaseStart)

	phaseStart = time.Now()
	y := 0
	headerL := lipgloss.NewLayer(header).Y(y).ID("header")
	y += lipgloss.Height(header)
	tabRowL := lipgloss.NewLayer(tabRow).Y(y).ID("tab-bar").AddLayers(tabLayers...)
	y += tabsHeight
	bodyL := lipgloss.NewLayer("").Y(y).AddLayers(bodyLayers...)
	y += paneH
	statusL := lipgloss.NewLayer(status).Y(y).ID("status-bar")

	comp := lipgloss.NewCompositor(headerL, tabRowL, bodyL, statusL)
	trace.phase("compositor_setup", phaseStart)
	phaseStart = time.Now()
	bodyRow := joinRenderedColumns(bodyParts...)
	baseRendered := joinFrameBlocks(header, tabRow, bodyRow, status)
	trace.phase("base_join", phaseStart)

	// Theme picker is special-cased: top-right anchored, doesn't dim
	// the background (so live preview is fully visible). Wins over
	// other overlays since opening it explicitly closes them first.
	phaseStart = time.Now()
	if picker := m.renderThemePicker(); picker != "" {
		trace.phase("theme_picker", phaseStart)
		x := m.width - lipgloss.Width(picker) - 1
		if x < 0 {
			x = 0
		}
		comp.AddLayers(lipgloss.NewLayer(picker).X(x).Y(1).Z(30).ID("theme-picker"))
		m.stashCompositor(comp)
		phaseStart = time.Now()
		rendered := comp.Render()
		trace.phase("final_render", phaseStart)
		m.rememberFrame(rendered)
		v := tea.NewView(rendered)
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		trace.setPath("theme_picker_compositor")
		trace.setOverlay(false, true)
		trace.setOutput(rendered)
		return v
	}
	trace.phase("theme_picker", phaseStart)

	// Modal overlays float over the base view. We layer them with the
	// v2 compositor so the underlying UI stays visible (but dimmed via
	// ANSI codes in the modal styling) instead of being replaced by a
	// blank background. First non-empty overlay wins — interactive
	// modals take precedence over info-only ones.
	phaseStart = time.Now()
	// The chip wizard contributes per-row click layers alongside its
	// overlay string; attach them only when it's the winning overlay.
	wizStr, wizLayers := m.renderChipWizardLayers()
	overlays := []string{
		// Palette renders ABOVE every other modal so the universal
		// "go anywhere" surface is never layered behind something.
		m.renderCommandPalette(),
		m.renderKeybindingsModal(),
		m.renderOpenMenu(),
		m.renderEditModal(),
		m.renderSOQLModal(),
		m.renderOrgPicker(),
		m.renderDeepCollect(),
		m.renderChoiceModal(),
		m.renderOrgManageModal(),
		m.renderTagPicker(),
		m.renderTagEditor(),
		m.renderCacheSettings(),
		m.renderCompareEditModal(),
		m.renderCompareScopeModal(),
		wizStr,
		m.renderGlobalSearch(),
		m.renderDownloadsModal(),
		m.renderExportSaveModal(),
		m.renderInfoModal(),
	}
	trace.phase("overlay_build", phaseStart)
	hasOverlay := false
	phaseStart = time.Now()
	for _, overlay := range overlays {
		if overlay == "" {
			continue
		}
		x := (m.width - lipgloss.Width(overlay)) / 2
		if x < 0 {
			x = 0
		}
		y := (m.height - lipgloss.Height(overlay)) / 2
		if y < 0 {
			y = 0
		}
		modalLayer := lipgloss.NewLayer(overlay).X(x).Y(y).Z(20).ID("modal")
		if overlay == wizStr && len(wizLayers) > 0 {
			modalLayer.AddLayers(wizLayers...)
		}
		comp.AddLayers(modalLayer)
		hasOverlay = true
		break
	}
	trace.phase("overlay_layout", phaseStart)
	// Anchored picker (chip overflow, chip wizard's field-add
	// dropdown, future sObject pickers) layers on top of whatever's
	// underneath — the base view OR a modal overlay. Caller-supplied
	// (anchorX, anchorY) clamped to fit on screen.
	hasPicker := false
	phaseStart = time.Now()
	if pk := m.renderPicker(); pk != "" {
		x, y := pickerOverlayPosition(pk, m.picker.anchorX, m.picker.anchorY, m.width, m.height)
		comp.AddLayers(lipgloss.NewLayer(pk).X(x).Y(y).Z(30).ID("picker"))
		hasPicker = true
	}
	trace.phase("picker", phaseStart)
	// Walkthrough corner panel: a non-dimming top layer that leaves the
	// UI underneath interactive (unlike the modal overlays above, it does
	// NOT set hasOverlay / block input). Bottom-right anchored so it
	// doesn't cover the header/tab bar the tour asks the user to use.
	hasWalkthrough := false
	if wt := m.renderWalkthrough(); wt != "" {
		x := m.width - lipgloss.Width(wt) - 1
		if x < 0 {
			x = 0
		}
		y := m.height - lipgloss.Height(wt) - 1
		if y < 0 {
			y = 0
		}
		comp.AddLayers(lipgloss.NewLayer(wt).X(x).Y(y).Z(25).ID("walkthrough"))
		hasWalkthrough = true
	}
	m.stashCompositor(comp)
	rendered := baseRendered
	if hasOverlay || hasPicker || hasWalkthrough {
		phaseStart = time.Now()
		rendered = comp.Render()
		trace.phase("final_render", phaseStart)
		trace.setPath("overlay_compositor")
	} else {
		trace.setPath("base_direct")
	}
	trace.setOverlay(hasOverlay, hasPicker)
	m.rememberFrame(rendered)
	v := tea.NewView(rendered)
	v.AltScreen = true
	// Cell-motion mouse tracking: emits MouseWheelMsg so we can
	// translate scroll-wheel into smooth list-cursor jumps instead
	// of letting the terminal interpret each tick as an arrow-key
	// event (which queues up and produces the lag-on-stop /
	// lag-on-resume behaviour the user reported).
	v.MouseMode = tea.MouseModeCellMotion
	trace.setOutput(rendered)
	return v
}

func (m Model) stashCompositor(comp *lipgloss.Compositor) {
	if comp == nil || m.lastCompositor == nil {
		return
	}
	*m.lastCompositor = *comp
}

func joinFrameBlocks(header, tabRow, bodyRow, status string) string {
	var b strings.Builder
	b.Grow(len(header) + len(tabRow) + len(bodyRow) + len(status) + 3)
	b.WriteString(header)
	b.WriteByte('\n')
	b.WriteString(tabRow)
	b.WriteByte('\n')
	b.WriteString(bodyRow)
	b.WriteByte('\n')
	b.WriteString(status)
	return b.String()
}

// buildGutterColumn returns a `width`-cell × `height`-row block of
// spaces, lines joined by '\n'. Used as a column-shaped pad in
// joinRenderedColumns so the symmetric edge-gutter on either side
// of the body row is the same height as the panes — short gutters
// would shift only their first row inward and break alignment.
func buildGutterColumn(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	row := strings.Repeat(" ", width)
	var b strings.Builder
	b.Grow((width + 1) * height)
	for i := 0; i < height; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(row)
	}
	return b.String()
}

func joinRenderedColumns(parts ...string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}

	lines := make([][]string, len(parts))
	lineCount := 0
	totalLen := 0
	for i, part := range parts {
		totalLen += len(part)
		lines[i] = strings.Split(part, "\n")
		if len(lines[i]) > lineCount {
			lineCount = len(lines[i])
		}
	}

	var b strings.Builder
	b.Grow(totalLen + lineCount - 1)
	for row := 0; row < lineCount; row++ {
		if row > 0 {
			b.WriteByte('\n')
		}
		for _, partLines := range lines {
			if row < len(partLines) {
				b.WriteString(partLines[row])
			}
		}
	}
	return b.String()
}

func (m Model) renderMainHitLayers(mainW int) []*lipgloss.Layer {
	inner := mainW - 4
	if inner <= 0 {
		return nil
	}
	// Hit-layer set must match what the strip actually shows —
	// pinned subtabs + a More… sentinel slot when overflow exists.
	// Otherwise click resolution is off-by-one for tabs that opt
	// into overflow.
	subs := m.tabSubtabsForStrip()
	if len(subs) <= 1 {
		return nil
	}
	selected := m.currentSubtabIndex(subs)
	_, layers := renderSubtabStripLayers(subs, selected, inner)
	// The subtab strip is ALSO drawn into the main pane body (mainStr),
	// so these layers exist only to carry click-zone IDs on top of it.
	//
	// They must draw content IDENTICAL to what's already beneath —
	// re-using the pill's own rendered content, positioned exactly over
	// where the body drew it. An earlier version blanked them with
	// spaces to avoid a "double strip", but blank cells are opaque:
	// when a modal forces the compositor path (base_direct skips layers
	// entirely, so the bug only showed with a modal up), those spaces
	// PAINTED OVER the real strip and the subtabs vanished. Drawing the
	// same pill content is a no-op visually while keeping the hit box.
	out := make([]*lipgloss.Layer, 0, len(layers))
	for _, layer := range layers {
		nl := lipgloss.NewLayer(layer.GetContent()).
			X(layer.GetX() + 2).
			Y(layer.GetY() + 1).
			Z(layer.GetZ()).
			ID(layer.GetID())
		out = append(out, nl)
	}
	return out
}

func (m Model) currentSubtabIndex(subs []subtabInfo) int {
	if len(subs) == 0 {
		return 0
	}
	cur := m.currentSubtab()
	for i, sub := range subs {
		if sub.ID == cur {
			return i
		}
	}
	// Active subtab not in this slice — likely the strip-shaped
	// subset where the active subtab lives in overflow. Highlight
	// the More… slot so the user can see the strip is reflecting
	// their selection.
	for i, sub := range subs {
		if sub.ID == SubtabMoreSentinelID {
			return i
		}
	}
	return 0
}

func pickerOverlayPosition(picker string, anchorX, anchorY, termW, termH int) (int, int) {
	mw := lipgloss.Width(picker)
	mh := lipgloss.Height(picker)
	x := anchorX
	y := anchorY
	if x+mw > termW {
		x = termW - mw - 1
	}
	if x < 0 {
		x = 0
	}
	if y+mh > termH {
		y = termH - mh - 1
	}
	if y < 0 {
		y = 0
	}
	return x, y
}

// padTabRowToWidth left-pads each line of the tab row with spaces
// until it's exactly width cols wide. Spaces go on the LEFT so the
// right-side nav cluster ends up flush with the screen edge — any
// short measurement (emoji, joined-pill drift) shows up as a tiny
// strip of empty space between the rail's "0 Orgs" pill and the
// numbered tabs, which is invisible against the dark background.
// mainTabBarWidth is the SINGLE source of truth for how wide the main
// tab bar renders: the inner frame width (m.width minus the symmetric
// edge gutters) minus the left rail's slot when it's open. Both the
// frame renderer (cachedTabBar) and visiblePinnedTabs (which decides
// what the More… overflow offers) MUST use this — when they disagreed
// (the modal used raw m.width), a pinned tab at the fit boundary was
// dropped from the strip but excluded from the overflow: unreachable.
// Mirrors the widget math in viewImpl; keep the two in sync.
func (m Model) mainTabBarWidth() int {
	if m.width <= 0 {
		return 0
	}
	const edgeGutter = 1
	innerWidth := m.width - 2*edgeGutter
	widgetW := 0
	if m.leftOpen {
		widgetW = clamp(innerWidth/5, 24, 34)
	}
	w := innerWidth - widgetW
	if w < 0 {
		w = 0
	}
	return w
}

// indentTabRow prepends n spaces to every line of a multi-line block —
// used to inset the tab row by the edge gutter so its left edge lines up
// with the body panel's left border.
func indentTabRow(s string, n int) string {
	if n <= 0 {
		return s
	}
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = pad + lines[i]
	}
	return strings.Join(lines, "\n")
}

// rightPadTabRowToWidth appends spaces to each line so it reaches width
// (leaving already-wide lines untouched). Unlike padTabRowToWidth this
// pads on the RIGHT, so a left-indented tab row keeps its left inset
// while still filling the full frame width.
func rightPadTabRowToWidth(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w < width {
			lines[i] = ln + strings.Repeat(" ", width-w)
		}
	}
	return strings.Join(lines, "\n")
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
