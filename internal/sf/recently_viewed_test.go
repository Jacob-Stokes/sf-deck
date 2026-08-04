package sf

import (
	"strings"
	"testing"
)

func TestRecentlyViewedSOQLQueriesTargetSObjectLastViewedDate(t *testing.T) {
	got := recentlyViewedSOQL(RecentlyViewedOpts{SObject: "Request__c", Limit: 200})
	if !strings.Contains(got, "FROM Request__c WHERE LastViewedDate != NULL") {
		t.Fatalf("per-sObject query should filter target object by LastViewedDate, got %q", got)
	}
	if strings.Contains(got, "FROM RecentlyViewed") || strings.Contains(got, "WHERE Type =") {
		t.Fatalf("per-sObject query should not query RecentlyViewed by Type, got %q", got)
	}
	if !strings.Contains(got, "LIMIT 200") {
		t.Fatalf("per-sObject query did not preserve limit, got %q", got)
	}
}

// TestRecentlyViewedUniversalFiltersNoise: the cross-sObject recent
// query must exclude builder-internal / admin noise types
// (FlowRecordElement etc.) so they don't drown out real records.
func TestRecentlyViewedUniversalFiltersNoise(t *testing.T) {
	got := recentlyViewedSOQL(RecentlyViewedOpts{Limit: 50})
	if !strings.Contains(got, "FROM RecentlyViewed") {
		t.Fatalf("universal query should read RecentlyViewed, got %q", got)
	}
	if !strings.Contains(got, "WHERE Type NOT IN (") {
		t.Fatalf("universal query should exclude noise types, got %q", got)
	}
	// Spot-check the headline offenders are in the exclusion list.
	for _, noise := range []string{"FlowRecordElement", "OmniProcessElement", "FlowRecordVersion"} {
		if !strings.Contains(got, "'"+noise+"'") {
			t.Errorf("noise type %q should be excluded, got %q", noise, got)
		}
	}
	// Real categories must NOT be excluded (they have their own chips).
	for _, keep := range []string{"'ListView'", "'Report'", "'Account'"} {
		if strings.Contains(got, keep) {
			t.Errorf("%s should not be in the exclusion list", keep)
		}
	}
}
