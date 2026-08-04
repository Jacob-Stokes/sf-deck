package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// TestSystemSubtabDispatch guards renderSystem's per-subtab switch: each
// /system subtab must render its OWN surface, not fall through to the
// default (Apex Logs). Regression for "Async Jobs + Scheduled Jobs both
// showed the APEX LOGS list" — a subtab registered in the TabSpec but
// missing from renderSystem's switch renders the wrong content while
// still passing the panic-only smoke test.
func TestSystemSubtabDispatch(t *testing.T) {
	c, err := cache.Open()
	if err != nil {
		t.Fatalf("cache open: %v", err)
	}
	defer c.Close()

	m := New(c)
	m.orgs = []sf.Org{{Alias: "d", Username: "d@d.test", InstanceURL: "https://d.my.salesforce.com", Status: "Connected", LastUsed: time.Now().Format(time.RFC3339)}}
	_ = m.ensureOrgData("d@d.test")
	m.width, m.height = 200, 60
	m.setTab(TabSystem)

	// subtab ID → a title fragment unique to that subtab's renderer.
	wantTitle := map[Subtab]string{
		SubtabSystemLogs:       "APEX LOGS",
		SubtabSystemAudit:      "SETUP AUDIT",
		SubtabSystemInterviews: "FLOW INTERVIEWS",
		SubtabSystemAsyncJobs:  "ASYNC JOBS",
		SubtabSystemScheduled:  "SCHEDULED JOBS",
	}

	subs := systemSubtabs()
	spec := lookupTabSpec(TabSystem)
	for i, sub := range subs {
		want, checked := wantTitle[sub.ID]
		if !checked {
			continue // API subtab has no list title; skip
		}
		spec.SetSubtabIdx(&m, i)
		body := m.renderSystem(m.width, m.height)
		if !strings.Contains(body, want) {
			t.Errorf("subtab %q rendered without its own title %q — likely falling through renderSystem's switch to the default. Body head: %.80q",
				sub.ID, want, body)
		}
	}
}
