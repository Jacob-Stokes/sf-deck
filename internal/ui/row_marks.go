package ui

// Per-domain RowMark builders + per-item pill renderers.

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func (m Model) flagsCellMode() uilayout.MarksCellMode {
	if m.settings == nil {
		return uilayout.MarksCellModeFull
	}
	if m.settings.FlagColumnDisplayMode() == settings.FlagColumnModeLetter {
		return uilayout.MarksCellModeLetter
	}
	return uilayout.MarksCellModeFull
}

func (m Model) flagsColumnVisible() bool {
	if m.settings == nil {
		return true
	}
	return m.settings.FlagColumnVisible()
}

func (m Model) renderFlagsCell(marks []uilayout.RowMark, row int) string {
	return uilayout.RenderMarksCellMode(marks, row, m.flagsCellMode())
}

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
		cols[last].Min = 5
		cols[last].Ideal = 5
		cols[last].Max = 8
	}
	if !m.flagsColumnVisible() {
		return cols[:last]
	}
	return cols
}

type markPill struct {
	Label     string
	PillColor color.Color
}

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

// markPredicateCustomSObject reports whether an sObject API name is
// custom-shaped (one of the user-creatable suffixes). Single source
// of truth — both the list-table mark and the per-item pill list
// route through this so the rule can't drift.
func markPredicateCustomSObject(name string) bool {
	return sf.IsCustom(name)
}

func markPredicateManagedSObject(name string) bool {
	return sf.IsManagedName(name)
}

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

func markPredicateManagedApex(namespace string) bool {
	return namespace != ""
}

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

func markPredicateManagedFlow(f sf.Flow) bool {
	return f.Namespace != ""
}

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

func markPredicateManagedLWC(b sf.LWCBundle) bool { return b.NamespacePrefix != "" }

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

func markPredicateManagedAura(b sf.AuraBundle) bool { return b.NamespacePrefix != "" }

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

func markPredicateManagedPermSet(p sf.PermissionSet) bool { return p.NamespacePrefix != "" }

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

func markPredicateManagedPSG(g sf.PermissionSetGroup) bool { return g.NamespacePrefix != "" }

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

func markPillsForProfile(p sf.Profile) []markPill {
	var out []markPill
	if p.UserType != "" && p.UserType != "Standard" {
		out = append(out, markPill{Label: strings.ToLower(p.UserType),
			PillColor: theme.Cyan})
	}
	return out
}
