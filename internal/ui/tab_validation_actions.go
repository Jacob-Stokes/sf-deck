package ui

// Validation-rule action menu — uses the generic Action[Ctx] core
// plus the ToolingEntity write surface for all PATCH/delete calls.
// Declaration-only: no per-entity save/load helpers, those live in
// actions.go as generic ToolingMetaKeyHelpers / ToolingBoolSaver /
// ToolingDelete.

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/services/metadataops"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type validationCtx struct {
	Alias    string
	OrgData  *orgData
	Sobject  string
	Rule     sf.ValidationRuleRow
	RulesRes *Resource[[]sf.ValidationRuleRow]
	Cache    *cache.Cache
	Metadata *metadataops.Service
}

// Entity returns the ToolingEntity that every PATCH / delete call
// targets. Implements the entity() extractor contract used by the
// generic helpers in actions.go.
func (c validationCtx) Entity() sf.ToolingEntity {
	return sf.ToolingEntity{
		Target: c.Alias,
		Type:   "ValidationRule",
		ID:     c.Rule.ID,
	}
}

var validationRegistry = ActionRegistry[validationCtx]{
	BuildContext: buildValidationCtx,
	Actions:      validationActionsFor,
}

func buildValidationCtx(m Model) (validationCtx, bool, string) {
	o, ok := m.currentOrg()
	if !ok {
		return validationCtx{}, false, ""
	}
	d := m.ensureOrgDataRef(o.Username)
	if d == nil || d.DescribeCur == "" {
		return validationCtx{}, false, ""
	}
	r, ok := d.ValidationRules.Lists[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		return validationCtx{}, false, "validation rules not loaded yet — press " + firstPretty(Keys.Refresh) + " to refresh"
	}
	rules := r.Value()
	if len(rules) == 0 {
		return validationCtx{}, false, ""
	}
	// Prefer drilled-id lookup; fall back to cursored row.
	var rule sf.ValidationRuleRow
	found := false
	if d.ValidationRules.DrillID != "" {
		for _, x := range rules {
			if x.ID == d.ValidationRules.DrillID {
				rule = x
				found = true
				break
			}
		}
	}
	if !found {
		idx := d.ValidationRules.Cursors[d.DescribeCur]
		if idx < 0 || idx >= len(rules) {
			idx = 0
		}
		rule = rules[idx]
	}
	return validationCtx{
		Alias:    orgAlias(o),
		OrgData:  d,
		Sobject:  d.DescribeCur,
		Rule:     rule,
		RulesRes: r,
		Cache:    m.cache,
		Metadata: metadataWriteService(m, o),
	}, true, ""
}

// validationEntity adapts validationCtx.Entity for the generic
// helpers; a func (not a method reference) so generics can infer.
func validationEntity(c validationCtx) sf.ToolingEntity       { return c.Entity() }
func validationMetadata(c validationCtx) *metadataops.Service { return c.Metadata }

// vrText is a tiny helper wrapping textKey → (LoadCurrent, Save)
// for validation rules. Saves repeating the closure types at every
// NewTextAction call.
func vrText(key string) (
	func(validationCtx) func() (string, error),
	func(validationCtx, string, any) error,
) {
	return ToolingMetaKeyHelpers(validationEntity, validationMetadata, key)
}

func validationActionsFor(ctx validationCtx) []Action[validationCtx] {
	titleFor := func(label string) func(validationCtx) string {
		return func(c validationCtx) string {
			return actionTitleFor(c.Sobject, c.Rule.ValidationName, label)
		}
	}
	success := func(c validationCtx) string { return "updated " + c.Rule.ValidationName }

	loadErrMsg, saveErrMsg := vrText("errorMessage")
	loadFormula, saveFormula := vrText("errorConditionFormula")
	loadDesc, saveDesc := vrText("description")

	return []Action[validationCtx]{
		NewBooleanAction(BooleanActionSpec[validationCtx]{
			ID:           "vr-toggle-active",
			Label:        "Toggle active",
			Hint:         "Enables or disables this rule.",
			Title:        titleFor("Toggle active"),
			TrueOption:   ChoiceOpt{Label: "Active", Hint: "Rule enforces on insert/update.", Value: true},
			FalseOption:  ChoiceOpt{Label: "Inactive", Hint: "Rule is defined but doesn't fire.", Value: false},
			Current:      func(c validationCtx) bool { return c.Rule.Active },
			Save:         ToolingBoolSaver(validationEntity, validationMetadata, "active"),
			SuccessFlash: success,
			OnSuccess:    refreshValidationFor,
		}),
		NewTextAction(TextActionSpec[validationCtx]{
			ID:           "vr-edit-error-msg",
			Label:        "Edit error message",
			Hint:         "Message shown to users when the rule fires.",
			Multiline:    true,
			Title:        titleFor("Edit error message"),
			LoadCurrent:  loadErrMsg,
			Save:         saveErrMsg,
			SuccessFlash: success,
			OnSuccess:    refreshValidationFor,
		}),
		NewTextAction(TextActionSpec[validationCtx]{
			ID:           "vr-edit-error-formula",
			Label:        "Edit error-condition formula",
			Hint:         "Boolean formula — true triggers the error.",
			Multiline:    true,
			Title:        titleFor("Edit error-condition formula"),
			LoadCurrent:  loadFormula,
			Save:         saveFormula,
			SuccessFlash: success,
			OnSuccess:    refreshValidationFor,
		}),
		NewTextAction(TextActionSpec[validationCtx]{
			ID:           "vr-edit-description",
			Label:        "Edit description",
			Hint:         "Internal description visible in Setup.",
			Multiline:    true,
			Title:        titleFor("Edit description"),
			LoadCurrent:  loadDesc,
			Save:         saveDesc,
			SuccessFlash: success,
			OnSuccess:    refreshValidationFor,
		}),
		NewDestructiveAction(DestructiveActionSpec[validationCtx]{
			ID:           "vr-delete",
			Label:        "Delete rule",
			Hint:         "Permanently removes this validation rule.",
			Title:        titleFor("Delete rule"),
			ConfirmHint:  "No undo.",
			Save:         ToolingDelete(validationEntity, validationMetadata),
			SuccessFlash: func(c validationCtx) string { return "deleted " + c.Rule.ValidationName },
			OnSuccess:    validationDeletedCmd,
		}),
	}
}

// validationDeletedCmd fires after a rule delete: list refresh +
// pop-back msg. Pulled out of the inline spec so future entities
// can mirror the shape without duplicating the closure.
func validationDeletedCmd(c validationCtx) tea.Cmd {
	if c.RulesRes == nil {
		return nil
	}
	refresh := c.RulesRes.Refresh(c.Cache)
	ruleID := c.Rule.ID
	alias := c.Alias
	return func() tea.Msg {
		return validationRulePoppedMsg{
			alias:    alias,
			ruleID:   ruleID,
			innerCmd: refresh,
		}
	}
}

// validationRulePoppedMsg signals that a rule was deleted and we
// should pop back from TabValidationDetail to TabObjectDetail +
// Validation subtab, then fire the carried refresh cmd.
type validationRulePoppedMsg struct {
	alias    string
	ruleID   string
	innerCmd tea.Cmd
}

// refreshValidationFor is the shared OnSuccess: list + drilled
// detail via SObjectChildren.RefreshSObject.
func refreshValidationFor(c validationCtx) tea.Cmd {
	if c.OrgData == nil {
		return nil
	}
	return c.OrgData.ValidationRules.RefreshSObject(c.Sobject, c.Cache)
}
