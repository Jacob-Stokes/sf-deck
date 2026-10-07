package ui

// "Collect" — shift+K from anywhere → add the cursored Openable to a
// dev project, tagged with the active org as its origin.
//
// Per design: cannot create projects from this flow. If no dev
// projects exist yet, we flash a hint pointing to /dev-projects.
// Keeps the collect flow strictly about adding-to-existing.

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func (m *Model) triggerCollect(forcePicker bool) tea.Cmd {
	if m.devProjects == nil {
		m.flash("dev-projects unavailable")
		return nil
	}
	if len(m.orgs) == 0 {
		m.flash("no org selected")
		return nil
	}
	if cmd, handled := m.triggerCollectReportFolder(); handled {
		return cmd
	}
	// Resolution order: Openable → Identity. Openable wins when set
	// because its FromOpenable return carries the per-kind Type field
	// (e.g. sObject for KindRecord) that Identity doesn't track. The
	// deep-collect wizard for sObjects also keys off Openable.
	//
	// Identity-only fallback covers surfaces with no Lightning URL
	// (saved SOQL queries today; future kinds that are pure local
	// artefacts). Without it those items would silently fail the
	// "nothing to collect here" guard despite having a valid Kind+Ref.
	var (
		kind      devproject.ItemKind
		ref       string
		typ       string
		name      string
		namespace string
	)
	target := m.cursorOpenable()
	switch {
	case target != nil:
		if IsDeepCollectTarget(target) {
			return m.openDeepCollect(target)
		}
		var ok bool
		kind, ref, typ, name, ok = devproject.FromOpenable(target)
		if !ok {
			m.flash("can't collect this kind yet")
			return nil
		}
		if id, idOk := m.resolveItemIdentity(); idOk && id.Kind == kind && id.Ref == ref {
			namespace = id.Namespace
		}
	default:
		id, idOk := m.resolveItemIdentity()
		if !idOk || id.Kind == "" || id.Ref == "" {
			m.flash("nothing to collect here")
			return nil
		}
		kind = id.Kind
		ref = id.Ref
		name = id.Label
		namespace = id.Namespace
	}
	user := m.orgs[m.selected].Username
	dps, err := m.devProjects.ListDevProjects()
	if err != nil {
		m.flash("collect: " + err.Error())
		return nil
	}
	if len(dps) == 0 {
		m.flash("no dev projects yet — open /dev-projects + press " + firstPretty(Keys.NewProject) + " to create one")
		return nil
	}
	pickedMsg := func(devID string) tea.Msg {
		return collectItemPickedMsg{
			Kind:      kind,
			Ref:       ref,
			Type:      typ,
			Name:      name,
			DevID:     devID,
			OrgUser:   user,
			Namespace: namespace,
		}
	}

	d := m.ensureOrgData(user)
	if !forcePicker && d.LoadedDevProjectID != "" {
		devID := d.LoadedDevProjectID
		inProject, err := m.devProjects.ItemInProject(devID, kind, ref, user)
		if err != nil {
			m.flash("collect: " + err.Error())
			return nil
		}
		if inProject {
			return func() tea.Msg {
				return collectItemRemovedMsg{
					Kind: kind, Ref: ref, Name: name,
					DevID: devID, OrgUser: user,
				}
			}
		}
		return func() tea.Msg { return pickedMsg(devID) }
	}

	// The removed-message factory mirrors pickedMsg — the picker
	// toggles per project, so selecting a project the item is ALREADY
	// in removes it (there'd otherwise be no way to uncollect from a
	// non-loaded project without opening its detail view).
	removedMsg := func(devID string) tea.Msg {
		return collectItemRemovedMsg{
			Kind: kind, Ref: ref, Name: name,
			DevID: devID, OrgUser: user,
		}
	}

	inProject := make(map[string]bool, len(dps))
	opts := make([]choiceOption, 0, len(dps))
	for _, p := range dps {
		has, err := m.devProjects.ItemInProject(p.ID, kind, ref, user)
		if err != nil {
			m.flash("collect: " + err.Error())
			return nil
		}
		inProject[p.ID] = has
		label := p.Name
		hint := fmt.Sprintf("touched %s · Enter to add", humanTimeAgo(p.TouchedAt))
		if has {
			label = "✓ " + p.Name
			hint = "already in this project · Enter to remove"
		}
		opts = append(opts, choiceOption{Label: label, Hint: hint, Value: p.ID})
	}
	state := choiceModalState{
		Title:      itemCollectTitle(target, kind, name),
		Hint:       "pick a dev project · ✓ = already in it (Enter toggles) · from " + user + "  ·  Esc to cancel",
		Options:    opts,
		Cursor:     0,
		Searchable: true,
		OnSuccessTyped: func(val any) tea.Cmd {
			devID, _ := val.(string)
			if inProject[devID] {
				return func() tea.Msg { return removedMsg(devID) }
			}
			return func() tea.Msg { return pickedMsg(devID) }
		},
	}
	return m.openChoiceModal(state)
}

func (m *Model) applyCollectItemPicked(msg collectItemPickedMsg) tea.Cmd {
	if m.devProjects == nil {
		return nil
	}
	added, err := m.devProjects.AddItem(devproject.Item{
		DevProjectID: msg.DevID,
		OrgUser:      msg.OrgUser,
		Kind:         msg.Kind,
		Ref:          msg.Ref,
		Type:         msg.Type,
		Name:         msg.Name,
		Namespace:    msg.Namespace,
	})
	if err != nil {
		applog.Error("collect.add", map[string]any{"err": err.Error()})
		m.flash("collect: " + err.Error())
		return nil
	}
	if !added {
		m.flash("already in that project")
		return nil
	}
	m.reconcileDevProject(msg.DevID)
	m.reloadDevProjects()
	if m.tab() == TabDevProjectDetail && m.devProjectCur == msg.DevID {
		m.reloadDevProjectItems()
	}
	if len(m.orgs) > 0 {
		d := m.ensureOrgData(m.orgs[m.selected].Username)
		if d.LoadedDevProjectID == msg.DevID && msg.OrgUser == m.orgs[m.selected].Username {
			m.refreshLoadedScope(d)
		}
	}
	m.flash("added " + msg.Name + " to project")
	applog.Info("collect.added", map[string]any{
		"dev_id": msg.DevID, "org": msg.OrgUser,
		"kind": string(msg.Kind), "ref": msg.Ref,
	})
	return nil
}

type collectItemPickedMsg struct {
	Kind      devproject.ItemKind
	Ref       string
	Type      string
	Name      string
	DevID     string
	OrgUser   string
	Namespace string
}

type collectItemRemovedMsg struct {
	Kind    devproject.ItemKind
	Ref     string
	Name    string
	DevID   string
	OrgUser string
}

func (m *Model) applyCollectItemRemoved(msg collectItemRemovedMsg) tea.Cmd {
	if m.devProjects == nil {
		return nil
	}
	if err := m.devProjects.RemoveItem(msg.DevID, msg.OrgUser, msg.Kind, msg.Ref); err != nil {
		applog.Error("collect.remove", map[string]any{"err": err.Error()})
		m.flash("uncollect: " + err.Error())
		return nil
	}
	m.reconcileDevProject(msg.DevID)
	m.reloadDevProjects()
	if m.tab() == TabDevProjectDetail && m.devProjectCur == msg.DevID {
		m.reloadDevProjectItems()
	}
	if len(m.orgs) > 0 {
		d := m.ensureOrgData(m.orgs[m.selected].Username)
		if d.LoadedDevProjectID == msg.DevID && msg.OrgUser == m.orgs[m.selected].Username {
			m.refreshLoadedScope(d)
		}
	}
	label := msg.Name
	if label == "" {
		label = msg.Ref
	}
	m.flash("removed " + label + " from project")
	applog.Info("collect.removed", map[string]any{
		"dev_id": msg.DevID, "org": msg.OrgUser,
		"kind": string(msg.Kind), "ref": msg.Ref,
	})
	return nil
}

func itemCollectTitle(target sf.Openable, kind devproject.ItemKind, name string) string {
	if name != "" {
		return "Collect " + name
	}
	return "Collect " + string(kind)
}
