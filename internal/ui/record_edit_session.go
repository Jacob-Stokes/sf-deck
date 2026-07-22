package ui

// Per-(sobject, recordId) edit session. Owns:
//
//   - editing      map[fieldAPIName]*EditState  — currently-focused
//                                                 field's in-progress edit
//   - dirty        map[fieldAPIName]any         — committed local edits
//                                                 awaiting PATCH
//   - saving       bool                         — true while a PATCH
//                                                 is in flight
//   - errors       map[fieldAPIName]string      — last server-side
//                                                 rejection per field
//   - lastError    string                       — last record-level
//                                                 error (validation
//                                                 rules, ENTITY_IS_LOCKED)
//
// Sessions live per-org on orgData (mirroring the rest of the
// per-record state) keyed by "<sobject>:<id>". Allocated lazily on
// first edit gesture and cleared after a successful save's
// re-fetch.

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/services/records"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// recordEditSession is the live edit state for one record. Created
// when the user first presses `e` on a field row.
type recordEditSession struct {
	Sobject  string
	RecordID string

	// Editing is the currently-focused field's live state. Only one
	// field is in edit mode at a time on /record (mirrors how every
	// other form UI works); committing moves the value into Dirty
	// and clears Editing.
	Editing      *EditState
	EditingField string // API name of the field in Editing

	// Dirty holds locally-committed edits awaiting PATCH. Keyed by
	// field API name; value is what UpdateRecord will serialise as
	// JSON (string / number / bool / null / etc).
	Dirty map[string]any

	// Saving is the in-flight PATCH flag. Disables further edits +
	// the ctrl+s gesture while a save is in progress.
	Saving bool

	// Errors carries per-field rejections from the most recent
	// failed PATCH. Cleared on the next attempt.
	Errors map[string]string

	// LastError is a record-level error from the most recent PATCH
	// (validation rule, ENTITY_IS_LOCKED, etc.) — surfaces in the
	// top banner.
	LastError string
}

// editSessionKey is the orgData map key for sessions.
func editSessionKey(sobject, id string) string {
	return sobject + ":" + id
}

// ensureEditSession returns the session for the given record,
// creating it lazily. Allocator stays on orgData so multi-org
// usage doesn't bleed sessions across orgs.
func (d *orgData) ensureEditSession(sobject, id string) *recordEditSession {
	if d.EditSessions == nil {
		d.EditSessions = map[string]*recordEditSession{}
	}
	key := editSessionKey(sobject, id)
	if s, ok := d.EditSessions[key]; ok {
		return s
	}
	s := &recordEditSession{
		Sobject:  sobject,
		RecordID: id,
		Dirty:    map[string]any{},
		Errors:   map[string]string{},
	}
	d.EditSessions[key] = s
	return s
}

// currentEditSession returns the session for the currently-drilled
// record on /record, or nil when no record is drilled in.
func (m Model) currentEditSession() (*orgData, *recordEditSession) {
	d, ok := m.activeOrgState()
	if !ok || d.RecordDetailCur == "" {
		return nil, nil
	}
	sobject, id := splitRecordKey(d.RecordDetailCur)
	if d.EditSessions == nil {
		return d, nil
	}
	return d, d.EditSessions[editSessionKey(sobject, id)]
}

// startFieldEdit begins editing of fieldName. If another field is
// already in edit mode, attempts to commit it first; format errors
// keep focus on the previous field. Returns the editor + state for
// the new edit, or nil if the field isn't editable.
func (m *Model) startFieldEdit(fieldName string) (FieldEditor, *EditState) {
	d, _ := m.activeOrgState()
	if d == nil || d.RecordDetailCur == "" {
		return nil, nil
	}
	sobject, id := splitRecordKey(d.RecordDetailCur)
	rec := d.RecordDetails[d.RecordDetailCur]
	if rec == nil || rec.FetchedAt().IsZero() {
		return nil, nil
	}
	// Find the field describe.
	desc, ok := d.Describes[sobject]
	if !ok || desc.FetchedAt().IsZero() {
		m.flash("describe not loaded — press " + firstPretty(Keys.Refresh) + " to refresh")
		return nil, nil
	}
	var field sf.Field
	for _, f := range desc.Value().Fields {
		if f.Name == fieldName {
			field = f
			break
		}
	}
	if field.Name == "" {
		return nil, nil
	}
	editor := resolveFieldEditor(field)
	if editor == nil || !editor.CanEdit(field) {
		m.flash(fieldName + " is read-only")
		return nil, nil
	}
	session := d.ensureEditSession(sobject, id)
	// Commit any in-flight edit on a different field first.
	if session.Editing != nil && session.EditingField != fieldName {
		m.commitCurrentEdit()
	}
	current := rec.Value()[fieldName]
	state := editor.Init(field, current)
	session.Editing = &state
	session.EditingField = fieldName
	return editor, &state
}

// commitCurrentEdit asks the active field's editor for its commit
// value. CommitValue / CommitNull moves the value to Dirty and
// clears Editing. CommitNone keeps the field in edit mode (used
// when the editor has a local format error to surface).
func (m *Model) commitCurrentEdit() {
	_, session := m.currentEditSession()
	if session == nil || session.Editing == nil {
		return
	}
	editor := resolveFieldEditor(session.Editing.Field)
	if editor == nil {
		return
	}
	mode, value, _ := editor.Commit(session.Editing)
	switch mode {
	case CommitValue:
		session.Dirty[session.EditingField] = value
		session.Editing = nil
		session.EditingField = ""
	case CommitNull:
		session.Dirty[session.EditingField] = nil
		session.Editing = nil
		session.EditingField = ""
	case CommitNone:
		// stay in edit mode; Editor stashed its error on Editing.
	}
}

// cancelCurrentEdit drops the in-progress edit without committing.
// The field reverts to its pre-edit display value.
func (m *Model) cancelCurrentEdit() {
	_, session := m.currentEditSession()
	if session == nil {
		return
	}
	session.Editing = nil
	session.EditingField = ""
}

// discardAllEdits clears every dirty field + cancels any in-flight
// edit. Used by the dirty-prompt's "discard" path when the user
// navigates away.
func (m *Model) discardAllEdits() {
	_, session := m.currentEditSession()
	if session == nil {
		return
	}
	session.Editing = nil
	session.EditingField = ""
	session.Dirty = map[string]any{}
	session.Errors = map[string]string{}
	session.LastError = ""
}

// recordEditSaveMsg lands when a PATCH completes.
type recordEditSaveMsg struct {
	Sobject  string
	RecordID string
	Errors   []sf.FieldError
	Err      error
}

// triggerRecordEditSave commits any open edit, then PATCHes every
// dirty field in one call. The per-org safety policy gates the write:
// the org must allow WriteRecord (the header `[READ]` pill is only a
// visual cue — the gate below is what actually blocks the PATCH).
func (m *Model) triggerRecordEditSave() tea.Cmd {
	m.commitCurrentEdit()
	d, session := m.currentEditSession()
	if d == nil || session == nil || len(session.Dirty) == 0 {
		m.flash("nothing to save")
		return nil
	}
	if session.Saving {
		return nil
	}
	if len(m.orgs) == 0 {
		return nil
	}
	o := m.orgs[m.selected]
	if ok, reason := m.canWriteOrg(o, settings.WriteRecord); !ok {
		m.flash(reason)
		return nil
	}
	session.Saving = true
	// Copy fields out of dirty so the async closure doesn't race
	// against further edits while the PATCH is in flight.
	fields := make(map[string]any, len(session.Dirty))
	for k, v := range session.Dirty {
		fields[k] = v
	}
	sobject := session.Sobject
	id := session.RecordID
	alias := targetArg(o)
	service := m.records
	if service == nil {
		gate := orgwrite.NewGate(func(string) (sf.Org, error) { return o, nil },
			func(org sf.Org) settings.SafetyLevel {
				return m.settings.Resolve(org.Username, settings.OrgKind(org.Kind()), org.Alias)
			})
		service = records.New(gate)
	}
	return func() tea.Msg {
		result, err := service.Update(context.Background(), records.UpdateInput{
			Target: alias, SObject: sobject, ID: id, Fields: fields,
		})
		return recordEditSaveMsg{
			Sobject:  sobject,
			RecordID: id,
			Errors:   result.FieldErrors,
			Err:      err,
		}
	}
}

// applyRecordEditSave processes a recordEditSaveMsg. On success
// the session clears + the record re-fetches. On per-field error,
// the session keeps Dirty intact + records error messages so the
// user can fix + retry.
func (m *Model) applyRecordEditSave(msg recordEditSaveMsg) tea.Cmd {
	d, _ := m.activeOrgState()
	if d == nil || d.EditSessions == nil {
		return nil
	}
	key := editSessionKey(msg.Sobject, msg.RecordID)
	session := d.EditSessions[key]
	if session == nil {
		return nil
	}
	session.Saving = false
	if msg.Err != nil {
		session.LastError = msg.Err.Error()
		return nil
	}
	if len(msg.Errors) > 0 {
		session.Errors = map[string]string{}
		for _, fe := range msg.Errors {
			if len(fe.Fields) > 0 {
				for _, f := range fe.Fields {
					session.Errors[f] = fe.Message
				}
			} else {
				session.LastError = fe.String()
			}
		}
		return nil
	}
	// Success — clear session and re-fetch the record so server-
	// computed fields (formula / audit / triggers that mutated
	// the row) refresh on screen.
	delete(d.EditSessions, key)
	if len(m.orgs) == 0 {
		return nil
	}
	o := m.orgs[m.selected]
	r := d.EnsureRecordDetail(targetArg(o), msg.Sobject, msg.RecordID)
	return r.Refresh(m.cache)
}

// moveRecordFieldCursor advances the /record field cursor by delta.
// Cursor walks the flat ordered field list (groupFieldsForDetail
// sections flattened), wrapping at the ends so j/k cycle.
//
// Initialises to the first field when no cursor is set yet.
// While a field is in Editing, j/k get consumed by the editor's
// HandleKey (the dispatcher in update_keys.go routes them
// through handleRecordEditKey first) — so this only fires in
// normal-nav mode.
func (m *Model) moveRecordFieldCursor(delta int) {
	d, _ := m.activeOrgState()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	r := d.RecordDetails[d.RecordDetailCur]
	if r == nil || r.FetchedAt().IsZero() {
		return
	}
	// Use describe-aware grouping when available so the cursor
	// traversal matches the rendered order. Without this the
	// cursor walks the legacy heuristic order while the renderer
	// uses the describe-driven RELATIONSHIPS placement → cursor
	// "skips" visually because the field indices disagree.
	sobj, _ := splitRecordKey(d.RecordDetailCur)
	var parentDescribe *sf.SObjectDescribe
	if dr, ok := d.Describes[sobj]; ok && !dr.FetchedAt().IsZero() {
		v := dr.Value()
		parentDescribe = &v
	}
	fields := orderedRecordFieldsWithDescribe(r.Value(), parentDescribe)
	if len(fields) == 0 {
		return
	}
	if d.RecordFieldCursor == nil {
		d.RecordFieldCursor = map[string]string{}
	}
	cur := d.RecordFieldCursor[d.RecordDetailCur]
	idx := indexOfString(fields, cur)
	if idx < 0 {
		// No cursor yet — first j moves to the first row, first k
		// moves to the last.
		if delta > 0 {
			idx = 0
		} else {
			idx = len(fields) - 1
		}
	} else {
		// Clamp at the ends. Other lists in sf-deck don't wrap —
		// hitting bottom on j just stays at the bottom row, matching
		// the muscle memory the user already has from /objects /flows
		// etc. Wrapping caused the viewport to teleport when the user
		// overshot the last field by one keystroke.
		idx += delta
		if idx < 0 {
			idx = 0
		}
		if idx >= len(fields) {
			idx = len(fields) - 1
		}
	}
	d.RecordFieldCursor[d.RecordDetailCur] = fields[idx]
}

// orderedRecordFieldsWithDescribe is the describe-aware traversal
// used by moveRecordFieldCursor. When describe is non-nil the
// grouping matches renderRecordDetail's exactly (every reference
// field lands in RELATIONSHIPS, custom lookups included).
//
// When describe is non-nil AND has child relationships, the
// synthetic "__related__:<RelationshipName>" rows are appended so
// the cursor can land on RELATED panel rows. Mirrors the render
// order in tab_record_detail.go.
func orderedRecordFieldsWithDescribe(rec map[string]any, describe *sf.SObjectDescribe) []string {
	sections := groupFieldsForDetailWithDescribe(rec, describe)
	var out []string
	for _, s := range sections {
		out = append(out, s.Keys...)
	}
	if describe != nil {
		for _, c := range describe.ChildRelationships {
			if c.RelationshipName == "" || c.DeprecatedAndHidden {
				continue
			}
			if isSystemChildRelationship(c.RelationshipName) {
				continue
			}
			out = append(out, relatedCursorKey(c.RelationshipName))
		}
	}
	return out
}

// relatedCursorKey returns the synthetic cursor key for a child
// relationship row. Prefixed so the renderer can distinguish it
// from a real field name.
func relatedCursorKey(relName string) string {
	return "__related__:" + relName
}

// IsRelatedCursorKey reports whether a cursor key points at a
// RELATED panel row instead of a regular field.
func IsRelatedCursorKey(s string) bool {
	return strings.HasPrefix(s, "__related__:")
}

// RelatedCursorRelName extracts the relationship name from a
// synthetic cursor key. Returns "" when the key doesn't have the
// related prefix.
func RelatedCursorRelName(s string) string {
	if !IsRelatedCursorKey(s) {
		return ""
	}
	return strings.TrimPrefix(s, "__related__:")
}

// indexOfString is a tiny helper since the codebase has both
// `slices.Index` and ad-hoc lookups; staying local avoids the
// stdlib import churn.
func indexOfString(slice []string, target string) int {
	for i, s := range slice {
		if s == target {
			return i
		}
	}
	return -1
}

// applyReferenceSearch routes a SOSL search request from the
// reference editor through to the network. Result lands as a
// referenceSearchResultMsg which the editor's state catches.
func (m Model) applyReferenceSearch(msg referenceSearchMsg) tea.Cmd {
	if len(m.orgs) == 0 {
		return nil
	}
	alias := targetArg(m.orgs[m.selected])
	return referenceSearchFor(alias, m, msg)
}

// handleRecordEditEnter is the `e` key handler — enters edit mode
// on the cursored field. Resolves the cursored field from
// orgData.RecordFieldCursor; if none yet, initialises to the
// first editable field in the record.
func (m Model) handleRecordEditEnter() (tea.Model, tea.Cmd) {
	d, _ := m.activeOrgState()
	if d == nil || d.RecordDetailCur == "" {
		return m, nil
	}
	field := ""
	if d.RecordFieldCursor != nil {
		field = d.RecordFieldCursor[d.RecordDetailCur]
	}
	if field == "" {
		// No cursor — pick the first editable field as a sensible
		// default so `e` always does something useful.
		r := d.RecordDetails[d.RecordDetailCur]
		if r == nil || r.FetchedAt().IsZero() {
			return m, nil
		}
		sobject, _ := splitRecordKey(d.RecordDetailCur)
		desc, ok := d.Describes[sobject]
		if !ok {
			return m, nil
		}
		rec := r.Value()
		for _, f := range desc.Value().Fields {
			if _, present := rec[f.Name]; !present {
				continue
			}
			editor := resolveFieldEditor(f)
			if editor != nil && editor.CanEdit(f) {
				field = f.Name
				break
			}
		}
		if field == "" {
			m.flash("no editable fields on this record")
			return m, nil
		}
		if d.RecordFieldCursor == nil {
			d.RecordFieldCursor = map[string]string{}
		}
		d.RecordFieldCursor[d.RecordDetailCur] = field
	}
	mm := m
	(&mm).startFieldEdit(field)
	return mm, nil
}

// applyReferenceSearchResult writes hits back to the editor state
// for the field the search was for.
func (m *Model) applyReferenceSearchResult(msg referenceSearchResultMsg) {
	_, session := m.currentEditSession()
	if session == nil || session.Editing == nil {
		return
	}
	if session.EditingField != msg.Field {
		// The user moved focus before the search returned; stale
		// result, drop it.
		return
	}
	state := referenceStateOf(session.Editing)
	state.Loading = false
	if msg.Err != nil {
		session.Editing.Error = msg.Err.Error()
		return
	}
	state.Hits = msg.Hits
	state.HitCursor = 0
	if len(state.Hits) == 0 {
		session.Editing.Error = ""
	}
}

// shim: keep fmt imported as future debug formatters may land here.
var _ = fmt.Sprintf
