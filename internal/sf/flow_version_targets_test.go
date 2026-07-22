package sf

import (
	"strings"
	"testing"
)

// TestFlowVersionTargetsIncludeViewDefinition pins that a flow version
// exposes the in-app "View definition" target (an in-terminal drill,
// no Path) alongside the browser targets — so the ctrl+o open menu can
// offer the definition viewer. The UI's fireMenuTarget matches on the
// exported ID, so a rename here without updating the intercept would
// silently break the menu path.
func TestFlowVersionTargetsIncludeViewDefinition(t *testing.T) {
	v := FlowVersion{ID: "301X", DefinitionID: "300X", MasterLabel: "My Flow", VersionNumber: 3}
	targets := v.Targets()

	var view *OpenTarget
	for i := range targets {
		if targets[i].ID == FlowVersionViewDefinitionTargetID {
			view = &targets[i]
			break
		}
	}
	if view == nil {
		t.Fatalf("Targets() missing %q; got %+v", FlowVersionViewDefinitionTargetID, targets)
	}
	// It's an in-app action: no URL to open. fireMenuTarget relies on
	// the empty Path/AbsoluteURL to route it to the drill.
	if view.Path != "" || view.AbsoluteURL != "" {
		t.Errorf("view-definition target must have no URL (in-app drill); Path=%q AbsoluteURL=%q", view.Path, view.AbsoluteURL)
	}

	// Browser targets still present.
	if !hasTargetID(targets, "builder") {
		t.Error("Flow Builder target missing")
	}
}

// TestFlowVersionNoIDNoViewDefinition: a version with no Id can't be
// viewed (nothing to fetch), so the target is omitted.
func TestFlowVersionNoIDNoViewDefinition(t *testing.T) {
	v := FlowVersion{DefinitionID: "300X"}
	if hasTargetID(v.Targets(), FlowVersionViewDefinitionTargetID) {
		t.Error("view-definition target should be omitted when the version has no Id")
	}
}

func hasTargetID(ts []OpenTarget, id string) bool {
	for _, t := range ts {
		if t.ID == id {
			return true
		}
	}
	return false
}

// The flows-list `o` must open the LATEST version (Setup parity: the
// draft is what you'd edit), with the active version as a secondary
// target only when it differs. It used to open the active version
// first — editing a stale version whenever a newer draft existed.
func TestFlowTargetsLatestFirstActiveSecondary(t *testing.T) {
	f := Flow{
		DefinitionID:    "300x",
		ActiveVersionID: "301A", ActiveVersionNum: 8,
		LatestVersionID: "301L", LatestVersionNum: 9,
	}
	ts := f.Targets()
	if len(ts) < 2 {
		t.Fatalf("want >=2 targets, got %d", len(ts))
	}
	if ts[0].ID != "builder" || !strings.Contains(ts[0].Path, "301L") {
		t.Errorf("primary must be the LATEST version: %+v", ts[0])
	}
	if ts[1].ID != "builder-active" || !strings.Contains(ts[1].Path, "301A") {
		t.Errorf("secondary must be the ACTIVE version: %+v", ts[1])
	}

	// Same version active+latest: one builder target, no duplicate.
	same := Flow{DefinitionID: "300x", ActiveVersionID: "301A", LatestVersionID: "301A", ActiveVersionNum: 3, LatestVersionNum: 3}
	for _, tgt := range same.Targets() {
		if tgt.ID == "builder-active" {
			t.Error("no secondary target when active == latest")
		}
	}

	// Never-activated flow (draft only): latest opens, no active entry.
	draft := Flow{DefinitionID: "300x", LatestVersionID: "301L", LatestVersionNum: 1}
	ts = draft.Targets()
	if ts[0].ID != "builder" || !strings.Contains(ts[0].Path, "301L") {
		t.Errorf("draft-only flow must open its latest version: %+v", ts[0])
	}
}

// The flow-open ordering is a user setting ([ui.extensions]
// flow_open_version, pushed down via ApplyConfig). "active" flips the
// primary to the running version, with the newer draft demoted to a
// secondary target; flows with no active version still open their
// latest.
func TestFlowTargetsActivePreference(t *testing.T) {
	ApplyConfig(Config{FlowOpenVersion: "active"})
	defer ApplyConfig(Config{}) // restore the latest-first default

	f := Flow{
		DefinitionID:    "300x",
		ActiveVersionID: "301A", ActiveVersionNum: 8,
		LatestVersionID: "301L", LatestVersionNum: 9,
	}
	ts := f.Targets()
	if len(ts) < 2 {
		t.Fatalf("want >=2 targets, got %d", len(ts))
	}
	if ts[0].ID != "builder" || !strings.Contains(ts[0].Path, "301A") {
		t.Errorf("active mode: primary must be the ACTIVE version: %+v", ts[0])
	}
	if ts[1].ID != "builder-latest" || !strings.Contains(ts[1].Path, "301L") {
		t.Errorf("active mode: secondary must be the LATEST version: %+v", ts[1])
	}

	// Never-activated flow: no active version to open — fall back to
	// the latest so `o` still works.
	draft := Flow{DefinitionID: "300x", LatestVersionID: "301L", LatestVersionNum: 1}
	ts = draft.Targets()
	if ts[0].ID != "builder" || !strings.Contains(ts[0].Path, "301L") {
		t.Errorf("active mode: draft-only flow must open its latest version: %+v", ts[0])
	}

	// Same version active+latest: one builder target, no duplicate.
	same := Flow{DefinitionID: "300x", ActiveVersionID: "301A", LatestVersionID: "301A", ActiveVersionNum: 3, LatestVersionNum: 3}
	for _, tgt := range same.Targets() {
		if tgt.ID == "builder-latest" {
			t.Error("active mode: no secondary target when active == latest")
		}
	}
}
