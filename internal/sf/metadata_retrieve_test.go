package sf

import "testing"

func TestMetadataComponentKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"MyClass.cls", "MyClass"},
		{"MyClass.cls-meta.xml", ""}, // sidecar skipped
		{"MyTrigger.trigger", "MyTrigger"},
		{"MyTrigger.trigger-meta.xml", ""},
		{"My_Rule.validationRule-meta.xml", "My_Rule"},
		{"Industry.field-meta.xml", "Industry"},
		{"Account.object-meta.xml", "Account"},
		{"Lead_Assignment.flow-meta.xml", "Lead_Assignment"},
		{"Some.page", "Some"},
	}
	for _, c := range cases {
		if got := metadataComponentKey(c.in); got != c.want {
			t.Errorf("metadataComponentKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
