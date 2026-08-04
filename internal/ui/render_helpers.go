package ui

// render_helpers.go — thin wrappers forwarding to uilayout.
// All pure render helpers now live in internal/ui/uilayout/render_helpers.go.
// These package-level aliases allow existing callers in internal/ui/
// to keep using unqualified names without any changes.

import (
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func statusDot(s string) string               { return uilayout.StatusDot(s) }
func truncate(s string, n int) string         { return uilayout.Truncate(s, n) }
func humanAge(t time.Time) string             { return uilayout.HumanAge(t) }
func clipLines(s string, n int) string        { return uilayout.ClipLines(s, n) }
func orgsToRows(orgs []sf.Org) []cache.OrgRow { return uilayout.OrgsToRows(orgs) }
func targetArg(o sf.Org) string               { return uilayout.TargetArg(o) }
func canUseOrg(o sf.Org) bool                 { return uilayout.CanUseOrg(o) }
