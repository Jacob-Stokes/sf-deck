package ui

// Record-type action menu — generic Action[Ctx] + ToolingEntity
// write surface. No per-entity save/load helpers.

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/services/metadataops"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type recordTypeCtx struct {
	Alias          string
	OrgData        *orgData
	Sobject        string
	RecordType     sf.RecordTypeRow
	RecordTypesRes *Resource[[]sf.RecordTypeRow]
	Cache          *cache.Cache
	Metadata       *metadataops.Service
}

func (c recordTypeCtx) Entity() sf.ToolingEntity {
	return sf.ToolingEntity{Target: c.Alias, Type: "RecordType", ID: c.RecordType.ID}
}

func recordTypeEntity(c recordTypeCtx) sf.ToolingEntity       { return c.Entity() }
func recordTypeMetadata(c recordTypeCtx) *metadataops.Service { return c.Metadata }

func rtText(key string) (
	func(recordTypeCtx) func() (string, error),
	func(recordTypeCtx, string, any) error,
) {
	return ToolingMetaKeyHelpers(recordTypeEntity, recordTypeMetadata, key)
}

var recordTypeRegistry = ActionRegistry[recordTypeCtx]{
	BuildContext: buildRecordTypeCtx,
	Actions:      recordTypeActionsFor,
}

func buildRecordTypeCtx(m Model) (recordTypeCtx, bool, string) {
	o, ok := m.currentOrg()
	if !ok {
		return recordTypeCtx{}, false, ""
	}
	d := m.ensureOrgDataRef(o.Username)
	if d == nil || d.DescribeCur == "" {
		return recordTypeCtx{}, false, ""
	}
	r, ok := d.RecordTypes.Lists[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		return recordTypeCtx{}, false, "record types not loaded yet — press " + firstPretty(Keys.Refresh) + " to refresh"
	}
	rts := r.Value()
	if len(rts) == 0 {
		return recordTypeCtx{}, false, ""
	}
	var rt sf.RecordTypeRow
	found := false
	if d.RecordTypes.DrillID != "" {
		for _, x := range rts {
			if x.ID == d.RecordTypes.DrillID {
				rt = x
				found = true
				break
			}
		}
	}
	if !found {
		idx := d.RecordTypes.Cursors[d.DescribeCur]
		if idx < 0 || idx >= len(rts) {
			idx = 0
		}
		rt = rts[idx]
	}
	return recordTypeCtx{
		Alias:          orgAlias(o),
		OrgData:        d,
		Sobject:        d.DescribeCur,
		RecordType:     rt,
		RecordTypesRes: r,
		Cache:          m.cache,
		Metadata:       metadataWriteService(m, o),
	}, true, ""
}

func recordTypeActionsFor(ctx recordTypeCtx) []Action[recordTypeCtx] {
	titleFor := func(label string) func(recordTypeCtx) string {
		return func(c recordTypeCtx) string {
			return actionTitleFor(c.Sobject, c.RecordType.DeveloperName, label)
		}
	}
	success := func(c recordTypeCtx) string { return "updated " + c.RecordType.DeveloperName }

	loadLabel, saveLabel := rtText("label")
	loadDesc, saveDesc := rtText("description")

	return []Action[recordTypeCtx]{
		NewBooleanAction(BooleanActionSpec[recordTypeCtx]{
			ID:           "rt-toggle-active",
			Label:        "Toggle active",
			Hint:         "Enables or disables this record type.",
			Title:        titleFor("Toggle active"),
			TrueOption:   ChoiceOpt{Label: "Active", Hint: "Record type is available to assigned profiles.", Value: true},
			FalseOption:  ChoiceOpt{Label: "Inactive", Hint: "Record type is defined but not assignable.", Value: false},
			Current:      func(c recordTypeCtx) bool { return c.RecordType.Active },
			Save:         ToolingBoolSaver(recordTypeEntity, recordTypeMetadata, "active"),
			SuccessFlash: success,
			OnSuccess:    refreshRecordTypesFor,
		}),
		NewTextAction(TextActionSpec[recordTypeCtx]{
			ID:           "rt-edit-label",
			Label:        "Edit label",
			Hint:         "Human-readable label shown in record-type pickers.",
			Title:        titleFor("Edit label"),
			LoadCurrent:  loadLabel,
			Save:         saveLabel,
			SuccessFlash: success,
			OnSuccess:    refreshRecordTypesFor,
		}),
		NewTextAction(TextActionSpec[recordTypeCtx]{
			ID:           "rt-edit-description",
			Label:        "Edit description",
			Hint:         "Internal description visible in Setup.",
			Multiline:    true,
			Title:        titleFor("Edit description"),
			LoadCurrent:  loadDesc,
			Save:         saveDesc,
			SuccessFlash: success,
			OnSuccess:    refreshRecordTypesFor,
		}),
		NewDestructiveAction(DestructiveActionSpec[recordTypeCtx]{
			ID:           "rt-delete",
			Label:        "Delete record type",
			Hint:         "Permanently removes this record type.",
			Title:        titleFor("Delete record type"),
			ConfirmHint:  "No undo. Every record's RecordTypeId becomes null.",
			Save:         ToolingDelete(recordTypeEntity, recordTypeMetadata),
			SuccessFlash: func(c recordTypeCtx) string { return "deleted " + c.RecordType.DeveloperName },
			OnSuccess:    recordTypeDeletedCmd,
		}),
	}
}

func recordTypeDeletedCmd(c recordTypeCtx) tea.Cmd {
	if c.RecordTypesRes == nil {
		return nil
	}
	refresh := c.RecordTypesRes.Refresh(c.Cache)
	rtID := c.RecordType.ID
	alias := c.Alias
	return func() tea.Msg {
		return recordTypePoppedMsg{
			alias:    alias,
			rtID:     rtID,
			innerCmd: refresh,
		}
	}
}

// recordTypePoppedMsg signals that a record type was deleted and we
// should pop back from TabRecordTypeDetail to TabObjectDetail +
// RecordTypes subtab, then fire the carried refresh cmd.
type recordTypePoppedMsg struct {
	alias    string
	rtID     string
	innerCmd tea.Cmd
}

func refreshRecordTypesFor(c recordTypeCtx) tea.Cmd {
	if c.OrgData == nil {
		return nil
	}
	return c.OrgData.RecordTypes.RefreshSObject(c.Sobject, c.Cache)
}
