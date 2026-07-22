package resource

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/cache"
)

// Resource is a cache-first piece of data: a typed payload plus the
// freshness + error + busy state every view needs. Each resource has a
// scope (org username or "global") and a short key (e.g. "sobjects",
// "home"). Together they identify the cache row and route update msgs.
//
// Lifecycle, driven by Ensure():
//  1. If we've never loaded from cache, emit a load command.
//  2. If the data is stale (older than TTL) or missing, emit a fetch
//     command. fetch() shells out to sf and writes back to the cache.
//  3. Both commands produce a UpdatedMsg; Apply() folds it into
//     the Resource state.
//
// The zero value is a valid "never-loaded" resource.
type Resource[T any] struct {
	Scope string
	Key   string
	TTL   time.Duration

	// NoCache disables the SQLite round-trip entirely. The resource
	// only holds data in-memory for the current session. Use for data
	// where persisting to disk is inappropriate (records — stale data
	// misleads, PII persistence is a privacy concern) or where
	// persisting is worthless (session-ephemeral).
	NoCache bool

	// Fetch is called by refresh commands to talk to sf. It returns the
	// fresh value. The caller (Resource.refreshCmd) takes care of writing
	// to cache afterwards.
	Fetch func() (T, error)

	// FetchWithExisting is like Fetch, but receives a snapshot of the
	// current in-memory value captured before the command goroutine
	// starts. Use it for delta fetches that need to merge against the
	// existing rows without reading Resource state off the update loop.
	FetchWithExisting func(existing T) (T, error)

	data      T
	fetchedAt time.Time
	err       error
	busy      bool
}

// Get returns the current value and a bool indicating whether it has
// ever been populated (either from cache or fetch).
func (r *Resource[T]) Get() (T, bool) {
	return r.data, !r.fetchedAt.IsZero()
}

// Value returns the current value without the "loaded" flag. Useful for
// renderers that treat zero values as "nothing to show".
func (r *Resource[T]) Value() T { return r.data }

// FetchedAt is when this resource's data last landed, either from cache
// on startup or from a fresh fetch.
func (r *Resource[T]) FetchedAt() time.Time { return r.fetchedAt }

// Loaded reports whether this resource has ever populated (from cache
// or fetch). Used by the global-refresh path to refresh only resources
// the user has actually touched, leaving cold ones to lazy-load.
func (r *Resource[T]) Loaded() bool { return !r.fetchedAt.IsZero() }

// Refreshable is the type-erased view of a Resource the global-refresh
// path iterates: "is it loaded, and give me a command to re-fetch it."
// *Resource[T] satisfies this for every T, so a heterogeneous set of
// resources can be refreshed uniformly.
type Refreshable interface {
	Loaded() bool
	Refresh(c *cache.Cache) tea.Cmd
}

// Set populates the resource directly with a value + fetched-at stamp.
// Bypasses the Fetch / cache pipeline — intended for ad-hoc fetches
// where the caller has already done the IO and just wants the
// in-memory state to reflect it. The cache is *not* written.
func (r *Resource[T]) Set(v T) {
	r.data = v
	r.fetchedAt = time.Now()
	r.err = nil
	r.busy = false
}

// Busy is true while a background fetch is in flight.
func (r *Resource[T]) Busy() bool { return r.busy }

// Err is the last fetch error, or nil.
func (r *Resource[T]) Err() error { return r.err }

// BenignFetchErr, when set, reports whether a fetch error should be
// treated as a non-error (data preserved, no error banner). Injected by
// the app layer to recognise the per-org demo sentinel (sf.ErrDemoTarget)
// without this package importing sf. Nil = every fetch error is real.
var BenignFetchErr func(error) bool

func benign(err error) bool {
	return err != nil && BenignFetchErr != nil && BenignFetchErr(err)
}

// Stale reports whether the TTL has expired (or data has never arrived).
// DemoMode freezes the data layer for `sf-deck --demo`: once a value
// is loaded (from the seeded demo cache) it never goes stale, NoCache
// resources become cacheable so they too serve from the seed, and
// Refresh re-serves rather than fetching. The network is never the
// source of truth in demo mode — the seed is.
var DemoMode bool

func (r *Resource[T]) Stale() bool {
	if DemoMode && !r.fetchedAt.IsZero() {
		return false
	}
	if r.TTL <= 0 {
		return r.fetchedAt.IsZero()
	}
	return r.fetchedAt.IsZero() || time.Since(r.fetchedAt) > r.TTL
}

// noCacheEffective is NoCache with the demo override applied.
func (r *Resource[T]) noCacheEffective() bool {
	if DemoMode {
		return false
	}
	return r.NoCache
}

// UpdatedMsg is emitted by Ensure-produced commands. It's
// intentionally untyped at the payload level so Bubble Tea's single
// Update switch can dispatch by (scope,key) without introducing a new
// msg type per resource.
type UpdatedMsg struct {
	Scope     string
	Key       string
	Payload   any // a *T of some Resource; type-asserted on receive
	Err       error
	FromCache bool
	CachedAt  time.Time
}

// Ensure returns the commands needed to bring this resource up-to-date.
// It's safe to call on every tick / whenever the user navigates — the
// commands are no-ops when the resource is already fresh-and-loaded or
// a fetch is in flight.
//
// The supplied cache is used for both load and save.
//
// On a first-Ensure for a cacheable resource, only the cache-load fires.
// Apply()'s FromCache branch then evaluates whether the cached data is
// fresh enough; if it's stale (TTL expired) Apply itself kicks the
// refresh. That way a freshly-cached payload short-circuits the
// network call instead of racing it.
func (r *Resource[T]) Ensure(c *cache.Cache) tea.Cmd {
	// NoCache: nothing to load, just fetch when stale.
	if r.noCacheEffective() {
		if r.Stale() && !r.busy && r.canFetch() {
			r.busy = true
			return r.refreshCmd(c)
		}
		return nil
	}
	// Cacheable + cold: read cache first; the load result decides
	// whether to refresh.
	if r.fetchedAt.IsZero() && !r.busy {
		r.busy = true
		return r.loadCmd(c)
	}
	// Already loaded: refresh only if stale.
	if r.Stale() && !r.busy && r.canFetch() {
		r.busy = true
		return r.refreshCmd(c)
	}
	return nil
}

// Refresh forces a fetch regardless of staleness. No-op if already busy
// or if no Fetch is configured.
func (r *Resource[T]) Refresh(c *cache.Cache) tea.Cmd {
	if r.busy || !r.canFetch() {
		return nil
	}
	if DemoMode {
		// The seed is the source of truth — a forced refresh
		// re-serves it (or re-loads from the demo cache when cold)
		// instead of touching the network.
		if r.fetchedAt.IsZero() {
			r.busy = true
			return r.loadCmd(c)
		}
		return nil
	}
	r.busy = true
	return r.refreshCmd(c)
}

// Apply folds a UpdatedMsg into the resource. Returns true if
// this resource owns the message (so Update can stop routing).
func (r *Resource[T]) Apply(msg UpdatedMsg) bool {
	if msg.Scope != r.Scope || msg.Key != r.Key {
		return false
	}
	if msg.FromCache {
		r.busy = false
		// Cache load: populate if we haven't gotten a fresh payload
		// in the meantime (fetch may have raced cache-load and won).
		if r.fetchedAt.IsZero() && msg.Err == nil && msg.Payload != nil {
			if p, ok := msg.Payload.(*T); ok && p != nil {
				r.data = *p
				r.fetchedAt = msg.CachedAt
			}
		}
		return true
	}
	// Fresh fetch:
	r.busy = false
	if benign(msg.Err) {
		// A demo org's fetch short-circuited (no live backend). Keep the
		// seeded data already loaded from cache; don't record it as an
		// error, and mark loaded so the resource stops trying to refetch.
		r.err = nil
		if r.fetchedAt.IsZero() {
			r.fetchedAt = time.Now()
		}
		return true
	}
	r.err = msg.Err
	if msg.Err == nil && msg.Payload != nil {
		if p, ok := msg.Payload.(*T); ok && p != nil {
			r.data = *p
			r.fetchedAt = time.Now()
		}
	}
	return true
}

// MaybeRefreshAfterCacheLoad is called by Update after a cache-load
// message is applied. If the cached payload was stale (or missing),
// fire the network refresh now. Returning the cmd lets Update batch
// it into the same Update tick rather than waiting for another
// trigger.
func (r *Resource[T]) MaybeRefreshAfterCacheLoad(c *cache.Cache) tea.Cmd {
	if r.noCacheEffective() {
		return nil
	}
	if r.busy || !r.canFetch() {
		return nil
	}
	if !r.Stale() {
		return nil
	}
	r.busy = true
	return r.refreshCmd(c)
}

func (r *Resource[T]) canFetch() bool {
	return r.Fetch != nil || r.FetchWithExisting != nil
}

func (r *Resource[T]) loadCmd(c *cache.Cache) tea.Cmd {
	scope, key := r.Scope, r.Key
	return func() tea.Msg {
		var v T
		at, ok, err := c.GetJSON(scope, key, &v)
		if err != nil || !ok {
			return UpdatedMsg{Scope: scope, Key: key, FromCache: true}
		}
		return UpdatedMsg{
			Scope:     scope,
			Key:       key,
			Payload:   &v,
			FromCache: true,
			CachedAt:  at,
		}
	}
}

func (r *Resource[T]) refreshCmd(c *cache.Cache) tea.Cmd {
	scope, key, fetch, fetchWithExisting, noCache := r.Scope, r.Key, r.Fetch, r.FetchWithExisting, r.NoCache
	existing := r.data
	return func() tea.Msg {
		var (
			v   T
			err error
		)
		if fetchWithExisting != nil {
			v, err = fetchWithExisting(existing)
		} else {
			v, err = fetch()
		}
		// Persist only when caching is enabled. NoCache resources keep
		// the fresh payload in-memory (via Apply) but never hit disk.
		if err == nil && !noCache && c != nil {
			if cacheErr := c.PutJSON(scope, key, v); cacheErr != nil {
				applog.Warn("resource.cache_write_failed", map[string]any{
					"scope": scope,
					"key":   key,
					"err":   cacheErr.Error(),
				})
			}
		}
		return UpdatedMsg{
			Scope:   scope,
			Key:     key,
			Payload: &v,
			Err:     err,
		}
	}
}
