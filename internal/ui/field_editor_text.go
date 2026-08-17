package ui

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

type textEditor struct {
	kind textKind
}

func init() {
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
		return true, nil
	case "ctrl+e", "end":
		return true, nil
	case "space", " ":
		s.Raw += " "
		s.Error = ""
		return true, nil
	}
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

type textareaEditor struct{}

func (e *textareaEditor) CanEdit(f sf.Field) bool {
	return f.Updateable && f.CalculatedFormula == "" && !f.AutoNumber
}

func (e *textareaEditor) Init(f sf.Field, current any) EditState {
	raw := stringifyFieldValue(current)
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
	raw = strings.ReplaceAll(raw, "⏎", "\n")
	return CommitValue, raw, nil
}

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

func singleRune(key string) (rune, bool) {
	if utf8.RuneCountInString(key) != 1 {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(key)
	return r, true
}

var _ = textinput.Model{}
