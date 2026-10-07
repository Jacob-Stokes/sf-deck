package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func (m Model) recordFindBuffer() string {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" || d.RecordFindBuffer == nil {
		return ""
	}
	return d.RecordFindBuffer[d.RecordDetailCur]
}

func (m Model) recordFindActive() bool {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" || d.RecordFindActive == nil {
		return false
	}
	return d.RecordFindActive[d.RecordDetailCur]
}

// openRecordFind transitions the active record into find-input
// mode. Resets the buffer if find wasn't already active so each
// "press /" starts fresh; otherwise re-focuses the existing buffer
// so the user can refine.
func (m *Model) openRecordFind() {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	if d.RecordFindBuffer == nil {
		d.RecordFindBuffer = map[string]string{}
	}
	if d.RecordFindActive == nil {
		d.RecordFindActive = map[string]bool{}
	}
	if !d.RecordFindActive[d.RecordDetailCur] {
		d.RecordFindBuffer[d.RecordDetailCur] = ""
	}
	d.RecordFindActive[d.RecordDetailCur] = true
}

func (m *Model) closeRecordFind() {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	if d.RecordFindActive != nil {
		d.RecordFindActive[d.RecordDetailCur] = false
	}
}

func (m *Model) clearRecordFind() {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	if d.RecordFindActive != nil {
		d.RecordFindActive[d.RecordDetailCur] = false
	}
	if d.RecordFindBuffer != nil {
		d.RecordFindBuffer[d.RecordDetailCur] = ""
	}
}

func (m Model) recordFindHint() string {
	buf := m.recordFindBuffer()
	if buf == "" && !m.recordFindActive() {
		return ""
	}
	if m.recordFindActive() {
		body := "  find: " + buf + "_"
		return lipgloss.NewStyle().Foreground(theme.Yellow).Render(body) +
			lipgloss.NewStyle().Foreground(theme.FgDim).Render("   ↵ commit · esc exit · C clear")
	}
	body := "  find: " + buf
	return lipgloss.NewStyle().Foreground(theme.Yellow).Render(body) +
		lipgloss.NewStyle().Foreground(theme.FgDim).Render("   n / N cycle · C clear")
}

func (m Model) onRecordDetailFindKey(key string) (Model, tea.Cmd, bool) {
	if m.tab() != TabRecordDetail {
		return m, nil, false
	}
	if m.recordFindActive() {
		switch key {
		case "esc":
			mm := m
			(&mm).closeRecordFind()
			return mm, nil, true
		case "enter":
			mm := m
			(&mm).commitRecordFind()
			return mm, nil, true
		case "backspace":
			mm := m
			(&mm).backspaceRecordFind()
			return mm, nil, true
		case "C":
			// Capital C clears the buffer + exits input mode. Wins
			// over the printable-append fallback below so it never
			// gets appended as a literal "C".
			mm := m
			(&mm).clearRecordFind()
			return mm, nil, true
		}
		if len(key) == 1 && key[0] >= 0x20 && key[0] < 0x7f {
			mm := m
			(&mm).appendRecordFindRune(rune(key[0]))
			return mm, nil, true
		}
		return m, nil, false
	}
	switch key {
	case "/":
		mm := m
		(&mm).openRecordFind()
		return mm, nil, true
	case "n":
		if m.recordFindBuffer() == "" {
			return m, nil, false
		}
		mm := m
		(&mm).jumpToRecordFindMatch(false)
		return mm, nil, true
	case "N":
		if m.recordFindBuffer() == "" {
			return m, nil, false
		}
		mm := m
		(&mm).jumpToRecordFindPrev()
		return mm, nil, true
	case "C":
		if m.recordFindBuffer() == "" {
			return m, nil, false
		}
		mm := m
		(&mm).clearRecordFind()
		return mm, nil, true
	}
	return m, nil, false
}

func (m *Model) commitRecordFind() {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	if d.RecordFindActive != nil {
		d.RecordFindActive[d.RecordDetailCur] = false
	}
}

func (m *Model) appendRecordFindRune(r rune) {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	if d.RecordFindBuffer == nil {
		d.RecordFindBuffer = map[string]string{}
	}
	d.RecordFindBuffer[d.RecordDetailCur] += string(r)
	m.jumpToRecordFindMatch(true)
}

func (m *Model) backspaceRecordFind() {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	if d.RecordFindBuffer == nil {
		return
	}
	buf := d.RecordFindBuffer[d.RecordDetailCur]
	if buf == "" {
		return
	}
	d.RecordFindBuffer[d.RecordDetailCur] = buf[:len(buf)-1]
	m.jumpToRecordFindMatch(true)
}

func (m *Model) jumpToRecordFindMatch(fromCurrent bool) {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	query := ""
	if d.RecordFindBuffer != nil {
		query = d.RecordFindBuffer[d.RecordDetailCur]
	}
	if strings.TrimSpace(query) == "" {
		return
	}
	r := d.RecordDetails[d.RecordDetailCur]
	if r == nil || r.FetchedAt().IsZero() {
		return
	}
	sobj, _ := splitRecordKey(d.RecordDetailCur)
	sfDescribe, labels := recordFindDescribe(d, sobj)
	fields := orderedRecordFieldsWithDescribe(r.Value(), sfDescribe)
	if len(fields) == 0 {
		return
	}
	cur := ""
	if d.RecordFieldCursor != nil {
		cur = d.RecordFieldCursor[d.RecordDetailCur]
	}
	curIdx := indexOfString(fields, cur)
	startIdx := 0
	if curIdx >= 0 {
		startIdx = curIdx
		if !fromCurrent {
			startIdx = curIdx + 1
		}
	}
	rec := r.Value()
	q := strings.ToLower(query)
	for offset := 0; offset < len(fields); offset++ {
		i := (startIdx + offset) % len(fields)
		if recordFieldMatchesQuery(fields[i], rec, labels, q) {
			if d.RecordFieldCursor == nil {
				d.RecordFieldCursor = map[string]string{}
			}
			d.RecordFieldCursor[d.RecordDetailCur] = fields[i]
			return
		}
	}
}

func (m *Model) jumpToRecordFindPrev() {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	query := ""
	if d.RecordFindBuffer != nil {
		query = d.RecordFindBuffer[d.RecordDetailCur]
	}
	if strings.TrimSpace(query) == "" {
		return
	}
	r := d.RecordDetails[d.RecordDetailCur]
	if r == nil || r.FetchedAt().IsZero() {
		return
	}
	sobj, _ := splitRecordKey(d.RecordDetailCur)
	sfDescribe, labels := recordFindDescribe(d, sobj)
	fields := orderedRecordFieldsWithDescribe(r.Value(), sfDescribe)
	if len(fields) == 0 {
		return
	}
	cur := ""
	if d.RecordFieldCursor != nil {
		cur = d.RecordFieldCursor[d.RecordDetailCur]
	}
	curIdx := indexOfString(fields, cur)
	startIdx := curIdx - 1
	if startIdx < 0 {
		startIdx = len(fields) - 1
	}
	rec := r.Value()
	q := strings.ToLower(query)
	for offset := 0; offset < len(fields); offset++ {
		i := (startIdx - offset + len(fields)) % len(fields)
		if recordFieldMatchesQuery(fields[i], rec, labels, q) {
			if d.RecordFieldCursor == nil {
				d.RecordFieldCursor = map[string]string{}
			}
			d.RecordFieldCursor[d.RecordDetailCur] = fields[i]
			return
		}
	}
}

func recordFieldMatchesQuery(fieldName string, rec map[string]any, labels map[string]string, q string) bool {
	if strings.Contains(strings.ToLower(fieldName), q) {
		return true
	}
	if label, ok := labels[fieldName]; ok && label != "" {
		if strings.Contains(strings.ToLower(label), q) {
			return true
		}
	}
	if v, ok := rec[fieldName]; ok && v != nil {
		val := formatCell(v)
		if strings.Contains(strings.ToLower(val), q) {
			return true
		}
	}
	return false
}

func recordFindDescribe(d *orgData, sobject string) (*sf.SObjectDescribe, map[string]string) {
	if d == nil || sobject == "" {
		return nil, nil
	}
	r, ok := d.Describes[sobject]
	if !ok || r.FetchedAt().IsZero() {
		return nil, nil
	}
	desc := r.Value()
	labels := map[string]string{}
	for _, f := range desc.Fields {
		if f.Label != "" && f.Label != f.Name {
			labels[f.Name] = f.Label
		}
	}
	return &desc, labels
}
