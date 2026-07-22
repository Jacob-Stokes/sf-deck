package sf

// auth.go — orchestration helpers for `sf` auth lifecycle commands.
// We don't reimplement OAuth; we shell out to the canonical sf CLI
// and surface the result.
//
// LoginWeb is intentionally omitted from this file because it's an
// interactive command (opens a browser, blocks until the user
// completes the flow). The UI layer drives that via tea.Exec rather
// than runSF, which captures stdout — for an interactive command we
// want the user's terminal to receive the sf CLI output directly.
//
// The non-interactive helpers (Logout, SetAlias, SetDefault) all
// shell out via runSF with the default 30s timeout.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// Logout runs `sf org logout --target-org <username> --no-prompt`.
// Removes the local auth file; the connected app remains valid in
// Salesforce until the user explicitly revokes it.
func Logout(usernameOrAlias string) error {
	if strings.TrimSpace(usernameOrAlias) == "" {
		return fmt.Errorf("logout: username required")
	}
	_, err := runSF("org", "logout", "--target-org", usernameOrAlias, "--no-prompt")
	return err
}

// SetAlias runs `sf alias set <newAlias>=<usernameOrCurrentAlias>`.
// `sf alias` uses NAME=VALUE syntax where VALUE is the username.
func SetAlias(usernameOrCurrentAlias, newAlias string) error {
	usernameOrCurrentAlias = strings.TrimSpace(usernameOrCurrentAlias)
	newAlias = strings.TrimSpace(newAlias)
	if usernameOrCurrentAlias == "" || newAlias == "" {
		return fmt.Errorf("set alias: both username and new alias required")
	}
	_, err := runSF("alias", "set", fmt.Sprintf("%s=%s", newAlias, usernameOrCurrentAlias))
	return err
}

// SetDefault runs `sf config set target-org=<…> --global`. Always
// writes the user-global config (~/.sfdx/sf-config.json) rather than
// the project-local one — sf-deck doesn't run inside an sfdx project
// directory by default, so the project-local form fails with
// InvalidProjectWorkspaceError. Global is the right scope anyway:
// the user is setting "which org sfdx targets by default", not a
// per-project override.
func SetDefault(aliasOrUsername string) error {
	aliasOrUsername = strings.TrimSpace(aliasOrUsername)
	if aliasOrUsername == "" {
		return fmt.Errorf("set default: target required")
	}
	_, err := runSF("config", "set", fmt.Sprintf("target-org=%s", aliasOrUsername), "--global")
	return err
}

// SetDefaultDevHub runs `sf config set target-dev-hub=<…> --global`.
// Same --global rationale as SetDefault: the project-local form
// fails outside an sfdx project, and "default DevHub for this user"
// is a global concept.
func SetDefaultDevHub(aliasOrUsername string) error {
	aliasOrUsername = strings.TrimSpace(aliasOrUsername)
	if aliasOrUsername == "" {
		return fmt.Errorf("set default dev hub: target required")
	}
	_, err := runSF("config", "set", fmt.Sprintf("target-dev-hub=%s", aliasOrUsername), "--global")
	return err
}

// UnsetAlias runs `sf alias unset <alias>`. Removes a single alias
// from sfdx's alias store; the underlying authed org is unchanged
// and continues to be reachable via its username.
//
// `sf alias unset` accepts the ALIAS, not the username — sfdx stores
// alias→username, so callers pass the human-readable name.
func UnsetAlias(alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return fmt.Errorf("unset alias: alias required")
	}
	_, err := runSF("alias", "unset", alias)
	return err
}

// LoginWebCommand returns the *exec.Cmd for `sf org login web` so
// the UI can hand it to bubbletea's tea.Exec, which suspends the
// alt-screen and lets the user interact with the spawned process
// (sf CLI prompts the user, opens a browser, waits for the OAuth
// callback). On return tea.Exec resumes the TUI.
//
// alias may be empty (sf will prompt for one) or pre-populated.
// instanceURL is the org's login endpoint:
//
//   - ""                                             → omit flag (sf defaults to login.salesforce.com)
//   - "https://login.salesforce.com"                 → production (same as default)
//   - "https://test.salesforce.com"                  → sandboxes
//   - "https://<MyDomain>.my.salesforce.com"         → production with My Domain
//   - "https://<MyDomain>--<Sandbox>.sandbox.my.salesforce.com" → sandbox with My Domain
//   - "https://prerellogin.pre.salesforce.com"       → pre-release orgs
//
// The caller is responsible for pre-validating the scheme (https://)
// — sf rejects malformed URLs with a parse error before opening the
// browser, but the message is generic. Callers can ValidateInstanceURL
// upstream to fail fast with a user-friendly error.
//
// We DON'T use runSF here because runSF captures stdout and applies
// timeouts — neither is appropriate for a user-driven OAuth flow.
func LoginWebCommand(alias, instanceURL string) *exec.Cmd {
	args := []string{"org", "login", "web"}
	if a := strings.TrimSpace(alias); a != "" {
		args = append(args, "--alias", a)
	}
	if u := strings.TrimSpace(instanceURL); u != "" {
		args = append(args, "--instance-url", u)
	}
	cmd := exec.Command("sf", args...)
	return cmd
}

// ValidateInstanceURL is the cheap pre-flight a caller runs before
// handing user input to LoginWebCommand. sf only accepts https://
// URLs and emits a confusing error mid-flow otherwise. Returns nil
// for the empty string (caller intentionally omitted the flag).
func ValidateInstanceURL(url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil
	}
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("instance URL must start with https:// (got %q)", url)
	}
	return nil
}

// LoginSfdxURLCommand returns the *exec.Cmd for `sf org login
// sfdx-url --sfdx-url-stdin` so the user can paste a sfdxAuthUrl
// (typically captured by `sf org display --verbose` on another
// machine, or pulled from a password manager) instead of running
// an interactive browser flow.
//
// `--sfdx-url-stdin` tells sf to read the URL from stdin — the
// terminal handles the typing while bubbletea is suspended via
// tea.Exec, so the user pastes once and presses Ctrl-D / Enter.
func LoginSfdxURLCommand(alias string) *exec.Cmd {
	args := []string{"org", "login", "sfdx-url", "--sfdx-url-stdin"}
	if a := strings.TrimSpace(alias); a != "" {
		args = append(args, "--alias", a)
	}
	cmd := exec.Command("sf", args...)
	return cmd
}

// AuthTimeout is the rough cap callers hand to runSFCtx for non-
// interactive auth helpers when they want to override the default.
// Currently unused by the helpers above (default 30s is plenty)
// but exported for parity with the surrounding package.
const AuthTimeout = 60 * time.Second

// _ keeps the context import live for future explicit-deadline
// auth flows without churning the import block.
var _ = context.Background

// SingleAccessURL exchanges the org's cached access token for a
// ONE-TIME frontdoor login URL via /services/oauth2/singleaccess —
// the same mechanism modern `sf org open` uses. The returned URL
// logs the browser in without a password prompt, carries a one-time
// code rather than the long-lived token (so browser history / the
// `open` process args never see the real session id), and lands on
// `path` via retURL.
func SingleAccessURL(target, path string) (string, error) {
	c, err := RESTClient(target)
	if err != nil {
		return "", err
	}
	body, err := c.post("/services/oauth2/singleaccess", nil)
	if err != nil {
		return "", fmt.Errorf("singleaccess: %w", err)
	}
	var parsed struct {
		FrontdoorURI string `json:"frontdoor_uri"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.FrontdoorURI == "" {
		return "", fmt.Errorf("singleaccess: unexpected response")
	}
	u := parsed.FrontdoorURI
	if path != "" {
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		u += sep + "retURL=" + url.QueryEscape(path)
	}
	return u, nil
}
