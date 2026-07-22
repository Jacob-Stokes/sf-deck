package ui

// Chip wizard — multi-field form modal that produces a query.Query
// directly. Replaces the old filter wizard (which authored a flat
// filter.Spec).
//
// Two modes share the same modal:
//
//	Simple   — flat row-per-field form. Each filled row becomes an
//	           AND clause. This is what most users want most of the time.
//	Advanced — a SOQL WHERE editor. Lets the user write a freeform
//	           predicate (date literals, OR groups, parens, NOT…)
//	           which we round-trip through query.Parse on save.
//
// Toggle between modes with `a`. Both modes write the same Chip; the
// underlying query.Query is the storage format regardless of which
// authoring path was used.
//
// Per-domain field catalogues live in chip_wizard_fields.go so adding
// a new surface (perm sets, profiles…) is just one new catalogue.

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

// cwField is one editable row in the wizard.
type cwField struct {
	// Field is the AST column name. CompareNodes built from this row
	// use it verbatim; SOQL emits it verbatim.
	Field string
	// Label is the left-column human label.
	Label string
	// Hint is the dim sub-line shown when the row is focused.
	Hint string
	// Op is the operator the row's value comparator uses (Contains for
	// string fields, Equals for enums, GTE for numeric, etc.).
	Op query.Op
	// Kind drives input mode: text, int, tristate, datestr.
	Kind cwKind
	// runtime state
	input    textinput.Model
	triValue *bool
}

type cwKind int

const (
	cwText cwKind = iota
	cwInt
	cwTri
	cwDate
	// cwLimit is the special-case row used by the pinned chipLimit
	// row. Combines a toggle (auto / manual) with a numeric input.
	// Auto mode shows the global default in dim text and disables
	// the input; manual lets the user pin a value or clear it for
	// unbounded.
	cwLimit
)

// chipWizardState is the live state of the wizard.
//
// Cursor convention:
//
//	-1   → label input is focused
//	 0..N-1 → criteria[i] is focused (a previously-added row)
//	 N    → "+ Add criterion…" affordance is focused
//
// where N == len(criteria). So Cursor == len(criteria) is the
// add-row position; pressing enter there opens the field picker.
type chipWizardState struct {
	Title  string
	Domain chipDomain // which surface this chip targets
	Scope  string

	// heightFloor pins the modal body's rendered line count so the
	// box doesn't resize as focus moves (focused rows add hint lines,
	// the empty state toggles). Grows monotonically per wizard
	// session; render pads with blank lines up to it.
	heightFloor int

	// existingID is "" for new chips. When set, save updates in place.
	existingID                                         string
	existingLbl                                        string
	existingOrigin                                     qchip.Origin
	existingSrcID, existingSrcName, existingImportedAt string
	// existingFavourite preserves the on-strip pin state across
	// edits — without it, the Save() round-trip would clobber any
	// F-toggles the user made between create and edit.
	existingFavourite bool
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

	// criteria is the list of CompareNodes the user has added. Each
	// renders as one row in the wizard. Edits happen in place via
	// the row's input / triValue.
	criteria []cwField
	Cursor   int

	// Advanced mode toggles to a single SOQL WHERE editor. Buffer is
	// pre-filled from the chip's current Query (ToSOQLWhere).
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

	// Label input — always available regardless of mode.
	labelInput textinput.Model

	// Result lifecycle.
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
		// Seed Share: edits start from the chip's current share (which
		// EffectiveShape on the config side already normalised); new
		// chips start as single-org for the active org. Empty active
		// org leaves Share zero; the save guard catches that.
		Share:     chipWizardInitialShare(m, existing),
		catalogue: m.wizardFieldsFor(d, scope),
		// Existing chips lock their mode immediately — toggling
		// would silently lose AST shapes simple mode can't
		// express. New chips stay unlocked until first save so the
		// user can flip while building.
		modeLocked: existing.ID != "",
	}

	// Pre-fill from the existing chip's AST. Each top-level And child
	// (or a single CompareNode) becomes one criterion row, looked up
	// against the catalogue so the row's Kind / Op / Hint match what
	// the picker would have configured. CompareNodes whose Field
	// isn't in the catalogue still appear — the wizard isn't a
	// blocker, just less informative for those rows.
	advanced, prefill, reason := splitForWizard(existing.Query.Where)
	state.Advanced = advanced
	if advanced && reason != "" {
		state.advancedLockReason = reason
	}
	if !advanced {
		state.criteria = criteriaFromCompares(state.catalogue, prefill)
	}
	// Always pin a Limit row at the end of the criteria list — gives
	// users a first-class slot to set per-chip overrides without
	// digging into the picker. Empty input = inherit settings default
	// at fetch time. Edit mode seeds the row from the existing chip's
	// stored Limit.
	if limitRow := wizardLimitRow(state.catalogue, existing.Query.Limit); limitRow != nil {
		state.criteria = append(state.criteria, *limitRow)
	}

	state.labelInput = newWizardInput(existing.Label)
	// Advanced mode lets the user write the post-FROM clauses
	// directly: WHERE … ORDER BY … LIMIT N. Seed from the
	// existing chip — but skip seeding the LIMIT clause when
	// existing.Query.Limit < 1, since the storage uses -1 to mean
	// "unbounded" and 0 to mean "auto" (neither valid in raw SOQL).
	advancedSeed := ""
	if hasMeaningfulWhere(existing.Query.Where) || len(existing.Query.OrderBy) > 0 || existing.Query.Limit > 0 {
		seed := existing.Query
		if seed.Limit <= 0 {
			seed.Limit = 0 // ToSOQLClauses skips zero
		}
		advancedSeed = query.ToSOQLClauses(seed)
	}
	state.advancedText = newWizardInput(advancedSeed)

	// Cursor on the label by default for new chips so the first
	// thing the user does is name it; for edits jump straight to
	// the "+ Add criterion…" row (label already has a value).
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
	// Picker is roughly 2/3 of the wizard width — wide enough to show
	// the field name + meta column comfortably, narrow enough that it
	// doesn't look like another modal stacked on top.
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

// criterionPickedMsg lands on the main loop after the user picks a
// field. We route through a tea.Msg rather than mutating Model
// inside OnPick so the model copy in the picker's closure is the
// live one.
type criterionPickedMsg struct {
	field cwField
}

// applyCriterionPicked inserts a fresh criterion row into the wizard
// and focuses it so the user can immediately type the value.
func (m Model) applyCriterionPicked(msg criterionPickedMsg) (Model, tea.Cmd) {
	st := m.chipWizard
	if st == nil {
		return m, nil
	}
	// Clone the catalogue entry so each criterion has its own
	// textinput + triValue independent of subsequent picks.
	row := msg.field
	if row.Kind != cwTri {
		row.input = newWizardInput("")
	}
	st.criteria = append(st.criteria, row)
	st.Cursor = len(st.criteria) - 1
	st.focusCursorField()
	return m, nil
}

// opLabelFor renders an Op as a short user-facing label for the
// criterion summary line. ("contains", "equals", ">=", etc.)
// sectionHeading renders a small bold uppercase section label used
// to break the wizard body into visual groups (Filters, Examples).
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

// wizardLimitRow returns the Limit row entry seeded from the
// existing chip's Query.Limit. nil when the catalogue doesn't carry
// a $limit row (defensive — every domain currently appends one via
// wizardFieldsFor, but a future domain might opt out).
//
// Storage convention for chip.Query.Limit:
//
//	0  → Auto mode (inherit settings.DefaultChipLimit at fetch time)
//	-1 → Manual mode, no limit (unbounded fetch via cursor follow)
//	>0 → Manual mode, pinned to that exact cap
//
// We surface those three states via the cwLimit kind: triValue
// holds the manual toggle (true = manual), input.Value() holds the
// pinned number when triValue == true.
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

// criteriaFromCompares maps the CompareNodes from a parsed Query into
// criterion rows for the wizard. Looks each one up in the catalogue
// for type / hint info; missing fields fall back to a string-text row
// so the user can still edit them.
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
		// Initialise the row with the criterion's value.
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

// catalogueLookup returns a pointer to the catalogue entry matching
// (field, op), or nil. Used both for criterion pre-fill and for the
// picker's add-flow when it needs to convert a picked field into a
// criterion row.
func catalogueLookup(catalogue []cwField, field string, op query.Op) *cwField {
	// Exact (field, op) match first — preserves the user's intent
	// when the catalogue offers the same field on multiple ops
	// (e.g. Name contains vs Name startsWith).
	for i := range catalogue {
		if catalogue[i].Field == field && catalogue[i].Op == op {
			return &catalogue[i]
		}
	}
	// Fall back to any catalogue entry on the same field — the picker
	// uses the catalogue's default Op, which is the right behaviour
	// when the user adds via the picker rather than parsing existing
	// SOQL.
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

// styleWizardInput themes a textinput for the wizard.
func styleWizardInput(ti *textinput.Model) {
	s := ti.Styles()
	s.Focused.Text = lipgloss.NewStyle().Foreground(theme.Fg)
	s.Focused.Placeholder = lipgloss.NewStyle().Foreground(theme.FgDim)
	s.Blurred.Text = lipgloss.NewStyle().Foreground(theme.Fg)
	s.Blurred.Placeholder = lipgloss.NewStyle().Foreground(theme.FgDim)
	s.Cursor.Color = theme.BorderHi
	ti.SetStyles(s)
}

// newWizardInput constructs a fully-initialised textinput.Model for
// the wizard's various fields. Single helper so tweaks to the
// styling, prompt, char-limit, or initial-cursor behaviour stay in
// one place.
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
	// (cursor position, rendered line index) pairs for click zones.
	type hitRow struct{ cursor, line int }
	var hits []hitRow
	// Hint-height bookkeeping for the stable-height padding below.
	maxHintLines, curHintLines := 0, 0
	lines = append(lines, titleStyle.Render(st.Title))

	// Mode line — plain English, with the toggle-key hint surfaced
	// only when toggling is actually available.
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

	// Label row — always editable. Wider gap above so the input
	// breathes; label column is narrower (16) since "Name" is short.
	const labelCol = 16
	labelFocused := st.Cursor == -1
	prefix := "  "
	if labelFocused {
		prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " "
	}
	st.labelInput.SetWidth(inner - labelCol - 2)
	hits = append(hits, hitRow{cursor: -1, line: len(lines)})
	lines = append(lines, prefix+padRight("Name", labelCol-2)+"["+padRight(st.labelInput.View(), inner-labelCol-2)+"]")

	// Scope row (read-only summary; press S to edit). Kept right under
	// Name so it's the first thing the user sees after naming the chip.
	// The "S to change" affordance is rendered in the chip-strip accent
	// colour rather than the muted footer style so it actually catches
	// the eye on first glance — without it, users miss the keybind and
	// assume Scope is fixed.
	scopeSummary := chipWizardShareSummary(m, st.Share)
	scopeHint := lipgloss.NewStyle().Foreground(theme.BorderHi).Render("S") +
		subStyle.Render(" to change")
	lines = append(lines, "  "+padRight("Scope", labelCol-2)+
		subStyle.Render(scopeSummary)+"  ["+scopeHint+"]")
	// Continuation lines: when the scope is multi-org or a group, list
	// the actual orgs (or the group's members) below the summary so the
	// user sees exactly what they're committing to before they save.
	// Single-org and global scopes are already fully described by the
	// summary line — no continuation needed.
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
		// Render every active criterion as a row. The "+ Add
		// criterion…" affordance sits at index len(criteria) — that
		// row is also focusable, and pressing enter on it opens the
		// field picker.
		valueCol := inner - labelCol
		// Worst-case hint height across ALL rows: the focused row's
		// hint renders below it, and hints can wrap. Reserving the
		// tallest hint keeps the modal height identical no matter
		// which row holds focus.
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
				// Limit row — plain-English presentation. The mode
				// pill says "default" or "custom" so the user reads
				// it as words; the value column shows what'll
				// actually apply (the inherited number, or the
				// editable input in custom mode).
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

		// Empty state — show before the +Add affordance so the
		// user reads "no criteria yet → + Add filter" top-down.
		addFocused := st.Cursor == len(st.criteria)
		if len(st.criteria) == 0 && !addFocused {
			lines = append(lines, subStyle.Italic(true).Render("    no filters yet — pick a field below"))
		}

		// Visual gap above the +Add row so it reads as a button,
		// not another criterion. Leading "+" + Enter hint helps
		// new users recognise it.
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
	// Footer split into two lines so it doesn't run off narrow
	// modals. First line covers movement + selection within the
	// form; second line covers lifecycle (toggle / save / cancel).
	moveHint := "tab to move · " + firstPretty(Keys.ChipWizardLookup) + " to look up values · " +
		firstPretty(Keys.ChipWizardDelete) + " to delete the focused row"
	lifeHint := "S change scope · " + firstPretty(Keys.ChipWizardMode) + " to switch mode · " +
		firstPretty(Keys.ChipWizardSave) + " to save · esc to cancel"
	if st.modeLocked {
		lifeHint = "S change scope · " + firstPretty(Keys.ChipWizardSave) + " to save · esc to cancel"
	}
	lines = append(lines, subStyle.Render(moveHint))
	lines = append(lines, subStyle.Render(lifeHint))

	// No line may exceed the modal's inner width: modalBox wraps long
	// lines onto extra physical rows, which both looks broken and
	// defeats the height floor below (it counts logical lines). Hard-
	// truncate everything — long hints lose their tail rather than
	// resizing the box.
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

	// Hit layers: repaint each focusable row, tagged with its wizrow
	// zone id, as a child of the modal layer. Rows are located by
	// ANSI-stripped content search in the FINAL box (monotonic scan)
	// rather than by computed line index — long rows can wrap inside
	// modalBox, which would shift arithmetic offsets.
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
		// Match on a prefix chunk: a wrapped row keeps its head on
		// the first physical line, which is the one users click.
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

// clickChipWizardRow focuses the wizard row under a mouse click —
// the same transitions the keyboard tab-moves perform. Clicking the
// "+ Add filter" affordance acts like pressing enter on it (opens
// the field picker).
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

// handleChipWizardKey is the reducer.
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
		// Open the value picker for the focused criterion. ctrl+l
		// for "lookup" is the canonical chord; ctrl+space (which
		// some terminals send as ctrl+@) is a fallback that matches
		// IDE autocomplete conventions. No-op when the focused
		// field has no constrained value source.
		return m, m.openValuePicker()
	case key == "S" && !st.textInputFocused():
		// Capital-S opens the cross-org scope chooser — but ONLY when a
		// text input isn't focused. Inside the label / advanced-SOQL / a
		// text-criterion buffer, S is a literal character the user is
		// typing (a view named "Summer School" must not fire the scope
		// chooser). The chooser updates st.Share in-place; the wizard
		// re-renders the "Scope: …" row on next paint.
		return m, (&m).chipWizardOpenScopeChooser()
	}

	// Cursor at -1 means the Label input is focused. Same rule as
	// other text rows: tab / down move to the first field; everything
	// printable (including j/k/etc.) goes into the buffer.
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

	// Cursor at len(criteria) is the "+ Add criterion…" affordance.
	if st.Cursor == len(st.criteria) {
		switch key {
		case "tab":
			// Tab wraps to label (cyclic tab order is the convention
			// for keyboard form navigation).
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

	// 'x' / delete on a focused criterion removes it (only when the
	// row isn't a text input that'd consume the key — tristate rows
	// have no buffer to fight with).
	// The pinned Limit row is exempt — it has no equivalent picker
	// entry, so deleting it would orphan the user from setting a
	// per-chip cap. Clearing the input is the right gesture there.
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
	// ctrl+x always deletes the focused criterion regardless of row
	// kind, so users can drop a text-row from inside the buffer.
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
		// Space toggles auto↔manual. In manual the input takes
		// keystrokes (digits + backspace + clear); in auto the row
		// shows the global default in dim text and ignores input.
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
			// Auto mode swallows keystrokes — input is non-editable.
			return m, nil
		}
		// Manual mode: digits + backspace + clear via the input model.
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

// cwMove shifts focus by delta, blurring/focusing inputs as needed.
//
// Cursor range is [-1, len(criteria)] inclusive:
//
//	-1   = label
//	0..N-1 = criteria[i]
//	N    = "+ Add criterion…" affordance
func (m Model) cwMove(delta int) Model {
	st := m.chipWizard
	// Blur the currently-focused criterion's textinput before moving.
	// cwTri rows have no input; cwLimit rows only have one in manual
	// mode — checking input.Focused() handles both.
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
		// Add row has no input to focus.
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
	// cwLimit only takes input when in manual mode; auto mode shows
	// a static dim default and shouldn't capture keys.
	if f.Kind == cwLimit && (f.triValue == nil || !*f.triValue) {
		return
	}
	f.input.Focus()
}

// textInputFocused reports whether the wizard's current focus is a text
// buffer the user is typing into — the Label field, the Advanced SOQL
// editor, or a text/int/date criterion (cwLimit only in manual mode).
// Used to stop single-letter wizard shortcuts (e.g. capital S) from
// hijacking a literal keystroke mid-typing. Mirrors focusCursorField's
// "does this row capture input" rule.
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
		// Replace criteria with what the parsed SOQL describes.
		st.criteria = criteriaFromCompares(st.catalogue, compares)
		// Re-pin the Limit row, seeded from the SOQL's LIMIT (or -1
		// when the user wrote no LIMIT — SOQL semantics: unbounded).
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

// submitChipWizard validates + persists.
func (m Model) submitChipWizard() (Model, tea.Cmd) {
	st := m.chipWizard
	if st == nil {
		return m, nil
	}
	label := strings.TrimSpace(st.labelInput.Value())
	// Same naming rules as the headless CLI — one validator, two
	// surfaces (services/chips owns it).
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
		// User supplies post-FROM clauses (WHERE / ORDER BY / LIMIT).
		// Prepend SELECT Id FROM X so query.Parse sees a complete
		// statement; the parser extracts WHERE/ORDER BY/LIMIT and we
		// keep the parsed Query as-is.
		parsed, _, err := query.Parse("SELECT Id FROM X " + text)
		if err != nil {
			st.Err = err.Error()
			return m, nil
		}
		q = parsed
		// SOQL semantics: no LIMIT = unbounded. Map that into our
		// storage (-1) so the fetcher emits SOQL with no LIMIT and
		// cursor-follows to completion.
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

	// Save runs inline on the Update goroutine. settings.Save is a
	// small TOML file write; not worth a tea.Cmd round-trip — and the
	// previous goroutine version mutated settings + the registry off
	// the main loop, racing renders that read the chip catalog.
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

// chipWizardResultMsg lands on the main loop after Save returns.
type chipWizardResultMsg struct {
	Err   error
	Label string
}

// applyChipWizardResult — Update branch.
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

// buildSimpleQuery constructs a query.Query by ANDing every filled
// row's CompareNode together. Empty rows drop out.
//
// The chipLimitSentinel field ("$limit") is special-cased: it's a
// cwLimit row whose state encodes one of three storage values for
// Query.Limit (see wizardLimitRow for the full mapping):
//
//	Auto (triValue=false)               → 0  (inherit default)
//	Manual + blank input (triValue=true) → -1 (unbounded)
//	Manual + N (triValue=true)           → N  (pinned)
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

// hasMeaningfulWhere reports whether a Where node carries any actual
// constraint. A nil node, an empty AndNode, or an empty OrNode all
// fall through as "no constraint" — used to decide whether to seed
// the advanced editor with text or leave it blank when toggling from
// simple mode.
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

// populateFromCompareNodes fills each catalogue row from the
// matching CompareNode in `cmps`. Match by (Field, Op).
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

// padRight pads a string to width with spaces. Truncates if wider.
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

// wizardTitleFor builds the title row.
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

// chipWizardOpenScopeChooser launches the scope chooser pre-seeded with
// the wizard's current share and writes the user's choice back into
// the wizard state. The wizard stays open behind the chooser; the
// chooser dismisses itself on completion or esc.
func (m *Model) chipWizardOpenScopeChooser() tea.Cmd {
	if m.chipWizard == nil {
		return nil
	}
	st := m.chipWizard
	return m.openChipScopeChooser("Chip scope", st.Share, chipScopeTarget{kind: chipScopeTargetWizard})
}

// chipWizardShareSummary formats the wizard's current share for the
// "Scope: …" row in the form (and for hint surfaces). Designed to be
// short — the form has limited width.
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

// chipWizardShareDetailLines returns continuation lines for the wizard's
// Scope row, expanding multi-org and group scopes into the actual list
// of orgs the chip will appear for. Returns an empty slice for the
// scopes the one-line summary already fully describes (single-org,
// global, zero) — keeps the wizard quiet when there's nothing to add.
//
// For "These orgs" we emit one bullet per username (alias-resolved). For
// a group we list the group's current members the same way, so the user
// can see who's actually in it without leaving the wizard. Group resolves
// against settings.OrgGroups so a renamed/edited group reflects live.
func chipWizardShareDetailLines(m Model, s settings.ChipShare) []string {
	switch s.Kind {
	case settings.ChipShareOrgs:
		// The one-line summary already lists ≤3 orgs inline; only emit
		// the per-org bullets when the summary collapsed to "N orgs".
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
		// Group id didn't resolve (deleted / renamed away). Tell the
		// user so they don't think their chip is silently visible.
		return []string{"  (group not found — pick another scope)"}
	}
	return nil
}

// chipShareFriendlyOrg formats one org username as alias-or-username,
// matching chipScopeFriendlyOrgName but callable without a *Model.
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

// wizardHintLines builds the focused-criterion hint and soft-wraps it
// to the given width. Shared by the renderer (which shows it under
// the focused row) and the stable-height reservation (which needs the
// tallest hint across all rows).
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
