package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

// modelWithOrgs builds a minimal Model carrying the orgs we want and a
// real settings.Settings (so OrgGroups() works in the share-summary
// helper). Avoids touching anything else on Model.
func modelWithOrgs(orgs []sf.Org, groups []settings.OrgGroupConfig) Model {
	s := &settings.Settings{}
	if groups != nil {
		s.SetOrgGroups(groups)
	}
	return Model{
		modelServices: modelServices{settings: s},
		modelOrgs:     modelOrgs{orgs: orgs},
	}
}

func TestChipWizardShareSummary(t *testing.T) {
	orgs := []sf.Org{
		{Username: "u@a", Alias: "alpha"},
		{Username: "u@b", Alias: "bravo"},
		{Username: "u@c"}, // no alias on this one
	}
	m := modelWithOrgs(orgs, []settings.OrgGroupConfig{
		{ID: "g1", Name: "Team One", Members: []string{"u@a", "u@b"}},
	})

	cases := []struct {
		name  string
		share settings.ChipShare
		want  string
	}{
		{"zero share prompts user", settings.ChipShare{}, "(no scope yet — press S)"},
		{"global is explicit",
			settings.ChipShare{Kind: settings.ChipShareGlobal},
			"global (every org)"},
		{"single org uses alias",
			settings.ChipShare{Kind: settings.ChipShareOrg, Orgs: []string{"u@a"}},
			"alpha"},
		{"single org falls back to username",
			settings.ChipShare{Kind: settings.ChipShareOrg, Orgs: []string{"u@c"}},
			"u@c"},
		{"two orgs lists them",
			settings.ChipShare{Kind: settings.ChipShareOrgs, Orgs: []string{"u@a", "u@b"}},
			"alpha, bravo"},
		{"many orgs collapse to count",
			settings.ChipShare{Kind: settings.ChipShareOrgs, Orgs: []string{"u@a", "u@b", "u@c", "u@d"}},
			"4 orgs"},
		{"group resolves to its display name",
			settings.ChipShare{Kind: settings.ChipShareGroup, Group: "g1"},
			"group · Team One"},
		{"unknown group id falls back to id",
			settings.ChipShare{Kind: settings.ChipShareGroup, Group: "missing"},
			"group · missing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := chipWizardShareSummary(m, c.share)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestChipWizardInitialShareNewChipStampsActiveOrg(t *testing.T) {
	m := modelWithOrgs([]sf.Org{{Username: "u@a", Alias: "alpha"}}, nil)
	mp := &m
	got := chipWizardInitialShare(mp, qchip.Chip{}) // existingID == ""
	if got.Kind != settings.ChipShareOrg || len(got.Orgs) != 1 || got.Orgs[0] != "u@a" {
		t.Errorf("new chip should stamp active org, got %+v", got)
	}
}

func TestChipWizardInitialShareNewChipWithoutActiveOrgIsZero(t *testing.T) {
	m := modelWithOrgs(nil, nil)
	mp := &m
	got := chipWizardInitialShare(mp, qchip.Chip{})
	if !got.IsZero() {
		t.Errorf("new chip with no active org should be zero (save guard catches), got %+v", got)
	}
}

func TestChipWizardInitialShareEditUsesExistingShare(t *testing.T) {
	m := modelWithOrgs([]sf.Org{{Username: "u@a"}}, nil)
	mp := &m
	existing := qchip.Chip{
		ID:    "x",
		Share: settings.ChipShare{Kind: settings.ChipShareGlobal},
	}
	got := chipWizardInitialShare(mp, existing)
	if got.Kind != settings.ChipShareGlobal {
		t.Errorf("editing existing chip should preserve its share, got %+v", got)
	}
}

func TestChipWizardInitialShareEditMigratesLegacyOrgUser(t *testing.T) {
	m := modelWithOrgs(nil, nil)
	mp := &m
	existing := qchip.Chip{
		ID:      "x",
		OrgUser: "legacy@org", // pre-Share chip
	}
	got := chipWizardInitialShare(mp, existing)
	if got.Kind != settings.ChipShareOrg || len(got.Orgs) != 1 || got.Orgs[0] != "legacy@org" {
		t.Errorf("legacy OrgUser-only chip should seed as single-org share, got %+v", got)
	}
}

// Sanity: the rendered summary string never includes the literal "ChipShare"
// type name (guard against accidental %+v of the struct).
func TestChipWizardShareSummaryNeverLeaksTypeName(t *testing.T) {
	m := modelWithOrgs(nil, nil)
	got := chipWizardShareSummary(m, settings.ChipShare{Kind: settings.ChipShareGlobal})
	if strings.Contains(got, "ChipShare") {
		t.Errorf("summary leaked Go type name: %q", got)
	}
}

func TestChipWizardShareDetailLines(t *testing.T) {
	orgs := []sf.Org{
		{Username: "u@a", Alias: "alpha"},
		{Username: "u@b", Alias: "bravo"},
		{Username: "u@c", Alias: "charlie"},
		{Username: "u@d", Alias: "delta"},
		{Username: "u@e", Alias: "echo"},
	}
	groups := []settings.OrgGroupConfig{
		{ID: "g1", Name: "Team", Members: []string{"u@a", "u@b"}},
		{ID: "empty", Name: "Empty"},
	}
	m := modelWithOrgs(orgs, groups)

	cases := []struct {
		name  string
		share settings.ChipShare
		want  []string
	}{
		{"single-org has no detail", settings.ChipShare{Kind: settings.ChipShareOrg, Orgs: []string{"u@a"}}, nil},
		{"global has no detail", settings.ChipShare{Kind: settings.ChipShareGlobal}, nil},
		{"3 orgs still fit in summary line", settings.ChipShare{Kind: settings.ChipShareOrgs, Orgs: []string{"u@a", "u@b", "u@c"}}, nil},
		{"4 orgs trigger per-org bullets", settings.ChipShare{
			Kind: settings.ChipShareOrgs, Orgs: []string{"u@a", "u@b", "u@c", "u@d"},
		}, []string{"  · alpha", "  · bravo", "  · charlie", "  · delta"}},
		{"group enumerates its members", settings.ChipShare{Kind: settings.ChipShareGroup, Group: "g1"},
			[]string{"  · alpha", "  · bravo"}},
		{"empty group says so", settings.ChipShare{Kind: settings.ChipShareGroup, Group: "empty"},
			[]string{"  (group has no members)"}},
		{"unknown group id flags it", settings.ChipShare{Kind: settings.ChipShareGroup, Group: "missing"},
			[]string{"  (group not found — pick another scope)"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := chipWizardShareDetailLines(m, c.share)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestChipDisplayLabel(t *testing.T) {
	cases := []struct {
		name string
		chip qchip.Chip
		want string
	}{
		{"builtin private", qchip.Chip{Label: "Recent", Origin: qchip.OriginBuiltIn}, "Recent"},
		{"user private", qchip.Chip{Label: "MyView", Origin: qchip.OriginUser,
			Share: settings.ChipShare{Kind: settings.ChipShareOrg, Orgs: []string{"u@a"}}}, "· MyView"},
		{"imported private", qchip.Chip{Label: "FromSF", Origin: qchip.OriginImported,
			Share: settings.ChipShare{Kind: settings.ChipShareOrg, Orgs: []string{"u@a"}}}, "↓ FromSF"},
		{"user global gets ⇄", qchip.Chip{Label: "Everywhere", Origin: qchip.OriginUser,
			Share: settings.ChipShare{Kind: settings.ChipShareGlobal}}, "· ⇄ Everywhere"},
		{"user group gets ⇄", qchip.Chip{Label: "TeamView", Origin: qchip.OriginUser,
			Share: settings.ChipShare{Kind: settings.ChipShareGroup, Group: "g1"}}, "· ⇄ TeamView"},
		{"user multi-org gets ⇄", qchip.Chip{Label: "Pair", Origin: qchip.OriginUser,
			Share: settings.ChipShare{Kind: settings.ChipShareOrgs, Orgs: []string{"u@a", "u@b"}}}, "· ⇄ Pair"},
		{"user orgs with single entry does NOT get ⇄", qchip.Chip{Label: "Solo", Origin: qchip.OriginUser,
			Share: settings.ChipShare{Kind: settings.ChipShareOrgs, Orgs: []string{"u@a"}}}, "· Solo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := chipDisplayLabel(c.chip); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}
