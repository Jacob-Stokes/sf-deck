package ui

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

	const edgeGutter = 1
	innerWidth := m.width - 2*edgeGutter
	widgetW := 0
	if m.leftOpen {
		widgetW = clamp(innerWidth/5, 24, 34)
	}
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

	bodyTotalH := m.height - lipgloss.Height(header) - tabsHeight - lipgloss.Height(status)
	paneH := bodyTotalH
	if paneH < 5 {
		paneH = 5
	}
	innerH := paneH - 2
	if innerH < 3 {
		innerH = 3
	}
	m.sidebarInnerH = innerH
	trace.setLayout(widgetTotal, mainTotal, sideTotal, paneH, innerH, m.leftOpen, m.sidebarOpen)

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
	mainPaneH := paneH
	stackedSideH := 0
	if m.sidebarStacked && m.sidebarOpen {
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

	phaseStart = time.Now()
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
