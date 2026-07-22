package ui

// Demo-mode browser handoff. In --demo mode there is no real org, so
// the open-in-browser gestures (`o` on a record/object/flow, and the
// add-org login flow) can't hit Salesforce. Rather than flash a
// dead-end "can't open in demo" message OR shell out to the real
// `sf org login web`, we render a small self-contained HTML page to a
// temp file and open THAT in the browser.
//
// The result: a demo user pressing `o` actually sees a browser tab
// pop up — the gesture feels real and is discoverable — but nothing
// external is touched. The page explains what would have happened
// against a real org.

import (
	"fmt"
	"html"
	"os"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// demoOpenTargetPage renders a context-aware "this is what would have
// opened" HTML page for a cursored open target, writes it to a temp
// file, and opens it in the browser. Returns an error only when the
// temp file can't be written; the browser launch itself is
// best-effort.
func demoOpenTargetPage(o sf.Org, t sf.OpenTarget, browser string) error {
	label := t.Label
	if label == "" {
		label = "this item"
	}
	url := t.AbsoluteURL
	if url == "" {
		url = sf.FullURL(o.InstanceURL, t.Path)
	}
	orgName := o.Alias
	if orgName == "" {
		orgName = o.Username
	}
	if orgName == "" {
		orgName = "the demo org"
	}

	body := fmt.Sprintf(`
    <div class="badge">DEMO MODE</div>
    <h1>%s</h1>
    <p>In a real org, pressing <span class="key">o</span> here would open
       this in Lightning at:</p>
    <p class="url">%s</p>
    <p class="muted">Org: %s</p>
    <hr>
    <p>You're running <strong>sf-deck --demo</strong>, a fully offline tour
       against a fictional org. Nothing is read from or written to
       Salesforce.</p>
    <p>Run <code>sf-deck</code> without <code>--demo</code> to point it at
       the orgs your <code>sf</code> CLI already knows.</p>`,
		html.EscapeString(label),
		html.EscapeString(url),
		html.EscapeString(orgName),
	)
	return writeAndOpenDemoPage("sf-deck-demo-open-*.html", "sf-deck demo · open", body, browser)
}

// demoAddOrgPage renders the "adding an org is disabled in demo" page
// and opens it. Replaces the real `sf org login web` shell-out so
// demo mode never launches a genuine auth flow.
func demoAddOrgPage(browser string) error {
	body := `
    <div class="badge">DEMO MODE</div>
    <h1>Connect a Salesforce org</h1>
    <p>In a real session, this runs <code>sf org login web</code> — a
       browser-based OAuth flow that hands the new org's credentials to
       your <code>sf</code> CLI.</p>
    <p>That's <strong>disabled in demo mode</strong>: the demo is fully
       offline and never touches a real Salesforce login.</p>
    <hr>
    <p>To connect real orgs, quit the demo and either:</p>
    <ul>
      <li>run <code>sf org login web</code> in your terminal, then launch
          <code>sf-deck</code>, or</li>
      <li>launch <code>sf-deck</code> against orgs your <code>sf</code>
          CLI already knows.</li>
    </ul>`
	return writeAndOpenDemoPage("sf-deck-demo-addorg-*.html", "sf-deck demo · add org", body, browser)
}

// writeAndOpenDemoPage wraps a body fragment in the shared demo-page
// shell, writes it to a temp .html file (0600, unpredictable name via
// os.CreateTemp), and opens it in the browser. The temp file is left
// on disk deliberately — the browser needs it after this returns; the
// OS temp dir is cleaned by the system, and the files are tiny.
func writeAndOpenDemoPage(pattern, title, bodyHTML, browser string) error {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return err
	}
	page := demoPageShell(title, bodyHTML)
	if _, err := f.WriteString(page); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// file:// URL. openURL rejects non-http(s), so open the path
	// directly via openPath (which handles the OS default browser for
	// .html). Best-effort — a launch failure isn't fatal to the demo.
	_ = openPath(f.Name())
	return nil
}

// demoPageShell wraps a body fragment in a minimal dark-themed HTML
// document matching sf-deck's aesthetic. Self-contained (inline CSS,
// no external assets) so it renders identically offline.
func demoPageShell(title, bodyHTML string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + html.EscapeString(title) + `</title>
<style>
  :root { color-scheme: dark; }
  body {
    margin: 0; min-height: 100vh;
    display: flex; align-items: center; justify-content: center;
    background: #1a1b26; color: #c0caf5;
    font: 16px/1.55 -apple-system, "SF Pro Text", Segoe UI, Roboto, sans-serif;
  }
  .card {
    max-width: 560px; margin: 2rem; padding: 2rem 2.25rem;
    background: #24283b; border: 1px solid #414868; border-radius: 12px;
    box-shadow: 0 8px 40px rgba(0,0,0,.4);
  }
  .badge {
    display: inline-block; margin-bottom: 1rem; padding: .2rem .6rem;
    background: #bb9af7; color: #1a1b26; font-weight: 700;
    font-size: .72rem; letter-spacing: .08em; border-radius: 5px;
  }
  h1 { margin: 0 0 1rem; font-size: 1.4rem; color: #7aa2f7; }
  p { margin: .7rem 0; }
  .url { font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
         word-break: break-all; color: #9ece6a; }
  .muted { color: #565f89; font-size: .9rem; }
  .key, code {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    background: #1a1b26; border: 1px solid #414868; border-radius: 4px;
    padding: .05rem .4rem; font-size: .92em; color: #c0caf5;
  }
  hr { border: 0; border-top: 1px solid #414868; margin: 1.4rem 0; }
  ul { margin: .5rem 0; padding-left: 1.2rem; }
  li { margin: .35rem 0; }
  a { color: #7aa2f7; }
</style>
</head>
<body>
  <div class="card">` + bodyHTML + `
  </div>
</body>
</html>
`
}
