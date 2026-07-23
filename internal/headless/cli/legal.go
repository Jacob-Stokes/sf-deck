package cli

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	productlegal "github.com/Jacob-Stokes/sf-deck/internal/legal"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

type legalStatusData struct {
	Accepted        bool   `json:"accepted"`
	PolicyVersion   string `json:"policy_version"`
	AcceptedVersion string `json:"accepted_version,omitempty"`
	AcceptedAt      string `json:"accepted_at,omitempty"`
	PrivacyURL      string `json:"privacy_url"`
	TermsURL        string `json:"terms_url"`
}

func dispatchLegal(args Args, stdout io.Writer, mode headless.WriteMode) int {
	verb := args.Verb
	if verb == "" {
		verb = "status"
	}
	switch verb {
	case "status":
		return legalStatus(args.Rest, stdout, mode)
	case "accept":
		return legalAccept(args.Rest, stdout, mode)
	}
	r := headless.Fail("legal."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown legal verb %q (expected status|accept)", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func loadLegalSettings(command string, stdout io.Writer, mode headless.WriteMode) (*settings.Settings, int) {
	st, err := settings.Load()
	if err != nil {
		r := headless.Fail(command, "", headless.ErrInternal,
			"load settings: "+err.Error(), nil)
		_ = r.Write(stdout, mode)
		return nil, headless.ExitCodeFor(r)
	}
	return st, headless.ExitOK
}

func legalStatus(rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("legal status")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("legal.status", err, stdout, mode)
	}
	st, code := loadLegalSettings("legal.status", stdout, mode)
	if st == nil {
		return code
	}
	version, acceptedAt := st.LegalAcceptance()
	data := legalStatusData{
		Accepted:        st.LegalAccepted(productlegal.PolicyVersion),
		PolicyVersion:   productlegal.PolicyVersion,
		AcceptedVersion: version,
		AcceptedAt:      acceptedAt,
		PrivacyURL:      productlegal.PrivacyURL,
		TermsURL:        productlegal.TermsURL,
	}
	if mode == headless.TextMode {
		if data.Accepted {
			fmt.Fprintf(stdout, "accepted · policy %s · %s\n", data.PolicyVersion, data.AcceptedAt)
		} else {
			fmt.Fprintf(stdout, "not accepted · review %s and %s\n", data.TermsURL, data.PrivacyURL)
		}
		return headless.ExitOK
	}
	r := headless.Success("legal.status", "", "", false, data)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func legalAccept(rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("legal accept")
	yes := fs.Bool("yes", false, "confirm acceptance non-interactively")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("legal.accept", err, stdout, mode)
	}
	if !*yes {
		return writeArgErr("legal.accept",
			errors.New("--yes is required after reviewing the privacy notice and user agreement"),
			stdout, mode)
	}
	st, code := loadLegalSettings("legal.accept", stdout, mode)
	if st == nil {
		return code
	}
	already := st.LegalAccepted(productlegal.PolicyVersion)
	st.AcceptLegal(productlegal.PolicyVersion, time.Now())
	if err := st.Save(); err != nil {
		r := headless.Fail("legal.accept", "", headless.ErrInternal,
			"save acceptance: "+err.Error(), nil)
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	version, acceptedAt := st.LegalAcceptance()
	data := legalStatusData{
		Accepted:        true,
		PolicyVersion:   productlegal.PolicyVersion,
		AcceptedVersion: version,
		AcceptedAt:      acceptedAt,
		PrivacyURL:      productlegal.PrivacyURL,
		TermsURL:        productlegal.TermsURL,
	}
	if mode == headless.TextMode {
		fmt.Fprintf(stdout, "accepted · policy %s · %s\n", data.PolicyVersion, data.AcceptedAt)
		return headless.ExitOK
	}
	r := headless.Success("legal.accept", "", "", !already, data)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// WriteLegalRequired emits the stable headless error used before app.Open.
// This guarantees no Salesforce org is enumerated before acknowledgement.
func WriteLegalRequired(command string, stdout io.Writer, mode headless.WriteMode) int {
	r := headless.Fail(command, "", headless.ErrInvalidArgument,
		"review and accept the sf-deck user agreement and privacy notice before accessing Salesforce",
		map[string]any{
			"accept_command": "sf-deck legal accept --yes",
			"policy_version": productlegal.PolicyVersion,
			"privacy_url":    productlegal.PrivacyURL,
			"terms_url":      productlegal.TermsURL,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}
