// Package devproject is the row-builder for DevProject exports —
// turns a DevProject + its Items into the generic exporters.ExportRow
// shape. Format-specific writing happens upstream in the exporters
// package; this file concerns itself only with "what columns + values
// do we want for a project export?"
//
// Used by:
//   - The TUI's `e` (export) key on /dev-projects → opens a format
//     picker → calls Rows() with the active project + URL resolver,
//     then exporters.Write().
//   - Future: package.xml export (Gearset workflow). Will be a
//     sibling file in this package that consumes the same Item
//     slice but produces a manifest XML, not ExportRows.
package devproject

import (
	"strings"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/exporters"
)

// Headers is the canonical column order. Stable so users can build
// scripts / sheets that reference columns by header. Adding a new
// column appends at the end; never insert in the middle without a
// version bump.
//
// Column choices, with reasoning:
//   - Name        : human-friendly label (Item.Name captured at collect time)
//   - Kind        : ItemKind in human form ("Apex Class" not "apex_class")
//   - Ref         : the stable identifier (Salesforce Id or API name)
//   - Parent      : Item.Type — sObject for fields/VRs/RTs/triggers, "Flow" for flow versions
//   - Org         : origin org username — important for multi-org projects
//   - URL         : Lightning URL for the item (resolved by caller-supplied function)
//   - Added       : ISO-8601 timestamp of when the item was K-collected
//   - Notes       : user's freeform note (currently empty until UI lands)
//
// `Ref` and `Parent` are deliberately separate columns rather than
// merged into "qualified name" — scripts often want one or the other
// alone (e.g. group by Parent to count fields per sObject). Joining
// is trivial in a spreadsheet, splitting back out is annoying.
var Headers = []string{
	"Name",
	"Kind",
	"Ref",
	"Parent",
	"Org",
	"URL",
	"Added",
	"Notes",
}

// URLResolver returns the Lightning URL for an item, or "" when the
// kind doesn't have a meaningful URL (KindRecord without org context,
// abstract kinds, etc.). Caller-supplied because the URL depends on
// the item's origin org's instance URL — that lives on the UI side
// and isn't visible to this package.
//
// Resolver is called once per Item; cheap (just string formatting).
type URLResolver func(devproject.Item) string

// Rows turns a slice of project Items into ExportRows. Pure: no I/O,
// no Salesforce calls. The resolver does the URL composition; if it's
// nil, the URL column is empty everywhere.
func Rows(items []devproject.Item, resolver URLResolver) []exporters.ExportRow {
	out := make([]exporters.ExportRow, 0, len(items))
	for _, it := range items {
		url := ""
		if resolver != nil {
			url = resolver(it)
		}
		out = append(out, exporters.ExportRow{
			Columns: map[string]string{
				"Name":   it.Name,
				"Kind":   kindLabel(it.Kind),
				"Ref":    it.Ref,
				"Parent": it.Type,
				"Org":    it.OrgUser,
				"URL":    url,
				"Added":  formatTime(it.AddedAt),
				"Notes":  it.Notes,
			},
		})
	}
	return out
}

// kindLabel translates an ItemKind enum value to a human-readable
// column value. The raw enum strings ("apex_class", "permset_group")
// are stable for code but ugly in spreadsheets; this map is the
// presentation layer. Returns the raw string when no mapping is
// declared so a new ItemKind never disappears from exports — it
// just shows up in its underscored form until someone adds a label.
func kindLabel(k devproject.ItemKind) string {
	switch k {
	case devproject.KindSObject:
		return "sObject"
	case devproject.KindField:
		return "Field"
	case devproject.KindFlow:
		return "Flow"
	case devproject.KindFlowVersion:
		return "Flow Version"
	case devproject.KindRecord:
		return "Record"
	case devproject.KindApexClass:
		return "Apex Class"
	case devproject.KindApexTrigger:
		return "Apex Trigger"
	case devproject.KindReport:
		return "Report"
	case devproject.KindPermissionSet:
		return "Permission Set"
	case devproject.KindPermissionSetGroup:
		return "Permission Set Group"
	case devproject.KindProfile:
		return "Profile"
	case devproject.KindValidationRule:
		return "Validation Rule"
	case devproject.KindRecordType:
		return "Record Type"
	case devproject.KindLWC:
		return "LWC"
	case devproject.KindAura:
		return "Aura"
	case devproject.KindQueue:
		return "Queue"
	case devproject.KindPublicGroup:
		return "Public Group"
	case devproject.KindSOQLQuery:
		return "SOQL Query"
	}
	return string(k)
}

// formatTime returns an ISO-8601 timestamp suitable for spreadsheet
// columns. Empty when the time is zero (unset) so blank cells render
// rather than "0001-01-01T00:00:00Z".
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// SuggestedFilename builds a filename stem for the export, given a
// project name. Lowercases + replaces non-alphanumerics with hyphens
// so the result is filesystem-safe across macOS / Linux / Windows
// without any further escaping. Doesn't include the extension —
// callers append `.csv` / `.xlsx` / `.json` from the chosen Format.
//
// Example: "Q2 migration" → "q2-migration"
//
//	"Customer-360 / cleanup" → "customer-360-cleanup"
func SuggestedFilename(projectName string) string {
	if projectName == "" {
		return "dev-project"
	}
	var b strings.Builder
	prevDash := false
	for _, r := range projectName {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32) // tolower
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if out == "" {
		return "dev-project"
	}
	return out
}
