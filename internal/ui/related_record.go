package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

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

func (m *Model) popRecordDrillStack() (recordDrillFrame, bool) {
	if m == nil || len(m.recordDrillStack) == 0 {
		return recordDrillFrame{}, false
	}
	last := len(m.recordDrillStack) - 1
	frame := m.recordDrillStack[last]
	m.recordDrillStack = m.recordDrillStack[:last]
	return frame, true
}

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
	}}
}

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
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}
