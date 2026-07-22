package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// placeTitleAndProjects is the stacked-mode header layout math: projects
// right-align on the title line when there's room, else fall to their own
// line. This is what moves TAGS/PROJECTS out of the truncation-prone body.
func TestPlaceTitleAndProjects(t *testing.T) {
	// Room to right-align: single line, projects flush right, title still
	// at the front.
	got := placeTitleAndProjects("Summer_School_Flow", "[proj]", 60)
	if strings.Contains(got, "\n") {
		t.Fatalf("wide inner: expected single line, got wrapped:\n%q", got)
	}
	if !strings.HasPrefix(got, "Summer_School_Flow") {
		t.Fatalf("title not at front: %q", got)
	}
	if !strings.HasSuffix(got, "[proj]") {
		t.Fatalf("projects not right-aligned to the end: %q", got)
	}
	if w := ansi.StringWidth(got); w != 60 {
		t.Fatalf("right-aligned line width = %d, want inner 60", w)
	}

	// No room (title+projects already fill the width): projects drop to
	// their own indented line rather than overflowing.
	longTitle := strings.Repeat("x", 55)
	got = placeTitleAndProjects(longTitle, "[proj]", 60)
	if !strings.Contains(got, "\n  [proj]") {
		t.Fatalf("tight inner: projects should fall to their own line, got:\n%q", got)
	}

	// No projects: title returned untouched, no trailing padding/newline.
	if got := placeTitleAndProjects("JustTitle", "", 60); got != "JustTitle" {
		t.Fatalf("empty projects should return title unchanged, got %q", got)
	}
}

// With no devProjects store, the stacked title composer returns just the
// styled title (+ any flag pills) — no tags/projects to fold in, no panic.
func TestStackedTitleNoStore(t *testing.T) {
	m := Model{}
	m.sidebarStacked = true
	got := m.stackedTitleWithTagsProjects("MyFlow", "", "flow", "301x", "user@org", 60)
	if got != "MyFlow" {
		t.Fatalf("nil store: expected bare title, got %q", got)
	}
}
