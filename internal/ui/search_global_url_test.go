package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sfurl"
)

func TestRecognizeURL_BareID(t *testing.T) {
	r := recognizeURL("01p5g00000ABCDEAAA") // 18-char ApexClass id
	if r == nil {
		t.Fatal("expected URL recognition for bare 18-char ApexClass Id")
	}
	if r.Label != "APEX CLASS" {
		t.Errorf("Label = %q, want APEX CLASS", r.Label)
	}
	if r.Enter == nil {
		t.Error("expected Enter closure for bare ApexClass Id")
	}
}

func TestRecognizeURL_RecordPath(t *testing.T) {
	r := recognizeURL("https://acme.lightning.force.com/lightning/r/Account/0011x00000ABCDE/view")
	if r == nil {
		t.Fatal("expected URL recognition for /lightning/r/ path")
	}
	if r.Label != "RECORD · Account" {
		t.Errorf("Label = %q, want RECORD · Account", r.Label)
	}
	if r.Enter == nil {
		t.Error("expected Enter closure for record URL")
	}
}

func TestRecognizeURL_Garbage(t *testing.T) {
	if r := recognizeURL("hello world"); r != nil {
		t.Fatalf("expected nil for non-URL text, got %#v", r)
	}
	if r := recognizeURL(""); r != nil {
		t.Fatalf("expected nil for empty input, got %#v", r)
	}
}

func TestRecognizeURL_UnsupportedSetupSection_ReturnsNil(t *testing.T) {
	// Setup pages we don't model fail Parse; recognizeURL should
	// return nil so the modal stays in fuzzy-search mode.
	if r := recognizeURL("https://acme.lightning.force.com/lightning/setup/SharingRules/home"); r != nil {
		t.Fatalf("expected nil for unsupported setup page, got Label=%q", r.Label)
	}
}

func TestNavigateFromParsed_NilWhenIDMissing(t *testing.T) {
	// Flow URL with no embedded id should not produce a navigator.
	p := sfurl.Parsed{
		Kind:  devproject.KindFlow,
		Extra: map[string]string{},
	}
	if fn := navigateFromParsed(p); fn != nil {
		t.Error("expected nil navigator for Flow with no Id")
	}
}
