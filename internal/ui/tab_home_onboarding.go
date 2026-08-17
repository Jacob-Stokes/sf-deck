package ui

import (
	"errors"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func (m Model) renderHomeOnboarding(inner, budget int) string {
	missingSF := m.orgsRes.Err() != nil && errors.Is(m.orgsRes.Err(), sf.ErrSFNotFound)

	headingStyle := lipgloss.NewStyle().
		Foreground(theme.Fg).
		Bold(true)
	bodyStyle := lipgloss.NewStyle().
		Foreground(theme.FgDim)
	mutedStyle := theme.Subtle
	cmdStyle := lipgloss.NewStyle().
		Foreground(theme.Cyan)

	var heading string
	var body []string

	if missingSF {
		heading = "Salesforce CLI not found"
		body = []string{
			"sf-deck talks to Salesforce through the `sf` CLI. It's not",
			"installed (or not on your PATH).",
			"",
			"Install it from:",
			"  " + cmdStyle.Render("https://developer.salesforce.com/tools/salesforcecli"),
			"",
			"Then authenticate to at least one org:",
			"  " + cmdStyle.Render("sf org login web"),
			"",
			"Once " + cmdStyle.Render("sf org list") + " returns your orgs, restart sf-deck.",
		}
	} else {
		heading = "Welcome to sf-deck"
		body = []string{
			"sf-deck is a terminal UI for working across your Salesforce",
			"orgs — schema, FLS, SOQL, records, deploys, users, and",
			"metadata diffs, all keyboard-driven.",
			"",
			"You don't have any orgs connected yet. Authenticate one with:",
			"  " + cmdStyle.Render("sf org login web"),
			"",
			"Or, from inside sf-deck, press " + cmdStyle.Render("'") + " then " +
				cmdStyle.Render("a") + " to open the add-org picker.",
			"",
			"Once an org is connected, sf-deck will load it automatically.",
			"Press " + cmdStyle.Render("?") + " on any screen for the full keymap.",
		}
	}

	footerHints := []string{
		mutedStyle.Render("Demo mode: ") + cmdStyle.Render("sf-deck --demo") +
			mutedStyle.Render("   ·   Quit: ") + cmdStyle.Render("Ctrl+C"),
	}

	lines := []string{
		"",
		"",
		centerOnboardingLine(headingStyle.Render(heading), inner),
		"",
	}
	for _, b := range body {
		lines = append(lines, centerOnboardingLine(bodyStyle.Render(b), inner))
	}
	lines = append(lines, "")
	for _, h := range footerHints {
		lines = append(lines, centerOnboardingLine(h, inner))
	}

	for len(lines) < budget {
		lines = append(lines, "")
	}
	if len(lines) > budget && budget > 0 {
		lines = lines[:budget]
	}
	return strings.Join(lines, "\n")
}

func noOrgPlaceholder() string {
	return theme.Subtle.Render("  No orgs connected. Press 1 for Home to set one up.")
}

func centerOnboardingLine(s string, inner int) string {
	w := lipgloss.Width(s)
	if w >= inner {
		return s
	}
	pad := (inner - w) / 2
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + s
}
