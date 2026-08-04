package diff

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Status is the per-component comparison outcome on the inventory list.
type Status int

const (
	StatusSame      Status = iota // present on both, bodies identical
	StatusDifferent               // present on both, bodies differ
	StatusAOnly                   // present in source only
	StatusBOnly                   // present in target only
)

func (s Status) String() string {
	switch s {
	case StatusSame:
		return "SAME"
	case StatusDifferent:
		return "DIFFERENT"
	case StatusAOnly:
		return "A ONLY"
	case StatusBOnly:
		return "B ONLY"
	}
	return "?"
}

// Component is one metadata item on one side, as enumerated by a
// Provider. Key is the stable cross-org identity (e.g. the class name)
// — two Components with the same (Type, Key) are "the same thing" in
// each org. Body is fetched lazily; Summary is the cheap one-liner the
// inventory column shows (e.g. "v61", "Active").
type Component struct {
	Type    string // metadata type label, e.g. "ApexClass"
	Key     string // cross-org identity (name)
	ID      string // org-local record id (for lazy body fetch)
	Summary string // cheap per-side cell, e.g. "v61 · Active"
}

// Row is one line of the comparison inventory: a component matched (or
// not) across the two orgs.
type Row struct {
	Type     string
	Key      string
	Status   Status
	ASummary string
	BSummary string
	AID      string // source record id (for lazy body fetch); "" if A-only absent
	BID      string // target record id; "" if B-only absent
}

// Provider knows how to enumerate and fetch bodies for ONE metadata
// type against an org alias. Adding a new comparable metadata type =
// implementing one Provider; the compare engine is type-agnostic.
//
// List is the cheap inventory pass (no bodies). Body is the lazy
// per-component fetch used on drill-in. Both take an org alias and are
// project-free (Tooling/REST by alias) for v1 types.
type Provider interface {
	// TypeLabel is the metadata type name shown in the TYPE column.
	TypeLabel() string
	// List enumerates every component of this type in the org.
	List(alias string) ([]Component, error)
	// Body fetches the source/text of one component for diffing.
	Body(alias, id string) (string, error)
}

// Inventory is the result of the cheap list-only compare pass: the
// matched rows plus any per-type errors (one side failed to list).
type Inventory struct {
	Rows   []Row
	Errors []TypeError
}

// TypeError records that listing one type on one side failed, so the
// UI can surface it instead of silently showing a type as empty.
type TypeError struct {
	Type string
	Side string // "source" or "target"
	Err  error
}

// Summary counts rows by status — drives the inventory footer.
func (inv Inventory) Summary() (same, different, aOnly, bOnly int) {
	for _, r := range inv.Rows {
		switch r.Status {
		case StatusSame:
			same++
		case StatusDifferent:
			different++
		case StatusAOnly:
			aOnly++
		case StatusBOnly:
			bOnly++
		}
	}
	return
}

// CompareInventory runs the cheap list-only pass: for each provider,
// list both orgs, match by Key, and classify. Bodies are NOT fetched
// here — Status is provisional (Same vs Different is resolved lazily on
// drill-in for matched pairs, OR eagerly by the caller if it wants).
// For matched pairs we mark StatusDifferent only when a cheap signal
// (Summary mismatch) suggests divergence; identical Summaries are left
// as a candidate the body-diff confirms. To keep v1 simple and honest,
// matched pairs are reported as StatusDifferent="needs body check" via
// the Summary heuristic; callers can upgrade to a full body pass.
//
// alias args: a = source, b = target. Providers run concurrently per
// type and per side.
func CompareInventory(a, b string, providers []Provider) Inventory {
	type listing struct {
		comps []Component
		err   error
	}
	var inv Inventory
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, p := range providers {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			var aList, bList listing
			var inner sync.WaitGroup
			inner.Add(2)
			go func() { defer inner.Done(); aList.comps, aList.err = p.List(a) }()
			go func() { defer inner.Done(); bList.comps, bList.err = p.List(b) }()
			inner.Wait()

			mu.Lock()
			defer mu.Unlock()
			if aList.err != nil {
				inv.Errors = append(inv.Errors, TypeError{Type: p.TypeLabel(), Side: "source", Err: aList.err})
			}
			if bList.err != nil {
				inv.Errors = append(inv.Errors, TypeError{Type: p.TypeLabel(), Side: "target", Err: bList.err})
			}
			rows := matchComponents(p.TypeLabel(), aList.comps, bList.comps)
			inv.Rows = append(inv.Rows, rows...)
		}()
	}
	wg.Wait()

	sort.Slice(inv.Rows, func(i, j int) bool {
		if inv.Rows[i].Type != inv.Rows[j].Type {
			return inv.Rows[i].Type < inv.Rows[j].Type
		}
		return inv.Rows[i].Key < inv.Rows[j].Key
	})
	return inv
}

// matchComponents joins two component slices by Key and classifies each
// match. When both sides exist, Status is provisionally StatusDifferent
// if the cheap Summary differs, else StatusSame — the body diff on
// drill-in is authoritative and can upgrade Same→Different. (We never
// claim Same falsely in a way that hides a change the user opens; the
// body view recomputes.)
func matchComponents(typeLabel string, a, b []Component) []Row {
	byKeyA := make(map[string]Component, len(a))
	for _, c := range a {
		byKeyA[c.Key] = c
	}
	byKeyB := make(map[string]Component, len(b))
	for _, c := range b {
		byKeyB[c.Key] = c
	}

	seen := make(map[string]bool)
	var rows []Row
	for _, ca := range a {
		seen[ca.Key] = true
		if cb, ok := byKeyB[ca.Key]; ok {
			st := StatusSame
			if ca.Summary != cb.Summary {
				st = StatusDifferent
			}
			rows = append(rows, Row{
				Type: typeLabel, Key: ca.Key, Status: st,
				ASummary: ca.Summary, BSummary: cb.Summary,
				AID: ca.ID, BID: cb.ID,
			})
		} else {
			rows = append(rows, Row{
				Type: typeLabel, Key: ca.Key, Status: StatusAOnly,
				ASummary: ca.Summary, BSummary: "—", AID: ca.ID,
			})
		}
	}
	for _, cb := range b {
		if seen[cb.Key] {
			continue
		}
		rows = append(rows, Row{
			Type: typeLabel, Key: cb.Key, Status: StatusBOnly,
			ASummary: "—", BSummary: cb.Summary, BID: cb.ID,
		})
	}
	return rows
}

// Snapshot is a full metadata capture from one org: type → (component
// name → source/XML). Produced by a single bulk retrieve. The
// snapshot-based compare path uses these directly — no per-component
// API calls, since every body is already in hand.
type Snapshot map[string]map[string]string

// CompareSnapshots diffs two already-retrieved snapshots. Because the
// bodies are present, this resolves Same vs Different EXACTLY (no
// summary heuristic) and needs zero further API calls. This is the fast
// "all metadata" path: one bulk retrieve per org feeds this.
//
// Rows carry the component name in AID/BID so BodyDiffFromSnapshots can
// re-diff on drill-in without any fetch.
// CompareSnapshots diffs two metadata snapshots. Pass any TypeErrors
// from the retrieve phase (a type that failed to fetch on one side): they
// are surfaced on Inv.Errors AND that type's rows are suppressed, so a
// type we couldn't fetch is reported as an error rather than as phantom
// "only-on-the-other-side" drift.
func CompareSnapshots(a, b Snapshot, errs ...TypeError) Inventory {
	var inv Inventory
	inv.Errors = append(inv.Errors, errs...)
	// Any type with a fetch error on either side can't be diffed reliably —
	// skip its rows so the inventory doesn't show fabricated drift.
	failedType := map[string]bool{}
	for _, e := range errs {
		failedType[e.Type] = true
	}
	types := map[string]bool{}
	for t := range a {
		types[t] = true
	}
	for t := range b {
		types[t] = true
	}
	for typeLabel := range types {
		if failedType[typeLabel] {
			continue
		}
		aMap := a[typeLabel]
		bMap := b[typeLabel]
		seen := map[string]bool{}
		for name, aBody := range aMap {
			seen[name] = true
			if bBody, ok := bMap[name]; ok {
				st := StatusSame
				// Cheap path first: identical raw bodies are Same with no
				// work (the common case — most components match). Only when
				// they differ raw do we pay normalizeBody (PrettyXML), which
				// is expensive and, over ~80k components, would otherwise
				// freeze the diff. normalizeBody collapses cosmetic-only
				// differences to avoid phantom "Different" rows.
				if aBody != bBody && normalizeBody(aBody) != normalizeBody(bBody) {
					st = StatusDifferent
				}
				inv.Rows = append(inv.Rows, Row{
					Type: typeLabel, Key: name, Status: st,
					ASummary: "present", BSummary: "present",
					AID: name, BID: name,
				})
			} else {
				inv.Rows = append(inv.Rows, Row{
					Type: typeLabel, Key: name, Status: StatusAOnly,
					ASummary: "present", BSummary: "—", AID: name,
				})
			}
		}
		for name := range bMap {
			if seen[name] {
				continue
			}
			inv.Rows = append(inv.Rows, Row{
				Type: typeLabel, Key: name, Status: StatusBOnly,
				ASummary: "—", BSummary: "present", BID: name,
			})
		}
	}
	sort.Slice(inv.Rows, func(i, j int) bool {
		if inv.Rows[i].Type != inv.Rows[j].Type {
			return inv.Rows[i].Type < inv.Rows[j].Type
		}
		return inv.Rows[i].Key < inv.Rows[j].Key
	})
	return inv
}

// BodyDiffFromSnapshots diffs one row's bodies straight from the
// snapshots — no API call, since the bulk retrieve already has them.
func BodyDiffFromSnapshots(row Row, a, b Snapshot) Result {
	var aBody, bBody string
	if m := a[row.Type]; m != nil {
		aBody = m[row.Key]
	}
	if m := b[row.Type]; m != nil {
		bBody = m[row.Key]
	}
	// Pretty-print so the line differ aligns on individual elements
	// (readMetadata XML is one giant line otherwise → useless diff).
	return Text(PrettyXML(aBody), PrettyXML(bBody))
}

// NormalizeBody is the exported form of normalizeBody, for callers that
// hash bodies BEFORE handing snapshots to CompareSnapshots: hashing the
// normalized body makes the cheap hash-equality verdict match the
// normalized comparison the doc promises (otherwise cosmetic-only XML
// differences are misclassified Different).
func NormalizeBody(s string) string { return normalizeBody(s) }

// normalizeBody pretty-prints the XML then trims trailing whitespace,
// so equality (Same vs Different) compares the same reflowed form the
// diff view shows. Salesforce returns consistent XML from the same
// API/version, so this is enough to avoid cosmetic phantom diffs.
func normalizeBody(s string) string {
	s = PrettyXML(s)
	if s == "" {
		return ""
	}
	lines := splitLines(s)
	for i := range lines {
		lines[i] = trimTrailingSpace(lines[i])
	}
	// Drop trailing empty lines.
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func trimTrailingSpace(s string) string {
	end := len(s)
	for end > 0 && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[:end]
}

// BodyDiff fetches both bodies for a matched row and diffs them. Used on
// drill-in (Screen 4). For A-only / B-only rows one side is empty, which
// the differ renders as a pure insert/delete.
func BodyDiff(row Row, source, target string, p Provider) (Result, error) {
	var aBody, bBody string
	var err error
	if row.AID != "" {
		if aBody, err = p.Body(source, row.AID); err != nil {
			return Result{}, fmt.Errorf("source body: %w", err)
		}
	}
	if row.BID != "" {
		if bBody, err = p.Body(target, row.BID); err != nil {
			return Result{}, fmt.Errorf("target body: %w", err)
		}
	}
	return Text(aBody, bBody), nil
}
