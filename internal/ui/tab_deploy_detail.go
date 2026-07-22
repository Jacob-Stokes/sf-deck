package ui

// /deploy drill — one DeployRequest's component failures, test
// failures, and coverage warnings, fetched lazily via the
// metadata/deployRequest REST endpoint (same payload `sf project
// deploy report` reads). Enter on /deploys lands here; Esc returns.

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// deployDetailRes returns (lazily creating) the keyed detail
// Resource for one deploy id. Terminal deploys never change, so the
// TTL is infinite (0 = no expiry); an in-flight deploy drilled early
// can be re-pulled with r.
func (d *orgData) deployDetailRes(alias, id string) *Resource[sf.DeployDetail] {
	if id == "" {
		return nil
	}
	if d.DeployDetailMap == nil {
		d.DeployDetailMap = map[string]*Resource[sf.DeployDetail]{}
	}
	if r, ok := d.DeployDetailMap[id]; ok {
		return r
	}
	target := alias
	if target == "" {
		target = d.username
	}
	r := &Resource[sf.DeployDetail]{
		Scope: d.username, Key: "deploy_detail:" + id, TTL: 0,
		Fetch: func() (sf.DeployDetail, error) {
			return sf.FetchDeployDetail(target, id)
		},
	}
	d.DeployDetailMap[id] = r
	return r
}

// drillIntoDeploy is the Enter handler on /deploys rows.
func (m *Model) drillIntoDeploy() tea.Cmd {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	row, ok := d.DeployList.Selected()
	if !ok {
		return nil
	}
	d.DeployCur = row.ID
	d.DeployDetailCursor = 0
	m.setTab(TabDeployDetail)
	res := d.deployDetailRes(m.orgs[m.selected].Alias, row.ID)
	if res == nil {
		return m.onTabChanged()
	}
	return tea.Batch(m.onTabChanged(), res.Ensure(m.cache))
}

// moveDeployDetailCursor scrolls the detail pane. The cursor is a
// top-line offset, clamped during render.
func (m *Model) moveDeployDetailCursor(delta int) {
	d := m.activeOrgData()
	if d == nil {
		return
	}
	d.DeployDetailCursor += delta
	if d.DeployDetailCursor < 0 {
		d.DeployDetailCursor = 0
	}
}

// deployRowByID resolves the cached list row for the drilled deploy
// (for the header + sidebar); zero row when the window scrolled past it.
func (m Model) deployRowByID(id string) (sf.DeployRow, bool) {
	d := m.activeOrgData()
	if d == nil || id == "" {
		return sf.DeployRow{}, false
	}
	for _, r := range d.Deploys.Value() {
		if r.ID == id {
			return r, true
		}
	}
	return sf.DeployRow{}, false
}

func (m Model) renderDeployDetail(w, innerH int) string {
	inner := w - 4
	d := m.activeOrgData()
	if d == nil || d.DeployCur == "" {
		return theme.Subtle.Render("  no deploy selected")
	}
	res := d.DeployDetailMap[d.DeployCur]

	var lines []string
	row, haveRow := m.deployRowByID(d.DeployCur)
	header := "DEPLOY · " + d.DeployCur
	if haveRow {
		kind := "deploy"
		if row.CheckOnly {
			kind = "validate"
		}
		header = "DEPLOY · " + row.Status + " · " + kind +
			" · " + deployDurationLabel(row) + " · by " + row.CreatedByName
	}
	lines = append(lines, sectionTitle("  "+header))
	lines = append(lines, "")

	if res == nil || (res.FetchedAt().IsZero() && !res.Busy() && res.Err() == nil) {
		lines = append(lines, dimLine("  press "+firstPretty(Keys.Refresh)+" to load deploy detail", inner))
		return strings.Join(lines, "\n")
	}
	if res.Err() != nil {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Red).Render("  "+res.Err().Error()))
		return strings.Join(lines, "\n")
	}
	if res.FetchedAt().IsZero() {
		lines = append(lines, dimLine("  loading deploy detail…", inner))
		return strings.Join(lines, "\n")
	}

	det := res.Value()
	body := deployDetailLines(det, haveRow, row, inner)

	// Scroll window: header stays pinned, body scrolls under it.
	budget := innerH - len(lines)
	if budget < 1 {
		budget = 1
	}
	maxTop := len(body) - budget
	if maxTop < 0 {
		maxTop = 0
	}
	top := d.DeployDetailCursor
	if top > maxTop {
		top = maxTop
	}
	end := top + budget
	if end > len(body) {
		end = len(body)
	}
	lines = append(lines, body[top:end]...)
	return strings.Join(lines, "\n")
}

// deployDetailLines renders the scrollable body: failures first
// (that's what you drilled in for), then test failures, coverage
// warnings, and a success summary.
func deployDetailLines(det sf.DeployDetail, haveRow bool, row sf.DeployRow, inner int) []string {
	var out []string
	errStyle := lipgloss.NewStyle().Foreground(theme.Red)
	dim := theme.Subtle

	if len(det.Failures) > 0 {
		out = append(out, sectionTitle(fmt.Sprintf("  COMPONENT FAILURES · %d", len(det.Failures))))
		for _, f := range det.Failures {
			loc := ""
			if f.LineNumber > 0 {
				loc = fmt.Sprintf(":%d", f.LineNumber)
				if f.ColumnNumber > 0 {
					loc += fmt.Sprintf(":%d", f.ColumnNumber)
				}
			}
			out = append(out, errStyle.Render("  ✗ "+f.ComponentType+" "+f.FullName+loc))
			for _, l := range wrapLines(f.Problem, inner-6) {
				out = append(out, dim.Render("      "+l))
			}
		}
		out = append(out, "")
	}

	if len(det.TestFails) > 0 {
		out = append(out, sectionTitle(fmt.Sprintf("  TEST FAILURES · %d", len(det.TestFails))))
		for _, f := range det.TestFails {
			out = append(out, errStyle.Render("  ✗ "+f.Name+"."+f.MethodName))
			for _, l := range wrapLines(f.Message, inner-6) {
				out = append(out, dim.Render("      "+l))
			}
			if f.StackTrace != "" {
				for _, l := range strings.Split(f.StackTrace, "\n") {
					out = append(out, dim.Render("      "+l))
				}
			}
		}
		out = append(out, "")
	}

	if len(det.Coverage) > 0 {
		out = append(out, sectionTitle(fmt.Sprintf("  COVERAGE WARNINGS · %d", len(det.Coverage))))
		for _, c := range det.Coverage {
			name := c.Name
			if name != "" {
				name += ": "
			}
			for _, l := range wrapLines(name+c.Message, inner-4) {
				out = append(out, dim.Render("    "+l))
			}
		}
		out = append(out, "")
	}

	// Success summary — counts up front, the component list after, so
	// a clean deploy still shows something useful without burying the
	// failures above on broken ones.
	summary := fmt.Sprintf("  %d component(s) succeeded", len(det.Successes))
	if det.TestsRun > 0 {
		summary += fmt.Sprintf(" · %d test(s) run", det.TestsRun)
	}
	out = append(out, sectionTitle("  RESULT"), dim.Render(summary))
	for _, c := range det.Successes {
		flag := "·"
		switch {
		case c.Created:
			flag = "+"
		case c.Deleted:
			flag = "-"
		case c.Changed:
			flag = "~"
		}
		label := c.ComponentType
		if label != "" {
			label += " "
		}
		out = append(out, dim.Render("    "+flag+" "+label+c.FullName))
	}
	if len(out) == 0 {
		out = append(out, dim.Render("  nothing recorded for this deploy"))
	}
	return out
}

// ensureDeployDetailData / refreshDeployDetailData are the /deploy
// drill's registry hooks (extracted in the registry-purity pass).
func (m *Model) ensureDeployDetailData(d *orgData, o sf.Org) tea.Cmd {
	if d.DeployCur == "" {
		return nil
	}
	res := d.deployDetailRes(targetArg(o), d.DeployCur)
	if res == nil {
		return nil
	}
	return res.Ensure(m.cache)
}

func (m Model) refreshDeployDetailData(d *orgData) tea.Cmd {
	if d.DeployCur == "" {
		return nil
	}
	if res := d.DeployDetailMap[d.DeployCur]; res != nil {
		return res.Refresh(m.cache)
	}
	return nil
}
