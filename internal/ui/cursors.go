package ui

// CursorStore consolidates every "remember the cursor position for
// this list/grid" map that used to live as separate fields on
// orgData (FLSCursors, ObjectPermsCursor, AssignedUsersCursor,
// SystemPermsCursor, RecordsRowCur, FlowVersionCur). One place owns
// key construction + bounds clamping so call sites can't build
// subtly-different keys, can't forget the bounds check, and can't
// cause cursor drift when the underlying data shrinks.
//
// All values are plain ints — except ListViewCur, which holds a
// string chip ID (not a row index) and stays separate.
//
// Adding a new cursor kind: pick a Kind constant and add a typed
// accessor. Don't add another map field on orgData.

import (
	"strings"
)

// CursorKind disambiguates cursor namespaces inside the single
// underlying map. Stable strings — they're part of the cache-friendly
// composite keys but not persisted to disk, so renaming is free.
type CursorKind string

const (
	cursorKindFLS           CursorKind = "fls"
	cursorKindObjectPerms   CursorKind = "objperms"
	cursorKindSystemPerms   CursorKind = "sysperms"
	cursorKindAssignedUsers CursorKind = "assigned"
	cursorKindRecordsRow    CursorKind = "records"
	// cursorKindRecordsAnchor stashes the pre-search cursor position so
	// clearing the filter restores the user to where they were before
	// they typed `/`. Captured on SearchStart for records views;
	// consumed by clearCommittedSearch.
	cursorKindRecordsAnchor  CursorKind = "recordsanchor"
	cursorKindFlowVersion    CursorKind = "flowver"
	cursorKindReportRow      CursorKind = "reportrow"
	cursorKindDevProjectItem CursorKind = "dpitem"
)

// CursorStore owns row-index cursors. Construct via NewCursorStore;
// zero-value is safe (Get returns 0 on a missing key).
type CursorStore struct {
	idx map[string]int
}

// NewCursorStore returns an empty store with the underlying map
// pre-allocated.
func NewCursorStore() CursorStore {
	return CursorStore{idx: map[string]int{}}
}

// joinKey builds the composite map key. Kept private so call-site
// typos can't silently spread mismatched keys.
func joinKey(kind CursorKind, parts ...string) string {
	var b strings.Builder
	b.WriteString(string(kind))
	for _, p := range parts {
		b.WriteByte(':')
		b.WriteString(p)
	}
	return b.String()
}

// Get returns the stored cursor clamped to [0, n). When the stored
// index is out of range OR n == 0 the result is 0. This replaces the
// `if cur < 0 || cur >= len(rows) { cur = 0 }` boilerplate that used
// to live at every read site.
func (s CursorStore) Get(kind CursorKind, n int, parts ...string) int {
	if n <= 0 || s.idx == nil {
		return 0
	}
	cur := s.idx[joinKey(kind, parts...)]
	if cur < 0 || cur >= n {
		return 0
	}
	return cur
}

// Peek returns the stored cursor unmodified. Used when bounds aren't
// known at the call site (e.g. opening menus where the cursor was
// set elsewhere); the caller is responsible for any further bounds
// checking. Most readers should use Get.
func (s CursorStore) Peek(kind CursorKind, parts ...string) int {
	if s.idx == nil {
		return 0
	}
	return s.idx[joinKey(kind, parts...)]
}

// Set stores a cursor index, clamped to [0, n) before persistence.
// Pass n = 0 to skip clamping (used when bounds aren't known yet —
// rare; prefer Move instead).
func (s CursorStore) Set(kind CursorKind, idx, n int, parts ...string) {
	if s.idx == nil {
		return
	}
	if n > 0 {
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
	}
	s.idx[joinKey(kind, parts...)] = idx
}

// Move adjusts the stored cursor by delta and returns the new value,
// clamped to [0, n). Saves callers from the load → +delta → clamp →
// store sequence that appears at every cursor-move call site.
func (s CursorStore) Move(kind CursorKind, delta, n int, parts ...string) int {
	cur := s.Get(kind, n, parts...)
	cur += delta
	if cur < 0 {
		cur = 0
	}
	if cur >= n {
		cur = n - 1
	}
	if n <= 0 {
		cur = 0
	}
	s.Set(kind, cur, n, parts...)
	return cur
}

// Reset zeros the cursor for one key (used by SearchClear and the
// like — the user typed a new query, don't keep them on row 47 of
// the previous result).
func (s CursorStore) Reset(kind CursorKind, parts ...string) {
	if s.idx == nil {
		return
	}
	s.idx[joinKey(kind, parts...)] = 0
}

// Clear drops every entry under a kind. Useful when invalidating a
// whole namespace (e.g. permset switch wipes all the per-permset
// system-perms cursors).
func (s CursorStore) Clear(kind CursorKind) {
	if s.idx == nil {
		return
	}
	prefix := string(kind) + ":"
	for k := range s.idx {
		if strings.HasPrefix(k, prefix) {
			delete(s.idx, k)
		}
	}
}
