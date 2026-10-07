package ui

import (
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/redact"
)

func TestFlashRedactsRegisteredSecrets(t *testing.T) {
	secret := "ui-runtime-secret-redaction-test"
	redact.RegisterSecret(secret)
	var model Model
	model.flash("request failed: Bearer " + secret)
	if strings.Contains(model.banner, secret) {
		t.Fatalf("flash banner retained registered secret: %q", model.banner)
	}
}
