package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func renderBundleRow(b devproject.Bundle, active, mainFocus bool, inner int) string {
	stale := b.Stale()
	leftStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	if stale {
		leftStyle = lipgloss.NewStyle().Foreground(theme.Muted)
	}
	if active {
		leftStyle = leftStyle.Bold(true)
	}

	prefix := "    "
	if active {
		bar := lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌")
		prefix = "  " + bar + " "
	}

	pathLabel := b.Path
	if stale {
		pathLabel += "  [stale]"
	}

	var rightParts []string
	if !b.LastRetrievedAt.IsZero() {
		rightParts = append(rightParts, "↓ "+humanTimeAgoBundle(b.LastRetrievedAt))
	}
	if !b.LastDeployedAt.IsZero() {
		rightParts = append(rightParts, "↑ "+humanTimeAgoBundle(b.LastDeployedAt))
	}
	if len(rightParts) == 0 {
		rightParts = append(rightParts, "never used")
	}
	right := strings.Join(rightParts, " · ")
	rightStyle := lipgloss.NewStyle().Foreground(theme.Muted)

	leftBudget := inner - lipgloss.Width(right) - lipgloss.Width(prefix) - 2
	if leftBudget < 20 {
		leftBudget = 20
	}
	if len(pathLabel) > leftBudget {
		pathLabel = "…" + pathLabel[len(pathLabel)-leftBudget+1:]
	}
	pad := inner - lipgloss.Width(prefix) - lipgloss.Width(pathLabel) - lipgloss.Width(right) - 2
	if pad < 1 {
		pad = 1
	}
	return prefix + leftStyle.Render(pathLabel) +
		strings.Repeat(" ", pad) + rightStyle.Render(right)
}

func renderBundleRowWithProject(b devproject.Bundle, projectName string, active, mainFocus bool, inner int) string {
	stale := b.Stale()
	leftStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	if stale {
		leftStyle = lipgloss.NewStyle().Foreground(theme.Muted)
	}
	if active {
		leftStyle = leftStyle.Bold(true)
	}

	prefix := "    "
	if active {
		bar := lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌")
		prefix = "  " + bar + " "
	}

	if projectName == "" {
		projectName = "(orphaned)"
	}
	projLabel := lipgloss.NewStyle().Foreground(theme.Cyan).Render("[" + projectName + "]")
	pathLabel := b.Path
	if stale {
		pathLabel += "  [stale]"
	}

	var rightParts []string
	if !b.LastRetrievedAt.IsZero() {
		rightParts = append(rightParts, "↓ "+humanTimeAgoBundle(b.LastRetrievedAt))
	}
	if !b.LastDeployedAt.IsZero() {
		rightParts = append(rightParts, "↑ "+humanTimeAgoBundle(b.LastDeployedAt))
	}
	if len(rightParts) == 0 {
		rightParts = append(rightParts, "never used")
	}
	right := strings.Join(rightParts, " · ")
	rightStyle := lipgloss.NewStyle().Foreground(theme.Muted)

	leftRendered := projLabel + " " + leftStyle.Render(pathLabel)
	leftWidth := lipgloss.Width(leftRendered)
	leftBudget := inner - lipgloss.Width(right) - lipgloss.Width(prefix) - 2
	if leftBudget < 20 {
		leftBudget = 20
	}
	if leftWidth > leftBudget {
		over := leftWidth - leftBudget
		if over < len(pathLabel)-3 {
			pathLabel = "…" + pathLabel[over+1:]
		}
		leftRendered = projLabel + " " + leftStyle.Render(pathLabel)
	}
	pad := inner - lipgloss.Width(prefix) - lipgloss.Width(leftRendered) - lipgloss.Width(right) - 2
	if pad < 1 {
		pad = 1
	}
	return prefix + leftRendered +
		strings.Repeat(" ", pad) + rightStyle.Render(right)
}

func humanTimeAgoBundle(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	return t.Format("Jan 2")
}

// bundleCursor returns the selected row index for the active project's
// bundle list, clamped to len. Stored on orgData (per-org cursor pool)
// so revisits return to the same row.
func (m Model) bundleCursor(n int) int {
	d := m.activeOrgData()
	if d == nil {
		return 0
	}
	if d.BundleCursor < 0 {
		return 0
	}
	if d.BundleCursor >= n && n > 0 {
		return n - 1
	}
	return d.BundleCursor
}

func moveAllBundlesCursor(m *Model, delta int) {
	if m.devProjects == nil {
		return
	}
	bundles, _ := m.devProjects.ListAllBundles()
	n := len(bundles)
	if n == 0 {
		return
	}
	d := m.activeOrgData()
	if d == nil {
		return
	}
	c := d.AllBundlesCursor + delta
	if c < 0 {
		c = 0
	}
	if c >= n {
		c = n - 1
	}
	d.AllBundlesCursor = c
}

func activateAllBundles(m *Model) tea.Cmd {
	if m.devProjects == nil {
		return nil
	}
	bundles, err := m.devProjects.ListAllBundles()
	if err != nil || len(bundles) == 0 {
		return nil
	}
	d := m.activeOrgData()
	c := 0
	if d != nil {
		c = d.AllBundlesCursor
	}
	if c < 0 || c >= len(bundles) {
		return nil
	}
	b := bundles[c]
	m.bundleCur = b.ID
	m.bundleFilesCwd = ""
	m.bundleFilesLoadedFor = ""
	m.setActiveDevProject(b.DevProjectID)
	m.setTab(TabBundleDetail)
	return m.onTabChanged()
}

func moveBundlesCursor(m *Model, delta int) {
	if m.devProjects == nil || m.devProjectCur == "" {
		return
	}
	bundles, _ := m.devProjects.ListBundlesFor(m.devProjectCur)
	n := len(bundles)
	if n == 0 {
		return
	}
	d := m.activeOrgData()
	if d == nil {
		return
	}
	c := d.BundleCursor + delta
	if c < 0 {
		c = 0
	}
	if c >= n {
		c = n - 1
	}
	d.BundleCursor = c
}

func activateBundles(m *Model) tea.Cmd {
	if m.devProjects == nil || m.devProjectCur == "" {
		return nil
	}
	bundles, err := m.devProjects.ListBundlesFor(m.devProjectCur)
	if err != nil || len(bundles) == 0 {
		return nil
	}
	c := m.bundleCursor(len(bundles))
	if c < 0 || c >= len(bundles) {
		return nil
	}
	m.bundleCur = bundles[c].ID
	m.bundleFilesCwd = ""
	m.bundleFilesLoadedFor = ""
	m.setTab(TabBundleDetail)
	return m.onTabChanged()
}

func (m *Model) onBundlesKey(key string) (bool, tea.Cmd) {
	if m.devProjects == nil {
		return false, nil
	}
	bundle, ok := m.cursoredBundle()
	if !ok {
		return false, nil
	}
	switch {
	case matches(key, Keys.BundleOpen):
		if bundle.Path == "" {
			return true, nil
		}
		if err := openPath(bundle.Path); err != nil {
			m.flash("open failed: " + err.Error())
		}
		return true, nil
	case matches(key, Keys.BundleUnlink):
		if err := m.devProjects.DeleteBundle(bundle.ID); err != nil {
			m.flash("unlink: " + err.Error())
			return true, nil
		}
		m.flash("bundle unlinked (directory left on disk)")
		return true, nil
	case matches(key, Keys.BundleRetrieve):
		return true, startBundleRetrieve(m, bundle)
	case matches(key, Keys.BundleDeploy):
		return true, startBundleDeploy(m, bundle)
	}
	return false, nil
}

func (m Model) cursoredBundle() (devproject.Bundle, bool) {
	switch m.tab() {
	case TabDevProjectDetail:
		if m.currentSubtab() != SubtabDevProjectBundles || m.devProjectCur == "" {
			return devproject.Bundle{}, false
		}
		bs, err := m.devProjects.ListBundlesFor(m.devProjectCur)
		if err != nil || len(bs) == 0 {
			return devproject.Bundle{}, false
		}
		c := m.bundleCursor(len(bs))
		if c < 0 || c >= len(bs) {
			return devproject.Bundle{}, false
		}
		return bs[c], true
	case TabDevProjects:
		if m.currentSubtab() != SubtabDevProjectsBundles {
			return devproject.Bundle{}, false
		}
		bs, err := m.devProjects.ListAllBundles()
		if err != nil || len(bs) == 0 {
			return devproject.Bundle{}, false
		}
		d := m.activeOrgData()
		c := 0
		if d != nil {
			c = d.AllBundlesCursor
		}
		if c < 0 || c >= len(bs) {
			return devproject.Bundle{}, false
		}
		return bs[c], true
	}
	return devproject.Bundle{}, false
}
