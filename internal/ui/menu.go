package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// requestOpenMenu pops up the overlay or short-circuits when there's a
// single target. Returns the updated model plus any immediately-fired
// command (when we short-circuited past the menu).
func (m Model) requestOpenMenu(mode openMenuMode) (Model, tea.Cmd) {
	o, ok := m.currentOrg()
	if !ok {
		return m, nil
	}
	target := m.cursorOpenable()
	if target == nil {
		verb := "open"
		if mode == menuYank {
			verb = "yank"
		}
		m.flash("nothing to " + verb + " here")
		return m, nil
	}
	var targets []sf.OpenTarget
	if mode == menuYank {
		// Yank menu: value targets (label / API name / Id / contextual)
		// first, then the URL variants folded in, so ctrl+y can copy any
		// of them. Mark each URL target so firing copies the URL.
		if y, ok := target.(sf.Yankable); ok {
			for _, yt := range y.YankTargets() {
				targets = append(targets, sf.OpenTarget{
					ID: yt.ID, Label: yt.Label, Shortcut: yt.Shortcut, YankValue: yt.Value,
				})
			}
		}
		// On a record drill, offer yanking the CURSORED FIELD'S VALUE —
		// the full content (JSON, long text) that's truncated in the main
		// pane. Prepended so it's the first, most-relevant yank option
		// when you're sitting on a field.
		if fv, name, ok := m.cursoredRecordFieldValue(); ok {
			targets = append([]sf.OpenTarget{{
				ID:        "field_value",
				Label:     "Field value · " + name,
				Shortcut:  "v",
				YankValue: fv,
			}}, targets...)
		}
		// On a picklist VALUE row (cursor rested on a value), offer that
		// single value first — the most-relevant yank when you're sitting
		// on a value.
		if pv, ok := m.cursoredFieldDetailYankValue(); ok {
			targets = append([]sf.OpenTarget{{
				ID:        "picklist_value",
				Label:     "This value · " + ansiTrunc(pv, 30),
				Shortcut:  "v",
				YankValue: pv,
			}}, targets...)
		}
		// On a field-detail drill, offer a "Field values…" sub-menu with
		// the field's copyable definition values (picklist formats,
		// formula, default, help, references). Synthetic target — no
		// YankValue; fireMenuTarget opens the sub-modal. Available from
		// any field-detail row (incl. a value row → "full set" from here).
		if fr, ok := target.(sf.FieldRef); ok && len(fieldValueYankOptions(fr.Field)) > 0 {
			targets = append(targets, sf.OpenTarget{
				ID:       fieldValuesYankTargetID,
				Label:    "Field values (whole set)…",
				Shortcut: "f",
			})
		}
		urlTargets := target.Targets()
		// Avoid a shortcut collision between value and URL targets — the
		// URL target keeps its label/path but drops a clashing shortcut.
		used := map[string]bool{}
		for _, t := range targets {
			if t.Shortcut != "" {
				used[t.Shortcut] = true
			}
		}
		for _, t := range urlTargets {
			// Skip in-app action targets (no Path / AbsoluteURL / YankValue)
			// — they drill somewhere, there's nothing to copy. Keeps the
			// yank menu to genuinely yankable entries.
			if t.Path == "" && t.AbsoluteURL == "" && t.YankValue == "" {
				continue
			}
			if used[t.Shortcut] {
				t.Shortcut = ""
			}
			targets = append(targets, t)
		}
	} else {
		targets = target.Targets()
		// Offer "Find in another org…" when the cursored resource has a
		// stable cross-org identity and there's another connected org to
		// search. Open mode only — this is a navigation, not a yank.
		if mv := m.moveOrgOpenTarget(); mv != nil {
			targets = append(targets, *mv)
		}
	}
	if len(targets) == 0 {
		m.flash("no targets")
		return m, nil
	}
	if len(targets) == 1 {
		if mode == menuOpen {
			m.recordRecentVisit(o.Username, target)
		}
		return m.fireMenuTarget(o, targets[0], mode, target)
	}
	title := cursorLabel(target)
	if mode == menuYank {
		title = "Copy · " + title
	} else {
		title = "Open · " + title
	}
	m.openMenu = &openMenuState{
		title: title, mode: mode, org: o, source: target, targets: targets, cursor: 0,
	}
	return m, nil
}

// handleOpenMenuKey is routed to when the overlay is visible.
func (m Model) handleOpenMenuKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.openMenu == nil {
		return m, nil
	}
	key := msg.String()
	switch key {
	case "ctrl+c", "esc":
		// Pop the sub-modal stack first when present — sub-modals
		// (e.g. "Open related <sObject>…") esc back to the parent
		// menu rather than dismissing the entire overlay.
		if n := len(m.openMenuStack); n > 0 {
			parent := m.openMenuStack[n-1]
			m.openMenuStack = m.openMenuStack[:n-1]
			m.openMenu = &parent
			return m, nil
		}
		// Restore global-search modal if this open-menu was
		// launched from there (ctrl+o on a hit). User resumes at
		// the exact input/cursor/scope they left.
		if restored := m.openMenu.restoreGlobalSearch; restored != nil {
			m.openMenu = nil
			m.globalSearch = restored
			return m, nil
		}
		m.openMenu = nil
		return m, nil
	case "j", "down":
		if m.openMenu.cursor < len(m.openMenu.targets)-1 {
			m.openMenu.cursor++
		}
		return m, nil
	case "k", "up":
		if m.openMenu.cursor > 0 {
			m.openMenu.cursor--
		}
		return m, nil
	case "enter":
		return m.fireSelectedMenuTarget(m.openMenu.cursor)
	case "shift+enter":
		// On the browser sub-picker, shift+enter opens the highlighted
		// browser in a private / incognito window (falls back to a
		// normal open for browsers with no CLI private mode). Plain
		// enter elsewhere is unaffected.
		if m.openMenu.pendingTarget != nil {
			return m.fireBrowserChoice(m.openMenu.cursor, true)
		}
		return m.fireSelectedMenuTarget(m.openMenu.cursor)
	case "ctrl+o":
		// Primary gesture: ctrl+o (the same key that opened the menu)
		// pops the browser sub-picker for the cursored target. Never
		// collides with a target shortcut. Only in open mode, and not
		// when already inside a browser picker.
		if m.openMenu.pendingTarget == nil && m.openMenu.mode == menuOpen {
			mm := m
			cmd := (&mm).openBrowserSubPicker()
			return mm, cmd
		}
		return m, nil
	}
	// Shortcut match: any single-letter key declared as a target's
	// Shortcut fires that target directly. j/k/enter/esc reserved above.
	for i, t := range m.openMenu.targets {
		if t.Shortcut != "" && key == t.Shortcut {
			return m.fireSelectedMenuTarget(i)
		}
	}
	// `b` opens the browser sub-picker, but only when no visible target
	// claimed `b` as its own shortcut (checked above). Keeps the
	// discoverable "b choose browser" hint without stealing a real
	// target's accelerator.
	if key == "b" && m.openMenu.pendingTarget == nil && m.openMenu.mode == menuOpen {
		mm := m
		cmd := (&mm).openBrowserSubPicker()
		return mm, cmd
	}
	return m, nil
}

// fireSelectedMenuTarget fires the target at the given index. Splits
// out from handleOpenMenuKey so Enter and shortcut dispatch share the
// same logic — including the sub-modal handling, where we must NOT
// clear m.openMenu before calling fireMenuTarget (it's expected to
// set up the new sub-modal state itself).
func (m Model) fireSelectedMenuTarget(idx int) (Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.openMenu.targets) {
		return m, nil
	}
	target := m.openMenu.targets[idx]
	org := m.openMenu.org
	mode := m.openMenu.mode
	source := m.openMenu.source

	// Sub-modal targets handle their own openMenu transitions. Don't
	// clear the menu here; fireMenuTarget will swap it.
	if target.ID == relatedRecordOpenTargetID || target.ID == moveOrgPickerTargetID {
		return m.fireMenuTarget(org, target, mode, source)
	}

	// Find-in-org choice: look up the same resource in the picked org
	// and switch there only if it exists. Unwinds the menu stack itself.
	if _, ok := parseMoveOrgChoiceID(target.ID); ok {
		return m.fireMoveOrgChoice(idx)
	}

	// Browser-choice target: fire the pending original target with the
	// picked browser as a one-off override (normal window).
	if _, ok := parseBrowserChoiceID(target.ID); ok {
		return m.fireBrowserChoice(idx, false)
	}

	m.openMenu = nil
	// Clear any parent stack — firing a terminal target unwinds
	// everything cleanly.
	m.openMenuStack = nil
	if mode == menuOpen && source != nil {
		m.recordRecentVisit(org.Username, source)
	}
	return m.fireMenuTarget(org, target, mode, source)
}

// fireBrowserChoice opens the browser sub-picker's pending target in
// the browser at idx, private when requested (falls back to a normal
// window when the browser has no CLI private mode). Unwinds the whole
// open-menu stack.
func (m Model) fireBrowserChoice(idx int, private bool) (Model, tea.Cmd) {
	if m.openMenu == nil || idx < 0 || idx >= len(m.openMenu.targets) {
		return m, nil
	}
	browser, ok := parseBrowserChoiceID(m.openMenu.targets[idx].ID)
	if !ok {
		return m, nil
	}
	org := m.openMenu.org
	source := m.openMenu.source
	pending := m.openMenu.pendingTarget
	m.openMenu = nil
	m.openMenuStack = nil
	if pending == nil {
		return m, nil
	}
	// private is meaningless for the system-default (empty) browser and
	// for browsers with no CLI private mode — quietly ignore it there.
	if private && browser != "" {
		if _, supported := browserPrivateFlag(browser); !supported {
			private = false
		}
	} else if browser == "" {
		private = false
	}
	if source != nil {
		m.recordRecentVisit(org.Username, source)
	}
	verb := "opening"
	if private {
		verb = "opening (private)"
	}
	m.flash(verb + " " + pending.Label + " in " + browserChoiceLabel(browser) + "…")
	return m, m.openInBrowserCmdWith(org, *pending, browser, private)
}

// browserChoiceIDPrefix marks a synthetic open-menu target that, when
// fired, opens the sub-picker's pendingTarget in a specific browser.
const browserChoiceIDPrefix = "__browser__:"

func browserChoiceID(name string) string { return browserChoiceIDPrefix + name }

func parseBrowserChoiceID(id string) (string, bool) {
	if !strings.HasPrefix(id, browserChoiceIDPrefix) {
		return "", false
	}
	return strings.TrimPrefix(id, browserChoiceIDPrefix), true
}

func browserChoiceLabel(name string) string {
	if name == "" {
		return "the default browser"
	}
	return name
}

// openBrowserSubPicker swaps the current open menu for a browser
// chooser targeting the cursored open target. Only meaningful in
// menuOpen mode (there's nothing to "open in a browser" when yanking).
// Pushes the current menu onto the stack so esc pops back to it.
func (m *Model) openBrowserSubPicker() tea.Cmd {
	if m.openMenu == nil || m.openMenu.mode != menuOpen {
		return nil
	}
	cur := m.openMenu.cursor
	if cur < 0 || cur >= len(m.openMenu.targets) {
		return nil
	}
	orig := m.openMenu.targets[cur]
	// Don't offer a browser choice for synthetic sub-picker rows (they
	// don't open a URL themselves) or for rows already inside a browser
	// picker.
	if _, isBrowser := parseBrowserChoiceID(orig.ID); isBrowser {
		return nil
	}
	if orig.ID == relatedRecordOpenTargetID || orig.ID == communityLoginPickerTargetID {
		return nil
	}

	rows := []sf.OpenTarget{
		{ID: browserChoiceID(""), Label: "System default", Shortcut: "d"},
	}
	for _, name := range discoverBrowsers() {
		row := sf.OpenTarget{ID: browserChoiceID(name), Label: name}
		// Flag browsers that support a private/incognito window via
		// shift+enter so the affordance is visible per-row.
		if _, ok := browserPrivateFlag(name); ok {
			row.Path = "shift+↵ private"
		}
		rows = append(rows, row)
	}

	prev := *m.openMenu
	m.openMenuStack = append(m.openMenuStack, prev)
	m.openMenu = &openMenuState{
		title:               "Open in browser · " + orig.Label,
		mode:                menuOpen,
		org:                 prev.org,
		source:              prev.source,
		targets:             rows,
		cursor:              0,
		restoreGlobalSearch: prev.restoreGlobalSearch,
		pendingTarget:       &orig,
	}
	return nil
}

// fireMenuTarget executes the chosen target + updates the flash banner.
// source is the Openable the menu was built against (nil when the
// menu was short-circuited past for a single-target Openable); used
// by synthetic targets that need the source's underlying data — eg.
// the community-login sub-picker needs the Contact record map.
func (m Model) fireMenuTarget(o sf.Org, t sf.OpenTarget, mode openMenuMode, source sf.Openable) (Model, tea.Cmd) {
	// Synthetic related-record picker: swap the current open menu
	// for a fresh one built against the related record. Esc on the
	// new menu pops back here (handled by handleOpenMenuKey via the
	// openMenuStack push).
	if t.ID == relatedRecordOpenTargetID {
		mm := m
		cmd := (&mm).openRelatedRecordMenu(mode)
		return mm, cmd
	}
	// Synthetic "Find in another org…" picker: swap the menu for an org
	// chooser. Esc pops back to the parent menu (openMenuStack push).
	if t.ID == moveOrgPickerTargetID {
		mm := m
		cmd := (&mm).openMoveOrgSubPicker()
		return mm, cmd
	}
	// Synthetic "Field values…": open the field-value copy sub-modal
	// built from the FieldRef source (picklist formats, formula, etc.).
	if t.ID == fieldValuesYankTargetID {
		fr, ok := source.(sf.FieldRef)
		if !ok {
			return m, nil
		}
		mm := m
		cmd := (&mm).openFieldValuesYankModal(fr.Field)
		return mm, cmd
	}
	// Synthetic "View definition (in-terminal)": drill into the flow-
	// version viewer instead of opening a URL. Yank is meaningless here
	// (nothing to copy); point the user at the viewer's own yank.
	if t.ID == sf.FlowVersionViewDefinitionTargetID {
		if mode == menuYank {
			m.flash("open the definition first, then " + firstPretty(Keys.YankDefault) + " to copy it")
			return m, nil
		}
		v, ok := source.(sf.FlowVersion)
		if !ok || v.ID == "" {
			return m, nil
		}
		mm := m
		cmd := (&mm).drillFlowVersion(v.ID)
		return mm, cmd
	}
	// Synthetic community-login picker: instead of opening a URL,
	// open the searchable Network sub-picker. yank is meaningless
	// for the picker entry (there's no single URL to copy yet).
	if t.ID == communityLoginPickerTargetID {
		if mode == menuYank {
			m.flash("pick a community first to copy its login URL")
			return m, nil
		}
		ref, ok := source.(sf.RecordRef)
		if !ok {
			return m, nil
		}
		mm := m
		cmd := (&mm).openCommunityLoginPicker(o, ref.Record)
		return mm, cmd
	}
	switch mode {
	case menuOpen:
		m.flash("opening " + t.Label + "…")
		return m, m.openInBrowserCmd(o, t)
	case menuYank:
		if t.YankValue != "" {
			// Preview short values inline; for long content (a big field
			// value) show a length instead of flooding the flash line.
			preview := t.YankValue
			if len(preview) > 60 || strings.ContainsRune(preview, '\n') {
				preview = fmt.Sprintf("%d chars", len(t.YankValue))
			}
			m.flash("copied " + t.Label + ": " + preview)
			return m, yankValueCmd(t.YankValue)
		}
		m.flash("url copied: " + t.Label)
		return m, yankURLCmd(o, t)
	}
	return m, nil
}

// yankValueCmd copies a literal value (not a URL) to the clipboard.
// A failed write is surfaced — Linux without xclip/xsel/wl-copy used
// to land here silently, so every yank flashed success while copying
// nothing.
func yankValueCmd(value string) tea.Cmd {
	return func() tea.Msg {
		if err := writeClipboard(value); err != nil {
			return demoFlashMsg{text: "clipboard unavailable (" + err.Error() + ") — install xclip or wl-clipboard"}
		}
		return nil
	}
}

// cursorLabel best-effort names the thing under the cursor so the menu
// title is meaningful. Falls back to the Openable's Go type if we
// can't come up with anything.
func cursorLabel(target sf.Openable) string {
	switch t := target.(type) {
	case sf.SObject:
		if t.Label != "" && t.Label != t.Name {
			return t.Name + " — " + t.Label
		}
		return t.Name
	case sf.FieldRef:
		return t.SObjectName + "." + t.Field.Name
	case sf.Flow:
		if t.DeveloperName != "" {
			return t.DeveloperName
		}
		return t.DefinitionID
	case sf.FlowVersion:
		if t.MasterLabel != "" {
			return t.MasterLabel
		}
		return t.ID
	case sf.Org:
		if t.Alias != "" {
			return t.Alias
		}
		return t.Username
	case sf.ApexLogRow:
		return "Apex log " + t.ID
	case sf.DeployRow:
		return "Deploy " + t.ID
	case sf.InstalledPackage:
		return t.SubscriberPackageName
	case sf.RecordRef:
		sobj, id := sf.SObjectAndIDFromRecord(t.Record)
		if sobj != "" && id != "" {
			return sobj + " " + id
		}
		return "Record"
	case setupLink:
		return t.Name
	}
	return "item"
}

// renderOpenMenu draws the overlay centered horizontally, anchored a
// few rows from the top. Returns the fully-styled overlay string plus
// the row/col at which to place it. Callers compose it into the final
// view via lipgloss.Place.
func (m Model) renderOpenMenu() string {
	if m.openMenu == nil {
		return ""
	}
	w := modalWidth(m.width, 44, 80)
	inner := w - 4

	titleStyle := lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true)
	subStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	itemStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	itemMuted := lipgloss.NewStyle().Foreground(theme.Muted)
	barStyle := lipgloss.NewStyle().Foreground(theme.BorderHi)
	shortcutStyle := lipgloss.NewStyle().Foreground(theme.Magenta).Bold(true)

	var lines []string
	lines = append(lines, titleStyle.Render(m.openMenu.title))
	lines = append(lines, strings.Repeat("─", inner))

	for i, t := range m.openMenu.targets {
		prefix := "  "
		labelStyle := itemStyle
		if i == m.openMenu.cursor {
			prefix = barStyle.Render("▌") + " "
			labelStyle = itemStyle.Bold(true)
		}
		defaultMark := ""
		if i == 0 {
			defaultMark = itemMuted.Render("  (default)")
		}
		shortcut := "   " // 3 cols reserved so rows align regardless
		if t.Shortcut != "" {
			shortcut = shortcutStyle.Render(t.Shortcut) + "  "
		}
		label := labelStyle.Render(t.Label)
		line := prefix + shortcut + label + defaultMark
		// Second line: the path itself, dimmed. Absolute URLs render
		// verbatim; instance-relative paths get the path alone (the
		// org's host is implicit).
		lines = append(lines, ansi.Truncate(line, inner, "…"))
		// Second line: the value that'll be copied. Value targets show
		// their literal value; URL targets show the path/absolute URL.
		hint := t.YankValue
		if hint == "" {
			hint = t.Path
			if t.AbsoluteURL != "" {
				hint = t.AbsoluteURL
			}
		}
		pathLine := itemMuted.Render("       " + hint)
		lines = append(lines, ansi.Truncate(pathLine, inner, "…"))
	}

	lines = append(lines, "")
	hint := "↑↓ move · ↵ select · esc cancel"
	switch {
	case m.openMenu.pendingTarget != nil:
		// Browser sub-picker: shift+enter opens private where supported.
		hint = "↑↓ move · ↵ open · ⇧↵ private · esc back"
	case m.openMenu.mode == menuOpen:
		// Normal open menu: offer the browser chooser.
		hint = "↑↓ move · ↵ select · b browser · esc cancel"
	}
	lines = append(lines, subStyle.Render(hint))

	return modalBox(strings.Join(lines, "\n"), w)
}
