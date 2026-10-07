package ui

// Chip wizard — multi-field form modal that produces a query.Query
// directly. Replaces the old filter wizard (which authored a flat
// filter.Spec).

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/services/chips"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

type cwField struct {
	Field    string
	Label    string
	Hint     string
	Op       query.Op
	Kind     cwKind
	input    textinput.Model
	triValue *bool
}

type cwKind int

const (
	cwText cwKind = iota
	cwInt
	cwTri
	cwDate
	cwLimit
)

type chipWizardState struct {
	Title  string
	Domain chipDomain // which surface this chip targets
	Scope  string

	heightFloor int

	existingID                                         string
	existingLbl                                        string
	existingOrigin                                     qchip.Origin
	existingSrcID, existingSrcName, existingImportedAt string
	existingFavourite                                  bool
	// existingOrgUser preserves the legacy per-org scope across edits so
	// a chip authored in org A doesn't silently get re-stamped to org B
	// if the user happens to be on B when they Save the edit. Superseded
	// by Share for chips authored after the ChipShare migration.
	existingOrgUser string

	// Share is the chip's cross-org visibility (org/orgs/group/global),
	// edited via the scope chooser launched from the wizard's "Scope: …"
	// row. Zero on first open for legacy chips (we display
	// existingOrgUser as a single-org share in that case). Stored on
	// disk as ChipConfig.Share; OrgUser is migrated out at write time.
	Share settings.ChipShare

	// catalogue is the full list of fields the picker offers — every
	// filterable field on the target sObject for records, or the
	// static per-domain list for objects/flows. The wizard never
	// renders this directly; it's the source the field picker draws
	// from when the user adds a criterion.
	catalogue []cwField

	criteria []cwField
	Cursor   int

	Advanced     bool
	advancedText textinput.Model

	// advancedLockReason is set when the underlying AST can't be
	// represented in simple mode (uses OR / NOT / nested groups).
	// Surfaces in the mode line so the user knows why ctrl+t is
	// refusing to switch back to simple.
	advancedLockReason string

	// modeLocked freezes the simple↔advanced toggle. New chips are
	// unlocked until first save (so users can pick the mode they
	// want); existing chips are always locked because round-tripping
	// non-trivial SOQL through simple mode loses info silently. ctrl+t
	// is a no-op while this is true.
	modeLocked bool

	labelInput textinput.Model

	Saving bool
	Err    string
}

// openChipWizard builds the wizard state for a new or existing chip.
// `existing` is the chip being edited (zero-value for new). Per-domain
// catalogue picks the right rows.
func (m *Model) openChipWizard(d chipDomain, existing qchip.Chip) tea.Cmd {
	scope := valueOr(existing.Scope, chipScopeFor(m, d))
	state := &chipWizardState{
		Title:              wizardTitleFor(d, existing),
		Domain:             d,
		Scope:              scope,
		existingID:         existing.ID,
		existingLbl:        existing.Label,
		existingOrigin:     existing.Origin,
		existingSrcID:      existing.SourceID,
		existingSrcName:    existing.SourceName,
		existingImportedAt: existing.ImportedAt,
		existingFavourite:  existing.Favourite,
		existingOrgUser:    existing.OrgUser,
		Share:              chipWizardInitialShare(m, existing),
		catalogue:          m.wizardFieldsFor(d, scope),
		// Existing chips lock their mode immediately — toggling
		// would silently lose AST shapes simple mode can't
		// express. New chips stay unlocked until first save so the
		// user can flip while building.
		modeLocked: existing.ID != "",
	}

	advanced, prefill, reason := splitForWizard(existing.Query.Where)
	state.Advanced = advanced
	if advanced && reason != "" {
		state.advancedLockReason = reason
	}
	if !advanced {
		state.criteria = criteriaFromCompares(state.catalogue, prefill)
	}
	if limitRow := wizardLimitRow(state.catalogue, existing.Query.Limit); limitRow != nil {
		state.criteria = append(state.criteria, *limitRow)
	}

	state.labelInput = newWizardInput(existing.Label)
	advancedSeed := ""
	if hasMeaningfulWhere(existing.Query.Where) || len(existing.Query.OrderBy) > 0 || existing.Query.Limit > 0 {
		seed := existing.Query
		if seed.Limit <= 0 {
			seed.Limit = 0 // ToSOQLClauses skips zero
		}
		advancedSeed = query.ToSOQLClauses(seed)
	}
	state.advancedText = newWizardInput(advancedSeed)

	if existing.ID == "" {
		state.Cursor = -1
		state.labelInput.Focus()
	} else {
		state.Cursor = len(state.criteria)
	}

	m.chipWizard = state
	return nil
}

// openCriterionFieldPicker opens the anchored field picker. The user
// types to filter the catalogue, hits enter to pick a field; the
// picker's OnPick adds a new criterion row (initialised with the
// catalogue entry's default Op + Kind) and focuses it so the user
// can immediately type the value.
//
// Anchor is the screen cell where the "+ Add criterion…" row sits.
// We compute it loosely — modal width × position-on-screen — and the
// picker's overlay clamps it to fit on screen.
func (m *Model) openCriterionFieldPicker() tea.Cmd {
	st := m.chipWizard
	if st == nil {
		return nil
	}
	if len(st.catalogue) == 0 {
		// Never no-op silently: a domain without a field catalogue
		// should say so rather than eat the keypress/click.
		st.Err = "this view type has no filterable fields yet — use SOQL mode (ctrl+t)"
		return nil
	}
	// Anchor the picker roughly under the wizard's add-row. The
	// wizard renders centered; we estimate the row's screen Y as
	// "centre top + label row + criteria + add row". A close-enough
	// approximation; the frame compositor clamps the picker to fit.
	wW := modalWidth(m.width, 80, 160)
	wX := (m.width - wW) / 2
	pickerW := wW * 2 / 3
	if pickerW < 48 {
		pickerW = 48
	}
	if pickerW > m.width-4 {
		pickerW = m.width - 4
	}
	anchorX := wX + 4
	anchorY := (m.height / 2) + 2

	// Drop the sentinel Limit row from the picker — it's not a
	// criterion, it always sits at the end of st.criteria as the
	// "Limit" pin, and exposing it here would let users add a
	// duplicate.
	pickerItems := make([]cwField, 0, len(st.catalogue))
	for _, c := range st.catalogue {
		if c.Field == chipLimitSentinel {
			continue
		}
		pickerItems = append(pickerItems, c)
	}
	return openPicker(m, pickerSpec[cwField]{
		Title:       "Add criterion · pick a field",
		Items:       pickerItems,
		Width:       pickerW,
		AnchorX:     anchorX,
		AnchorY:     anchorY,
		Placeholder: "type to filter…",
		Match: func(f cwField, q string) bool {
			lq := strings.ToLower(q)
			return strings.Contains(strings.ToLower(f.Field), lq) ||
				strings.Contains(strings.ToLower(f.Label), lq) ||
				strings.Contains(strings.ToLower(f.Hint), lq)
		},
		RenderRow: func(f cwField, focused bool) string {
			label := f.Label
			if label == "" {
				label = f.Field
			}
			line := "  " + label
			if focused {
				line = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " " +
					lipgloss.NewStyle().Foreground(theme.Fg).Bold(true).Render(label)
			}
			meta := lipgloss.NewStyle().Foreground(theme.FgDim).Render(
				"  " + f.Field + " · " + opLabelFor(f.Op))
			return line + meta
		},
		OnPick: func(f cwField) tea.Cmd {
			return func() tea.Msg { return criterionPickedMsg{field: f} }
		},
	})
}

type criterionPickedMsg struct {
	field cwField
}

func (m Model) applyCriterionPicked(msg criterionPickedMsg) (Model, tea.Cmd) {
	st := m.chipWizard
	if st == nil {
		return m, nil
	}
	row := msg.field
	if row.Kind != cwTri {
		row.input = newWizardInput("")
	}
	st.criteria = append(st.criteria, row)
	st.Cursor = len(st.criteria) - 1
	st.focusCursorField()
	return m, nil
}

func sectionHeading(label string) string {
	return lipgloss.NewStyle().Foreground(theme.FgDim).Bold(true).Render(strings.ToUpper(label))
}

func opLabelFor(op query.Op) string {
	switch op {
	case query.OpEq:
		return "equals"
	case query.OpNotEq:
		return "≠"
	case query.OpContains:
		return "contains"
	case query.OpStartsWith:
		return "starts with"
	case query.OpEndsWith:
		return "ends with"
	case query.OpIn:
		return "in"
	case query.OpGT:
		return ">"
	case query.OpGTE:
		return "≥"
	case query.OpLT:
		return "<"
	case query.OpLTE:
		return "≤"
	case query.OpIsNull:
		return "is null"
	case query.OpDateLiteral:
		return "is"
	}
	return string(op)
}

func wizardLimitRow(catalogue []cwField, existingLimit int) *cwField {
	for _, c := range catalogue {
		if c.Field != chipLimitSentinel {
			continue
		}
		row := c
		manual := existingLimit != 0
		t := manual
		row.triValue = &t
		seed := ""
		if existingLimit > 0 {
			seed = intToString(existingLimit)
		}
		row.input = newWizardInput(seed)
		return &row
	}
	return nil
}

func criteriaFromCompares(catalogue []cwField, cmps []query.CompareNode) []cwField {
	out := make([]cwField, 0, len(cmps))
	for _, c := range cmps {
		row := catalogueLookup(catalogue, c.Field, c.Op)
		if row == nil {
			row = &cwField{
				Field: c.Field,
				Label: c.Field,
				Op:    c.Op,
				Kind:  cwText,
			}
		}
		fresh := *row
		switch fresh.Kind {
		case cwTri:
			if b, ok := c.Value.(bool); ok {
				fresh.triValue = &b
			}
		case cwInt:
			if n, ok := c.Value.(int); ok {
				fresh.input = newWizardInput(intToString(n))
			} else {
				fresh.input = newWizardInput("")
			}
		default:
			if s, ok := c.Value.(string); ok {
				fresh.input = newWizardInput(s)
			} else {
				fresh.input = newWizardInput("")
			}
		}
		out = append(out, fresh)
	}
	return out
}

func catalogueLookup(catalogue []cwField, field string, op query.Op) *cwField {
	for i := range catalogue {
		if catalogue[i].Field == field && catalogue[i].Op == op {
			return &catalogue[i]
		}
	}
	for i := range catalogue {
		if catalogue[i].Field == field {
			return &catalogue[i]
		}
	}
	return nil
}

// intToString is a tiny helper to avoid importing strconv in the hot
// path — same shape as itoa elsewhere in the package.
func intToString(n int) string {
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

func styleWizardInput(ti *textinput.Model) {
	s := ti.Styles()
	s.Focused.Text = lipgloss.NewStyle().Foreground(theme.Fg)
	s.Focused.Placeholder = lipgloss.NewStyle().Foreground(theme.FgDim)
	s.Blurred.Text = lipgloss.NewStyle().Foreground(theme.Fg)
	s.Blurred.Placeholder = lipgloss.NewStyle().Foreground(theme.FgDim)
	s.Cursor.Color = theme.BorderHi
	ti.SetStyles(s)
}

func newWizardInput(initial string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 0
	styleWizardInput(&ti)
	if initial != "" {
		ti.SetValue(initial)
		ti.CursorEnd()
	}
	return ti
}

// renderChipWizardLayers renders the wizard and, alongside, one hit
// layer per focusable row so the modal is clickable. Each layer
// repaints the row's own rendered line (idempotent) carrying a
// wizrow zone id; render.go attaches them as children of the modal
// layer at the modalBox content offset (border+padding = x+2, y+1).
func (m Model) renderChipWizardLayers() (string, []*lipgloss.Layer) {
	if m.chipWizard == nil {
		return "", nil
	}
	w := modalWidth(m.width, 80, 160)
	inner := w - 4
	st := m.chipWizard

	titleStyle := lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true)
	subStyle := lipgloss.NewStyle().Foreground(theme.FgDim)

	var lines []string
	type hitRow struct{ cursor, line int }
	var hits []hitRow
	maxHintLines, curHintLines := 0, 0
	lines = append(lines, titleStyle.Render(st.Title))

	mode := "Form"
	if st.Advanced {
		mode = "SOQL"
	}
	var modeLine string
	switch {
	case st.modeLocked:
		modeLine = mode + " mode  ·  locked after first save"
	case st.Advanced && st.advancedLockReason != "":
		modeLine = mode + " mode  ·  locked (" + st.advancedLockReason + ")"
	default:
		modeLine = mode + " mode  ·  ctrl+t to switch"
	}
	lines = append(lines, subStyle.Render(modeLine))
	lines = append(lines, strings.Repeat("─", inner))
	lines = append(lines, "")

	const labelCol = 16
	labelFocused := st.Cursor == -1
	prefix := "  "
	if labelFocused {
		prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " "
	}
	st.labelInput.SetWidth(inner - labelCol - 2)
	hits = append(hits, hitRow{cursor: -1, line: len(lines)})
	lines = append(lines, prefix+padRight("Name", labelCol-2)+"["+padRight(st.labelInput.View(), inner-labelCol-2)+"]")

	scopeSummary := chipWizardShareSummary(m, st.Share)
	scopeHint := lipgloss.NewStyle().Foreground(theme.BorderHi).Render("S") +
		subStyle.Render(" to change")
	lines = append(lines, "  "+padRight("Scope", labelCol-2)+
		subStyle.Render(scopeSummary)+"  ["+scopeHint+"]")
	for _, detail := range chipWizardShareDetailLines(m, st.Share) {
		lines = append(lines, "  "+padRight("", labelCol-2)+subStyle.Render(detail))
	}

	if st.Advanced {
		lines = append(lines, "")
		lines = append(lines, sectionHeading("SOQL"))
		lines = append(lines, subStyle.Render("Write WHERE, ORDER BY, and LIMIT clauses. No LIMIT means unbounded."))
		lines = append(lines, "")
		st.advancedText.SetWidth(inner - 2)
		focused := st.Cursor == 0
		prefix := "  "
		if focused {
			prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " "
		}
		lines = append(lines, prefix+st.advancedText.View())
		lines = append(lines, "")
		lines = append(lines, sectionHeading("Examples"))
		lines = append(lines, subStyle.Render("  WHERE Status = 'Active' AND ApiVersion >= 60"))
		lines = append(lines, subStyle.Render("  WHERE CreatedDate = LAST_N_DAYS:30 AND OwnerId = $userId"))
		lines = append(lines, subStyle.Render("  WHERE ProcessType IN ('Flow', 'AutoLaunchedFlow') ORDER BY Label LIMIT 200"))
	} else {
		lines = append(lines, "")
		lines = append(lines, sectionHeading("Filters"))
		valueCol := inner - labelCol
		for _, f := range st.criteria {
			if n := len(m.wizardHintLines(st, f, inner-labelCol)); n > maxHintLines {
				maxHintLines = n
			}
		}
		for i, f := range st.criteria {
			criterionHit := hitRow{cursor: i, line: -1}
			focused := i == st.Cursor
			prefix := "  "
			labelStyle := lipgloss.NewStyle().Foreground(theme.Fg)
			if focused {
				prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " "
				labelStyle = labelStyle.Bold(true)
			}
			label := labelStyle.Render(padRight(f.Label, labelCol-2))
			var value string
			switch f.Kind {
			case cwText, cwInt, cwDate:
				f.input.SetWidth(valueCol - 4) // -4 for "[ ]" + the X glyph
				val := f.input.View()
				if !focused {
					v := f.input.Value()
					if v == "" {
						v = subStyle.Render("—")
					}
					val = v
				}
				value = "[" + padRight(val, valueCol-4) + "]"
			case cwTri:
				var s string
				switch {
				case f.triValue == nil:
					s = "any"
				case *f.triValue:
					s = "yes"
				default:
					s = "no"
				}
				if focused {
					s = lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true).Render(s)
				}
				value = "(" + s + ")  " + subStyle.Render("space cycles")
			case cwLimit:
				manual := f.triValue != nil && *f.triValue
				modeLabel := "default"
				if manual {
					modeLabel = "custom"
				}
				modeStyle := subStyle
				if focused {
					modeStyle = lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true)
				}
				modeSeg := "[" + modeStyle.Render(modeLabel) + "] "
				var inputSeg string
				if manual {
					f.input.SetWidth(valueCol - 14)
					val := f.input.View()
					if !focused {
						v := f.input.Value()
						if v == "" {
							v = subStyle.Render("no limit")
						}
						val = v
					} else if f.input.Value() == "" {
						val = f.input.View() + "  " + subStyle.Render("blank = no limit")
					}
					inputSeg = "[" + padRight(val, valueCol-14) + "]"
				} else {
					def := 0
					if m.settings != nil {
						def = m.settings.DefaultChipLimit()
					}
					inputSeg = "[" + padRight(subStyle.Render("inherits global · "+intToString(def)), valueCol-14) + "]"
				}
				value = modeSeg + inputSeg
			}
			delGlyph := " "
			if focused {
				delGlyph = subStyle.Render("✕")
			}
			criterionHit.line = len(lines)
			hits = append(hits, criterionHit)
			lines = append(lines, prefix+label+value+" "+delGlyph)
			if focused {
				for _, hl := range m.wizardHintLines(st, f, inner-labelCol) {
					curHintLines++
					lines = append(lines, padRight("", labelCol)+subStyle.Render(hl))
				}
			}
		}

		addFocused := st.Cursor == len(st.criteria)
		if len(st.criteria) == 0 && !addFocused {
			lines = append(lines, subStyle.Italic(true).Render("    no filters yet — pick a field below"))
		}

		lines = append(lines, "")
		addPrefix := "  "
		addStyle := subStyle
		addLabel := "+ Add filter…"
		if addFocused {
			addPrefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " "
			addStyle = lipgloss.NewStyle().Foreground(theme.Green).Bold(true)
			addLabel = "+ Add filter  ·  press enter to pick a field"
		}
		hits = append(hits, hitRow{cursor: len(st.criteria), line: len(lines)})
		lines = append(lines, addPrefix+addStyle.Render(addLabel))
	}

	lines = append(lines, "")
	if st.Err != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Red).Render("error: "+st.Err))
	}
	if st.Saving {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Yellow).Render("saving…"))
	}
	moveHint := "tab to move · " + firstPretty(Keys.ChipWizardLookup) + " to look up values · " +
		firstPretty(Keys.ChipWizardDelete) + " to delete the focused row"
	lifeHint := "S change scope · " + firstPretty(Keys.ChipWizardMode) + " to switch mode · " +
		firstPretty(Keys.ChipWizardSave) + " to save · esc to cancel"
	if st.modeLocked {
		lifeHint = "S change scope · " + firstPretty(Keys.ChipWizardSave) + " to save · esc to cancel"
	}
	lines = append(lines, subStyle.Render(moveHint))
	lines = append(lines, subStyle.Render(lifeHint))

	for i := range lines {
		if lipgloss.Width(lines[i]) > inner {
			lines[i] = ansi.Truncate(lines[i], inner, "…")
		}
	}

	// Stable height. Three pieces:
	//   1. (maxHintLines - curHintLines) — reserve the tallest
	//      possible focused-row hint so focus moves between rows
	//      with different (or no) hints never change the total.
	//   2. emptyStateReserve — the "no filters yet" line hides while
	//      the add-row is focused; reserve its slot symmetrically.
	//   3. A session high-water floor as the backstop.
	// Padding inserts ABOVE the two footer hint lines so they stay
	// pinned to the bottom.
	target := len(lines) + (maxHintLines - curHintLines)
	if len(st.criteria) == 0 && st.Cursor == len(st.criteria) {
		target++ // hidden empty-state line
	}
	if target < st.heightFloor {
		target = st.heightFloor
	}
	st.heightFloor = target
	if pad := target - len(lines); pad > 0 {
		footer := lines[len(lines)-2:]
		lines = lines[:len(lines)-2]
		for i := 0; i < pad; i++ {
			lines = append(lines, "")
		}
		lines = append(lines, footer...)
	}

	box := modalBox(strings.Join(lines, "\n"), w)

	boxRows := strings.Split(box, "\n")
	layers := make([]*lipgloss.Layer, 0, len(hits))
	next := 0
	for _, h := range hits {
		if h.line < 0 || h.line >= len(lines) {
			continue
		}
		needle := strings.TrimSpace(ansi.Strip(lines[h.line]))
		if needle == "" {
			continue
		}
		if len(needle) > 24 {
			needle = needle[:24]
		}
		for r := next; r < len(boxRows); r++ {
			if strings.Contains(ansi.Strip(boxRows[r]), needle) {
				layers = append(layers,
					lipgloss.NewLayer(boxRows[r]).X(0).Y(r).Z(21).ID(zoneChipWizardRowID(h.cursor)))
				next = r + 1
				break
			}
		}
	}
	return box, layers
}

func (m Model) clickChipWizardRow(cursor int) (tea.Model, tea.Cmd) {
	st := m.chipWizard
	if st == nil || st.Saving {
		return m, nil
	}
	if cursor < -1 || cursor > len(st.criteria) {
		return m, nil
	}
	st.labelInput.Blur()
	st.advancedText.Blur()
	if st.Cursor >= 0 && st.Cursor < len(st.criteria) {
		st.criteria[st.Cursor].input.Blur()
	}
	st.Cursor = cursor
	if cursor == -1 {
		st.labelInput.Focus()
		return m, nil
	}
	if cursor == len(st.criteria) {
		return m, (&m).openCriterionFieldPicker()
	}
	st.focusCursorField()
	return m, nil
}

func (m Model) handleChipWizardKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.chipWizard == nil {
		return m, nil
	}
	st := m.chipWizard
	if st.Saving {
		return m, nil
	}
	key := msg.String()
	switch {
	case key == "esc" || key == "ctrl+c":
		m.chipWizard = nil
		return m, nil
	case matches(key, Keys.ChipWizardSave):
		return m.submitChipWizard()
	case matches(key, Keys.ChipWizardMode):
		// Mode toggle. Locked once a chip has been saved — round-
		// tripping non-trivial SOQL between simple and advanced
		// silently drops AST shapes the simple form can't express
		// (OR, NOT, nested groups). New chips stay flippable until
		// first save so authors can pick what fits.
		if st.modeLocked {
			st.Err = "mode is locked once the chip is saved"
			return m, nil
		}
		st.toggleMode()
		return m, nil
	case matches(key, Keys.ChipWizardLookup), key == "ctrl+ ", key == "ctrl+@":
		return m, m.openValuePicker()
	case key == "S" && !st.textInputFocused():
		// Capital-S opens the cross-org scope chooser — but ONLY when a
		// text input isn't focused. Inside the label / advanced-SOQL / a
		// text-criterion buffer, S is a literal character the user is
		// typing (a view named "Carrier Operations" must not fire the scope
		// chooser). The chooser updates st.Share in-place; the wizard
		// re-renders the "Scope: …" row on next paint.
		return m, (&m).chipWizardOpenScopeChooser()
	}

	if st.Cursor == -1 {
		switch key {
		case "tab", "down":
			st.labelInput.Blur()
			st.Cursor = 0
			st.focusCursorField()
			return m, nil
		case "enter":
			return m.submitChipWizard()
		}
		newInput, cmd := st.labelInput.Update(msg)
		st.labelInput = newInput
		return m, cmd
	}

	if st.Advanced {
		switch key {
		case "shift+tab", "up":
			st.advancedText.Blur()
			st.Cursor = -1
			st.labelInput.Focus()
			return m, nil
		case "enter":
			return m.submitChipWizard()
		}
		newInput, cmd := st.advancedText.Update(msg)
		st.advancedText = newInput
		return m, cmd
	}

	if st.Cursor == len(st.criteria) {
		switch key {
		case "tab":
			st.Cursor = -1
			st.labelInput.Focus()
			return m, nil
		case "down":
			// Clamp at the bottom. Wrap would burn through the form
			// on a trackpad burst (each wheel tick synthesizes a
			// KeyDown; cycling top→bottom looks like uncontrolled
			// scroll).
			return m, nil
		case "shift+tab", "up":
			if len(st.criteria) > 0 {
				st.Cursor = len(st.criteria) - 1
				st.focusCursorField()
			} else {
				st.Cursor = -1
				st.labelInput.Focus()
			}
			return m, nil
		case "enter":
			return m, m.openCriterionFieldPicker()
		}
		return m, nil
	}

	if len(st.criteria) == 0 || st.Cursor < 0 || st.Cursor >= len(st.criteria) {
		return m, nil
	}
	cur := &st.criteria[st.Cursor]

	if cur.Kind == cwTri && (key == "x" || key == "delete" || key == "backspace") {
		if cur.Field == chipLimitSentinel {
			return m, nil
		}
		st.criteria = append(st.criteria[:st.Cursor], st.criteria[st.Cursor+1:]...)
		if st.Cursor > len(st.criteria) {
			st.Cursor = len(st.criteria)
		}
		st.focusCursorField()
		return m, nil
	}
	if matches(key, Keys.ChipWizardDelete) {
		if cur.Field == chipLimitSentinel {
			cur.input.SetValue("")
			return m, nil
		}
		st.criteria = append(st.criteria[:st.Cursor], st.criteria[st.Cursor+1:]...)
		if st.Cursor > len(st.criteria) {
			st.Cursor = len(st.criteria)
		}
		st.focusCursorField()
		return m, nil
	}

	switch cur.Kind {
	case cwTri:
		switch key {
		case "j", "down", "tab":
			return m.cwMove(+1), nil
		case "k", "up", "shift+tab":
			return m.cwMove(-1), nil
		case " ", "space", "enter":
			cycleTri(cur)
			return m, nil
		}
	case cwLimit:
		switch key {
		case "tab", "j", "down":
			return m.cwMove(+1), nil
		case "shift+tab", "k", "up":
			return m.cwMove(-1), nil
		case " ", "space":
			if cur.triValue == nil {
				t := true
				cur.triValue = &t
			} else {
				*cur.triValue = !*cur.triValue
			}
			if cur.triValue != nil && *cur.triValue {
				cur.input.Focus()
			} else {
				cur.input.Blur()
				cur.input.SetValue("")
			}
			return m, nil
		case "enter":
			return m.submitChipWizard()
		}
		manual := cur.triValue != nil && *cur.triValue
		if !manual {
			return m, nil
		}
		if len(key) == 1 && (key[0] < '0' || key[0] > '9') &&
			key != "backspace" && key != "delete" {
			return m, nil
		}
		newInput, cmd := cur.input.Update(msg)
		cur.input = newInput
		return m, cmd
	case cwText, cwInt, cwDate:
		switch key {
		case "tab", "down":
			return m.cwMove(+1), nil
		case "shift+tab", "up":
			return m.cwMove(-1), nil
		case "enter":
			return m.submitChipWizard()
		}
		if cur.Kind == cwInt && len(key) == 1 && (key[0] < '0' || key[0] > '9') &&
			key != "backspace" && key != "delete" {
			return m, nil
		}
		newInput, cmd := cur.input.Update(msg)
		cur.input = newInput
		return m, cmd
	}
	return m, nil
}

func (m Model) cwMove(delta int) Model {
	st := m.chipWizard
	if st.Cursor >= 0 && st.Cursor < len(st.criteria) {
		f := &st.criteria[st.Cursor]
		if f.Kind != cwTri {
			f.input.Blur()
		}
	}
	st.Cursor += delta
	if st.Cursor < -1 {
		st.Cursor = -1
	}
	addRow := len(st.criteria)
	if st.Cursor > addRow {
		st.Cursor = addRow
	}
	switch {
	case st.Cursor == -1:
		st.labelInput.Focus()
	case st.Cursor == addRow:
		st.labelInput.Blur()
	default:
		st.labelInput.Blur()
		st.focusCursorField()
	}
	return m
}

func (st *chipWizardState) focusCursorField() {
	if st.Cursor < 0 || st.Cursor >= len(st.criteria) {
		return
	}
	f := &st.criteria[st.Cursor]
	if f.Kind == cwTri {
		return
	}
	if f.Kind == cwLimit && (f.triValue == nil || !*f.triValue) {
		return
	}
	f.input.Focus()
}

func (st *chipWizardState) textInputFocused() bool {
	if st.Cursor == -1 {
		return true // Label input
	}
	if st.Advanced {
		return true // single SOQL WHERE editor
	}
	if st.Cursor < 0 || st.Cursor >= len(st.criteria) {
		return false // "+ Add criterion…" affordance or out of range
	}
	f := st.criteria[st.Cursor]
	switch f.Kind {
	case cwText, cwInt, cwDate:
		return true
	case cwLimit:
		return f.triValue != nil && *f.triValue // manual mode only
	}
	return false // cwTri (toggle) — not a text buffer
}

func (st *chipWizardState) toggleMode() {
	st.Advanced = !st.Advanced
	st.Err = ""
	st.Cursor = 0
	if st.Advanced {
		// Move buffer from simple → advanced: serialise current rows
		// as a SOQL WHERE clause so the user can keep editing. Coming
		// from simple mode means whatever's there is round-trippable
		// by definition, so clear any prior lock reason.
		//
		// Empty simple form (no rows filled) → empty advanced editor.
		// Without this guard, ToSOQLWhere of an empty AND emits the
		// "Id != null" sentinel, which would leak into the editor as
		// a confusing default.
		q := buildSimpleQuery(st.criteria)
		if hasMeaningfulWhere(q.Where) {
			st.advancedText.SetValue(query.ToSOQLWhere(q.Where))
		} else {
			st.advancedText.SetValue("")
		}
		st.advancedLockReason = ""
		st.advancedText.Focus()
	} else {
		// Try to round-trip the SOQL back into rows. If the AST has
		// shape that simple mode can't represent (OR, NOT, nested),
		// pop back to advanced and show an error.
		text := strings.TrimSpace(st.advancedText.Value())
		if text == "" {
			return
		}
		parsed, _, err := query.Parse("SELECT Id FROM X " + text)
		if err != nil {
			st.Err = "advanced: " + err.Error()
			st.Advanced = true
			return
		}
		advanced, compares, reason := splitForWizard(parsed.Where)
		if advanced {
			st.Err = "can't switch to simple mode — query " + reason
			st.advancedLockReason = reason
			st.Advanced = true
			return
		}
		st.advancedLockReason = ""
		st.criteria = criteriaFromCompares(st.catalogue, compares)
		seedLimit := parsed.Limit
		if text != "" && parsed.Limit == 0 {
			seedLimit = -1
		}
		if limitRow := wizardLimitRow(st.catalogue, seedLimit); limitRow != nil {
			st.criteria = append(st.criteria, *limitRow)
		}
		if st.Cursor > len(st.criteria) {
			st.Cursor = len(st.criteria)
		}
		st.focusCursorField()
	}
}

func (m Model) submitChipWizard() (Model, tea.Cmd) {
	st := m.chipWizard
	if st == nil {
		return m, nil
	}
	label := strings.TrimSpace(st.labelInput.Value())
	if err := chips.ValidateLabel(label); err != nil {
		st.Err = err.Error()
		return m, nil
	}

	var q query.Query
	if st.Advanced {
		text := strings.TrimSpace(st.advancedText.Value())
		if text == "" {
			st.Err = "clauses cannot be empty"
			return m, nil
		}
		parsed, _, err := query.Parse("SELECT Id FROM X " + text)
		if err != nil {
			st.Err = err.Error()
			return m, nil
		}
		q = parsed
		if q.Limit == 0 {
			q.Limit = -1
		}
	} else {
		q = buildSimpleQuery(st.criteria)
		if q.Where == nil && len(q.OrderBy) == 0 && q.Limit == 0 && st.existingID == "" {
			st.Err = "fill at least one field"
			return m, nil
		}
	}

	id := st.existingID
	if id == "" {
		id = autoChipID(st.Domain, label)
	}
	if err := chips.ValidateID(id); err != nil {
		st.Err = err.Error()
		return m, nil
	}
	origin := st.existingOrigin
	if origin == qchip.OriginBuiltIn || origin == 0 {
		origin = qchip.OriginUser
	}
	// Favourite policy:
	//   - New view (no existingID): default false. The view is saved
	//     and findable via M; the user pins it to the strip with F or
	//     via "pin to strip" in V. Auto-pinning on create would clutter
	//     the strip with every experimental view a user authors.
	//   - Edit (existingID set): preserve whatever the existing state
	//     was so an F-toggle isn't silently reset every Save.
	favourite := false
	if st.existingID != "" {
		favourite = st.existingFavourite
	}
	// Share replaces the old single-OrgUser stamping. The wizard always
	// has a populated Share at save time (chipWizardInitialShare seeds
	// it on open; the scope chooser updates it if the user runs it).
	// Empty Share is treated as a refuse-to-save so a chip never lands
	// on disk with no scope — that would silently leak everywhere.
	share := st.Share
	if share.IsZero() {
		// Fallback for the rare path where openChipWizard was called
		// without an active org AND the user never opened the chooser:
		// keep the legacy OrgUser stamp if present, else fail loudly.
		if st.existingOrgUser != "" {
			share = settings.ChipShare{Kind: settings.ChipShareOrg, Orgs: []string{st.existingOrgUser}}
		} else {
			st.Saving = false
			st.Err = "pick a scope first (S) — no org context to stamp"
			return m, nil
		}
	}
	c := qchip.Chip{
		ID:         id,
		Label:      label,
		Scope:      st.Scope,
		Origin:     origin,
		Share:      share,
		Query:      q,
		SourceID:   st.existingSrcID,
		SourceName: st.existingSrcName,
		ImportedAt: st.existingImportedAt,
		Favourite:  favourite,
	}

	st.Saving = true
	st.Err = ""

	if m.settings != nil {
		m.settings.UpsertChip(qchip.ToConfig(c, string(st.Domain)))
		if err := m.settings.Save(); err != nil {
			return m, func() tea.Msg { return chipWizardResultMsg{Err: err} }
		}
		if reg := m.registryFor(st.Domain); reg != nil {
			reg.LoadFromSettings(m.settings)
		}
	}
	return m, func() tea.Msg { return chipWizardResultMsg{Label: label} }
}

type chipWizardResultMsg struct {
	Err   error
	Label string
}

func (m Model) applyChipWizardResult(msg chipWizardResultMsg) (Model, tea.Cmd) {
	if m.chipWizard == nil {
		return m, nil
	}
	if msg.Err != nil {
		m.chipWizard.Saving = false
		m.chipWizard.Err = msg.Err.Error()
		return m, nil
	}
	m.chipWizard = nil
	m.flash("view saved: " + msg.Label)
	return m, m.onTabChanged()
}

func buildSimpleQuery(fields []cwField) query.Query {
	var children []query.Node
	out := query.Query{}
	for _, f := range fields {
		if f.Field == chipLimitSentinel {
			out.Limit = limitRowValue(f)
			continue
		}
		var v any
		switch f.Kind {
		case cwText, cwDate:
			s := strings.TrimSpace(f.input.Value())
			if s == "" {
				continue
			}
			v = s
		case cwInt:
			s := strings.TrimSpace(f.input.Value())
			if s == "" {
				continue
			}
			n, err := strconv.Atoi(s)
			if err != nil {
				continue
			}
			v = n
		case cwTri:
			if f.triValue == nil {
				continue
			}
			v = *f.triValue
		}
		children = append(children, query.Cmp(f.Field, f.Op, v))
	}
	out.Where = query.And(children...)
	return out
}

// limitRowValue resolves the storage value for a cwLimit row. Auto =
// 0 (inherit), manual + blank = -1 (unbounded), manual + N = N.
// Negative explicit values are clamped to -1; non-numeric input
// behaves like blank.
func limitRowValue(f cwField) int {
	manual := f.triValue != nil && *f.triValue
	if !manual {
		return 0
	}
	s := strings.TrimSpace(f.input.Value())
	if s == "" {
		return -1
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return -1
	}
	return n
}

// splitForWizard inspects the AST and decides whether simple mode can
// represent it. Simple mode is a flat AND of CompareNodes — anything
// else (OR, NOT, nested groups) needs advanced mode.
//
// Returns advanced=true with a human-readable reason when the AST
// can't be flattened, or advanced=false with the list of compares to
// populate row-by-row.
func splitForWizard(n query.Node) (advanced bool, cmps []query.CompareNode, reason string) {
	if n == nil {
		return false, nil, ""
	}
	switch x := n.(type) {
	case query.CompareNode:
		return false, []query.CompareNode{x}, ""
	case query.AndNode:
		out := make([]query.CompareNode, 0, len(x.Children))
		for _, c := range x.Children {
			cn, ok := c.(query.CompareNode)
			if !ok {
				return true, nil, describeNonFlatNode(c)
			}
			out = append(out, cn)
		}
		return false, out, ""
	case query.OrNode:
		return true, nil, "uses OR"
	case query.NotNode:
		return true, nil, "uses NOT"
	}
	return true, nil, "shape can't be flattened"
}

func hasMeaningfulWhere(n query.Node) bool {
	if n == nil {
		return false
	}
	switch x := n.(type) {
	case query.AndNode:
		return len(x.Children) > 0
	case query.OrNode:
		return len(x.Children) > 0
	}
	return true
}

// describeNonFlatNode names the reason a child node prevents simple-
// mode representation. Used to give the user a helpful error in the
// "tried to switch from advanced → simple but can't" path.
func describeNonFlatNode(n query.Node) string {
	switch n.(type) {
	case query.OrNode:
		return "uses OR"
	case query.NotNode:
		return "uses NOT"
	case query.AndNode:
		return "has nested AND groups"
	}
	return "has a nested group"
}

func populateFromCompareNodes(fields []cwField, cmps []query.CompareNode) {
	for _, c := range cmps {
		for i := range fields {
			f := &fields[i]
			if f.Field != c.Field || f.Op != c.Op {
				continue
			}
			switch f.Kind {
			case cwText, cwDate:
				if s, ok := c.Value.(string); ok {
					if f.input.Value() == "" {
						f.input.SetValue(s)
					}
				}
			case cwInt:
				if n, ok := c.Value.(int); ok {
					if f.input.Value() == "" {
						f.input.SetValue(strconv.Itoa(n))
					}
				}
			case cwTri:
				if b, ok := c.Value.(bool); ok && f.triValue == nil {
					f.triValue = &b
				}
			}
			break
		}
	}
}

// cycleTri — same shape as the legacy wizard's helper, copied here so
// both wizards (theme picker doesn't use it; only this) live in one
// file with no shared state.
func cycleTri(f *cwField) {
	switch {
	case f.triValue == nil:
		t := true
		f.triValue = &t
	case *f.triValue:
		fl := false
		f.triValue = &fl
	default:
		f.triValue = nil
	}
}

// autoChipID generates a stable kebab-cased id when the user hasn't
// chosen one explicitly. domain prefix avoids collisions across
// surfaces (records.recent vs objects.recent).
func autoChipID(domain chipDomain, label string) string {
	stamp := time.Now().Format("20060102-150405")
	return string(domain) + "-" + slugify(label) + "-" + stamp
}

func padRight(s string, width int) string {
	if width <= 0 {
		return s
	}
	if lipgloss.Width(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-lipgloss.Width(s))
}

// valueOr returns s when non-empty, otherwise fallback.
func valueOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func wizardTitleFor(d chipDomain, existing qchip.Chip) string {
	verb := "New"
	if existing.ID != "" {
		verb = "Edit"
	}
	return fmt.Sprintf("%s view · %s", verb, d)
}

// chipWizardInitialShare seeds the wizard's Share on open.
//
//   - Editing an existing chip → use the chip's current Share. The
//     adapter has already migrated legacy OrgUser-only chips into a
//     single-org Share via ChipConfig.EffectiveShare, so this just
//     reads the runtime field.
//   - New chip → stamp the active org (single-org Share). If no org is
//     selected, return zero — the save guard will catch it.
func chipWizardInitialShare(m *Model, existing qchip.Chip) settings.ChipShare {
	if existing.ID != "" {
		if !existing.Share.IsZero() {
			return existing.Share
		}
		if existing.OrgUser != "" {
			return settings.ChipShare{Kind: settings.ChipShareOrg, Orgs: []string{existing.OrgUser}}
		}
		return settings.ChipShare{}
	}
	if u := m.activeOrgUserForChips(); u != "" {
		return settings.ChipShare{Kind: settings.ChipShareOrg, Orgs: []string{u}}
	}
	return settings.ChipShare{}
}

func (m *Model) chipWizardOpenScopeChooser() tea.Cmd {
	if m.chipWizard == nil {
		return nil
	}
	st := m.chipWizard
	return m.openChipScopeChooser("Chip scope", st.Share, chipScopeTarget{kind: chipScopeTargetWizard})
}

func chipWizardShareSummary(m Model, s settings.ChipShare) string {
	if s.IsZero() {
		return "(no scope yet — press S)"
	}
	switch s.Kind {
	case settings.ChipShareGlobal:
		return "global (every org)"
	case settings.ChipShareGroup:
		name := s.Group
		for _, g := range m.chipScopeGroupOptions() {
			if g.ID == s.Group {
				name = g.Name
				break
			}
		}
		return "group · " + name
	case settings.ChipShareOrgs:
		switch len(s.Orgs) {
		case 0:
			return "(no orgs picked)"
		case 1:
			return chipShareFriendlyOrg(m, s.Orgs[0])
		case 2, 3:
			names := make([]string, 0, len(s.Orgs))
			for _, u := range s.Orgs {
				names = append(names, chipShareFriendlyOrg(m, u))
			}
			return strings.Join(names, ", ")
		default:
			return fmt.Sprintf("%d orgs", len(s.Orgs))
		}
	default: // ChipShareOrg or unknown
		if len(s.Orgs) == 1 {
			return chipShareFriendlyOrg(m, s.Orgs[0])
		}
		return string(s.Kind)
	}
}

func chipWizardShareDetailLines(m Model, s settings.ChipShare) []string {
	switch s.Kind {
	case settings.ChipShareOrgs:
		if len(s.Orgs) <= 3 {
			return nil
		}
		out := make([]string, 0, len(s.Orgs))
		for _, u := range s.Orgs {
			out = append(out, "  · "+chipShareFriendlyOrg(m, u))
		}
		return out
	case settings.ChipShareGroup:
		if m.settings == nil || s.Group == "" {
			return nil
		}
		for _, g := range m.settings.OrgGroups() {
			if g.ID != s.Group {
				continue
			}
			if len(g.Members) == 0 {
				return []string{"  (group has no members)"}
			}
			out := make([]string, 0, len(g.Members))
			for _, u := range g.Members {
				out = append(out, "  · "+chipShareFriendlyOrg(m, u))
			}
			return out
		}
		return []string{"  (group not found — pick another scope)"}
	}
	return nil
}

func chipShareFriendlyOrg(m Model, username string) string {
	for _, o := range m.orgs {
		if o.Username == username {
			if o.Alias != "" {
				return o.Alias
			}
			return o.Username
		}
	}
	return username
}

func (m Model) wizardHintLines(st *chipWizardState, f cwField, width int) []string {
	if f.Op == "" || width < 8 {
		return nil
	}
	hint := f.Field + " " + opLabelFor(f.Op)
	if f.Hint != "" {
		hint += " · " + f.Hint
	}
	if m.valueSourceFor(st.Scope, f) != nil {
		hint += " · " + firstPretty(Keys.ChipWizardLookup) + " to pick"
	}
	wrapped := ansi.Wrap(hint, width, "")
	return strings.Split(wrapped, "\n")
}
