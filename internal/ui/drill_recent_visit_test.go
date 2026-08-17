package ui

// TestEveryDrillTabDecidesOnRecentVisits requires every drill tab
// (Stem != Tab) to make an explicit decision:
// either declare RecordRecentVisit, or appear in the exemption list
// below with a written reason. A new drill tab that does neither
// fails here, forcing the author to think about recency instead of
// silently inheriting the gap.
import "testing"

// drillRecentVisitExemptions lists drill tabs that deliberately do
// NOT record recent visits, with the reason. Add here only with a
// justification.
var drillRecentVisitExemptions = map[Tab]string{
	TabValidationDetail:  "child of object visit; too granular for recents",
	TabRecordTypeDetail:  "child of object visit; too granular for recents",
	TabTriggerDetail:     "child of object visit; too granular for recents",
	TabFlowVersionDetail: "child of flow visit; per-version, too granular for recents",
	TabDeployDetail:      "operational inspection; no RecentKind exists",
	TabMetaTypeDetail:    "type catalogue browse; no RecentKind exists",
	TabQueueDetail:       "membership inspection; queue visits recorded via o-path",
	TabPublicGroupDetail: "membership inspection; group visits recorded via o-path",
	TabDevProjectDetail:  "project context; DevProjects carry their own recency",
	TabTagDetail:         "tag context; tags are an annotation layer, not an org entity",
	TabUserSessions:      "live session inspection; a session is transient runtime state, not a recentable org entity",
	TabCommunityDetail:   "community pages drill; a page list isn't a single recentable org entity",
}

func TestEveryDrillTabDecidesOnRecentVisits(t *testing.T) {
	specs := tabSpecs()
	for tab, spec := range specs {
		if spec.Stem == tab {
			continue // top-level, not a drill
		}
		hasHook := spec.RecordRecentVisit != nil
		_, exempt := drillRecentVisitExemptions[tab]
		switch {
		case hasHook && exempt:
			t.Errorf("/%s declares RecordRecentVisit AND is exempted — remove it from the exemption list", tab)
		case !hasHook && !exempt:
			t.Errorf("/%s is a drill tab with no RecordRecentVisit and no exemption — decide: wire the hook (see recentVisitLWCDetail) or exempt it with a reason", tab)
		}
	}
}
