package sf

// frontdoor.jsp session bridge.

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
	switch resp.StatusCode {
	case http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusOK:
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

func isVerificationChallenge(body []byte, finalURL string) bool {
	if strings.Contains(finalURL, "/_ui/identity/verification/") {
		return true
	}
	return strings.Contains(string(body), "VerificationStartUi") ||
		strings.Contains(string(body), "/_ui/identity/verification/")
}

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
