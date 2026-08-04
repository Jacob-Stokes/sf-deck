// Package legal contains the versioned product-policy identifiers shared by
// first-run acknowledgement, headless commands, and product surfaces.
package legal

const (
	// PolicyVersion changes only when users need to acknowledge materially
	// revised terms or data handling. A date keeps the value readable in
	// settings.toml and avoids coupling policy acceptance to app releases.
	PolicyVersion = "2026-07-23"

	PrivacyURL  = "https://sfdeck.dev/privacy.html"
	TermsURL    = "https://sfdeck.dev/terms.html"
	SecurityURL = "https://github.com/Jacob-Stokes/sf-deck/blob/main/.github/SECURITY.md"
)
