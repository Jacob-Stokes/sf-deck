package ui

// Modal/overlay state + small session-scoped UI cursors that don't
// belong to orgData.
//
// Extracted from model.go. modelTransient is embedded into Model so
// existing field access (m.openMenu, m.editModal, m.chipWizard, …)
// keeps working unchanged.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/exporters/bulk"
)

// modelTransient owns modal/overlay state plus small session-scoped UI
// cursors that do not naturally belong to orgData.
type modelTransient struct {
	// openMenu is the transient overlay shown by shift+O / shift+Y so the
	// user can pick a non-default target from sf.Openable.Targets().
	// nil = hidden.
	openMenu *openMenuState

	// openMenuStack carries parent open-menu states when a synthetic
	// sub-modal opens on top (e.g. "Open related <sObject>…" picker
	// that swaps the menu to the related record's targets). Esc on
	// the sub-modal pops one frame instead of dismissing — only when
	// the stack is empty does esc close the overlay entirely.
	openMenuStack []openMenuState

	// move is an in-flight "move to the same resource in another org"
	// request, armed when the user picks a destination org from the
	// open menu's org sub-picker. It rides here until the target org's
	// list loads and resolvePendingMove either drills into the matched
	// resource or gives up. nil = no move pending. See move_org.go.
	move *pendingMove

	// infoModal is a dismiss-only read-only overlay (legends, per-view
	// help). nil = hidden. See modal.go.
	infoModal *infoModalState

	// soqlModal is a lightweight SOQL workspace over the current tab.
	// It owns an independent soqlSession so opening it from a related
	// row never tramples the user's top-level /soql query/results.
	soqlModal *soqlModalState

	// editModal is the active in-place text editor modal (e.g. edit
	// help text on a field). nil = hidden.
	editModal *editModalState

	// choiceModal is the active "pick one of these options" modal
	// (e.g. toggle a field's required / unique / externalId flags).
	// nil = hidden. See modal_choice.go.
	choiceModal *choiceModalState

	// walkthrough is the guided first-launch tour. Zero value =
	// inactive. Unlike the modals above it's non-blocking: a small
	// corner panel that lets the user navigate while it watches model
	// state to confirm each task. Advancement remains manual. See
	// walkthrough.go.
	walkthrough walkthroughState

	// commandPalette is the global fuzzy-find modal — bound to ;
	// (and discoverable via the help screen). Walks the TabSpec
	// registry + the keymap.Commands registry to surface every
	// reachable destination + action. nil = hidden.
	commandPalette *commandPaletteState

	// keybindingsModal is the user-editable list of every command
	// in the keymap registry. Reachable via the command palette
	// ("Edit keybindings…"). Edits are applied to ui.Keys in
	// place + persisted to ~/.sf-deck/keybindings.toml.
	keybindingsModal *keybindingsModalState

	tagPicker  *tagPickerState
	tagEditor  *tagEditorState
	tagsCursor int // cursor on the /tags master list

	// Drill-in state for TabTagDetail. tagCur is the ID of the
	// drilled-in tag; tagItems is a per-Model ListView of the items
	// carrying that tag across every org (tags are org-independent
	// so this lives here rather than per-orgData). tagKindChip is
	// the active kind-filter chip on the detail view, mirroring the
	// dev-project pattern. All cleared on esc back.
	tagCur            int64
	tagItems          ListView[devproject.Item]
	tagKindChip       devproject.ItemKind
	tagKindChipCursor int

	// homeBadgeFrame cycles 0..N-1 to drive the cloud-banner
	// animation on the Home tab. Advanced by a tea.Tick when the
	// active tab is /home; left untouched elsewhere so the banner
	// resumes its rotation from where it left off after navigating
	// away and back.
	homeBadgeFrame int

	// homeBadgeTickRunning is the single-flight guard for the
	// banner-tick scheduler. Without it, every entry to /home
	// (org switch, drill-back, refresh) would queue another
	// tea.Tick — and once N ticks are in flight they all advance
	// homeBadgeFrame on the same period, so the animation appears
	// to speed up linearly with the number of times the user
	// returned to /home. Setting + clearing this flag in the tick
	// handler keeps exactly one timer alive at a time.
	homeBadgeTickRunning bool

	// orgPicker is the multi-select org-list modal used by the
	// dev-project create wizard. nil = hidden. See modal_org_picker.go.
	orgPicker *orgPickerState

	// downloadsModal is the Ctrl+J overlay that lists in-flight +
	// recent exports. nil = hidden. See modal_downloads.go.
	downloadsModal *downloadsModalState

	// homeDownloadsCursor is the row cursor on the /home Downloads
	// subtab. Indexes into the merged inflight+history slice the
	// renderer builds. Bespoke (rather than ListView-backed) because
	// the data lives on Model.exports, not orgData.
	homeDownloadsCursor int

	// bundleCur is the ID of the drilled-in bundle when the user is on
	// TabBundleDetail. Set by activateBundles when Enter fires on a
	// /bundles row. Cleared on EscBack.
	bundleCur string

	// bundlePreviews caches the retrieve/deploy preview output keyed
	// by bundle ID. Populated lazily on TabBundleDetail entry; reset
	// after a successful retrieve or deploy so the diff matches the
	// new state. Stale entries are fine — the user can refresh with r.
	bundlePreviews map[string]bundlePreview

	// deepCollect is the change-set-style "what to bring along" wizard
	// fired when shift+K targets a container kind (sObject today; will
	// grow to permset/profile). nil = hidden. See modal_deep_collect.go.
	deepCollect *deepCollectState

	// globalSearch is the cross-cutting Force-Navigator-style modal
	// (ctrl+k by default). nil = hidden. See search_global.go.
	globalSearch *globalSearchState

	// themePicker is the floating top-right theme browser. nil = hidden.
	// See modal_theme_picker.go.
	themePicker *themePickerState

	// chipWizard is the unified multi-field + advanced-SOQL editor
	// used to author chips on every surface (records / objects /
	// flows). nil = hidden. See chip_wizard.go.
	chipWizard *chipWizardState

	// picker is the reusable anchored-dropdown overlay. nil = hidden.
	// See picker.go. Generic over the item type at the call site;
	// the runtime state is type-erased so Model holds a single
	// non-generic field.
	picker *pickerState

	// cacheSettings is the cache & refresh policy overview modal.
	// nil = hidden. Reached from = → Cache & refresh policy.
	cacheSettings *cacheSettingsState

	// compareEdit is the /compare edit-saved-comparison modal. nil =
	// hidden. Opened via `e` on a saved comparison (row or loaded
	// inventory); owns the edit/clone state so it can't leak onto the
	// New subtab.
	compareEdit *compareEditModalState

	// compareScope is the /compare scope multi-select modal. nil =
	// hidden. Opened from the New setup form or the edit modal; writes
	// the chosen types back via its OnConfirm callback.
	compareScope *compareScopeModalState

	// compareTypesRefreshed tracks, per org alias, whether this SESSION
	// has already re-fetched the metadata-type catalog via
	// describeMetadata. The first scope-open per org per session refreshes
	// (types change rarely; relaunch is the refresh); later opens read the
	// kv cache. nil until first use; lazily made in loadComparableTypes.
	compareTypesRefreshed map[string]bool

	// listTableWidthPrefs caches persisted per-org column width
	// overrides from cache.db. Map value is shared across Model value
	// copies; the loaded sentinel avoids hitting SQLite from render
	// hot paths after the first lookup.
	listTableWidthPrefs       map[string]*listTableWidthPrefs
	listTableWidthPrefsLoaded map[string]bool

	// perViewSort stashes each view's sort when [ui] sort_per_view is on.
	// Keyed by "<widthScope>|<chipID>" so switching views restores that
	// view's sort into the shared ListTableState. Session-only (not
	// persisted). See applyPerViewSort. Empty/nil when the setting is
	// off — sort then stays shared across views, the default.
	perViewSort    map[string]sortPref
	perViewSortKey string // the key the shared state currently reflects

	// bulkExport holds the in-flight Bulk-API full-dataset export's
	// handles (event channel + cancel func + label). nil when no
	// export is running. The UI cmd loop reads from .Events() and
	// re-arms; ctrl+c during the run calls .Cancel(). Owned by the
	// internal/exporters/bulk subpackage; Model just stores the pointer.
	bulkExport *bulk.Flight

	// exportSave is the two-field save-as modal opened after the
	// user picks export format/scope. Fields: path (editable) +
	// auto-open (checkbox). Confirm dispatches the actual export
	// kickoff message. nil = hidden. See modal_export_save.go.
	exportSave *exportSaveState
}
