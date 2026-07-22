package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

func TestDispatchOtherOrgPreviewAddsStripRow(t *testing.T) {
	st := makeSettings(t, []settings.ChipConfig{
		{
			ID:     "phd-journals",
			Label:  "Phd Journals",
			Domain: string(domainFlows),
			Scope:  "*",
			Share: settings.ChipShare{
				Kind: settings.ChipShareOrg,
				Orgs: []string{"u@other"},
			},
		},
	}, nil)
	reg := qchip.NewRegistry(string(domainFlows), nil)
	reg.SetActiveOrg("u@active")

	m := Model{
		modelServices: modelServices{settings: st},
		modelOrgs: modelOrgs{
			orgs:     []sf.Org{{Username: "u@active"}, {Username: "u@other", Alias: "other"}},
			selected: 0,
			data:     map[string]*orgData{},
		},
		modelChips: modelChips{chipRegistries: map[chipDomain]*qchip.Registry{domainFlows: reg}},
	}

	(&m).dispatchChipManagerAction(domainFlows, "*", "otherpreview:phd-journals")

	if got := m.chipPreviewsFor(domainFlows, "*"); len(got) != 1 || got[0].Chip.ID != "phd-journals" {
		t.Fatalf("preview dispatch did not store preview: %+v", got)
	}
	rows := m.stripRows(domainFlows, "*")
	var found *chipRow
	for i := range rows {
		if rows[i].ID == "phd-journals" {
			found = &rows[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("preview dispatch did not add a strip row: %+v", rows)
	}
	if found.Count != chipRowKindPreview {
		t.Fatalf("preview strip row kind = %d, want %d", found.Count, chipRowKindPreview)
	}
	if !strings.Contains(found.Label, "(from other)") {
		t.Fatalf("preview strip row label did not include origin alias: %q", found.Label)
	}
}

func TestApplyChipScopeChosenOtherOrgPersistsGlobalShare(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	st := &settings.Settings{}
	st.UI.Chips = []settings.ChipConfig{
		{
			ID:      "phd-journals",
			Label:   "Phd Journals",
			Domain:  string(domainFlows),
			Scope:   "*",
			OrgUser: "u@other",
		},
	}
	reg := qchip.NewRegistry(string(domainFlows), nil)
	reg.SetActiveOrg("u@active")
	reg.LoadFromSettings(st)

	m := Model{
		modelServices: modelServices{settings: st},
		modelOrgs: modelOrgs{
			orgs:     []sf.Org{{Username: "u@active"}, {Username: "u@other"}},
			selected: 0,
		},
		modelChips: modelChips{chipRegistries: map[chipDomain]*qchip.Registry{domainFlows: reg}},
	}
	(&m).addChipPreview(domainFlows, "*", qchip.FromConfig(st.UI.Chips[0]), "u@other")

	(&m).applyChipScopeChosen(chipScopeChosenMsg{
		share: settings.ChipShare{Kind: settings.ChipShareGlobal},
		target: chipScopeTarget{
			kind:   chipScopeTargetOtherOrg,
			domain: domainFlows,
			chipID: "phd-journals",
			scope:  "*",
		},
	})

	chips := st.Chips()
	if len(chips) != 1 {
		t.Fatalf("chips len = %d, want 1", len(chips))
	}
	if chips[0].Share.Kind != settings.ChipShareGlobal {
		t.Fatalf("share kind = %q, want %q", chips[0].Share.Kind, settings.ChipShareGlobal)
	}
	if chips[0].OrgUser != "" {
		t.Fatalf("org_user was not cleared: %q", chips[0].OrgUser)
	}
	if got := m.chipPreviewsFor(domainFlows, "*"); got != nil {
		t.Fatalf("preview was not removed after scope persist: %+v", got)
	}
	if rows := reg.ChipsFor("*"); len(rows) != 1 || rows[0].ID != "phd-journals" {
		t.Fatalf("registry was not refreshed with global chip: %+v", rows)
	}

	b, err := os.ReadFile(filepath.Join(home, ".sf-deck", "settings.toml"))
	if err != nil {
		t.Fatalf("settings.toml not written: %v", err)
	}
	body := string(b)
	if !strings.Contains(body, "kind = \"global\"") {
		t.Fatalf("settings.toml missing global share kind:\n%s", body)
	}
	if strings.Contains(body, "org_user") {
		t.Fatalf("settings.toml still contains org_user:\n%s", body)
	}
}

func TestApplyChipScopeChosenWizardWritesBackWithoutPersisting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	m := Model{
		modelServices: modelServices{settings: &settings.Settings{}},
		modelTransient: modelTransient{
			chipWizard: &chipWizardState{
				Share: settings.ChipShare{Kind: settings.ChipShareOrg, Orgs: []string{"u@active"}},
			},
		},
	}

	(&m).applyChipScopeChosen(chipScopeChosenMsg{
		share:  settings.ChipShare{Kind: settings.ChipShareGlobal},
		target: chipScopeTarget{kind: chipScopeTargetWizard},
	})

	if got := m.chipWizard.Share.Kind; got != settings.ChipShareGlobal {
		t.Fatalf("wizard share kind = %q, want %q", got, settings.ChipShareGlobal)
	}
	if _, err := os.Stat(filepath.Join(home, ".sf-deck", "settings.toml")); !os.IsNotExist(err) {
		t.Fatalf("wizard scope write-back should not persist settings, stat err = %v", err)
	}
}
