package ui

// orggroups.go — left-rail grouping computation. Pure data shapes
// here; rendering lives in leftrail.go and the keyboard handlers in
// update_keys.go.
//
// The grouping is purely client-side decoration on top of the list
// of orgs that `sf` already knows about. Each authed org belongs to
// at most one user-defined group; orgs not explicitly assigned land
// under a synthetic "Ungrouped" section at the bottom of the rail.
//
// renderOrgsWidget walks orgRailRow values produced by buildRailRows,
// and the cursor (m.orgRailCursor) addresses the same flat list.

import (
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// orgRailRowKind identifies what a single rail row represents.
type orgRailRowKind int

const (
	railRowGroupHeader orgRailRowKind = iota // group label + count, possibly collapsed
	railRowOrg                               // a single org under a group (or under "Ungrouped")
)

// orgRailRow is one renderable line in the left-rail org panel. The
// rail's cursor (m.orgRailCursor) addresses this list directly so a
// single nav key handler can treat headers + orgs uniformly.
type orgRailRow struct {
	Kind    orgRailRowKind
	GroupID string // header: own id; org: containing group id (or "" for ungrouped)
	Org     sf.Org // valid when Kind == railRowOrg
	OrgIdx  int    // index into m.orgs when Kind == railRowOrg (else -1)
}

// ungroupedID is the synthetic group id for orgs the user hasn't
// assigned anywhere. Reserved so a user-created group can't collide.
const ungroupedID = "__ungrouped"

// buildRailRows flattens (groups, orgs) into the row list the rail
// renderer + cursor walk together. Always returns the synthetic
// "Ungrouped" header at the bottom when there are unassigned orgs;
// otherwise it's omitted (no point showing an empty section the
// user can't act on).
//
// Stale group members (usernames that aren't in `orgs`) are silently
// skipped here. The persistent prune happens elsewhere on refresh.
func buildRailRows(orgs []sf.Org, groups []settings.OrgGroupConfig) []orgRailRow {
	if len(orgs) == 0 {
		return nil
	}

	byUser := make(map[string]int, len(orgs))
	for i, o := range orgs {
		byUser[o.Username] = i
	}

	// Track which orgs got pulled into a group so we know what's left
	// for the Ungrouped section.
	taken := make(map[string]bool, len(orgs))
	var rows []orgRailRow

	for _, g := range groups {
		rows = append(rows, orgRailRow{
			Kind:    railRowGroupHeader,
			GroupID: g.ID,
			OrgIdx:  -1,
		})
		if g.Collapsed {
			// Still mark members as taken so they don't fall through
			// to Ungrouped.
			for _, m := range g.Members {
				if _, ok := byUser[m]; ok {
					taken[m] = true
				}
			}
			continue
		}
		for _, m := range g.Members {
			i, ok := byUser[m]
			if !ok {
				continue
			}
			taken[m] = true
			rows = append(rows, orgRailRow{
				Kind:    railRowOrg,
				GroupID: g.ID,
				Org:     orgs[i],
				OrgIdx:  i,
			})
		}
	}

	// Ungrouped section — only render the header when there are
	// any unassigned orgs (or when there are no user groups at all,
	// so the user sees their orgs even with zero config).
	hasUngrouped := false
	for _, o := range orgs {
		if !taken[o.Username] {
			hasUngrouped = true
			break
		}
	}
	if hasUngrouped {
		rows = append(rows, orgRailRow{
			Kind:    railRowGroupHeader,
			GroupID: ungroupedID,
			OrgIdx:  -1,
		})
		// When there are zero user groups we don't need a header at
		// all — the rail looks the same as it always did. Drop the
		// header in that case so the existing UX is preserved for
		// users who haven't opted in to groups.
		if len(groups) == 0 {
			rows = rows[:len(rows)-1]
		}
		for i, o := range orgs {
			if taken[o.Username] {
				continue
			}
			rows = append(rows, orgRailRow{
				Kind:    railRowOrg,
				GroupID: ungroupedID,
				Org:     o,
				OrgIdx:  i,
			})
		}
	}

	return rows
}

// findGroupByID returns a copy of the named group from the slice,
// plus its index. Returns (-1) when not found. Linear scan; the
// group list is short.
func findGroupByID(groups []settings.OrgGroupConfig, id string) (int, settings.OrgGroupConfig) {
	for i, g := range groups {
		if g.ID == id {
			return i, g
		}
	}
	return -1, settings.OrgGroupConfig{}
}

// groupHeaderLabel returns the human label for a group id. Handles
// the synthetic Ungrouped id.
func groupHeaderLabel(groups []settings.OrgGroupConfig, id string) string {
	if id == ungroupedID {
		return "Ungrouped"
	}
	if _, g := findGroupByID(groups, id); g.ID != "" {
		return g.Name
	}
	return id
}

// groupHeaderCollapsed reports whether a header row should render
// as collapsed. Synthetic Ungrouped is never collapsed.
func groupHeaderCollapsed(groups []settings.OrgGroupConfig, id string) bool {
	if id == ungroupedID {
		return false
	}
	if _, g := findGroupByID(groups, id); g.ID != "" {
		return g.Collapsed
	}
	return false
}

// groupMemberCount counts how many orgs from `orgs` fall under the
// group id, honouring the synthetic Ungrouped bucket.
func groupMemberCount(orgs []sf.Org, groups []settings.OrgGroupConfig, id string) int {
	if id == ungroupedID {
		taken := map[string]bool{}
		for _, g := range groups {
			for _, m := range g.Members {
				taken[m] = true
			}
		}
		n := 0
		for _, o := range orgs {
			if !taken[o.Username] {
				n++
			}
		}
		return n
	}
	_, g := findGroupByID(groups, id)
	if g.ID == "" {
		return 0
	}
	byUser := map[string]bool{}
	for _, o := range orgs {
		byUser[o.Username] = true
	}
	n := 0
	for _, m := range g.Members {
		if byUser[m] {
			n++
		}
	}
	return n
}

// slugifyGroupName produces a stable id from a user-supplied group
// name. Lowercase, alphanumerics + hyphens; collapses runs and trims
// leading/trailing hyphens. Falls back to "group" when the input is
// purely punctuation.
func slugifyGroupName(name string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		case r == ' ' || r == '-' || r == '_':
			if !prevHyphen && b.Len() > 0 {
				b.WriteRune('-')
				prevHyphen = true
			}
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if out == "" {
		return "group"
	}
	return out
}

// uniqueGroupID returns slugifyGroupName(name) suffixed with -2/-3/…
// when an existing group already uses that id. Reserved synthetic
// ids (ungroupedID) are also avoided.
func uniqueGroupID(name string, existing []settings.OrgGroupConfig) string {
	base := slugifyGroupName(name)
	used := map[string]bool{ungroupedID: true}
	for _, g := range existing {
		used[g.ID] = true
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		cand := base + "-" + itoa(i)
		if !used[cand] {
			return cand
		}
	}
}
