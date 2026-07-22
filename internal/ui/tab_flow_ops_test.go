package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// TestFlowVersionIsActive guards the delete gate: the active version
// must be recognised (and thus refused) whether it's identified by the
// header's ActiveVersionID or only by the version's own status.
func TestFlowVersionIsActive(t *testing.T) {
	header := sf.Flow{ActiveVersionID: "301ACTIVE"}

	cases := []struct {
		name string
		v    sf.FlowVersion
		want bool
	}{
		{"matches active id", sf.FlowVersion{ID: "301ACTIVE", Status: "Active"}, true},
		{"active by status only", sf.FlowVersion{ID: "301OTHER", Status: "Active"}, true},
		{"inactive draft", sf.FlowVersion{ID: "301DRAFT", Status: "Draft"}, false},
		{"inactive obsolete", sf.FlowVersion{ID: "301OLD", Status: "Obsolete"}, false},
		{"empty id, non-active", sf.FlowVersion{ID: "", Status: "Draft"}, false},
	}
	for _, c := range cases {
		if got := flowVersionIsActive(c.v, header); got != c.want {
			t.Errorf("%s: flowVersionIsActive = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestFlowVersionIsActiveNoActiveHeader covers a flow with no active
// version (header.ActiveVersionID empty): only a version whose own
// status is "Active" counts, and an empty id never spuriously matches
// the empty header id.
func TestFlowVersionIsActiveNoActiveHeader(t *testing.T) {
	header := sf.Flow{} // no active version
	if flowVersionIsActive(sf.FlowVersion{ID: "", Status: "Draft"}, header) {
		t.Error("empty version id must not match empty header ActiveVersionID")
	}
	if !flowVersionIsActive(sf.FlowVersion{ID: "301X", Status: "Active"}, header) {
		t.Error("a version with Active status should still be treated as active")
	}
}
