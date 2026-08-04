package ui

// Registry completeness tests. These walk the TabSpec registry and
// the listSurface declarations to assert basic structural invariants
// — the kind of "did I forget to set Field X on the new entry I just
// added" mistakes that would otherwise show up as a runtime nil-deref
// or as silent "this gesture does nothing on this tab."
//
// What's enforced here:
//
//   1. Every registered Tab has a Renderer (or its Subtabs all do).
//   2. Every Subtabs entry has a non-empty Label (used in the tab
//      strip; empty Label = invisible subtab).
//   3. Every tab declaring Subtabs also declares GetSubtabIdx +
//      SetSubtabIdx (the subtab cursor accessors).
//   4. Every listSurface that sets BuildRenderModel returns a model
//      with non-nil State + Search + Cols + Cell when called against
//      an empty orgData. This catches regressions where a new
//      surface forgets one of those — the shared renderer would
//      crash at first frame.
//
// These tests don't replace the smoke tests (which exercise actual
// rendering); they're cheaper structural checks that fail loudly at
// "go test" time when the registry shape drifts.

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
)

func TestTabRegistryCompleteness(t *testing.T) {
	specs := tabSpecs()
	for tab, spec := range specs {
		tab := tab
		spec := spec
		t.Run(tab.String(), func(t *testing.T) {
			// (1) Renderer required at the tab level OR on every subtab.
			if spec.Renderer == nil {
				if len(spec.Subtabs) == 0 {
					t.Errorf("tab %v has no Renderer and no Subtabs — render dispatch will crash", tab)
				} else {
					for i, sub := range spec.Subtabs {
						if sub.Renderer == nil {
							t.Errorf("tab %v subtab %d (%s) has no Renderer (parent has none either)",
								tab, i, sub.ID)
						}
					}
				}
			}

			// (2) Every Subtabs entry needs a non-empty Label.
			for i, sub := range spec.Subtabs {
				if sub.Label == "" {
					t.Errorf("tab %v subtab %d (%s) has empty Label — would be invisible in tab strip",
						tab, i, sub.ID)
				}
			}

			// (3) Subtab cursor accessors required when Subtabs non-empty.
			if len(spec.Subtabs) > 0 {
				if spec.GetSubtabIdx == nil {
					t.Errorf("tab %v has Subtabs but no GetSubtabIdx — subtab nav broken", tab)
				}
				if spec.SetSubtabIdx == nil {
					t.Errorf("tab %v has Subtabs but no SetSubtabIdx — subtab nav broken", tab)
				}
			}
		})
	}
}

// TestListSurfaceBuildRenderModelShape walks every listSurface that
// declares BuildRenderModel and asserts the produced model has the
// fields the shared renderer requires. Uses a fresh empty orgData
// because every surface should handle "no data yet" gracefully — if
// it can't, the smoke test would catch that, but this gives a
// faster, more targeted failure mode.
func TestListSurfaceBuildRenderModelShape(t *testing.T) {
	c, err := cache.Open()
	if err != nil {
		t.Fatalf("cache open: %v", err)
	}
	defer c.Close()
	m := New(c)
	d := m.ensureOrgData("registry-completeness@test.local")

	specs := tabSpecs()
	checked := 0
	for tab, spec := range specs {
		tab := tab
		check := func(name string, surf *listSurface) {
			if surf == nil || surf.BuildRenderModel == nil {
				return
			}
			checked++
			t.Run(name, func(t *testing.T) {
				model, ok := surf.BuildRenderModel(m, d)
				if !ok {
					// Surface declined to produce a model for empty
					// data. That's allowed — but if it returned ok=true,
					// the rest of the asserts apply.
					return
				}
				if model.State == nil {
					t.Errorf("%s: model.State is nil", name)
				}
				if model.Search == nil {
					t.Errorf("%s: model.Search is nil", name)
				}
				if len(model.Cols) == 0 {
					t.Errorf("%s: model.Cols is empty", name)
				}
				if model.Cell == nil {
					t.Errorf("%s: model.Cell is nil", name)
				}
			})
		}
		check(tab.String()+".List", spec.List)
		for i, sub := range spec.Subtabs {
			_ = i
			check(tab.String()+"."+string(sub.ID)+".List", sub.List)
		}
	}
	if checked == 0 {
		t.Fatal("no listSurface entries with BuildRenderModel found — registry empty?")
	}
}

// TestListOpenSurfacesHaveIdentity enforces the identity-coverage
// invariant: any surface that is both a List (a browsable table of
// resources) AND Openable (its rows map to a Lightning URL) must
// declare an Identity resolver — the single hook that makes its rows
// taggable, collectable, movable, and yankable.
//
// The failure mode this guards against is subtle: a List+Open surface
// with no Identity renders and opens fine, so it looks complete, but
// its rows silently can't be tagged / collected / found-in-another-org
// / value-yanked. The whole point of the Identity seam is that these
// gestures light up from ONE hook; forgetting it means a surface is
// quietly half-wired. (This is exactly how the /meta subtabs shipped
// non-collectable.)
//
// Opt out deliberately by setting NoCollectReason on the surface —
// then the row is documented as intentionally inert, and this test
// treats it as covered.
func TestListOpenSurfacesHaveIdentity(t *testing.T) {
	specs := tabSpecs()
	checked := 0
	for tab, spec := range specs {
		// Tab-level List+Open surface (tabs without subtabs, e.g.
		// /flows, /packages). A parent Identity or NoCollectReason
		// satisfies it.
		if spec.List != nil && spec.Open != nil && len(spec.Subtabs) == 0 {
			checked++
			if spec.Identity == nil && spec.NoCollectReason == "" {
				t.Errorf("tab %v is List+Open but has no Identity resolver and no "+
					"NoCollectReason — its rows can't be tagged/collected/moved/"+
					"yanked. Add an Identity closure, or set NoCollectReason to "+
					"document the opt-out.", tab)
			}
		}

		// Subtab-level List+Open surfaces. Identity may live on the
		// subtab OR be inherited from the parent tab; either counts.
		for _, sub := range spec.Subtabs {
			if sub.List == nil || sub.Open == nil {
				continue
			}
			checked++
			hasIdentity := sub.Identity != nil || spec.Identity != nil
			hasOptOut := sub.NoCollectReason != "" || spec.NoCollectReason != ""
			if !hasIdentity && !hasOptOut {
				t.Errorf("tab %v subtab %q is List+Open but neither it nor its "+
					"parent declares an Identity resolver, and no NoCollectReason "+
					"is set — its rows can't be tagged/collected/moved/yanked. "+
					"Add Identity, or set NoCollectReason to document the opt-out.",
					tab, sub.ID)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no List+Open surfaces found — registry empty or shape changed?")
	}
}
