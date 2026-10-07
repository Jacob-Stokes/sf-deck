package ui

// /bundle (TabBundleDetail) — drill-in view for one sfdx-project
// bundle linked to a DevProject.

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// renderProjectBundleDetail draws the bundle detail body.
//
// Three blocks stacked vertically:
//
//  1. STICKY HEADER — identity (path, default org, timestamps).
//     Always shown, never scrolls.
//  2. ROW BODY — sortable list table over the preview components.
//     Up/Down, /, [ ], etc. operate here for free via the
//     TabSpec.ListTable wiring.
//  3. FOOTER HINT — one line of keymap reminders.
//
// Preview data is loaded by the EnsureData hook; while it's still
// in flight, the body shows a placeholder. After it lands,
// applyBundlePreviewLoaded folds the preview into
// m.bundleDetailList and the renderListModel pass takes over.
func (m Model) renderProjectBundleDetail(w, innerH int) string {
	inner := w - 4
	if m.devProjects == nil {
		return theme.Subtle.Render("  dev-projects unavailable")
	}
	if m.bundleCur == "" {
		return theme.Subtle.Render("  no bundle drilled in")
	}
	b, err := m.devProjects.GetBundle(m.bundleCur)
	if err != nil {
		return redLine("  bundle: " + err.Error())
	}

	var lines []string
	lines = append(lines, sectionTitle("Bundle"))
	lines = append(lines, dimLine("  "+b.Path, inner))
	lines = append(lines, "")
	lines = append(lines, kvRow("  default org", dashIfEmpty(b.DefaultOrgAlias), inner))
	lines = append(lines, kvRow("  created", b.CreatedAt.Format("2006-01-02 15:04"), inner))
	if !b.LastRetrievedAt.IsZero() {
		lines = append(lines, kvRow("  last retrieved", b.LastRetrievedAt.Format("2006-01-02 15:04"), inner))
	}
	if !b.LastDeployedAt.IsZero() {
		lines = append(lines, kvRow("  last deployed", b.LastDeployedAt.Format("2006-01-02 15:04"), inner))
	}

	if b.Stale() {
		lines = append(lines, "")
		lines = append(lines,
			lipgloss.NewStyle().Foreground(theme.Yellow).Render(
				"  ⚠ on-disk directory is missing or no longer a sfdx project."))
		lines = append(lines, dimLine(
			"  the bundle is still tracked here — re-create the directory or unlink (d) on /bundles.",
			inner))
		lines = append(lines, "", dimLine("  esc back · "+firstPretty(Keys.BundleUnlink)+" unlink (on /bundles)", inner))
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "")
	lines = append(lines, renderBundleViewSwitcher(m, inner))

	preview, ok := m.bundlePreviews[b.ID]
	if m.bundleDetailView == bundleViewComponents {
		switch {
		case !ok:
			lines = append(lines, "", dimLine("  loading preview…", inner))
		case preview.Err != nil:
			lines = append(lines, "", redLine("  preview failed: "+preview.Err.Error()))
		case preview.Fallback:
			lines = append(lines, "")
			lines = append(lines, dimLine(
				"  diff via timestamp comparison (org has no source tracking)",
				inner))
			lines = append(lines, dimLine(
				"  · no conflict detection · org-side deletions not detected · "+firstPretty(Keys.BundleRefreshDiff)+" to refresh",
				inner))
		}
	} else {
		crumb := "/"
		if m.bundleFilesCwd != "" {
			crumb = "/" + m.bundleFilesCwd
		}
		lines = append(lines, "", dimLine("  "+crumb, inner))
	}

	wantBody := m.bundleDetailView == bundleViewFiles || (ok && preview.Err == nil)
	if wantBody {
		lines = append(lines, "")
		usedH := len(lines)
		remaining := innerH - usedH - 2 // -2 for footer hint + spacing
		if remaining < 3 {
			remaining = 3
		}
		bodyLines := m.renderBundleDetailBody(inner, remaining)
		lines = append(lines, bodyLines...)
	}

	hint := bundleDetailFooterHint(m.bundleDetailView)
	lines = append(lines, "", dimLine(hint, inner))
	return strings.Join(lines, "\n")
}

func bundleDetailFooterHint(view bundleDetailView) string {
	viewKeys := firstPretty(Keys.PrevView) + " " + firstPretty(Keys.NextView)
	switch view {
	case bundleViewFiles:
		return "  ↵ open dir / .. up · " + firstPretty(Keys.BundleOpen) + " open file · " + viewKeys + " switch view · esc back"
	default:
		return "  ↵ open org-side · " + firstPretty(Keys.BundleOpen) + " reveal on disk · " + viewKeys +
			" switch view · " + firstPretty(Keys.BundleRetrieve) + " retrieve · " + firstPretty(Keys.BundleDeploy) +
			" deploy · " + firstPretty(Keys.BundleValidate) + " validate · " + firstPretty(Keys.BundleRefreshDiff) +
			" refresh · " + firstPretty(Keys.BundleYankPath) + " yank path · esc back"
	}
}

func renderBundleViewSwitcher(m Model, inner int) string {
	active := lipgloss.NewStyle().
		Foreground(theme.Fg).
		Bold(true)
	idle := lipgloss.NewStyle().Foreground(theme.Muted)

	compsLabel := "Components · " + intToStr(m.bundleDetailList.Len())
	filesLabel := "Bundle files"
	if m.bundleDetailView == bundleViewFiles {
		filesLabel = "Bundle files · " + intToStr(m.bundleFilesList.Len())
	}

	var compsRendered, filesRendered string
	if m.bundleDetailView == bundleViewComponents {
		compsRendered = active.Render("[ " + compsLabel + " ]")
		filesRendered = idle.Render("  " + filesLabel + "  ")
	} else {
		compsRendered = idle.Render("  " + compsLabel + "  ")
		filesRendered = active.Render("[ " + filesLabel + " ]")
	}
	return "  " + compsRendered + "  " + filesRendered
}

func (m Model) renderBundleDetailBody(inner, budget int) []string {
	if m.bundleDetailView == bundleViewFiles {
		return m.renderBundleFilesBody(inner, budget)
	}
	rows := m.bundleDetailList.Filtered()
	cols := bundleDetailListCols()
	cell := func(row, col int) string {
		if row < 0 || row >= len(rows) || col < 0 || col >= len(cols) {
			return ""
		}
		return resolvedCellByID(mustResolveColumns(bundleDetailColumnSchema()), rows[row], cols[col].Name)
	}
	recolor := func(row, col int, base lipgloss.Style) lipgloss.Style {
		if row < 0 || row >= len(rows) {
			return base
		}
		return recolorBundleDetailRow(rows[row], col, base)
	}
	title := bundleDetailTitle(len(rows))
	model := listRenderModel{
		Title:       title,
		State:       &m.bundleDetailTable,
		Search:      m.bundleDetailList.SearchPtr(),
		Cols:        cols,
		N:           len(rows),
		Cursor:      m.bundleDetailList.Cursor(),
		Cell:        cell,
		Recolor:     recolor,
		Empty:       "  nothing to retrieve, deploy, or resolve — bundle is in sync",
		DataVersion: int(m.bundleDetailList.Version()),
	}
	return renderListModel(m, model, m.focus, inner, budget)
}

func (m Model) renderBundleFilesBody(inner, budget int) []string {
	rows := m.bundleFilesList.Filtered()
	cols := bundleFileListCols()
	cell := func(row, col int) string {
		if row < 0 || row >= len(rows) || col < 0 || col >= len(cols) {
			return ""
		}
		return resolvedCellByID(mustResolveColumns(bundleFileColumnSchema()), rows[row], cols[col].Name)
	}
	recolor := func(row, col int, base lipgloss.Style) lipgloss.Style {
		if row < 0 || row >= len(rows) {
			return base
		}
		r := rows[row]
		if r.IsParent || r.IsDir {
			if col == 0 {
				return base.Foreground(theme.Blue).Bold(true)
			}
		}
		return base
	}
	title := "FILES · " + intToStr(len(rows))
	model := listRenderModel{
		Title:       title,
		State:       &m.bundleFilesTable,
		Search:      m.bundleFilesList.SearchPtr(),
		Cols:        cols,
		N:           len(rows),
		Cursor:      m.bundleFilesList.Cursor(),
		Cell:        cell,
		Recolor:     recolor,
		Empty:       "  empty directory",
		DataVersion: int(m.bundleFilesList.Version()),
	}
	return renderListModel(m, model, m.focus, inner, budget)
}

func bundleDetailTitle(n int) string {
	return "COMPONENTS · " + intToStr(n)
}

func kvRow(label, value string, inner int) string {
	left := lipgloss.NewStyle().Foreground(theme.Muted).Render(label)
	right := lipgloss.NewStyle().Foreground(theme.Fg).Render(value)
	pad := inner - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}
