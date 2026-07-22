package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/project"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/resource"
)

func TestGlobalResourceCacheMissSchedulesRefresh(t *testing.T) {
	m := Model{
		modelServices: modelServices{
			settings: &settings.Settings{Orgs: map[string]settings.OrgConfig{}},
		},
		modelOrgs: modelOrgs{
			data: map[string]*orgData{},
		},
	}

	orgFetches := 0
	m.orgsRes = Resource[[]sf.Org]{
		Scope: "global", Key: "orgs",
		Fetch: func() ([]sf.Org, error) {
			orgFetches++
			return []sf.Org{{Username: "user@example.com"}}, nil
		},
	}
	_, orgCmd := m.applyResourceMsg(resource.UpdatedMsg{
		Scope:     "global",
		Key:       "orgs",
		FromCache: true,
	})
	runCmds(t, orgCmd)
	if orgFetches != 1 {
		t.Fatalf("org cache miss triggered %d fetches, want 1", orgFetches)
	}

	projectFetches := 0
	m.projectsRes = Resource[[]*project.Project]{
		Scope: "global", Key: "projects",
		Fetch: func() ([]*project.Project, error) {
			projectFetches++
			return []*project.Project{{Name: "demo"}}, nil
		},
	}
	_, projectCmd := m.applyResourceMsg(resource.UpdatedMsg{
		Scope:     "global",
		Key:       "projects",
		FromCache: true,
	})
	runCmds(t, projectCmd)
	if projectFetches != 1 {
		t.Fatalf("project cache miss triggered %d fetches, want 1", projectFetches)
	}
}

func TestOrgStartupPinSurvivesInitialCacheMiss(t *testing.T) {
	st := &settings.Settings{Orgs: map[string]settings.OrgConfig{
		"training@example.com": {Default: true},
	}}
	st.SetDisableHomeBanner(true)
	m := Model{
		modelServices: modelServices{
			settings: st,
		},
		modelOrgs: modelOrgs{
			data: map[string]*orgData{},
		},
	}
	liveOrgs := []sf.Org{
		{Username: "first@example.com", Alias: "first"},
		{Username: "training@example.com", Alias: "training-sandbox"},
	}
	m.orgsRes = Resource[[]sf.Org]{
		Scope: "global", Key: "orgs",
		Fetch: func() ([]sf.Org, error) { return liveOrgs, nil },
	}

	next, cmd := m.applyResourceMsg(resource.UpdatedMsg{
		Scope:     "global",
		Key:       "orgs",
		FromCache: true,
	})
	m = next.(Model)
	if m.pinnedDefaultRestored {
		t.Fatal("cache miss consumed startup pin restore before a real org list arrived")
	}
	if cmd == nil {
		t.Fatal("cache miss did not schedule live org refresh")
	}

	next, _ = m.applyResourceMsg(resource.UpdatedMsg{
		Scope:   "global",
		Key:     "orgs",
		Payload: &liveOrgs,
	})
	m = next.(Model)
	if got := m.orgs[m.selected].Username; got != "training@example.com" {
		t.Fatalf("selected org = %q, want pinned training@example.com", got)
	}
	if m.selectedUsername != "training@example.com" {
		t.Fatalf("selectedUsername = %q, want training@example.com", m.selectedUsername)
	}
	if !m.pinnedDefaultRestored {
		t.Fatal("live org list should mark startup pin restore complete")
	}
}

func TestEnsureOrgDataRebuildsWhenAliasChanges(t *testing.T) {
	m := Model{
		modelServices: modelServices{
			cache:    nil,
			settings: &settings.Settings{Orgs: map[string]settings.OrgConfig{}},
		},
		modelOrgs: modelOrgs{
			data: map[string]*orgData{},
			orgs: []sf.Org{{
				Username: "user@example.com",
				Alias:    "old-alias",
			}},
		},
	}

	first := m.ensureOrgData("user@example.com")
	if first.target != "old-alias" {
		t.Fatalf("initial target = %q, want old-alias", first.target)
	}

	m.orgs[0].Alias = "new-alias"
	second := m.ensureOrgData("user@example.com")
	if second == first {
		t.Fatal("orgData was reused after target alias changed")
	}
	if second.target != "new-alias" {
		t.Fatalf("rebuilt target = %q, want new-alias", second.target)
	}
}

func runCmds(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected refresh command, got nil")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			if child != nil {
				runCmds(t, child)
			}
		}
	}
}
