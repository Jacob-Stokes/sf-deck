package ui

import (
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func testSafetyModel(orgs []sf.Org, selected int) Model {
	return Model{
		modelServices: modelServices{
			settings: &settings.Settings{},
		},
		modelOrgs: modelOrgs{
			orgs:     orgs,
			selected: selected,
		},
	}
}

func TestUserDetailMutationsRespectSafety(t *testing.T) {
	d := &orgData{
		orgDataUsers: orgDataUsers{
			UserCur: "005000000000001",
			UserDetailRows: map[string]sf.UserRow{
				"005000000000001": {
					ID:       "005000000000001",
					Username: "user@example.com",
					IsActive: true,
				},
			},
			UserLoginRows: map[string]sf.UserLoginRow{
				"005000000000001": {
					ID:       "0Yw000000000001",
					UserID:   "005000000000001",
					IsFrozen: false,
				},
			},
		},
	}

	for _, tc := range []struct {
		name       string
		org        sf.Org
		wantReason string
	}{
		{
			name:       "production read_only",
			org:        sf.Org{Alias: "prod", Username: "admin@example.com", Status: "Connected"},
			wantReason: "read_only",
		},
		{
			name:       "sandbox records",
			org:        sf.Org{Alias: "sbx", Username: "admin@example.com", IsSandbox: true, Status: "Connected"},
			wantReason: "records",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testSafetyModel([]sf.Org{tc.org}, 0)
			actions := m.cursoredUserActions(d)
			blocked := map[string]bool{}
			allowed := map[string]bool{}
			for _, a := range actions {
				if a.Separator {
					continue
				}
				if a.Allowed {
					allowed[a.ID] = true
				} else if strings.Contains(a.Reason, tc.wantReason) {
					blocked[a.ID] = true
				}
			}

			for _, id := range []string{"reset-password", "reset-password-link", "freeze", "deactivate"} {
				if !blocked[id] {
					t.Fatalf("%s was not blocked by %s safety; blocked=%v allowed=%v", id, tc.wantReason, blocked, allowed)
				}
			}
			for _, id := range []string{"open-detail", "yank-id", "yank-username"} {
				if !allowed[id] {
					t.Fatalf("%s should remain available under %s safety; blocked=%v allowed=%v", id, tc.wantReason, blocked, allowed)
				}
			}
		})
	}

	t.Run("scratch full allows", func(t *testing.T) {
		m := testSafetyModel([]sf.Org{{Alias: "scratch", Username: "scratch@example.com", IsScratch: true, Status: "Connected"}}, 0)
		actions := m.cursoredUserActions(d)
		allowed := map[string]bool{}
		for _, a := range actions {
			if a.Allowed {
				allowed[a.ID] = true
			}
		}
		for _, id := range []string{"reset-password", "reset-password-link", "freeze", "deactivate"} {
			if !allowed[id] {
				t.Fatalf("%s should be allowed under full scratch safety; allowed=%v", id, allowed)
			}
		}
	})
}

func TestBundleDeployUsesBundleTargetSafety(t *testing.T) {
	m := testSafetyModel([]sf.Org{
		{Alias: "scratch", Username: "scratch@example.com", IsScratch: true, Status: "Connected"},
		{Alias: "prod", Username: "prod@example.com", Status: "Connected"},
	}, 0)

	cmd := startBundleDeploy(&m, devproject.Bundle{
		ID:              "bundle-1",
		Path:            "/tmp/bundle",
		DefaultOrgAlias: "prod",
	})
	if cmd != nil {
		t.Fatal("deploy command should be blocked for bundle default target prod")
	}
	if !strings.Contains(m.banner, "prod is read_only") {
		t.Fatalf("blocked deploy banner = %q, want read_only prod warning", m.banner)
	}
}
