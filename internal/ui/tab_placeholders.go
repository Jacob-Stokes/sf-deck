package ui

// Shared placeholder renderer for "subtab exists, renderer doesn't
// yet" surfaces. Surfaces a short hint plus the equivalent Salesforce
// Setup URL so the user can o-open as a fallback while the real
// renderer hasn't landed.
//
// Used by /meta, /reports, /perms, /apex, /components, /home — every
// tab grew subtab placeholders in the big April-29 reorg.

import (
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// joinPlaceholder composes the standard "coming soon" body. Caller
// supplies the (already rendered) subtab strip plus the title +
// human-readable hint + an optional Setup path that o would normally
// route to.
func joinPlaceholder(subStrip, title, hint, setupPath string, inner int) string {
	var lines []string
	if subStrip != "" {
		lines = append(lines, subStrip)
	}
	lines = append(lines, sectionTitle(title))
	lines = append(lines, "")
	lines = append(lines, theme.Subtle.Render("  "+hint))
	if setupPath != "" {
		lines = append(lines, "")
		lines = append(lines, dimLine("  Setup path: "+setupPath, inner))
		lines = append(lines, dimLine("  Press o to open in Salesforce.", inner))
	}
	lines = append(lines, "")
	lines = append(lines, dimLine(
		"  shift+1..N to switch subtab · esc to leave", inner))
	return strings.Join(lines, "\n")
}
