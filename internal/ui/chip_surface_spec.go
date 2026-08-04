package ui

// chip_surface_spec.go — typed builder that collapses chipSurface
// closure repetition.
//
// Before: every chipped surface declared 20-40 lines of identical
// closures wiring SetExtra(chipMatcherFor[T]), applyVisitedListOrder,
// clearVisitedListOrder, and the ScopeCount lookup. The variation
// per surface was just the ListView field, the qchip.Registry
// pointer, the visit-RecentKind, the optional scope predicate, and
// the ID extractor.
//
// After: each surface declares a typed ListChipSurfaceSpec[T] with
// just that variation. newListChipSurface(spec) produces the full
// chipSurface value with all closures pre-wired.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/orgproject"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

// ListChipSurfaceSpec[T] declares the per-surface variation that
// drives chip / project-chip / visited-chip behaviour. T is the
// row type of the underlying ListView (sf.SObject, sf.Flow, etc.).
//
// Required:
//   - Domain: the chipDomain enum value ("objects", "flows", ...)
//   - ChipIdx + SetChipIdx: read/write the active chip cursor on
//     the Model. Surface-specific because each tab stores its own
//     cursor int (m.objectsChipIdx(), m.flowsChipIdx(), etc.).
//   - ListPtr: returns the per-orgData ListView[T] this surface
//     owns. Used for ResetList, ApplyChip, ApplyVisitedChip, and
//     ClearVisitedOrder.
//   - Registry: returns the *qchip.Registry to consult.
//   - VisitKind: the RecentKind constant this surface's visit
//     entries are tagged with (RecentKindFlow, RecentKindLWC, ...).
//   - IDOf: extracts the visit-bucket key from a row. Usually the
//     row's .ID; objects use .Name (sObject API name).
//   - VisitKey: short string passed to applyVisitedListOrder as
//     part of the order-cache key — distinguishes flow vs. lwc when
//     they happen to share IDs. Conventionally the lowercase
//     domain name ("flow", "lwc", "permset").
//   - ManagerTitle: string shown atop the V chip-manager modal.
//
// Optional:
//   - ScopeContains: returns true when the project scope contains
//     a row's ID. Setting this enables the project chip on the
//     surface and wires ApplyProjectChip automatically.
//   - ScopeCount: how many of this surface's items the active
//     scope contains. Required when ScopeContains is set so the
//     strip can decide whether to surface the project chip.
//   - ImportFromSF: surface offers "Import from Salesforce" in the
//     chip manager modal (only Records + Flows historically).
//   - ManagerScope: for per-row scoped surfaces (Records is per-
//     sObject); nil means universal "*".
type ListChipSurfaceSpec[T query.Row] struct {
	Domain     chipDomain
	ChipIdx    func(Model) int
	SetChipIdx func(*Model, int)
	ListPtr    func(d *orgData) *ListView[T]
	Registry   func(m *Model) *qchip.Registry
	VisitKind  string
	IDOf       func(T) string
	VisitKey   string

	ScopeContains func(s *orgproject.Scope, id string) bool
	ScopeCount    func(s *orgproject.Scope) int

	ManagerTitle string
	ManagerScope func(Model) string
	ImportFromSF bool
}

// newListChipSurface compiles a ListChipSurfaceSpec[T] into the
// type-erased chipSurface value that the registry expects.
func newListChipSurface[T query.Row](spec ListChipSurfaceSpec[T]) chipSurface {
	cs := chipSurface{
		Domain:     spec.Domain,
		ChipIdx:    spec.ChipIdx,
		SetChipIdx: spec.SetChipIdx,
		Registry:   spec.Registry,
		ResetList: func(d *orgData) {
			if d == nil {
				return
			}
			spec.ListPtr(d).ResetCursor()
		},
		ApplyChip: func(d *orgData, c qchip.Chip) {
			if d == nil {
				return
			}
			spec.ListPtr(d).SetExtra(chipMatcherFor[T](c, chipSubs(d)))
		},
		ApplyVisitedChip: func(m Model, d *orgData) {
			if d == nil {
				return
			}
			lv := spec.ListPtr(d)
			visited := m.recentVisitedIDsByKind(orgUserOrEmpty(m), spec.VisitKind)
			lv.SetExtra(func(t T) bool {
				return visited[spec.IDOf(t)]
			})
			rank := m.recentVisitedRankByKind(orgUserOrEmpty(m), spec.VisitKind)
			applyVisitedListOrder(lv, rank, spec.IDOf, spec.VisitKey, d.recentGen)
		},
		ClearVisitedOrder: func(d *orgData) {
			if d == nil {
				return
			}
			clearVisitedListOrder(spec.ListPtr(d))
		},
		ManagerTitle: func(Model) string { return spec.ManagerTitle },
		ManagerScope: spec.ManagerScope,
		ImportFromSF: spec.ImportFromSF,
	}
	if spec.ScopeContains != nil {
		cs.ApplyProjectChip = func(d *orgData, scope *orgproject.Scope) {
			if d == nil {
				return
			}
			lv := spec.ListPtr(d)
			lv.SetExtra(func(t T) bool {
				return spec.ScopeContains(scope, spec.IDOf(t))
			})
		}
	}
	cs.ScopeCount = spec.ScopeCount
	return cs
}
