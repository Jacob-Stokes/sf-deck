package redact

import (
	"strings"
	"testing"
)

func TestStringRedactsRegisteredAndKnownSecrets(t *testing.T) {
	registered := "runtime-secret-\\\"redact-test-7Lq9"
	RegisterSecret(registered)
	salesforce := "00D000000000001!AQ0AQK9abcdefghijklmnopqrstuvwxyz012345"
	github := "github_pat_11AA22BB33CC44DD55EE66FF77"
	privateKey := "-----BEGIN PRIVATE KEY-----\nsecret material\n-----END PRIVATE KEY-----"

	got := String(strings.Join([]string{
		registered,
		`runtime-secret-\\\"redact-test-7Lq9`,
		"Bearer bearer-secret-value",
		salesforce,
		"force://user:password@example.test",
		github,
		privateKey,
	}, "\n"))
	for _, secret := range []string{registered, "bearer-secret-value", salesforce, "password", github, "secret material"} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted output contains %q: %q", secret, got)
		}
	}
}

func TestBytesPreservesNonSecretBinaryData(t *testing.T) {
	in := []byte{0xff, 0x00, 'o', 'k', 0xfe}
	got := Bytes(in)
	if string(got) != string(in) {
		t.Fatalf("Bytes() = %v, want %v", got, in)
	}
}

func TestMapRedactsNestedStringsWithoutMutatingInput(t *testing.T) {
	secret := "nested-runtime-secret-redact-test"
	RegisterSecret(secret)
	in := map[string]any{"error": map[string]any{"message": secret}}
	out := Map(in)
	if got := out["error"].(map[string]any)["message"]; got != replacement {
		t.Fatalf("nested message = %v", got)
	}
	if got := in["error"].(map[string]any)["message"]; got != secret {
		t.Fatalf("input was mutated: %v", got)
	}
}
