package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/highlight"
)

func (m *Model) activateFlowVersionDetail() tea.Cmd {
	d := m.activeOrgData()
	if d == nil || d.FlowCur == "" {
		return nil
	}
	if m.settings.FlowVersionEnterOpens() {
		o, ok := m.currentOrg()
		if !ok {
			return nil
		}
		target := m.cursorOpenable()
		if target == nil {
			m.flash("nothing to open here")
			return nil
		}
		targets := target.Targets()
		if len(targets) == 0 {
			m.flash("no targets")
			return nil
		}
		m.recordRecentVisit(o.Username, target)
		m.flash("opening " + targets[0].Label + "…")
		return m.openInBrowserCmd(o, targets[0])
	}
	v, ok := m.cursoredFlowVersion(d)
	if !ok || v.ID == "" {
		return nil
	}
	return m.drillFlowVersion(v.ID)
}

func (m *Model) drillFlowVersion(versionID string) tea.Cmd {
	d := m.activeOrgData()
	if d == nil || versionID == "" {
		return nil
	}
	d.FlowVersionCur = versionID
	m.setTab(TabFlowVersionDetail)
	return m.onTabChanged()
}

// flowVersionDefBodyID is the cursor/scroll cache key for the version
// viewer, namespaced so it can't collide with apex/trigger body keys.
func flowVersionDefBodyID(versionID string) string {
	if versionID == "" {
		return ""
	}
	return "flowver:" + versionID
}

func flowVersionDefBody(meta map[string]any) string {
	if len(meta) == 0 {
		return ""
	}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Sprintf("// could not format definition: %v", err)
	}
	return string(b)
}

func (m *Model) ensureFlowVersionDetailData(d *orgData, o sf.Org) tea.Cmd {
	if d.FlowVersionCur == "" {
		return nil
	}
	r := d.EnsureFlowVersionDetail(targetArg(o), d.FlowVersionCur)
	return r.Ensure(m.cache)
}

func (m Model) refreshFlowVersionDetailData(d *orgData) tea.Cmd {
	if d.FlowVersionCur == "" {
		return nil
	}
	if r, ok := d.FlowVersionDetail[d.FlowVersionCur]; ok {
		return r.Refresh(m.cache)
	}
	return nil
}

func (m *Model) moveFlowVersionDetailCursor(delta int) {
	d := m.activeOrgData()
	if d == nil || d.FlowVersionCur == "" {
		return
	}
	r, ok := d.FlowVersionDetail[d.FlowVersionCur]
	if !ok {
		return
	}
	body := flowVersionDefBody(r.Value())
	if body == "" {
		return
	}
	m.codeViewMoveCursor(d, flowVersionDefBodyID(d.FlowVersionCur), lineCount(body), delta)
}

func (m Model) renderFlowVersionDetail(w, innerH int) string {
	inner := w - 4
	if len(m.orgs) == 0 {
		return noOrgPlaceholder()
	}
	d := m.activeOrgData()
	if d == nil || d.FlowVersionCur == "" {
		return theme.Subtle.Render("  no flow version drilled in")
	}

	title := "Flow version"
	if vr, ok := d.FlowVersions[d.FlowCur]; ok && !vr.FetchedAt().IsZero() {
		for _, v := range vr.Value() {
			if v.ID == d.FlowVersionCur {
				name := v.MasterLabel
				if name == "" {
					name = d.FlowCur
				}
				title = fmt.Sprintf("%s · v%d", name, v.VersionNumber)
				if v.Status != "" {
					title += " (" + strings.ToLower(v.Status) + ")"
				}
				break
			}
		}
	}

	var lines []string
	lines = append(lines, sectionTitle(title))

	r, ok := d.FlowVersionDetail[d.FlowVersionCur]
	if !ok || r == nil || r.FetchedAt().IsZero() {
		if r != nil && r.Busy() {
			lines = append(lines, "", dimLine("  loading definition…", inner))
		} else if r != nil && r.Err() != nil {
			lines = append(lines, "", redLine("  "+r.Err().Error()))
		} else {
			lines = append(lines, "", dimLine("  press "+firstPretty(Keys.Refresh)+" to load the definition", inner))
		}
		return strings.Join(lines, "\n")
	}

	body := flowVersionDefBody(r.Value())
	if body == "" {
		lines = append(lines, "", dimLine("  (empty definition)", inner))
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "", dimLine("  "+firstPretty(Keys.YankDefault)+" yank definition · "+firstPretty(Keys.OpenDefault)+" → Flow Builder · / find · esc back", inner))
	lines = append(lines, "")

	bodyHeight := innerH - len(lines)
	bodyView := m.renderCodeView(d, codeViewSpec{
		BodyID:  flowVersionDefBodyID(d.FlowVersionCur),
		Body:    body,
		Lang:    highlight.LangJSON,
		Inner:   inner,
		Height:  bodyHeight,
		Focused: true,
	})
	lines = append(lines, bodyView...)
	return strings.Join(lines, "\n")
}

func (m *Model) yankFlowVersionDefinition() tea.Cmd {
	d := m.activeOrgData()
	if d == nil || d.FlowVersionCur == "" {
		return nil
	}
	r, ok := d.FlowVersionDetail[d.FlowVersionCur]
	if !ok || r.FetchedAt().IsZero() {
		m.flash("definition not loaded yet")
		return nil
	}
	body := flowVersionDefBody(r.Value())
	if body == "" {
		m.flash("nothing to yank")
		return nil
	}
	return m.yankToClipboard(body, "flow definition copied")
}
