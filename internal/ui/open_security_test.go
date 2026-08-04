package ui

import "testing"

func TestSafeToOpenTargetURL(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		allowExtension bool
		wantErr        bool
	}{
		{"https", "https://example.my.salesforce.com/lightning", false, false},
		{"http dev", "http://localhost:8080/", false, false},
		{"inspector explicitly allowed", "chrome-extension://abcdefghijklmnop/inspect.html?host=x", true, false},
		{"firefox inspector explicitly allowed", "moz-extension://3f42d3f0-1234/inspect.html?host=x", true, false},
		{"extension denied by default", "chrome-extension://abcdefghijklmnop/inspect.html", false, true},
		{"wrong extension page", "chrome-extension://abcdefghijklmnop/background.html", true, true},
		{"extension with user info", "chrome-extension://user@abcdefghijklmnop/inspect.html", true, true},
		{"file", "file:///tmp/x", false, true},
		{"javascript", "javascript:alert(1)", false, true},
		{"leading flag", "-a Calculator", false, true},
		{"missing host", "https:///path", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := safeToOpenTargetURL(tc.url, tc.allowExtension)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
