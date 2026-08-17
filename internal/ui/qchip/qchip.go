// Package qchip is the unified chip type backed by query.Query.
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

	Label string

	Scope string

	Origin Origin

	// Query is the AST. The same Query drives both Eval and ToSOQL
	// so a chip can never disagree with itself between client and
	// server execution paths.
	Query query.Query

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

	Share settings.ChipShare
}
