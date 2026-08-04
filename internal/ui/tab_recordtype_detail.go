package ui

// TabRecordTypeDetail — full-pane view of a single RecordType. Drill
// from the Record Types subtab's list (enter on a row).
//
// Layout parallels the other drill-detail surfaces: every editable
// property is a navigable MAIN-pane row (active toggle, label,
// description) + a DANGER ZONE delete. Arrow keys walk the rows; Enter
// fires the edit / toggle / delete modal. Sidebar is INFO-ONLY.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// Record-type action indices — track recordTypeActionsFor's order.
const (
	rtActToggleActive = 0
	rtActLabel        = 1
	rtActDescription  = 2
	rtActDelete       = 3
)

// recordTypeDetailRows builds the ordered row model.
func recordTypeDetailRows(sobject, rowDev, rowLabel string, det sf.RecordTypeDetail, meta detailMeta, inner int) []detailRow {
	b := newDetailRowBuilder(inner)

	b.title(sobject + "  /  " + rowDev)
	status := "inactive"
	if det.Active {
		status = "active"
	}
	b.dim("  " + status + "  ·  id " + det.ID + "  ·  " +
		humanAge(meta.FetchedAt) + stateSuffix(meta.Busy, meta.Err))
	b.blank()

	b.title("STATUS")
	b.kv("active", yesNo(det.Active), rtActToggleActive)
	b.blank()

	label := det.Label
	if label == "" {
		label = rowLabel
	}
	b.title("LABEL")
	b.kv("label", label, rtActLabel)
	b.blank()

	if det.BusinessProcess != "" {
		b.title("BUSINESS PROCESS")
		b.kv("process", det.BusinessProcess, noAction)
		b.blank()
	}

	b.title("DESCRIPTION")
	b.kvWrapped("description", det.Description, rtActDescription)

	b.dangerSection("delete record type", rtActDelete)
	return b.rows
}

// renderRecordTypeDetail is the main-pane renderer for
// TabRecordTypeDetail.
func (m Model) renderRecordTypeDetail(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	d := m.ensureOrgDataRef(o.Username)
	if d.DescribeCur == "" || d.RecordTypes.DrillID == "" {
		return theme.Subtle.Render("  press enter on a record type in the Record Types subtab first")
	}
	r, ok := d.RecordTypes.Details[d.RecordTypes.DrillID]
	if !ok || r.FetchedAt().IsZero() {
		if r != nil && r.Busy() {
			return theme.Subtle.Render("  loading record type Metadata…")
		}
		if r != nil && r.Err() != nil {
			return redLine("  " + r.Err().Error())
		}
		return theme.Subtle.Render("  fetching record type Metadata…")
	}
	rows := m.recordTypeDetailRowsFor(d, inner)
	return renderDetailRows(rows, m.recordTypeActionCur, m.focus == focusMain, inner, innerH)
}

// recordTypeDetailRowsFor resolves the row model for the drilled RT.
func (m Model) recordTypeDetailRowsFor(d *orgData, inner int) []detailRow {
	r, ok := d.RecordTypes.Details[d.RecordTypes.DrillID]
	if !ok || r == nil || r.FetchedAt().IsZero() {
		return nil
	}
	var rowDev, rowLabel string
	if lr, ok := d.RecordTypes.Lists[d.DescribeCur]; ok {
		for _, rt := range lr.Value() {
			if rt.ID == d.RecordTypes.DrillID {
				rowDev = rt.DeveloperName
				rowLabel = rt.Name
				break
			}
		}
	}
	if rowDev == "" {
		rowDev = d.RecordTypes.DrillID
	}
	meta := detailMeta{FetchedAt: r.FetchedAt(), Busy: r.Busy(), Err: r.Err()}
	return recordTypeDetailRows(d.DescribeCur, rowDev, rowLabel, r.Value(), meta, inner)
}

func (m Model) recordTypeDetailRowModel() ([]detailRow, bool) {
	o, ok := m.currentOrg()
	if !ok {
		return nil, false
	}
	d := m.data[o.Username]
	if d == nil || d.DescribeCur == "" || d.RecordTypes.DrillID == "" {
		return nil, false
	}
	rows := m.recordTypeDetailRowsFor(d, 60)
	if rows == nil {
		return nil, false
	}
	return rows, true
}

func (m Model) recordTypeDetailNavCount() int {
	rows, ok := m.recordTypeDetailRowModel()
	if !ok {
		return 0
	}
	return len(detailNavIndex(rows))
}

func (m Model) recordTypeDetailActionForCursor() (int, bool) {
	rows, ok := m.recordTypeDetailRowModel()
	if !ok {
		return noAction, false
	}
	return detailActionForCursor(rows, m.recordTypeActionCur)
}

// sidebarRecordTypeActions renders the TabRecordTypeDetail right
// sidebar — a context panel for the cursored row.
func (m Model) sidebarRecordTypeActions(inner int) string {
	ctx := m.recordTypeRowContext()
	ctx.Hints = detailNavHints(true)
	return m.sidebarRowContext("RECORD TYPE · CONTEXT", inner, ctx)
}

func (m Model) recordTypeRowContext() rowContext {
	idx, ok := m.recordTypeDetailActionForCursor()
	if !ok {
		return rowContext{ReadOnlyMsg: "read-only row. The active toggle, label, description + the DANGER ZONE delete carry a ↵ hint."}
	}
	rows := RegistryRows(m, recordTypeRegistry)
	if idx < 0 || idx >= len(rows) {
		return rowContext{}
	}
	a := rows[idx]
	ctx := rowContext{
		Heading: a.Label,
		Help:    a.Hint,
		Routing: "Tooling API · RecordType patch",
		Danger:  idx == rtActDelete,
	}
	if !a.Allowed {
		ctx.Blocked = a.Reason
	}
	switch idx {
	case rtActToggleActive:
		ctx.Affects = "whether the record type is assignable to profiles."
	case rtActLabel:
		ctx.Affects = "the label shown in record-type pickers."
	case rtActDescription:
		ctx.Affects = "the Setup-only description."
	case rtActDelete:
		ctx.Routing = "Tooling API · delete RecordType"
		ctx.Affects = "permanently removes it; every record's RecordTypeId becomes null. No undo."
	}
	if det, ok := m.currentRecordTypeDetail(); ok {
		switch idx {
		case rtActToggleActive:
			ctx.Current = yesNo(det.Active)
		case rtActLabel:
			ctx.Current = dashIfEmpty(det.Label)
		case rtActDescription:
			ctx.Current = dashIfEmpty(det.Description)
		}
	}
	return ctx
}

func (m Model) currentRecordTypeDetail() (sf.RecordTypeDetail, bool) {
	o, ok := m.currentOrg()
	if !ok {
		return sf.RecordTypeDetail{}, false
	}
	d := m.data[o.Username]
	if d == nil || d.RecordTypes.DrillID == "" {
		return sf.RecordTypeDetail{}, false
	}
	r, ok := d.RecordTypes.Details[d.RecordTypes.DrillID]
	if !ok || r == nil || r.FetchedAt().IsZero() {
		return sf.RecordTypeDetail{}, false
	}
	return r.Value(), true
}
