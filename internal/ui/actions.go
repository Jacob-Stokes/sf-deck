package ui

// Generic action-registry core.
//
// Five TabXxxDetail surfaces (field / object / validation rule /
// record type / trigger) share the same UX: a right-sidebar menu of
// actions on the drilled-in entity; selecting one opens an edit or
// choice modal; save commits via the sf package. The plumbing is
// almost identical per entity — only the entity identity + the
// underlying PATCH/deploy helper change.
//
// This file owns the generic half of that pattern. Each entity file
// only needs to:
//
//   1. Define a Ctx struct (the per-selection snapshot — alias,
//      sobject, drilled entity, parent Resource, cache).
//   2. Implement a ContextBuilder that resolves the current Model
//      state into a Ctx (with safety gate + flash reasons baked in).
//   3. Declare its actions as Action[Ctx] values using NewTextAction,
//      NewBooleanAction, or inline Start closures for one-off cases
//      (delete, complex confirmations).
//   4. Call StartAction(m, registry, index) from its update_nav
//      activate branch.
//
// No more "per-entity variants of newTextAction + savePatch +
// loadCurrent + refreshX" — the helpers below close over the Ctx
// type via generics.

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/services/metadataops"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// Action is one selectable row in a drill-tab sidebar action menu.
// Ctx is the per-selection state that Start closes over — entity-
// specific (sf.Field, sf.ValidationRuleRow, …). The presentational
// bits (Label, Hint, Kind) drive the sidebar render + safety gate.
type Action[Ctx any] struct {
	// ID is a stable string used for debugging and (eventually)
	// user-configurable action pinning. Must be unique within one
	// entity's action list.
	ID string
	// Label is the left-side text shown in the sidebar menu.
	Label string
	// Hint is the one-line subtitle rendered under the label when
	// the cursor is on this action + reused as the modal subtitle.
	Hint string
	// Kind is the safety-level this action writes under. The
	// sidebar dims + unclickable for actions the org's safety
	// policy blocks.
	Kind settings.WriteKind
	// Disabled, when set, returns (true, reason) to dim the row +
	// surface the reason in the sidebar. Nil = always enabled (up
	// to the safety gate). Typical use: "only on custom entities",
	// "needs a drill-in target first", etc.
	Disabled func(Ctx) (bool, string)
	// Start opens whatever modal the action needs and returns the
	// tea.Cmd to fire. Nil = no-op (placeholder actions).
	Start func(m *Model, ctx Ctx) tea.Cmd
}

// ActionRegistry is the entity-specific bundle that update_nav's
// activate path calls into. All five entities implement this shape
// with their own Ctx.
type ActionRegistry[Ctx any] struct {
	// BuildContext resolves the Model into a typed Ctx, or returns
	// (_, false, reason) if preconditions aren't met. Reason is
	// flashed to the user. Lets each entity own "do we have a
	// drilled thing + cached state + …" checks.
	BuildContext func(m Model) (Ctx, bool, string)
	// Actions returns the action list for the current Ctx. Called
	// during both sidebar rendering (to show the menu) and
	// activation (to fire the selected action).
	Actions func(ctx Ctx) []Action[Ctx]
}

// StartAction is the single dispatcher used by every drill tab's
// activate path. Builds ctx, enforces safety gate + action-specific
// Disabled hook, hands off to act.Start.
func StartAction[Ctx any](m Model, reg ActionRegistry[Ctx], idx int) (Model, tea.Cmd) {
	ctx, ok, reason := reg.BuildContext(m)
	if !ok {
		if reason != "" {
			m.flash(reason)
		}
		return m, nil
	}
	acts := reg.Actions(ctx)
	if idx < 0 || idx >= len(acts) {
		return m, nil
	}
	act := acts[idx]
	if act.Disabled != nil {
		if blocked, why := act.Disabled(ctx); blocked {
			if why != "" {
				m.flash(why)
			}
			return m, nil
		}
	}
	if ok, why := m.canWriteCurrent(act.Kind); !ok {
		m.flash(why)
		return m, nil
	}
	if act.Start == nil {
		return m, nil
	}
	return m, act.Start(&m, ctx)
}

// RegistryRows renders the action list as the actionRow slice the
// sidebar rendering layer expects. Dims entries whose Disabled hook
// returns true or whose Kind exceeds the org's safety level.
func RegistryRows[Ctx any](m Model, reg ActionRegistry[Ctx]) []actionRow {
	ctx, ok, _ := reg.BuildContext(m)
	if !ok {
		return nil
	}
	o, okOrg := m.currentOrg()
	if !okOrg {
		return nil
	}
	lvl := m.safetyFor(o)
	acts := reg.Actions(ctx)
	rows := make([]actionRow, len(acts))
	for i, a := range acts {
		allowed := lvl.Allows(a.Kind)
		reason := ""
		if !allowed {
			reason = "blocked by safety (" + lvl.String() + ")"
		} else if a.Disabled != nil {
			if blocked, why := a.Disabled(ctx); blocked {
				allowed = false
				reason = why
			}
		}
		rows[i] = actionRow{
			Label:   a.Label,
			Hint:    a.Hint,
			Allowed: allowed,
			Reason:  reason,
		}
	}
	return rows
}

// TextActionSpec drives NewTextAction. The helpers for the two most
// common shapes (string-field edit, boolean toggle) are generic over
// Ctx so each entity gets them for free.
type TextActionSpec[Ctx any] struct {
	ID        string
	Label     string
	Hint      string
	Multiline bool
	// Title builds the modal title for a given ctx (e.g.
	// "Account.Name  —  Edit label").
	Title func(ctx Ctx) string
	// SuccessFlash is the banner shown after a successful commit.
	SuccessFlash func(ctx Ctx) string
	// InitialBody synchronously returns the current value to
	// pre-populate the editor. nil means "use LoadCurrent".
	InitialBody func(ctx Ctx) string
	// LoadCurrent is the async loader fired when the modal opens;
	// used for entities whose current value isn't in the describe
	// (e.g. CustomField.description). Ignored when InitialBody is
	// non-nil.
	LoadCurrent func(ctx Ctx) func() (string, error)
	// Save commits the new value. Returns an error the modal
	// renders; nil = success. Preview/baseline arg is passed
	// through from the editModalState contract.
	Save func(ctx Ctx, val string, baseline any) error
	// Preview, if set, shows a diff before commit (Metadata API
	// flow). nil = commit directly.
	Preview func(ctx Ctx, val string) (PreviewResult, error)
	// OnSuccess is the post-save refresh — typically the relevant
	// Resource.Refresh + detail-refresh cmd.
	OnSuccess func(ctx Ctx) tea.Cmd
	// Disabled mirrors Action.Disabled but is baked into the spec
	// for declaration-site convenience.
	Disabled func(Ctx) (bool, string)
	// Kind defaults to WriteMetadata; override for destructive
	// actions.
	Kind settings.WriteKind
}

// NewTextAction builds an Action that opens the edit-modal. Every
// entity's "edit label / edit description / edit formula" type
// actions use this instead of hand-rolling a Start closure.
func NewTextAction[Ctx any](spec TextActionSpec[Ctx]) Action[Ctx] {
	kind := spec.Kind
	if kind == 0 {
		kind = settings.WriteMetadata
	}
	return Action[Ctx]{
		ID:       spec.ID,
		Label:    spec.Label,
		Hint:     spec.Hint,
		Kind:     kind,
		Disabled: spec.Disabled,
		Start: func(m *Model, ctx Ctx) tea.Cmd {
			state := editModalState{
				Hint:      spec.Hint,
				Multiline: spec.Multiline,
			}
			if spec.Title != nil {
				state.Title = spec.Title(ctx)
			}
			if spec.SuccessFlash != nil {
				state.SuccessMsg = spec.SuccessFlash(ctx)
			}
			if spec.InitialBody != nil {
				state.InitialBody = spec.InitialBody(ctx)
			} else if spec.LoadCurrent != nil {
				state.LoadCurrent = spec.LoadCurrent(ctx)
			}
			if spec.Save != nil {
				save := spec.Save
				state.Save = func(val string, baseline any) error {
					return save(ctx, val, baseline)
				}
			}
			if spec.Preview != nil {
				preview := spec.Preview
				state.Preview = func(val string) (PreviewResult, error) {
					return preview(ctx, val)
				}
			}
			if spec.OnSuccess != nil {
				onSuccess := spec.OnSuccess
				state.OnSuccess = func() tea.Cmd {
					return onSuccess(ctx)
				}
			}
			return m.openEditModal(state)
		},
	}
}

// BooleanActionSpec drives NewBooleanAction. Two-option choice
// modal (Active/Inactive, Required/Optional, etc). Value type is
// any so entities can pass bool OR string — some Salesforce
// attributes take "Active"/"Inactive" strings, not booleans.
type BooleanActionSpec[Ctx any] struct {
	ID    string
	Label string
	Hint  string
	Title func(ctx Ctx) string
	// TrueOption / FalseOption describe the two choice rows.
	TrueOption  ChoiceOpt
	FalseOption ChoiceOpt
	// Current returns the current state — drives the default
	// cursor position. nil = default to False.
	Current func(ctx Ctx) bool
	// Save commits the picked option's Value. Callers typically
	// dispatch on val.(bool) or val.(string).
	Save         func(ctx Ctx, val any) error
	SuccessFlash func(ctx Ctx) string
	OnSuccess    func(ctx Ctx) tea.Cmd
	Disabled     func(Ctx) (bool, string)
	Kind         settings.WriteKind
}

// ChoiceOpt is the declaration-site shape for one option on a
// boolean action. Matches the existing choiceOption fields so the
// spec stays readable at call sites.
type ChoiceOpt struct {
	Label string
	Hint  string
	Value any
}

// NewBooleanAction builds an Action that opens a two-choice modal.
func NewBooleanAction[Ctx any](spec BooleanActionSpec[Ctx]) Action[Ctx] {
	kind := spec.Kind
	if kind == 0 {
		kind = settings.WriteMetadata
	}
	return Action[Ctx]{
		ID:       spec.ID,
		Label:    spec.Label,
		Hint:     spec.Hint,
		Kind:     kind,
		Disabled: spec.Disabled,
		Start: func(m *Model, ctx Ctx) tea.Cmd {
			options := []choiceOption{
				{Label: spec.TrueOption.Label, Hint: spec.TrueOption.Hint, Value: spec.TrueOption.Value},
				{Label: spec.FalseOption.Label, Hint: spec.FalseOption.Hint, Value: spec.FalseOption.Value},
			}
			cursor := 1
			if spec.Current != nil && spec.Current(ctx) {
				cursor = 0
			}
			state := choiceModalState{
				Hint:    spec.Hint,
				Options: options,
				Cursor:  cursor,
			}
			if spec.Title != nil {
				state.Title = spec.Title(ctx)
			}
			if spec.SuccessFlash != nil {
				state.SuccessMsg = spec.SuccessFlash(ctx)
			}
			if spec.Save != nil {
				save := spec.Save
				state.Save = func(val any) error {
					return save(ctx, val)
				}
			}
			if spec.OnSuccess != nil {
				onSuccess := spec.OnSuccess
				state.OnSuccess = func() tea.Cmd {
					return onSuccess(ctx)
				}
			}
			return m.openChoiceModal(state)
		},
	}
}

// DestructiveActionSpec drives NewDestructiveAction — the cancel-
// first confirmation flow shared by every "delete X" action. The
// cancel option comes first + is marked Cancel so esc/enter-on-
// cancel dismisses cleanly; the destructive row fires Save.
type DestructiveActionSpec[Ctx any] struct {
	ID           string
	Label        string
	Hint         string
	Title        func(ctx Ctx) string
	ConfirmHint  string // rendered next to the "Delete permanently" row
	SuccessFlash func(ctx Ctx) string
	// Save is the destroy call. Returns non-nil error = modal stays
	// open with the error.
	Save func(ctx Ctx) error
	// OnSuccess is the post-delete pop-back + list refresh. Almost
	// always a xxxPoppedMsg that Update handles.
	OnSuccess func(ctx Ctx) tea.Cmd
	Disabled  func(Ctx) (bool, string)
}

// NewDestructiveAction builds an Action that opens a "cancel or
// destroy permanently" choice modal. Uses the Cancel: true option
// flag so cancel short-circuits without firing Save.
func NewDestructiveAction[Ctx any](spec DestructiveActionSpec[Ctx]) Action[Ctx] {
	return Action[Ctx]{
		ID:       spec.ID,
		Label:    spec.Label,
		Hint:     spec.Hint,
		Kind:     settings.WriteAnonymous,
		Disabled: spec.Disabled,
		Start: func(m *Model, ctx Ctx) tea.Cmd {
			options := []choiceOption{
				{Label: "Cancel", Hint: "Don't delete.", Cancel: true},
				{Label: "Delete permanently", Hint: spec.ConfirmHint, Value: true},
			}
			state := choiceModalState{
				Hint:    spec.Hint,
				Options: options,
				Cursor:  0, // default to safe option
			}
			if spec.Title != nil {
				state.Title = spec.Title(ctx)
			}
			if spec.SuccessFlash != nil {
				state.SuccessMsg = spec.SuccessFlash(ctx)
			}
			if spec.Save != nil {
				save := spec.Save
				state.Save = func(_ any) error {
					return save(ctx)
				}
			}
			if spec.OnSuccess != nil {
				onSuccess := spec.OnSuccess
				state.OnSuccess = func() tea.Cmd {
					return onSuccess(ctx)
				}
			}
			return m.openChoiceModal(state)
		},
	}
}

// ToolingMetaKeyHelpers returns the LoadCurrent + Save pair wired
// to a specific Tooling entity + Metadata key. Collapses the
// per-entity saveXxxMetaString / loadXxxMetaString pairs that used
// to live in each _actions.go file into one generic helper.
//
// Usage inside a TextActionSpec[Ctx]:
//
//	load, save := ToolingMetaKeyHelpers(
//	    func(c Ctx) sf.ToolingEntity { return c.entity() },
//	    "errorMessage",
//	)
//	spec := TextActionSpec[Ctx]{
//	    LoadCurrent: load,
//	    Save:        save,
//	    ...
//	}
//
// The entity() extractor is the only entity-specific bit — it
// returns a fully-formed sf.ToolingEntity from the caller's ctx.
func ToolingMetaKeyHelpers[Ctx any](
	entity func(Ctx) sf.ToolingEntity,
	service func(Ctx) *metadataops.Service,
	key string,
) (
	loadCurrent func(Ctx) func() (string, error),
	save func(Ctx, string, any) error,
) {
	loadCurrent = func(c Ctx) func() (string, error) {
		e := entity(c)
		return func() (string, error) { return e.GetMetaString(key) }
	}
	save = func(c Ctx, val string, _ any) error {
		e := entity(c)
		_, err := service(c).Update(context.Background(), metadataops.UpdateInput{
			Target: e.Target, Type: e.Type, ID: e.ID, Patch: map[string]any{key: val},
		})
		return err
	}
	return
}

// ToolingBoolSaver returns a Save closure for a boolean Metadata
// key on a Tooling entity. Entity types differ in whether they
// accept a Go bool directly (most) or need it wrapped as a string
// ("Active"/"Inactive" for ApexTrigger.status) — the modal passes
// whichever Value the caller put on the ChoiceOpt, so this helper
// accepts `any` and forwards unchanged.
func ToolingBoolSaver[Ctx any](
	entity func(Ctx) sf.ToolingEntity,
	service func(Ctx) *metadataops.Service,
	key string,
) func(Ctx, any) error {
	return func(c Ctx, val any) error {
		e := entity(c)
		_, err := service(c).Update(context.Background(), metadataops.UpdateInput{
			Target: e.Target, Type: e.Type, ID: e.ID, Patch: map[string]any{key: val},
		})
		return err
	}
}

// ToolingDelete returns a Save closure for destructive actions
// against a Tooling entity. Binds to the entity's own Delete method.
func ToolingDelete[Ctx any](
	entity func(Ctx) sf.ToolingEntity,
	service func(Ctx) *metadataops.Service,
) func(Ctx) error {
	return func(c Ctx) error {
		e := entity(c)
		_, err := service(c).Delete(context.Background(), metadataops.DeleteInput{
			Target: e.Target, Type: e.Type, ID: e.ID,
		})
		return err
	}
}

// actionTitleFor is a tiny helper entity files use to build
// "<sobject>/<entity>  —  <label>" titles without string-fragment
// repetition. Sobject + EntityLabel become the left side;
// actionLabel becomes the right.
func actionTitleFor(sobject, entityLabel, actionLabel string) string {
	left := sobject
	if entityLabel != "" {
		left = fmt.Sprintf("%s / %s", sobject, entityLabel)
	}
	return fmt.Sprintf("%s  —  %s", left, actionLabel)
}
