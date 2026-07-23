package ui

import "testing"

// Every org-data-backed tab must surface the header "refreshed Xm ago"
// stamp via PrimaryFetchedAt — either on the TabSpec or on every one
// of its org-backed subtabs. Local-store tabs (dev projects, tags),
// ad-hoc surfaces (SOQL, exec editor), /compare (own staleness
// banner), and the static /setup nav are exempt.
func TestHeaderFreshnessCoverage(t *testing.T) {
	exempt := map[Tab]bool{
		TabSOQL: true, TabExec: true, TabCompare: true, TabSetup: true,
		TabDevProjects: true, TabDevProjectDetail: true, TabBundleDetail: true,
		TabTags: true, TabTagDetail: true, TabProjects: true,
	}
	// Subtabs that render session-local state rather than org fetches.
	exemptSub := map[Subtab]bool{
		SubtabSystemAPI: true,
	}
	for tab, spec := range tabSpecs() {
		if exempt[tab] {
			continue
		}
		if spec.PrimaryFetchedAt != nil {
			continue
		}
		if len(spec.Subtabs) == 0 {
			t.Errorf("%s: no PrimaryFetchedAt — header can't show data age", tab)
			continue
		}
		for _, sub := range spec.Subtabs {
			if exemptSub[sub.ID] {
				continue
			}
			if sub.PrimaryFetchedAt == nil {
				t.Errorf("%s/%s: no PrimaryFetchedAt — header can't show data age", tab, sub.ID)
			}
		}
	}
}
