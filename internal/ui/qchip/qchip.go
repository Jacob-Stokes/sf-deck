// Package qchip is the unified chip type backed by query.Query.
//
// One Chip type covers every surface (records / objects / flows /
// future); the difference between server-filtering (records: ToSOQL
// + fetch) and client-filtering (objects + flows: predicate over
// cached rows) is an engine choice at apply time, not a separate type.
//
// Origin + Registry live alongside the type itself rather than in a
// separate package — the pre-unification "two implementers of a
// chip.Chip interface" world is gone, so the interface seam isn't
// needed. If a different shape of chip (saved-soql, group-by) ever
// wants the manager + persistence UX, it'll come back; YAGNI for now.
package qchip

import (
	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

// Chip is one named query a user can apply to a surface.
//
// The same Chip runs server-side via ApplyToSOQL (records) or
// client-side via ApplyToRow (objects, flows). The Query AST can
// never disagree with itself across modes — ToSOQL and Eval are
// tested in lockstep over the same tree.
type Chip struct {
	// ID is the stable identifier. For built-ins it's a hand-chosen
	// kebab-case slug ("active", "custom"); for user-authored chips
	// it's auto-generated (slug + timestamp). Imported chips re-use
	// the SF list-view name slugified, so re-importing the same
	// view overwrites cleanly.
	ID string

	// Label is the user-facing name shown on the chip strip.
	Label string

	// Scope is "*" / "" for surface-wide chips (Active flows, Custom
	// objects). For records-shaped surfaces it's the sObject API
	// name the chip applies to (Case, Account, …).
	Scope string

	// Origin is built-in / user / imported. Drives the prefix glyph
	// in the chip strip and edit/delete affordances in the manager.
	Origin Origin

	// Query is the AST. The same Query drives both Eval and ToSOQL
	// so a chip can never disagree with itself between client and
	// server execution paths.
	Query query.Query

	// SourceID / SourceName / ImportedAt are populated when Origin
	// is OriginImported — they let the manager modal show
	// "imported from <list view>" + a posterity link to the
	// original Salesforce list view.
	SourceID   string
	SourceName string
	ImportedAt string

	// Favourite controls whether the chip appears on the surface's
	// quick-cycle strip. Non-favourite chips live in the "+ N more"
	// overflow modal — accessible but not fetched until explicitly
	// chosen. Built-ins ship with sensible defaults; users toggle
	// the flag from the chip manager.
	Favourite bool

	// LockedFavourite means the chip's favourite-on-strip flag
	// can't be toggled by the user. Used for the "Recent" built-in
	// on records — every records-shaped surface should always have
	// at least one cycle target. The chip manager hides the
	// pin/unpin affordance when this is set.
	LockedFavourite bool

	// OrgUser is the LEGACY single-org binding, superseded by Share.
	// Kept on the runtime type during the migration so adapter/persistence
	// can still round-trip pre-Share TOML; new code should read Share
	// (which carries the OrgUser value when only OrgUser was set on disk).
	// All user/imported chips are stamped at create time — either as
	// Share{Kind:Org} (new) or OrgUser (legacy, normalised on next write).
	OrgUser string

	// Share is the cross-org visibility scope: single org, list of orgs,
	// org-group, or global. The Registry filters chips by calling
	// Share.Allows(activeOrg, groupMembers). Built-in chips ship with
	// the global default; user/imported chips ship per-org by default
	// and can be widened via the chip manager's scope chooser. See
	// settings.ChipShare for the discriminator and resolution rules.
	Share settings.ChipShare
}
