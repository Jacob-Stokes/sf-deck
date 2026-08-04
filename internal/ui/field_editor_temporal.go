package ui

// Date / datetime / time editors. Phase 1 keeps the widget simple
// (textinput buffer) and accepts loose formats — Salesforce's PATCH
// endpoint parses date strings flexibly. We do a light client-side
// sanity check on Commit so obvious typos surface locally before the
// network round-trip; anything Go's time.Parse accepts goes through
// untouched.

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// dateEditor accepts ISO 8601 dates (YYYY-MM-DD) and a few loose
// variants. Salesforce's PATCH accepts ISO; we normalise on Commit.
type dateEditor struct{}

// datetimeEditor accepts ISO 8601 datetimes (YYYY-MM-DDTHH:MM:SS+ZZ).
// Most users type org-local time; SF returns the actual stored UTC.
// We surface a brief hint on the cell to remind them.
type datetimeEditor struct{}

// booleanEditor handles checkbox fields. Space / "t" / "f" / arrow
// keys all flip between true and false. The buffer always reads
// "true" or "false" so the typed value matches what SF expects.
type booleanEditor struct{}

func init() {
	registerFieldEditor(&dateEditor{}, "date")
	registerFieldEditor(&datetimeEditor{}, "datetime")
	registerFieldEditor(&booleanEditor{}, "boolean")
	// time-only fields are rare on standard objects; same widget as
	// date works (loose text format HH:MM:SS).
	registerFieldEditor(&dateEditor{}, "time")
}

// ---- dateEditor -----------------------------------------------

func (e *dateEditor) CanEdit(f sf.Field) bool {
	return f.Updateable && f.CalculatedFormula == "" && !f.AutoNumber
}

func (e *dateEditor) Init(f sf.Field, current any) EditState {
	return EditState{Field: f, Raw: stringifyFieldValue(current)}
}

func (e *dateEditor) RenderEditCell(s *EditState, width int, focused bool) string {
	var body string
	if focused {
		body = lipgloss.NewStyle().Foreground(theme.Fg).Render(s.Raw) +
			lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true).Render("▌")
	} else {
		body = lipgloss.NewStyle().Foreground(theme.FgDim).Render(s.Raw)
	}
	if s.Error != "" {
		body += "  " + lipgloss.NewStyle().Foreground(theme.Red).Render("· "+s.Error)
	} else if focused {
		body += "  " + lipgloss.NewStyle().Foreground(theme.Muted).Render("(YYYY-MM-DD)")
	}
	return body
}

func (e *dateEditor) HandleKey(s *EditState, msg tea.KeyMsg) (bool, tea.Cmd) {
	return handleTextKey(s, msg)
}

// dateLayouts is the small set of formats we accept locally. Only
// UNAMBIGUOUS forms: a slash date like 03/04/2026 could be MM/DD or
// DD/MM and there's no way to know which the user meant, so accepting
// both (first-match-wins) silently stored the wrong date for half the
// audience. We accept ISO (the advertised hint) and year-first slash,
// which can't be misread, and reject ambiguous DD/MM-vs-MM/DD input.
var dateLayouts = []string{
	"2006-01-02",
	"2006/01/02",
	"2006-1-2",
}

func (e *dateEditor) Commit(s *EditState) (CommitMode, any, error) {
	raw := strings.TrimSpace(s.Raw)
	if raw == "" {
		if !s.Field.Nillable {
			s.Error = "required"
			return CommitNone, nil, fmt.Errorf("required")
		}
		return CommitNull, nil, nil
	}
	// Already-ISO from a previous load is the common case.
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return CommitValue, t.Format("2006-01-02"), nil
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return CommitValue, t.Format("2006-01-02"), nil
		}
	}
	s.Error = "not a date"
	return CommitNone, nil, fmt.Errorf("not a date")
}

// ---- datetimeEditor -------------------------------------------

func (e *datetimeEditor) CanEdit(f sf.Field) bool {
	return f.Updateable && f.CalculatedFormula == "" && !f.AutoNumber
}

func (e *datetimeEditor) Init(f sf.Field, current any) EditState {
	return EditState{Field: f, Raw: stringifyFieldValue(current)}
}

func (e *datetimeEditor) RenderEditCell(s *EditState, width int, focused bool) string {
	var body string
	if focused {
		body = lipgloss.NewStyle().Foreground(theme.Fg).Render(s.Raw) +
			lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true).Render("▌")
	} else {
		body = lipgloss.NewStyle().Foreground(theme.FgDim).Render(s.Raw)
	}
	if s.Error != "" {
		body += "  " + lipgloss.NewStyle().Foreground(theme.Red).Render("· "+s.Error)
	} else if focused {
		body += "  " + lipgloss.NewStyle().Foreground(theme.Muted).Render("(ISO 8601; no offset = local time)")
	}
	return body
}

func (e *datetimeEditor) HandleKey(s *EditState, msg tea.KeyMsg) (bool, tea.Cmd) {
	return handleTextKey(s, msg)
}

// datetimeZonedLayouts carry an explicit offset (or Z) — parse as-is.
var datetimeZonedLayouts = []string{
	"2006-01-02T15:04:05.000Z",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05-07:00",
	"2006-01-02T15:04:05+07:00",
}

// datetimeLocalLayouts have NO zone. A bare wall-clock like
// "2026-06-20 09:00" means the user's LOCAL 9am, so it must be parsed
// in time.Local before converting to UTC. time.Parse would treat it as
// UTC and silently write the wrong instant (e.g. 9am PDT stored as 9am
// UTC = 2am local).
var datetimeLocalLayouts = []string{
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
}

func (e *datetimeEditor) Commit(s *EditState) (CommitMode, any, error) {
	raw := strings.TrimSpace(s.Raw)
	if raw == "" {
		if !s.Field.Nillable {
			s.Error = "required"
			return CommitNone, nil, fmt.Errorf("required")
		}
		return CommitNull, nil, nil
	}
	// Normalise to the format SF returns + accepts: UTC ISO 8601 with ms.
	for _, layout := range datetimeZonedLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return CommitValue, t.UTC().Format("2006-01-02T15:04:05.000Z"), nil
		}
	}
	for _, layout := range datetimeLocalLayouts {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return CommitValue, t.UTC().Format("2006-01-02T15:04:05.000Z"), nil
		}
	}
	s.Error = "not a datetime"
	return CommitNone, nil, fmt.Errorf("not a datetime")
}

// ---- booleanEditor --------------------------------------------

func (e *booleanEditor) CanEdit(f sf.Field) bool {
	return f.Updateable
}

func (e *booleanEditor) Init(f sf.Field, current any) EditState {
	raw := "false"
	switch v := current.(type) {
	case bool:
		if v {
			raw = "true"
		}
	case string:
		if strings.EqualFold(v, "true") {
			raw = "true"
		}
	}
	return EditState{Field: f, Raw: raw}
}

func (e *booleanEditor) RenderEditCell(s *EditState, width int, focused bool) string {
	fg := theme.Fg
	if !focused {
		fg = theme.FgDim
	}
	label := "[ ]"
	if s.Raw == "true" {
		label = "[x]"
	}
	body := lipgloss.NewStyle().Foreground(fg).Render(label + " " + s.Raw)
	if focused {
		body += "  " + lipgloss.NewStyle().Foreground(theme.Muted).Render("(space toggles)")
	}
	if s.Error != "" {
		body += "  " + lipgloss.NewStyle().Foreground(theme.Red).Render("· "+s.Error)
	}
	return body
}

func (e *booleanEditor) HandleKey(s *EditState, msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "space", " ", "x", "X", "t", "T", "up", "down":
		if s.Raw == "true" {
			s.Raw = "false"
		} else {
			s.Raw = "true"
		}
		s.Error = ""
		return true, nil
	case "f", "F":
		s.Raw = "false"
		s.Error = ""
		return true, nil
	}
	return false, nil
}

func (e *booleanEditor) Commit(s *EditState) (CommitMode, any, error) {
	return CommitValue, s.Raw == "true", nil
}

// ---- shared text key handler ----------------------------------

// handleTextKey is the routing helper for the temporal editors —
// they share textEditor's printable/backspace/clear/space semantics
// without duplicating the switch. Pulled out so date / datetime /
// future "loose text" types stay aligned.
func handleTextKey(s *EditState, msg tea.KeyMsg) (bool, tea.Cmd) {
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
