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
	// Subtabs that render static link pages or session-local state,
	// not org fetches: sharing rules is a Setup-link page; system-api
	// is the in-memory API call ring buffer (no fetch time to show).
	exemptSub := map[Subtab]bool{
		SubtabPermsSharingRules: true,
		SubtabSystemAPI:         true,
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
