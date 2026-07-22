package qchip

// Origin tags where a chip came from. Drives the prefix glyph in the
// chip strip + which edit/delete affordances the manager modal offers.
//
//	OriginBuiltIn  ships with sf-deck — read-only
//	OriginUser     authored via the wizard / chip manager
//	OriginImported copied from a Salesforce list view via the import
//	               flow (records / flows — any sObject SF will describe)
type Origin int

const (
	OriginBuiltIn Origin = iota
	OriginUser
	OriginImported
)

// String renders an origin label for help / debug surfaces.
func (o Origin) String() string {
	switch o {
	case OriginBuiltIn:
		return "built-in"
	case OriginUser:
		return "user"
	case OriginImported:
		return "imported"
	}
	return "?"
}

// Glyph is the small prefix character used in the chip strip:
//
//	built-in  →  no prefix (the default-feeling kind)
//	user      →  · (small dot)
//	imported  →  ↓ (small arrow)
//
// Centralised so all chip-rendering surfaces stay visually consistent.
func (o Origin) Glyph() string {
	switch o {
	case OriginUser:
		return "· "
	case OriginImported:
		return "↓ "
	}
	return ""
}

// SharedGlyph is the marker shown next to a chip whose Share spans
// more than just the current org — multi-org list, OrgGroup, or global.
// Single-org chips render WITHOUT this glyph (they're functionally
// private). Trailing space because callers concatenate it inline.
//
// Kept separate from the origin glyph so a user-authored shared chip
// reads as "· ⇄ Name" (mine, shared) rather than conflating the two.
const SharedGlyph = "⇄ "
