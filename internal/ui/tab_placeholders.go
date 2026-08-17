package ui

import (
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

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
