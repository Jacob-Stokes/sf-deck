package ui

// /exec — anonymous Apex workspace. Mirrors /soql's shape: an
// always-visible editor body, plus Saved / History sibling subtabs,
// plus an Output subtab that shows the most recent run's debug log.
//
// The editor itself is a multi-line textarea (bubbles/textarea) so
// users can compose 5-50 line snippets in-app. `e` opens $EDITOR
// when they want their real editor; enter (when blurred) runs the
// current body.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// renderExec branches by subtab. Saved / Output / History delegate
// to their own renderers (declared in tab_exec_library.go / log
// viewer); the Editor is the default body.
func (m Model) renderExec(w, innerH int) string {
	return m.dispatchSubtab(w, innerH, m.tabSubtabs(), m.execSubtabIdx,
		map[Subtab]subtabBranch{
			SubtabExecOutput:  {Render: m.renderExecOutput},
			SubtabExecSaved:   {Render: m.renderExecSaved},
			SubtabExecHistory: {Render: m.renderExecHistory},
		},
		subtabBranch{Render: m.renderExecEditor},
	)
}

// renderExecEditor is the Editor subtab body.
func (m Model) renderExecEditor(w, innerH int) string {
	inner := w - 4
	titleSuffix := ""
	if m.execCaptureLog {
		titleSuffix += "  " + lipgloss.NewStyle().Foreground(theme.Cyan).Render("[log]")
	}
	if m.execEditing {
		titleSuffix += "  " + lipgloss.NewStyle().Foreground(theme.Yellow).Render("[editing]")
	}

	var lines []string
	lines = append(lines, sectionTitle("EXEC"+titleSuffix))

	width := inner - 2
	if width < 20 {
		width = 20
	}
	taHeight := taLineCount(m.execInput.Value())
	if taHeight < 5 {
		taHeight = 5
	}
	if taHeight > 20 {
		taHeight = 20
	}
	m.execInput.SetWidth(width)
	m.execInput.SetHeight(taHeight)
	lines = append(lines, m.execInput.View())

	help := "  "
	if m.execEditing {
		help += "ctrl+enter run · esc done · ctrl+u clear"
	} else {
		help += firstPretty(Keys.ExecEdit) + " edit · " +
			firstPretty(Keys.Drill) + " run · " +
			firstPretty(Keys.ExecSave) + " save · " +
			firstPretty(Keys.ExecExternalEditor) + " $EDITOR · " +
			firstPretty(Keys.ExecToggleLog) + " log"
	}
	lines = append(lines, dimLine(help, inner))
	lines = append(lines, "")

	switch {
	case m.execRunning:
		lines = append(lines, theme.Subtle.Render("  running…"))
	case m.execErr != nil:
		lines = append(lines, redLine("  "+m.execErr.Error()))
	case m.execResult.Body != "":
		lines = append(lines, renderExecResultSummary(m.execResult, inner)...)
	default:
		lines = append(lines, theme.Subtle.Render("  no result yet — write some Apex, press "+firstPretty(Keys.Drill)+" to run"))
	}

	return strings.Join(lines, "\n")
}

// renderExecResultSummary lays out the compile + run status block
// shown below the editor after a run lands. Output (the full debug
// log) lives on its own subtab — this is the at-a-glance summary.
func renderExecResultSummary(r sf.ExecuteAnonymousResult, inner int) []string {
	var lines []string
	tookMs := r.Took.Milliseconds()
	var statusLine string
	switch {
	case !r.Compiled:
		statusLine = lipgloss.NewStyle().Foreground(theme.Red).Bold(true).
			Render("  COMPILE ERROR") + dimLine(
			fmt.Sprintf("  · %dms · line %d:%d", tookMs, r.Line, r.Column), inner)
	case !r.Success:
		statusLine = lipgloss.NewStyle().Foreground(theme.Red).Bold(true).
			Render("  RUNTIME ERROR") + dimLine(
			fmt.Sprintf("  · %dms · line %d:%d", tookMs, r.Line, r.Column), inner)
	default:
		statusLine = lipgloss.NewStyle().Foreground(theme.Green).Bold(true).
			Render("  SUCCESS") + dimLine(
			fmt.Sprintf("  · %dms", tookMs), inner)
	}
	lines = append(lines, statusLine)

	if !r.Compiled && r.CompileProblem != "" {
		lines = append(lines, "", dimLine("  compile problem:", inner))
		for _, ln := range wrapLines(r.CompileProblem, inner-4) {
			lines = append(lines, "    "+ln)
		}
	}
	if r.Compiled && !r.Success && r.ExceptionMessage != "" {
		lines = append(lines, "", dimLine("  exception:", inner))
		for _, ln := range wrapLines(r.ExceptionMessage, inner-4) {
			lines = append(lines, "    "+ln)
		}
		if r.ExceptionStack != "" {
			lines = append(lines, "", dimLine("  stack:", inner))
			for _, ln := range strings.Split(r.ExceptionStack, "\n") {
				if ln == "" {
					continue
				}
				lines = append(lines, "    "+ln)
			}
		}
	}
	if r.LogID != "" {
		lines = append(lines, "", dimLine(
			"  debug log captured · open the Output subtab to read it ("+r.LogID+")",
			inner))
	}
	return lines
}

// renderExecOutput is the Output subtab — full debug-log scroll
// viewer for the most recent run. Reuses the shared codeView path
// (same one /apex class detail uses) so j/k scrolling, gutter line
// numbers, and the focused/unfocused cursor styling come for free.
//
// When no log has been captured yet (no run, or log capture was
// off), shows the editor-style "no result" hint.
func (m Model) renderExecOutput(w, innerH int) string {
	if m.execResult.LogBody == "" {
		hint := "  no log captured yet — run some Apex with [log] on first"
		if m.execResult.Body != "" && !m.execCaptureLog {
			hint = "  log capture is off — toggle with " + firstPretty(Keys.ExecToggleLog) + " and re-run"
		}
		return theme.Subtle.Render(hint)
	}
	d, ok := m.activeOrgState()
	if !ok {
		return noOrgPlaceholder()
	}
	inner := w - 4
	var lines []string
	title := "OUTPUT"
	if m.execResult.LogID != "" {
		title += " · " + m.execResult.LogID
	}
	lines = append(lines, sectionTitle(title))
	lines = append(lines, dimLine(
		"  "+firstPretty(Keys.MoveUp)+"/"+firstPretty(Keys.MoveDown)+" scroll · "+
			firstPretty(Keys.SearchStart)+" search · esc back", inner))
	lines = append(lines, "")
	body := m.renderCodeView(d, codeViewSpec{
		BodyID:  "exec-log:" + m.execResult.LogID,
		Body:    m.execResult.LogBody,
		Lang:    "log",
		Inner:   inner,
		Height:  innerH - len(lines) - 1,
		Focused: m.focus == focusMain,
	})
	lines = append(lines, body...)
	return strings.Join(lines, "\n")
}

// renderExecSaved + renderExecHistory delegate to tab_exec_library.go.
func (m Model) renderExecSaved(w, innerH int) string {
	return m.renderExecSavedSubtab(w, innerH)
}

func (m Model) renderExecHistory(w, innerH int) string {
	return m.renderExecHistorySubtab(w, innerH)
}

// taLineCount counts newlines in s; used to size the textarea.
func taLineCount(s string) int {
	return strings.Count(s, "\n") + 1
}

// wrapLines hard-wraps long strings to width. Splits on whitespace
// when possible; falls back to mid-word break for absurdly long
// tokens (rare in compile messages).
func wrapLines(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	var out []string
	for _, paragraph := range strings.Split(s, "\n") {
		if paragraph == "" {
			out = append(out, "")
			continue
		}
		words := strings.Fields(paragraph)
		line := ""
		for _, w := range words {
			if len(line)+1+len(w) > width {
				if line != "" {
					out = append(out, line)
				}
				if len(w) > width {
					for len(w) > width {
						out = append(out, w[:width])
						w = w[width:]
					}
				}
				line = w
				continue
			}
			if line == "" {
				line = w
			} else {
				line += " " + w
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
