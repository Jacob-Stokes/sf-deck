package ui

// FLS grid write path. Toggling Read or Edit on the cursored field
// fires a single FieldPermissions POST or PATCH (no modal; we
// optimize for rapid admin-tweaking).

import (
	tea "charm.land/bubbletea/v2"
	"context"

	"github.com/Jacob-Stokes/sf-deck/internal/services/permissionops"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// inFLSGridContext reports whether the FLS grid is the active view.
// Two places: /objects drill-in FLS subtab, and /perms parent-detail
// Objects subtab after the user has drilled into a specific object.
// objPermReadContext reports whether the `r` key is owned by a
// field-level-security or object-permission toggle on the current
// surface — i.e. one of the FLSToggleRead / ObjPermRead cases would
// fire. The generic Keys.Refresh case (which also defaults to `r` and
// is dispatched far earlier in the switch) must yield here, otherwise
// it shadows those toggles and the advertised Read toggle never runs.
func (m Model) objPermReadContext() bool {
	if m.inFLSGridContext() {
		return true
	}
	return m.tab() == TabPermParentDetail && m.currentSubtab() == SubtabParentObjects
}

func (m Model) inFLSGridContext() bool {
	if m.tab() == TabObjectDetail && m.currentSubtab() == SubtabFLS {
		return true
	}
	if m.tab() == TabPermParentDetail && m.currentSubtab() == SubtabParentObjects {
		if len(m.orgs) > 0 {
			d := m.data[m.orgs[m.selected].Username]
			if d != nil && d.PermParentPermSetID != "" && d.PermFieldsSObject != "" {
				return true
			}
		}
	}
	return false
}

func (m Model) flsToggleCell(which string) (Model, tea.Cmd) {
	o, ok := m.currentOrg()
	if !ok {
		return m, nil
	}
	d := m.ensureOrgData(o.Username)

	var sobj, parentID string
	if m.tab() == TabPermParentDetail {
		sobj = d.PermFieldsSObject
		parentID = d.PermParentPermSetID
	} else {
		sobj = d.DescribeCur
		parentID = d.FLSParentID
	}

	if sobj == "" || parentID == "" {
		return m, nil
	}
	// Safety gate — FLS writes are metadata.
	if ok, reason := m.canWriteCurrent(settings.WriteMetadata); !ok {
		m.flash(reason)
		return m, nil
	}
	dr, ok := d.Describes[sobj]
	if !ok || dr.FetchedAt().IsZero() {
		m.flash("describe not loaded yet")
		return m, nil
	}
	fields := dr.Value().Fields
	if len(fields) == 0 {
		return m, nil
	}
	key := sobj + ":" + parentID
	idx := d.Cursors.Get(cursorKindFLS, len(fields), sobj, parentID)
	field := fields[idx]
	if !field.Permissionable {
		m.flash(field.Name + " isn't permissionable (always visible)")
		return m, nil
	}

	flsRes, ok := d.FLS[key]
	if !ok || flsRes == nil || flsRes.FetchedAt().IsZero() {
		m.flash("FLS not loaded yet")
		return m, nil
	}
	var existing sf.FieldPermissionRow
	for _, fp := range flsRes.Value() {
		name := fp.Field
		if i := indexByte(name, '.'); i >= 0 {
			name = name[i+1:]
		}
		if name == field.Name {
			existing = fp
			break
		}
	}
	curRead := existing.Read
	curEdit := existing.Edit

	// Decide new state + enforce invariant.
	newRead, newEdit := curRead, curEdit
	switch which {
	case "read":
		newRead = !curRead
		if !newRead {
			newEdit = false // Read off implies Edit off
		}
	case "edit":
		newEdit = !curEdit
		if newEdit {
			newRead = true // Edit on implies Read on
		}
	}

	alias := orgAlias(o)
	fullField := sobj + "." + field.Name
	existingID := existing.ID
	service := permissionWriteService(m, o)

	if Demo {
		// Demo: apply the toggle to the seeded world directly — same
		// row-existence semantics as the live API (both-off deletes
		// the row), written through to the demo cache so a re-load
		// serves it back. The safety gate above already ran, so the
		// read-only prod org still declines exactly like live.
		rows := make([]sf.FieldPermissionRow, 0, len(flsRes.Value()))
		found := false
		for _, fp := range flsRes.Value() {
			name := fp.Field
			if i := indexByte(name, '.'); i >= 0 {
				name = name[i+1:]
			}
			if name != field.Name {
				rows = append(rows, fp)
				continue
			}
			found = true
			if newRead || newEdit {
				fp.Read, fp.Edit = newRead, newEdit
				rows = append(rows, fp)
			}
		}
		if !found && (newRead || newEdit) {
			rows = append(rows, sf.FieldPermissionRow{
				ID: "01kDM00000DEMOUPAAA", Field: fullField,
				ParentID: parentID, Read: newRead, Edit: newEdit,
			})
		}
		flsRes.Set(rows)
		if m.cache != nil {
			_ = m.cache.PutJSON(d.username, "fls:"+key, rows)
		}
		return m, nil
	}

	cmd := func() tea.Msg {
		result, err := service.SetField(context.Background(), permissionops.FieldInput{
			Target: alias, ID: existingID, SObject: sobj, Field: fullField,
			ParentID: parentID, Read: newRead, Edit: newEdit,
		})
		return flsWriteDoneMsg{ok: err == nil, noop: result.Noop, err: err}
	}
	return m, cmd
}

type flsWriteDoneMsg struct {
	ok   bool
	noop bool
	err  error
}

func (m Model) applyFLSWriteDone(msg flsWriteDoneMsg) (Model, tea.Cmd) {
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
	if d == nil {
		return m, nil
	}
	var sobj, parentID string
	if m.tab() == TabPermParentDetail {
		sobj = d.PermFieldsSObject
		parentID = d.PermParentPermSetID
	} else {
		sobj = d.DescribeCur
		parentID = d.FLSParentID
	}
	if sobj == "" || parentID == "" {
		return m, nil
	}
	key := sobj + ":" + parentID
	if r, ok := d.FLS[key]; ok && r != nil {
		return m, r.Refresh(m.cache)
	}
	return m, nil
}

// indexByte is a mini-helper to avoid importing strings for one call.
func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
