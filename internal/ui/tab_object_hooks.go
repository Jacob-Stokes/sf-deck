package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

func (m *Model) cycleObjectDetailChip(delta int) tea.Cmd {
	if len(m.orgs) == 0 {
		return nil
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	switch m.currentSubtab() {
	case SubtabSchema:
		// ←/→ cycles the field-filter chip (All / Custom / Picklist / …).
		m.cycleSchemaChip(delta)
		return m.onTabChanged()
	case SubtabRecords:
		sobj := d.DescribeCur
		if sobj == "" {
			return nil
		}
		chips := recordsChips(*m, d, sobj)
		if len(chips) == 0 {
			return nil
		}
		navChips := withoutOverflow(chips)
		if len(navChips) == 0 {
			return nil
		}
		cur := findChipIndex(navChips, selectedRecordsChip(d, sobj))
		cur = wrapIdx(cur+delta, len(navChips))
		d.ListViewCur[sobj] = navChips[cur].ID
		return m.onTabChanged()
	case SubtabFLS:
		perms := d.PermissionSets.Value()
		if len(perms) == 0 {
			return nil
		}
		cur := 0
		for i, p := range perms {
			if p.ID == d.FLSParentID {
				cur = i
				break
			}
		}
		cur = wrapIdx(cur+delta, len(perms))
		d.FLSParentID = perms[cur].ID
		return m.onTabChanged()
	}
	return nil
}

func (m Model) objectDetailReloadOnSwitch(idx int) bool {
	subs := objectDrillSubtabs()
	if idx < 0 || idx >= len(subs) {
		return false
	}
	switch subs[idx].ID {
	case SubtabRecords, SubtabValidation, SubtabRecordTypes, SubtabTriggers, SubtabFLS,
		SubtabObjectLayouts, SubtabObjectFlows:
		return true
	}
	return false
}

func (m *Model) moveObjectDetailCursor(delta int) {
	if len(m.orgs) == 0 {
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	if d.DescribeCur == "" {
		return
	}
	switch m.currentSubtab() {
	case SubtabDetails:
		// Cursor walks the navigable rows of the MAIN pane (identity /
		// features / fields), not the sidebar action menu. The sidebar
		// is info-only now.
		n := m.objectDetailNavCount()
		m.objectActionCur = clampDelta(m.objectActionCur, delta, n)
	case SubtabSchema:
		r := d.Describes[d.DescribeCur]
		if r == nil {
			return
		}
		// Move through the shared ListView (cursor clamps to the
		// filtered slice automatically).
		fs := d.syncFieldList(d.DescribeCur, r.Value().Fields)
		fs.List.MoveBy(delta)
	case SubtabValidation:
		vr := d.ValidationRules.Lists[d.DescribeCur]
		if vr == nil {
			return
		}
		n := len(vr.Value())
		d.ValidationRules.Cursors[d.DescribeCur] = clampDelta(d.ValidationRules.Cursors[d.DescribeCur], delta, n)
	case SubtabRecordTypes:
		rt := d.RecordTypes.Lists[d.DescribeCur]
		if rt == nil {
			return
		}
		n := len(rt.Value())
		d.RecordTypes.Cursors[d.DescribeCur] = clampDelta(d.RecordTypes.Cursors[d.DescribeCur], delta, n)
	case SubtabObjectLayouts:
		pl := d.PageLayouts.Lists[d.DescribeCur]
		if pl == nil {
			return
		}
		n := len(pl.Value())
		d.PageLayouts.Cursors[d.DescribeCur] = clampDelta(d.PageLayouts.Cursors[d.DescribeCur], delta, n)
	case SubtabObjectFlows:
		fl := d.ObjectFlows.Lists[d.DescribeCur]
		if fl == nil {
			return
		}
		n := len(fl.Value())
		d.ObjectFlows.Cursors[d.DescribeCur] = clampDelta(d.ObjectFlows.Cursors[d.DescribeCur], delta, n)
	case SubtabTriggers:
		tr := d.Triggers.Lists[d.DescribeCur]
		if tr == nil {
			return
		}
		n := len(tr.Value())
		d.Triggers.Cursors[d.DescribeCur] = clampDelta(d.Triggers.Cursors[d.DescribeCur], delta, n)
	case SubtabFLS:
		r := d.Describes[d.DescribeCur]
		if r == nil {
			return
		}
		n := len(r.Value().Fields)
		d.Cursors.Move(cursorKindFLS, delta, n, d.DescribeCur, d.FLSParentID)
	case SubtabRecords:
		recordsMoveCursor(d, d.DescribeCur, delta)
	}
}

func (m Model) objectDetailSearchPtr() *searchState {
	if len(m.orgs) == 0 {
		return nil
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil || d.DescribeCur == "" {
		return nil
	}
	switch m.currentSubtab() {
	case SubtabSchema:
		return d.FieldState(d.DescribeCur).List.SearchPtr()
	case SubtabRecords:
		return d.RecordsSearchPtr(d.DescribeCur, selectedRecordsChip(d, d.DescribeCur))
	}
	return nil
}

func (m *Model) resetObjectDetailCursor() {
	if len(m.orgs) == 0 {
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	if d.DescribeCur == "" {
		return
	}
	// Each subtab inside the object drill owns its own cursor —
	// Schema's field cursor, Records' per-(sobject, chip) row
	// cursor, etc.  Reset whichever the user is currently on so
	// gestures like column-sort and search-clear visibly snap the
	// view to row 0.  Without subtab-aware reset, sorting on the
	// Records subtab silently keeps the cursor glued to its old
	// row (which now sits at a different visual position post-
	// sort) — the user can't see that anything happened.
	switch m.currentSubtab() {
	case SubtabSchema:
		d.FieldState(d.DescribeCur).List.ResetCursor()
	case SubtabRecords:
		sobj := d.DescribeCur
		visible, visibleIdx := visibleRecordsAndIdx(d, sobj)
		if len(visibleIdx) == 0 {
			d.Cursors.Reset(cursorKindRecordsRow, sobj)
			break
		}
		recordsRowAdapter(d, sobj, visible, visibleIdx).ResetDisplayTop()
	default:
		// Other subtabs (Details, FLS, Validation, RecordTypes,
		// Triggers) don't have a row cursor in the usual sense —
		// they're action menus or detail panes.  No-op.
	}
}

func (m *Model) ensureObjectDetailData(d *orgData, o sf.Org) tea.Cmd {
	cmds := []tea.Cmd{d.SObjects.Ensure(m.cache)}
	if d.DescribeCur == "" {
		return tea.Batch(cmds...)
	}
	r := d.EnsureDescribe(targetArg(o), d.DescribeCur)
	cmds = append(cmds, r.Ensure(m.cache))
	// Prefetch the CustomObject baseline (Tooling) so the object-
	// action toggles know each flag's current state by the time
	// the user opens an action modal. NoCache and 10-min TTL on
	// the resource itself, so this fires once per session per
	// object visit but won't hammer the API.
	//
	// Standard objects (Account, Contact, etc.) don't have a
	// CustomObject row — they live in platform metadata, not the
	// Tooling CustomObject table. Skipping the fetch here avoids
	// the "no CustomObject row for X" error toast that otherwise
	// surfaces on every standard-object visit. Custom-suffix
	// detection is synchronous; the authoritative describe-Custom
	// flag isn't available until the describe lands but the
	// suffix is a reliable proxy (every custom object ends in __c,
	// and only custom objects do).
	if strings.HasSuffix(d.DescribeCur, "__c") {
		bl := d.EnsureCustomObjectBaseline(targetArg(o), d.DescribeCur)
		cmds = append(cmds, bl.Ensure(m.cache))
	}
	switch m.currentSubtab() {
	case SubtabRecords:
		cmds = append(cmds, m.ensureObjectDetailRecordsData(d, o)...)
	case SubtabValidation:
		vr := d.EnsureValidationRules(targetArg(o), d.DescribeCur)
		cmds = append(cmds, vr.Ensure(m.cache))
	case SubtabRecordTypes:
		rt := d.EnsureRecordTypes(targetArg(o), d.DescribeCur)
		cmds = append(cmds, rt.Ensure(m.cache))
	case SubtabObjectLayouts:
		pl := d.PageLayouts.EnsureList(d.username, targetArg(o), d.DescribeCur)
		cmds = append(cmds, pl.Ensure(m.cache))
	case SubtabObjectFlows:
		fl := d.ObjectFlows.EnsureList(d.username, targetArg(o), d.DescribeCur)
		cmds = append(cmds, fl.Ensure(m.cache))
	case SubtabTriggers:
		tr := d.EnsureTriggers(targetArg(o), d.DescribeCur)
		cmds = append(cmds, tr.Ensure(m.cache))
	case SubtabFLS:
		cmds = append(cmds, d.PermissionSets.Ensure(m.cache))
		if d.FLSParentID != "" {
			fls := d.EnsureFLS(targetArg(o), d.DescribeCur, d.FLSParentID)
			cmds = append(cmds, fls.Ensure(m.cache))
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) ensureObjectDetailRecordsData(d *orgData, o sf.Org) []tea.Cmd {
	cmds := []tea.Cmd{}
	lv := d.EnsureListViews(targetArg(o), d.DescribeCur)
	cmds = append(cmds, lv.Ensure(m.cache))

	// Records-capability gate (central — see records_capability.go).
	// Non-queryable entities (Platform Events / Big Objects / External
	// Objects) reject SOQL with INVALID_TYPE_FOR_OPERATION, so don't fire
	// any record-producing fetch once the describe confirms it; the
	// render path shows the hint. (DescribeLoaded=false → wait, don't
	// gate; the describe is ensured in the same batch and re-fires this.)
	recCap := recordsCapabilityForData(d, d.DescribeCur)
	if recCap.DescribeLoaded && !recCap.Queryable {
		return cmds
	}

	selected := selectedRecordsChip(d, d.DescribeCur)
	if selected == chipOverflowID {
		return cmds
	}
	if currentChipMode(d, d.DescribeCur) == ChipModeSalesforce {
		// SF mode's synthetic "Recently Viewed" chip needs the
		// per-sObject RecentlyViewed payload loaded.  d.RecentlyViewed
		// (the global top-N) is NOT sufficient — for users who've
		// viewed many other sObjects it returns zero rows for the one
		// we care about.  EnsureRecentlyViewedPerSObject runs a SOQL
		// query scoped to this sObject.
		//
		// Skip the fetch for objects that aren't recently-viewable
		// (mruEnabled=false) — they have no LastViewedDate, so the query
		// throws INVALID_FIELD. Only query once the describe confirms
		// MRU-enabled (DescribeLoaded=false → wait). The render path
		// shows a hint for the non-viewable ones.
		if recCap.DescribeLoaded && recCap.MruEnabled {
			rv := d.EnsureRecentlyViewedPerSObject(targetArg(o), d.DescribeCur)
			cmds = append(cmds, rv.Ensure(m.cache))
		}
		switch selected {
		case "":
			// Catalog hasn't loaded yet — nothing else to fetch.
		case sfRecentlyViewedChipID:
			// Synthetic SF Recently Viewed chip — routes through the
			// chip-records resource (SOQL `Id IN (visited-ids)`) using
			// the per-sObject RecentlyViewed payload.  No-op when the
			// payload hasn't landed yet OR when it landed empty; the
			// `recently_viewed_per_sobject` Apply hook re-fires
			// EnsureChipRecords once IDs are available.
			if c, ok := m.salesforceVisitedRecordsChip(d, d.DescribeCur, o.Username); ok {
				rr := d.EnsureChipRecords(targetArg(o), d.DescribeCur, c, qchip.Substitutions{})
				cmds = append(cmds, rr.Ensure(m.cache))
			}
		default:
			rr := d.EnsureListViewResult(targetArg(o), d.DescribeCur, selected)
			cmds = append(cmds, rr.Ensure(m.cache))
		}
	} else if selected == projectChipID {
		if c, ok := m.projectRecordsChip(d, d.DescribeCur); ok {
			rr := d.EnsureChipRecords(targetArg(o), d.DescribeCur, c, qchip.Substitutions{})
			cmds = append(cmds, rr.Ensure(m.cache))
		}
	} else if selected == recentlyViewedChipID {
		if c, ok := m.visitedRecordsChip(d, d.DescribeCur, o.Username); ok {
			rr := d.EnsureChipRecords(targetArg(o), d.DescribeCur, c, qchip.Substitutions{})
			cmds = append(cmds, rr.Ensure(m.cache))
		}
	} else {
		subs := chipSubs(d)
		c, ok := m.chipRegistry(domainRecords).FindByID(selected)
		if !ok {
			c, _ = m.chipRegistry(domainRecords).FindByID(syntheticRecentID)
		}
		if selected == syntheticRecentID && c.Query.Where == nil && len(c.Query.OrderBy) == 0 {
			rr := d.EnsureRecords(targetArg(o), d.DescribeCur)
			cmds = append(cmds, rr.Ensure(m.cache))
		} else {
			rr := d.EnsureChipRecords(targetArg(o), d.DescribeCur, c, subs)
			cmds = append(cmds, rr.Ensure(m.cache))
		}
	}
	return cmds
}

func (m Model) refreshObjectDetailData(d *orgData) tea.Cmd {
	if m.currentSubtab() == SubtabRecords && d.DescribeCur != "" {
		return m.activeChipRefreshCmd(d, d.DescribeCur)
	}
	cmds := []tea.Cmd{d.SObjects.Refresh(m.cache)}
	if d.DescribeCur == "" {
		return tea.Batch(cmds...)
	}
	if r, ok := d.Describes[d.DescribeCur]; ok {
		cmds = append(cmds, r.Refresh(m.cache))
	}
	switch m.currentSubtab() {
	case SubtabValidation:
		if vr, ok := d.ValidationRules.Lists[d.DescribeCur]; ok {
			cmds = append(cmds, vr.Refresh(m.cache))
		}
	case SubtabRecordTypes:
		if rt, ok := d.RecordTypes.Lists[d.DescribeCur]; ok {
			cmds = append(cmds, rt.Refresh(m.cache))
		}
	case SubtabTriggers:
		if tr, ok := d.Triggers.Lists[d.DescribeCur]; ok {
			cmds = append(cmds, tr.Refresh(m.cache))
		}
	case SubtabFLS:
		cmds = append(cmds, d.PermissionSets.Refresh(m.cache))
		if d.FLSParentID != "" {
			if fls, ok := d.FLS[d.DescribeCur+":"+d.FLSParentID]; ok {
				cmds = append(cmds, fls.Refresh(m.cache))
			}
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) activateObjectDetail() tea.Cmd {
	switch m.currentSubtab() {
	case SubtabRecords:
		return m.activateObjectDetailRecord()
	case SubtabDetails:
		// Enter fires the action the cursored MAIN-pane row maps to.
		// Read-only rows (api name, capabilities, …) are a no-op.
		idx, ok := m.objectDetailActionForCursor()
		if !ok {
			return nil
		}
		mm, cmd := StartAction(*m, objectRegistry, idx)
		*m = mm
		return cmd
	case SubtabValidation:
		return m.activateObjectValidation()
	case SubtabRecordTypes:
		return m.activateObjectRecordType()
	case SubtabTriggers:
		return m.activateObjectTrigger()
	case SubtabObjectFlows:
		return m.activateObjectFlow()
	case SubtabSchema:
	default:
		return nil
	}
	return m.activateObjectField()
}

func (m *Model) activateObjectDetailRecord() tea.Cmd {
	if len(m.orgs) == 0 {
		return nil
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	if d.DescribeCur == "" {
		return nil
	}
	idx := recordsCursorDisplay(d, d.DescribeCur)
	rec, ok := currentRecordAt(d, d.DescribeCur, idx)
	if !ok {
		return nil
	}
	id, _ := rec["Id"].(string)
	if id == "" {
		return nil
	}
	name := recordDisplayName(rec)
	return m.triggerRecordDrill(d.DescribeCur, id, name, TabObjectDetail)
}

func (m *Model) activateObjectValidation() tea.Cmd {
	if len(m.orgs) == 0 {
		return nil
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	r, ok := d.ValidationRules.Lists[d.DescribeCur]
	if !ok {
		return nil
	}
	rules := r.Value()
	if len(rules) == 0 {
		return nil
	}
	idx := d.ValidationRules.Cursors[d.DescribeCur]
	if idx < 0 || idx >= len(rules) {
		idx = 0
	}
	d.ValidationRules.DrillID = rules[idx].ID
	m.validationActionCur = 0
	m.setTab(TabValidationDetail)
	return m.onTabChanged()
}

func (m *Model) activateObjectRecordType() tea.Cmd {
	if len(m.orgs) == 0 {
		return nil
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	r, ok := d.RecordTypes.Lists[d.DescribeCur]
	if !ok {
		return nil
	}
	rts := r.Value()
	if len(rts) == 0 {
		return nil
	}
	idx := d.RecordTypes.Cursors[d.DescribeCur]
	if idx < 0 || idx >= len(rts) {
		idx = 0
	}
	d.RecordTypes.DrillID = rts[idx].ID
	m.recordTypeActionCur = 0
	m.setTab(TabRecordTypeDetail)
	return m.onTabChanged()
}

func (m *Model) activateObjectTrigger() tea.Cmd {
	if len(m.orgs) == 0 {
		return nil
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	r, ok := d.Triggers.Lists[d.DescribeCur]
	if !ok {
		return nil
	}
	trigs := r.Value()
	if len(trigs) == 0 {
		return nil
	}
	idx := d.Triggers.Cursors[d.DescribeCur]
	if idx < 0 || idx >= len(trigs) {
		idx = 0
	}
	return m.triggerDetailDrill(d.DescribeCur, trigs[idx].ID, TabObjectDetail)
}

func (m *Model) activateObjectField() tea.Cmd {
	if len(m.orgs) == 0 {
		return nil
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	if d.DescribeCur == "" {
		return nil
	}
	r, ok := d.Describes[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		return nil
	}
	f, ok := d.cursoredField(d.DescribeCur, r)
	if !ok {
		return nil
	}
	d.FieldCur = f.Name
	m.fieldActionCur = 0
	m.setTab(TabFieldDetail)
	return m.onTabChanged()
}
