package ui

// Chip scope chooser — a small flow that picks a settings.ChipShare.

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

type chipScopeTargetKind string

const (
	chipScopeTargetWizard   chipScopeTargetKind = "wizard"
	chipScopeTargetOtherOrg chipScopeTargetKind = "other_org"
)

type chipScopeTarget struct {
	kind   chipScopeTargetKind
	domain chipDomain
	chipID string
	scope  string
}

type chipScopeKindPickedMsg struct {
	title   string
	kind    settings.ChipShareKind
	initial settings.ChipShare
	target  chipScopeTarget
}

type chipScopeChosenMsg struct {
	share  settings.ChipShare
	target chipScopeTarget
}

// openChipScopeChooser opens the kind picker. If the user esc's out at
// any step nothing else happens — the choice modal simply dismisses,
// leaving the caller's pre-chooser state untouched (which is the right
// semantic: "the user changed their mind, don't update the chip").
//
// initial positions the cursor on the kind picker so re-editing lands
// on the chip's current kind. Returns a tea.Cmd the caller threads up
// through Update (the underlying choice modal returns nil today; the
// signature is preserved for forward-compat).
func (m *Model) openChipScopeChooser(
	title string,
	initial settings.ChipShare,
	target chipScopeTarget,
) tea.Cmd {
	opts := []choiceOption{
		{Label: "This org", Hint: chipScopeHintThisOrg(m), Value: string(settings.ChipShareOrg)},
		{Label: "These orgs (pick a few)", Hint: "share with an explicit list", Value: string(settings.ChipShareOrgs)},
		{Label: "Org group", Hint: chipScopeHintGroup(m), Value: string(settings.ChipShareGroup)},
		{Label: "Global (every org)", Hint: "use sparingly — sObject names collide across orgs", Value: string(settings.ChipShareGlobal)},
	}
	cursor := 0
	for i, o := range opts {
		if o.Value == string(initial.Kind) {
			cursor = i
			break
		}
	}
	state := choiceModalState{
		Title:   title,
		Hint:    "Who should see this chip?",
		Options: opts,
		Cursor:  cursor,
		OnSuccessTyped: func(val any) tea.Cmd {
			kind, _ := val.(string)
			return func() tea.Msg {
				return chipScopeKindPickedMsg{
					title:   title,
					kind:    settings.ChipShareKind(kind),
					initial: initial,
					target:  target,
				}
			}
		},
	}
	return m.openChoiceModal(state)
}

func (m *Model) applyChipScopeKindPicked(msg chipScopeKindPickedMsg) tea.Cmd {
	switch msg.kind {
	case settings.ChipShareOrg:
		// "This org" — resolve to the currently active org. If there is
		// none, refuse (an unstamped chip would leak everywhere).
		username := m.activeOrgUserForChips()
		if username == "" {
			m.flash("no active org — switch to an org first")
			return nil
		}
		return chipScopeChosenCmd(
			settings.ChipShare{Kind: settings.ChipShareOrg, Orgs: []string{username}},
			msg.target,
		)

	case settings.ChipShareGlobal:
		return chipScopeChosenCmd(settings.ChipShare{Kind: settings.ChipShareGlobal}, msg.target)

	case settings.ChipShareOrgs:
		preselect := map[string]bool{}
		if msg.initial.Kind == settings.ChipShareOrgs || msg.initial.Kind == settings.ChipShareOrg {
			for _, u := range msg.initial.Orgs {
				preselect[u] = true
			}
		}
		items := make([]orgPickerOption, 0, len(m.orgs))
		for _, o := range m.orgs {
			items = append(items, orgPickerOption{Org: o, Picked: preselect[o.Username]})
		}
		state := &orgPickerState{
			Title: msg.title + " — pick orgs",
			Hint:  "Chip will be visible for every org you tick.",
			Items: items,
			OnCommit: func(picked []string) tea.Cmd {
				if len(picked) == 0 {
					return nil
				}
				return chipScopeChosenCmd(settings.ChipShare{Kind: settings.ChipShareOrgs, Orgs: picked}, msg.target)
			},
		}
		return m.openOrgPicker(state)

	case settings.ChipShareGroup:
		groups := m.chipScopeGroupOptions()
		if len(groups) == 0 {
			m.flash("no org groups defined — create one in Org Manager first")
			return nil
		}
		initialGroup := ""
		if msg.initial.Kind == settings.ChipShareGroup {
			initialGroup = msg.initial.Group
		}
		opts := make([]choiceOption, 0, len(groups))
		cursor := 0
		for i, g := range groups {
			opts = append(opts, choiceOption{Label: g.Name, Hint: chipScopeGroupOptHint(g), Value: g.ID})
			if g.ID == initialGroup {
				cursor = i
			}
		}
		state := choiceModalState{
			Title:   msg.title + " — pick a group",
			Hint:    "Chip will be visible for every org in this group.",
			Options: opts,
			Cursor:  cursor,
			OnSuccessTyped: func(val any) tea.Cmd {
				groupID, _ := val.(string)
				return chipScopeChosenCmd(settings.ChipShare{Kind: settings.ChipShareGroup, Group: groupID}, msg.target)
			},
		}
		return m.openChoiceModal(state)
	}
	m.flash(fmt.Sprintf("unknown chip scope kind: %s", msg.kind))
	return nil
}

func chipScopeChosenCmd(share settings.ChipShare, target chipScopeTarget) tea.Cmd {
	return func() tea.Msg {
		return chipScopeChosenMsg{share: share, target: target}
	}
}

func (m *Model) applyChipScopeChosen(msg chipScopeChosenMsg) tea.Cmd {
	switch msg.target.kind {
	case chipScopeTargetWizard:
		if m.chipWizard != nil {
			m.chipWizard.Share = msg.share
		}
	case chipScopeTargetOtherOrg:
		if m.settings == nil {
			m.flash("settings unavailable")
			return nil
		}
		cfg, ok := m.findChipConfigByID(msg.target.domain, msg.target.chipID)
		if !ok {
			m.flash("chip not found")
			return nil
		}
		cfg.Share = msg.share
		cfg.OrgUser = ""
		m.settings.UpsertChip(cfg)
		if err := m.settings.Save(); err != nil {
			m.flash("save failed: " + err.Error())
			return nil
		}
		if reg := m.registryFor(msg.target.domain); reg != nil {
			reg.LoadFromSettings(m.settings)
		}
		m.removeChipPreview(msg.target.domain, msg.target.scope, cfg.ID)
		m.flash("scope updated")
	}
	return nil
}

func chipScopeHintThisOrg(m *Model) string {
	if u := m.activeOrgUserForChips(); u != "" {
		if friendly := chipScopeFriendlyOrgName(m, u); friendly != "" {
			return friendly
		}
		return u
	}
	return "(no active org)"
}

func chipScopeHintGroup(m *Model) string {
	n := len(m.chipScopeGroupOptions())
	switch n {
	case 0:
		return "no org groups defined yet"
	case 1:
		return "1 group available"
	default:
		return fmt.Sprintf("%d groups available", n)
	}
}

func chipScopeGroupOptHint(g settings.OrgGroupConfig) string {
	switch len(g.Members) {
	case 1:
		return "1 org"
	default:
		return fmt.Sprintf("%d orgs", len(g.Members))
	}
}

func chipScopeFriendlyOrgName(m *Model, username string) string {
	for _, o := range m.orgs {
		if o.Username == username {
			if o.Alias != "" {
				return o.Alias
			}
			return o.Username
		}
	}
	return ""
}

func (m Model) chipScopeGroupOptions() []settings.OrgGroupConfig {
	if m.settings == nil {
		return nil
	}
	return m.settings.OrgGroups()
}
