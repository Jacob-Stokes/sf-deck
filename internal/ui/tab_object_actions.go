package ui

// Object-level action menu — uses the generic Action[Ctx] core.
//
// Writes go through the Metadata API (DeployCustomObjectPatch),
// not Tooling REST. See README_API_ROUTING.md for the split.
// Every text edit ships with a Preview closure that fetches the
// baseline and builds a diff before commit, so the user can see
// what else rides in the CustomObject deploy envelope.

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/services/metadataops"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type objectCtx struct {
	Alias       string
	OrgData     *orgData
	Sobject     string
	Describe    sf.SObjectDescribe
	DescribeRes *Resource[sf.SObjectDescribe]
	Cache       *cache.Cache
	Editors     *metadataops.EditorService
}

var objectRegistry = ActionRegistry[objectCtx]{
	BuildContext: buildObjectCtx,
	Actions:      objectActionsFor,
}

func buildObjectCtx(m Model) (objectCtx, bool, string) {
	o, ok := m.currentOrg()
	if !ok {
		return objectCtx{}, false, ""
	}
	d := m.ensureOrgDataRef(o.Username)
	if d == nil || d.DescribeCur == "" {
		return objectCtx{}, false, ""
	}
	r, ok := d.Describes[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		return objectCtx{}, false, "describe not loaded yet — press " + firstPretty(Keys.Refresh) + " to refresh"
	}
	return objectCtx{
		Alias:       orgAlias(o),
		OrgData:     d,
		Sobject:     d.DescribeCur,
		Describe:    r.Value(),
		DescribeRes: r,
		Cache:       m.cache,
		Editors:     metadataEditorService(m, o),
	}, true, ""
}

// customObjectOnly blocks object-level actions on standard objects
// (which don't have a CustomObject row, so the Metadata deploy would
// fail). Reused on every action in this registry.
func customObjectOnly() func(objectCtx) (bool, string) {
	return func(c objectCtx) (bool, string) {
		if !c.Describe.Custom {
			return true, "can only edit this on custom objects"
		}
		return false, ""
	}
}

func objectActionsFor(ctx objectCtx) []Action[objectCtx] {
	return []Action[objectCtx]{
		objectTextEdit("obj-edit-label", "Edit object label",
			"User-facing singular label (e.g. 'Account').",
			"label", false,
			func(c objectCtx) string { return c.Describe.Label }),
		objectTextEdit("obj-edit-plural-label", "Edit object plural label",
			"User-facing plural label (e.g. 'Accounts').",
			"pluralLabel", false,
			func(c objectCtx) string { return c.Describe.LabelPlural }),
		objectTextEdit("obj-edit-description", "Edit object description",
			"Internal description visible in Setup / Object Manager.",
			"description", true, nil),
		objectBoolToggle("obj-toggle-reports", "Toggle 'Allow Reports'",
			"Whether records of this object appear in the reporting catalog.",
			"enableReports",
			"Reports allowed", "Object is available in Report Builder.",
			"Reports disabled", "Hidden from Report Builder.",
			func(b *sf.CustomObjectBaseline) *bool { return b.EnableReports }),
		objectBoolToggle("obj-toggle-activities", "Toggle 'Allow Activities'",
			"Whether Tasks + Events can relate to this object.",
			"enableActivities",
			"Activities enabled", "Tasks + Events can link to records of this object.",
			"Activities disabled", "No Tasks/Events relation — can't re-enable later cleanly.",
			func(b *sf.CustomObjectBaseline) *bool { return b.EnableActivities }),
		objectBoolToggle("obj-toggle-feeds", "Toggle 'Enable Chatter Feeds'",
			"Whether this object exposes a Chatter feed on records.",
			"enableFeeds",
			"Feeds enabled", "Records get a Chatter feed panel.",
			"Feeds disabled", "No Chatter panel on this object.",
			func(b *sf.CustomObjectBaseline) *bool { return b.EnableFeeds }),
		objectBoolToggle("obj-toggle-history", "Toggle 'Track Field History'",
			"Whether field changes are recorded in the object's history.",
			"enableHistory",
			"History tracked", "Per-field trackHistory flags become effective.",
			"No history", "Field changes are not audited.",
			func(b *sf.CustomObjectBaseline) *bool { return b.EnableHistory }),
		objectBoolToggle("obj-toggle-search", "Toggle 'Allow Search'",
			"Whether records of this object appear in global search.",
			"enableSearch",
			"Search allowed", "Object is searchable via global search.",
			"Search disabled", "Records won't surface in global-search results.",
			func(b *sf.CustomObjectBaseline) *bool { return b.EnableSearch }),
	}
}

// objectTextEdit builds a NewTextAction for a single string field on
// CustomObject. When `initial` is nil the current value loads
// asynchronously via Tooling. Every text edit wires the Preview
// closure so the user sees the deploy diff before committing.
func objectTextEdit(id, label, hint, metaKey string, multiline bool, initial func(objectCtx) string) Action[objectCtx] {
	return NewTextAction(TextActionSpec[objectCtx]{
		ID:        id,
		Label:     label,
		Hint:      hint,
		Multiline: multiline,
		Title: func(c objectCtx) string {
			return actionTitleFor(c.Sobject, "", label)
		},
		InitialBody: initial,
		LoadCurrent: func() func(objectCtx) func() (string, error) {
			if initial != nil {
				return nil // synchronous init wins
			}
			return loadObjectMetaString(metaKey)
		}(),
		Save:         saveObjectMetaString(metaKey),
		Preview:      previewObjectMetaString(metaKey),
		SuccessFlash: func(c objectCtx) string { return "updated " + c.Sobject },
		OnSuccess:    refreshObjectDescribeFor,
		Disabled:     customObjectOnly(),
	})
}

// objectBoolToggle builds a NewBooleanAction for a single enable*
// flag. The current state is read from the cached
// CustomObjectBaseline (prefetched in ensureObjectDetailData via
// the Tooling CustomObject GET). When the baseline isn't loaded
// yet, or Salesforce didn't return a value for this flag, Current
// returns false and the cursor defaults to the False option — the
// deploy preserves what it doesn't explicitly set either way so a
// "wrong default" never causes drift.
//
// `currentExtract` pulls the *bool flag from the baseline so each
// toggle reads its own field (EnableReports, EnableActivities, …).
func objectBoolToggle(
	id, label, hint, metaKey, trueLabel, trueHint, falseLabel, falseHint string,
	currentExtract func(*sf.CustomObjectBaseline) *bool,
) Action[objectCtx] {
	return NewBooleanAction(BooleanActionSpec[objectCtx]{
		ID:    id,
		Label: label,
		Hint:  hint,
		Title: func(c objectCtx) string {
			return actionTitleFor(c.Sobject, "", label) + currentStateSuffix(c, currentExtract)
		},
		TrueOption:  ChoiceOpt{Label: trueLabel, Hint: trueHint, Value: true},
		FalseOption: ChoiceOpt{Label: falseLabel, Hint: falseHint, Value: false},
		Current: func(c objectCtx) bool {
			b := readBaselineToggle(c, currentExtract)
			return b != nil && *b
		},
		Save:         saveObjectMetaBool(metaKey),
		SuccessFlash: func(c objectCtx) string { return "updated " + c.Sobject },
		OnSuccess:    refreshObjectDescribeFor,
		Disabled:     customObjectOnly(),
	})
}

// readBaselineToggle pulls the cached CustomObjectBaseline for the
// active sobject and runs the per-flag extractor. Returns nil when
// the baseline hasn't loaded yet or when Salesforce didn't return a
// value for this particular flag — caller treats nil as "default
// to False / unknown."
func readBaselineToggle(c objectCtx, extract func(*sf.CustomObjectBaseline) *bool) *bool {
	if c.OrgData == nil || extract == nil {
		return nil
	}
	r, ok := c.OrgData.CustomObjectBaselines[c.Sobject]
	if !ok || r.FetchedAt().IsZero() {
		return nil
	}
	base := r.Value()
	if base == nil {
		return nil
	}
	return extract(base)
}

// currentStateSuffix appends "(currently: enabled)" / "(currently:
// disabled)" / "(current state unknown)" to the modal title so the
// user sees the actual state at a glance — not just the cursor
// position. nil pointer = unknown (baseline not loaded or SF
// didn't return a value for this flag).
func currentStateSuffix(c objectCtx, extract func(*sf.CustomObjectBaseline) *bool) string {
	b := readBaselineToggle(c, extract)
	if b == nil {
		return " — current state unknown"
	}
	if *b {
		return " — currently: enabled"
	}
	return " — currently: disabled"
}

// saveObjectMetaString builds the Save closure for a single string
// field on CustomObject. Uses the baseline handed over by Preview
// when available so the commit skips the re-fetch; otherwise
// DeployCustomObjectPatch does its own round-trip.
func saveObjectMetaString(metaKey string) func(objectCtx, string, any) error {
	return func(c objectCtx, val string, baseline any) error {
		logDeploy("saveObject metaKey=%s val=%q baseline?=%v", metaKey, val, baseline != nil)
		patch, err := stringPatchFor(metaKey, val)
		if err != nil {
			return err
		}
		in := metadataops.ObjectDeployInput{Target: c.Alias, APIName: c.Sobject, Patch: patch}
		if b, ok := baseline.(*sf.CustomObjectBaseline); ok && b != nil {
			in.Baseline = b
		}
		result, err := c.Editors.DeployObject(context.Background(), in)
		if err != nil {
			return err
		}
		if !result.Deploy.Success {
			return deployResultErrorForUI(result.Deploy)
		}
		return nil
	}
}

// saveObjectMetaBool is the bool equivalent: one enable* flag
// through a CustomObject deploy. No baseline round-trip — toggles
// are rarely re-edited, and the deploy preserves un-set fields.
func saveObjectMetaBool(metaKey string) func(objectCtx, any) error {
	return func(c objectCtx, val any) error {
		b, _ := val.(bool)
		patch, err := boolPatchFor(metaKey, b)
		if err != nil {
			return err
		}
		result, err := c.Editors.DeployObject(context.Background(), metadataops.ObjectDeployInput{
			Target: c.Alias, APIName: c.Sobject, Patch: patch,
		})
		if err != nil {
			return err
		}
		if !result.Deploy.Success {
			return deployResultErrorForUI(result.Deploy)
		}
		return nil
	}
}

// loadObjectMetaString is a LoadCurrent for the subset of CustomObject
// metadata that *is* queryable via Tooling (only description today).
// Label / pluralLabel use synchronous InitialBody from the describe
// so they don't need to hit this path.
func loadObjectMetaString(metaKey string) func(objectCtx) func() (string, error) {
	return func(c objectCtx) func() (string, error) {
		return func() (string, error) {
			if metaKey != "description" {
				return "", nil
			}
			id, err := customObjectIDCached(c.OrgData, c.Alias, c.Sobject)
			if err != nil {
				return "", err
			}
			return sf.GetCustomObjectDescription(c.Alias, id)
		}
	}
}

// previewObjectMetaString fetches the baseline + builds the diff
// lines the edit-modal renders before commit. Returns the baseline
// as the opaque token Save re-uses so no duplicate round-trip.
//
// Cache discipline: prefers the cached baseline Resource when fresh,
// falling back to a direct fetch only when the Resource is empty or
// unavailable. Without this the modal-prep path used to fire 4 API
// calls (describe + 2 tooling + EntityDefinition) every single time
// even though EnsureCustomObjectBaseline holds the same data with
// a 10-minute TTL — observed in the api-trace as a duplicate
// baseline sequence ~20s after the original drill-in.
func previewObjectMetaString(metaKey string) func(objectCtx, string) (PreviewResult, error) {
	return func(c objectCtx, val string) (PreviewResult, error) {
		var base *sf.CustomObjectBaseline
		// Cached path: ensure-and-read. If the Resource already has a
		// fresh value, .Value() returns it without firing the Fetch
		// closure. Stale values still get served here — the post-
		// deploy Refresh in saveObjectMeta is what guarantees the
		// baseline reflects the latest state after a write.
		if c.OrgData != nil {
			if r := c.OrgData.EnsureCustomObjectBaseline(c.Alias, c.Sobject); r != nil {
				if v, ok := r.Get(); ok && v != nil {
					base = v
				}
			}
		}
		if base == nil {
			// Cache miss — fall back to a direct fetch. Rare in
			// practice (the baseline was loaded on drill-in).
			b, err := sf.FetchCustomObjectBaseline(c.Alias, c.Sobject)
			if err != nil {
				return PreviewResult{}, err
			}
			base = b
		}
		lines := []PreviewLine{
			previewTextLine("label", base.Label, metaKey, val, "label"),
			previewTextLine("pluralLabel", base.PluralLabel, metaKey, val, "pluralLabel"),
			previewTextLine("description", base.Description, metaKey, val, "description"),
			// sharingModel + nameField ship with the deploy too —
			// show them as unchanged so the user knows.
			{Field: "sharingModel", Before: base.SharingModel, After: base.SharingModel, Changed: false},
			{Field: "nameField",
				Before: base.NameFieldType + " / " + base.NameFieldLabel,
				After:  base.NameFieldType + " / " + base.NameFieldLabel, Changed: false},
		}
		return PreviewResult{Lines: lines, Baseline: base}, nil
	}
}

func refreshObjectDescribeFor(c objectCtx) tea.Cmd {
	var cmds []tea.Cmd
	if c.DescribeRes != nil {
		if cmd := c.DescribeRes.Refresh(c.Cache); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	// Re-fetch the CustomObject baseline so the next time the user
	// opens a toggle modal (or peeks at the Details panel) the
	// "currently: enabled/disabled" suffix reflects the just-
	// deployed state. Without this the cached baseline keeps the
	// pre-deploy values forever.
	if c.OrgData != nil {
		if r := c.OrgData.EnsureCustomObjectBaseline(c.Alias, c.Sobject); r != nil {
			if cmd := r.Refresh(c.Cache); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	if len(cmds) == 1 {
		return cmds[0]
	}
	return tea.Batch(cmds...)
}

// --- deploy-patch shape helpers ----------------------------------------

// stringPatchFor builds a one-field CustomObjectPatch for a string-
// valued metadata key.
func stringPatchFor(metaKey, val string) (sf.CustomObjectPatch, error) {
	patch := sf.CustomObjectPatch{}
	switch metaKey {
	case "label":
		patch.Label = val
	case "pluralLabel":
		patch.PluralLabel = val
	case "description":
		patch.Description = val
	default:
		return patch, fmt.Errorf("unsupported metadata key for object text edit: %s", metaKey)
	}
	return patch, nil
}

// boolPatchFor builds a one-field CustomObjectPatch for a bool-valued
// enable* flag.
func boolPatchFor(metaKey string, val bool) (sf.CustomObjectPatch, error) {
	patch := sf.CustomObjectPatch{}
	b := sf.BoolPtr(val)
	switch metaKey {
	case "enableReports":
		patch.EnableReports = b
	case "enableActivities":
		patch.EnableActivities = b
	case "enableHistory":
		patch.EnableHistory = b
	case "enableFeeds":
		patch.EnableFeeds = b
	case "enableSearch":
		patch.EnableSearch = b
	default:
		return patch, fmt.Errorf("unsupported metadata key for object toggle: %s", metaKey)
	}
	return patch, nil
}

// previewTextLine builds one diff row for a string field.
func previewTextLine(field, current, editingKey, newVal, myKey string) PreviewLine {
	if field == editingKey {
		return PreviewLine{Field: field, Before: current, After: newVal, Changed: current != newVal}
	}
	return PreviewLine{Field: field, Before: current, After: current, Changed: false}
}

// deployResultErrorForUI turns a non-success DeployResult into an
// error the UI can render.
func deployResultErrorForUI(r *sf.DeployResult) error {
	msg := r.FirstError
	if msg == "" {
		msg = fmt.Sprintf("deploy %s: %s", r.Status, r.ID)
	}
	return fmt.Errorf("deploy failed: %s", msg)
}

// customObjectIDCached caches CustomObject.Id lookups on orgData.
// Runs on edit-modal goroutines (LoadCurrent/Save closures), so all map
// access — including the lazy re-init — takes d.customIDMu; see
// customFieldIDCached for the race this prevents.
func customObjectIDCached(d *orgData, alias, sobject string) (string, error) {
	if d != nil {
		d.customIDMu.Lock()
		id, ok := d.CustomObjectIDs[sobject]
		d.customIDMu.Unlock()
		if ok && id != "" {
			return id, nil
		}
	}
	id, err := sf.CustomObjectID(alias, sobject)
	if err != nil {
		return "", err
	}
	if d != nil {
		d.customIDMu.Lock()
		if d.CustomObjectIDs == nil {
			d.CustomObjectIDs = map[string]string{}
		}
		d.CustomObjectIDs[sobject] = id
		d.customIDMu.Unlock()
	}
	return id, nil
}

// logDeploy writes a UI-side trace to ~/.sf-deck/deploy.log when
// SFDECK_DEBUG_DEPLOY=1. Mirrors the sf-package dlogf so deploy-
// hang reports can distinguish "UI fired save" from "HTTP went out".
func logDeploy(format string, args ...any) {
	if os.Getenv("SFDECK_DEBUG_DEPLOY") != "1" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	f, err := os.OpenFile(home+"/.sf-deck/deploy.log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[ui] "+format+"\n", args...)
}
