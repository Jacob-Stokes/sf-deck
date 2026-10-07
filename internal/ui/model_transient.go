package ui

import (
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/exporters/bulk"
)

type modelTransient struct {
	openMenu *openMenuState

	openMenuStack []openMenuState

	move *pendingMove

	infoModal *infoModalState

	// soqlModal is a lightweight SOQL workspace over the current tab.
	// It owns an independent soqlSession so opening it from a related
	// row never tramples the user's top-level /soql query/results.
	soqlModal *soqlModalState

	editModal *editModalState

	choiceModal *choiceModalState

	// walkthrough is the guided first-launch tour. Zero value =
	// inactive. Unlike the modals above it's non-blocking: a small
	// corner panel that lets the user navigate while it watches model
	// state to confirm each task. Advancement remains manual. See
	// walkthrough.go.
	walkthrough walkthroughState

	commandPalette *commandPaletteState

	keybindingsModal *keybindingsModalState

	tagPicker  *tagPickerState
	tagEditor  *tagEditorState
	tagsCursor int // cursor on the /tags master list

	tagCur            int64
	tagItems          ListView[devproject.Item]
	tagKindChip       devproject.ItemKind
	tagKindChipCursor int

	homeBadgeFrame int

	homeBadgeTickRunning bool

	orgPicker *orgPickerState

	downloadsModal *downloadsModalState

	// homeDownloadsCursor is the row cursor on the /home Downloads
	// subtab. Indexes into the merged inflight+history slice the
	// renderer builds. Bespoke (rather than ListView-backed) because
	// the data lives on Model.exports, not orgData.
	homeDownloadsCursor int

	bundleCur string

	bundlePreviews map[string]bundlePreview

	deepCollect *deepCollectState

	globalSearch *globalSearchState

	themePicker *themePickerState

	// chipWizard is the unified multi-field + advanced-SOQL editor
	// used to author chips on every surface (records / objects /
	// flows). nil = hidden. See chip_wizard.go.
	chipWizard *chipWizardState

	picker *pickerState

	cacheSettings *cacheSettingsState

	// compareEdit is the /compare edit-saved-comparison modal. nil =
	// hidden. Opened via `e` on a saved comparison (row or loaded
	// inventory); owns the edit/clone state so it can't leak onto the
	// New subtab.
	compareEdit *compareEditModalState

	compareScope *compareScopeModalState

	compareTypesRefreshed map[string]bool

	// listTableWidthPrefs caches persisted per-org column width
	// overrides from cache.db. Map value is shared across Model value
	// copies; the loaded sentinel avoids hitting SQLite from render
	// hot paths after the first lookup.
	listTableWidthPrefs       map[string]*listTableWidthPrefs
	listTableWidthPrefsLoaded map[string]bool

	perViewSort    map[string]sortPref
	perViewSortKey string // the key the shared state currently reflects

	bulkExport *bulk.Flight

	exportSave *exportSaveState
}
