package resource

// SObjectChildren[Row, Detail] is the consolidated state for any
// entity kind that lives as a per-sobject child list on orgData:
// validation rules, record types, triggers, and (eventually) layouts,
// compact layouts, approval processes, etc.
//
// Every such entity has the same five pieces of state:
//
//	Lists[sobject]     — Resource of the child-list for one parent sobject.
//	Cursors[sobject]   — index into that list, per parent, preserved across
//	                      refreshes so cursor position survives.
//	Details[childID]   — Resource of the drilled child's full payload.
//	DrillID            — childID the user has currently drilled into.
//	fetchList / fetchDetail — typed closures used by Ensure helpers.
//
// Generic over Row (the list-row shape) and Detail (the drilled
// payload shape) so each entity kind gets its own concretely-typed
// value without re-implementing the scaffolding. Before this, orgData
// had four flat fields times three entity kinds = 12 fields; now it's
// one SObjectChildren field per kind.
//
// Field, which is carried inside sf.SObjectDescribe.Fields rather
// than as its own Resource, deliberately doesn't use this type —
// its lifecycle is owned by the describe. Every OTHER sobject-
// child entity in the roadmap should.

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
)

// SObjectChildren holds every piece of per-parent child-entity
// state: the list Resource per parent, a cursor per parent, a
// details Resource per drilled child, and which child is currently
// drilled in.
type SObjectChildren[Row any, Detail any] struct {
	// Lists maps parent sobject api-name → child-list Resource.
	Lists map[string]*Resource[[]Row]
	// Cursors is the j/k cursor index into Lists[sobject].Value(),
	// keyed by parent sobject api-name. Preserved across refreshes.
	Cursors map[string]int
	// Details maps child Id → full-payload Resource, used when the
	// user drills into a single child for its detail view.
	Details map[string]*Resource[Detail]
	// DrillID is the child Id currently drilled into (TabXxxDetail).
	// Empty when not on a detail tab.
	DrillID string

	// ResourceKeyPrefix is the cache-key prefix used for list
	// Resources (e.g. "validationrules:" or "triggers:"). The full
	// key is prefix+sobject. Routes incoming UpdatedMsg
	// back to the right Resource via applyResourceMsg.
	ResourceKeyPrefix string
	// DetailKeyPrefix is the cache-key prefix used for detail
	// Resources (e.g. "validationruledetail:" or "triggerdetail:").
	// The full key is prefix+childID.
	DetailKeyPrefix string

	// fetchList is the typed sf-package call that returns the full
	// child list for a parent sobject. Bound once at construction
	// so EnsureList doesn't need the caller to repeat it.
	fetchList func(alias, sobject string) ([]Row, error)
	// fetchDetail is the typed sf-package call that returns the
	// full payload for one child Id.
	fetchDetail func(alias, id string) (Detail, error)
	// listTTL / detailTTL tune the per-Resource TTL. Zero = default
	// of 1 minute (the current value across all the concrete
	// per-entity Ensure helpers).
	listTTL      time.Duration
	detailTTL    time.Duration
	listNoDisk   bool
	detailNoDisk bool
}

// NewSObjectChildren binds the fetch closures + key prefixes and
// zero-inits the maps. Call once per entity kind from newOrgData.
func NewSObjectChildren[Row any, Detail any](
	listKeyPrefix, detailKeyPrefix string,
	fetchList func(alias, sobject string) ([]Row, error),
	fetchDetail func(alias, id string) (Detail, error),
) SObjectChildren[Row, Detail] {
	return SObjectChildren[Row, Detail]{
		Lists:             map[string]*Resource[[]Row]{},
		Cursors:           map[string]int{},
		Details:           map[string]*Resource[Detail]{},
		ResourceKeyPrefix: listKeyPrefix,
		DetailKeyPrefix:   detailKeyPrefix,
		fetchList:         fetchList,
		fetchDetail:       fetchDetail,
		listTTL:           time.Minute,
		detailTTL:         time.Minute,
		listNoDisk:        true,
		detailNoDisk:      true,
	}
}

// EnsureList lazily wires a list Resource for parent sobject. Safe
// to call repeatedly; the first call allocates, later calls return
// the existing Resource. Scope must be the active org username.
func (c *SObjectChildren[Row, Detail]) EnsureList(scope, alias, sobject string) *Resource[[]Row] {
	if r, ok := c.Lists[sobject]; ok {
		return r
	}
	fetch := c.fetchList
	r := &Resource[[]Row]{
		Scope:   scope,
		Key:     c.ResourceKeyPrefix + sobject,
		TTL:     c.listTTL,
		NoCache: c.listNoDisk,
		Fetch: func() ([]Row, error) {
			return fetch(alias, sobject)
		},
	}
	c.Lists[sobject] = r
	return r
}

// EnsureDetail lazily wires a detail Resource for one child Id.
func (c *SObjectChildren[Row, Detail]) EnsureDetail(scope, alias, id string) *Resource[Detail] {
	if r, ok := c.Details[id]; ok {
		return r
	}
	fetch := c.fetchDetail
	r := &Resource[Detail]{
		Scope:   scope,
		Key:     c.DetailKeyPrefix + id,
		TTL:     c.detailTTL,
		NoCache: c.detailNoDisk,
		Fetch: func() (Detail, error) {
			return fetch(alias, id)
		},
	}
	c.Details[id] = r
	return r
}

// ApplyResourceMsg routes an incoming UpdatedMsg to the
// right Resource based on the msg.Key prefix. Returns true if the
// msg belonged to this children set (caller can stop routing).
func (c *SObjectChildren[Row, Detail]) ApplyResourceMsg(msg UpdatedMsg) bool {
	if c.ResourceKeyPrefix != "" && hasPrefix(msg.Key, c.ResourceKeyPrefix) {
		sobject := msg.Key[len(c.ResourceKeyPrefix):]
		if r, ok := c.Lists[sobject]; ok {
			r.Apply(msg)
		}
		return true
	}
	if c.DetailKeyPrefix != "" && hasPrefix(msg.Key, c.DetailKeyPrefix) {
		id := msg.Key[len(c.DetailKeyPrefix):]
		if r, ok := c.Details[id]; ok {
			r.Apply(msg)
		}
		return true
	}
	return false
}

// ApplyAndMaybeRefresh routes the msg like ApplyResourceMsg but also
// returns a refresh cmd when the msg was a (possibly empty) cache load
// that left the resource stale. Without this follow-up, drill-in
// resources whose first encounter is a cache miss would sit on
// "loading…" forever — only the cache-load fires from Ensure now,
// and stale-load needs its own kick.
func (c *SObjectChildren[Row, Detail]) ApplyAndMaybeRefresh(msg UpdatedMsg, cache *cache.Cache) (handled bool, refresh tea.Cmd) {
	if c.ResourceKeyPrefix != "" && hasPrefix(msg.Key, c.ResourceKeyPrefix) {
		sobject := msg.Key[len(c.ResourceKeyPrefix):]
		if r, ok := c.Lists[sobject]; ok {
			r.Apply(msg)
			if msg.FromCache {
				return true, r.MaybeRefreshAfterCacheLoad(cache)
			}
		}
		return true, nil
	}
	if c.DetailKeyPrefix != "" && hasPrefix(msg.Key, c.DetailKeyPrefix) {
		id := msg.Key[len(c.DetailKeyPrefix):]
		if r, ok := c.Details[id]; ok {
			r.Apply(msg)
			if msg.FromCache {
				return true, r.MaybeRefreshAfterCacheLoad(cache)
			}
		}
		return true, nil
	}
	return false, nil
}

// ListFor returns the (possibly nil) list Resource for a parent
// sobject. Sibling of the direct map access used throughout the
// codebase today — reads stay symmetrical with EnsureList writes.
func (c *SObjectChildren[Row, Detail]) ListFor(sobject string) (*Resource[[]Row], bool) {
	r, ok := c.Lists[sobject]
	return r, ok
}

// DetailFor returns the detail Resource for a drilled child Id.
func (c *SObjectChildren[Row, Detail]) DetailFor(id string) (*Resource[Detail], bool) {
	r, ok := c.Details[id]
	return r, ok
}

// RefreshSObject returns a tea.Cmd that refreshes both the list and
// the drilled detail (if one is loaded) — the common OnSuccess shape
// for every child-entity action. Nil when there's nothing to refresh.
func (c *SObjectChildren[Row, Detail]) RefreshSObject(sobject string, cache *cache.Cache) tea.Cmd {
	var cmds []tea.Cmd
	if r, ok := c.Lists[sobject]; ok {
		cmds = append(cmds, r.Refresh(cache))
	}
	if c.DrillID != "" {
		if det, ok := c.Details[c.DrillID]; ok {
			cmds = append(cmds, det.Refresh(cache))
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

// ClearDrill resets DrillID and evicts the detail cache for the
// given id — used after a delete so subsequent drills refetch.
func (c *SObjectChildren[Row, Detail]) ClearDrill(id string) {
	c.DrillID = ""
	if id != "" {
		delete(c.Details, id)
	}
}

// hasPrefix avoids a strings import here; equivalent to
// strings.HasPrefix but inlined for a tiny internal helper.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
