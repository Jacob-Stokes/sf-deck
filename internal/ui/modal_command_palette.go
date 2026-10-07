package ui

// Command palette — global fuzzy-find modal over every Tab + every
// command in the keymap registry. Bound to `;` by default.

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/keymap"
)

type paletteEntry struct {
	Label       string // primary display text — what fuzzy-matches
	Hint        string // right-aligned dim text (key binding, kind)
	Description string // optional second-line description (unused in v1)
	Category    string // grouping for empty-input view

	TabTarget Tab
	SubtabIdx int  // when targeting a subtab within TabTarget
	HasSubtab bool // distinguishes "tab only" from "tab+subtab 0"
	Run       func(*Model) tea.Cmd
}

// paletteCommand is a static command entry — registered separately
// from tabs because tab entries come from tabSpecs() walk. Add a
// new entry here when a feature is "reachable from anywhere via
// the palette" (refresh org list, toggle safety, edit chip, etc.).
//
// Available is optional. When set, the function is consulted at
// modal open and the entry is hidden when it returns false (the
// action isn't reachable in the current state, e.g. "Save SOQL
// query" with an empty editor). UnavailableReason is shown if the
// user types the entry's exact label — it surfaces grayed out
// rather than missing entirely, so power users can discover what
// state would unlock it.
//
// KeyHint is the keybinding shown right-aligned on the entry row.
// Optional — only set when the action also has a top-level
// keybinding the user could learn (e.g. "Refresh current view"
// has key `r`).
type paletteCommand struct {
	ID                string
	Label             string
	Description       string
	Category          string
	Run               func(*Model) tea.Cmd
	Available         func(Model) bool
	UnavailableReason string
	KeyHint           string
}

var paletteCommands = []paletteCommand{

	{ID: "walkthrough_start", Label: "Start guided walkthrough", Category: "Help",
		Description: "Re-run the hands-on tour of sf-deck's core features",
		Run: func(m *Model) tea.Cmd {
			m.startWalkthrough()
			return nil
		}},
	{ID: "demo_import", Label: "Import demo org", Category: "Help",
		Description: "Add a fully-populated fictional org to explore, alongside your real orgs",
		Available:   func(m Model) bool { return !m.settings.DemoOrgImported() },
		Run: func(m *Model) tea.Cmd {
			m.importDemoOrg()
			return nil
		}},
	{ID: "demo_remove", Label: "Remove demo org", Category: "Help",
		Description: "Drop the demo org and purge its cached data",
		Available:   func(m Model) bool { return m.settings.DemoOrgImported() },
		Run: func(m *Model) tea.Cmd {
			m.removeDemoOrg()
			return nil
		}},

	{ID: "org_add", Label: "Add org", Category: "Org",
		Description: "Authenticate a new Salesforce org via web or sfdx-url",
		Run:         func(m *Model) tea.Cmd { return m.openAddOrgChoice() }},
	{ID: "org_safety", Label: "Edit safety level for active org", Category: "Org",
		Description: "Per-org safety gate (read-only → records → metadata → full → anonymous)",
		Available:   func(m Model) bool { return len(m.orgs) > 0 },
		Run: func(m *Model) tea.Cmd {
			_, cmd := m.openOrgManageModal()
			return cmd
		}},
	{ID: "org_refresh", Label: "Refresh org list", Category: "Org",
		Description: "Re-read `sf org list` to pick up logins/logouts from outside sf-deck",
		Run: func(m *Model) tea.Cmd {
			return m.orgsRes.Refresh(m.cache)
		}},
	{ID: "org_refresh_all", Label: "Refresh all loaded data for active org", Category: "Org",
		Available: func(m Model) bool { return len(m.orgs) > 0 },
		KeyHint:   firstPretty(Keys.Refresh),
		Run: func(m *Model) tea.Cmd {
			mm, cmd := m.refreshAllLoaded()
			*m = mm
			return cmd
		}},

	{ID: "view_manage", Label: "Open view (chip) manager", Category: "View",
		Description: "Manage saved filters for the current surface",
		Available: func(m Model) bool {
			return m.resolveChipSurface() != nil
		},
		KeyHint: firstPretty(Keys.OpenLensManager),
		Run: func(m *Model) tea.Cmd {
			surf := m.resolveChipSurface()
			if surf == nil {
				return nil
			}
			return m.openChipManagerFor(
				surf.Domain,
				surfaceManagerScope(*surf, *m),
				surfaceManagerTitle(*surf, *m),
				surf.ImportFromSF,
			)
		}},
	{ID: "view_toggle_sidebar", Label: "Toggle sidebar", Category: "View",
		KeyHint: firstPretty(Keys.ToggleSidebar),
		Run: func(m *Model) tea.Cmd {
			m.sidebarOpen = !m.sidebarOpen
			return nil
		}},
	{ID: "view_toggle_zen", Label: "Toggle zen (full-screen) mode", Category: "View",
		KeyHint: firstPretty(Keys.ZenMode),
		Available: func(m Model) bool {
			return (&m).activeListTableState() != nil
		},
		Run: func(m *Model) tea.Cmd {
			if st := m.activeListTableState(); st != nil {
				st.Zen = !st.Zen
			}
			return nil
		}},
	{ID: "view_toggle_left", Label: "Toggle left rail (orgs panel)", Category: "View",
		KeyHint: firstPretty(Keys.ToggleLeft),
		Run: func(m *Model) tea.Cmd {
			m.leftOpen = !m.leftOpen
			return nil
		}},

	{ID: "project_new", Label: "New dev project", Category: "Project",
		Description: "Create a named working set of items across orgs",
		Run:         func(m *Model) tea.Cmd { return m.openNewDevProjectModal() }},
	{ID: "project_go", Label: "Go to dev projects", Category: "Project",
		KeyHint: "→ /dev-projects",
		Run: func(m *Model) tea.Cmd {
			m.setTab(TabDevProjects)
			return nil
		}},

	{ID: "open_settings", Label: "Open settings", Category: "System",
		KeyHint: firstPretty(Keys.OpenSettings),
		Run:     func(m *Model) tea.Cmd { return m.openSettingsModal() }},
	{ID: "open_keybindings", Label: "Edit keybindings", Category: "System",
		KeyHint: firstPretty(Keys.Help),
		Run:     func(m *Model) tea.Cmd { return m.openKeybindingsModal() }},
	{ID: "open_api_log", Label: "Open API call log", Category: "System",
		Description: "Recent `sf` shell-outs + REST calls with timing",
		Run:         func(m *Model) tea.Cmd { m.openAPILogModal(); return nil }},
	{ID: "open_downloads", Label: "Open downloads", Category: "System",
		Description: "Files sf-deck has written (exports, bundles)",
		Run:         func(m *Model) tea.Cmd { m.openDownloadsModal(); return nil }},
	{ID: "open_cache_settings", Label: "Cache settings", Category: "System",
		Description: "Per-resource cache TTLs and clear actions",
		Run:         func(m *Model) tea.Cmd { return m.openCacheSettingsModal() }},
	{ID: "refresh_current", Label: "Refresh current view", Category: "System",
		KeyHint: firstPretty(Keys.Refresh),
		Run: func(m *Model) tea.Cmd {
			mm, cmd := m.refreshCurrent()
			*m = mm
			return cmd
		}},
	{ID: "browser_modal", Label: "Choose default browser", Category: "System",
		Description: "Which browser `o` opens (sf-deck remembers per machine)",
		Run:         func(m *Model) tea.Cmd { return m.openBrowserModal() }},
	{ID: "inspector_modal", Label: "Configure Salesforce Inspector URL", Category: "System",
		Description: "Inspector Reloaded base URL for cross-tool jumps",
		Run:         func(m *Model) tea.Cmd { return m.openInspectorURLModal() }},
	{ID: "show_version", Label: "Show sf-deck version", Category: "System",
		Run: func(m *Model) tea.Cmd {
			m.flash("sf-deck — run `sf-deck --version` for build info")
			return nil
		}},

	{ID: "help_keymap", Label: "Show all keybindings", Category: "Help",
		KeyHint: firstPretty(Keys.Help),
		Run:     func(m *Model) tea.Cmd { return m.openKeybindingsModal() }},
}

type commandPaletteState struct {
	Query string

	Entries []paletteEntry

	Filtered []paletteEntry

	Cursor int
}

// openCommandPalette builds the entry list from tabSpecs +
// paletteCommands and shows the modal. Idempotent: a second open
// closes-and-reopens with a fresh query.
func (m *Model) openCommandPalette() tea.Cmd {
	entries := buildPaletteEntries(*m)
	m.commandPalette = &commandPaletteState{
		Entries:  entries,
		Filtered: entries, // empty query shows everything; ranked-by-default
		Cursor:   0,
	}
	return nil
}

func (m *Model) closeCommandPalette() {
	m.commandPalette = nil
}

func buildPaletteEntries(m Model) []paletteEntry {
	out := []paletteEntry{}

	numbered := TabsForNumbers()
	numberedSet := map[Tab]bool{}
	for i, t := range numbered {
		numberedSet[t] = true
		out = append(out, paletteEntry{
			Label:     "/" + t.String(),
			Hint:      fmt.Sprintf("tab · %d", i+1),
			Category:  "Tabs",
			TabTarget: t,
		})
	}

	for tab, spec := range tabSpecs() {
		for i, sub := range spec.Subtabs {
			out = append(out, paletteEntry{
				Label:     "/" + tab.String() + " " + sub.Label,
				Hint:      "subtab",
				Category:  "Tabs",
				TabTarget: tab,
				SubtabIdx: i,
				HasSubtab: true,
			})
		}
	}

	allTabs := []Tab{
		TabHome, TabSOQL, TabObjects, TabFlows, TabApex, TabLWC, TabPerms,
		TabReports, TabMeta, TabRecords,
		TabPackages, TabSetup, TabSystem, TabDevProjects,
		TabTags, TabUsers, TabExec, TabCompare,
		// TabRecent intentionally excluded — /recent now lives as
		// /home → Recent. The Tab constant remains valid for drill-
		// return logic but isn't a directly-reachable top-level tab.
	}
	for _, t := range allTabs {
		if numberedSet[t] {
			continue
		}
		out = append(out, paletteEntry{
			Label:     "/" + t.String(),
			Hint:      "tab",
			Category:  "Tabs",
			TabTarget: t,
		})
	}

	// Static commands. Each is checked against its Available gate so
	// commands that require state the user doesn't have right now
	// (e.g. "Open view manager" without a chip surface) drop out.
	// Future polish: surface them grayed-with-reason on exact name
	// match so power users can discover what's gated.
	for _, c := range paletteCommands {
		// Capture the value for the closure — Go's range-loop var
		// reuse would otherwise share one Run pointer.
		cmd := c
		if cmd.Available != nil && !cmd.Available(m) {
			continue
		}
		hint := "cmd"
		if cmd.KeyHint != "" {
			hint = cmd.KeyHint
		}
		out = append(out, paletteEntry{
			Label:       cmd.Label,
			Hint:        hint,
			Description: cmd.Description,
			Category:    cmd.Category,
			Run:         cmd.Run,
		})
	}

	_ = keymap.First(Keys.CommandPalette)

	return out
}

// applyPaletteFilter updates the Filtered list + clamps the cursor
// based on the current Query. Empty query → all entries in their
// original order; non-empty query → ranked by fuzzy-match score
// descending.
func (cp *commandPaletteState) applyPaletteFilter() {
	if cp.Query == "" {
		cp.Filtered = cp.Entries
		if cp.Cursor >= len(cp.Filtered) {
			cp.Cursor = 0
		}
		return
	}
	q := strings.ToLower(strings.TrimSpace(cp.Query))
	type scored struct {
		entry paletteEntry
		score int
	}
	out := []scored{}
	for _, e := range cp.Entries {
		if s := palettematchScore(strings.ToLower(e.Label), q); s > 0 {
			out = append(out, scored{e, s})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].entry.Label < out[j].entry.Label
	})
	cp.Filtered = make([]paletteEntry, len(out))
	for i, s := range out {
		cp.Filtered[i] = s.entry
	}
	if cp.Cursor >= len(cp.Filtered) {
		cp.Cursor = 0
	}
}

// palettematchScore is the fuzzy-match scorer. Higher = better
// match. Returns 0 when q can't be found in label at all.
//
// Score chain:
//
//	1000 — exact match
//	 800 — label starts with q
//	 600 — label contains q as substring
//	 400 — label contains q's chars as a subsequence
//	   0 — no match
//
// Word-boundary subsequence matches (each char starting a word)
// score higher within the 400 band.
func palettematchScore(label, q string) int {
	if label == q {
		return 1000
	}
	if strings.HasPrefix(label, q) {
		return 800
	}
	if strings.Contains(label, q) {
		return 600
	}
	li, qi := 0, 0
	first, last := -1, -1
	for li < len(label) && qi < len(q) {
		if label[li] == q[qi] {
			if first < 0 {
				first = li
			}
			last = li
			qi++
		}
		li++
	}
	if qi < len(q) {
		return 0 // not all chars found
	}
	span := last - first + 1
	tightness := 0
	if span > 0 {
		tightness = 200 - (span * 200 / max(len(label), 1))
		if tightness < 0 {
			tightness = 0
		}
	}
	return 400 + tightness
}

func (m *Model) activatePaletteEntry() tea.Cmd {
	cp := m.commandPalette
	if cp == nil || cp.Cursor < 0 || cp.Cursor >= len(cp.Filtered) {
		m.closeCommandPalette()
		return nil
	}
	e := cp.Filtered[cp.Cursor]
	m.closeCommandPalette()
	if e.Run != nil {
		return e.Run(m)
	}
	m.setTab(e.TabTarget)
	if e.HasSubtab {
		if spec := lookupTabSpec(e.TabTarget); spec != nil && spec.SetSubtabIdx != nil {
			spec.SetSubtabIdx(m, e.SubtabIdx)
		}
	}
	return m.onTabChanged()
}

func (m Model) renderCommandPalette() string {
	cp := m.commandPalette
	if cp == nil {
		return ""
	}

	w := modalWidth(m.width, 60, 90)
	inner := w - 4

	var lines []string
	titleStyle := lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true)
	lines = append(lines, titleStyle.Render("Command menu"))
	lines = append(lines, strings.Repeat("─", inner))

	caret := lipgloss.NewStyle().Foreground(theme.BorderHi).Render("│")
	prompt := lipgloss.NewStyle().Foreground(theme.FgDim).Render("> ")
	queryLine := prompt + cp.Query + caret
	lines = append(lines, queryLine, "")

	maxRows := m.settings.LayoutCommandPaletteRows()
	if len(cp.Filtered) < maxRows {
		maxRows = len(cp.Filtered)
	}
	if cp.Cursor < 0 {
		cp.Cursor = 0
	}
	if cp.Cursor >= len(cp.Filtered) {
		cp.Cursor = len(cp.Filtered) - 1
		if cp.Cursor < 0 {
			cp.Cursor = 0
		}
	}
	start := 0
	if cp.Cursor >= maxRows {
		start = cp.Cursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(cp.Filtered) {
		end = len(cp.Filtered)
	}

	if len(cp.Filtered) == 0 {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(theme.FgDim).Italic(true).
				Render("  no matches"))
	}

	for i := start; i < end; i++ {
		e := cp.Filtered[i]
		hint := ""
		if e.Hint != "" {
			hint = lipgloss.NewStyle().Foreground(theme.FgDim).Render(e.Hint)
		}
		labelW := inner - lipglossWidth(hint) - 4
		if labelW < 8 {
			labelW = 8
		}
		label := e.Label
		if lipglossWidth(label) > labelW {
			label = ansi.Truncate(label, labelW, "…")
		}
		row := "  " + label + strings.Repeat(" ", labelW-lipglossWidth(label)) + "  " + hint
		if i == cp.Cursor {
			row = lipgloss.NewStyle().Foreground(theme.Bg).Background(theme.Blue).Render(row)
		}
		lines = append(lines, row)
	}

	lines = append(lines, "")
	footer := lipgloss.NewStyle().Foreground(theme.FgDim).
		Render("↑↓ navigate · ↵ run · esc cancel")
	lines = append(lines, footer)

	body := strings.Join(lines, "\n")
	return modalBox(body, w)
}

func (m *Model) handleCommandPaletteKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.commandPalette == nil {
		return false, nil
	}
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return true, nil // swallow non-press events while palette is open
	}
	cp := m.commandPalette

	switch press.Code {
	case tea.KeyEsc:
		m.closeCommandPalette()
		return true, nil
	case tea.KeyEnter, tea.KeyTab:
		return true, m.activatePaletteEntry()
	case tea.KeyUp:
		if cp.Cursor > 0 {
			cp.Cursor--
		}
		return true, nil
	case tea.KeyDown:
		if cp.Cursor < len(cp.Filtered)-1 {
			cp.Cursor++
		}
		return true, nil
	case tea.KeyBackspace:
		if len(cp.Query) > 0 {
			cp.Query = cp.Query[:len(cp.Query)-1]
			cp.applyPaletteFilter()
		}
		return true, nil
	}

	r := keypressRune(press)
	if r != 0 {
		cp.Query += string(r)
		cp.applyPaletteFilter()
		return true, nil
	}
	return true, nil
}

func keypressRune(msg tea.KeyPressMsg) rune {
	if len(msg.Text) == 1 {
		r := []rune(msg.Text)[0]
		if r >= 32 && r != 127 {
			return r
		}
	}
	return 0
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
