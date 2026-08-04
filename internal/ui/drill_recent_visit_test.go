package ui

// TestEveryDrillTabDecidesOnRecentVisits is the ratchet for the
// LWC-chip bug class (2026-06-12): TabLWCDetail shipped without a
// RecordRecentVisit hook, so Enter-drilling never fed the
// Recently-viewed chip — only `o` did, and nothing caught it.
//
// Every drill tab (Stem != Tab) must now make an EXPLICIT decision:
// either declare RecordRecentVisit, or appear in the exemption list
// below with a written reason. A new drill tab that does neither
// fails here, forcing the author to think about recency instead of
// silently inheriting the gap.
import "testing"

// drillRecentVisitExemptions lists drill tabs that deliberately do
// NOT record recent visits, with the reason. Add here only with a
// justification — "forgot" is the bug this test exists to catch.
var drillRecentVisitExemptions = map[Tab]string{
	// Per-row child entities of an object: the OBJECT visit is
	// recorded by TabObjectDetail; recording every rule/RT/trigger
	// poked at would drown the recent stream in noise.
	TabValidationDetail: "child of object visit; too granular for recents",
	TabRecordTypeDetail: "child of object visit; too granular for recents",
	TabTriggerDetail:    "child of object visit; too granular for recents",
	// Flow-version viewer: a sub-drill of the flow (which TabFlowDetail
	// records); recording every version definition opened would be noise.
	TabFlowVersionDetail: "child of flow visit; per-version, too granular for recents",
	// Transient operational drills — inspecting a deploy or a
	// metadata type list isn't a destination users navigate back to
	// via recents (and neither kind has a RecentKind).
	TabDeployDetail:   "operational inspection; no RecentKind exists",
	TabMetaTypeDetail: "type catalogue browse; no RecentKind exists",
	// Group-membership drills: queues/public groups are recorded at
	// the LIST level via the perms recent machinery, not per-drill.
	TabQueueDetail:       "membership inspection; queue visits recorded via o-path",
	TabPublicGroupDetail: "membership inspection; group visits recorded via o-path",
	// Dev-project detail: project navigation, not org-entity
	// recency — projects have their own touched-at ordering.
	TabDevProjectDetail: "project context; DevProjects carry their own recency",
	// Tag detail: tag navigation, not org-entity recency — tags
	// are the user's own annotation layer and don't fit RecentKind
	// (a tag isn't an org item with a stable RecentKind shape).
	TabTagDetail: "tag context; tags are an annotation layer, not an org entity",
	// User-session drill: live runtime state (sessions come and go),
	// not an org entity with a stable RecentKind — recording "recently
	// viewed session" is meaningless.
	TabUserSessions: "live session inspection; a session is transient runtime state, not a recentable org entity",
	// Community-detail drill: lists a community's pages (best-effort
	// FlexiPages), not a single recentable org entity.
	TabCommunityDetail: "community pages drill; a page list isn't a single recentable org entity",
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
