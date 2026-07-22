package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func (m *Model) cycleRecordsChip(delta int) tea.Cmd {
	if len(m.orgs) == 0 {
		return nil
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	sobj := d.RecordsSObjectCur
	if sobj != "" {
		chips := recordsChips(*m, d, sobj)
		navChips := withoutOverflow(chips)
		if len(navChips) == 0 {
			return nil
		}
		cur := findChipIndex(navChips, selectedRecordsChip(d, sobj))
		cur = wrapIdx(cur+delta, len(navChips))
		d.ListViewCur[sobj] = navChips[cur].ID
		return m.onTabChanged()
	}
	return nil
}

func (m *Model) moveRecordsCursor(delta int) {
	if len(m.orgs) == 0 {
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	if d.RecordsSObjectCur == "" {
		d.SObjectList.MoveBy(delta)
		return
	}
	recordsMoveCursor(d, d.RecordsSObjectCur, delta)
}

func (m Model) recordsSearchPtr() *searchState {
	if len(m.orgs) == 0 {
		return nil
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil {
		return nil
	}
	if d.RecordsSObjectCur == "" {
		return d.SObjectList.SearchPtr()
	}
	return d.RecordsSearchPtr(d.RecordsSObjectCur, selectedRecordsChip(d, d.RecordsSObjectCur))
}

func (m *Model) resetRecordsCursor() {
	if len(m.orgs) == 0 {
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	if d.RecordsSObjectCur == "" {
		d.SObjectList.ResetCursor()
		return
	}
	sobj := d.RecordsSObjectCur
	visible, visibleIdx := visibleRecordsAndIdx(d, sobj)
	if len(visibleIdx) == 0 {
		d.Cursors.Reset(cursorKindRecordsRow, sobj)
		return
	}
	recordsRowAdapter(d, sobj, visible, visibleIdx).ResetDisplayTop()
}

func (m *Model) ensureRecordsData(d *orgData, o sf.Org) tea.Cmd {
	cmds := []tea.Cmd{d.SObjects.Ensure(m.cache)}
	if d.RecordsSObjectCur != "" {
		r := d.EnsureRecords(targetArg(o), d.RecordsSObjectCur)
		cmds = append(cmds, r.Ensure(m.cache))
	}
	return tea.Batch(cmds...)
}

func (m Model) refreshRecordsData(d *orgData) tea.Cmd {
	if d.RecordsSObjectCur == "" {
		return d.SObjects.Refresh(m.cache)
	}
	return m.activeChipRefreshCmd(d, d.RecordsSObjectCur)
}

func (m *Model) activateRecords() tea.Cmd {
	if len(m.orgs) == 0 {
		return nil
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	if d.RecordsSObjectCur == "" {
		selected, ok := d.SObjectList.Selected()
		if !ok {
			return nil
		}
		d.RecordsSObjectCur = selected.Name
		if s := d.SObjectList.SearchPtr(); s.Active {
			s.Active = false
			s.Committed = s.Buffer() != ""
		}
		return m.onTabChanged()
	}
	idx := recordsCursorDisplay(d, d.RecordsSObjectCur)
	rec, ok := currentRecordAt(d, d.RecordsSObjectCur, idx)
	if !ok {
		return nil
	}
	id, _ := rec["Id"].(string)
	if id == "" {
		return nil
	}
	name := recordDisplayName(rec)
	return m.triggerRecordDrill(d.RecordsSObjectCur, id, name, TabRecords)
}

func (m *Model) ensureRecordDetailData(d *orgData, o sf.Org) tea.Cmd {
	if d.RecordDetailCur == "" {
		return nil
	}
	sobj, id := splitRecordKey(d.RecordDetailCur)
	if sobj == "" || id == "" {
		return nil
	}
	alias := targetArg(o)
	r := d.EnsureRecordDetail(alias, sobj, id)
	// Also kick the describe — record→record drill on a reference
	// field consults the describe to learn referenceTo[] for the
	// new sObject. Without this, the second drill would silently
	// no-op because cursoredRelatedRecord bails when the describe
	// hasn't loaded. Ensure is cheap when already cached.
	desc := d.EnsureDescribe(alias, sobj)
	cmds := []tea.Cmd{r.Ensure(m.cache), desc.Ensure(m.cache)}
	// Reference-name + child-count resources require the describe
	// to be cached (they read it at Ensure time). When the describe
	// is already hot the Ensure helpers return non-nil; when it's
	// cold they return nil and the describe-Apply route picks up
	// the work the moment the describe lands.
	if refs := d.EnsureRecordReferenceNames(alias, sobj, id); refs != nil {
		cmds = append(cmds, refs.Ensure(m.cache))
	}
	if counts := d.EnsureRecordChildCounts(alias, sobj, id); counts != nil {
		cmds = append(cmds, counts.Ensure(m.cache))
	}
	return tea.Batch(cmds...)
}

func (m Model) refreshRecordDetailData(d *orgData) tea.Cmd {
	if d.RecordDetailCur != "" {
		if r, ok := d.RecordDetails[d.RecordDetailCur]; ok {
			return r.Refresh(m.cache)
		}
	}
	return nil
}
