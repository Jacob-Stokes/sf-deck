package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func execSubtabIndex(s Subtab) int {
	switch s {
	case SubtabExecEditor:
		return 0
	case SubtabExecOutput:
		return 1
	case SubtabExecSaved:
		return 2
	case SubtabExecHistory:
		return 3
	}
	return 0
}

func (m *Model) persistExecHistory(msg execResultMsg) {
	if m.devProjects == nil {
		return
	}
	errMsg := ""
	if msg.err != nil {
		errMsg = msg.err.Error()
	}
	_, err := m.devProjects.LogApexHistory(devproject.ApexHistoryEntry{
		OrgUser:          msg.orgUser,
		Body:             msg.body,
		DurationMs:       int(msg.data.Took.Milliseconds()),
		Compiled:         msg.data.Compiled,
		Success:          msg.data.Success,
		CompileProblem:   msg.data.CompileProblem,
		ExceptionMessage: stringOr(msg.data.ExceptionMessage, errMsg),
		Line:             msg.data.Line,
		Column:           msg.data.Column,
		LogID:            msg.data.LogID,
		LogBody:          msg.data.LogBody,
	})
	if err != nil {
		m.flash("history: " + err.Error())
	}
	if d, ok := m.activeOrgState(); ok {
		d.ExecHistoryLoaded = false
	}
}

func (m Model) handleExecExternalEditor() (tea.Model, tea.Cmd) {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		m.flash("$EDITOR not set — set EDITOR=vim (or similar) and try again")
		return m, nil
	}

	tmp, err := os.CreateTemp("", "sf-deck-exec-*.apex")
	if err != nil {
		m.flash("temp file: " + err.Error())
		return m, nil
	}
	path := tmp.Name()
	body := m.execInput.Value()
	if _, err := tmp.WriteString(body); err != nil {
		tmp.Close()
		os.Remove(path)
		m.flash("write temp: " + err.Error())
		return m, nil
	}
	tmp.Close()

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		raw, readErr := os.ReadFile(path)
		os.Remove(path)
		if err != nil && readErr != nil {
			return execEditorClosedMsg{err: fmt.Errorf("editor: %w", err)}
		}
		return execEditorClosedMsg{body: string(raw)}
	})
}

type execEditorClosedMsg struct {
	body string
	err  error
}

type execProdConfirmMsg struct {
	body string
}

// openExecProdGate opens a confirmation modal before running
// anonymous Apex against a production org. Anonymous Apex can
// DELETE records, run flows, fire triggers — everything a normal
// API call can. SBX runs skip this; PROD runs require an explicit
// "I know what I'm doing" click.
//
// The org's display name and the first line of the snippet are
// shown so the user sees both the target and the action at once.
func (m *Model) openExecProdGate(o sfOrg, body string) tea.Cmd {
	preview := firstNonEmptyLine(body)
	if preview == "" {
		preview = "(empty)"
	}
	hint := "PRODUCTION org · " + o.Display() +
		"\nThis snippet will execute against live data:\n  " + preview +
		"\nEnter to run anyway · esc to cancel"
	return m.openChoiceModal(choiceModalState{
		Title: "Run anonymous Apex on PROD?",
		Hint:  hint,
		Options: []choiceOption{
			{Label: "Run on production", Value: "run"},
			{Label: "Cancel", Cancel: true},
		},
		Cursor: 1, // Default to Cancel — explicit confirmation required.
		Save:   func(val any) error { return nil },
		OnSuccessTyped: func(val any) tea.Cmd {
			if val != "run" {
				return nil
			}
			return func() tea.Msg { return execProdConfirmMsg{body: body} }
		},
	})
}

type sfOrg = sf.Org

// stringOr returns a when non-empty, otherwise b.
func stringOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

var _ = strings.TrimSpace
