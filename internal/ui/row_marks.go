package ui

// Per-domain RowMark builders + per-item pill renderers.
//
// RowMarks live in two places:
//   - On a ListTableSpec.Marks slice — evaluated per row index at
//     render time, applied as inline tints/badges on the row body.
//   - On a detail surface — evaluated against one specific item,
//     rendered as a row of pills below the title so the user sees
//     the same metadata-shape labels they saw in the list.
//
// Same conceptual rule, two different surfaces. To avoid duplicating
// the predicates, this file exposes:
//
//   - marksFor<Domain>(items): a closure-driven []RowMark used by
//     list-tables (matches against a row index)
//   - markPillsFor<Domain>(item): a static-form list of {label, color}
//     used by detail renderers (matches against one specific item)
//
// Both share the underlying predicate (e.g. "non-empty namespace
// prefix means managed package") so adding a new domain mark only
// requires updating one predicate plus exposing it via both forms.

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// flagsCellMode resolves the user's Ctrl+F cycle state to the
// MarksCellMode the renderer should use. Hidden mode is handled by
// the column-inclusion check (flagsColumnVisible), not here, so this
// only ever returns Full or Letter.
func (m Model) flagsCellMode() uilayout.MarksCellMode {
	if m.settings == nil {
		return uilayout.MarksCellModeFull
	}
	if m.settings.FlagColumnDisplayMode() == settings.FlagColumnModeLetter {
		return uilayout.MarksCellModeLetter
	}
	return uilayout.MarksCellModeFull
}

// flagsColumnVisible reports whether the FLAGS column should be
// rendered at all. Drives surfaces' col-inclusion logic.
func (m Model) flagsColumnVisible() bool {
	if m.settings == nil {
		return true
	}
	return m.settings.FlagColumnVisible()
}

// renderFlagsCell is the surface-level helper. Each list_surface's
// Cell closure calls this for the FLAGS column instead of the
// generic uilayout.RenderMarksCell so the user's Ctrl+F mode flows
// through transparently.
func (m Model) renderFlagsCell(marks []uilayout.RowMark, row int) string {
	return uilayout.RenderMarksCellMode(marks, row, m.flagsCellMode())
}

// applyFlagsColumnMode rewrites a list-surface's columns slice for
// the user's current Ctrl+F mode. Surfaces declare the FLAGS column
// at the tail of their Cols spec; this helper either drops it
// (hidden mode) or tightens its Min/Ideal/Max (letter mode) so the
// user's pick flows through to the renderer without each surface
// duplicating the logic.
//
// Returns the slice unchanged when the trailing column isn't named
// "Marks" — defensive against future col-order changes.
func (m Model) applyFlagsColumnMode(cols []uilayout.ListColumn) []uilayout.ListColumn {
	if len(cols) == 0 {
		return cols
	}
	last := len(cols) - 1
	if cols[last].Name != "Marks" {
		return cols
	}
	switch m.flagsCellMode() {
	case uilayout.MarksCellModeLetter:
		// Compact: one cell per matching mark, no separator. Header
		// floor of 5 keeps the "FLAGS" label intact even when no row
		// in view has any flags.
		cols[last].Min = 5
		cols[last].Ideal = 5
		cols[last].Max = 8
	}
	if !m.flagsColumnVisible() {
		return cols[:last]
	}
	return cols
}

// markPill is one rendered annotation pill in a detail pane. Mirrors
// the Treatment shape on a RowMark but flattened to "this is what
// appears under the title for this specific item."
type markPill struct {
	Label string
	// PillColor tints both the badge bracket characters and the
	// label inside. Nil falls back to muted — but every caller in
	// this codebase sets it.
	PillColor color.Color
}

// renderMarkPills formats a slice of pills as a single space-separated
// row. Each pill renders as `[label]` with its color. Returns "" when
// the slice is empty so callers can drop the line cleanly.
//
// No subheading text — the pills' visual style + their match with
// what users already see in the list view carries the meaning.
func renderMarkPills(pills []markPill) string {
	if len(pills) == 0 {
		return ""
	}
	parts := make([]string, len(pills))
	for i, p := range pills {
		style := lipgloss.NewStyle().Bold(true)
		if p.PillColor != nil {
			style = style.Foreground(p.PillColor)
		} else {
			style = style.Foreground(theme.Muted)
		}
		parts[i] = style.Render("[" + p.Label + "]")
	}
	return strings.Join(parts, " ")
}

// --- sObject marks ----------------------------------------------------

// markPredicateCustomSObject reports whether an sObject API name is
// custom-shaped (one of the user-creatable suffixes). Single source
// of truth — both the list-table mark and the per-item pill list
// route through this so the rule can't drift.
func markPredicateCustomSObject(name string) bool {
	return sf.IsCustom(name)
}

// markPredicateManagedSObject reports whether an sObject's API name
// has a managed-package namespace prefix. Delegates to sf.IsManagedName
// so the marks column and the Managed/Unmanaged chips share one source
// of truth.
func markPredicateManagedSObject(name string) bool {
	return sf.IsManagedName(name)
}

// marksForSObjectList builds the RowMark slice for an sObject
// list-table. The closures index into the items slice the caller
// passes — typical pattern for list-table marks.
func marksForSObjectList(items []sf.SObject) []uilayout.RowMark {
	return []uilayout.RowMark{
		{
			ID:    "custom-sobject",
			Label: "custom",
			Matches: func(row int) bool {
				if row < 0 || row >= len(items) {
					return false
				}
				return markPredicateCustomSObject(items[row].Name)
			},
			Treatment: uilayout.Treatment{NameColor: theme.Cyan},
		},
		{
			ID:    "managed-sobject",
			Label: "managed",
			Matches: func(row int) bool {
				if row < 0 || row >= len(items) {
					return false
				}
				return markPredicateManagedSObject(items[row].Name)
			},
			Treatment: uilayout.Treatment{
				BadgeColor: theme.Yellow,
			},
		},
	}
}

// markPillsForSObject returns the per-item pills shown in a detail
// pane. Routes through the same predicates as marksForSObjectList
// so list view and detail view always agree.
func markPillsForSObject(name string) []markPill {
	var out []markPill
	if markPredicateCustomSObject(name) {
		out = append(out, markPill{Label: "custom", PillColor: theme.Cyan})
	}
	if markPredicateManagedSObject(name) {
		out = append(out, markPill{Label: "managed", PillColor: theme.Yellow})
	}
	return out
}

// --- Apex class marks -------------------------------------------------

func markPredicateManagedApex(namespace string) bool {
	return namespace != ""
}

// marksForApexClassList builds the RowMark slice for the /apex
// classes list. Today there's only the managed badge; more rules
// (deprecated, generated, etc.) drop in here as they're added.
func marksForApexClassList(items []sf.ApexClassRow) []uilayout.RowMark {
	return []uilayout.RowMark{
		{
			ID:    "managed-apex",
			Label: "managed",
			Matches: func(row int) bool {
				if row < 0 || row >= len(items) {
					return false
				}
				return markPredicateManagedApex(items[row].NamespacePrefix)
			},
			Treatment: uilayout.Treatment{
				BadgeColor: theme.Yellow,
			},
		},
	}
}

// markPillsForApexClass renders pills for one ApexClassRow. Used by
// the /apex detail sidebar.
func markPillsForApexClass(row sf.ApexClassRow) []markPill {
	var out []markPill
	if markPredicateManagedApex(row.NamespacePrefix) {
		out = append(out, markPill{Label: "managed: " + row.NamespacePrefix,
			PillColor: theme.Yellow})
	}
	if !row.IsValid {
		out = append(out, markPill{Label: "invalid", PillColor: theme.Red})
	}
	return out
}

// --- Apex trigger marks ----------------------------------------------

// marksForApexTriggerList is the trigger-list equivalent of
// marksForApexClassList. Same managed-package rule.
func marksForApexTriggerList(items []sf.TriggerRow) []uilayout.RowMark {
	return []uilayout.RowMark{
		{
			ID:    "managed-trigger",
			Label: "managed",
			Matches: func(row int) bool {
				if row < 0 || row >= len(items) {
					return false
				}
				return markPredicateManagedApex(items[row].NamespacePrefix)
			},
			Treatment: uilayout.Treatment{
				BadgeColor: theme.Yellow,
			},
		},
	}
}

// markPillsForApexTrigger renders pills for one TriggerRow.
func markPillsForApexTrigger(t sf.TriggerRow) []markPill {
	var out []markPill
	if markPredicateManagedApex(t.NamespacePrefix) {
		out = append(out, markPill{Label: "managed: " + t.NamespacePrefix,
			PillColor: theme.Yellow})
	}
	if !t.Valid {
		out = append(out, markPill{Label: "invalid", PillColor: theme.Red})
	}
	if t.Status != "" && t.Status != "Active" {
		out = append(out, markPill{Label: strings.ToLower(t.Status),
			PillColor: theme.Muted})
	}
	return out
}

// --- Flow marks ------------------------------------------------------

// markPredicateManagedFlow reports whether a flow row carries a
// managed-package namespace prefix. EntityDefinition stores the
// prefix on FlowDefinition; we cache it on Flow.Namespace.
func markPredicateManagedFlow(f sf.Flow) bool {
	return f.Namespace != ""
}

// marksForFlowList builds the RowMark slice for /flows.
func marksForFlowList(items []sf.Flow) []uilayout.RowMark {
	return []uilayout.RowMark{
		{
			ID:    "managed-flow",
			Label: "managed",
			Matches: func(row int) bool {
				if row < 0 || row >= len(items) {
					return false
				}
				return markPredicateManagedFlow(items[row])
			},
			Treatment: uilayout.Treatment{BadgeColor: theme.Yellow},
		},
		{
			ID:    "inactive-flow",
			Label: "inactive",
			Matches: func(row int) bool {
				if row < 0 || row >= len(items) {
					return false
				}
				return items[row].ActiveVersionID == ""
			},
			Treatment: uilayout.Treatment{BadgeColor: theme.Muted},
		},
	}
}

// markPillsForFlow renders pills for one Flow row in the detail pane.
func markPillsForFlow(f sf.Flow) []markPill {
	var out []markPill
	if markPredicateManagedFlow(f) {
		out = append(out, markPill{Label: "managed: " + f.Namespace,
			PillColor: theme.Yellow})
	}
	if f.ActiveVersionID == "" {
		out = append(out, markPill{Label: "inactive", PillColor: theme.Muted})
	}
	return out
}

// --- LWC bundle marks ------------------------------------------------

func markPredicateManagedLWC(b sf.LWCBundle) bool { return b.NamespacePrefix != "" }

// marksForLWCList badges managed bundles + exposed bundles.
func marksForLWCList(items []sf.LWCBundle) []uilayout.RowMark {
	return []uilayout.RowMark{
		{
			ID:    "managed-lwc",
			Label: "managed",
			Matches: func(row int) bool {
				if row < 0 || row >= len(items) {
					return false
				}
				return markPredicateManagedLWC(items[row])
			},
			Treatment: uilayout.Treatment{BadgeColor: theme.Yellow},
		},
		{
			ID:    "exposed-lwc",
			Label: "exposed",
			Matches: func(row int) bool {
				if row < 0 || row >= len(items) {
					return false
				}
				return items[row].IsExposed
			},
			Treatment: uilayout.Treatment{BadgeColor: theme.Green},
		},
	}
}

// --- Aura bundle marks -----------------------------------------------

func markPredicateManagedAura(b sf.AuraBundle) bool { return b.NamespacePrefix != "" }

// marksForAuraList badges managed bundles. Aura has no IsExposed flag.
func marksForAuraList(items []sf.AuraBundle) []uilayout.RowMark {
	return []uilayout.RowMark{
		{
			ID:    "managed-aura",
			Label: "managed",
			Matches: func(row int) bool {
				if row < 0 || row >= len(items) {
					return false
				}
				return markPredicateManagedAura(items[row])
			},
			Treatment: uilayout.Treatment{BadgeColor: theme.Yellow},
		},
	}
}

// --- PermSet marks ---------------------------------------------------

func markPredicateManagedPermSet(p sf.PermissionSet) bool { return p.NamespacePrefix != "" }

// marksForPermSetList badges managed PSes + session-based PSes.
func marksForPermSetList(items []sf.PermissionSet) []uilayout.RowMark {
	return []uilayout.RowMark{
		{
			ID:    "managed-permset",
			Label: "managed",
			Matches: func(row int) bool {
				if row < 0 || row >= len(items) {
					return false
				}
				return markPredicateManagedPermSet(items[row])
			},
			Treatment: uilayout.Treatment{BadgeColor: theme.Yellow},
		},
		{
			ID:    "session-permset",
			Label: "session",
			Matches: func(row int) bool {
				if row < 0 || row >= len(items) {
					return false
				}
				return items[row].Type == "Session"
			},
			Treatment: uilayout.Treatment{BadgeColor: theme.Cyan},
		},
		{
			ID:    "custom-permset",
			Label: "custom",
			Matches: func(row int) bool {
				if row < 0 || row >= len(items) {
					return false
				}
				return items[row].IsCustom
			},
			Treatment: uilayout.Treatment{BadgeColor: theme.Muted},
		},
	}
}

// markPillsForPermSet renders pills for one PermissionSet detail pane.
func markPillsForPermSet(p sf.PermissionSet) []markPill {
	var out []markPill
	if markPredicateManagedPermSet(p) {
		out = append(out, markPill{Label: "managed: " + p.NamespacePrefix,
			PillColor: theme.Yellow})
	}
	if p.Type == "Session" {
		out = append(out, markPill{Label: "session", PillColor: theme.Cyan})
	}
	if p.IsCustom {
		out = append(out, markPill{Label: "custom", PillColor: theme.Muted})
	}
	return out
}

// --- PermSet Group marks --------------------------------------------

func markPredicateManagedPSG(g sf.PermissionSetGroup) bool { return g.NamespacePrefix != "" }

// marksForPSGList badges managed PSGs and surfaces non-Updated status
// (Outdated/Failed/Updating) so admins can spot rebuild work at a glance.
func marksForPSGList(items []sf.PermissionSetGroup) []uilayout.RowMark {
	return []uilayout.RowMark{
		{
			ID:    "managed-psg",
			Label: "managed",
			Matches: func(row int) bool {
				if row < 0 || row >= len(items) {
					return false
				}
				return markPredicateManagedPSG(items[row])
			},
			Treatment: uilayout.Treatment{BadgeColor: theme.Yellow},
		},
		{
			ID:    "stale-psg",
			Label: "outdated",
			Matches: func(row int) bool {
				if row < 0 || row >= len(items) {
					return false
				}
				s := items[row].Status
				return s == "Outdated" || s == "Failed"
			},
			Treatment: uilayout.Treatment{BadgeColor: theme.Red},
		},
	}
}

// markPillsForPSG renders pills for one PermissionSetGroup detail pane.
func markPillsForPSG(g sf.PermissionSetGroup) []markPill {
	var out []markPill
	if markPredicateManagedPSG(g) {
		out = append(out, markPill{Label: "managed: " + g.NamespacePrefix,
			PillColor: theme.Yellow})
	}
	if g.Status == "Outdated" || g.Status == "Failed" {
		out = append(out, markPill{Label: strings.ToLower(g.Status),
			PillColor: theme.Red})
	}
	return out
}

// --- Profile marks ---------------------------------------------------

// marksForProfileList badges custom profiles (non-standard UserType).
// Standard profiles have UserType == "Standard"; everything else is
// either a license-driven profile (Partner / Customer Portal) or a
// custom built by an admin — both worth flagging.
func marksForProfileList(items []sf.Profile) []uilayout.RowMark {
	return []uilayout.RowMark{
		{
			ID:    "non-standard-profile",
			Label: "non-standard",
			Matches: func(row int) bool {
				if row < 0 || row >= len(items) {
					return false
				}
				return items[row].UserType != "" && items[row].UserType != "Standard"
			},
			Treatment: uilayout.Treatment{BadgeColor: theme.Cyan},
		},
	}
}

// markPillsForProfile renders pills for one Profile detail pane.
func markPillsForProfile(p sf.Profile) []markPill {
	var out []markPill
	if p.UserType != "" && p.UserType != "Standard" {
		out = append(out, markPill{Label: strings.ToLower(p.UserType),
			PillColor: theme.Cyan})
	}
	return out
}
