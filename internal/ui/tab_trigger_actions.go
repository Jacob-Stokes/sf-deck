package ui

// Apex trigger action menu — generic Action[Ctx] core. Trigger
// writes don't use the standard ToolingEntity.PatchMeta helper
// because UpdateTriggerMetadata sorts keys between top-level
// columns (Body, Status) and the Metadata envelope — that
// dispatcher lives behind metadataops.EditorService. Delete uses the
// generic safety-enforced metadata service.

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/services/metadataops"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type triggerCtx struct {
	Alias       string
	OrgData     *orgData
	Sobject     string
	Trigger     sf.TriggerRow
	TriggersRes *Resource[[]sf.TriggerRow]
	Cache       *cache.Cache
	Metadata    *metadataops.Service
	Editors     *metadataops.EditorService
}

func (c triggerCtx) Entity() sf.ToolingEntity {
	return sf.ToolingEntity{Target: c.Alias, Type: "ApexTrigger", ID: c.Trigger.ID}
}

func triggerEntity(c triggerCtx) sf.ToolingEntity       { return c.Entity() }
func triggerMetadata(c triggerCtx) *metadataops.Service { return c.Metadata }

var triggerRegistry = ActionRegistry[triggerCtx]{
	BuildContext: buildTriggerCtx,
	Actions:      triggerActionsFor,
}

func buildTriggerCtx(m Model) (triggerCtx, bool, string) {
	o, ok := m.currentOrg()
	if !ok {
		return triggerCtx{}, false, ""
	}
	d := m.ensureOrgDataRef(o.Username)
	if d == nil || d.DescribeCur == "" {
		return triggerCtx{}, false, ""
	}
	r, ok := d.Triggers.Lists[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		return triggerCtx{}, false, "triggers not loaded yet — press " + firstPretty(Keys.Refresh) + " to refresh"
	}
	trigs := r.Value()
	if len(trigs) == 0 {
		return triggerCtx{}, false, ""
	}
	var tr sf.TriggerRow
	found := false
	if d.Triggers.DrillID != "" {
		for _, x := range trigs {
			if x.ID == d.Triggers.DrillID {
				tr = x
				found = true
				break
			}
		}
	}
	if !found {
		idx := d.Triggers.Cursors[d.DescribeCur]
		if idx < 0 || idx >= len(trigs) {
			idx = 0
		}
		tr = trigs[idx]
	}
	return triggerCtx{
		Alias:       orgAlias(o),
		OrgData:     d,
		Sobject:     d.DescribeCur,
		Trigger:     tr,
		TriggersRes: r,
		Cache:       m.cache,
		Metadata:    metadataWriteService(m, o),
		Editors:     metadataEditorService(m, o),
	}, true, ""
}

// trgSave calls UpdateTriggerMetadata — not the generic
// ToolingEntity.PatchMeta — because ApexTrigger has the
// top-level-column-vs-Metadata dispatch that PatchMeta's envelope-
// only shape can't express. Narrow helper so both status + body
// share a call site.
func trgSave(c triggerCtx, patch map[string]any) error {
	_, err := c.Editors.UpdateTrigger(context.Background(), metadataops.TriggerUpdateInput{
		Target: c.Alias, ID: c.Trigger.ID, Patch: patch,
	})
	return err
}

func triggerActionsFor(ctx triggerCtx) []Action[triggerCtx] {
	titleFor := func(label string) func(triggerCtx) string {
		return func(c triggerCtx) string {
			return actionTitleFor(c.Sobject, c.Trigger.Name, label)
		}
	}
	success := func(c triggerCtx) string { return "updated " + c.Trigger.Name }

	return []Action[triggerCtx]{
		NewBooleanAction(BooleanActionSpec[triggerCtx]{
			ID:    "trigger-toggle-status",
			Label: "Toggle status",
			Hint:  "Switches between Active and Inactive.",
			Title: titleFor("Toggle status"),
			// ApexTrigger.status is a string column, not a bool — the
			// ChoiceOpt Value is literally "Active"/"Inactive".
			TrueOption:  ChoiceOpt{Label: "Active", Hint: "Fires on matching DML.", Value: "Active"},
			FalseOption: ChoiceOpt{Label: "Inactive", Hint: "Defined but dormant.", Value: "Inactive"},
			Current:     func(c triggerCtx) bool { return c.Trigger.Status == "Active" },
			Save: func(c triggerCtx, val any) error {
				return trgSave(c, map[string]any{"status": val})
			},
			SuccessFlash: success,
			OnSuccess:    refreshTriggersFor,
		}),
		NewTextAction(TextActionSpec[triggerCtx]{
			ID:        "trigger-edit-body",
			Label:     "Edit body",
			Hint:      "Apex source. Save compiles — errors come back as a red line.",
			Multiline: true,
			Title:     titleFor("Edit body"),
			LoadCurrent: func(c triggerCtx) func() (string, error) {
				return func() (string, error) {
					det, err := sf.GetTrigger(c.Alias, c.Trigger.ID)
					if err != nil {
						return "", err
					}
					return det.Body, nil
				}
			},
			Save: func(c triggerCtx, val string, _ any) error {
				return trgSave(c, map[string]any{"body": val})
			},
			SuccessFlash: success,
			OnSuccess:    refreshTriggersFor,
		}),
		NewDestructiveAction(DestructiveActionSpec[triggerCtx]{
			ID:           "trigger-delete",
			Label:        "Delete trigger",
			Hint:         "Permanently removes this trigger.",
			Title:        titleFor("Delete trigger"),
			ConfirmHint:  "No undo. DML previously guarded by this trigger will run unchecked.",
			Save:         ToolingDelete(triggerEntity, triggerMetadata),
			SuccessFlash: func(c triggerCtx) string { return "deleted " + c.Trigger.Name },
			OnSuccess:    triggerDeletedCmd,
		}),
	}
}

func triggerDeletedCmd(c triggerCtx) tea.Cmd {
	if c.TriggersRes == nil {
		return nil
	}
	refresh := c.TriggersRes.Refresh(c.Cache)
	id := c.Trigger.ID
	alias := c.Alias
	return func() tea.Msg {
		return triggerPoppedMsg{
			alias:    alias,
			id:       id,
			innerCmd: refresh,
		}
	}
}

// triggerPoppedMsg signals that a trigger was deleted and we should
// pop back from TabTriggerDetail to TabObjectDetail + Triggers subtab.
type triggerPoppedMsg struct {
	alias    string
	id       string
	innerCmd tea.Cmd
}

func refreshTriggersFor(c triggerCtx) tea.Cmd {
	if c.OrgData == nil {
		return nil
	}
	return c.OrgData.Triggers.RefreshSObject(c.Sobject, c.Cache)
}
