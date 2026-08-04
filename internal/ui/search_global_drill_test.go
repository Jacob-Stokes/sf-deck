package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// Global-search Enter on a code artefact must land IN the code (the
// detail drill), not on the list tab with the row merely selected —
// that regressed UX had users pressing Enter twice. Triggers already
// drilled; this pins classes + LWC + Aura to the same behaviour.
func TestGlobalSearchOpensCodeDetail(t *testing.T) {
	newModel := func() (*Model, *orgData) {
		d := &orgData{}
		d.Tab = TabHome
		m := &Model{
			modelOrgs: modelOrgs{
				orgs:     []sf.Org{{Username: "gs@test", Alias: "gs"}},
				selected: 0,
				data:     map[string]*orgData{"gs@test": d},
			},
		}
		return m, d
	}
	// ensureOrgData may swap a bare fixture entry for a fully-wired
	// one mid-drill, so post-call assertions must re-fetch.
	liveData := func(m *Model) *orgData { return m.data["gs@test"] }

	t.Run("apex class", func(t *testing.T) {
		m, d := newModel()
		d.ApexClassList.Set([]sf.ApexClassRow{{ID: "01pX", Name: "FooService"}})
		_ = openApexClassCmd("01pX")(m)
		if got := m.tab(); got != TabApexDetail {
			t.Errorf("tab = %v, want TabApexDetail (the code view)", got)
		}
		if cur := liveData(m).ApexCur; cur != "01pX" {
			t.Errorf("ApexCur = %q, want the opened class id", cur)
		}
	})

	t.Run("lwc bundle", func(t *testing.T) {
		m, d := newModel()
		d.LWCBundleList.Set([]sf.LWCBundle{{ID: "0RbX", DeveloperName: "fooCard"}})
		_ = openLWCBundleCmd("0RbX")(m)
		if got := m.tab(); got != TabLWCDetail {
			t.Errorf("tab = %v, want TabLWCDetail (the code view)", got)
		}
		if cur := liveData(m).LWCCur; cur != "0RbX" {
			t.Errorf("LWCCur = %q, want the opened bundle id", cur)
		}
	})

	t.Run("permset", func(t *testing.T) {
		m, d := newModel()
		d.PermSetList.Set([]sf.PermissionSet{{ID: "0PSX", Label: "Admin Extras"}})
		_ = openPermSetCmd("0PSX")(m)
		if got := m.tab(); got != TabPermParentDetail {
			t.Errorf("tab = %v, want TabPermParentDetail", got)
		}
		ld := liveData(m)
		if ld.PermParentKind != "permset" || ld.PermParentID != "0PSX" {
			t.Errorf("perm parent = %q/%q, want permset/0PSX", ld.PermParentKind, ld.PermParentID)
		}
	})

	t.Run("queue", func(t *testing.T) {
		m, d := newModel()
		d.QueueList.Set([]sf.QueueRow{{ID: "00GX", Name: "Support Queue"}})
		_ = openQueueCmd("00GX")(m)
		if got := m.tab(); got != TabQueueDetail {
			t.Errorf("tab = %v, want TabQueueDetail", got)
		}
		if ld := liveData(m); ld.GroupMemberID != "00GX" {
			t.Errorf("GroupMemberID = %q, want the queue id", ld.GroupMemberID)
		}
	})

	t.Run("report", func(t *testing.T) {
		m, d := newModel()
		d.ReportList.Set([]sf.ReportSummary{{ID: "00OX", Name: "Pipeline"}})
		_ = openReportCmd("00OX")(m)
		if got := m.tab(); got != TabReportDetail {
			t.Errorf("tab = %v, want TabReportDetail", got)
		}
		if ld := liveData(m); ld.ReportCur != "00OX" {
			t.Errorf("ReportCur = %q, want the report id", ld.ReportCur)
		}
	})

	t.Run("aura bundle", func(t *testing.T) {
		m, d := newModel()
		d.AuraBundleList.Set([]sf.AuraBundle{{ID: "0AbX", DeveloperName: "fooCmp"}})
		_ = openAuraBundleCmd("0AbX")(m)
		if got := m.tab(); got != TabLWCDetail {
			t.Errorf("tab = %v, want TabLWCDetail (the code view)", got)
		}
		if cur := liveData(m).LWCCur; cur != "0AbX" {
			t.Errorf("LWCCur = %q, want the opened bundle id", cur)
		}
	})
}
