package diff

import (
	"strings"
	"testing"
)

func TestPrettyXMLReflowsSingleLine(t *testing.T) {
	// A field def as readMetadata returns it: all on one line.
	in := `<fullName>Destination_Risk_Rating__c</fullName><label>Destination Risk Rating</label><required>false</required><type>Picklist</type>`
	out := PrettyXML(in)
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 leaf lines, got %d:\n%s", len(lines), out)
	}
	// Each leaf on its own line, content intact.
	wantContain := []string{"<fullName>Destination_Risk_Rating__c</fullName>",
		"<label>Destination Risk Rating</label>", "<type>Picklist</type>"}
	for _, w := range wantContain {
		found := false
		for _, l := range lines {
			if strings.TrimSpace(l) == w {
				found = true
			}
		}
		if !found {
			t.Errorf("missing leaf line %q in:\n%s", w, out)
		}
	}
}

func TestPrettyXMLIndentsContainers(t *testing.T) {
	in := `<records><fullName>X</fullName><fields><fullName>F</fullName><type>Text</type></fields></records>`
	out := PrettyXML(in)
	lines := strings.Split(out, "\n")
	// <records> opens (depth 0), children indented (depth 1), nested
	// <fields> opens (depth 1) with its children at depth 2.
	var fieldLine, fIndent string
	for _, l := range lines {
		if strings.Contains(l, "<fields>") {
			fieldLine = l
		}
		if strings.Contains(l, "<type>Text</type>") {
			fIndent = l
		}
	}
	if !strings.HasPrefix(fieldLine, "  <fields>") {
		t.Errorf("fields not indented under records: %q", fieldLine)
	}
	if !strings.HasPrefix(fIndent, "    <type>") {
		t.Errorf("nested type not double-indented: %q", fIndent)
	}
}

func TestPrettyXMLLeavesMultilineAlone(t *testing.T) {
	in := "<a>\n  <b>1</b>\n  <c>2</c>\n</a>"
	if PrettyXML(in) != in {
		t.Errorf("already-formatted XML should pass through unchanged")
	}
}

func TestPrettyXMLMakesDiffAlign(t *testing.T) {
	// Two field defs differing only in <label> — pretty-printed, the
	// diff should be ONE changed line, not whole-component.
	a := `<fullName>F</fullName><label>Old</label><type>Text</type>`
	b := `<fullName>F</fullName><label>New</label><type>Text</type>`
	res := Text(PrettyXML(a), PrettyXML(b))
	if res.Added != 1 || res.Removed != 1 {
		t.Errorf("label-only change: added=%d removed=%d, want 1/1 (aligned diff)", res.Added, res.Removed)
	}
}
