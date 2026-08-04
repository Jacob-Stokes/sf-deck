package ui

// qchip registries + transient chip-strip state.
//
// Extracted from model.go. modelChips is embedded into Model so
// existing field access (m.chipRegistry(domainRecords), m.activeTransient, …) keeps
// working unchanged.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

// modelChips owns all qchip registries and transient chip-strip state.
type modelChips struct {
	// chipRegistries holds one qchip.Registry per domain, built from
	// the chipDomainDefs table (chip_domains.go). All domains use the
	// unified qchip.Chip backed by query.Query — one engine, two
	// execution modes (server vs client). Access via m.chipRegistry(d).
	chipRegistries map[chipDomain]*qchip.Registry

	// activeTransient tracks one non-favourite chip per (domain, scope)
	// that the user picked from the overflow modal — appears on the
	// strip in a distinct style next to the favourites until either
	// the user picks a different one (replaces) or pins it via 'f'
	// (becomes favourite, transient slot clears). Key is "<domain>|<scope>"
	// so per-sObject records surfaces have their own slot.
	activeTransient map[string]string

	// chipPreviews holds ephemeral session-only previews of chips from
	// OTHER orgs — surfaced via the manage-chips "chips from your other
	// orgs" section so the user can try a chip on the current org
	// without permanently widening its scope. Cleared on every relaunch
	// (not persisted). Keyed by transientSlotKey(domain, scope) so a
	// preview only shows where the user previewed it; each slot can
	// hold multiple previews so several "try" actions accumulate.
	//
	// Distinguished from activeTransient because:
	//   * activeTransient is "I picked it from THIS org's overflow",
	//     stays in scope normally, and persists into next launch via
	//     the user pinning it (no migration needed).
	//   * chipPreviews is "I'm peeking at a chip OWNED BY ANOTHER ORG",
	//     visually marked as such (dotted border + "from <other-org>"),
	//     and intentionally evaporates on relaunch so casual previews
	//     don't accumulate into a permanent cross-org pile.
	chipPreviews map[string][]chipPreview
}

// chipPreview is one ephemeral session-only entry in chipPreviews. The
// chip itself is carried by value (not by ID) so we don't have to chase
// it back through a registry that might not even hold it — the source
// org's registry isn't loaded in the current session necessarily.
//
// Columns / Limit / Clauses live alongside the qchip.Chip (they're
// settings.toml ChipConfig fields, not parsed-AST fields) so the
// chip.preview.save promotion path can rebuild a chips.CreateInput
// without re-parsing anything. Cross-org previews populate them
// from the source org's registry; IPC-spawned previews populate
// them from PreviewChipArgs verbatim.
type chipPreview struct {
	Domain        chipDomain
	Scope         string
	Chip          qchip.Chip
	OriginOrgUser string // canonical username of the org that OWNS the chip
	Columns       []string
	Limit         int
	Clauses       string
}
