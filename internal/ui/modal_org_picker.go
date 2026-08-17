package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

type orgPickerOption struct {
	Org      sf.Org
	Picked   bool
	Disabled bool   // already has an OrgProject under the target DP
	Hint     string // explanation when disabled
}

type orgPickerState struct {
	Title    string
	Hint     string
	Items    []orgPickerOption
	Cursor   int
	OnCommit func(picked []string) tea.Cmd
}

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
