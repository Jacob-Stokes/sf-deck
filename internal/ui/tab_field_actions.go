package ui

// Field-detail action menu — uses the generic Action[Ctx] core.
// See actions.go for the primitives.
//
// Adding a new action = one entry in fieldActionsFor(). Text edits
// go through NewTextAction; toggle-style booleans through
// NewBooleanAction; destructive actions through NewDestructiveAction.
// Custom inline Start closures stay an escape hatch for one-offs.

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/services/metadataops"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// fieldCtx is the per-selection snapshot handed to every field-
// action Start closure.
type fieldCtx struct {
	Alias       string
	OrgUser     string
	OrgData     *orgData
	Sobject     string
	Field       sf.Field
	DescribeRes *Resource[sf.SObjectDescribe]
	Cache       *cache.Cache
	Metadata    *metadataops.Service
}

var fieldRegistry = ActionRegistry[fieldCtx]{
	BuildContext: buildFieldCtx,
	Actions:      fieldActionsFor,
}

func buildFieldCtx(m Model) (fieldCtx, bool, string) {
	o, ok := m.currentOrg()
	if !ok {
		return fieldCtx{}, false, ""
	}
	d := m.ensureOrgDataRef(o.Username)
	if d == nil || d.DescribeCur == "" || d.FieldCur == "" {
		return fieldCtx{}, false, ""
	}
	r, ok := d.Describes[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		return fieldCtx{}, false, "describe not loaded yet — press " + firstPretty(Keys.Refresh) + " to refresh"
	}
	f, ok := findFieldByName(r.Value().Fields, d.FieldCur)
	if !ok {
		return fieldCtx{}, false, "field not found — describe may be stale (press r)"
	}
	return fieldCtx{
		Alias:       orgAlias(o),
		OrgUser:     o.Username,
		OrgData:     d,
		Sobject:     d.DescribeCur,
		Field:       f,
		DescribeRes: d.Describes[d.DescribeCur],
		Cache:       m.cache,
		Metadata:    metadataWriteService(m, o),
	}, true, ""
}

// customOnly returns a Disabled closure that blocks an action on
// standard (non-custom) fields. Reused across every action that
// requires ownership of the field's metadata.
func customOnly() func(fieldCtx) (bool, string) {
	return func(c fieldCtx) (bool, string) {
		if !c.Field.Custom {
			return true, "can only edit this on custom fields"
		}
		return false, ""
	}
}

func fieldActionsFor(ctx fieldCtx) []Action[fieldCtx] {
	return []Action[fieldCtx]{
		NewTextAction(TextActionSpec[fieldCtx]{
			ID:    "field-edit-label",
			Label: "Edit field label",
			Hint:  "User-facing label shown on pages, reports, and list views.",
			Title: func(c fieldCtx) string {
				return actionTitleFor(c.Sobject, c.Field.Name, "Edit field label")
			},
			InitialBody:  func(c fieldCtx) string { return c.Field.Label },
			Save:         saveFieldMetaString("label"),
			SuccessFlash: fieldSuccessFlash,
			OnSuccess:    refreshFieldDescribeFor,
			Disabled:     customOnly(),
		}),
		NewTextAction(TextActionSpec[fieldCtx]{
			ID:        "field-edit-help",
			Label:     "Edit help text",
			Hint:      "Shown next to this field in Lightning record pages.",
			Multiline: true,
			Title: func(c fieldCtx) string {
				return actionTitleFor(c.Sobject, c.Field.Name, "Edit help text")
			},
			InitialBody:  func(c fieldCtx) string { return c.Field.InlineHelpText },
			Save:         saveFieldMetaString("inlineHelpText"),
			SuccessFlash: fieldSuccessFlash,
			OnSuccess:    refreshFieldDescribeFor,
		}),
		NewTextAction(TextActionSpec[fieldCtx]{
			ID:        "field-edit-description",
			Label:     "Edit description",
			Hint:      "Internal description visible in Setup / Object Manager.",
			Multiline: true,
			Title: func(c fieldCtx) string {
				return actionTitleFor(c.Sobject, c.Field.Name, "Edit description")
			},
			LoadCurrent:  loadFieldMetaString("description"),
			Save:         saveFieldMetaString("description"),
			SuccessFlash: fieldSuccessFlash,
			// The description lives outside the describe, so refreshing the
			// describe won't update the field-detail page's cached value —
			// re-fetch the description itself (which also refreshes the
			// describe for the other properties).
			OnSuccess: refreshFieldDescriptionAfterEdit,
			Disabled:  customOnly(),
		}),
		NewTextAction(TextActionSpec[fieldCtx]{
			ID:        "field-edit-default",
			Label:     "Edit default value (formula)",
			Hint:      "Formula expression evaluated when a new record is created.",
			Multiline: true,
			Title: func(c fieldCtx) string {
				return actionTitleFor(c.Sobject, c.Field.Name, "Edit default value")
			},
			InitialBody: func(c fieldCtx) string {
				if s, ok := stringish(c.Field.DefaultValue); ok {
					return s
				}
				return ""
			},
			Save:         saveFieldMetaString("defaultValue"),
			SuccessFlash: fieldSuccessFlash,
			OnSuccess:    refreshFieldDescribeFor,
			Disabled:     customOnly(),
		}),
		NewBooleanAction(BooleanActionSpec[fieldCtx]{
			ID:    "field-toggle-required",
			Label: "Toggle required",
			Hint:  "Whether the field must have a value on every record.",
			Title: func(c fieldCtx) string {
				return actionTitleFor(c.Sobject, c.Field.Name, "Toggle required")
			},
			TrueOption:   ChoiceOpt{Label: "Required", Hint: "Every record must set this field (insert/update will fail without).", Value: true},
			FalseOption:  ChoiceOpt{Label: "Optional", Hint: "Records can leave this field blank.", Value: false},
			Current:      func(c fieldCtx) bool { return !c.Field.Nillable },
			Save:         saveFieldMetaAny("required"),
			SuccessFlash: fieldSuccessFlash,
			OnSuccess:    refreshFieldDescribeFor,
			Disabled:     customOnly(),
		}),
		NewBooleanAction(BooleanActionSpec[fieldCtx]{
			ID:    "field-toggle-unique",
			Label: "Toggle unique",
			Hint:  "Whether duplicate values are allowed across records.",
			Title: func(c fieldCtx) string {
				return actionTitleFor(c.Sobject, c.Field.Name, "Toggle unique")
			},
			TrueOption:   ChoiceOpt{Label: "Unique", Hint: "Salesforce rejects inserts that duplicate an existing value.", Value: true},
			FalseOption:  ChoiceOpt{Label: "Allow duplicates", Hint: "Multiple records can share the same value.", Value: false},
			Current:      func(c fieldCtx) bool { return c.Field.Unique },
			Save:         saveFieldMetaAny("unique"),
			SuccessFlash: fieldSuccessFlash,
			OnSuccess:    refreshFieldDescribeFor,
			Disabled:     customOnly(),
		}),
		NewBooleanAction(BooleanActionSpec[fieldCtx]{
			ID:    "field-toggle-external-id",
			Label: "Toggle external ID",
			Hint:  "Marks the field as an external identifier for upsert operations.",
			Title: func(c fieldCtx) string {
				return actionTitleFor(c.Sobject, c.Field.Name, "Toggle external ID")
			},
			TrueOption:   ChoiceOpt{Label: "External ID", Hint: "Usable as the matching key in Bulk API + REST upsert calls.", Value: true},
			FalseOption:  ChoiceOpt{Label: "Not an external ID", Hint: "Field is not usable as an upsert key.", Value: false},
			Current:      func(c fieldCtx) bool { return c.Field.ExternalID },
			Save:         saveFieldMetaAny("externalId"),
			SuccessFlash: fieldSuccessFlash,
			OnSuccess:    refreshFieldDescribeFor,
			Disabled:     customOnly(),
		}),
		NewDestructiveAction(DestructiveActionSpec[fieldCtx]{
			ID:    "field-delete",
			Label: "Delete field",
			Hint:  "Permanently removes the field + all its data. No undo.",
			Title: func(c fieldCtx) string {
				return actionTitleFor(c.Sobject, c.Field.Name, "Delete field")
			},
			ConfirmHint: "Deletes the field AND every record's value for it. No undo.",
			Save: func(c fieldCtx) error {
				id, err := customFieldIDCached(c.OrgData, c.Alias, c.Sobject, c.Field.Name)
				if err != nil {
					return err
				}
				_, err = c.Metadata.Delete(context.Background(), metadataops.DeleteInput{
					Target: c.Alias, Type: "CustomField", ID: id,
				})
				return err
			},
			SuccessFlash: func(c fieldCtx) string {
				return "deleted " + c.Sobject + "." + c.Field.Name
			},
			OnSuccess: func(c fieldCtx) tea.Cmd {
				refresh := refreshFieldDescribeFor(c)
				cacheKey := c.Sobject + "." + c.Field.Name
				return func() tea.Msg {
					return fieldDeletedMsg{
						cacheKey: cacheKey,
						innerCmd: refresh,
					}
				}
			},
			Disabled: customOnly(),
		}),
	}
}

// fieldDeletedMsg signals that a field was deleted and we should
// pop back from TabFieldDetail to TabObjectDetail + Schema, then
// fire the carried describe-refresh cmd.
type fieldDeletedMsg struct {
	cacheKey string
	innerCmd tea.Cmd
}

// saveFieldMetaString returns the Save closure for a string-valued
// Metadata key on the current CustomField.
func saveFieldMetaString(metaKey string) func(fieldCtx, string, any) error {
	return func(c fieldCtx, val string, _ any) error {
		id, err := customFieldIDCached(c.OrgData, c.Alias, c.Sobject, c.Field.Name)
		if err != nil {
			return err
		}
		_, err = c.Metadata.Update(context.Background(), metadataops.UpdateInput{
			Target: c.Alias, Type: "CustomField", ID: id, Patch: map[string]any{metaKey: val},
		})
		return err
	}
}

// saveFieldMetaAny returns the Save closure for an arbitrary-typed
// Metadata key. Used by booleans — Salesforce accepts Go's bool
// literal in the JSON payload.
func saveFieldMetaAny(metaKey string) func(fieldCtx, any) error {
	return func(c fieldCtx, val any) error {
		id, err := customFieldIDCached(c.OrgData, c.Alias, c.Sobject, c.Field.Name)
		if err != nil {
			return err
		}
		_, err = c.Metadata.Update(context.Background(), metadataops.UpdateInput{
			Target: c.Alias, Type: "CustomField", ID: id, Patch: map[string]any{metaKey: val},
		})
		return err
	}
}

// loadFieldMetaString returns a LoadCurrent closure that reads one
// string Metadata key via Tooling. Used for properties the describe
// doesn't expose (CustomField.description).
func loadFieldMetaString(metaKey string) func(fieldCtx) func() (string, error) {
	return func(c fieldCtx) func() (string, error) {
		return func() (string, error) {
			id, err := customFieldIDCached(c.OrgData, c.Alias, c.Sobject, c.Field.Name)
			if err != nil {
				return "", err
			}
			meta, err := sf.GetToolingMetadata(c.Alias, "CustomField", id)
			if err != nil {
				return "", err
			}
			if v, ok := meta[metaKey].(string); ok {
				return v, nil
			}
			return "", nil
		}
	}
}

func fieldSuccessFlash(c fieldCtx) string {
	return "updated " + c.Sobject + "." + c.Field.Name
}

// refreshFieldDescribeFor re-pulls the describe so the saved value
// shows up in the field list + detail view without a manual r.
func refreshFieldDescribeFor(c fieldCtx) tea.Cmd {
	if c.DescribeRes == nil {
		return nil
	}
	return c.DescribeRes.Refresh(c.Cache)
}

// refreshFieldDescriptionAfterEdit is the description action's success
// hook: the describe doesn't carry the description, so a describe refresh
// alone would leave the field-detail page showing the stale cached value.
// Drop the cached entry and re-fetch it via Tooling, batched with the
// normal describe refresh (which keeps label/help/default fresh).
func refreshFieldDescriptionAfterEdit(c fieldCtx) tea.Cmd {
	if c.OrgData != nil && c.OrgData.FieldDescriptions != nil {
		delete(c.OrgData.FieldDescriptions, c.Sobject+"."+c.Field.Name)
	}
	alias, sobject, field := c.Alias, c.Sobject, c.Field.Name
	orgUser := c.OrgUser
	key := sobject + "." + field
	refetch := func() tea.Msg {
		id, err := sf.CustomFieldID(alias, sobject, field)
		if err != nil {
			return fieldDescriptionLoadedMsg{orgUser: orgUser, key: key, err: err}
		}
		meta, err := sf.GetToolingMetadata(alias, "CustomField", id)
		if err != nil {
			return fieldDescriptionLoadedMsg{orgUser: orgUser, key: key, err: err}
		}
		desc, _ := meta["description"].(string)
		return fieldDescriptionLoadedMsg{orgUser: orgUser, key: key, desc: desc}
	}
	return tea.Batch(refreshFieldDescribeFor(c), refetch)
}

// orgAlias returns the preferred target argument for sf shelling out
// / REST bootstrap — alias wins, username is the fallback.
func orgAlias(o sf.Org) string {
	if o.Alias != "" {
		return o.Alias
	}
	return o.Username
}

// customFieldIDCached looks up a CustomField's Tooling Id, caching
// the result on orgData. The Id is stable for the life of the field.
// customFieldIDCached resolves a field's Tooling CustomField.Id with a
// per-org cache. Called from edit-modal LoadCurrent/Save closures that
// run on tea.Cmd GOROUTINES, so both the read and the write take
// d.customIDMu — the main loop deletes entries from the same map (field
// delete) and an unlocked concurrent access is a fatal map race.
func customFieldIDCached(d *orgData, alias, sobject, fieldAPI string) (string, error) {
	key := sobject + "." + fieldAPI
	if d != nil {
		d.customIDMu.Lock()
		id, ok := d.CustomFieldIDs[key]
		d.customIDMu.Unlock()
		if ok && id != "" {
			return id, nil
		}
	}
	id, err := sf.CustomFieldID(alias, sobject, fieldAPI)
	if err != nil {
		return "", err
	}
	if d != nil {
		d.customIDMu.Lock()
		d.CustomFieldIDs[key] = id
		d.customIDMu.Unlock()
	}
	return id, nil
}
