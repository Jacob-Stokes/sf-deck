package ui

// Org-multi-picker modal — used by the "create DevProject + initial
// OrgProjects" wizard. The choice modal is single-select; we want the
// user to tick which orgs to provision OrgProjects for, all at once.
//
// Built on the same modalBox primitive; shares cursor / key dispatch
// with the simpler modals. State lives on Model.orgPicker (nil when
// hidden); render path mirrors the other overlays.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// orgPickerOption is one row in the picker — the org plus a few
// derived fields for rendering.
type orgPickerOption struct {
	Org      sf.Org
	Picked   bool
	Disabled bool   // already has an OrgProject under the target DP
	Hint     string // explanation when disabled
}

// orgPickerState is the multi-select org picker. Rendered when not nil.
type orgPickerState struct {
	Title  string
	Hint   string
	Items  []orgPickerOption
	Cursor int
	// OnCommit fires with the picked org-user list; called after the
	// modal closes so subsequent state changes don't fight the modal's
	// own dispatch path.
	OnCommit func(picked []string) tea.Cmd
}

// openOrgPicker installs the multi-select modal.
func (m *Model) openOrgPicker(state *orgPickerState) tea.Cmd {
	if state == nil {
		return nil
	}
	if state.Cursor < 0 || state.Cursor >= len(state.Items) {
		state.Cursor = 0
	}
	m.orgPicker = state
	return nil
}

// renderOrgPicker draws the modal box. Empty when no picker active.
func (m Model) renderOrgPicker() string {
	if m.orgPicker == nil {
		return ""
	}
	w := modalWidth(m.width, 56, 90)
	inner := w - 4
	st := m.orgPicker
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(theme.Fg).Bold(true).Render(st.Title))
	if st.Hint != "" {
		lines = append(lines, theme.Subtle.Render(st.Hint))
	}
	lines = append(lines, "")
	for i, opt := range st.Items {
		mark := "[ ]"
		if opt.Picked {
			mark = "[x]"
		}
		label := opt.Org.Display()
		if label == "" {
			label = opt.Org.Username
		}
		badge := orgKindBadge(opt.Org)
		row := fmt.Sprintf("  %s  %s%s", mark, label, badge)
		if opt.Hint != "" {
			row += "  " + theme.Subtle.Render("("+opt.Hint+")")
		}
		style := lipgloss.NewStyle().Foreground(theme.Fg)
		if opt.Disabled {
			style = lipgloss.NewStyle().Foreground(theme.FgDim)
		}
		if i == st.Cursor {
			barColor := theme.BorderHi
			row = lipgloss.NewStyle().Foreground(barColor).Render("▌") + " " + style.Bold(true).Render(row[2:])
		} else {
			row = style.Render(row)
		}
		lines = append(lines, row)
	}
	lines = append(lines, "")
	lines = append(lines, theme.Subtle.Render("space toggle · enter commit · esc cancel"))
	body := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Width(inner).
		Render(body)
	return box
}

// orgKindBadge formats the small kind label rendered to the right of
// each org name. Mirrors the badge palette from headerOrgPill so the
// visual language stays consistent.
func orgKindBadge(o sf.Org) string {
	switch {
	case o.IsScratch:
		return "  " + theme.Subtle.Render("scratch")
	case o.IsSandbox:
		return "  " + theme.Subtle.Render("sandbox")
	case o.IsDevHub:
		return "  " + theme.Subtle.Render("devhub")
	}
	return "  " + theme.Subtle.Render("prod")
}

// handleOrgPickerKey dispatches keys while the picker is up.
func (m *Model) handleOrgPickerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.orgPicker == nil {
		return *m, nil
	}
	st := m.orgPicker
	key := msg.String()
	switch key {
	case "esc":
		m.orgPicker = nil
		return *m, nil
	case "up", "k":
		if st.Cursor > 0 {
			st.Cursor--
		}
		return *m, nil
	case "down", "j":
		if st.Cursor < len(st.Items)-1 {
			st.Cursor++
		}
		return *m, nil
	case "space", " ":
		if st.Cursor >= 0 && st.Cursor < len(st.Items) {
			it := &st.Items[st.Cursor]
			if !it.Disabled {
				it.Picked = !it.Picked
			}
		}
		return *m, nil
	case "enter":
		var picked []string
		for _, opt := range st.Items {
			if opt.Picked && !opt.Disabled {
				picked = append(picked, opt.Org.Username)
			}
		}
		commit := st.OnCommit
		m.orgPicker = nil
		if commit == nil {
			return *m, nil
		}
		return *m, commit(picked)
	}
	return *m, nil
}
