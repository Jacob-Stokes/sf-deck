package sf

// REST client.
//
// Stage 2 of the perf roadmap: skip the sf CLI's ~1s Node-startup cost
// for hot-path data calls. sf still owns auth (JWT refresh, OAuth web
// flow, token storage, keyring integration) — we borrow the org metadata
// from `sf org display` and, on newer CLIs that redact secrets from that
// command, fetch the access token through the explicit credential command
// `sf org auth show-access-token`.
//
// When a token expires, Salesforce returns 401 INVALID_SESSION_ID. The
// client catches this, discards the token, re-runs bootstrap (which
// triggers sf's refresh machinery), retries once. User never sees it.
//
// REST-direct calls are ~10× faster than shelling out to sf every time.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// maxResponseBytes caps how much of an org HTTP response body we
// read into memory. A trusted-org-turned-hostile or a MITM (were TLS
// ever bypassed) could otherwise return a multi-GB body and OOM the
// TUI via io.ReadAll. 512 MiB is far above any legitimate REST /
// SOAP / report-export payload (report exports are the largest and
// cap at 100k rows) yet bounds the worst case.
const maxResponseBytes = 512 << 20 // 512 MiB

// readBodyLimited reads an HTTP response body up to maxResponseBytes.
// Returns an error when the body exceeds the cap (rather than
// silently truncating, which would hand a half-parsed body to JSON /
// XML decoders). Callers pass resp.Body directly.
func readBodyLimited(body io.Reader) ([]byte, error) {
	// +1 so a body exactly at the cap reads fully and anything larger
	// trips the length check below.
	data, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxResponseBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes cap", maxResponseBytes)
	}
	return data, nil
}

// Client is a cached REST handle for one org.
type Client struct {
	alias       string
	accessToken string
	instanceURL string
	apiVersion  string
	http        *http.Client
	mu          sync.Mutex
}

// clients is a per-alias singleton registry. First call to RESTClient
// for a given alias bootstraps via sf; subsequent calls reuse.
//
// Each alias has its own bootstrap latch (sync.Once-style) so two
// concurrent RESTClient(alias) callers serialise on the bootstrap
// instead of both shelling out to `sf org display`. Holding the
// global clientsMu through bootstrap would serialise across aliases,
// hurting multi-org parallelism on cold launch; per-alias latches
// only serialise within a single alias.
var (
	clientsMu sync.Mutex
	clients   = map[string]*clientEntry{}
)

type clientEntry struct {
	once   sync.Once
	client *Client
	err    error
}

// RESTClient returns a REST client for the given org alias, bootstrapping
// auth via sf on first use. Concurrent calls for the same alias share a
// single bootstrap (no duplicate `sf org display` calls).
// DemoMode is the live-Salesforce kill switch for `sf-deck --demo`.
// The resource layer's demo freeze means these paths should never be
// reached; this is the backstop that turns a missed fixture into a
// fast, honest error instead of a hung subprocess or network call.
var DemoMode bool

// rateLimitBackoff is how long to wait before retrying a 429. Salesforce
// short-term throttles clear quickly; a single bounded wait is enough
// without parsing Retry-After (which sfHTTPError doesn't carry).
const rateLimitBackoff = 2 * time.Second

func RESTClient(alias string) (*Client, error) {
	if DemoMode {
		return nil, errors.New("demo mode: live Salesforce calls are disabled")
	}
	// Per-org demo: this alias/username is an injected demo org with no
	// live backend. Short-circuit before touching `sf` so its surfaces
	// serve seeded cache instead of failing on a missing token.
	if isDemoTarget(alias) {
		return nil, ErrDemoTarget
	}
	clientsMu.Lock()
	entry, ok := clients[alias]
	if !ok {
		entry = &clientEntry{}
		clients[alias] = entry
	}
	clientsMu.Unlock()

	entry.once.Do(func() {
		c := &Client{
			alias: alias,
			http: &http.Client{
				Timeout: cfgHTTPTimeout(),
			},
		}
		if err := c.bootstrap(); err != nil {
			entry.err = err
			return
		}
		entry.client = c
	})
	if entry.err != nil {
		// Reset the entry so a future call can retry — sync.Once
		// would otherwise pin the error forever.
		clientsMu.Lock()
		delete(clients, alias)
		clientsMu.Unlock()
		return nil, entry.err
	}
	return entry.client, nil
}

// InvalidateRESTClients drops every cached client. Called at startup
// so session-to-session token staleness doesn't surprise us, and so
// tests can reset cleanly. Also clears the CustomObject.Id cache
// (process-wide; lives alongside the REST clients) so aliases that
// got reauthed don't accidentally see Ids from a prior session.
func InvalidateRESTClients() {
	clientsMu.Lock()
	clients = map[string]*clientEntry{}
	clientsMu.Unlock()
	invalidateCustomObjectIDCache()
}

// ReconcileRESTClients drops only the cached clients that no longer
// match the current alias→org mapping, keeping every still-valid token
// alive. wantInstanceURL maps each currently-known org alias to its
// instanceURL. A cached client is dropped when its alias has vanished
// from the map (org logged out) or its instanceURL differs from what
// the map now reports (alias repointed to a different org via
// `sf alias set` in another terminal) — the two cases the blanket
// InvalidateRESTClients was guarding against.
//
// This exists because a routine live orgs-list refetch used to call
// InvalidateRESTClients unconditionally, throwing away every good token
// and forcing a full `sf` re-bootstrap (two subprocess spawns on
// redacting CLIs) on the next data call — the source of "refresh got
// slow." Reconciling instead means a refetch that changed nothing costs
// nothing: valid tokens survive, only genuinely-stale aliases re-bootstrap.
func ReconcileRESTClients(wantInstanceURL map[string]string) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	for alias, entry := range clients {
		want, known := wantInstanceURL[alias]
		if !known {
			// Alias no longer in the org list — drop it.
			delete(clients, alias)
			continue
		}
		if entry.client == nil {
			// A bootstrap that errored (client nil) — let it retry.
			delete(clients, alias)
			continue
		}
		entry.client.mu.Lock()
		have := entry.client.instanceURL
		entry.client.mu.Unlock()
		// Only compare when the org list actually carries an
		// instanceURL for this alias; an empty want means "unknown,
		// don't second-guess a working client."
		if want != "" && have != "" && !sameInstanceHost(have, want) {
			delete(clients, alias)
		}
	}
}

// sameInstanceHost reports whether two instance URLs point at the same
// Salesforce host, tolerating trailing slashes and scheme differences.
// Used by ReconcileRESTClients to detect an alias repoint without being
// fooled by cosmetic URL variance.
func sameInstanceHost(a, b string) bool {
	norm := func(s string) string {
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(s, "https://")
		s = strings.TrimPrefix(s, "http://")
		return strings.TrimRight(s, "/")
	}
	return norm(a) == norm(b)
}

// tokenFetchInFlight counts how many bootstrap() calls are currently
// shelling out to `sf` for a token. It's a process-wide gauge the UI
// reads (TokenFetchInFlight) so the syncing indicator can tell the user
// a refresh is slow *because* it's minting a fresh token — the ~2.5s
// two-subprocess bootstrap on redacting CLIs — rather than leaving them
// guessing why this refresh took longer than the last.
var tokenFetchInFlight atomic.Int32

// TokenFetchInFlight reports whether any org is mid token-bootstrap
// (the slow `sf` round-trip). The UI surfaces this in the syncing label.
func TokenFetchInFlight() bool { return tokenFetchInFlight.Load() > 0 }

// bootstrap shells out to sf once or twice per org per process: display
// supplies non-secret org metadata, and access-token supplies the secret
// on newer CLIs where display redacts it. Older CLIs still work because
// display may return a usable accessToken and avoid the second command.
func (c *Client) bootstrap() error {
	tokenFetchInFlight.Add(1)
	defer tokenFetchInFlight.Add(-1)
	out, err := runSF("org", "display", "--verbose", "-o", c.alias, "--json")
	if err != nil {
		return fmt.Errorf("sf org display (REST bootstrap): %w", err)
	}
	var parsed struct {
		Result struct {
			AccessToken string `json:"accessToken"`
			InstanceURL string `json:"instanceUrl"`
			APIVersion  string `json:"apiVersion"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return fmt.Errorf("decode org display: %w", err)
	}
	accessToken := parsed.Result.AccessToken
	if tokenIsRedacted(accessToken) {
		token, err := explicitAccessToken(c.alias)
		if err != nil {
			return fmt.Errorf("sf org auth show-access-token (REST bootstrap): %w", err)
		}
		accessToken = token
	}
	c.mu.Lock()
	c.accessToken = accessToken
	c.instanceURL = parsed.Result.InstanceURL
	// A user-forced API version (settings [ui.api] api_version) wins;
	// otherwise use what the org reported; otherwise the package default.
	if forced := cfgAPIVersion(); forced != "" {
		c.apiVersion = forced
	} else {
		c.apiVersion = parsed.Result.APIVersion
	}
	if c.apiVersion == "" {
		c.apiVersion = defaultAPIVersion
	}
	c.mu.Unlock()
	if c.accessToken == "" {
		return fmt.Errorf("sf returned no access token for %s", c.alias)
	}
	return nil
}

func explicitAccessToken(alias string) (string, error) {
	out, err := runSF("org", "auth", "show-access-token", "--target-org", alias, "--no-prompt", "--json")
	if err != nil {
		return "", err
	}
	token, err := parseAccessToken(out)
	if err != nil {
		return "", err
	}
	if tokenIsRedacted(token) {
		return "", fmt.Errorf("access token output was redacted")
	}
	return token, nil
}

func parseAccessToken(out []byte) (string, error) {
	var raw struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return "", fmt.Errorf("decode access token response: %w", err)
	}
	var result struct {
		AccessToken string `json:"accessToken"`
		Token       string `json:"token"`
	}
	if len(raw.Result) > 0 {
		if err := json.Unmarshal(raw.Result, &result); err == nil {
			if result.AccessToken != "" {
				return result.AccessToken, nil
			}
			if result.Token != "" {
				return result.Token, nil
			}
		}
		var s string
		if err := json.Unmarshal(raw.Result, &s); err == nil {
			return s, nil
		}
	}
	return "", fmt.Errorf("access token response did not include result.accessToken")
}

func tokenIsRedacted(token string) bool {
	t := strings.TrimSpace(strings.ToLower(token))
	if t == "" {
		return true
	}
	if strings.Contains(t, "redact") || strings.Contains(t, "<hidden>") {
		return true
	}
	trimmed := strings.Trim(t, "*")
	return trimmed == ""
}

// get performs a GET against a path relative to the instance URL,
// returning the raw body. Handles auto-re-bootstrap on 401 once.
func (c *Client) get(path string, query url.Values) ([]byte, error) {
	return c.doWithRetry("GET", path, query, nil)
}

// getCtx is the cancellable twin of get.  The given context is
// passed to http.NewRequestWithContext so its Done channel aborts
// the in-flight request — the user's ctrl+c on a long-running SOQL
// returns the modal to idle without waiting for the server.
//
// Added as a parallel path rather than retrofitting every existing
// REST callsite because cancellation is only meaningful on a couple
// of user-driven hot paths (SOQL execute today, plus future SOSL
// global search).  Everything else uses the simpler get(...).
func (c *Client) getCtx(ctx context.Context, path string, query url.Values) ([]byte, error) {
	return c.doWithRetryCtx(ctx, "GET", path, query, nil)
}

// getWithAcceptTimeout is getWithAccept with a per-call timeout override.
// Pass 0 to use the client's default. Used by the report-export path
// where the analytics endpoint regularly takes 60-120s to serialize a
// large workbook server-side and the default 30s aborts mid-flight.
func (c *Client) getWithAcceptTimeout(path string, query url.Values, accept string, timeout time.Duration) ([]byte, error) {
	resp, err := c.doOnceWithAccept(path, query, accept, timeout)
	if err == nil {
		return resp, nil
	}
	if !isSessionExpired(err) {
		return nil, err
	}
	if berr := c.bootstrap(); berr != nil {
		return nil, fmt.Errorf("re-auth failed: %w (original: %v)", berr, err)
	}
	return c.doOnceWithAccept(path, query, accept, timeout)
}

func (c *Client) doOnceWithAccept(path string, query url.Values, accept string, timeout time.Duration) (out []byte, err error) {
	c.mu.Lock()
	token := c.accessToken
	base := c.instanceURL
	c.mu.Unlock()

	u := strings.TrimRight(base, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "sf-deck/0.1")
	// xlsx exports go through the same call-tracking the rest of the
	// REST path uses — these count against the daily API limit.
	logPath := path
	if len(query) > 0 {
		logPath += "?" + query.Encode()
	}
	startedAt := time.Now()
	defer func() { fireOnCall(c.alias, []string{"GET", logPath}, err, time.Since(startedAt)) }()

	// When the caller passed a per-call timeout, build a one-shot client
	// that won't enforce the shared client's shorter default. The shared
	// http.Client.Timeout is a hard ceiling that overrides the request
	// context, so we need a fresh client to honour the longer deadline.
	httpc := c.http
	if timeout > 0 && (c.http.Timeout == 0 || timeout > c.http.Timeout) {
		httpc = &http.Client{Timeout: timeout, Transport: c.http.Transport}
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := readBodyLimited(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, &sfHTTPError{Status: resp.StatusCode, Body: respBody}
	}
	return respBody, nil
}

// patch performs a PATCH against a path relative to the instance URL
// with a JSON body. Used for write-path calls — primarily Tooling API
// updates to CustomField / CustomObject / ValidationRule. Handles the
// same auto-re-bootstrap on 401 that GETs do.
func (c *Client) patch(path string, body []byte) ([]byte, error) {
	return c.doWithRetry("PATCH", path, nil, body)
}

// post creates a Tooling sobject row. Same auth + retry path.
func (c *Client) post(path string, body []byte) ([]byte, error) {
	return c.doWithRetry("POST", path, nil, body)
}

// delete destroys a Tooling sobject row by path. Same auth + retry path.
func (c *Client) delete(path string) ([]byte, error) {
	return c.doWithRetry("DELETE", path, nil, nil)
}

// postMultipart sends a multipart/form-data POST. Used by the
// Metadata REST API which accepts deploy payloads as a two-part
// form: "entity_content" (JSON with deploy options) and "file"
// (ZIP bytes). Bypasses doWithRetry's JSON-body path since we need
// a custom Content-Type + prebuilt body.
func (c *Client) postMultipart(path string, contentType string, body []byte) ([]byte, error) {
	resp, err := c.doOnceMultipart(path, contentType, body)
	if err == nil {
		return resp, nil
	}
	if !isSessionExpired(err) {
		return nil, err
	}
	if berr := c.bootstrap(); berr != nil {
		return nil, fmt.Errorf("re-auth failed: %w (original: %v)", berr, err)
	}
	return c.doOnceMultipart(path, contentType, body)
}

func (c *Client) doOnceMultipart(path, contentType string, body []byte) (out []byte, err error) {
	c.mu.Lock()
	token := c.accessToken
	base := c.instanceURL
	c.mu.Unlock()

	u := strings.TrimRight(base, "/") + path
	req, err := http.NewRequest("POST", u, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "sf-deck/0.1")
	req.Header.Set("Content-Type", contentType)

	mpStart := time.Now()
	defer func() { fireOnCall(c.alias, []string{"POST", path}, err, time.Since(mpStart)) }()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := readBodyLimited(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, &sfHTTPError{Status: resp.StatusCode, Body: respBody}
	}
	return respBody, nil
}

func (c *Client) doWithRetry(method, path string, query url.Values, body []byte) ([]byte, error) {
	resp, err := c.doOnce(method, path, query, body)
	if err == nil {
		return resp, nil
	}
	// Short-term rate limit (429): wait briefly and retry once. Routine
	// on a busy org; the token is fine so no re-bootstrap.
	if isRateLimited(err) {
		time.Sleep(rateLimitBackoff)
		return c.doOnce(method, path, query, body)
	}
	// On auth failure, re-bootstrap once and retry. Anything else
	// bubbles up.
	if !isSessionExpired(err) {
		return nil, err
	}
	if berr := c.bootstrap(); berr != nil {
		return nil, fmt.Errorf("re-auth failed: %w (original: %v)", berr, err)
	}
	return c.doOnce(method, path, query, body)
}

// doWithRetryCtx is the cancellable twin of doWithRetry.  ctx is
// threaded into the underlying http.Request so its cancellation
// aborts an in-flight call.
func (c *Client) doWithRetryCtx(ctx context.Context, method, path string, query url.Values, body []byte) ([]byte, error) {
	resp, err := c.doOnceCtx(ctx, method, path, query, body)
	if err == nil {
		return resp, nil
	}
	if isRateLimited(err) {
		select {
		case <-time.After(rateLimitBackoff):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return c.doOnceCtx(ctx, method, path, query, body)
	}
	if !isSessionExpired(err) {
		return nil, err
	}
	if berr := c.bootstrap(); berr != nil {
		return nil, fmt.Errorf("re-auth failed: %w (original: %v)", berr, err)
	}
	return c.doOnceCtx(ctx, method, path, query, body)
}

func (c *Client) doOnce(method, path string, query url.Values, body []byte) (out []byte, err error) {
	return c.doOnceCtx(context.Background(), method, path, query, body)
}

// doOnceCtx is the context-aware HTTP path.  doOnce delegates here
// with context.Background so the simple synchronous callers don't
// have to thread context — only the cancellable hot paths (SOQL,
// SOSL) build a real cancellable context and pass it through.
func (c *Client) doOnceCtx(ctx context.Context, method, path string, query url.Values, body []byte) (out []byte, err error) {
	c.mu.Lock()
	token := c.accessToken
	base := c.instanceURL
	c.mu.Unlock()

	u := strings.TrimRight(base, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var bodyReader io.Reader
	if body != nil {
		bodyReader = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "sf-deck/0.1")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Every REST call still ticks the usage tracker — Salesforce counts
	// these exactly the same as sf-CLI-mediated calls. Include the
	// query string so the API log can show what SOQL / which describe
	// was actually fetched, not just the bare endpoint.
	logPath := path
	if len(query) > 0 {
		logPath += "?" + query.Encode()
	}
	doStart := time.Now()
	defer func() { fireOnCall(c.alias, []string{method, logPath}, err, time.Since(doStart)) }()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := readBodyLimited(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, &sfHTTPError{
			Status: resp.StatusCode,
			Body:   respBody,
		}
	}
	return respBody, nil
}

// APIPath prepends the standard /services/data/v<N>/ prefix and
// returns an absolute-to-instance path for callers that want to build
// a URL themselves.
func (c *Client) APIPath(suffix string) string {
	return "/services/data/v" + c.apiVersion + "/" + strings.TrimLeft(suffix, "/")
}

// ToolingPath is the tooling-API equivalent of APIPath.
func (c *Client) ToolingPath(suffix string) string {
	return "/services/data/v" + c.apiVersion + "/tooling/" + strings.TrimLeft(suffix, "/")
}

// defaultAPIVersion is the fallback API version when neither the
// REST client nor `sf org display` can supply one. Kept on the high
// end so a brand-new SF release doesn't immediately reject calls
// that fall through to this default. Bump alongside any release
// where the codebase starts relying on newer endpoints.
const defaultAPIVersion = "62.0"

// APIVersionForAlias resolves the org's API version without
// requiring a fully-bootstrapped REST Client. Falls back to the
// package default when neither the cached client nor a fresh
// `sf org display` shell-out can supply one.
//
// Used by CLI-fallback paths (e.g. RunListView's `sf api request
// rest` branch) that build /services/data/v<N>/ paths by hand —
// previously these hard-coded v62.0 and would drift silently when
// the REST client was bumped. Centralising the lookup means version
// updates happen in one place.
func APIVersionForAlias(alias string) string {
	if alias == "" {
		return defaultAPIVersion
	}
	if c, ok := lookupClient(alias); ok && c.apiVersion != "" {
		return c.apiVersion
	}
	// Fresh shell-out as a last resort. Result isn't cached here
	// because callers on this path are already in the CLI-fallback
	// branch where one more shell-out is cheap relative to whatever
	// else they're doing.
	if d, err := DisplayOrg(alias); err == nil && d.APIVersion != "" {
		return d.APIVersion
	}
	return defaultAPIVersion
}

// lookupClient returns the cached client for alias if one exists,
// without bootstrapping. Used by APIVersionForAlias to avoid a full
// RESTClient bootstrap on the CLI-fallback path.
func lookupClient(alias string) (*Client, bool) {
	clientsMu.Lock()
	entry, ok := clients[alias]
	clientsMu.Unlock()
	if !ok || entry == nil || entry.client == nil {
		return nil, false
	}
	return entry.client, true
}

// sfHTTPError carries a non-2xx response so callers (and retry logic)
// can introspect.
type sfHTTPError struct {
	Status int
	Body   []byte
}

func (e *sfHTTPError) Error() string {
	var arr []struct {
		Message   string `json:"message"`
		ErrorCode string `json:"errorCode"`
	}
	if err := json.Unmarshal(e.Body, &arr); err == nil && len(arr) > 0 {
		if arr[0].ErrorCode != "" {
			return fmt.Sprintf("%s: %s", arr[0].ErrorCode, arr[0].Message)
		}
		return arr[0].Message
	}
	return fmt.Sprintf("HTTP %d: %s", e.Status, string(e.Body))
}

func isSessionExpired(err error) bool {
	he, ok := err.(*sfHTTPError)
	if !ok {
		return false
	}
	body := string(he.Body)
	expired := strings.Contains(body, "INVALID_SESSION_ID") ||
		strings.Contains(body, "Session expired")
	if !expired {
		return false
	}
	return he.Status == 401 || he.Status == 403 || he.Status == 500
}

// isRateLimited reports whether err is a short-term rate-limit response
// worth retrying after a brief wait: HTTP 429 (concurrent-request /
// short-term throttle). This is routine on a busy production org. The
// 24h daily limit (REQUEST_LIMIT_EXCEEDED) is NOT included here — that
// isn't fixed by waiting a few seconds; isDailyLimitExceeded surfaces it
// with a clear message instead.
func isRateLimited(err error) bool {
	he, ok := err.(*sfHTTPError)
	return ok && he.Status == 429
}

// isDailyLimitExceeded reports whether err is the 24h API-limit error,
// which retrying won't help — callers surface it with a clear message
// rather than the raw HTTP error.
func isDailyLimitExceeded(err error) bool {
	he, ok := err.(*sfHTTPError)
	return ok && strings.Contains(string(he.Body), "REQUEST_LIMIT_EXCEEDED")
}

// classifyQueryErr wraps a SOQL/pagination error with a clearer message
// for the two limit cases, leaving everything else untouched.
func classifyQueryErr(err error) error {
	if isDailyLimitExceeded(err) {
		return fmt.Errorf("daily API request limit exhausted for this org (REQUEST_LIMIT_EXCEEDED): %w", err)
	}
	return err
}

// --- high-level convenience methods --------------------------------------

// QueryREST runs a SOQL via the REST API (or Tooling API when tooling
// is true). Same shape as the CLI-based Query() so call-sites can be
// swapped out with no downstream changes.
//
// Follows nextRecordsUrl automatically when the response is chunked
// (Tooling caps responses at 500 rows and standard REST at 2000
// regardless of LIMIT — queries beyond that cap come back with
// done=false + a cursor URL). Every page's records get appended and
// the final QueryResult always has Done=true + TotalSize from the
// first response.
func (c *Client) QueryREST(soql string, tooling bool) (QueryResult, error) {
	return c.QueryRESTCapped(soql, tooling, 0)
}

// queryRows is the shared body behind the ~45 simple list functions that
// all do: bootstrap a REST client, run one SOQL query, and map each
// result record to a typed row. It owns the invariant scaffolding
// (client fetch + error guard, query + error guard, slice pre-alloc,
// range loop, return); callers supply the SOQL and a per-row mapper.
//
// It matches the DIRECT RESTClient→QueryREST path — NOT the package-level
// Query(), which adds a CLI fallback on bootstrap failure. The 45 callers
// all used the direct path (returning the bootstrap error), so this
// preserves their exact behavior. Functions that paginate, merge multiple
// queries, dedup, sort, or post-process stay bespoke.
func queryRows[T any](target, soql string, tooling bool, mapRow func(map[string]any) T) ([]T, error) {
	c, err := RESTClient(target)
	if err != nil {
		return nil, err
	}
	q, err := c.QueryREST(soql, tooling)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(q.Records))
	for _, r := range q.Records {
		out = append(out, mapRow(r))
	}
	return out, nil
}

// QueryRESTCapped is QueryREST with a client-side row cap. Pulls
// pages until either the cursor signals done or len(Records) >= cap.
// cap <= 0 disables the cap (full QueryREST semantics — pull
// everything until done).
//
// TotalSize is the row count Salesforce reports for the WHERE clause
// in the FIRST page — it's the true unbounded total, regardless of
// whether we cap. Renderers compare TotalSize to len(Records) to
// surface a "showing X of Y · capped" hint when the cap kicked in.
//
// When the cap stops the cursor follow early, Done is set to false
// (mirroring SF's "more pages exist" signal) so callers can detect
// truncation either via Done or via TotalSize > len(Records).
func (c *Client) QueryRESTCapped(soql string, tooling bool, cap int) (QueryResult, error) {
	var path string
	if tooling {
		path = c.ToolingPath("query")
	} else {
		path = c.APIPath("query")
	}
	q := url.Values{}
	q.Set("q", soql)

	body, err := c.get(path, q)
	if err != nil {
		return QueryResult{}, err
	}
	var page queryPage
	if err := json.Unmarshal(body, &page); err != nil {
		return QueryResult{}, err
	}
	out := QueryResult{
		Records:   page.Records,
		TotalSize: page.TotalSize,
		Done:      page.Done,
	}

	// Truncate the FIRST page if the cap is already smaller than what
	// SF returned. Salesforce hands back up to 2000 rows per page; if
	// the chip's cap is e.g. 5, we'd otherwise return all 700+ rows
	// SF squeezed into page 1 because page.Done is already true. The
	// page.TotalSize stays as SF reported it (the unbounded match
	// count), so the "X of Y · capped" hint still works.
	if cap > 0 && len(out.Records) > cap {
		out.Records = out.Records[:cap]
		out.Done = false
		return out, nil
	}

	// Follow the cursor until done OR we hit the row cap.
	// nextRecordsUrl is an absolute path like
	// "/services/data/vNN/query/01g…-2000" — passed through the get
	// helper as-is, no params.
	for !page.Done && page.NextRecordsURL != "" {
		if cap > 0 && len(out.Records) >= cap {
			// Truncate — leave Done=false so callers see the cursor
			// stopped early.
			out.Done = false
			if len(out.Records) > cap {
				out.Records = out.Records[:cap]
			}
			return out, nil
		}
		body, err := c.get(page.NextRecordsURL, nil)
		if err != nil {
			// A follow-on page failed (rate limit, daily limit, network).
			// Don't throw away the thousands of rows already accumulated —
			// return them with Done=false so the caller can show a partial
			// result + a clear error instead of nothing.
			out.Done = false
			return out, classifyQueryErr(err)
		}
		page = queryPage{}
		if err := json.Unmarshal(body, &page); err != nil {
			out.Done = false
			return out, err
		}
		out.Records = append(out.Records, page.Records...)
		out.Done = page.Done
	}
	return out, nil
}

// QueryRESTCtx is the cancellable twin of QueryREST.  Same paging
// semantics (follow nextRecordsUrl to completion) but the ctx is
// threaded through every HTTP call, so cancelling it aborts both
// the in-flight request AND the next paging follow-on.  Used by the
// /soql modal's ctrl+c cancel.
func (c *Client) QueryRESTCtx(ctx context.Context, soql string, tooling bool) (QueryResult, error) {
	var path string
	if tooling {
		path = c.ToolingPath("query")
	} else {
		path = c.APIPath("query")
	}
	q := url.Values{}
	q.Set("q", soql)

	body, err := c.getCtx(ctx, path, q)
	if err != nil {
		return QueryResult{}, err
	}
	var page queryPage
	if err := json.Unmarshal(body, &page); err != nil {
		return QueryResult{}, err
	}
	out := QueryResult{
		Records:   page.Records,
		TotalSize: page.TotalSize,
		Done:      page.Done,
	}
	for !page.Done && page.NextRecordsURL != "" {
		body, err := c.getCtx(ctx, page.NextRecordsURL, nil)
		if err != nil {
			// Keep the rows already fetched (see QueryRESTCapped).
			out.Done = false
			return out, classifyQueryErr(err)
		}
		page = queryPage{}
		if err := json.Unmarshal(body, &page); err != nil {
			out.Done = false
			return out, err
		}
		out.Records = append(out.Records, page.Records...)
		out.Done = page.Done
	}
	return out, nil
}

// queryPage is the wire shape of one SOQL response. Separate from
// QueryResult so we can keep the public type clean while still
// unmarshaling nextRecordsUrl for cursor-following.
type queryPage struct {
	Records        []map[string]any `json:"records"`
	TotalSize      int              `json:"totalSize"`
	Done           bool             `json:"done"`
	NextRecordsURL string           `json:"nextRecordsUrl"`
}

// DescribeREST is the REST-direct equivalent of the Describe() shell-out.
func (c *Client) DescribeREST(sobjectName string) (SObjectDescribe, error) {
	path := c.APIPath("sobjects/" + sobjectName + "/describe")
	body, err := c.get(path, nil)
	if err != nil {
		return SObjectDescribe{}, err
	}
	var out SObjectDescribe
	if err := json.Unmarshal(body, &out); err != nil {
		return SObjectDescribe{}, err
	}
	return out, nil
}

// LimitsREST mirrors Limits() but goes direct.
func (c *Client) LimitsREST() ([]Limit, error) {
	path := c.APIPath("limits")
	body, err := c.get(path, nil)
	if err != nil {
		return nil, err
	}
	// Limits endpoint returns { "LimitName": { "Max": 100, "Remaining": 99 } }.
	var raw map[string]struct {
		Max       int `json:"Max"`
		Remaining int `json:"Remaining"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := make([]Limit, 0, len(raw))
	for name, v := range raw {
		out = append(out, Limit{Name: name, Max: v.Max, Remaining: v.Remaining})
	}
	return out, nil
}

// ListViewDescribe is the describe-payload of a Salesforce list view —
// columns + the generated SOQL the server would run for it. Used by
// the import-as-lens flow to extract a WHERE clause we can save.
type ListViewDescribe struct {
	ID      string            // ListView Id (15 or 18 char)
	Query   string            // server-generated SOQL (full SELECT … FROM … WHERE …)
	OrderBy []ListViewOrderBy // declared ordering
	Columns []ListViewColumn  // user-picked columns
}

// ListViewOrderBy is one ordering entry in a list view's describe.
type ListViewOrderBy struct {
	FieldName string `json:"fieldNameOrPath"`
	Nulls     string `json:"nullsPosition"` // "first" / "last" / ""
	SortDir   string `json:"sortDirection"` // "ascending" / "descending"
}

// DescribeListView returns the full describe payload for a list view
// — including the underlying SOQL Salesforce generates from its
// structured filter rows. Read-only.
func (c *Client) DescribeListView(sobjectName, listViewID string) (ListViewDescribe, error) {
	path := c.APIPath("sobjects/" + url.PathEscape(sobjectName) +
		"/listviews/" + url.PathEscape(listViewID) + "/describe")
	body, err := c.get(path, nil)
	if err != nil {
		return ListViewDescribe{}, err
	}
	var parsed struct {
		ID      string            `json:"id"`
		Query   string            `json:"query"`
		OrderBy []ListViewOrderBy `json:"orderBy"`
		Columns []ListViewColumn  `json:"columns"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ListViewDescribe{}, err
	}
	return ListViewDescribe{
		ID: parsed.ID, Query: parsed.Query,
		OrderBy: parsed.OrderBy, Columns: parsed.Columns,
	}, nil
}

// DefaultListViewPreviewLimit is the row cap fallback for callers
// that don't pass a setting-driven limit. Kept for callers that
// don't have a settings handle (rare); the UI should pass
// settings.ListViewPreviewLimit() at call time.
const DefaultListViewPreviewLimit = 50

// ListViewResultsREST runs a list view via the REST API. Same shape as
// the CLI-based RunListView for drop-in replacement.
//
// limit is the row cap; <=0 falls back to DefaultListViewPreviewLimit.
// The renderer surfaces a "capped — import to see all" hint when
// len(records) hits the cap. Salesforce's `size` field reports
// rows-in-response (not unbounded total), so the UI uses the
// heuristic "len == cap → likely truncated" instead.
func (c *Client) ListViewResultsREST(sobjectName, listViewID string, limit int) (ListViewResult, error) {
	if limit <= 0 {
		limit = DefaultListViewPreviewLimit
	}
	path := c.APIPath("sobjects/" + sobjectName + "/listviews/" + listViewID + "/results")
	q := url.Values{}
	q.Set("limit", fmt.Sprintf("%d", limit))
	body, err := c.get(path, q)
	if err != nil {
		return ListViewResult{}, err
	}
	var parsed struct {
		Columns []ListViewColumn `json:"columns"`
		Records []struct {
			Columns []struct {
				FieldNameOrPath string `json:"fieldNameOrPath"`
				Value           any    `json:"value"`
				Label           string `json:"label"`
			} `json:"columns"`
			ID string `json:"Id"`
		} `json:"records"`
		Size int  `json:"size"`
		Done bool `json:"done"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ListViewResult{}, err
	}
	out := ListViewResult{
		Columns:   parsed.Columns,
		TotalSize: parsed.Size,
		Done:      parsed.Done,
	}
	for _, r := range parsed.Records {
		row := map[string]any{"Id": r.ID}
		for _, col := range r.Columns {
			if col.Label != "" {
				row[col.FieldNameOrPath] = col.Label
			} else {
				row[col.FieldNameOrPath] = col.Value
			}
		}
		out.Records = append(out.Records, row)
	}
	return out, nil
}
