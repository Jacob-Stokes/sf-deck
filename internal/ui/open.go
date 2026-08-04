package ui

import (
	"fmt"
	neturl "net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func safeToOpenTargetURL(raw string, allowBrowserExtension bool) error {
	if strings.HasPrefix(raw, "-") {
		return fmt.Errorf("refusing to open URL starting with '-': %q", raw)
	}
	u, err := neturl.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.Opaque != "" || u.User != nil {
		return fmt.Errorf("refusing to open malformed URL: %q", raw)
	}
	switch strings.ToLower(u.Scheme) {
	case "https", "http":
		return nil
	case "chrome-extension", "moz-extension":
		if allowBrowserExtension && u.Port() == "" && u.Path == "/inspect.html" {
			return nil
		}
	}
	return fmt.Errorf("refusing to open unsupported URL: %q", raw)
}

// sameHost reports whether two URLs share the same host (case-
// insensitive). Used to confirm an org-returned frontdoor URL points
// at the instance we actually authenticated to before opening a
// browser at it. A parse failure on either side returns false (fail
// closed — fall back to the known-good direct URL).
func sameHost(a, b string) bool {
	ua, err := neturl.Parse(a)
	if err != nil {
		return false
	}
	ub, err := neturl.Parse(b)
	if err != nil {
		return false
	}
	return ua.Host != "" && strings.EqualFold(ua.Host, ub.Host)
}

// openInBrowserCmd opens a specific OpenTarget in the user's default
// browser. Two auth modes (settings.OpenAuth):
//
//	frontdoor (default) — exchange the sfdx token for a ONE-TIME
//	  login URL via oauth2/singleaccess (the modern `sf org open`
//	  mechanism), so the browser lands authenticated even when no
//	  session exists — the "other tools pass the sfdx auth" flow.
//	  Falls back to the direct URL if the exchange fails.
//	direct — navigate straight to instanceURL+path, reusing the
//	  existing browser cookie. The pre-2026-06-13 behaviour; kept
//	  as an option because frontdoor logins mint a fresh session
//	  each time, and some strictly-configured orgs answer them
//	  with identity-verification prompts.
//
// Non-Salesforce AbsoluteURL targets always open directly.
func (m Model) openInBrowserCmd(o sf.Org, t sf.OpenTarget) tea.Cmd {
	return m.openInBrowserCmdWith(o, t, "", false)
}

// openInBrowserCmdWith is openInBrowserCmd with an explicit browser
// override and optional private/incognito mode. A non-empty override
// wins over the configured default (settings.Browser) — used by the
// open-menu browser sub-picker so a one-off "open this in Chrome"
// (or "…in a private window") doesn't change the user's default.
//
// private only applies when browserOverride names a browser with a
// CLI private mode; it silently falls back to a normal open otherwise
// (openURLPrivate returns an error we swallow into a normal openURL).
func (m Model) openInBrowserCmdWith(o sf.Org, t sf.OpenTarget, browserOverride string, private bool) tea.Cmd {
	// The demo org has no live backend — it can't mint a frontdoor URL
	// or open in a browser. Flash an explanation instead of shelling out
	// to a doomed `sf`/frontdoor call.
	if sf.IsDemoOrgTarget(targetArg(o)) || sf.IsDemoOrgTarget(o.Username) {
		return func() tea.Msg {
			return demoFlashMsg{text: "This is the demo org — connect a real org to open it in a browser."}
		}
	}
	browser := ""
	// Default matches settings.OpenAuth() ("direct") for the nil-settings
	// path; direct reuses the existing browser session and avoids the
	// fresh-session identity prompt frontdoor triggers on strict orgs.
	mode := "direct"
	if m.settings != nil {
		browser = m.settings.Browser()
		mode = m.settings.OpenAuth()
	}
	if browserOverride != "" {
		browser = browserOverride
	}
	// launch routes a resolved URL to private-or-normal open. Private
	// mode needs a named browser + CLI support; on any gap it falls
	// back to a normal open so the URL always lands somewhere.
	allowBrowserExtension := t.AllowBrowserExtension
	launch := func(url string) error {
		if private && browser != "" {
			if err := openURLPrivate(url, browser, allowBrowserExtension); err == nil {
				return nil
			}
		}
		return openURL(url, browser, allowBrowserExtension)
	}
	launchResult := func(url string) tea.Msg {
		if err := launch(url); err != nil {
			return demoFlashMsg{text: "couldn't open in browser (" + err.Error() + ") — press " +
				firstPretty(Keys.YankDefault) + " to copy the URL instead"}
		}
		return nil
	}
	if Demo {
		// The demo's URLs are fictional — launching a browser at them
		// would 404. Instead, render a self-contained local HTML page
		// that names what WOULD have opened and pop it in the browser,
		// so the `o` gesture feels real without touching Salesforce.
		return func() tea.Msg {
			if err := demoOpenTargetPage(o, t, browser); err != nil {
				return demoFlashMsg{text: "demo: couldn't open preview (" + err.Error() + ")"}
			}
			return nil
		}
	}
	if t.AbsoluteURL != "" {
		url := t.AbsoluteURL
		return func() tea.Msg {
			return launchResult(url)
		}
	}
	if o.InstanceURL != "" {
		direct := sf.FullURL(o.InstanceURL, t.Path)
		alias := targetArg(o)
		path := t.Path
		instanceURL := o.InstanceURL
		return func() tea.Msg {
			if mode == "frontdoor" {
				// Network call — runs on this tea.Cmd goroutine, not
				// the Update loop. One-time URL; safe in history.
				if u, err := sf.SingleAccessURL(alias, path); err == nil {
					// frontdoor_uri is returned verbatim from the org's
					// HTTP response. Verify it points at the SAME host we
					// authenticated to before opening a browser at it —
					// a compromised org could otherwise redirect the
					// login handoff to an attacker's site. On mismatch,
					// fall back to the known-good direct URL.
					if sameHost(u, instanceURL) {
						return launchResult(u)
					}
				}
				// Exchange failed (offline, scopes) or host mismatch →
				// direct URL is still a working link, just maybe behind
				// a login.
			}
			return launchResult(direct)
		}
	}
	// Last-resort fallback: shell to `sf` so the user gets *something*
	// open even if instanceURL hasn't landed yet. Will trigger MFA but
	// at least the link works. A failure here used to be swallowed —
	// pressing o silently did nothing — so surface it with the recovery
	// hint (yank still works without a live session).
	alias := targetArg(o)
	path := t.Path
	return func() tea.Msg {
		if _, err := sf.OrgOpen(alias, path); err != nil {
			return demoFlashMsg{text: "couldn't open in browser (" + err.Error() + ") — press " +
				firstPretty(Keys.YankDefault) + " to copy the URL instead"}
		}
		return nil
	}
}

// yankURLCmd copies the target URL to the system clipboard. For
// Salesforce targets this is instanceURL+Path; for absolute targets
// it's the raw URL.
func yankURLCmd(o sf.Org, t sf.OpenTarget) tea.Cmd {
	url := t.AbsoluteURL
	if url == "" {
		url = sf.FullURL(o.InstanceURL, t.Path)
	}
	return func() tea.Msg {
		if err := writeClipboard(url); err != nil {
			return demoFlashMsg{text: "clipboard unavailable (" + err.Error() + ") — install xclip or wl-clipboard"}
		}
		return nil
	}
}

// openURL launches the given URL in the OS default handler, or the
// named browser when one is configured (macOS: `open -a <browser>` so
// extension schemes resolve; Linux: the browser binary directly).
//
// This used to hardcode the macOS `open` binary for every platform —
// on desktop Linux (where no `open` exists) every o-open failed.
// Linux now uses xdg-open, and WSL is detected and routed through
// Windows interop so no helper package (the discontinued wslu) is
// needed to reach the Windows browser.
func openURL(url, browser string, allowBrowserExtension bool) error {
	if err := safeToOpenTargetURL(url, allowBrowserExtension); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		if browser != "" {
			cmd = exec.Command("open", "-a", browser, url)
		} else {
			// `--` stops `open` from treating a leading-dash URL as a
			// flag (belt-and-braces; safeToOpenURL already rejects those).
			cmd = exec.Command("open", "--", url)
		}
	case "windows":
		// Argv-passed to the protocol handler — no shell, so URL
		// metacharacters (&, ^) need no quoting. Exits 0 on success,
		// unlike `start` via cmd.
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default: // linux, incl. WSL
		if browser != "" {
			if bin := linuxBrowserBinary(browser); bin != "" {
				cmd = exec.Command(bin, url)
				return cmd.Run()
			}
		}
		if isWSL() {
			// WSL interop: hand the URL straight to Windows' protocol
			// handler. Works out of the box — wslu/wslview is
			// discontinued (archived 2025), so we don't depend on it.
			cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
		} else {
			cmd = exec.Command("xdg-open", url)
		}
	}
	return cmd.Run()
}

// isWSL reports whether we're running inside Windows Subsystem for
// Linux. Env vars cover the common case; the osrelease probe catches
// sessions launched without them (e.g. some service contexts).
var (
	wslOnce     sync.Once
	wslDetected bool
)

func isWSL() bool {
	wslOnce.Do(func() {
		if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
			wslDetected = true
			return
		}
		if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil &&
			strings.Contains(strings.ToLower(string(b)), "microsoft") {
			wslDetected = true
		}
	})
	return wslDetected
}

// openURLPrivate launches url in the named browser's private /
// incognito window. Returns an error when the browser has no CLI
// private mode (Safari, Arc) so the caller can fall back to a normal
// open. `browser` must be non-empty and known.
func openURLPrivate(url, browser string, allowBrowserExtension bool) error {
	if err := safeToOpenTargetURL(url, allowBrowserExtension); err != nil {
		return err
	}
	flag, ok := browserPrivateFlag(browser)
	if !ok {
		return fmt.Errorf("%s has no command-line private mode", browser)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// `open -a <app> -n --args <flag> <url>`: -n forces a new
		// instance so the flag takes effect even when the browser is
		// already running; --args passes the rest to the app.
		cmd = exec.Command("open", "-a", browser, "-n", "--args", flag, url)
	default:
		// Linux: invoke the browser binary directly with the flag.
		bin := linuxBrowserBinary(browser)
		if bin == "" {
			return fmt.Errorf("no launch binary known for %s", browser)
		}
		cmd = exec.Command(bin, flag, url)
	}
	return cmd.Run()
}

// linuxBrowserBinary maps a display name to its first candidate exec
// stem for direct invocation on Linux. Returns "" when unknown.
func linuxBrowserBinary(name string) string {
	for _, b := range knownBrowsers {
		if b.name == name && len(b.linExec) > 0 {
			return b.linExec[0]
		}
	}
	return ""
}

// openPath launches the OS default-app for a local file path. macOS
// uses `open`, Linux uses `xdg-open`, Windows uses `cmd /C start`.
// Path may be relative or absolute (the OS handler resolves either).
func openPath(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("cmd", "/C", "start", "", path)
	default:
		if isWSL() {
			// explorer.exe opens files/dirs with the Windows default
			// app — but exits 1 even on success (long-standing quirk),
			// so don't treat its status as failure.
			_ = exec.Command("explorer.exe", path).Run()
			return nil
		}
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Run()
}

// writeClipboard writes s to the system clipboard. Cross-platform
// via atotto/clipboard which dispatches to pbcopy (darwin),
// xclip / wl-copy (linux), or clip.exe (windows). Returns an error
// when no copy backend is available.
func writeClipboard(s string) error {
	return clipboard.WriteAll(s)
}
