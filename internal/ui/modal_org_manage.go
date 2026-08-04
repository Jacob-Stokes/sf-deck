package ui

// modal_org_manage.go — the roomy "Org Manager" modal that owns
// every group / auth-lifecycle edit action.
//
// The rail (focus=orgs) stays a quick-nav surface: 0 to focus,
// j/k/quick-jump letters for selection, space to fold/expand.
// Anything that *changes* state (create / rename / delete groups,
// move orgs, add org, logout, set default, rename alias) lives
// here so there's room to show the keybindings alongside a wide
// view of the grouped tree.
//
// Layout (term width permitting):
//
//   ┌─ Org Manager ─────────────────────────────────────────────────┐
//   │ Groups & orgs                       │ Actions                  │
//   │ ─────────────────────────────────── │ ──────────────────────── │
//   │ ▌ Client A                       3  │  Group keys              │
//   │ ▌ ● alice@…prod      META           │   n  new group           │
//   │   Production · alice@…              │   R  rename group        │
//   │ ▌ ● alice@…uat       REC            │   x  delete group        │
//   │ ...                                 │   space  fold/expand     │
//   │                                     │   [/]  reorder groups    │
//   │ ▷ Internal                       1  │                          │
//   │ ▌ Ungrouped                      2  │  Org keys                │
//   │   ● my-scratch       FULL           │   A  add org             │
//   │     Scratch · me@example.com        │   D  logout              │
//   │                                     │   *  set default         │
//   │                                     │   =  rename alias        │
//   │                                     │   g  move to group       │
//   │                                     │   <,>  reorder org       │
//   │                                     │                          │
//   │ j/k navigate · esc close                                       │
//   └────────────────────────────────────────────────────────────────┘

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

// orgManageModalState is the modal's live state. The cursor is
// kept here (not on Model) so closing the modal returns the user
// to the rail with its own cursor unchanged.
type orgManageModalState struct {
	// Cursor addresses the modal's row list (built from m.orgs +
	// m.settings.OrgGroups() — same shape as the rail's row list).
	Cursor int
}

// openOrgManageModal returns a bool/cmd matching the onOrgsKey
// intercept signature. The modal seeds its cursor from the rail's
// current cursor so users land on whichever row they were looking at.
func (m *Model) openOrgManageModal() (bool, tea.Cmd) {
	m.orgManageModal = &orgManageModalState{Cursor: m.orgRailCursor}
	return true, nil
}

// closeOrgManageModal dismisses the modal and syncs the rail cursor
// to whatever row the modal cursor was on, so the rail reflects the
// user's last-touched row when they return.
func (m *Model) closeOrgManageModal() {
	if m.orgManageModal == nil {
		return
	}
	m.orgRailCursor = m.orgManageModal.Cursor
	m.clampOrgRailCursor()
	m.orgManageModal = nil
}

// renderOrgManageModal draws the modal, or "" when not active.
func (m Model) renderOrgManageModal() string {
	if m.orgManageModal == nil {
		return ""
	}

	// Prefer wide so there's room for both columns + the keybindings
	// pane. Org manager packs two columns + dense help text, so we
	// give it more room than other modals — up to 140 cols, and at
	// least 75% of the terminal width when available.
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
	// Two-column split: ~55% list, ~45% keys. The keys pane has
	// dense help text (multiple subheaders + key/desc rows) so it
	// needs more room than a simple 60/40 would give.
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

	// Render both columns into string slices; we'll join row-by-row.
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
			// Blank divider above every header except the first.
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

// renderManageGroupHeader is the modal's wider variant of the
// rail's group-header row.
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

// renderManageOrgRow renders one indented org row in the modal. Two
// lines per org — main + sub — same shape as the rail but wider.
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

	// ★ marks the sf-deck-level startup pin (distinct from the sf CLI
	// default). Empty when this org isn't the pinned one — keeps the
	// row clean when no pin is set anywhere.
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

	// Groups + grouping ops — pure sf-deck state.
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

	// Truncate any over-wide lines.
	for i, ln := range lines {
		if lipgloss.Width(ln) > w {
			lines[i] = ansi.Truncate(ln, w, "…")
		}
	}
	return strings.Join(lines, "\n")
}

// handleOrgManageModalKey dispatches input while the org-manage
// modal is open. Returns (handled, cmd). The caller (handleKey) ALWAYS
// returns when handled=true — even on no-op keys — so global
// shortcuts don't leak through.
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
		// `g` is the move-to-group key; `g g` would be ambiguous so
		// we don't double-bind it for go-top here. Use `home` for
		// jump-to-top inside the modal.
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
	// Unrecognised keys are absorbed so global shortcuts don't fire
	// while the modal is open.
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

// orgManageCursorOnHeader reports whether the modal cursor is on a
// group-header row.
func (m *Model) orgManageCursorOnHeader() bool {
	rows := m.currentOrgRailRows()
	c := m.orgManageModal.Cursor
	if c < 0 || c >= len(rows) {
		return false
	}
	return rows[c].Kind == railRowGroupHeader
}

// orgManageCursoredOrg returns the org under the modal cursor, or
// (sf.Org{}, false) when cursor is on a header.
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

// orgManageCursoredGroupID returns the group id at the cursor (header
// row's own id, or the org's containing group id).
func (m *Model) orgManageCursoredGroupID() string {
	rows := m.currentOrgRailRows()
	c := m.orgManageModal.Cursor
	if c < 0 || c >= len(rows) {
		return ""
	}
	return rows[c].GroupID
}

// --- modal-cursor variants of the rail handlers ----------------

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
	// Re-find the group's header position so the modal cursor follows it.
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
	// Re-find the org's row index in the (possibly reshaped) row list
	// and store back to the modal cursor.
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

// pinStartupManageCursoredOrg toggles the sf-deck-level startup pin
// on the cursor org. Pinning is exclusive (one default at a time);
// re-pinning the already-pinned org clears it.
func (m *Model) pinStartupManageCursoredOrg() (bool, tea.Cmd) {
	o, ok := m.orgManageCursoredOrg()
	if !ok {
		return true, nil
	}
	cur := m.settings.DefaultOrgUsername()
	target := o.Username
	if cur == target {
		// Already pinned → unpin.
		target = ""
	}
	if m.settings.PinDefault(target) {
		if !m.saveSettings("") {
			// Save can fail when another sf-deck process changed the file.
			// Keep this process aligned with the last persisted selection.
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
	// Read the explicit override (NOT the effective resolved level).
	// If no override is set, we start the cycle from read_only —
	// matches the visible row label so the first press has an
	// obvious result.
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
		// No override → start at read_only.
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
		// Full → clear (inherit kind default).
		return settings.SafetyReadOnly, true
	}
	return settings.SafetyReadOnly, false
}

// startReauthManageOrg re-runs the web login flow for the cursored
// org, preserving its alias — the recovery path for a red-dot org
// (RefreshTokenAuthError etc.) without retyping anything. Reuses the
// same tea.ExecProcess shape as the add-org flow: bubbletea suspends,
// sf prompts + opens the browser, and on return the org list
// refetches so the status dot goes green.
//
// The login host is derived from what we know about the org:
// instance URL when present (My Domain hosts are valid login hosts),
// else test.salesforce.com for sandboxes, else sf's default.
func (m *Model) startReauthManageOrg() (bool, tea.Cmd) {
	o, ok := m.orgManageCursoredOrg()
	if !ok {
		return true, nil
	}
	if Demo {
		// No real `sf org login` in demo mode — open the explainer.
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
		// Mangled cached URL — fall back to sf's default login host
		// rather than failing the whole flow.
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
