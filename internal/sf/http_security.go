package sf

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// authenticatedHTTPClient returns a copy of base that can only follow
// redirects within the HTTPS Salesforce origin that issued the session.
// Copying avoids mutating a client while another request may be using it.
func authenticatedHTTPClient(base *http.Client, instanceURL string) (*http.Client, error) {
	origin, err := validateSalesforceInstanceURL(instanceURL)
	if err != nil {
		return nil, err
	}
	if base == nil {
		base = &http.Client{Timeout: cfgHTTPTimeout()}
	}
	next := *base
	next.CheckRedirect = sameOriginRedirectPolicy(origin)
	return &next, nil
}

func validateSalesforceInstanceURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid Salesforce instance URL: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return nil, fmt.Errorf("salesforce instance URL must use https")
	}
	if u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("salesforce instance URL must contain a host and no credentials")
	}
	if u.Path != "" && u.Path != "/" {
		return nil, fmt.Errorf("salesforce instance URL must not contain a path")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("salesforce instance URL must not contain a query or fragment")
	}
	return u, nil
}

func sameOriginRedirectPolicy(origin *url.URL) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		if !sameOrigin(origin, req.URL) {
			return fmt.Errorf("refusing authenticated redirect from %s to %s", originString(origin), originString(req.URL))
		}
		return nil
	}
}

func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil || !strings.EqualFold(a.Scheme, b.Scheme) {
		return false
	}
	return strings.EqualFold(a.Hostname(), b.Hostname()) && effectivePort(a) == effectivePort(b)
}

func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return "443"
	case "http":
		return "80"
	}
	return ""
}

func originString(u *url.URL) string {
	if u == nil {
		return "<invalid>"
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
}

func clientWithTimeout(base *http.Client, timeout time.Duration) *http.Client {
	if base == nil {
		return &http.Client{Timeout: timeout}
	}
	next := *base
	next.Timeout = timeout
	return &next
}
