package ui

import (
	"hash/fnv"
	"strconv"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/diff"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/resource"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

type modelCompare struct {
	compareSubtabIdx int

	compareFrame       int
	compareTickRunning bool
}

const (
	compareSubtabNewIdx     = 0
	compareSubtabResultIdx  = 1
	compareSubtabSavedIdx   = 2
	compareSubtabHistoryIdx = 3
)

type comparePhase int

const (
	comparePhaseSetup      comparePhase = iota // choosing source/target/scope
	comparePhaseRetrieving                     // fetch in flight
	comparePhaseInventory                      // results shown (the workspace)
)

type compareRun struct {
	Source endpoint // source side
	Target endpoint // target side
	Scope  []string // metadata type labels included (provider TypeLabels)
	Method compareMethod

	OriginSavedID   string
	OriginSavedName string
	OpenedSavedAt   time.Time
	SaveAsNew       bool

	Phase comparePhase
	Err   error // retrieval/compare error, surfaced in the setup/inventory state

	Inv diff.Inventory // the matched rows (populated when Phase == Inventory)

	snapA diff.Snapshot
	snapB diff.Snapshot

	hashA diff.Snapshot
	hashB diff.Snapshot

	retainedBytes int64
	// bodyCap / retainCeiling are this run's thresholds (bytes), snapshotted
	// from settings at startCompare so a mid-run settings edit can't skew it.
	bodyCap       int
	retainCeiling int64

	Progress map[string]retrieveProgress
	expected int

	diffing bool

	RetrieveScroll int

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

func (r *compareRun) shouldRetain(size int) bool {
	if r.bodyCap > 0 && size > r.bodyCap {
		return false
	}
	if r.retainCeiling > 0 && r.retainedBytes+int64(size) > r.retainCeiling {
		return false
	}
	return true
}

func hashBody(s string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	// Prefix with length so two distinct-but-colliding bodies of different
	// length never compare equal (belt-and-suspenders on top of the hash).
	return strconv.Itoa(len(s)) + ":" + strconv.FormatUint(h.Sum64(), 16)
}

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

type endpointKind int

const (
	endpointOrg     endpointKind = iota // an authed org (Ref = username)
	endpointProject                     // a local dev project (Ref = project id) — FUTURE
)

type endpoint struct {
	Kind endpointKind
	Ref  string // org username (org) or project id (project)
}

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

type orgDataCompare struct {
	Run *compareRun

	InventoryList  resource.ListView[diff.Row]
	InventoryTable uilayout.ListTableState
	InventoryChip  string // active status-filter chip id

	SetupCursor int

	SavedList     resource.ListView[CompareDefRow]
	SavedTable    uilayout.ListTableState
	SavedLoaded   bool
	HistoryList   resource.ListView[CompareHistoryRow]
	HistoryTable  uilayout.ListTableState
	HistoryLoaded bool

	Diff *compareDiffView

	preview        *diff.Result
	previewKey     string // "Type|Key" the preview was built for
	previewScroll  int
	previewLoading bool // a dropped body is being fetched for the preview
}

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

type compareDiffView struct {
	Row     diff.Row    // the inventory row being viewed
	Result  diff.Result // computed line diff
	Lang    string      // highlight language
	Scroll  int         // top visible diff-line index
	Unified bool        // u toggles unified vs side-by-side
	Loading bool
	Err     error
}
