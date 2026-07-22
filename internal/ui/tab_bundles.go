package ui

// /bundles — list of sfdx-project bundles linked to the active
// DevProject.
//
// Reached via `b` on /dev-project-detail. Each row is one Bundle
// stored in SQLite (path, default org, last retrieved/deployed
// timestamps). Row actions:
//
//   Enter / b   drill into TabBundleDetail (preview tables + per-row
//               retrieve/deploy)
//   r           retrieve all (full sf project retrieve)
//   D           deploy all (full sf project deploy)
//   o           open the bundle dir with `open` (Finder / xdg-open)
//   d           unlink (removes the SQLite row, leaves the directory)
//
// Doesn't touch on-disk files for unlink — the bundle directory may
// be in git, the user's IDE may have it open, etc. Conservative.

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// renderBundleRow formats one bundle row: bullet · path · status.
// Active row gets the highlight bar; stale (path missing) rows get
// dimmed + a "[stale]" suffix.
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
		// Truncate from the LEFT — the tail (project name) is more
		// distinguishing than the leading directory chain.
		pathLabel = "…" + pathLabel[len(pathLabel)-leftBudget+1:]
	}
	pad := inner - lipgloss.Width(prefix) - lipgloss.Width(pathLabel) - lipgloss.Width(right) - 2
	if pad < 1 {
		pad = 1
	}
	return prefix + leftStyle.Render(pathLabel) +
		strings.Repeat(" ", pad) + rightStyle.Render(right)
}

// renderBundleRowWithProject is renderBundleRow + a parent project
// label up front. Used by the top-level /dev-projects → Bundles
// subtab where each bundle has to identify which project it belongs
// to (the per-project list doesn't need this since the project is
// in the breadcrumb).
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
		// Trim path from the left edge — project label stays intact.
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

// humanTimeAgoBundle is a compact relative-time string for the
// bundles list. Shorter than the home-downloads variant ("3m" not
// "3 minutes ago") so the right-side cluster stays narrow.
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

// moveAllBundlesCursor is the SubtabSpec.MoveCursor closure for the
// top-level /dev-projects → Bundles subtab. Operates on the
// Model-wide all-bundles slice rather than the per-project one.
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

// activateAllBundles is Enter on the top-level all-bundles list.
// Drills into TabBundleDetail.
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
	// Reset the per-bundle FILES view state — cwd back to root,
	// invalidate the loaded-key so the next ensure re-reads the
	// new bundle's dir. Mode (components/files) is preserved so
	// a user who likes files view stays in it across bundles.
	m.bundleFilesCwd = ""
	m.bundleFilesLoadedFor = ""
	// Also set devProjectCur so EscBack lands on the right dev
	// project (the bundle's parent).
	m.setActiveDevProject(b.DevProjectID)
	m.setTab(TabBundleDetail)
	return m.onTabChanged()
}

// moveBundlesCursor is the TabSpec.MoveCursor closure for /bundles.
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

// activateBundles is Enter on /bundles — drills into the cursored
// bundle's detail view.
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
	// Reset the per-bundle FILES view state on bundle switch.
	m.bundleFilesCwd = ""
	m.bundleFilesLoadedFor = ""
	m.setTab(TabBundleDetail)
	return m.onTabChanged()
}

// onBundlesKey routes the per-tab keys (r retrieve, D deploy, o
// reveal, d unlink) for the Bundles list — works on both the
// per-project Bundles subtab (TabDevProjectDetail) and the
// top-level all-bundles subtab (TabDevProjects). Returns
// (consumed, cmd); cmd is the goroutine to kick for retrieve/deploy.
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

// cursoredBundle resolves the bundle currently under the cursor
// across both contexts the bundles list appears in:
//
//	/dev-project-detail · Bundles subtab → ListBundlesFor(devProjectCur)[BundleCursor]
//	/dev-projects · Bundles subtab        → ListAllBundles()[AllBundlesCursor]
//
// Returns (zero, false) when not in either context or the cursor is
// out of range.
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
