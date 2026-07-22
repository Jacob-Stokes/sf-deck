package ui

// Flow-detail write operations: rename the flow's display label and
// delete an individual (inactive) flow version. Both are metadata
// writes, gated by the org's safety level (WriteMetadata) — the same
// chokepoint every other schema mutation goes through.
//
// Reached from the tab=flow-detail versions view:
//   e  → rename flow label (edit modal)
//   D  → delete cursored version (confirm modal; blocks the active
//        version with a flash, since Salesforce refuses that delete
//        with an opaque error)

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// flowDetailHeader returns the Flow list-row for the drilled-in
// definition, plus the org and orgData, or ok=false when the view
// isn't in a state where a write makes sense.
func (m Model) flowDetailContext() (o sf.Org, d *orgData, header sf.Flow, ok bool) {
	o, ok = m.currentOrg()
	if !ok {
		return o, nil, header, false
	}
	d = m.ensureOrgDataRef(o.Username)
	if d.FlowCur == "" {
		return o, d, header, false
	}
	for _, f := range d.Flows.Value() {
		if f.DefinitionID == d.FlowCur {
			return o, d, f, true
		}
	}
	// Header row not loaded (rare — versions came from a direct drill
	// before the list finished). Fall back to a stub carrying just the
	// definition id so rename still works; delete needs the version
	// list which is independent of the header.
	return o, d, sf.Flow{DefinitionID: d.FlowCur}, true
}

// cursoredFlowVersion returns the version under the cursor in the
// flow-detail versions view, or ok=false when none is loaded.
func (m Model) cursoredFlowVersion(d *orgData) (sf.FlowVersion, bool) {
	r, ok := d.FlowVersions[d.FlowCur]
	if !ok || r.FetchedAt().IsZero() {
		return sf.FlowVersion{}, false
	}
	versions := r.Value()
	if len(versions) == 0 {
		return sf.FlowVersion{}, false
	}
	sel := d.Cursors.Get(cursorKindFlowVersion, len(versions), d.FlowCur)
	if sel < 0 || sel >= len(versions) {
		return sf.FlowVersion{}, false
	}
	return versions[sel], true
}

// handleFlowRename opens an edit modal pre-filled with the flow's
// current display label and writes FlowDefinition.masterLabel on save.
// The API name (DeveloperName) is deliberately left untouched.
func (m Model) handleFlowRename() (Model, tea.Cmd) {
	o, _, header, ok := m.flowDetailContext()
	if !ok {
		return m, nil
	}
	if ok2, reason := m.canWriteOrg(o, settings.WriteMetadata); !ok2 {
		m.flash(reason)
		return m, nil
	}

	// Pre-fill with the current label; fall back to the API name when
	// the flow has no label set (common — see the rename design notes).
	initial := header.MasterLabel
	if initial == "" {
		initial = header.DeveloperName
	}
	name := header.DeveloperName
	if name == "" {
		name = "flow"
	}

	target := targetArg(o)
	defID := header.DefinitionID
	cmd := m.openEditModal(editModalState{
		Title:       "Rename flow · " + name,
		Hint:        "Sets the display label (not the API name). Enter to save · Esc to cancel.",
		InitialBody: initial,
		SuccessMsg:  "flow label updated",
		Save: func(val string, _ any) error {
			label := strings.TrimSpace(val)
			if label == "" {
				return fmt.Errorf("label required")
			}
			// Safety may have been lowered over IPC while this modal was
			// open. Re-check at commit time, immediately before Salesforce.
			if err := requireFlowMetadataWrite(m, o); err != nil {
				return err
			}
			return sf.RenameFlow(target, defID, label)
		},
		OnSuccess: func() tea.Cmd {
			return func() tea.Msg { return flowChangedMsg{username: o.Username} }
		},
	})
	return m, cmd
}

// handleFlowVersionDelete confirms and deletes the cursored flow
// version. The active version can't be deleted (Salesforce refuses
// with an opaque error), so that case is blocked up front with a
// clear flash.
func (m Model) handleFlowVersionDelete() (Model, tea.Cmd) {
	o, d, header, ok := m.flowDetailContext()
	if !ok {
		return m, nil
	}
	if ok2, reason := m.canWriteOrg(o, settings.WriteMetadata); !ok2 {
		m.flash(reason)
		return m, nil
	}
	v, ok := m.cursoredFlowVersion(d)
	if !ok {
		m.flash("no version selected")
		return m, nil
	}
	if flowVersionIsActive(v, header) {
		m.flash("can't delete the active version — deactivate the flow first")
		return m, nil
	}

	target := targetArg(o)
	versionID := v.ID
	label := fmt.Sprintf("v%d", v.VersionNumber)
	cmd := m.openChoiceModal(choiceModalState{
		Title: "Delete flow version · " + label,
		Hint:  "Permanently deletes this inactive version. This can't be undone.",
		Options: []choiceOption{
			{Label: "Cancel", Hint: "Keep the version.", Cancel: true},
			{Label: "Delete permanently", Hint: "Remove " + label + " from " + o.Display(), Value: true},
		},
		Cursor:     0, // default to the safe option
		SuccessMsg: label + " deleted",
		Save: func(_ any) error {
			// The confirmation can remain open while IPC lowers safety.
			// Never let a stale pre-modal check authorize the destructive call.
			if err := requireFlowMetadataWrite(m, o); err != nil {
				return err
			}
			return sf.DeleteFlowVersion(target, versionID)
		},
		OnSuccess: func() tea.Cmd {
			return func() tea.Msg { return flowChangedMsg{username: o.Username} }
		},
	})
	return m, cmd
}

func requireFlowMetadataWrite(m Model, o sf.Org) error {
	if allowed, reason := m.canWriteOrg(o, settings.WriteMetadata); !allowed {
		return errors.New(reason)
	}
	return nil
}

// flowVersionIsActive reports whether v is the flow's active version —
// the one Salesforce refuses to delete. Matches either on the header's
// ActiveVersionID (authoritative when the list row is loaded) or the
// version's own "Active" status (fallback when the header is a stub).
func flowVersionIsActive(v sf.FlowVersion, header sf.Flow) bool {
	if v.ID != "" && v.ID == header.ActiveVersionID {
		return true
	}
	return v.Status == "Active"
}

// flowChangedMsg is emitted after a successful flow-detail write so
// Update can refresh both the version list (the deleted/renamed row)
// and the Flows list (a label change shows there too).
type flowChangedMsg struct{ username string }
