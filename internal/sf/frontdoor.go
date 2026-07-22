package sf

// frontdoor.jsp session bridge.
//
// The Analytics REST API is JSON-only and capped at 2000 detail rows;
// the only "give me the full xlsx" endpoint Salesforce ships is the
// classic export URL (<instance>/<reportId>?export=1&xf=xlsx). That
// URL authenticates by UI session cookie, not Bearer token. frontdoor
// is the SF-blessed bridge: POST your access token as `sid` and SF
// hands back a `Set-Cookie: sid=<permanent>` we can use on the next
// hop.
//
// Requires the connected app's OAuth scopes to include `web` (or
// `full`). sf-cli's default `sf org login web` flow has it; pure-JWT
// or `api`-only apps won't, and this path will return the login page
// instead. classicExportViaFrontdoor surfaces that as a clearer error.

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
)

// classicExportViaFrontdoor exchanges the cached Bearer token for a UI
// session cookie via secur/frontdoor.jsp, then GETs the classic export
// URL with that cookie. Returns the xlsx bytes (or a HTML-detection
// error when the connected app's scopes don't include `web`).
func (c *Client) classicExportViaFrontdoor(reportID string) ([]byte, error) {
	c.mu.Lock()
	token := c.accessToken
	base := strings.TrimRight(c.instanceURL, "/")
	c.mu.Unlock()

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("cookie jar: %w", err)
	}
	// Don't auto-follow — frontdoor's 302 carries the Set-Cookie we need;
	// once the cookies are in the jar we issue the export request
	// directly. CheckRedirect returning ErrUseLastResponse stops at the
	// first redirect without erroring.
	httpc := &http.Client{
		Timeout: c.http.Timeout,
		Jar:     jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Step 1: POST the access token as sid. Body-encoded keeps the
	// token out of the URL/access-log/Referer.
	form := url.Values{}
	form.Set("sid", token)
	// retURL is mandatory — SF won't issue the cookie otherwise. We
	// point it at the export URL so the 302 Location is also useful;
	// we still issue the export GET ourselves below for clarity.
	retURL := fmt.Sprintf("/%s?export=1&enc=UTF-8&xf=xlsx&isdtp=p1", reportID)
	form.Set("retURL", retURL)

	req, err := http.NewRequest("POST",
		base+"/secur/frontdoor.jsp",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "sf-deck/0.1")

	fdStart := time.Now()
	defer func() { fireOnCall(c.alias, []string{"POST", "/secur/frontdoor.jsp"}, nil, time.Since(fdStart)) }()
	resp, err := httpc.Do(req)
	if err != nil {
		applog.Error("frontdoor.post", map[string]any{"err": err.Error()})
		return nil, fmt.Errorf("frontdoor POST: %w", err)
	}
	// Read the body (small; just for diagnostics) before closing.
	var fdBody []byte
	if resp.Body != nil {
		fdBody, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}
	cookieCount := len(resp.Header.Values("Set-Cookie"))
	applog.Info("frontdoor.response", map[string]any{
		"status":             resp.StatusCode,
		"location":           resp.Header.Get("Location"),
		"set_cookie_count":   cookieCount,
		"has_session_cookie": jarHasSession(jar, base),
		"content_type":       resp.Header.Get("Content-Type"),
		"bytes":              len(fdBody),
	})
	// Frontdoor's success path is 302; an unauthenticated/missing-scope
	// response is 200 + login HTML. Also accept 303/307 just in case
	// SF changes behaviour.
	switch resp.StatusCode {
	case http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusOK:
		// Continue.
	default:
		applog.Dump([]string{"frontdoor", "unexpected", fmt.Sprintf("%d", resp.StatusCode)},
			"html", fdBody)
		return nil, fmt.Errorf("frontdoor returned HTTP %d", resp.StatusCode)
	}

	// Verify a sid cookie actually landed in the jar — otherwise the
	// next call will follow the same login-redirect path the original
	// Bearer request did. This catches the "scopes don't include web"
	// case before we waste a round-trip.
	if !jarHasSession(jar, base) {
		applog.Dump([]string{"frontdoor", "no-session"}, "html", fdBody)
		return nil, fmt.Errorf("frontdoor didn't return a session cookie — the connected app's OAuth scopes likely don't include 'web'. sf org login web typically grants it; api-only or jwt-only apps won't")
	}

	// Step 2: follow the frontdoor redirect chain ourselves so we can
	// see every hop (frontdoor often issues 2-3 redirects under
	// Lightning, each setting another cookie). Use the jar throughout.
	// Allow auto-follow this time.
	httpc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	}

	// Step 2: GET the classic export URL with the cookie jar. NO
	// Authorization header — sending both confuses SF.
	exportURL := base + retURL
	greq, err := http.NewRequest("GET", exportURL, nil)
	if err != nil {
		return nil, err
	}
	greq.Header.Set("User-Agent", "sf-deck/0.1")
	greq.Header.Set("Accept",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,application/vnd.ms-excel,*/*")

	expStart := time.Now()
	defer func() {
		fireOnCall(c.alias, []string{"GET", "/" + reportID + "?export=1&xf=xlsx"}, nil, time.Since(expStart))
	}()
	gresp, err := httpc.Do(greq)
	if err != nil {
		applog.Error("export.get", map[string]any{"err": err.Error()})
		return nil, fmt.Errorf("export GET: %w", err)
	}
	defer gresp.Body.Close()

	body, err := readBodyLimited(gresp.Body)
	if err != nil {
		return nil, err
	}
	ct := gresp.Header.Get("Content-Type")
	finalURL := gresp.Request.URL.String()
	applog.Info("export.response", map[string]any{
		"status":       gresp.StatusCode,
		"content_type": ct,
		"final_url":    finalURL,
		"bytes":        len(body),
	})
	if gresp.StatusCode >= 400 {
		dump := applog.Dump([]string{"export", "http-error"}, "bin", body)
		return nil, fmt.Errorf("export HTTP %d (Content-Type %s, dump: %s)", gresp.StatusCode, ct, dump)
	}
	// Sanity check the body — xlsx is a zip ("PK"); HTML means we
	// somehow ended up at a login page anyway.
	if len(body) < 2 || body[0] != 'P' || body[1] != 'K' {
		ext := "bin"
		if strings.Contains(ct, "html") {
			ext = "html"
		}
		dump := applog.Dump([]string{"export", "non-xlsx"}, ext, body)
		// Identity-verification challenge: SF flags this device as new
		// for browser-style access (cookie session) and refuses to serve
		// the export until the user clears the challenge in a browser.
		// Bearer-token REST works fine; only the cookie-session hop is
		// gated. Surface a clear, actionable message rather than the
		// raw byte count.
		if isVerificationChallenge(body, finalURL) {
			return nil, fmt.Errorf("salesforce is challenging this session for identity verification. "+
				"Open the org in a browser once (`sf org open -o <alias>`), complete the verification prompt, "+
				"then retry the export. This only affects the cookie-session export fallback; "+
				"bearer-token REST is unaffected. (dump: %s)", dump)
		}
		if len(body) > 0 && body[0] == '<' {
			return nil, fmt.Errorf("export returned a login/error page (Content-Type %s, final URL %s, dump saved to %s)", ct, finalURL, dump)
		}
		return nil, fmt.Errorf("export returned %d bytes (Content-Type %s), not xlsx (dump: %s)", len(body), ct, dump)
	}
	return body, nil
}

// isVerificationChallenge reports whether the response is SF's identity
// verification page (the "We need to verify your identity" challenge
// shown for unrecognized devices). The page is HTML that redirects to
// /_ui/identity/verification/policy/VerificationStartUi — both the body
// and the final URL carry that path.
func isVerificationChallenge(body []byte, finalURL string) bool {
	if strings.Contains(finalURL, "/_ui/identity/verification/") {
		return true
	}
	// Body may be the redirect-shim HTML rather than the final page.
	return strings.Contains(string(body), "VerificationStartUi") ||
		strings.Contains(string(body), "/_ui/identity/verification/")
}

// jarHasSession reports whether the cookie jar contains an sid cookie
// for the instance host. The SF cookie domain is the instance hostname,
// not the org's My Domain — http.cookiejar returns cookies the next
// request to that URL would carry, which is exactly what we want.
func jarHasSession(jar *cookiejar.Jar, base string) bool {
	u, err := url.Parse(base)
	if err != nil {
		return false
	}
	for _, ck := range jar.Cookies(u) {
		if ck.Name == "sid" && ck.Value != "" {
			return true
		}
	}
	return false
}
