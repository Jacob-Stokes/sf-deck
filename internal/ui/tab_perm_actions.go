package ui

// Object and system permission write paths.

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/services/permissionops"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type objPermState struct {
	Read, Create, Edit, Delete, ViewAllRecords, ModifyAllRecords bool
}

func applyObjPermInvariants(s objPermState, which string) objPermState {
	switch which {
	case "read":
		if !s.Read {
			s.Edit = false
			s.Delete = false
			s.ViewAllRecords = false
			s.ModifyAllRecords = false
		}
	case "create":
		// No upstream deps, but ModifyAll requires Create.
		if !s.Create {
			s.ModifyAllRecords = false
		}
	case "edit":
		if s.Edit {
			s.Read = true
		} else {
			s.Delete = false
			s.ModifyAllRecords = false
		}
	case "delete":
		if s.Delete {
			s.Edit = true
			s.Read = true
		}
	case "viewall":
		if s.ViewAllRecords {
			s.Read = true
		} else {
			s.ModifyAllRecords = false
		}
	case "modifyall":
		if s.ModifyAllRecords {
			s.ViewAllRecords = true
			s.Read = true
			s.Create = true
			s.Edit = true
			s.Delete = true
		}
	}
	return s
}

func (m Model) objPermToggleCell(which string) (Model, tea.Cmd) {
	o, ok := m.currentOrg()
	if !ok {
		return m, nil
	}
	d := m.ensureOrgData(o.Username)
	if d.PermParentPermSetID == "" {
		return m, nil
	}
	if ok2, reason := m.canWriteCurrent(settings.WriteMetadata); !ok2 {
		m.flash(reason)
		return m, nil
	}

	key := d.PermParentKind + ":" + d.PermParentPermSetID
	res, ok := d.ObjectPerms[key]
	if !ok || res == nil || res.FetchedAt().IsZero() {
		m.flash("object permissions not loaded yet")
		return m, nil
	}
	rows := res.Value()
	if len(rows) == 0 {
		return m, nil
	}
	cur := d.Cursors.Get(cursorKindObjectPerms, len(rows), d.PermParentKind, d.PermParentPermSetID)
	row := rows[cur]

	cur_s := objPermState{
		Read:             row.Read,
		Create:           row.Create,
		Edit:             row.Edit,
		Delete:           row.Delete,
		ViewAllRecords:   row.ViewAllRecords,
		ModifyAllRecords: row.ModifyAllRecords,
	}
	switch which {
	case "read":
		cur_s.Read = !cur_s.Read
	case "create":
		cur_s.Create = !cur_s.Create
	case "edit":
		cur_s.Edit = !cur_s.Edit
	case "delete":
		cur_s.Delete = !cur_s.Delete
	case "viewall":
		cur_s.ViewAllRecords = !cur_s.ViewAllRecords
	case "modifyall":
		cur_s.ModifyAllRecords = !cur_s.ModifyAllRecords
	}
	new_s := applyObjPermInvariants(cur_s, which)

	alias := orgAlias(o)
	parentID := d.PermParentPermSetID
	sobject := row.SObjectType
	existingID := row.ID
	service := permissionWriteService(m, o)

	cmd := func() tea.Msg {
		result, err := service.SetObject(context.Background(), permissionops.ObjectInput{
			Target: alias, ID: existingID, ParentID: parentID, SObject: sobject,
			Read: new_s.Read, Create: new_s.Create, Edit: new_s.Edit, Delete: new_s.Delete,
			ViewAll: new_s.ViewAllRecords, ModifyAll: new_s.ModifyAllRecords,
		})
		return objPermWriteDoneMsg{ok: err == nil, noop: result.Noop, err: err}
	}
	return m, cmd
}

type objPermWriteDoneMsg struct {
	ok   bool
	noop bool
	err  error
}

func (m Model) applyObjPermWriteDone(msg objPermWriteDoneMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		if typed := sf.AsSFError(msg.err); typed != nil {
			m.flash(typed.Error())
		} else {
			m.flash(msg.err.Error())
		}
		return m, nil
	}
	if msg.noop {
		return m, nil
	}
	if len(m.orgs) == 0 {
		return m, nil
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil || d.PermParentPermSetID == "" {
		return m, nil
	}
	key := d.PermParentKind + ":" + d.PermParentPermSetID
	if r, ok := d.ObjectPerms[key]; ok && r != nil {
		return m, r.Refresh(m.cache)
	}
	return m, nil
}

func (m Model) sysPermToggleCell() (Model, tea.Cmd) {
	o, ok := m.currentOrg()
	if !ok {
		return m, nil
	}
	d := m.ensureOrgData(o.Username)
	if d.PermParentPermSetID == "" {
		return m, nil
	}
	if ok2, reason := m.canWriteCurrent(settings.WriteMetadata); !ok2 {
		m.flash(reason)
		return m, nil
	}

	res, ok := d.SystemPerms[d.PermParentPermSetID]
	if !ok || res == nil || res.FetchedAt().IsZero() {
		m.flash("system permissions not loaded yet")
		return m, nil
	}
	perms := res.Value()
	if len(perms) == 0 {
		return m, nil
	}
	cur := d.Cursors.Get(cursorKindSystemPerms, len(perms), d.PermParentPermSetID)
	p := perms[cur]

	alias := orgAlias(o)
	parentID := d.PermParentPermSetID
	fullName := "Permissions" + p.Name
	newVal := !p.Value
	service := permissionWriteService(m, o)

	cmd := func() tea.Msg {
		_, err := service.SetSystem(context.Background(), permissionops.SystemInput{
			Target: alias, ParentID: parentID, Field: fullName, Value: newVal,
		})
		return sysPermWriteDoneMsg{ok: err == nil, err: err, field: fullName}
	}
	return m, cmd
}

type sysPermWriteDoneMsg struct {
	ok    bool
	err   error
	field string
}

func (m Model) applySysPermWriteDone(msg sysPermWriteDoneMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		if typed := sf.AsSFError(msg.err); typed != nil {
			m.flash(typed.Error())
		} else {
			m.flash(msg.err.Error())
		}
		return m, nil
	}
	if len(m.orgs) == 0 {
		return m, nil
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil || d.PermParentPermSetID == "" {
		return m, nil
	}
	if r, ok := d.SystemPerms[d.PermParentPermSetID]; ok && r != nil {
		return m, r.Refresh(m.cache)
	}
	return m, nil
}
