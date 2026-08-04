package ui

// /compare — the setup-form pickers: source/target org, scope, and the
// "save as template" flow. All reuse the generic choiceModal/editModal
// primitives so no bespoke modal chrome is needed.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

// openCompareOrgPicker opens a choice modal to pick the source (isSource
// = true) or target org for the active run.
func (m *Model) openCompareOrgPicker(d *orgData, isSource bool) tea.Cmd {
	if d.Run == nil {
		d.Run = m.newCompareRun()
	}
	role := "target"
	current := d.Run.Target.OrgRef()
	if isSource {
		role = "source"
		current = d.Run.Source.OrgRef()
	}
	opts := make([]choiceOption, 0, len(m.orgs)+1)
	cursor := 0
	for i, o := range m.orgs {
		label := o.Username
		if o.Alias != "" {
			label = o.Alias
		}
		opts = append(opts, choiceOption{Label: label, Hint: o.Username, Value: o.Username})
		if o.Username == current {
			cursor = i
		}
	}
	if len(opts) == 0 {
		return nil
	}
	state := choiceModalState{
		Title:      "Compare " + role + " org",
		Hint:       "pick the " + role + " org for the comparison",
		Options:    opts,
		Cursor:     cursor,
		Searchable: true,
		Save: func(val any) error {
			username, _ := val.(string)
			if username == "" {
				return nil
			}
			if isSource {
				d.Run.Source = orgEndpoint(username)
			} else {
				d.Run.Target = orgEndpoint(username)
			}
			d.Run.Err = nil
			return nil
		},
	}
	return m.openChoiceModal(state)
}

func (m *Model) openCompareScopePicker(d *orgData) tea.Cmd {
	if d.Run == nil {
		d.Run = m.newCompareRun()
	}
	run := d.Run
	return m.openCompareScopeModal(run.Scope, func(selected []string) {
		run.Scope = selected
	})
}

// saveCurrentCompareAsTemplate prompts for a name and persists the
// active run's source/target/scope as a reusable saved definition.
func (m *Model) saveCurrentCompareAsTemplate(d *orgData) tea.Cmd {
	if d.Run == nil || d.Run.Target.IsZero() {
		m.flash("set up a comparison first")
		return nil
	}
	run := d.Run
	state := editModalState{
		Title:       "Save comparison as…",
		Hint:        "name this comparison template",
		InitialBody: fmt.Sprintf("%s → %s", endpointLabel(*m, run.Source), endpointLabel(*m, run.Target)),
		Multiline:   false,
		SuccessMsg:  "comparison saved",
		Save: func(name string, _ any) error {
			name = strings.TrimSpace(name)
			if name == "" {
				return fmt.Errorf("name required")
			}
			if m.settings == nil {
				return fmt.Errorf("settings unavailable")
			}
			defs := m.settings.CompareDefs()
			defs = append(defs, settingsCompareDef(name, run))
			m.settings.SetCompareDefs(defs)
			if err := m.settings.Save(); err != nil {
				return err
			}
			d.SavedLoaded = false // force reload next time Saved is opened
			return nil
		},
	}
	return m.openEditModal(state)
}

// saveCurrentComparison persists the active comparison RESULT to the
// devproject store, honouring the run's overwrite-vs-new intent:
//   - linked + overwrite → update the original in place, no prompt
//   - linked + new copy (SaveAsNew, set via the edit modal), or
//     unlinked → name prompt → insert new
//
// A run becomes linked+overwrite when opened (↵) from a saved
// comparison (so ctrl+s saves back to it); it becomes linked+new when
// the edit modal's clone toggle was chosen.
func (m *Model) saveCurrentComparison(d *orgData) tea.Cmd {
	if d.Run == nil || d.Run.Phase != comparePhaseInventory {
		m.flash("run a comparison first")
		return nil
	}
	if m.devProjects == nil {
		m.flash("store unavailable")
		return nil
	}
	if d.Run.OriginSavedID != "" && !d.Run.SaveAsNew {
		return m.overwriteSavedComparison(d)
	}
	return m.saveComparisonAsNew(d)
}

// overwriteSavedComparison updates the originating saved comparison's
// stored result in place.
func (m *Model) overwriteSavedComparison(d *orgData) tea.Cmd {
	run := d.Run
	blob, err := serializeCompareRun(run)
	if err != nil {
		m.flash("save: " + err.Error())
		return nil
	}
	err = m.devProjects.UpdateComparison(run.OriginSavedID,
		run.Source.OrgRef(), run.Target.OrgRef(), scopeLabel(run.Scope), run.Method.String(), blob)
	if err != nil {
		m.flash("save: " + err.Error())
		return nil
	}
	d.SavedLoaded = false
	m.flash("updated '" + run.OriginSavedName + "'")
	return nil
}

// saveComparisonAsNew prompts for a name and inserts a new saved
// comparison from the active run's result.
func (m *Model) saveComparisonAsNew(d *orgData) tea.Cmd {
	run := d.Run
	initial := fmt.Sprintf("%s → %s", endpointLabel(*m, run.Source), endpointLabel(*m, run.Target))
	if run.OriginSavedName != "" {
		initial = run.OriginSavedName + " (copy)"
	}
	state := editModalState{
		Title:       "Save comparison",
		Hint:        "name this comparison — saved with its data for offline reopen",
		InitialBody: initial,
		Multiline:   false,
		SuccessMsg:  "comparison saved",
		Save: func(name string, _ any) error {
			name = strings.TrimSpace(name)
			if name == "" {
				return fmt.Errorf("name required")
			}
			blob, err := serializeCompareRun(run)
			if err != nil {
				return err
			}
			sc, err := m.devProjects.SaveComparison(devproject.SavedComparison{
				Name:   name,
				Source: run.Source.OrgRef(),
				Target: run.Target.OrgRef(),
				Scope:  scopeLabel(run.Scope),
				Method: run.Method.String(),
				Blob:   blob,
			})
			if err != nil {
				return err
			}
			// The run now corresponds to this saved comparison, so a
			// subsequent ctrl+s offers overwrite of it.
			run.OriginSavedID = sc.ID
			run.OriginSavedName = sc.Name
			d.SavedLoaded = false
			return nil
		},
	}
	return m.openEditModal(state)
}

// openCompareMethodPicker lets the user choose the retrieval route.
func (m *Model) openCompareMethodPicker(d *orgData) tea.Cmd {
	if d.Run == nil {
		d.Run = m.newCompareRun()
	}
	methods := []compareMethod{compareMethodAuto, compareMethodTooling, compareMethodMetadataAPI}
	opts := make([]choiceOption, 0, len(methods))
	cursor := 0
	for i, cm := range methods {
		opts = append(opts, choiceOption{Label: cm.String(), Hint: methodHint(cm), Value: int(cm)})
		if cm == d.Run.Method {
			cursor = i
		}
	}
	state := choiceModalState{
		Title:   "Retrieval method",
		Hint:    "how to fetch metadata — affects speed and API-call count",
		Options: opts,
		Cursor:  cursor,
		Save: func(val any) error {
			if i, ok := val.(int); ok {
				d.Run.Method = compareMethod(i)
			}
			return nil
		},
	}
	return m.openChoiceModal(state)
}
