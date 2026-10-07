package ui

import (
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

func previewChip(id, label string) qchip.Chip {
	return qchip.Chip{ID: id, Label: label, Scope: "*", Origin: qchip.OriginUser}
}

func TestAddChipPreviewStoresAndDedupes(t *testing.T) {
	m := &Model{}
	m.addChipPreview(domainObjects, "*", previewChip("a", "Alpha"), "u@orig")

	if got := m.chipPreviewsFor(domainObjects, "*"); len(got) != 1 || got[0].Chip.ID != "a" {
		t.Fatalf("first add not recorded: %+v", got)
	}

	m.addChipPreview(domainObjects, "*", previewChip("a", "Alpha"), "u@orig")
	if got := m.chipPreviewsFor(domainObjects, "*"); len(got) != 1 {
		t.Errorf("duplicate add should be a no-op, got %d previews", len(got))
	}

	m.addChipPreview(domainObjects, "*", previewChip("b", "Bravo"), "u@orig")
	if got := m.chipPreviewsFor(domainObjects, "*"); len(got) != 2 {
		t.Errorf("second distinct add should accumulate, got %d", len(got))
	}

	if got := m.chipPreviewsFor(domainFlows, "*"); got != nil {
		t.Errorf("other slot should be empty, got %+v", got)
	}
}

func TestRemoveChipPreview(t *testing.T) {
	m := &Model{}
	m.addChipPreview(domainObjects, "*", previewChip("a", "Alpha"), "u@orig")
	m.addChipPreview(domainObjects, "*", previewChip("b", "Bravo"), "u@orig")

	m.removeChipPreview(domainObjects, "*", "a")
	got := m.chipPreviewsFor(domainObjects, "*")
	if len(got) != 1 || got[0].Chip.ID != "b" {
		t.Errorf("after removing 'a', expected only 'b': %+v", got)
	}

	m.removeChipPreview(domainObjects, "*", "b")
	if got := m.chipPreviewsFor(domainObjects, "*"); got != nil {
		t.Errorf("emptied slot should be nil, got %+v", got)
	}

	// Removing from a nil map is safe.
	(&Model{}).removeChipPreview(domainObjects, "*", "missing")
}

// TestStripRowsIncludesPreviews integrates with the strip-rows builder:
// previews should appear in the row list with the preview-kind sentinel
// and the "(from X)" tag in the label, so the renderer can style them.
func TestStripRowsIncludesPreviews(t *testing.T) {
	reg := qchip.NewRegistry(string(domainObjects), nil)
	m := Model{
		modelServices: modelServices{settings: &settings.Settings{}},
		modelOrgs: modelOrgs{
			orgs: []sf.Org{{Username: "u@orig", Alias: "prod"}},
		},
		modelChips: modelChips{
			chipRegistries: map[chipDomain]*qchip.Registry{domainObjects: reg},
		},
	}
	(&m).addChipPreview(domainObjects, "*", previewChip("a", "Alpha"), "u@orig")

	got := m.stripRows(domainObjects, "*")
	var found *chipRow
	for i := range got {
		if got[i].ID == "a" {
			found = &got[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("preview row not in strip rows: %+v", got)
	}
	if found.Count != chipRowKindPreview {
		t.Errorf("preview row should carry chipRowKindPreview sentinel, got %d", found.Count)
	}
	if !strings.Contains(found.Label, "(from prod)") {
		t.Errorf("preview label should include origin org tag, got %q", found.Label)
	}
}
