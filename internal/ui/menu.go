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
		if y, ok := target.(sf.Yankable); ok {
			for _, yt := range y.YankTargets() {
				targets = append(targets, sf.OpenTarget{
					ID: yt.ID, Label: yt.Label, Shortcut: yt.Shortcut, YankValue: yt.Value,
				})
			}
		}
		if fv, name, ok := m.cursoredRecordFieldValue(); ok {
			targets = append([]sf.OpenTarget{{
				ID:        "field_value",
				Label:     "Field value · " + name,
				Shortcut:  "v",
				YankValue: fv,
			}}, targets...)
		}
		if pv, ok := m.cursoredFieldDetailYankValue(); ok {
			targets = append([]sf.OpenTarget{{
				ID:        "picklist_value",
				Label:     "This value · " + ansiTrunc(pv, 30),
				Shortcut:  "v",
				YankValue: pv,
			}}, targets...)
		}
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

func (m Model) handleOpenMenuKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.openMenu == nil {
		return m, nil
	}
	key := msg.String()
	switch key {
	case "ctrl+c", "esc":
		if n := len(m.openMenuStack); n > 0 {
			parent := m.openMenuStack[n-1]
			m.openMenuStack = m.openMenuStack[:n-1]
			m.openMenu = &parent
			return m, nil
		}
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
	for i, t := range m.openMenu.targets {
		if t.Shortcut != "" && key == t.Shortcut {
			return m.fireSelectedMenuTarget(i)
		}
	}
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

	if target.ID == relatedRecordOpenTargetID || target.ID == moveOrgPickerTargetID {
		return m.fireMenuTarget(org, target, mode, source)
	}

	if _, ok := parseMoveOrgChoiceID(target.ID); ok {
		return m.fireMoveOrgChoice(idx)
	}

	if _, ok := parseBrowserChoiceID(target.ID); ok {
		return m.fireBrowserChoice(idx, false)
	}

	m.openMenu = nil
	m.openMenuStack = nil
	if mode == menuOpen && source != nil {
		m.recordRecentVisit(org.Username, source)
	}
	return m.fireMenuTarget(org, target, mode, source)
}

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

func (m *Model) openBrowserSubPicker() tea.Cmd {
	if m.openMenu == nil || m.openMenu.mode != menuOpen {
		return nil
	}
	cur := m.openMenu.cursor
	if cur < 0 || cur >= len(m.openMenu.targets) {
		return nil
	}
	orig := m.openMenu.targets[cur]
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

func (m Model) fireMenuTarget(o sf.Org, t sf.OpenTarget, mode openMenuMode, source sf.Openable) (Model, tea.Cmd) {
	if t.ID == relatedRecordOpenTargetID {
		mm := m
		cmd := (&mm).openRelatedRecordMenu(mode)
		return mm, cmd
	}
	if t.ID == moveOrgPickerTargetID {
		mm := m
		cmd := (&mm).openMoveOrgSubPicker()
		return mm, cmd
	}
	if t.ID == fieldValuesYankTargetID {
		fr, ok := source.(sf.FieldRef)
		if !ok {
			return m, nil
		}
		mm := m
		cmd := (&mm).openFieldValuesYankModal(fr.Field)
		return mm, cmd
	}
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
		lines = append(lines, ansi.Truncate(line, inner, "…"))
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
		hint = "↑↓ move · ↵ open · ⇧↵ private · esc back"
	case m.openMenu.mode == menuOpen:
		hint = "↑↓ move · ↵ select · b browser · esc cancel"
	}
	lines = append(lines, subStyle.Render(hint))

	return modalBox(strings.Join(lines, "\n"), w)
}
