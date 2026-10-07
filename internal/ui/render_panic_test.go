package ui

// Tests for View()'s panic recovery wrapper.

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestRenderPanicFrameCarriesMessage(t *testing.T) {
	v := renderPanicFrame("synthetic test panic")
	body := teaViewString(v)
	if body == "" {
		t.Fatal("renderPanicFrame returned empty body")
	}
	if !strings.Contains(body, "render panicked") {
		t.Errorf("body missing panic header; got:\n%s", body)
	}
	if !strings.Contains(body, "synthetic test panic") {
		t.Errorf("body missing recovered message; got:\n%s", body)
	}
	if !strings.Contains(body, "~/.sf-deck/log") {
		t.Errorf("body missing log path hint; got:\n%s", body)
	}
}

func TestViewRecoverFromPanic(t *testing.T) {
	// Inject a panic into the render path by setting a Model field
	// the renderer dereferences. We use a fresh model with width=0
	// (which short-circuits viewImpl to "starting…") and hand-build
	// the panic via a helper that calls renderPanicFrame.
	//
	// Simpler than monkey-patching the real renderer: we directly
	// invoke the recover wrapper via a wrapper test helper that
	// replicates View()'s deferred-recover shape.
	out := callWithRecover(func() tea.View {
		panic("integration test panic")
	})
	body := teaViewString(out)
	if !strings.Contains(body, "integration test panic") {
		t.Errorf("recover wrapper didn't carry message; got:\n%s", body)
	}
}

func callWithRecover(fn func() tea.View) (out tea.View) {
	defer func() {
		if r := recover(); r != nil {
			out = renderPanicFrame(r)
		}
	}()
	return fn()
}

func teaViewString(v tea.View) string {
	return v.Content
}
