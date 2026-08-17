package ui

// modal_org_manage.go — the roomy "Org Manager" modal that owns
// every group / auth-lifecycle edit action.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

type orgManageModalState struct {
	Cursor int
}

func (m *Model) openOrgManageModal() (bool, tea.Cmd) {
	m.orgManageModal = &orgManageModalState{Cursor: m.orgRailCursor}
	return true, nil
}

func (m *Model) closeOrgManageModal() {
	if m.orgManageModal == nil {
		return
	}
	m.orgRailCursor = m.orgManageModal.Cursor
	m.clampOrgRailCursor()
	m.orgManageModal = nil
}

func (m Model) renderOrgManageModal() string {
	if m.orgManageModal == nil {
		return ""
	}

	w := m.width * 3 / 4
	if w < 90 {
		w = 90
	}
	if w > 140 {
		w = 140
	}
	if w > m.width-2 {
		w = m.width - 2
	}
	// Inner is the usable content width. modalBox renders with
	// Width(w-2) + Padding(0,1) + a 1-char border on each side; in
	// lipgloss v2 the padding sits inside Width, so the visible
	// content area is (w-2) - 2 = w-4. Subtract another 2 as a
	// safety margin against off-by-one wrap when terminal-side
	// rendering disagrees on the byte/cell count of unicode glyphs
	// (the ─ U+2500 divider and ★ pin star both cost more bytes
	// than cells).
	inner := w - 6
	if inner < 40 {
		inner = 40
	}
	listW := inner * 55 / 100
	keysW := inner - listW - 2
	if keysW < 30 {
		keysW = 30
		listW = inner - keysW - 2
	}

	lines := []string{
		lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true).Render("Org Manager"),
		lipgloss.NewStyle().Foreground(theme.Muted).Render(strings.Repeat("─", inner)),
	}

	listCol := m.renderOrgManageList(listW)
	keysCol := m.renderOrgManageKeys(keysW)

	listRows := strings.Split(listCol, "\n")
	keyRows := strings.Split(keysCol, "\n")
	rowCount := len(listRows)
	if len(keyRows) > rowCount {
		rowCount = len(keyRows)
	}
	for i := 0; i < rowCount; i++ {
		var l, r string
		if i < len(listRows) {
			l = listRows[i]
		}
		if i < len(keyRows) {
			r = keyRows[i]
		}
		l = padRight(l, listW)
		lines = append(lines, l+"  "+r)
	}

	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(theme.FgDim).
		Render("j/k navigate · esc close · keys above act on the cursored row"))

	return modalBox(strings.Join(lines, "\n"), w)
}

// renderOrgManageList renders the grouped tree at width w. Same
// shape as the rail but wider — full alias, full username, kind tag,
// safety tag.
func (m Model) renderOrgManageList(w int) string {
	groups := m.settings.OrgGroups()
	rows := buildRailRows(m.orgs, groups)
	cursor := 0
	if m.orgManageModal != nil {
		cursor = m.orgManageModal.Cursor
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(rows) {
		cursor = len(rows) - 1
	}
	if len(rows) == 0 {
		return lipgloss.NewStyle().Foreground(theme.FgDim).Render("  no orgs")
	}

	var b strings.Builder
	headersSeen := 0
	for i, row := range rows {
		onCursor := i == cursor
		switch row.Kind {
		case railRowGroupHeader:
			if headersSeen > 0 {
				b.WriteByte('\n')
			}
			headersSeen++
			b.WriteString(m.renderManageGroupHeader(row, onCursor, groups, w))
			b.WriteByte('\n')
		case railRowOrg:
			b.WriteString(m.renderManageOrgRow(row, onCursor, w))
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderManageGroupHeader(row orgRailRow, onCursor bool, groups []settings.OrgGroupConfig, w int) string {
	collapsed := groupHeaderCollapsed(groups, row.GroupID)
	count := groupMemberCount(m.orgs, groups, row.GroupID)
	name := groupHeaderLabel(groups, row.GroupID)

	arrow := "▌"
	if collapsed {
		arrow = "▷"
	}
	arrowStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	nameStyle := lipgloss.NewStyle().Foreground(theme.Fg).Bold(true)
	if row.GroupID == ungroupedID {
		nameStyle = lipgloss.NewStyle().Foreground(theme.FgDim)
	}
	if onCursor {
		arrowStyle = arrowStyle.Foreground(theme.BorderHi)
		nameStyle = nameStyle.Underline(true)
	}

	countStr := fmt.Sprintf("%d", count)
	countStyle := lipgloss.NewStyle().Foreground(theme.Muted)

	left := arrowStyle.Render(arrow) + " "
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(countStr)
	nameMax := w - leftW - rightW - 2
	if nameMax < 4 {
		nameMax = 4
	}
	name = ansi.Truncate(name, nameMax, "…")
	rendered := nameStyle.Render(name)
	pad := w - leftW - lipgloss.Width(rendered) - rightW
	if pad < 1 {
		pad = 1
	}
	return left + rendered + strings.Repeat(" ", pad) + countStyle.Render(countStr)
}

func (m Model) renderManageOrgRow(row orgRailRow, onCursor bool, w int) string {
	o := row.Org

	prefix := "  "
	if onCursor {
		prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " "
	}

	dot := statusDot(o.Status)
	label := o.Display()
	if label == "" {
		label = "(no alias)"
	}
	labelStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	subStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	if onCursor {
		labelStyle = labelStyle.Bold(true)
		subStyle = lipgloss.NewStyle().Foreground(theme.Muted)
	}

	safetyTag := safetyTagInline(m.safetyFor(o))
	safetyW := lipgloss.Width(safetyTag)

	pinStar := ""
	if m.settings.DefaultOrgUsername() == o.Username {
		pinStar = lipgloss.NewStyle().Foreground(theme.Yellow).Render("★ ")
	}
	pinW := lipgloss.Width(pinStar)
	defaults := cliDefaultMarkers(o)
	defaultsW := lipgloss.Width(defaults)

	prefixW := lipgloss.Width(prefix) + 2
	labelMax := w - prefixW - pinW - safetyW - defaultsW - 1
	if labelMax < 6 {
		labelMax = 6
	}
	label = ansi.Truncate(label, labelMax, "…")
	main := prefix + dot + " " + pinStar + labelStyle.Render(label) + defaults + " " + safetyTag
	main = ansi.Truncate(main, w, "…")

	sub := "    " + o.Kind() + " · " + o.Username
	sub = subStyle.Render(ansi.Truncate(sub, w, "…"))
	if tag := scratchExpiryTag(o); tag != "" {
		sub += " " + tag
	}

	return main + "\n" + sub
}

// renderOrgManageKeys renders the right-hand keybindings + help
// pane. Headings + key list. Reads from the live keymap so user
// rebindings show up.
//
// Layout principle: actions are split into clearly-labelled
// subsections so users know which keys mutate sf-deck's own state
// (settings.toml) and which ones shell out to the `sf` CLI (and so
// require an sfdx project context / can fail with sfdx errors).
//
//   - sf-deck only        : pin startup, safety, groups
//   - sf CLI (sfdx)       : add org, logout, set default, alias edits
//   - Local view (grouping, reordering) : sf-deck state
//
// Adding a new key: put it in the section that matches its blast
// radius, not the noun. "rename alias" affects the sf CLI default
// even though it's an alias edit, so it lives under sf CLI.
func (m Model) renderOrgManageKeys(w int) string {
	hdr := func(s string) string {
		return lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true).Render(s)
	}
	subhdr := func(s string) string {
		return lipgloss.NewStyle().Foreground(theme.FgDim).Italic(true).Render(s)
	}
	row := func(key, desc string) string {
		k := lipgloss.NewStyle().Foreground(theme.Fg).Bold(true).Render(padRight(key, 8))
		d := lipgloss.NewStyle().Foreground(theme.FgDim).Render(desc)
		return "  " + k + d
	}

	var lines []string

	lines = append(lines, hdr("Groups (sf-deck)"))
	lines = append(lines, row(firstPretty(Keys.OrgGroupCreate), "new group"))
	lines = append(lines, row(firstPretty(Keys.OrgGroupRename), "rename group"))
	lines = append(lines, row(firstPretty(Keys.OrgGroupDelete), "delete group"))
	lines = append(lines, row(firstPretty(Keys.OrgGroupToggle), "fold / expand"))
	// " or " (not ", ") between the labels: these default to [ and ], and
	// "[, ]" reads as a single malformed token rather than two keys.
	lines = append(lines, row(firstPretty(Keys.OrgGroupReorderUp)+" or "+firstPretty(Keys.OrgGroupReorderDn), "reorder groups"))
	lines = append(lines, "")

	// Org actions that affect sf-deck only — startup pin, safety,
	// where the org sits in our grouping. None of these touch the
	// sf CLI or require an sfdx project.
	lines = append(lines, hdr("Org · sf-deck"))
	lines = append(lines, subhdr("  writes ~/.sf-deck/settings.toml"))
	lines = append(lines, row(firstPretty(Keys.OrgPinStartup), "pin as startup org ★"))
	lines = append(lines, row(firstPretty(Keys.OrgCycleSafety), "cycle safety level"))
	lines = append(lines, row(firstPretty(Keys.OrgMoveToGroup), "move to group"))
	lines = append(lines, row(firstPretty(Keys.OrgMoveUp)+" / "+firstPretty(Keys.OrgMoveDown), "reorder org"))
	lines = append(lines, "")

	// Org actions that shell out to the `sf` CLI. These can fail
	// independently of sf-deck (no sfdx project, expired auth, etc.)
	// and the error surfaces in the flash banner.
	lines = append(lines, hdr("Org · sfdx (sf CLI)"))
	lines = append(lines, subhdr("  shells out to `sf …`"))
	lines = append(lines, row(firstPretty(Keys.OrgAddOrg), "add org"))
	lines = append(lines, row(firstPretty(Keys.OrgDisconnect), "logout"))
	lines = append(lines, row(firstPretty(Keys.OrgReauth), "re-authenticate"))
	lines = append(lines, row(firstPretty(Keys.OrgSetDefault), "sfdx default org"))
	lines = append(lines, row(firstPretty(Keys.OrgSetDefaultDevHub), "sfdx default DevHub"))
	lines = append(lines, row(firstPretty(Keys.OrgSetAlias), "rename alias"))
	lines = append(lines, row(firstPretty(Keys.OrgUnsetAlias), "clear alias"))
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(theme.FgDim).Italic(true).
		Render(ansi.Truncate("Tip: header row = group keys; org row = org keys.", w, "…")))

	for i, ln := range lines {
		if lipgloss.Width(ln) > w {
			lines[i] = ansi.Truncate(ln, w, "…")
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Model) handleOrgManageModalKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.orgManageModal == nil {
		return false, nil
	}
	key := msg.String()

	switch key {
	case "esc", "ctrl+c":
		m.closeOrgManageModal()
		return true, nil
	case "j", "down":
		m.stepOrgManageCursor(1)
		return true, nil
	case "k", "up":
		m.stepOrgManageCursor(-1)
		return true, nil
	case "g", "home":
	case "G", "end":
		m.orgManageModal.Cursor = 1 << 30
		m.clampOrgManageCursor()
		return true, nil
	}

	switch {
	case matches(key, Keys.OrgGroupToggle):
		return m.toggleManageCursoredGroup()
	case matches(key, Keys.OrgGroupCreate):
		return m.startCreateGroup()
	case matches(key, Keys.OrgGroupRename):
		return m.startRenameManageCursoredGroup()
	case matches(key, Keys.OrgGroupDelete):
		return m.deleteManageCursoredGroup()
	case matches(key, Keys.OrgGroupReorderUp):
		return m.reorderManageCursoredGroup(-1)
	case matches(key, Keys.OrgGroupReorderDn):
		return m.reorderManageCursoredGroup(1)
	case matches(key, Keys.OrgMoveUp):
		return m.moveManageCursoredOrg(-1)
	case matches(key, Keys.OrgMoveDown):
		return m.moveManageCursoredOrg(1)
	case matches(key, Keys.OrgMoveToGroup):
		return m.startMoveManageOrgToGroup()
	case matches(key, Keys.OrgAddOrg):
		return m.startAddOrg()
	case matches(key, Keys.OrgDisconnect):
		return m.startDisconnectManageOrg()
	case matches(key, Keys.OrgReauth):
		return m.startReauthManageOrg()
	case matches(key, Keys.OrgSetDefault):
		return m.setDefaultManageCursoredOrg()
	case matches(key, Keys.OrgSetDefaultDevHub):
		return m.setDefaultDevHubManageCursoredOrg()
	case matches(key, Keys.OrgPinStartup):
		return m.pinStartupManageCursoredOrg()
	case matches(key, Keys.OrgCycleSafety):
		return m.cycleSafetyManageCursoredOrg()
	case matches(key, Keys.OrgSetAlias):
		return m.startRenameManageCursoredAlias()
	case matches(key, Keys.OrgUnsetAlias):
		return m.startUnsetManageCursoredAlias()
	}
	return true, nil
}

// stepOrgManageCursor advances the modal cursor by `delta`. Clamps
// to the row list.
func (m *Model) stepOrgManageCursor(delta int) {
	rows := m.currentOrgRailRows()
	if len(rows) == 0 {
		m.orgManageModal.Cursor = 0
		return
	}
	c := m.orgManageModal.Cursor + delta
	if c < 0 {
		c = 0
	}
	if c >= len(rows) {
		c = len(rows) - 1
	}
	m.orgManageModal.Cursor = c
}

// clampOrgManageCursor keeps the modal cursor inside the row list.
func (m *Model) clampOrgManageCursor() {
	rows := m.currentOrgRailRows()
	if len(rows) == 0 {
		m.orgManageModal.Cursor = 0
		return
	}
	if m.orgManageModal.Cursor < 0 {
		m.orgManageModal.Cursor = 0
	}
	if m.orgManageModal.Cursor >= len(rows) {
		m.orgManageModal.Cursor = len(rows) - 1
	}
}

func (m *Model) orgManageCursorOnHeader() bool {
	rows := m.currentOrgRailRows()
	c := m.orgManageModal.Cursor
	if c < 0 || c >= len(rows) {
		return false
	}
	return rows[c].Kind == railRowGroupHeader
}

func (m *Model) orgManageCursoredOrg() (sf.Org, bool) {
	rows := m.currentOrgRailRows()
	c := m.orgManageModal.Cursor
	if c < 0 || c >= len(rows) {
		return sf.Org{}, false
	}
	if rows[c].Kind != railRowOrg {
		return sf.Org{}, false
	}
	return rows[c].Org, true
}

func (m *Model) orgManageCursoredGroupID() string {
	rows := m.currentOrgRailRows()
	c := m.orgManageModal.Cursor
	if c < 0 || c >= len(rows) {
		return ""
	}
	return rows[c].GroupID
}

func (m *Model) toggleManageCursoredGroup() (bool, tea.Cmd) {
	if !m.orgManageCursorOnHeader() {
		return true, nil
	}
	gid := m.orgManageCursoredGroupID()
	if gid == "" || gid == ungroupedID {
		return true, nil
	}
	groups := m.settings.OrgGroups()
	idx, _ := findGroupByID(groups, gid)
	if idx < 0 {
		return true, nil
	}
	groups[idx].Collapsed = !groups[idx].Collapsed
	m.settings.SetOrgGroups(groups)
	m.saveSettings("")
	m.clampOrgManageCursor()
	return true, nil
}

func (m *Model) startRenameManageCursoredGroup() (bool, tea.Cmd) {
	if !m.orgManageCursorOnHeader() {
		return true, nil
	}
	gid := m.orgManageCursoredGroupID()
	if gid == "" || gid == ungroupedID {
		return true, nil
	}
	_, g := findGroupByID(m.settings.OrgGroups(), gid)
	if g.ID == "" {
		return true, nil
	}
	return true, m.openOrgGroupPrompt(orgGroupPromptRename, gid, g.Name)
}

func (m *Model) deleteManageCursoredGroup() (bool, tea.Cmd) {
	if !m.orgManageCursorOnHeader() {
		return true, nil
	}
	gid := m.orgManageCursoredGroupID()
	if gid == "" || gid == ungroupedID {
		return true, nil
	}
	groups := m.settings.OrgGroups()
	out := groups[:0]
	for _, g := range groups {
		if g.ID == gid {
			continue
		}
		out = append(out, g)
	}
	m.settings.SetOrgGroups(out)
	m.saveSettings("")
	m.clampOrgManageCursor()
	return true, nil
}

func (m *Model) reorderManageCursoredGroup(delta int) (bool, tea.Cmd) {
	if !m.orgManageCursorOnHeader() {
		return true, nil
	}
	gid := m.orgManageCursoredGroupID()
	if gid == "" || gid == ungroupedID {
		return true, nil
	}
	groups := m.settings.OrgGroups()
	idx, _ := findGroupByID(groups, gid)
	if idx < 0 {
		return true, nil
	}
	target := idx + delta
	if target < 0 || target >= len(groups) {
		return true, nil
	}
	groups[idx], groups[target] = groups[target], groups[idx]
	m.settings.SetOrgGroups(groups)
	m.saveSettings("")
	rows := m.currentOrgRailRows()
	for i, r := range rows {
		if r.Kind == railRowGroupHeader && r.GroupID == gid {
			m.orgManageModal.Cursor = i
			break
		}
	}
	return true, nil
}

func (m *Model) moveManageCursoredOrg(delta int) (bool, tea.Cmd) {
	if m.orgManageCursorOnHeader() {
		return true, nil
	}
	o, ok := m.orgManageCursoredOrg()
	if !ok {
		return true, nil
	}
	username := o.Username
	// Reuse the existing shared mover by temporarily syncing the
	// rail cursor, calling moveCursoredOrg, then re-syncing the
	// modal cursor. Avoids duplicating the cross-group fall-through
	// logic.
	m.orgRailCursor = m.orgManageModal.Cursor
	_, _ = m.moveCursoredOrg(delta)
	rows := m.currentOrgRailRows()
	for i, r := range rows {
		if r.Kind == railRowOrg && r.Org.Username == username {
			m.orgManageModal.Cursor = i
			break
		}
	}
	return true, nil
}

func (m *Model) startMoveManageOrgToGroup() (bool, tea.Cmd) {
	o, ok := m.orgManageCursoredOrg()
	if !ok {
		return true, nil
	}
	return true, m.openOrgMoveToGroupPicker(o.Username)
}

func (m *Model) startDisconnectManageOrg() (bool, tea.Cmd) {
	o, ok := m.orgManageCursoredOrg()
	if !ok {
		return true, nil
	}
	return true, m.openDisconnectOrgConfirm(o)
}

func (m *Model) setDefaultManageCursoredOrg() (bool, tea.Cmd) {
	o, ok := m.orgManageCursoredOrg()
	if !ok {
		return true, nil
	}
	return true, m.runSetDefaultOrg(o)
}

func (m *Model) setDefaultDevHubManageCursoredOrg() (bool, tea.Cmd) {
	o, ok := m.orgManageCursoredOrg()
	if !ok {
		return true, nil
	}
	return true, m.runSetDefaultDevHub(o)
}

func (m *Model) startRenameManageCursoredAlias() (bool, tea.Cmd) {
	o, ok := m.orgManageCursoredOrg()
	if !ok {
		return true, nil
	}
	return true, m.openOrgAliasPrompt(o)
}

func (m *Model) startUnsetManageCursoredAlias() (bool, tea.Cmd) {
	o, ok := m.orgManageCursoredOrg()
	if !ok {
		return true, nil
	}
	return true, m.openUnsetAliasConfirm(o)
}

func (m *Model) pinStartupManageCursoredOrg() (bool, tea.Cmd) {
	o, ok := m.orgManageCursoredOrg()
	if !ok {
		return true, nil
	}
	cur := m.settings.DefaultOrgUsername()
	target := o.Username
	if cur == target {
		target = ""
	}
	if m.settings.PinDefault(target) {
		if !m.saveSettings("") {
			m.settings.PinDefault(cur)
			return true, nil
		}
		if target == "" {
			m.flash("startup pin cleared (using lastUsed order)")
		} else {
			label := o.Display()
			m.flash("startup pin → " + label)
		}
	}
	return true, nil
}

// cycleSafetyManageCursoredOrg steps the per-org safety override
// through: read_only → records → metadata → full → (cleared, inherit
// the kind default) → read_only.
//
// The cleared state matters because users with a sandbox at the
// "records" default may want to RAISE one specific sandbox to
// "metadata" while leaving the rest inheriting the kind default —
// and clearing the override is what reverts that.
func (m *Model) cycleSafetyManageCursoredOrg() (bool, tea.Cmd) {
	o, ok := m.orgManageCursoredOrg()
	if !ok {
		return true, nil
	}
	override, hasOverride := m.settings.OrgSafetyOverride(o.Username)
	next, clear := cycleSafetyOverride(override, hasOverride)
	m.settings.SetOrg(o.Username, next, clear)
	if !m.saveSettings("") {
		// Do not leave an unpersisted safety decision active in memory: it
		// would disappear on restart while the UI had appeared to accept it.
		if hasOverride {
			m.settings.SetOrg(o.Username, settings.ParseSafetyLevel(override), false)
		} else {
			m.settings.SetOrg(o.Username, settings.SafetyReadOnly, true)
		}
		return true, nil
	}
	if clear {
		m.flash(o.Display() + " safety → (inherit default)")
	} else {
		m.flash(o.Display() + " safety → " + next.String())
	}
	return true, nil
}

// cycleSafetyOverride is the pure state-transition function for the
// safety cycle. Extracted for unit tests so the cycle ladder stays
// pinned independently of the modal wiring.
//
// Inputs:
//
//	override     — the current explicit override string ("", "read_only",
//	               "records", "metadata", "full"). Empty when no override.
//	hasOverride  — whether the entry exists at all. Distinct from the
//	               empty-string override because the cycle has a
//	               "(cleared)" state.
//
// Returns the next SafetyLevel + a clear flag (true → call SetOrg
// with clear=true to drop the override entirely).
func cycleSafetyOverride(override string, hasOverride bool) (settings.SafetyLevel, bool) {
	if !hasOverride {
		return settings.SafetyReadOnly, false
	}
	switch settings.ParseSafetyLevel(override) {
	case settings.SafetyReadOnly:
		return settings.SafetyRecords, false
	case settings.SafetyRecords:
		return settings.SafetyMetadata, false
	case settings.SafetyMetadata:
		return settings.SafetyFull, false
	case settings.SafetyFull:
		return settings.SafetyReadOnly, true
	}
	return settings.SafetyReadOnly, false
}

func (m *Model) startReauthManageOrg() (bool, tea.Cmd) {
	o, ok := m.orgManageCursoredOrg()
	if !ok {
		return true, nil
	}
	if Demo {
		browser := ""
		if m.settings != nil {
			browser = m.settings.Browser()
		}
		return true, func() tea.Msg {
			_ = demoAddOrgPage(browser)
			return demoFlashMsg{text: "demo: re-auth is disabled — opened an explainer"}
		}
	}
	instanceURL := o.InstanceURL
	if instanceURL == "" && o.IsSandbox {
		instanceURL = "https://test.salesforce.com"
	}
	if err := sf.ValidateInstanceURL(instanceURL); err != nil {
		instanceURL = ""
	}
	cmd := sf.LoginWebCommand(o.Alias, instanceURL)
	return true, tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return orgLifecycleResultMsg{
				Op:      "reauth",
				Err:     err,
				Message: "re-auth failed: " + err.Error(),
			}
		}
		return orgLifecycleResultMsg{
			Op:      "reauth",
			Message: "re-authenticated " + o.Display() + " — refreshing org list",
			Refetch: true,
		}
	})
}
