package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func (m *Model) moveFlowDetailCursor(delta int) {
	if len(m.orgs) == 0 {
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	if d.FlowCur == "" {
		return
	}
	r, ok := d.FlowVersions[d.FlowCur]
	if !ok {
		return
	}
	n := len(r.Value())
	d.Cursors.Move(cursorKindFlowVersion, delta, n, d.FlowCur)
}

// ensureFlowsListData loads the flows list AND resets the drill-refresh
// latch. Being back on the list means the next drill into any flow
// re-fetches its versions (picking up Salesforce-side changes between
// drills) — see ensureFlowDetailData / orgData.flowVersionsLoadedFor.
func (m *Model) ensureFlowsListData(d *orgData, _ sf.Org) tea.Cmd {
	d.flowVersionsLoadedFor = ""
	return d.Flows.Ensure(m.cache)
}

func (m *Model) ensureFlowDetailData(d *orgData, o sf.Org) tea.Cmd {
	cmds := []tea.Cmd{d.Flows.Ensure(m.cache)}
	if d.FlowCur != "" {
		r := d.EnsureFlowVersions(targetArg(o), d.FlowCur)
		// Force fresh versions when drilling into a NEW flow (the user is
		// inspecting it now and may have just changed it in Salesforce);
		// fall back to cache-respecting Ensure on intra-family returns
		// (esc-back from the version viewer) so navigation isn't a
		// re-fetch every time.
		if d.takeFlowVersionsDrillRefresh() {
			cmds = append(cmds, r.Refresh(m.cache))
		} else {
			cmds = append(cmds, r.Ensure(m.cache))
		}
	}
	return tea.Batch(cmds...)
}

// takeFlowVersionsDrillRefresh reports whether the current flow's
// versions should be force-refreshed on this drill, latching so a
// subsequent return to the same flow (without leaving to the list)
// doesn't re-fetch. Returns true exactly once per fresh flow drill.
func (d *orgData) takeFlowVersionsDrillRefresh() bool {
	if d.FlowCur == d.flowVersionsLoadedFor {
		return false
	}
	d.flowVersionsLoadedFor = d.FlowCur
	return true
}

func (m Model) refreshFlowDetailData(d *orgData) tea.Cmd {
	cmd := d.Flows.Refresh(m.cache)
	if d.FlowCur != "" {
		if r, ok := d.FlowVersions[d.FlowCur]; ok {
			return tea.Batch(cmd, r.Refresh(m.cache))
		}
	}
	return cmd
}
