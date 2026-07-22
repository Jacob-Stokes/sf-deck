package ui

// related_record.go — resolve the "related record" sitting behind a
// reference-type field on a record detail surface.
//
// Used by:
//   - Enter on a relationship field → drill into related record
//   - ^O "Open related X" sub-modal → action menu against the
//     related record without leaving the parent
//
// Both rely on three pieces of context:
//   1. The cursored field name (read from d.RecordFieldCursor)
//   2. The field's metadata from the parent sObject's describe
//      (to learn referenceTo[] and field type)
//   3. The actual value of the field on the record (the related Id)
//
// Polymorphic lookups (WhoId, WhatId, OwnerId on most objects) have
// multiple referenceTo entries. We resolve the actual target by the
// Id's 3-char key prefix against the per-org SObjects catalog.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// relatedRecordHit is the resolved (sObject, id) pair behind a
// reference field, plus convenience metadata used by the open menu
// and the Enter-to-drill handler.
type relatedRecordHit struct {
	SObject string // resolved API name (e.g. "Account")
	ID      string // 15- or 18-char SF Id
	Label   string // best-effort human label ("Account 0014I…", "Acme")
	Field   string // the source field on the parent (e.g. "AccountId")
}

// cursoredRelatedRecord inspects the record-detail cursor and returns
// the related record it points at, when:
//   - the cursored field is a reference type
//   - the field has a non-empty value
//   - the referenced sObject can be resolved (single-target lookup,
//     or polymorphic with a key-prefix that matches a cached sObject)
//
// Returns ok=false otherwise — callers should treat that as "no
// drill / no sub-modal here" without surfacing an error.
func (m Model) cursoredRelatedRecord() (relatedRecordHit, bool) {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return relatedRecordHit{}, false
	}
	field := ""
	if d.RecordFieldCursor != nil {
		field = d.RecordFieldCursor[d.RecordDetailCur]
	}
	if field == "" {
		return relatedRecordHit{}, false
	}
	parentSObject, _ := splitRecordKey(d.RecordDetailCur)
	if parentSObject == "" {
		return relatedRecordHit{}, false
	}
	r, ok := d.RecordDetails[d.RecordDetailCur]
	if !ok || r.FetchedAt().IsZero() {
		return relatedRecordHit{}, false
	}
	rec := r.Value()
	if rec == nil {
		return relatedRecordHit{}, false
	}

	// Field metadata — type must be reference. Bail silently if the
	// describe hasn't loaded yet; the next render after Describes
	// lands will pick up the cursor again.
	desc, ok := d.Describes[parentSObject]
	if !ok || desc.FetchedAt().IsZero() {
		return relatedRecordHit{}, false
	}
	parent := desc.Value()
	var fieldMeta *sf.Field
	for i := range parent.Fields {
		if parent.Fields[i].Name == field {
			fieldMeta = &parent.Fields[i]
			break
		}
	}
	if fieldMeta == nil || fieldMeta.Type != "reference" {
		return relatedRecordHit{}, false
	}

	id, _ := rec[field].(string)
	id = strings.TrimSpace(id)
	if id == "" {
		return relatedRecordHit{}, false
	}

	// Resolve the referenced sObject:
	//   1 entry        → use it directly
	//   2+ entries     → key-prefix lookup against d.SObjects
	//   0 entries      → field metadata is broken; bail
	target := ""
	switch len(fieldMeta.ReferenceTo) {
	case 0:
		return relatedRecordHit{}, false
	case 1:
		target = fieldMeta.ReferenceTo[0]
	default:
		target = resolveSObjectByKeyPrefix(d, id, fieldMeta.ReferenceTo)
	}
	if target == "" {
		return relatedRecordHit{}, false
	}

	// Best-effort label. When the SOQL pulled a relationship name
	// like `Account.Name`, the value lives at rec[relationshipName]
	// as a nested map. Otherwise fall back to "<sObject> <id>".
	label := target + " " + id
	if rel := fieldMeta.RelationshipName; rel != "" {
		if nested, ok := rec[rel].(map[string]any); ok {
			if name, ok := nested["Name"].(string); ok && name != "" {
				label = name
			}
		}
	}

	return relatedRecordHit{
		SObject: target,
		ID:      id,
		Label:   label,
		Field:   field,
	}, true
}

// drillIntoRelatedRecord pushes the current record onto the drill
// stack and switches to the related record's detail. Called by the
// Enter handler on TabRecordDetail when the cursor sits on a non-
// null reference field.
//
// The previous record's (sObject, id) is captured BEFORE we
// overwrite d.RecordDetailCur so Esc can pop back to it. The
// original recordDetailReturnTab is preserved across drills — only
// when the stack empties does Esc fall back to it.
func (m *Model) drillIntoRelatedRecord(hit relatedRecordHit) tea.Cmd {
	if m == nil || hit.SObject == "" || hit.ID == "" {
		return nil
	}
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return nil
	}
	prevSobject, prevID := splitRecordKey(d.RecordDetailCur)
	if prevSobject == "" || prevID == "" {
		return nil
	}
	// Don't push a frame when drilling to the same record (degenerate
	// case where the cursor sits on a self-reference like ParentId
	// pointing at the same row — shouldn't happen but cheap to guard).
	if prevSobject == hit.SObject && prevID == hit.ID {
		return nil
	}
	m.recordDrillStack = append(m.recordDrillStack, recordDrillFrame{
		SObject: prevSobject,
		ID:      prevID,
	})
	// triggerRecordDrill resets recordDetailReturnTab — we don't want
	// that here because the original return target (e.g. TabRecords)
	// should persist all the way to the bottom of the stack. Save
	// + restore it across the call.
	savedReturn := m.recordDetailReturnTab
	cmd := m.triggerRecordDrill(hit.SObject, hit.ID, hit.Label, TabRecordDetail)
	m.recordDetailReturnTab = savedReturn
	return cmd
}

// popRecordDrillStack restores the previous record in the drill
// chain. Returns ok=false when the stack is empty — caller falls
// back to the standard EscBack-to-returnTab behaviour.
func (m *Model) popRecordDrillStack() (recordDrillFrame, bool) {
	if m == nil || len(m.recordDrillStack) == 0 {
		return recordDrillFrame{}, false
	}
	last := len(m.recordDrillStack) - 1
	frame := m.recordDrillStack[last]
	m.recordDrillStack = m.recordDrillStack[:last]
	return frame, true
}

// activateRecordDetail is the Activate (Enter) handler for
// TabRecordDetail. Three cases:
//
//   - cursor on a non-null reference field → drill into related
//     record (push onto drill stack, esc unwinds).
//   - cursor on a RELATED panel row → open the SOQL editor with
//     a query that selects every child record of this row's
//     relationship pre-populated. User can edit + run from there.
//   - cursor on a regular field → no-op (Enter on a regular
//     field opens the inline editor, handled separately by the
//     record-edit-session code path).
func (m *Model) activateRecordDetail() tea.Cmd {
	d := m.activeOrgData()
	if d != nil {
		cur := d.RecordFieldCursor[d.RecordDetailCur]
		if IsRelatedCursorKey(cur) {
			return m.openRelatedSOQL(RelatedCursorRelName(cur))
		}
	}
	hit, ok := m.cursoredRelatedRecord()
	if !ok {
		return nil
	}
	return m.drillIntoRelatedRecord(hit)
}

// openRelatedSOQL opens a SOQL modal with a query that selects every
// child record of the cursored RELATED row. e.g. cursor on the
// "Sub-Requests" RELATED row of a Request__c fires
// `SELECT Id, Name FROM Request__c WHERE Parent_Request__c = '<id>'`
// into an independent SOQL editor over the current record detail.
//
// Returns nil + flashes "no related schema" when the parent
// describe isn't loaded (shouldn't normally happen since the
// cursor only lands on RELATED rows when describe IS loaded).
func (m *Model) openRelatedSOQL(relName string) tea.Cmd {
	if relName == "" {
		return nil
	}
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return nil
	}
	parentSobj, parentID := splitRecordKey(d.RecordDetailCur)
	desc, ok := d.Describes[parentSobj]
	if !ok || desc.FetchedAt().IsZero() {
		m.flash("describe not loaded — open SOQL manually")
		return nil
	}
	var child sf.ChildRelationship
	for _, c := range desc.Value().ChildRelationships {
		if c.RelationshipName == relName {
			child = c
			break
		}
	}
	if child.ChildSObject == "" {
		return nil
	}
	// Resolve the child's display field. Most sObjects use "Name", but
	// system objects like Task/Event use Subject, Order uses
	// OrderNumber, etc.  Hardcoding "Name" throws INVALID_FIELD on
	// those.  Prefer the child's own describe (Fields[].NameField is
	// the authoritative flag) when cached; fall back to the curated
	// registry in sf.NameFieldFor; final fallback is "Name" for any
	// custom sObject not in the registry.
	nameField := sf.NameFieldFor(child.ChildSObject)
	if cdesc, ok := d.Describes[child.ChildSObject]; ok && !cdesc.FetchedAt().IsZero() {
		for _, f := range cdesc.Value().Fields {
			if f.NameField {
				nameField = f.Name
				break
			}
		}
	}
	projection := "Id"
	if nameField != "" && nameField != "Id" {
		projection = "Id, " + nameField
	}
	soql := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s = '%s' LIMIT 200",
		projection, child.ChildSObject, child.Field, parentID,
	)
	m.openSOQLModal(child.ChildSObject+" via "+relName, soql)
	return nil
}

// relatedRecordOpenTargetID is the sentinel ID on the synthetic
// "Open related <sObject>…" OpenTarget. fireMenuTarget intercepts
// it and opens a sub-modal whose targets are built against the
// related record (without leaving the parent record's view).
const relatedRecordOpenTargetID = "__related_record_picker__"

// relatedRecordOpenTarget returns the single synthetic target that
// surfaces in the open menu when the cursor sits on a non-null
// reference field. Returns nil when there's no related record to
// open — open menu just shows the standard targets.
//
// Only emits on TabRecordDetail; other surfaces with reference
// fields (record edit forms, FLS detail) can opt in later if it
// proves useful.
func (m Model) relatedRecordOpenTarget() []sf.OpenTarget {
	if m.tab() != TabRecordDetail {
		return nil
	}
	hit, ok := m.cursoredRelatedRecord()
	if !ok {
		return nil
	}
	return []sf.OpenTarget{{
		ID:       relatedRecordOpenTargetID,
		Label:    "Open related " + hit.SObject + " (" + hit.Label + ")",
		Shortcut: "o",
		// No Path / AbsoluteURL — fireMenuTarget intercepts this ID
		// before either is consulted.
	}}
}

// openRelatedRecordMenu replaces the active open menu with a fresh
// menu built against the related record. The user sees the same
// r/e/i/list/manager options they'd see if they were on the
// related record's detail page — but without leaving the current
// drill context.
//
// Esc on the sub-modal returns to the parent menu (handled by
// handleOpenMenuKey via the openMenuStack push).
func (m *Model) openRelatedRecordMenu(mode openMenuMode) tea.Cmd {
	if m == nil {
		return nil
	}
	hit, ok := m.cursoredRelatedRecord()
	if !ok {
		return nil
	}
	o, ok := m.currentOrg()
	if !ok {
		return nil
	}
	// Build a RecordRef for the related record. Use a synthetic
	// attributes block so RecordRef.Targets() can extract the
	// (sObject, id) — same pattern copyRecordWithAttrs uses for
	// list-view rows that don't carry attributes.
	rec := map[string]any{
		"Id": hit.ID,
		"attributes": map[string]any{
			"type": hit.SObject,
		},
	}
	ref := m.newRecordRef(rec)
	targets := ref.Targets()
	if len(targets) == 0 {
		return nil
	}
	// Push the current menu state onto the stack so esc on the sub-
	// modal restores it. When the stack is empty, esc behaves as
	// before (closes the menu).
	if m.openMenu != nil {
		m.openMenuStack = append(m.openMenuStack, *m.openMenu)
	}
	title := "Open · " + hit.Label
	if mode == menuYank {
		title = "Copy URL · " + hit.Label
	}
	m.openMenu = &openMenuState{
		title:   title,
		mode:    mode,
		org:     o,
		source:  ref,
		targets: targets,
		cursor:  0,
	}
	return nil
}

// resolveSObjectByKeyPrefix narrows a polymorphic reference field
// to its actual target sObject by matching the Id's first 3 chars
// against the cached SObjects catalog's KeyPrefix.
//
// candidates is the field's referenceTo list — used to constrain
// matches when the same prefix appears on multiple sObjects (rare;
// typically only standard objects share prefixes with custom
// types that wouldn't be in referenceTo anyway).
//
// Returns "" when:
//   - the id is too short (< 3 chars)
//   - no cached sObject matches the prefix
//   - the matched sObject isn't in the candidates list
//
// Falls back to the first candidate when SObjects isn't cached yet
// — better to attempt the wrong sObject than show nothing; SF will
// return its own "not found" if the guess is wrong.
func resolveSObjectByKeyPrefix(d *orgData, id string, candidates []string) string {
	if len(id) < 3 {
		return ""
	}
	sobjects := d.SObjects.Value()
	if len(sobjects) == 0 {
		if len(candidates) > 0 {
			return candidates[0]
		}
		return ""
	}
	if s, ok := sf.SObjectByKeyPrefix(sobjects, id); ok {
		for _, c := range candidates {
			if c == s.Name {
				return s.Name
			}
		}
	}
	// No allowed candidate matched the prefix; surface the first
	// candidate as a last-resort guess. Better to try and fail than
	// silently drop the action.
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}
