package uilayout

// Small pure helpers used across render / view code: the org-status
// dot color, safe truncation, human-readable timestamps, line clipping,
// and a couple of org/cache type adapters.

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// StatusDot returns a colored `●` that matches an org's connectedStatus.
func StatusDot(s string) string {
	switch s {
	case "Connected":
		return lipgloss.NewStyle().Foreground(theme.Green).Render("●")
	case "AuthDecryptError", "RefreshTokenAuthError":
		return lipgloss.NewStyle().Foreground(theme.Red).Render("●")
	case "":
		return lipgloss.NewStyle().Foreground(theme.Muted).Render("●")
	default:
		return lipgloss.NewStyle().Foreground(theme.Yellow).Render("●")
	}
}

// Truncate is a byte-count-safe string trimmer with an ellipsis. Used
// for short labels where ansi.Truncate's cost isn't worth it.
// Truncate clamps s to n display cells with an ellipsis. Width-aware
// and rune-safe — the byte-slicing original split multi-byte runes
// mid-character (mojibake on non-ASCII labels/values).
func Truncate(s string, n int) string {
	if n <= 0 {
		return s
	}
	return ansi.Truncate(s, n, "…")
}

// HumanAge formats a cache timestamp as "just now" / "4m ago" / etc.
func HumanAge(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// ClipLines hard-caps s to the first n lines so overflowing content
// can't push a fixed-height pane past its declared size.
func ClipLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n")
}

func OrgsToRows(orgs []sf.Org) []cache.OrgRow {
	out := make([]cache.OrgRow, len(orgs))
	for i, o := range orgs {
		out[i] = cache.OrgRow{
			Alias: o.Alias, Username: o.Username, InstanceURL: o.InstanceURL,
			OrgID: o.OrgID, IsSandbox: o.IsSandbox, IsScratch: o.IsScratch,
			IsDevHub: o.IsDevHub, Status: o.Status, LastUsed: o.LastUsed,
			ExpirationDate: o.ExpirationDate,
			IsDefault:      o.IsDefault, IsDefaultDevHub: o.IsDefaultDevHub,
		}
	}
	return out
}

// TargetArg is the value to pass to `sf -o <target>`. Prefers alias
// (shorter, stable) but falls back to username if no alias is set.
func TargetArg(o sf.Org) string {
	if o.Alias != "" {
		return o.Alias
	}
	return o.Username
}

// CanUseOrg reports whether the org's token is in a state where we can
// safely shell out to sf for it. Auth-errored orgs are skipped so we
// don't spam failures.
func CanUseOrg(o sf.Org) bool {
	switch o.Status {
	case "AuthDecryptError", "RefreshTokenAuthError":
		return false
	}
	return true
}
