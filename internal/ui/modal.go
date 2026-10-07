package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func modalBox(body string, width int) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.BorderHi).
		Padding(0, 1).
		Width(width).
		Render(body)
}

// modalWidth returns a sensible modal width for the current terminal:
// ~60% of width, clamped so it's never too small or too sprawling.
func modalWidth(termW, minW, maxW int) int {
	w := termW * 3 / 5
	if w < minW {
		w = minW
	}
	if w > maxW {
		w = maxW
	}
	return w
}

func modalHeight(termH, minH, maxH int) int {
	h := termH * 4 / 5
	if h < minH {
		h = minH
	}
	if h > maxH {
		h = maxH
	}
	return h
}

type infoModalState struct {
	Title       string
	Rows        []infoRow
	PreRendered string

	// Scroll is the first visible line offset into the body — used when
	// the content is taller than the modal (e.g. a huge field value).
	// j/k/arrows/pgup/pgdn adjust it; renderInfoModal clamps it. Zero =
	// top. When the content fits, scroll is irrelevant and any key
	// dismisses as before.
	Scroll int

	OnDismiss func() tea.Cmd
}

type infoRow struct {
	Label string
	Body  string
}

func (m *Model) showInfoModal(state infoModalState) {
	m.infoModal = &state
}

func (m Model) renderInfoModal() string {
	if m.infoModal == nil {
		return ""
	}
	w := modalWidth(m.width, 44, 80)
	inner := w - 4

	header := []string{
		lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true).Render(m.infoModal.Title),
		strings.Repeat("─", inner),
	}

	var body []string
	if m.infoModal.PreRendered != "" {
		body = strings.Split(m.infoModal.PreRendered, "\n")
	} else {
		for _, r := range m.infoModal.Rows {
			switch {
			case r.Label == "" && r.Body == "":
				body = append(body, "")
			case r.Label == "":
				body = append(body, lipgloss.NewStyle().Foreground(theme.Fg).Render(r.Body))
			default:
				label := lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true).Render(r.Label)
				val := lipgloss.NewStyle().Foreground(theme.Fg).Render(r.Body)
				body = append(body, label+"  "+val)
			}
		}
	}

	maxBody := m.height - 8
	if maxBody < 4 {
		maxBody = 4
	}
	scroll := infoModalScroll(m.infoModal.Scroll, len(body), maxBody)
	overflow := len(body) > maxBody

	lines := header
	if overflow {
		windowH := maxBody - 2
		if windowH < 1 {
			windowH = 1
		}
		end := scroll + windowH
		if end > len(body) {
			end = len(body)
		}
		up := "  "
		if scroll > 0 {
			up = lipgloss.NewStyle().Foreground(theme.FgDim).Render("  ↑ more")
		}
		down := "  "
		if end < len(body) {
			down = lipgloss.NewStyle().Foreground(theme.FgDim).
				Render(fmt.Sprintf("  ↓ %d more", len(body)-end))
		}
		lines = append(lines, up)
		lines = append(lines, body[scroll:end]...)
		lines = append(lines, down)
	} else {
		lines = append(lines, body...)
	}

	footer := "any key dismisses · esc"
	if overflow {
		footer = "j / k scroll · esc dismiss"
	}
	lines = append(lines, "",
		lipgloss.NewStyle().Foreground(theme.FgDim).Render(footer))
	return modalBox(strings.Join(lines, "\n"), w)
}

// infoModalScroll clamps a scroll offset so the window stays within the
// body. n = total lines, visible = height budget (before reserving
// marker lines). Returns 0 when everything fits.
func infoModalScroll(offset, n, visible int) int {
	if n <= visible || visible <= 0 {
		return 0
	}
	windowH := visible - 2 // marker lines top+bottom
	if windowH < 1 {
		windowH = 1
	}
	maxOffset := n - windowH
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset < 0 {
		return 0
	}
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

func (m Model) handleInfoModalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.infoModal == nil {
		return m, nil
	}
	// maxScroll is the real ceiling for the current content + terminal.
	// Clamping the STORED offset (not just the rendered window) is what
	// fixes the "keeps scrolling silently at the end, then reversing lags"
	// bug: without it, holding j pushes Scroll far past the last line, so
	// k has to unwind all that phantom distance before the view moves.
	maxScroll := m.infoModalMaxScroll()
	page := m.height - 10
	if page < 3 {
		page = 3
	}
	clamp := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > maxScroll {
			return maxScroll
		}
		return v
	}
	switch msg.String() {
	case "down", "j":
		m.infoModal.Scroll = clamp(m.infoModal.Scroll + 1)
		return m, nil
	case "up", "k":
		m.infoModal.Scroll = clamp(m.infoModal.Scroll - 1)
		return m, nil
	case "pgdown", "ctrl+d", " ":
		m.infoModal.Scroll = clamp(m.infoModal.Scroll + page)
		return m, nil
	case "pgup", "ctrl+u":
		m.infoModal.Scroll = clamp(m.infoModal.Scroll - page)
		return m, nil
	case "g", "home":
		m.infoModal.Scroll = 0
		return m, nil
	case "G", "end":
		m.infoModal.Scroll = maxScroll
		return m, nil
	}
	onDismiss := m.infoModal.OnDismiss
	m.infoModal = nil
	if onDismiss != nil {
		return m, onDismiss()
	}
	return m, nil
}

func (m Model) infoModalMaxScroll() int {
	if m.infoModal == nil {
		return 0
	}
	n := infoModalBodyLen(m.infoModal)
	maxBody := m.height - 8
	if maxBody < 4 {
		maxBody = 4
	}
	if n <= maxBody {
		return 0
	}
	windowH := maxBody - 2 // marker lines top+bottom
	if windowH < 1 {
		windowH = 1
	}
	maxOffset := n - windowH
	if maxOffset < 0 {
		maxOffset = 0
	}
	return maxOffset
}

func infoModalBodyLen(s *infoModalState) int {
	if s.PreRendered != "" {
		return strings.Count(s.PreRendered, "\n") + 1
	}
	return len(s.Rows)
}

type editModalState struct {
	Title string
	Hint  string
	// InitialBody is the starting buffer for the editor. Pre-populate
	// with the current value if you have it synchronously (e.g. from a
	// cached describe). For values that require an async fetch, leave
	// empty and set LoadCurrent — the modal shows "loading…" and
	// populates the buffer when the fetch returns.
	InitialBody string
	Multiline   bool
	SuccessMsg  string

	LoadCurrent func() (string, error)
	// Save is the on-wire commit. It's called with the current Body
	// value and should return nil on success or an error (ideally an
	// *sf.SFError — the modal will render its Code + Hint if so).
	//
	// When a Preview callback is wired, Save is called with the same
	// `baseline` token the preview returned — letting the Save re-use
	// the already-fetched current state instead of round-tripping again.
	// Plain edits (no Preview) ignore the baseline arg and pass nil.
	Save      func(val string, baseline any) error
	Preview   func(val string) (PreviewResult, error)
	OnSuccess func() tea.Cmd

	OnCancel func() tea.Cmd

	Loading      bool
	Saving       bool
	Confirming   bool
	Previewing   bool
	PreviewLines []PreviewLine
	// PreviewBaseline is the opaque state token returned by Preview
	// and handed back to Save, so the commit uses the same baseline
	// the preview diffed against.
	PreviewBaseline any
	Err             string
	editor          *textarea.Model
}

// PreviewLine is one row of the diff shown during the Confirming
// phase. Field is the property name (e.g. "label"), Before is the
// current value, After is what Save will set it to. An empty Before
// means the field was unset previously.
type PreviewLine struct {
	Field   string
	Before  string
	After   string
	Changed bool // rendered differently when unchanged (dim)
}

// PreviewResult bundles the diff lines to display + an opaque
// baseline token that Save will receive. The token is whatever the
// Preview closure wants to hand off — typically the already-fetched
// current state of the entity, so Save doesn't re-fetch.
type PreviewResult struct {
	Lines    []PreviewLine
	Baseline any
}

func buildEditor(multiline bool, width, height int, initial string) textarea.Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.SetWidth(width)
	if multiline {
		if height < 3 {
			height = 3
		}
		ta.SetHeight(height)
	} else {
		ta.SetHeight(1)
	}
	s := ta.Styles()
	s.Focused.Base = lipgloss.NewStyle().Foreground(theme.Fg)
	s.Focused.Text = lipgloss.NewStyle().Foreground(theme.Fg)
	s.Focused.Placeholder = lipgloss.NewStyle().Foreground(theme.FgDim)
	s.Focused.CursorLine = lipgloss.NewStyle()
	s.Focused.CursorLineNumber = lipgloss.NewStyle()
	s.Cursor.Color = theme.BorderHi
	ta.SetStyles(s)
	ta.SetValue(initial)
	ta.CursorEnd()
	ta.Focus()
	return ta
}

func (m Model) renderEditModal() string {
	if m.editModal == nil {
		return ""
	}
	w := modalWidth(m.width, 60, 100)
	inner := w - 4
	em := m.editModal

	var lines []string
	lines = append(lines,
		lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true).Render(em.Title),
		strings.Repeat("─", inner),
	)
	if em.Hint != "" {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(theme.FgDim).Render(em.Hint),
			"",
		)
	}

	switch {
	case em.Loading:
		lines = append(lines,
			lipgloss.NewStyle().Foreground(theme.FgDim).Italic(true).
				Render("loading current value…"))
	case em.Previewing:
		lines = append(lines,
			lipgloss.NewStyle().Foreground(theme.FgDim).Italic(true).
				Render("building preview…"))
	case em.Confirming:
		lines = append(lines, renderPreviewBody(em, inner))
	case em.editor != nil:
		lines = append(lines, em.editor.View())
	}

	lines = append(lines, "")
	switch {
	case em.Saving:
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Yellow).Render("saving…"))
	case em.Err != "":
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Red).Render("error: "+em.Err))
	}

	hint := editModalKeyHint(em)
	lines = append(lines,
		lipgloss.NewStyle().Foreground(theme.FgDim).Render(hint))
	return modalBox(strings.Join(lines, "\n"), w)
}

func editModalKeyHint(em *editModalState) string {
	switch {
	case em.Saving:
		return "esc cancel"
	case em.Confirming:
		return "enter confirm & deploy · esc back to edit"
	case em.Multiline:
		return "ctrl+s preview · esc cancel · enter newline · arrows/home/end/ctrl+w to edit"
	default:
		if em.Preview != nil {
			return "enter preview · esc cancel · arrows/home/end/ctrl+w to edit"
		}
		return "enter save · esc cancel · arrows/home/end/ctrl+w to edit"
	}
}

func renderPreviewBody(em *editModalState, inner int) string {
	if len(em.PreviewLines) == 0 {
		return lipgloss.NewStyle().Foreground(theme.FgDim).Italic(true).
			Render("(no changes)")
	}
	labelStyle := lipgloss.NewStyle().Foreground(theme.Muted).Width(18)
	beforeStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	afterStyle := lipgloss.NewStyle().Foreground(theme.Green).Bold(true)
	arrow := lipgloss.NewStyle().Foreground(theme.BorderHi).Render(" → ")
	unchangedStyle := lipgloss.NewStyle().Foreground(theme.FgDim)

	var out []string
	for _, p := range em.PreviewLines {
		if !p.Changed {
			out = append(out,
				labelStyle.Render(p.Field+":")+
					unchangedStyle.Render(previewValDisplay(p.After)+" (unchanged)"))
			continue
		}
		line := labelStyle.Render(p.Field+":") +
			beforeStyle.Render(previewValDisplay(p.Before)) +
			arrow +
			afterStyle.Render(previewValDisplay(p.After))
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func previewValDisplay(s string) string {
	if s == "" {
		return "—"
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " …"
	}
	return s
}

func (m Model) handleEditModalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.editModal == nil {
		return m, nil
	}
	em := m.editModal

	// Saving / loading / previewing lock everything except esc-cancel so
	// we don't double-submit or edit a buffer that's about to be
	// replaced by the loader's result.
	if em.Saving || em.Loading || em.Previewing {
		if msg.String() == "esc" {
			m.editModal = nil
		}
		return m, nil
	}

	if em.Confirming {
		switch msg.String() {
		case "esc":
			em.Confirming = false
			em.PreviewLines = nil
			em.PreviewBaseline = nil
			em.Err = ""
			return m, nil
		case "ctrl+c":
			m.editModal = nil
			return m, nil
		case "enter":
			return m.commitEditModal()
		}
		return m, nil
	}

	switch msg.String() {
	case "esc", "ctrl+c":
		onCancel := m.editModal.OnCancel
		key := msg.String()
		m.editModal = nil
		if key == "esc" && onCancel != nil {
			return m, onCancel()
		}
		return m, nil
	case "ctrl+s":
		return m.submitEditModal()
	case "enter":
		if !em.Multiline {
			return m.submitEditModal()
		}
	}

	if em.editor != nil {
		newEditor, cmd := em.editor.Update(msg)
		em.editor = &newEditor
		return m, cmd
	}
	return m, nil
}

// submitEditModal routes an Enter/ctrl+s gesture: when a Preview is
// wired it fires the preview first (transitioning to Confirming on
// the result); otherwise it commits directly.
func (m Model) submitEditModal() (Model, tea.Cmd) {
	em := m.editModal
	if em.Preview != nil {
		em.Previewing = true
		em.Err = ""
		preview := em.Preview
		val := ""
		if em.editor != nil {
			val = em.editor.Value()
		}
		return m, func() tea.Msg {
			res, err := preview(val)
			return editModalPreviewMsg{Result: res, Err: err}
		}
	}
	return m.commitEditModal()
}

func (m Model) commitEditModal() (Model, tea.Cmd) {
	em := m.editModal
	em.Saving = true
	em.Confirming = false
	em.Err = ""
	save := em.Save
	baseline := em.PreviewBaseline
	val := ""
	if em.editor != nil {
		val = em.editor.Value()
	}
	return m, func() tea.Msg {
		if save == nil {
			return editModalResultMsg{Err: nil}
		}
		err := save(val, baseline)
		return editModalResultMsg{Err: err}
	}
}

type editModalResultMsg struct {
	Err error
}

// editModalPreviewMsg carries the outcome of a Preview call — the
// diff lines + baseline token, or an error. On success the main
// Update loop transitions the modal into the Confirming phase.
type editModalPreviewMsg struct {
	Result PreviewResult
	Err    error
}

type editModalLoadedMsg struct {
	Value string
	Err   error
}

func (m *Model) openEditModal(state editModalState) tea.Cmd {
	if state.LoadCurrent != nil {
		state.Loading = true
	}
	editorW := modalWidth(m.width, 60, 100) - 6
	if editorW < 20 {
		editorW = 20
	}
	editorH := m.height - 10
	if editorH > 30 {
		editorH = 30
	}
	if editorH < 8 {
		editorH = 8
	}
	editor := buildEditor(state.Multiline, editorW, editorH, state.InitialBody)
	state.editor = &editor
	s := state
	m.editModal = &s
	if s.LoadCurrent == nil {
		return nil
	}
	loader := s.LoadCurrent
	return func() tea.Msg {
		v, err := loader()
		return editModalLoadedMsg{Value: v, Err: err}
	}
}

// helpForCurrentView returns the info-modal state appropriate for the
// active tab + subtab combination. When focus is on the left rail's
// Orgs utility, the "current view" is effectively the orgs list — so
// we show the safety-policy explainer instead of whatever's in the
// main pane.
func helpForCurrentView(m Model) infoModalState {
	if m.focus == focusOrgs && m.currentUtility().ID == utilityOrgs {
		return helpSafety()
	}
	spec, sub := m.activeSpec()
	if sub != nil && sub.Help != nil {
		if state := sub.Help(m); state.Title != "" {
			return state
		}
	}
	if spec != nil && spec.Help != nil {
		if state := spec.Help(m); state.Title != "" {
			return state
		}
	}
	return infoModalState{
		Title: "Help",
		Rows: []infoRow{
			{Body: "No help page yet for this view."},
			{Body: ""},
			{Label: firstPretty(Keys.CommandPalette), Body: "command menu — fuzzy-find every action"},
			{Body: ""},
			{Body: "Press ? on /objects → schema to see the fields-table legend."},
			{Body: "Press ? on /home or the orgs list to see safety-policy help."},
		},
	}
}

func helpFieldDetail() infoModalState {
	return infoModalState{
		Title: "Field detail · actions",
		Rows: []infoRow{
			{Body: "Main pane lists every property. Editable ones (label, help,"},
			{Body: "description, default, required/unique/external-id, delete) take"},
			{Body: "the cursor; read-only ones (SOQL caps, picklist…) are skipped-over."},
			{Body: "The right sidebar is now info-only — safe to hide."},
			{},
			{Label: "j / k", Body: "move cursor through properties"},
			{Label: "enter", Body: "edit / toggle / delete the cursored row"},
			{Label: "i", Body: "open the full sidebar panel in a modal"},
			{Label: "esc", Body: "back to fields list"},
			{Label: "r", Body: "refresh describe (pull latest metadata)"},
			{},
			{Body: "In an edit modal:"},
			{Label: "ctrl+s", Body: "save (also enter if single-line)"},
			{Label: "esc", Body: "cancel"},
			{Label: "ctrl+u", Body: "clear buffer"},
			{Label: "ctrl+w", Body: "delete last word"},
			{},
			{Body: "Edits are blocked client-side unless the org's safety"},
			{Body: "level is metadata or full. See ? on the orgs list."},
		},
	}
}

func helpValidationDetail() infoModalState {
	return infoModalState{
		Title: "Validation rule · actions",
		Rows: []infoRow{
			{Body: "Main pane lists the rule's Metadata. Editable rows (active,"},
			{Body: "error message, condition formula, description) + a DANGER ZONE"},
			{Body: "delete take the cursor. Sidebar is info-only."},
			{},
			{Label: "j / k", Body: "move cursor through properties"},
			{Label: "enter", Body: "edit / toggle / delete the cursored row"},
			{Label: "esc", Body: "back to rules list"},
			{Label: "r", Body: "refresh rule + list"},
			{},
			{Body: "Actions:"},
			{Label: "toggle active", Body: "enable or disable the rule"},
			{Label: "error message", Body: "the text shown to users"},
			{Label: "condition formula", Body: "true triggers the error"},
			{Label: "description", Body: "internal note for admins"},
			{},
			{Body: "Creating / deleting rules is a follow-up commit."},
			{Body: "Edits gated by safety ≥ metadata."},
		},
	}
}

func helpTriggerDetail() infoModalState {
	return infoModalState{
		Title: "Trigger · actions",
		Rows: []infoRow{
			{Body: "Main pane: a status row + the Apex body + a delete row."},
			{Body: "Tab swaps the row cursor (status/edit/delete) ↔ body scroll."},
			{Body: "Sidebar is info-only."},
			{},
			{Label: "j / k", Body: "move row cursor / scroll body"},
			{Label: "tab", Body: "swap between row cursor and body scroll"},
			{Label: "enter", Body: "toggle status / edit body / delete"},
			{Label: "esc", Body: "back to triggers list"},
			{Label: "r", Body: "refresh body + list"},
			{},
			{Body: "Actions:"},
			{Label: "toggle status", Body: "Active ↔ Inactive"},
			{Label: "edit body", Body: "edit Apex source (compiles on save)"},
			{Label: "delete", Body: "permanently remove (full safety)"},
			{},
			{Body: "Edits gated by safety ≥ metadata."},
		},
	}
}

func helpRecordTypeDetail() infoModalState {
	return infoModalState{
		Title: "Record type · actions",
		Rows: []infoRow{
			{Body: "Main pane lists the record type's Metadata. Editable rows"},
			{Body: "(active, label, description) + a DANGER ZONE delete take the"},
			{Body: "cursor. Sidebar is info-only."},
			{},
			{Label: "j / k", Body: "move cursor through properties"},
			{Label: "enter", Body: "edit / toggle / delete the cursored row"},
			{Label: "esc", Body: "back to record types list"},
			{Label: "r", Body: "refresh record type + list"},
			{},
			{Body: "Actions:"},
			{Label: "toggle active", Body: "enable or disable the record type"},
			{Label: "edit label", Body: "human label shown in pickers"},
			{Label: "edit description", Body: "internal note for admins"},
			{Label: "delete", Body: "permanently remove (full safety)"},
			{},
			{Body: "Edits gated by safety ≥ metadata."},
		},
	}
}

func helpObjectDetails() infoModalState {
	return infoModalState{
		Title: "Object detail · actions",
		Rows: []infoRow{
			{Body: "Main pane lists the object's metadata. Editable rows (label,"},
			{Body: "plural label, description, and the FEATURES toggles) take the"},
			{Body: "cursor; read-only rows are skipped-over. Sidebar is info-only."},
			{},
			{Label: "j / k", Body: "move cursor through properties"},
			{Label: "enter", Body: "edit / toggle the cursored row"},
			{Label: "i", Body: "open the full sidebar panel in a modal"},
			{Label: "tab/[]", Body: "cycle between Details / Schema / Records"},
			{Label: "esc", Body: "back to objects list"},
			{Label: "r", Body: "refresh describe"},
			{},
			{Body: "Standard objects (Account, Contact, …) have no"},
			{Body: "CustomObject row — object-level edits are blocked."},
			{Body: "Edit them via the Metadata API / Setup instead."},
			{},
			{Body: "Edits are blocked client-side unless the org's safety"},
			{Body: "level is metadata or full. See ? on the orgs list."},
		},
	}
}

func helpObjectFLS() infoModalState {
	return infoModalState{
		Title: "Field-Level Security · grid",
		Rows: []infoRow{
			{Body: "2D grid: rows are fields, columns are Read + Edit."},
			{Body: "Chip strip at the top picks the scope (profile or permset)."},
			{Body: "← / → cycles scope; j/k moves the field cursor."},
			{},
			{Label: "space", Body: "toggle Read on cursored field"},
			{Label: "e", Body: "toggle Edit (automatically sets Read)"},
			{Label: "← / →", Body: "cycle profile / permset"},
			{Label: "r", Body: "refresh FLS for current scope"},
			{Label: "esc", Body: "back to Objects"},
			{},
			{Body: "Rules:"},
			{Label: "Edit → Read", Body: "turning Edit on auto-sets Read"},
			{Label: "Read off", Body: "turning Read off also turns Edit off"},
			{Label: "both off", Body: "deletes the row (absent = off)"},
			{},
			{Body: "Writes are single POSTs/PATCHes — no deploy cycle."},
			{Body: "Edits gated by safety ≥ metadata."},
		},
	}
}

func helpRecordsLenses() infoModalState {
	return infoModalState{
		Title: "Records · views",
		Rows: []infoRow{
			{Body: "The strip at top lists saved views — each one is a query"},
			{Body: "(WHERE / ORDER / columns) you can switch into with one keystroke."},
			{Body: "Two sources coexist on the same surface:"},
			{},
			{Label: "sf-deck", Body: "your built-in + user-saved views (default)"},
			{Label: "Salesforce", Body: "the org's own ListView records"},
			{},
			{Label: "← / →", Body: "cycle through views in the active source"},
			{Label: "L", Body: "toggle source (sf-deck ↔ Salesforce)"},
			{Label: "V", Body: "manage views (new / edit / delete)"},
			{Label: "M", Body: "show non-favourite views (\"+ N more\")"},
			{Label: "r", Body: "refresh records under the current view"},
			{Label: "↵", Body: "drill into the highlighted record"},
			{Label: "o / ^o", Body: "open record (default / pick target)"},
			{},
			{Body: "Built-in views: Recent, Today, This week, Mine, Mine recent."},
			{Body: "User views are persisted to ~/.sf-deck/settings.toml."},
			{Body: ":userId in a WHERE clause expands to the active org's user id."},
		},
	}
}

// helpSafety explains the per-org safety levels and where to change
// them. Surfaced from /home and from the orgs utility.
func helpSafety() infoModalState {
	return infoModalState{
		Title: "Safety · per-org write policy",
		Rows: []infoRow{
			{Body: "sf-deck gates writes client-side. Salesforce still checks"},
			{Body: "profiles + FLS; this just stops the request before it fires."},
			{},
			{Label: "READ", Body: "read-only — no writes of any kind"},
			{Label: "REC", Body: "record DML allowed (insert/update/delete rows)"},
			{Label: "META", Body: "+ metadata changes (fields, objects, deploys)"},
			{Label: "FULL", Body: "+ execute-anonymous Apex & destructive ops"},
			{},
			{Body: "Defaults by org kind:"},
			{Body: "  production → READ    sandbox → REC"},
			{Body: "  scratch    → FULL    devhub  → REC"},
			{},
			{Body: "Override per-org in ~/.sf-deck/settings.toml:"},
			{Body: "  [orgs.\"your-username@example.com\"]"},
			{Body: "  safety = \"read_only\""},
		},
	}
}

func helpFieldsTable() infoModalState {
	return infoModalState{
		Title: "FIELDS · FLAGS column",
		Rows: []infoRow{
			{Body: "Each row's flags live at fixed positions so columns of letters"},
			{Body: "can be scanned at a glance. Inactive slots render as · (dim)."},
			{},
			{Label: "R", Body: "required (not nillable)"},
			{Label: "U", Body: "unique"},
			{Label: "X", Body: "external id"},
			{Label: "A", Body: "auto-populated (autonumber or calculated formula)"},
			{Label: "B", Body: "behavior modifier:"},
			{Body: "       E · encrypted"},
			{Body: "       P · restricted picklist"},
			{Body: "       C · case-sensitive"},
			{Body: "       D · cascade-delete"},
			{},
			{Body: "Full field metadata (including complete picklist values,"},
			{Body: "formula bodies, help text, and SOQL capabilities) lives in"},
			{Body: "the sidebar when a field is selected."},
		},
	}
}
