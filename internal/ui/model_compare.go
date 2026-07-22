package ui

// Compare feature — in-memory state.
//
// The /compare tab mirrors /soql's New/Saved/History subtab shape. This
// file holds the per-org compare state (the active comparison run + its
// inventory ListView) and the Model-level subtab index. Saved
// comparison DEFINITIONS persist in settings.toml (see settings
// CompareDefs); RUN results live only in memory for the session
// (retrieved metadata is large + staleness misleads, same rationale as
// the records NoCache resources).

import (
	"hash/fnv"
	"strconv"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/diff"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/resource"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// modelCompare holds Model-level (not per-org) compare state — just the
// subtab index. The lists + run state live per-org on orgDataCompare.
type modelCompare struct {
	compareSubtabIdx int

	// compareFrame drives the retrieving-screen spinner + animated bar
	// (advanced by compareTickMsg). compareTickRunning guards against
	// stacking multiple self-rescheduling ticks.
	compareFrame       int
	compareTickRunning bool
}

// compareSubtab indices — order of the New/Result/Saved/History strip.
const (
	compareSubtabNewIdx     = 0
	compareSubtabResultIdx  = 1
	compareSubtabSavedIdx   = 2
	compareSubtabHistoryIdx = 3
)

// comparePhase tracks where the New subtab is in its lifecycle.
type comparePhase int

const (
	comparePhaseSetup      comparePhase = iota // choosing source/target/scope
	comparePhaseRetrieving                     // fetch in flight
	comparePhaseInventory                      // results shown (the workspace)
)

// compareRun is one active comparison: its definition + results.
type compareRun struct {
	Source endpoint // source side
	Target endpoint // target side
	Scope  []string // metadata type labels included (provider TypeLabels)
	Method compareMethod

	// OriginSavedID is the store id of the saved comparison this run was
	// opened/reran from (empty for a fresh ad-hoc run). When set, the
	// setup screen shows an "Editing" row and SaveMode controls whether
	// a save overwrites the original or forks to a new one.
	OriginSavedID   string
	OriginSavedName string
	// OpenedSavedAt is when the saved comparison was last run (its store
	// UpdatedAt), set only when this run was OPENED from a saved result.
	// Drives the "ran N ago — re-run?" staleness banner. Zero for live runs.
	OpenedSavedAt time.Time
	// SaveAsNew, when true, detaches from the origin on save (clone →
	// new) rather than overwriting. Toggled via the setup "Editing" row.
	SaveAsNew bool

	Phase comparePhase
	Err   error // retrieval/compare error, surfaced in the setup/inventory state

	Inv diff.Inventory // the matched rows (populated when Phase == Inventory)

	// snapA / snapB hold the RETAINED bodies for drill-in (Auto / Metadata
	// API routes). A body is retained only if it's under the per-component
	// cap AND total retained is under the ceiling (see retainBody) — so a
	// huge all-types compare doesn't hold ~800MB of XML. Bodies NOT
	// retained are absent here and re-fetched live on drill-in.
	snapA diff.Snapshot
	snapB diff.Snapshot

	// hashA / hashB hold a content hash for EVERY component (type → name →
	// hash) regardless of retention. The inventory diff (Same/Different)
	// runs off these, so it's exact + complete even when bodies were
	// dropped. Cheap: ~64 bytes/component vs the body's KB–MB.
	hashA diff.Snapshot
	hashB diff.Snapshot

	// retainedBytes tracks total retained body bytes for the ceiling check.
	retainedBytes int64
	// bodyCap / retainCeiling are this run's thresholds (bytes), snapshotted
	// from settings at startCompare so a mid-run settings edit can't skew it.
	bodyCap       int
	retainCeiling int64

	// Progress tracks per-(side,type) retrieve status during the
	// retrieving phase, driving the live progress screen. Keyed by
	// progressKey(side, type). started counts total expected units;
	// done counts finished (success or fail) so the screen knows when
	// the whole comparison is complete.
	Progress map[string]retrieveProgress
	expected int

	// diffing guards the off-thread CompareSnapshots launch so the
	// finishing diff fires exactly once (every applier calls
	// maybeFinishCompare; only the unit that completes the set should
	// kick the diff). Reset on a fresh run.
	diffing bool

	// RetrieveScroll is the top visible type-row index on the retrieving
	// screen (the per-type list can be ~600 rows — far taller than the
	// viewport — so it scrolls; ↑/↓/j/k/pgup/pgdn drive it).
	RetrieveScroll int

	// sem bounds actual compare API calls. All retrieve cmds launch as
	// goroutines (cheap), but listMetadata/readMetadata/bulk Apex calls share
	// this semaphore, so at most cap(sem) requests are in flight. Sized from
	// Settings.CompareConcurrency(). nil ⇒ unbounded (Tooling path / tests).
	sem chan struct{}
}

// acquire/release gate one retrieve on the run's concurrency semaphore.
// nil sem (unbounded) is a no-op, so callers need no special-casing.
func (r *compareRun) acquire() {
	if r != nil && r.sem != nil {
		r.sem <- struct{}{}
	}
}

func (r *compareRun) release() {
	if r != nil && r.sem != nil {
		<-r.sem
	}
}

// hashSnap returns the hash sidecar for a side (mirrors snapshotFor).
func (r *compareRun) hashSnap(side string) diff.Snapshot {
	if side == "target" {
		return r.hashB
	}
	return r.hashA
}

// recordComponents folds one (side,type) retrieve's components into the
// run: it ALWAYS records a content hash for every component (so the
// inventory diff is exact + complete), and RETAINS the full body in the
// snapshot only while within the memory budget — under the per-component
// cap and under the total ceiling. Bodies not retained are re-fetched
// live on drill-in. Called serially on the UI goroutine (no locking).
func (r *compareRun) recordComponents(side, typeLabel string, components map[string]string) {
	hashes := r.hashSnap(side)
	bodies := r.snapshotFor(side)
	if hashes[typeLabel] == nil {
		hashes[typeLabel] = map[string]string{}
	}
	if bodies[typeLabel] == nil {
		bodies[typeLabel] = map[string]string{}
	}
	for name, body := range components {
		// Hash the NORMALIZED body so CompareSnapshots' hash-equality
		// verdict collapses cosmetic-only XML differences (whitespace /
		// reflow / trailing newline) instead of flagging them Different —
		// CompareSnapshots can't re-normalize a hash, so it must happen
		// here. Retain the RAW body for drill-in (BodyDiffFromSnapshots
		// pretty-prints it itself).
		hashes[typeLabel][name] = hashBody(diff.NormalizeBody(body))
		if r.shouldRetain(len(body)) {
			bodies[typeLabel][name] = body
			r.retainedBytes += int64(len(body))
		}
	}
}

// shouldRetain reports whether a body of this size is kept in memory:
// under the per-component cap AND total retained under the ceiling.
func (r *compareRun) shouldRetain(size int) bool {
	if r.bodyCap > 0 && size > r.bodyCap {
		return false
	}
	if r.retainCeiling > 0 && r.retainedBytes+int64(size) > r.retainCeiling {
		return false
	}
	return true
}

// hashBody returns a short content hash for equality comparison. FNV-64a
// is plenty: we only need collision resistance for "did this component's
// XML change", not cryptographic strength, and it's fast over MBs of XML.
func hashBody(s string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	// Prefix with length so two distinct-but-colliding bodies of different
	// length never compare equal (belt-and-suspenders on top of the hash).
	return strconv.Itoa(len(s)) + ":" + strconv.FormatUint(h.Sum64(), 16)
}

// retrieveProgress is one (side, type) retrieve's live status.
type retrieveProgress struct {
	Side  string // "source" / "target"
	Type  string
	State retrieveState
	Count int    // components retrieved (when done)
	Note  string // e.g. "chunked" / error short text
}

type retrieveState int

const (
	retrievePending retrieveState = iota
	retrieveRunning
	retrieveDone
	retrieveFailed
)

func progressKey(side, typ string) string { return side + "|" + typ }

// endpointKind is what a comparison side points at. Today only org;
// project is reserved so "compare against a dev project" drops in later
// without refactoring the run/setup/persistence shape.
type endpointKind int

const (
	endpointOrg     endpointKind = iota // an authed org (Ref = username)
	endpointProject                     // a local dev project (Ref = project id) — FUTURE
)

// endpoint is one side of a comparison. Modeled as a typed reference
// (not a bare org string) so future source kinds (dev project, git
// branch) are additive. Ref's meaning depends on Kind.
type endpoint struct {
	Kind endpointKind
	Ref  string // org username (org) or project id (project)
}

// orgEndpoint builds an org-kind endpoint.
func orgEndpoint(username string) endpoint { return endpoint{Kind: endpointOrg, Ref: username} }

// IsZero reports an unset endpoint.
func (e endpoint) IsZero() bool { return e.Ref == "" }

// OrgRef returns the org username when this endpoint is an org, else "".
// Most v1 code paths assume org endpoints; this is the bridge.
func (e endpoint) OrgRef() string {
	if e.Kind == endpointOrg {
		return e.Ref
	}
	return ""
}

// Equal compares two endpoints by kind+ref.
func (e endpoint) Equal(o endpoint) bool { return e.Kind == o.Kind && e.Ref == o.Ref }

// compareMethod selects the retrieval route. Auto prefers fast Tooling
// per type and falls back to the Metadata API for Tooling-blind types.
type compareMethod int

const (
	compareMethodAuto        compareMethod = iota // Tooling-first hybrid (default)
	compareMethodTooling                          // force Tooling (fast, fewer types, more calls)
	compareMethodMetadataAPI                      // force Metadata API (slow, all types, fewest calls)
)

func (cm compareMethod) String() string {
	switch cm {
	case compareMethodTooling:
		return "Tooling"
	case compareMethodMetadataAPI:
		return "Metadata API"
	default:
		return "Auto"
	}
}

// orgDataCompare is the per-org compare state, embedded into orgData.
type orgDataCompare struct {
	// Run is the active comparison for this org context (nil until the
	// user runs one). Source defaults to the active org.
	Run *compareRun

	// InventoryList is the shared-engine ListView over Run.Inv.Rows so
	// the inventory gets cursor / sort / search / chips for free.
	InventoryList  resource.ListView[diff.Row]
	InventoryTable uilayout.ListTableState
	InventoryChip  string // active status-filter chip id

	// Setup cursor: which of source/target/scope/compare the user is on.
	SetupCursor int

	// Saved + History list state (definitions persist in settings; these
	// are the per-org ListView snapshots, mirroring SOQL Saved/History).
	SavedList     resource.ListView[CompareDefRow]
	SavedTable    uilayout.ListTableState
	SavedLoaded   bool
	HistoryList   resource.ListView[CompareHistoryRow]
	HistoryTable  uilayout.ListTableState
	HistoryLoaded bool

	// Drill-in state (Screen 4 side-by-side body diff).
	Diff *compareDiffView

	// preview is the live mini-diff shown in the side panel for the
	// SELECTED inventory row (computed lazily, focused on the first
	// difference). previewKey guards staleness — when the cursor moves to
	// a different row, the cached preview is rebuilt. previewScroll is the
	// top visible diff-line index within the panel (n/N jump by hunk).
	preview        *diff.Result
	previewKey     string // "Type|Key" the preview was built for
	previewScroll  int
	previewLoading bool // a dropped body is being fetched for the preview
}

// savedRowKind distinguishes the two things the Saved subtab lists: a
// full saved comparison (data-ful, in devprojects.db) vs a template
// (data-less recipe, in settings.toml).
type savedRowKind int

const (
	savedRowComparison savedRowKind = iota // has stored result data
	savedRowTemplate                       // recipe only
)

// CompareDefRow is one row in the Saved subtab — either a saved
// comparison (with data) or a template (recipe only), discriminated by
// Kind. ID is the store id for comparisons; for templates the Name is
// the settings key.
type CompareDefRow struct {
	Kind   savedRowKind
	ID     string // saved-comparison store id (empty for templates)
	Name   string
	Source string
	Target string
	Scope  string // human-joined scope ("ApexClass, ApexTrigger")
	Saved  string // "saved <age>" for comparisons; "" for templates
}

// KindLabel renders the row-kind tag for the list.
func (r CompareDefRow) KindLabel() string {
	if r.Kind == savedRowComparison {
		return "comparison"
	}
	return "template"
}

// CompareHistoryRow is one past run.
type CompareHistoryRow struct {
	Name      string // def name, or "(ad-hoc)"
	Source    string
	Target    string
	RanAt     string // humanized
	Different int
	AOnly     int
	BOnly     int
}

// compareDiffView is the side-by-side body diff drill-in state.
type compareDiffView struct {
	Row     diff.Row    // the inventory row being viewed
	Result  diff.Result // computed line diff
	Lang    string      // highlight language
	Scroll  int         // top visible diff-line index
	Unified bool        // u toggles unified vs side-by-side
	// Loading is set while a dropped (over-budget) body is being
	// re-fetched live; the diff fills in via compareBodyFetchedMsg.
	Loading bool
	Err     error
}
