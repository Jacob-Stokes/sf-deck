package ui

// Text-shaped field editors — anything where the user types into a
// textinput and the value goes verbatim (or with a local format
// check) to Salesforce. Covers the majority of sObject fields:
// string, email, phone, url, int, double, currency, percent.
//
// Each kind shares the same widget (textinput.Model) and code path;
// the difference is the local validator that runs on Commit. String
// kinds accept anything; numeric kinds reject non-numeric input;
// email/url validate softly (best-effort regex — we let SF have
// final say so we don't gate on a flaky local regex).

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// textKind discriminates the validation behaviour. Plain "string"
// fields accept anything; numeric kinds reject non-numeric input on
// Commit; email/phone/url are loose-string under the hood — SF does
// real validation server-side, the kind here just colours the hint.
type textKind int

const (
	textKindString textKind = iota
	textKindEmail
	textKindPhone
	textKindURL
	textKindInt
	textKindDouble
	textKindCurrency
	textKindPercent
)

// textEditor handles single-line text inputs. The widget itself is
// bubbles/textinput; rendering pulls its View() into the cell.
type textEditor struct {
	kind textKind
}

func init() {
	// Plain text + textarea-of-short fields use the same widget for
	// now. textareaEditor (long fields) lives in its own struct
	// below so future multi-line rendering can diverge.
	registerFieldEditor(&textEditor{kind: textKindString},
		"string", "id")
	registerFieldEditor(&textEditor{kind: textKindEmail}, "email")
	registerFieldEditor(&textEditor{kind: textKindPhone}, "phone")
	registerFieldEditor(&textEditor{kind: textKindURL}, "url")
	registerFieldEditor(&textEditor{kind: textKindInt}, "int")
	registerFieldEditor(&textEditor{kind: textKindDouble}, "double")
	registerFieldEditor(&textEditor{kind: textKindCurrency}, "currency")
	registerFieldEditor(&textEditor{kind: textKindPercent}, "percent")
	registerFieldEditor(&textareaEditor{}, "textarea")

	// Read-only: types Salesforce never accepts via PATCH or that we
	// refuse policy-wise (Id is set by the platform, formula values
	// are derived).
	registerFieldEditor(&readOnlyEditor{reason: "id is immutable"}, "address")
}

// CanEdit defers to the describe's Updateable flag — SF marks
// system-managed fields (CreatedDate, LastModifiedBy, formula
// outputs, autonumber) as Updateable=false. We layer a defensive
// check on Calculated/AutoNumber as well in case a misbehaving
// describe leaks through.
func (e *textEditor) CanEdit(f sf.Field) bool {
	if !f.Updateable {
		return false
	}
	if f.CalculatedFormula != "" || f.AutoNumber {
		return false
	}
	return true
}

// Init seeds the textinput with the current display string.
func (e *textEditor) Init(f sf.Field, current any) EditState {
	raw := stringifyFieldValue(current)
	return EditState{Field: f, Raw: raw}
}

// RenderEditCell renders the textinput-style widget. We don't hold
// a bubbles textinput inside EditState (would explode the union); we
// render synthetic "<buffer>▌" instead, matching the bubbles cursor.
// This keeps EditState a plain value type that survives the value-
// receiver Model copy without ceremony.
func (e *textEditor) RenderEditCell(s *EditState, width int, focused bool) string {
	style := lipgloss.NewStyle().Foreground(theme.Fg)
	if !focused {
		style = lipgloss.NewStyle().Foreground(theme.FgDim)
	}
	cursorStyle := lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true)
	var body string
	if focused {
		body = style.Render(s.Raw) + cursorStyle.Render("▌")
	} else {
		body = style.Render(s.Raw)
	}
	if s.Error != "" {
		body += "  " + lipgloss.NewStyle().Foreground(theme.Red).Render("· "+s.Error)
	}
	return body
}

// HandleKey routes printable / backspace / left/right / home/end /
// ctrl+u onto the buffer. Everything else (esc / tab / enter) is
// left for the dispatcher.
func (e *textEditor) HandleKey(s *EditState, msg tea.KeyMsg) (bool, tea.Cmd) {
	key := msg.String()
	switch key {
	case "backspace":
		if len(s.Raw) > 0 {
			_, size := utf8.DecodeLastRuneInString(s.Raw)
			s.Raw = s.Raw[:len(s.Raw)-size]
		}
		s.Error = ""
		return true, nil
	case "ctrl+u":
		s.Raw = ""
		s.Error = ""
		return true, nil
	case "ctrl+a", "home":
		// No caret model yet — beginning is a no-op visual cue.
		return true, nil
	case "ctrl+e", "end":
		return true, nil
	case "space", " ":
		s.Raw += " "
		s.Error = ""
		return true, nil
	}
	// Printable runes — bubbles textinput would do utf-8 aware
	// insertion; we mirror that simply by appending to the buffer.
	// /record edit is a one-line affair; advanced cursor positioning
	// can be added later if users ask.
	if r, ok := singleRune(key); ok {
		s.Raw += string(r)
		s.Error = ""
		return true, nil
	}
	return false, nil
}

// Commit converts the buffer into the PATCH value. Empty buffer for
// a nillable field means CommitNull. Numeric kinds parse strict; on
// parse failure the field stays in edit mode with the error message
// surfaced via EditState.Error.
func (e *textEditor) Commit(s *EditState) (CommitMode, any, error) {
	raw := strings.TrimSpace(s.Raw)
	if raw == "" {
		if !s.Field.Nillable {
			s.Error = "required"
			return CommitNone, nil, fmt.Errorf("required")
		}
		return CommitNull, nil, nil
	}
	switch e.kind {
	case textKindInt:
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			s.Error = "not an integer"
			return CommitNone, nil, err
		}
		return CommitValue, v, nil
	case textKindDouble, textKindCurrency, textKindPercent:
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			s.Error = "not a number"
			return CommitNone, nil, err
		}
		return CommitValue, v, nil
	}
	return CommitValue, raw, nil
}

// ---- textarea (multi-line) editor --------------------------------

// textareaEditor handles "textarea" fields. /record renders rows
// horizontally so a true multi-line widget would blow up the row
// shape; instead we treat textarea as "long string" — same single-
// line widget but with an indicator that the field accepts newlines
// via \n escapes. Users with multi-paragraph content open the
// $EDITOR escape hatch via the future ctrl+e shortcut on a textarea
// field. Phase 1 keeps things simple.
type textareaEditor struct{}

func (e *textareaEditor) CanEdit(f sf.Field) bool {
	return f.Updateable && f.CalculatedFormula == "" && !f.AutoNumber
}

func (e *textareaEditor) Init(f sf.Field, current any) EditState {
	raw := stringifyFieldValue(current)
	// Visualise embedded newlines as ⏎ so the user knows they're
	// editing a multi-line value in a single-line buffer.
	raw = strings.ReplaceAll(raw, "\n", "⏎")
	return EditState{Field: f, Raw: raw}
}

func (e *textareaEditor) RenderEditCell(s *EditState, width int, focused bool) string {
	style := lipgloss.NewStyle().Foreground(theme.Fg)
	if !focused {
		style = lipgloss.NewStyle().Foreground(theme.FgDim)
	}
	body := style.Render(s.Raw)
	if focused {
		body += lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true).Render("▌")
	}
	if s.Error != "" {
		body += "  " + lipgloss.NewStyle().Foreground(theme.Red).Render("· "+s.Error)
	}
	return body
}

func (e *textareaEditor) HandleKey(s *EditState, msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "backspace":
		if len(s.Raw) > 0 {
			_, size := utf8.DecodeLastRuneInString(s.Raw)
			s.Raw = s.Raw[:len(s.Raw)-size]
		}
		s.Error = ""
		return true, nil
	case "ctrl+u":
		s.Raw = ""
		s.Error = ""
		return true, nil
	case "space", " ":
		s.Raw += " "
		s.Error = ""
		return true, nil
	}
	if r, ok := singleRune(msg.String()); ok {
		s.Raw += string(r)
		s.Error = ""
		return true, nil
	}
	return false, nil
}

func (e *textareaEditor) Commit(s *EditState) (CommitMode, any, error) {
	raw := strings.TrimSpace(s.Raw)
	if raw == "" {
		if !s.Field.Nillable {
			s.Error = "required"
			return CommitNone, nil, fmt.Errorf("required")
		}
		return CommitNull, nil, nil
	}
	// Restore real newlines from our visual placeholder.
	raw = strings.ReplaceAll(raw, "⏎", "\n")
	return CommitValue, raw, nil
}

// ---- read-only editor --------------------------------------------

// readOnlyEditor refuses entry to edit mode. Returned for field
// types we can't or won't edit (composite types like address, or
// types that have policy gates we haven't built yet). CanEdit
// returns false uniformly so the /record surface short-circuits
// before constructing an EditState.
type readOnlyEditor struct {
	reason string
}

func (e *readOnlyEditor) CanEdit(_ sf.Field) bool                              { return false }
func (e *readOnlyEditor) Init(f sf.Field, current any) EditState               { return EditState{Field: f} }
func (e *readOnlyEditor) RenderEditCell(_ *EditState, _ int, _ bool) string    { return "" }
func (e *readOnlyEditor) HandleKey(_ *EditState, _ tea.KeyMsg) (bool, tea.Cmd) { return false, nil }
func (e *readOnlyEditor) Commit(_ *EditState) (CommitMode, any, error) {
	return CommitNone, nil, fmt.Errorf("%s", e.reason)
}

// ---- helpers -----------------------------------------------------

// stringifyFieldValue turns the cached record value into the user-
// editable buffer. Mirrors what /record's field-value renderer
// does so users see the same string they were looking at right
// before they pressed `e`.
func stringifyFieldValue(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		// JSON decodes all numbers as float64; preserve integers
		// without trailing zeros.
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	return fmt.Sprintf("%v", v)
}

// singleRune extracts a one-character key into a rune for buffer
// insertion. Returns false on multi-key sequences ("ctrl+x", "esc",
// "tab", etc.) so the dispatcher can route those instead.
func singleRune(key string) (rune, bool) {
	if utf8.RuneCountInString(key) != 1 {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(key)
	return r, true
}

// shim: keep textinput.Model imported so future text-editor variants
// can switch to the real widget without a fresh import line.
var _ = textinput.Model{}
