package ui

import "testing"

func TestProvidersForMethod(t *testing.T) {
	tooling := providersForMethod(compareMethodTooling)
	if len(tooling) != 2 {
		t.Errorf("Tooling route = %d providers, want 2 (Apex + Trigger)", len(tooling))
	}
	for _, p := range tooling {
		if _, ok := toolingCompareTypes[p.TypeLabel()]; !ok {
			t.Errorf("Tooling route included non-Tooling type %q", p.TypeLabel())
		}
	}

	mdapi := providersForMethod(compareMethodMetadataAPI)
	if len(mdapi) != len(mdapiCompareTypes) {
		t.Errorf("Metadata API route = %d providers, want %d", len(mdapi), len(mdapiCompareTypes))
	}

	auto := providersForMethod(compareMethodAuto)
	labels := map[string]int{}
	for _, p := range auto {
		labels[p.TypeLabel()]++
	}
	for l, n := range labels {
		if n != 1 {
			t.Errorf("Auto route has %d providers for %q, want 1 (no dupes)", n, l)
		}
	}
	if labels["ApexClass"] == 0 || labels["CustomField"] == 0 {
		t.Errorf("Auto route missing expected types: %v", labels)
	}
}

func TestEstimateCompareCalls(t *testing.T) {
	if got := estimateCompareCalls(compareMethodTooling, []string{"ApexClass", "ApexTrigger"}); got != 2 {
		t.Errorf("tooling estimate = %d, want 2", got)
	}
	if got := estimateCompareCalls(compareMethodMetadataAPI, []string{"Flow", "Layout", "Profile"}); got != 6 {
		t.Errorf("mdapi 3-type estimate = %d, want 6", got)
	}
	if got := estimateCompareCalls(compareMethodMetadataAPI, []string{"Flow"}); got != 2 {
		t.Errorf("mdapi 1-type estimate = %d, want 2", got)
	}
	if got := estimateCompareCalls(compareMethodAuto, []string{"ApexClass", "CustomField"}); got != 7 {
		t.Errorf("auto apex+objchild estimate = %d, want 7", got)
	}
}

func TestProvidersForScopeMethodDoesNotFallbackOnUnsupportedScope(t *testing.T) {
	if got := providersForScopeMethod([]string{"Flow"}, compareMethodTooling); len(got) != 0 {
		t.Fatalf("Tooling+Flow providers = %d, want 0", len(got))
	}
	plan := buildComparePlan([]string{"Flow"}, compareMethodTooling)
	if err := plan.validate(); err == nil {
		t.Fatal("Tooling+Flow plan should be invalid")
	}
}

func TestParseCompareMethodRoundTrip(t *testing.T) {
	for _, cm := range []compareMethod{compareMethodAuto, compareMethodTooling, compareMethodMetadataAPI} {
		if got := parseCompareMethod(cm.String()); got != cm {
			t.Errorf("round-trip %v → %q → %v", cm, cm.String(), got)
		}
	}
	if parseCompareMethod("garbage") != compareMethodAuto {
		t.Error("unknown method should default to Auto")
	}
}

func TestEndpointHelpers(t *testing.T) {
	e := orgEndpoint("alice@x")
	if e.IsZero() || e.OrgRef() != "alice@x" {
		t.Errorf("orgEndpoint wrong: %+v", e)
	}
	if (endpoint{}).OrgRef() != "" || !(endpoint{}).IsZero() {
		t.Error("zero endpoint should be zero with empty OrgRef")
	}
	proj := endpoint{Kind: endpointProject, Ref: "p1"}
	if proj.OrgRef() != "" {
		t.Error("project endpoint OrgRef should be empty")
	}
	if !e.Equal(orgEndpoint("alice@x")) || e.Equal(proj) {
		t.Error("endpoint Equal wrong")
	}
}

func TestEstimateCompareCallsScalesWithScope(t *testing.T) {
	// The estimate must grow with scope (regression guard against it
	// silently going flat like the old "~8 regardless" bug).
	small := estimateCompareCalls(compareMethodAuto, []string{"Flow", "Layout"})
	var big []string
	for i := 0; i < 280; i++ {
		big = append(big, "Type"+itoa(i))
	}
	bigEst := estimateCompareCalls(compareMethodAuto, big)
	if bigEst <= small {
		t.Errorf("280-type estimate (%d) should exceed 2-type estimate (%d)", bigEst, small)
	}
	if bigEst < 280 {
		t.Errorf("280 SOAP types should estimate >= ~280 (>=1 retrieve each), got %d", bigEst)
	}
	apexOnly := estimateCompareCalls(compareMethodAuto, []string{"ApexClass", "ApexTrigger"})
	if apexOnly < 1 {
		t.Errorf("apex-only estimate should be >=1, got %d", apexOnly)
	}
}
