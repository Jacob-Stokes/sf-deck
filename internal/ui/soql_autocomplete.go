package ui

// SOQL autocomplete UI wrapper. Bridges the bubbletea SOQL editor
// to the pure-logic internal/soqlauto engine: snapshot per
// keystroke → classify → suggest → render popup → accept inserts
// into the textinput.
//
// Lifecycle:
//   - Editor enters edit mode → autocompleteState.Enabled = true.
//   - Every keystroke recomputes the suggestion slice (cached by
//     (query, cursor) memo key so idle ticks are free).
//   - Popup renders below the input line.
//   - Tab/Enter inserts the selected suggestion at the token slot.
//   - Esc dismisses the popup (keeps editor open).
//
// The state struct lives on soqlSession so the /soql tab and the
// SOQL modal each carry their own popup independently.

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/soqlauto"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func init() {
	// Wire the engine's NameFieldHint to the curated standard-sObject
	// registry so "FROM Task" expands to "SELECT Id, Subject FROM Task"
	// (not the INVALID_FIELD-throwing "Id, Name").
	soqlauto.NameFieldHint = sf.NameFieldFor
}

// autocompleteState is the per-session popup state. The popup
// renders unconditionally while the editor is open — Items may be
// empty (popup shows "no suggestions" + key hints) but the box
// height is always the same so the layout doesn't jump.
type autocompleteState struct {
	Enabled bool
	Items   []soqlauto.Suggestion
	Cursor  int
	Loading []string

	// MemoKey skips recompute when (query, cursor) hasn't changed
	// since the previous tick. Set by refresh().
	MemoKey string

	// Class captures the most recent classification so accept() can
	// reuse the token boundary computed there.
	Class soqlauto.Classification

	// Live distinct-value fetch state (Ctrl+Space on a text field
	// in WhereValue context). ValuesGen is bumped on each fetch
	// so in-flight results from prior fetches are dropped.
	ValuesGen     uint64
	ValuesLoading bool
	ValuesField   string
	ValuesValues  []string
	ValuesErr     error
	ValuesCancel  func()
}

// autocompleteRefresh recomputes suggestions for the active SOQL
// session. Called from the SOQL edit-key handler after each
// keystroke. Cheap on no-change (memo short-circuit).
func (m *Model) autocompleteRefresh(s *soqlSession) {
	if s == nil {
		return
	}
	if s.autocomplete == nil {
		s.autocomplete = &autocompleteState{Enabled: true}
	}
	ac := s.autocomplete
	if !ac.Enabled {
		ac.Items = nil
		return
	}

	query := s.soqlInput.Value()
	// textarea exposes cursor as (line, col) — flatten across the
	// newline-joined Value() to a byte offset for the engine's regex
	// math.
	byteCursor := textareaCursorByte(&s.soqlInput, query)

	key := autocompleteMemoKey(query, byteCursor)
	if key == ac.MemoKey {
		return
	}
	ac.MemoKey = key
	// Any post-fetch keystroke invalidates the in-flight live
	// values fetch. Bump gen + cancel ctx so the late result is
	// dropped by applyAutocompleteValues.
	if ac.ValuesCancel != nil {
		ac.ValuesCancel()
		ac.ValuesCancel = nil
	}
	ac.ValuesGen++
	ac.ValuesLoading = false

	d := m.activeOrgData()
	snap := m.buildAutocompleteSnapshot(d, query, byteCursor)
	cls := soqlauto.Classify(snap)
	items := soqlauto.Suggest(snap, &cls)
	ac.Class = cls
	ac.Items = items
	ac.Loading = cls.LoadingFor
	if ac.Cursor >= len(items) {
		ac.Cursor = 0
	}
}

// buildAutocompleteSnapshot wires the engine's lookup callbacks to
// the active org's describe + sObject caches.
func (m *Model) buildAutocompleteSnapshot(d *orgData, query string, byteCursor int) soqlauto.Snapshot {
	if d == nil {
		return soqlauto.Snapshot{Query: query, CursorPos: byteCursor, SelEnd: byteCursor}
	}
	alias := ""
	if len(m.orgs) > 0 && m.selected >= 0 && m.selected < len(m.orgs) {
		alias = targetArg(m.orgs[m.selected])
	}

	// sObject catalog — only the API names; queryable filter is
	// applied at suggestion time when we have it. If the catalog
	// hasn't been fetched yet, kick the fetch via the cmd queue;
	// the popup will populate on the next render after results land.
	var sobjects []string
	if d.SObjects.FetchedAt().IsZero() {
		if alias != "" && !d.SObjects.Busy() {
			m.queueAutocompleteCmd(d.SObjects.Ensure(m.cache))
		}
	} else {
		for _, s := range d.SObjects.Value() {
			sobjects = append(sobjects, s.Name)
		}
	}

	describes := func(sobject string) soqlauto.DescribeRef {
		r, ok := d.Describes[sobject]
		if !ok || r == nil {
			return soqlauto.DescribeRef{Status: soqlauto.StatusUnknown}
		}
		if r.FetchedAt().IsZero() {
			if r.Busy() {
				return soqlauto.DescribeRef{Status: soqlauto.StatusLoading}
			}
			return soqlauto.DescribeRef{Status: soqlauto.StatusUnknown}
		}
		v := r.Value()
		return soqlauto.DescribeRef{Status: soqlauto.StatusLoaded, Describe: &v}
	}

	// EnsureDescribe is fire-and-forget. We can't easily return a
	// tea.Cmd from here (the engine is sync), but the bubble-tea
	// model already has an EnsureDescribe path used elsewhere —
	// invoking it here just allocates the Resource; the actual
	// fetch fires when something asks Ensure to issue the cmd.
	// Without firing the cmd the describe never loads. So we
	// queue a cmd via the model's pending-cmd hook.
	ensure := func(sobject string) {
		if alias == "" || sobject == "" {
			return
		}
		r := d.EnsureDescribe(alias, sobject)
		if r == nil {
			return
		}
		m.queueAutocompleteCmd(r.Ensure(m.cache))
	}

	return soqlauto.Snapshot{
		Query:          query,
		CursorPos:      byteCursor,
		SelEnd:         byteCursor,
		Describes:      describes,
		EnsureDescribe: ensure,
		SObjects:       sobjects,
	}
}

// activeAutocompleteSession returns the SOQL session whose popup
// is currently visible (modal first, tab second), or nil when no
// popup is open. Used by the wheel dispatcher to redirect scroll
// events to the suggestion list when it's active.
func (m Model) activeAutocompleteSession() *soqlSession {
	if m.soqlModal != nil {
		s := &m.soqlModal.session
		if s.soqlEditing && s.autocomplete != nil && len(s.autocomplete.Items) > 0 {
			return s
		}
	}
	if m.tab() == TabSOQL && m.soqlEditing && m.soqlSession.autocomplete != nil && len(m.soqlSession.autocomplete.Items) > 0 {
		return &m.soqlSession
	}
	return nil
}

// autocompleteInvalidate clears the memo key on the active SOQL
// session(s) so the next render re-runs Classify+Suggest against
// fresh describe / sobject data. Called when a resource the
// autocomplete engine consumes lands (sobjects catalog, a describe).
//
// Both /soql tab and the SOQL modal carry their own session — we
// invalidate both because we can't cheaply tell which one is
// currently visible.
func (m *Model) autocompleteInvalidate() {
	if m.soqlSession.autocomplete != nil {
		m.soqlSession.autocomplete.MemoKey = ""
		// Pre-emptively repopulate so the next render shows the
		// fresh suggestions without waiting on a keystroke.
		m.autocompleteRefresh(&m.soqlSession)
	}
	if m.soqlModal != nil && m.soqlModal.session.autocomplete != nil {
		m.soqlModal.session.autocomplete.MemoKey = ""
		m.autocompleteRefresh(&m.soqlModal.session)
	}
}

// queueAutocompleteCmd buffers a tea.Cmd that should fire on the
// next return-to-bubbletea. The describe-ensure path needs a cmd
// channel; the edit-key handler reads + clears this buffer when it
// returns.
func (m *Model) queueAutocompleteCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	m.autocompletePending = append(m.autocompletePending, cmd)
}

// drainAutocompleteCmds returns the buffered cmds + clears the
// buffer. Called once per edit-key tick.
func (m *Model) drainAutocompleteCmds() tea.Cmd {
	if len(m.autocompletePending) == 0 {
		return nil
	}
	cmds := m.autocompletePending
	m.autocompletePending = nil
	return tea.Batch(cmds...)
}

// autocompleteKey handles popup-specific keys BEFORE the textinput
// gets them. Returns (consumed, cmd). Consumed=true means the key
// was handled by the popup and shouldn't reach the editor.
//
// Up/down navigation requires at least one suggestion. Tab/ctrl+space
// accept require a non-empty list. Esc dismisses ONLY the popup —
// the editor stays in edit mode (consistent with "popup is a hint
// overlay, not a separate input mode").
func (m *Model) autocompleteKey(s *soqlSession, key string) (bool, tea.Cmd) {
	if s == nil || s.autocomplete == nil || !s.soqlEditing {
		return false, nil
	}
	ac := s.autocomplete
	if len(ac.Items) == 0 {
		return false, nil
	}
	switch key {
	case "down":
		// Up/Down step suggestions, clamped at the ends. Ctrl+N/Ctrl+P
		// move the editor cursor between logical lines — bound via
		// the textarea's own KeyMap (CursorDown/CursorUp) in
		// newSOQLInput. Clamp (not wrap) so a trackpad burst lands
		// at the edge instead of cycling the popup for ages.
		if ac.Cursor < len(ac.Items)-1 {
			ac.Cursor++
		}
		return true, nil
	case "up":
		if ac.Cursor > 0 {
			ac.Cursor--
		}
		return true, nil
	case "tab":
		m.autocompleteAccept(s)
		return true, nil
	case "ctrl+@", "ctrl+space":
		// Ctrl+Space has two modes (matches Inspector Reloaded):
		//
		//   - SELECT field context: bulk-expand. Replace the
		//     current token with ALL matching field names
		//     comma-joined.
		//   - WhereValue / InWithValues on a text/reference field:
		//     fire a live SOQL distinct-value fetch to populate
		//     the popup with values that actually exist in the
		//     org.
		switch ac.Class.Context {
		case soqlauto.ContextAfterSelectKeyword:
			m.autocompleteBulkExpand(s)
		case soqlauto.ContextWhereValue, soqlauto.ContextInWithValues:
			// Pick the target based on whether we're in the
			// modal or the tab. Compare receiver pointer.
			target := soqlSessionTab
			if m.soqlModal != nil && s == &m.soqlModal.session {
				target = soqlSessionModal
			}
			if cmd := m.autocompleteFetchValues(s, target); cmd != nil {
				return true, cmd
			}
		}
		return true, nil
	case "esc":
		// Don't consume — let the caller's edit-mode esc handler
		// run (it exits edit mode, which is what the user wants
		// when they press esc with the popup visible).
		return false, nil
	}
	return false, nil
}

// autocompleteBulkExpand handles Ctrl+Space in SELECT context:
// replace the current token with every field-kind suggestion
// joined by ", ". Outside SELECT context, no-op (the user gets
// nothing — same as Inspector).
func (m *Model) autocompleteBulkExpand(s *soqlSession) {
	if s == nil || s.autocomplete == nil {
		return
	}
	ac := s.autocomplete
	if ac.Class.Context != soqlauto.ContextAfterSelectKeyword {
		return
	}
	var names []string
	for _, sug := range ac.Items {
		if sug.Kind == soqlauto.KindField {
			names = append(names, sug.Value)
		}
	}
	if len(names) == 0 {
		return
	}
	query := s.soqlInput.Value()
	byteCursor := textareaCursorByte(&s.soqlInput, query)
	tokenLen := len(ac.Class.SearchToken)
	tokenStart := byteCursor - tokenLen
	if tokenStart < 0 {
		tokenStart = 0
	}
	inserted := strings.Join(names, ", ")
	newQuery := query[:tokenStart] + inserted + query[byteCursor:]
	s.soqlInput.SetValue(newQuery)
	textareaSetCursorByte(&s.soqlInput, newQuery, tokenStart+len(inserted))
	ac.MemoKey = ""
}

// autocompleteAccept inserts the cursored suggestion at the token
// boundary in the soqlInput buffer. Recomputes the popup on the
// next refresh (which the caller triggers by re-running the
// edit-key path).
func (m *Model) autocompleteAccept(s *soqlSession) {
	if s == nil || s.autocomplete == nil {
		return
	}
	ac := s.autocomplete
	if ac.Cursor < 0 || ac.Cursor >= len(ac.Items) {
		return
	}
	sug := ac.Items[ac.Cursor]
	query := s.soqlInput.Value()
	byteCursor := textareaCursorByte(&s.soqlInput, query)

	// Token boundary: cursor minus the length of the search token
	// (which the engine already extracted into Classification).
	tokenLen := len(ac.Class.SearchToken)
	tokenStart := byteCursor - tokenLen
	if tokenStart < 0 {
		tokenStart = 0
	}

	inserted := sug.Value + sug.Suffix
	newQuery := query[:tokenStart] + inserted + query[byteCursor:]
	s.soqlInput.SetValue(newQuery)

	// Move cursor to right after the inserted text. Position by
	// re-deriving line/col from the new byte offset.
	textareaSetCursorByte(&s.soqlInput, newQuery, tokenStart+len(inserted))

	// Force refresh on the next call.
	ac.MemoKey = ""
}

// renderAutocompletePopup builds the popup block as a FIXED-HEIGHT
// block of lines spliced into the surrounding renderer. Always
// renders while the editor is open — empty slots pad with blanks
// so the layout never jumps as suggestions appear/disappear.
//
// width is the inner width of the surrounding container.
func renderAutocompletePopup(ac *autocompleteState, width, maxRows int) []string {
	if ac == nil {
		return nil
	}
	if maxRows < 3 {
		maxRows = 8
	}
	const popupPad = 4 // borders + padding
	popupW := width - 4
	if popupW < 30 {
		popupW = 30
	}
	rowW := popupW - popupPad

	rows := make([]string, 0, maxRows+2)

	// Slide window so the cursor stays visible.
	start := 0
	end := len(ac.Items)
	if end > maxRows {
		end = maxRows
	}
	if ac.Cursor >= maxRows {
		start = ac.Cursor - maxRows + 1
		end = start + maxRows
		if end > len(ac.Items) {
			end = len(ac.Items)
		}
	}
	for i := start; i < end; i++ {
		s := ac.Items[i]
		rows = append(rows, renderAutocompleteRow(s, i == ac.Cursor, rowW))
	}

	// Empty-state hint occupies the first row when there are no
	// suggestions. Keeps the box from looking broken.
	if len(rows) == 0 {
		hint := "  no suggestions — keep typing or press esc to dismiss"
		if len(ac.Loading) > 0 {
			hint = "  loading " + strings.Join(uniqueStrings(ac.Loading), ", ") + " metadata…"
		}
		rows = append(rows, lipgloss.NewStyle().Foreground(theme.FgDim).Italic(true).Render(hint))
	}

	// Pad remaining rows with blanks so the box height is stable.
	for len(rows) < maxRows {
		rows = append(rows, "")
	}

	// Loading footer (italic yellow) — used for both describe-load
	// hops and live-values fetch.
	switch {
	case ac.ValuesLoading:
		rows = append(rows, lipgloss.NewStyle().Foreground(theme.Yellow).Italic(true).
			Render("  fetching live values for "+ac.ValuesField+"…"))
	case len(ac.Loading) > 0 && len(ac.Items) > 0:
		rows = append(rows, lipgloss.NewStyle().Foreground(theme.Yellow).Italic(true).
			Render("  loading "+strings.Join(uniqueStrings(ac.Loading), ", ")+" metadata…"))
	default:
		rows = append(rows, "") // keep slot height stable
	}

	// Footer with key hints — always rendered.
	footer := "  ↑/↓ cycle · tab accept · ctrl+space expand"
	if len(ac.Items) > maxRows {
		footer = "  ↑/↓ cycle (" + itoaShort(ac.Cursor+1) + "/" + itoaShort(len(ac.Items)) + ") · tab accept · ctrl+space expand"
	}
	rows = append(rows, lipgloss.NewStyle().Foreground(theme.FgDim).Render(footer))

	body := strings.Join(rows, "\n")
	// Double border in the main panel colour — distinct from the
	// single-rounded chrome everywhere else (record-drill uses
	// magenta thick, global search uses cyan double already) so
	// the popup reads as "important live overlay" without
	// requiring a new colour.
	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Width(popupW).
		Render(body)
	return strings.Split(box, "\n")
}

// renderAutocompleteRow formats one suggestion line: cursor bar,
// kind-tinted display text, dim detail. Truncates to width.
func renderAutocompleteRow(s soqlauto.Suggestion, cursor bool, width int) string {
	prefix := "  "
	displayStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	if cursor {
		// BorderHi bar against the dim frame gives a strong
		// "you are here" cue without changing the popup's overall
		// tone — same affordance as listtable row cursors.
		prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " "
		displayStyle = displayStyle.Bold(true).Background(theme.BgAlt)
	}
	displayStyle = applyKindColor(displayStyle, s.Kind, cursor)

	detailStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	if cursor {
		detailStyle = detailStyle.Background(theme.BgAlt)
	}

	display := s.Display
	detail := s.Detail
	maxDisplay := width / 2
	if maxDisplay < 12 {
		maxDisplay = 12
	}
	if len(display) > maxDisplay {
		display = ansi.Truncate(display, maxDisplay, "…")
	}
	gap := "  "
	remaining := width - len(display) - len(gap) - len(prefix)
	if remaining < 0 {
		remaining = 0
	}
	if len(detail) > remaining && remaining > 1 {
		detail = ansi.Truncate(detail, remaining, "…")
	}
	gapStyle := lipgloss.NewStyle()
	if cursor {
		gapStyle = gapStyle.Background(theme.BgAlt)
	}
	body := prefix + displayStyle.Render(display) + gapStyle.Render(gap) + detailStyle.Render(detail)
	if cursor {
		// Pad to width with BgAlt so the highlight bar runs to the
		// right edge.
		body = lipgloss.NewStyle().Width(width).Background(theme.BgAlt).Render(body)
	}
	return body
}

// applyKindColor tints the display text by suggestion kind so
// users learn the vocabulary at a glance (relationships green,
// keywords cyan, picklist values yellow, etc.).
func applyKindColor(base lipgloss.Style, kind soqlauto.SuggestionKind, cursor bool) lipgloss.Style {
	var styled lipgloss.Style
	switch kind {
	case soqlauto.KindRelationship:
		styled = base.Foreground(theme.Green)
	case soqlauto.KindSObject:
		styled = base.Foreground(theme.Cyan)
	case soqlauto.KindKeyword, soqlauto.KindFunction:
		styled = base.Foreground(theme.Blue)
	case soqlauto.KindPicklist, soqlauto.KindDateLiteral, soqlauto.KindBoolean:
		styled = base.Foreground(theme.Yellow)
	default:
		return base
	}
	if cursor {
		return styled.Bold(true)
	}
	return styled
}

// textareaCursorByte flattens the textarea's (line, column) cursor
// into a byte offset across the newline-joined Value() string.
// Lines are joined with `\n` (textarea's own separator). The Column
// returned by bubbles/textarea is a RUNE index within the active
// line — not a byte index — so we walk the line decoding runes to
// land on the right byte.
func textareaCursorByte(ta *textareaModel, value string) int {
	line := ta.Line()
	col := ta.Column()
	// Walk newline-joined lines: sum each prior line's byte length
	// (+1 for the joining \n) until we reach the cursor's line.
	off := 0
	lineIdx := 0
	for lineIdx < line && off < len(value) {
		nl := strings.IndexByte(value[off:], '\n')
		if nl < 0 {
			off = len(value)
			break
		}
		off += nl + 1
		lineIdx++
	}
	// Add the rune-col offset into the current line.
	off += runeIndexToByte(value[off:], col)
	if off > len(value) {
		off = len(value)
	}
	return off
}

// textareaSetCursorByte positions the textarea cursor at the given
// byte offset of the post-edit Value(). Walks newlines to derive
// (line, col) and then uses CursorDown/Up + SetCursorColumn to land
// there. col passed to SetCursorColumn is a rune index — convert
// from byte offset within the line.
func textareaSetCursorByte(ta *textareaModel, value string, byteOff int) {
	if byteOff < 0 {
		byteOff = 0
	}
	if byteOff > len(value) {
		byteOff = len(value)
	}
	// Find which line + within-line byte offset.
	line := 0
	lineStart := 0
	for i := 0; i < byteOff; i++ {
		if value[i] == '\n' {
			line++
			lineStart = i + 1
		}
	}
	withinByte := byteOff - lineStart
	// Determine the matching rune-column.
	col := 0
	for i := 0; i < withinByte; {
		_, sz := decodeRune(value[lineStart+i:])
		if sz <= 0 {
			break
		}
		i += sz
		col++
	}
	// Walk vertically: textarea exposes CursorDown/CursorUp.
	currentLine := ta.Line()
	for currentLine < line {
		ta.CursorDown()
		currentLine++
	}
	for currentLine > line {
		ta.CursorUp()
		currentLine--
	}
	ta.SetCursorColumn(col)
}

// textareaModel is a tiny re-export of the textarea pointer-receiver
// shape so the helpers above can live in this file without needing
// a direct charm.land import.
type textareaModel = textarea.Model

// runeIndexToByte converts a rune-indexed cursor position (what
// bubbles/textinput uses) to a byte offset (what the engine's
// regex math needs). For ASCII queries the two are identical.
func runeIndexToByte(s string, runeIdx int) int {
	if runeIdx <= 0 {
		return 0
	}
	i, count := 0, 0
	for i < len(s) {
		if count == runeIdx {
			return i
		}
		_, size := decodeRune(s[i:])
		i += size
		count++
	}
	return i
}

// decodeRune is a tiny wrapper around utf8.DecodeRuneInString so
// the file doesn't need an extra import.
func decodeRune(s string) (r rune, size int) {
	if len(s) == 0 {
		return 0, 0
	}
	b := s[0]
	switch {
	case b < 0x80:
		return rune(b), 1
	case b < 0xC0:
		return rune(b), 1
	case b < 0xE0:
		if len(s) < 2 {
			return rune(b), 1
		}
		return rune(b&0x1F)<<6 | rune(s[1]&0x3F), 2
	case b < 0xF0:
		if len(s) < 3 {
			return rune(b), 1
		}
		return rune(b&0x0F)<<12 | rune(s[1]&0x3F)<<6 | rune(s[2]&0x3F), 3
	default:
		if len(s) < 4 {
			return rune(b), 1
		}
		return rune(b&0x07)<<18 | rune(s[1]&0x3F)<<12 | rune(s[2]&0x3F)<<6 | rune(s[3]&0x3F), 4
	}
}

// autocompleteMemoKey is the cheap dedup key for refresh().
func autocompleteMemoKey(query string, cursor int) string {
	return query + "\x00" + itoaShort(cursor)
}

// itoaShort is strconv.Itoa without the import cost.
func itoaShort(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// uniqueStrings dedupes while preserving order.
func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
