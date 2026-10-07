package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/buildinfo"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/updatecheck"
)

type uiUpdateStub struct {
	result updatecheck.Result
	err    error
	calls  int
	opts   updatecheck.Options
}

func (s *uiUpdateStub) Check(_ context.Context, _ string, opts updatecheck.Options) (updatecheck.Result, error) {
	s.calls++
	s.opts = opts
	return s.result, s.err
}

func TestAutomaticUpdateCheckAppliesAvailableRelease(t *testing.T) {
	buildinfo.Set("0.1.0", "abc", "today")
	t.Cleanup(func() { buildinfo.Set("dev", "none", "unknown") })
	stub := &uiUpdateStub{result: updatecheck.Result{
		CurrentVersion:  "v0.1.0",
		LatestVersion:   "v0.2.0",
		UpdateAvailable: true,
		Kind:            "minor",
	}}
	m := Model{modelServices: modelServices{
		settings: &settings.Settings{},
		updates:  stub,
	}}
	cmd := m.updateCheckCmd(false)
	if cmd == nil {
		t.Fatal("release build should schedule automatic check")
	}
	msg, ok := cmd().(updateCheckMsg)
	if !ok {
		t.Fatalf("message type = %T", cmd())
	}
	m.applyUpdateCheck(msg)
	if !m.updateChecked || !m.updateResult.UpdateAvailable || stub.calls != 1 {
		t.Fatalf("state checked=%v result=%+v calls=%d", m.updateChecked, m.updateResult, stub.calls)
	}
	if notice := m.updateNoticeText(); !strings.Contains(notice, "v0.2.0") {
		t.Fatalf("notice = %q", notice)
	}
}

func TestAutomaticUpdateCheckSkipsDevelopmentAndDemo(t *testing.T) {
	buildinfo.Set("dev", "none", "unknown")
	stub := &uiUpdateStub{}
	m := Model{modelServices: modelServices{
		settings: &settings.Settings{},
		updates:  stub,
	}}
	if cmd := m.updateCheckCmd(false); cmd != nil {
		t.Fatal("development build scheduled automatic check")
	}

	oldDemo := Demo
	Demo = true
	t.Cleanup(func() { Demo = oldDemo })
	if cmd := m.updateCheckCmd(false); cmd != nil {
		t.Fatal("demo scheduled automatic check")
	}
	cmd := m.updateCheckCmd(true)
	msg := cmd().(updateCheckMsg)
	if msg.err == nil || !strings.Contains(msg.err.Error(), "demo mode") || stub.calls != 0 {
		t.Fatalf("manual demo msg=%+v calls=%d", msg, stub.calls)
	}
}

func TestManualUpdateFailureOpensInfoModal(t *testing.T) {
	m := Model{modelServices: modelServices{settings: &settings.Settings{}}}
	m.applyUpdateCheck(updateCheckMsg{err: errors.New("offline"), manual: true})
	if m.infoModal == nil || m.infoModal.Title != "sf-deck update" {
		t.Fatalf("info modal = %#v", m.infoModal)
	}
	if m.updateErr != "offline" {
		t.Fatalf("updateErr = %q", m.updateErr)
	}
}

func TestAboutModalIncludesReleaseIdentity(t *testing.T) {
	buildinfo.Set("0.1.0", "0123456789abcdef", "2026-07-23")
	t.Cleanup(func() { buildinfo.Set("dev", "none", "unknown") })
	m := Model{modelServices: modelServices{settings: &settings.Settings{}}}
	m.openAboutModal()
	if m.infoModal == nil || m.infoModal.Title != "About sf-deck" {
		t.Fatalf("info modal = %#v", m.infoModal)
	}
	body := ""
	for _, row := range m.infoModal.Rows {
		body += row.Label + " " + row.Body + "\n"
	}
	for _, want := range []string{"v0.1.0", "Apache License 2.0", "Jacob Stokes", "0123456789ab"} {
		if !strings.Contains(body, want) {
			t.Fatalf("about body missing %q:\n%s", want, body)
		}
	}
	if m.infoModal.OnDismiss == nil {
		t.Fatal("About modal should return to Settings on dismiss")
	}
}
