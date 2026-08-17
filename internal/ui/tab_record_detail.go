package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func (m *Model) triggerRecordDrill(sobject, id, name string, returnTab Tab) tea.Cmd {
	if sobject == "" || id == "" || len(m.orgs) == 0 {
		return nil
	}
	o := m.orgs[m.selected]
	d := m.ensureOrgData(o.Username)
	d.RecordDetailCur = sobject + ":" + id
	// All callers pass a meaningful returnTab; the previous
	// `returnTab != 0` guard turned a real TabHome (== iota 0) into
	// "unset" and silently overwrote to TabRecords, which is why
	// drilling from /home appeared in the /records overflow slot
	// with no way to get back. Accept the caller's value verbatim;
	// the 0-case can only happen if a caller forgets the parameter
	// entirely, which would be a bug elsewhere worth surfacing
	// rather than papering over.
	m.recordDetailReturnTab = returnTab
	d.EnsureRecordDetail(targetArg(o), sobject, id)
	m.rememberRecentRecord(o.Username, sobject, id, name)
	m.setTab(TabRecordDetail)
	return m.onTabChanged()
}

func (m Model) renderRecordDetail(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	d := m.ensureOrgDataRef(o.Username)
	if d.RecordDetailCur == "" {
		return theme.Subtle.Render("  no record drilled in — press enter on a record from /records, /soql, /reports, or /recent") +
			"\n\n" + dimLine("  esc back", inner)
	}
	sobject, id := splitRecordKey(d.RecordDetailCur)
	// Back hint is shown both at top (so it's visible during long
	// loads + on tall records where the bottom-line hint scrolls off)
	// and at the bottom alongside the full key list. The top variant
	// is intentionally bare ("esc back") — the rich hint with Lightning-
	// open and refresh keys stays at the bottom so it doesn't fight
	// the title for attention. Dynamic ESC label tracks the actual
	// pop target: a parent record (when the user drilled record→record
	// via Enter on a reference field) or the original returnTab (when
	// the drill stack is empty).
	backLabel := m.recordDetailEscLabel()
	r, ok := d.RecordDetails[d.RecordDetailCur]
	if !ok || r.FetchedAt().IsZero() {
		var lines []string
		title := sobject + " · " + id
		lines = append(lines, sectionTitle(title))
		lines = append(lines, dimLine("  "+backLabel, inner))
		lines = append(lines, "")
		if r != nil && r.Busy() {
			lines = append(lines, theme.Subtle.Render("  loading…"))
		} else if r != nil && r.Err() != nil {
			lines = append(lines, redLine("  "+r.Err().Error()))
		} else {
			lines = append(lines, theme.Subtle.Render("  fetching record…"))
		}
		return strings.Join(lines, "\n")
	}
	rec := r.Value()

	displayName := recordDisplayName(rec)
	if displayName == "" {
		displayName = id
	}
	var lines []string
	lines = append(lines, sectionTitle(displayName)+stateSuffix(r.Busy(), r.Err()))
	lines = append(lines, dimLine("  "+sobject+" · "+id+"  ·  "+backLabel, inner))
	if !r.FetchedAt().IsZero() {
		lines = append(lines, dimLine("  refreshed "+humanAge(r.FetchedAt()), inner))
	}
	lines = append(lines, "")

	if hint := m.recordFindHint(); hint != "" {
		lines = append(lines, hint)
	}

	session := d.EditSessions[d.RecordDetailCur]

	if session != nil && (len(session.Dirty) > 0 || session.LastError != "") {
		if session.LastError != "" {
			lines = append(lines, redLine("  save error: "+session.LastError))
		}
		if len(session.Dirty) > 0 {
			save := firstPretty(Keys.RecordEditSave)
			esc := firstPretty(Keys.RecordEditCancelAll)
			lines = append(lines, lipgloss.NewStyle().Foreground(theme.Yellow).
				Render(fmt.Sprintf("  %d unsaved · %s save · %s discard", len(session.Dirty), save, esc)))
		}
		lines = append(lines, "")
	}

	// Body: sectioned KV grid. Fields are grouped semantically and
	// rendered through the shared renderRows viewport so scrolling,
	// cursor-following, and wheel feel match every other list
	// surface (/objects, /flows, /apex). Section titles are inert
	// rows in the flat list — the cursor skips them.
	//
	// When the parent describe is cached, grouping is describe-driven
	// (every field.Type == "reference" lands in RELATIONSHIPS).
	// Otherwise we fall back to the name-heuristic grouping that
	// preceded the describe wiring.
	var parentDescribe *sf.SObjectDescribe
	if dr, ok := d.Describes[sobject]; ok && !dr.FetchedAt().IsZero() {
		v := dr.Value()
		parentDescribe = &v
	}
	resolvedNames := map[string]string{}
	if r, ok := d.RecordReferenceNames[d.RecordDetailCur]; ok && !r.FetchedAt().IsZero() {
		resolvedNames = r.Value()
	}
	sections := groupFieldsForDetailWithDescribe(rec, parentDescribe)
	footer := dimLine(
		"  "+firstPretty(Keys.OpenDefault)+" → open in Lightning · "+
			firstPretty(Keys.YankDefault)+" copy URL · "+
			firstPretty(Keys.RecordEditField)+" edit · "+
			firstPretty(Keys.RecordEditSave)+" save · "+
			firstPretty(Keys.Refresh)+" refresh · "+backLabel, inner)
	if len(sections) == 0 {
		lines = append(lines, theme.Subtle.Render("  no fields"))
		lines = append(lines, "", footer)
		return strings.Join(lines, "\n")
	}

	var allKeys []string
	for _, s := range sections {
		allKeys = append(allKeys, s.Keys...)
	}
	apiCap := inner / 3
	if apiCap < 24 {
		apiCap = 24
	}
	if apiCap > 50 {
		apiCap = 50
	}
	labelW := minLabelWidth(allKeys, 18, apiCap)

	humanW := 22
	if parentDescribe != nil {
		longest := 0
		seen := map[string]bool{}
		for _, s := range sections {
			for _, k := range s.Keys {
				seen[k] = true
			}
		}
		for _, f := range parentDescribe.Fields {
			if !seen[f.Name] {
				continue
			}
			if f.Label == f.Name {
				continue
			}
			if len(f.Label) > longest {
				longest = len(f.Label)
			}
		}
		if longest > 0 {
			humanW = longest
		}
	}
	humanCap := inner / 4
	if humanCap < 14 {
		humanCap = 14
	}
	if humanCap > 30 {
		humanCap = 30
	}
	if humanW > humanCap {
		humanW = humanCap
	}
	cursor := ""
	if d.RecordFieldCursor != nil {
		cursor = d.RecordFieldCursor[d.RecordDetailCur]
	}

	type rowItem struct {
		isSection   bool
		title       string // section title
		field       string // field API name
		isRelated   bool
		relName     string
		relChildObj string
		relField    string
		relCount    int
	}
	var items []rowItem
	sel := 0
	for i, s := range sections {
		if i > 0 {
			items = append(items, rowItem{isSection: true, title: ""}) // blank separator
		}
		items = append(items, rowItem{isSection: true, title: s.Title})
		for _, k := range s.Keys {
			if k == cursor {
				sel = len(items)
			}
			items = append(items, rowItem{field: k})
		}
	}

	var resolvedCounts map[string]int
	if r, ok := d.RecordChildCounts[d.RecordDetailCur]; ok && !r.FetchedAt().IsZero() {
		resolvedCounts = r.Value()
	}
	if parentDescribe != nil && len(parentDescribe.ChildRelationships) > 0 {
		var relatedRows []sf.ChildRelationship
		for _, c := range parentDescribe.ChildRelationships {
			if c.RelationshipName == "" || c.DeprecatedAndHidden {
				continue
			}
			if isSystemChildRelationship(c.RelationshipName) {
				continue
			}
			relatedRows = append(relatedRows, c)
		}
		if len(relatedRows) > 0 {
			items = append(items, rowItem{isSection: true, title: ""})
			items = append(items, rowItem{isSection: true, title: "RELATED"})
			for _, c := range relatedRows {
				count := 0
				if resolvedCounts != nil {
					count = resolvedCounts[c.RelationshipName]
				}
				synthKey := relatedCursorKey(c.RelationshipName)
				if synthKey == cursor {
					sel = len(items)
				}
				items = append(items, rowItem{
					isRelated:   true,
					relName:     c.RelationshipName,
					relChildObj: c.ChildSObject,
					relField:    c.Field,
					relCount:    count,
				})
			}
		}
	}

	const footerHeight = 2
	rowFn := func(i int) string {
		if i < 0 || i >= len(items) {
			return ""
		}
		it := items[i]
		if it.isSection {
			if it.title == "" {
				return ""
			}
			return sectionTitle(it.title)
		}
		if it.isRelated {
			isCur := relatedCursorKey(it.relName) == cursor
			return renderRelatedRow(it.relName, it.relChildObj, it.relField, it.relCount, labelW, inner, isCur)
		}
		return m.renderRecordFieldRow(rec, it.field, labelW, humanW, inner, session, it.field == cursor, parentDescribe, resolvedNames)
	}
	reserved := usedLines(lines)
	rows := renderRows(len(items), sel, innerH, reserved, footerHeight, inner, rowFn)
	lines = append(lines, rows...)
	lines = append(lines, "", footer)
	return strings.Join(lines, "\n")
}

// renderRecordFieldRow draws one field row with edit affordances.
// Layout: "[cursor] LABEL  VALUE [* dirty] [! error] [editor widget]"
//
//	cursor      "▌" when this row is the field cursor, two spaces otherwise
//	LABEL       padded to labelW for column alignment
//	VALUE       the live display value — or the editor widget when
//	            the field is the one currently in Editing
//	* dirty     yellow marker when the field has a committed local
//	            edit awaiting PATCH
//	! error     red marker + message when the most recent save
//	            rejected this field
func (m Model) renderRecordFieldRow(rec map[string]any, fieldName string, labelW, humanW, inner int,
	session *recordEditSession, isCursor bool, describe *sf.SObjectDescribe, resolvedNames map[string]string) string {
	labelStyle := lipgloss.NewStyle().Foreground(theme.Muted).Width(labelW)
	humanLabelStyle := lipgloss.NewStyle().Foreground(theme.FgDim).Width(humanW)
	valueStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	dimValueStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	if isCursor {
		labelStyle = labelStyle.Background(theme.BgAlt).Bold(true)
		humanLabelStyle = humanLabelStyle.Background(theme.BgAlt)
		valueStyle = valueStyle.Background(theme.BgAlt)
		dimValueStyle = dimValueStyle.Background(theme.BgAlt)
	}
	displayField := fieldName
	if len(displayField) > labelW {
		displayField = ansi.Truncate(displayField, labelW, "…")
	}
	humanLabel := ""
	if describe != nil {
		if f := findField(describe, fieldName); f != nil {
			humanLabel = f.Label
		}
	}
	if humanLabel == fieldName {
		humanLabel = ""
	}
	if len(humanLabel) > humanW {
		humanLabel = ansi.Truncate(humanLabel, humanW, "…")
	}

	gapStyle := lipgloss.NewStyle()
	if isCursor {
		gapStyle = gapStyle.Background(theme.BgAlt)
	}
	gap := gapStyle.Render("  ")
	prefix := gap
	if isCursor {
		barStyle := lipgloss.NewStyle().Foreground(theme.BorderHi).Background(theme.BgAlt)
		prefix = barStyle.Render("▌") + gapStyle.Render(" ")
	}

	hlTerms := []string{}
	if buf := m.recordFindBuffer(); strings.TrimSpace(buf) != "" {
		hlTerms = []string{buf}
	}

	renderCell := func(text string, base lipgloss.Style) string {
		if len(hlTerms) == 0 {
			return base.Render(text)
		}
		hl := uilayout.HighlightInStyle(text, hlTerms, base.Width(0))
		wrap := lipgloss.NewStyle().Width(base.GetWidth())
		if isCursor {
			wrap = wrap.Background(theme.BgAlt)
		}
		return wrap.Render(hl)
	}

	if session != nil && session.Editing != nil && session.EditingField == fieldName {
		editor := resolveFieldEditor(session.Editing.Field)
		widget := ""
		if editor != nil {
			widget = editor.RenderEditCell(session.Editing, inner-labelW-humanW-6, true)
		}
		return prefix + renderCell(displayField, labelStyle) + gap +
			renderCell(humanLabel, humanLabelStyle) + gap + widget
	}

	val := formatCell(rec[fieldName])
	vs := valueStyle
	if val == "" {
		val = "—"
		vs = dimValueStyle
	}
	if session != nil {
		if dirty, ok := session.Dirty[fieldName]; ok {
			if dirty == nil {
				val = "(cleared)"
				vs = dimValueStyle
			} else {
				val = formatCell(dirty)
				vs = valueStyle
			}
		}
	}
	// Value column has no fixed Width style but MUST stay single-line
	// to keep the renderRows row-count math correct. Long descriptions
	// + embedded newlines (rich text fields) would otherwise wrap the
	// terminal and push every subsequent row down, leaving the cursor
	// pointing at stale visual rows. Collapse newlines + tabs to spaces,
	// then hard-truncate to the value column width. The full value is
	// still reachable via the field editor.
	valDisplay := collapseWhitespace(val)
	valW := inner - labelW - humanW - 6
	annotationW := 0
	if describe != nil {
		if fieldMeta := findField(describe, fieldName); fieldMeta != nil && fieldMeta.Type == "reference" && val != "" && val != "—" && len(fieldMeta.ReferenceTo) > 0 {
			annotationW = 4 + len(fieldMeta.ReferenceTo[0])
			if len(fieldMeta.ReferenceTo) > 1 {
				annotationW += len(" · polymorphic")
			}
			if resolved := resolvedNames[fieldName]; resolved != "" {
				annotationW += 2 + len(resolved)
			}
		}
	}
	if valW > annotationW+4 {
		valW -= annotationW
	}
	if valW < 8 {
		valW = 8
	}
	if lipgloss.Width(valDisplay) > valW {
		valDisplay = ansi.Truncate(valDisplay, valW, "…")
	}
	valOut := vs.Render(valDisplay)
	if len(hlTerms) > 0 {
		valOut = uilayout.HighlightInStyle(valDisplay, hlTerms, vs)
	}
	body := prefix + renderCell(displayField, labelStyle) + gap +
		renderCell(humanLabel, humanLabelStyle) + gap + valOut

	if describe != nil {
		if fieldMeta := findField(describe, fieldName); fieldMeta != nil && fieldMeta.Type == "reference" {
			body += renderReferenceAnnotation(fieldMeta, val, resolvedNames[fieldName], isCursor)
		}
	}

	if session != nil {
		if _, dirty := session.Dirty[fieldName]; dirty {
			markStyle := lipgloss.NewStyle().Foreground(theme.Yellow)
			if isCursor {
				markStyle = markStyle.Background(theme.BgAlt)
			}
			body += gap + markStyle.Render("*")
		}
		if msg := session.Errors[fieldName]; msg != "" {
			errStyle := lipgloss.NewStyle().Foreground(theme.Red)
			if isCursor {
				errStyle = errStyle.Background(theme.BgAlt)
			}
			body += gap + errStyle.Render("! "+msg)
		}
	}
	if isCursor {
		body = lipgloss.NewStyle().Width(inner).Background(theme.BgAlt).Render(body)
	}
	return body
}

// renderRelatedRow draws one RELATED-panel row showing a child
// relationship summary. Layout: "  Relationship    ChildSObject
// via Field   N rows". Count column is dim when zero, normal
// otherwise, so non-empty children stand out.
//
// labelW is the same label column width used by field rows so the
// RELATED rows align visually with the field grid above. inner is
// the body width for trailing-style truncation.
func renderRelatedRow(relName, childObj, parentField string, count int, labelW, inner int, isCursor bool) string {
	relStyle := lipgloss.NewStyle().Foreground(theme.Muted).Width(labelW)
	objStyle := lipgloss.NewStyle().Foreground(theme.Cyan)
	viaStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	countStr := "—"
	countStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	if count > 0 {
		countStr = fmt.Sprintf("%d", count)
		countStyle = lipgloss.NewStyle().Foreground(theme.Fg)
	}
	if isCursor {
		relStyle = relStyle.Background(theme.BgAlt).Bold(true)
		objStyle = objStyle.Background(theme.BgAlt)
		viaStyle = viaStyle.Background(theme.BgAlt)
		countStyle = countStyle.Background(theme.BgAlt)
	}
	gapStyle := lipgloss.NewStyle()
	if isCursor {
		gapStyle = gapStyle.Background(theme.BgAlt)
	}
	gap := gapStyle.Render("  ")
	prefix := gap
	if isCursor {
		prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Background(theme.BgAlt).Render("▌") + gapStyle.Render(" ")
	}
	body := prefix + relStyle.Render(relName) + gap +
		objStyle.Render(childObj) + gap +
		viaStyle.Render("via "+parentField) + gap +
		countStyle.Render(countStr)
	if isCursor {
		body = lipgloss.NewStyle().Width(inner).Background(theme.BgAlt).Render(body)
	}
	return body
}

func isSystemChildRelationship(name string) bool {
	suffixes := []string{
		"Feeds", "ChangeEvents", "Histories", "Histories__r",
		"Shares", "__Share", "RecordActions",
		"FeedSubscriptionsForEntity", "ProcessInstance", "ProcessSteps",
		"DuplicateRecordItems",
	}
	for _, s := range suffixes {
		if strings.HasSuffix(name, s) || name == s {
			return true
		}
	}
	return false
}

func renderReferenceAnnotation(f *sf.Field, rawValue, resolvedName string, isCursor bool) string {
	if rawValue == "" || rawValue == "—" {
		return ""
	}
	if len(f.ReferenceTo) == 0 {
		return ""
	}
	target := f.ReferenceTo[0]
	suffix := ""
	if len(f.ReferenceTo) > 1 {
		suffix = " · polymorphic"
	}
	arrowStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	targetStyle := lipgloss.NewStyle().Foreground(theme.Cyan)
	nameStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	if isCursor {
		arrowStyle = arrowStyle.Background(theme.BgAlt)
		targetStyle = targetStyle.Background(theme.BgAlt)
		nameStyle = nameStyle.Background(theme.BgAlt)
	}
	out := arrowStyle.Render("  → ") + targetStyle.Render(target+suffix)
	if resolvedName != "" {
		out += arrowStyle.Render("  ") + nameStyle.Render(resolvedName)
	}
	return out
}

func collapseWhitespace(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// findField is a small helper for renderRecordFieldRow's reference-
// annotation path. Linear scan because describes typically have
// 100-300 fields and this runs once per visible row per render.
func findField(d *sf.SObjectDescribe, name string) *sf.Field {
	if d == nil {
		return nil
	}
	for i := range d.Fields {
		if d.Fields[i].Name == name {
			return &d.Fields[i]
		}
	}
	return nil
}

type detailSection struct {
	Title string
	Keys  []string
}

// groupFieldsForDetail bins record fields into semantic sections so
// dense records read top-to-bottom rather than as one alphabetical
// blob.
//
// When describe metadata is available, RELATIONSHIPS is sourced
// authoritatively from field.Type == "reference" rather than from
// name heuristics — every lookup / master-detail field, including
// custom ones like Preferred_Carrier__c that wouldn't match the *Id /
// *ParentId pattern, lands in the right bucket. Other sections
// (identifiers, address, dates, audit) stay heuristic since they
// don't have a single describe flag to key off.
//
// describe is the parent sObject's describe (nil = describe not
// yet cached; the renderer falls back to the legacy name-heuristic
// grouping until it lands).
func groupFieldsForDetailWithDescribe(rec map[string]any, describe *sf.SObjectDescribe) []detailSection {
	relationshipFields := map[string]bool{}
	if describe != nil {
		for _, f := range describe.Fields {
			if f.Type == "reference" {
				relationshipFields[f.Name] = true
			}
		}
	}
	return groupFieldsForDetailCore(rec, relationshipFields)
}

func groupFieldsForDetailCore(rec map[string]any, relationshipFields map[string]bool) []detailSection {
	seen := map[string]bool{"attributes": true}
	pick := func(keys ...string) []string {
		var out []string
		for _, k := range keys {
			if seen[k] {
				continue
			}
			if _, ok := rec[k]; ok {
				out = append(out, k)
				seen[k] = true
			}
		}
		return out
	}

	identifiers := pick("Id", "Name", "Subject", "CaseNumber",
		"DeveloperName", "MasterLabel", "Title")
	classification := pick("RecordTypeId", "Type", "Stage",
		"StageName", "Status", "Sub_Status__c", "Priority", "Severity",
		"Reason", "Origin")
	ownership := pick("OwnerId", "AccountId", "ContactId",
		"OpportunityId", "ParentId", "Manager__c", "AssignedTo__c")
	var relExtra []string
	if relationshipFields != nil {
		var names []string
		for name := range relationshipFields {
			if seen[name] {
				continue
			}
			if _, ok := rec[name]; !ok {
				continue
			}
			names = append(names, name)
		}
		sort.Strings(names)
		for _, n := range names {
			relExtra = append(relExtra, n)
			seen[n] = true
		}
	}
	ownership = append(ownership, relExtra...)
	address := pickWithSuffix(rec, seen, []string{
		"Street", "City", "State", "PostalCode", "Country",
		"Latitude", "Longitude", "GeocodeAccuracy",
	})
	dates := pickWithSuffix(rec, seen, []string{
		"Date", "Date__c", "DateTime", "DateTime__c",
	})
	audit := pick("CreatedDate", "CreatedById", "LastModifiedDate",
		"LastModifiedById", "SystemModstamp", "LastViewedDate",
		"LastReferencedDate")

	rest := make([]string, 0, len(rec))
	for k := range rec {
		if seen[k] {
			continue
		}
		rest = append(rest, k)
	}
	sort.Strings(rest)

	sections := []detailSection{
		{Title: "IDENTIFIERS", Keys: identifiers},
		{Title: "CLASSIFICATION", Keys: classification},
		{Title: "RELATIONSHIPS", Keys: ownership},
		{Title: "ADDRESS", Keys: address},
		{Title: "DATES", Keys: dates},
		{Title: "OTHER", Keys: rest},
		{Title: "AUDIT", Keys: audit},
	}
	out := sections[:0]
	for _, s := range sections {
		if len(s.Keys) > 0 {
			out = append(out, s)
		}
	}
	return out
}

func pickWithSuffix(rec map[string]any, seen map[string]bool, suffixes []string) []string {
	var out []string
	keys := make([]string, 0, len(rec))
	for k := range rec {
		if seen[k] {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		base := strings.TrimSuffix(k, "__c")
		for _, suf := range suffixes {
			if strings.HasSuffix(base, suf) {
				out = append(out, k)
				seen[k] = true
				break
			}
		}
	}
	return out
}

// minLabelWidth picks a label-column width given the longest key but
// clamps to a reasonable range so very long API names don't push the
// values off-screen on narrow terminals.
func minLabelWidth(keys []string, min, max int) int {
	w := min
	for _, k := range keys {
		if len(k) > w {
			w = len(k)
		}
	}
	if w > max {
		w = max
	}
	return w
}

func (m Model) recordDetailEscLabel() string {
	if n := len(m.recordDrillStack); n > 0 {
		parent := m.recordDrillStack[n-1]
		label := parentRecordDisplayLabel(m, parent)
		if label != "" {
			return "esc → " + label
		}
		return "esc back to parent"
	}
	if m.recordDetailReturnTab != TabRecordDetail {
		if name := m.recordDetailReturnTab.String(); name != "" {
			return "esc → /" + name
		}
	}
	return "esc back"
}

func parentRecordDisplayLabel(m Model, f recordDrillFrame) string {
	if f.SObject == "" || f.ID == "" {
		return ""
	}
	d := m.activeOrgData()
	if d != nil {
		key := f.SObject + ":" + f.ID
		if r, ok := d.RecordDetails[key]; ok && !r.FetchedAt().IsZero() {
			if name := recordDisplayName(r.Value()); name != "" && name != f.ID {
				return f.SObject + " " + name
			}
		}
	}
	id := f.ID
	if len(id) > 15 {
		id = id[:15]
	}
	return f.SObject + " " + id
}

func splitRecordKey(key string) (sobject, id string) {
	idx := strings.Index(key, ":")
	if idx < 0 {
		return "", key
	}
	return key[:idx], key[idx+1:]
}

func recordRefForDrill(d *orgData, m Model) *sf.RecordRef {
	if d == nil || d.RecordDetailCur == "" {
		return nil
	}
	r, ok := d.RecordDetails[d.RecordDetailCur]
	if !ok || r.FetchedAt().IsZero() {
		return nil
	}
	rec := r.Value()
	if rec == nil {
		return nil
	}
	ref := m.newRecordRef(rec)
	return &ref
}

var _ = fmt.Sprintf
