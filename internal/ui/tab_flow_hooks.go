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

func (m *Model) ensureFlowsListData(d *orgData, _ sf.Org) tea.Cmd {
	d.flowVersionsLoadedFor = ""
	return d.Flows.Ensure(m.cache)
}

func (m *Model) ensureFlowDetailData(d *orgData, o sf.Org) tea.Cmd {
	cmds := []tea.Cmd{d.Flows.Ensure(m.cache)}
	if d.FlowCur != "" {
		r := d.EnsureFlowVersions(targetArg(o), d.FlowCur)
		if d.takeFlowVersionsDrillRefresh() {
			cmds = append(cmds, r.Refresh(m.cache))
		} else {
			cmds = append(cmds, r.Ensure(m.cache))
		}
	}
	return tea.Batch(cmds...)
}

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
