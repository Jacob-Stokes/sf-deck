package ui

// Browser auto-discovery. Rather than show every browser name whether
// or not it's installed, we filter a known list down to the ones
// actually present on the machine:
//
//   macOS — stat each browser's .app bundle in /Applications and
//           ~/Applications.
//   Linux — match known exec names against .desktop files in the
//           standard applications dirs.
//   other — fall back to the full known list (better a superset than
//           an empty picker).
//
// The result always leads with "" (system default). The settings
// browser picker and the open-menu browser sub-picker both consume
// this, so a machine with only Firefox + Safari shows exactly those.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// knownBrowser pairs a user-facing name (what `open -a <name>` /
// settings.Browser expects) with the discovery hints per platform.
type knownBrowser struct {
	name    string   // Launch Services / display name
	macApp  string   // .app bundle basename (without /Applications prefix)
	linExec []string // candidate exec/.desktop stems on Linux
	// privateFlag is the command-line flag that opens a private /
	// incognito window, or "" when the browser has no CLI private
	// mode (Safari, Arc — macOS can't drive their private mode via
	// `open`). Used by the open-menu browser sub-picker's shift+enter.
	privateFlag string
}

// knownBrowsers is the discovery catalogue. Order here is the order
// installed browsers appear in the picker (after the leading default).
var knownBrowsers = []knownBrowser{
	{name: "Safari", macApp: "Safari.app"}, // no CLI private mode
	{name: "Google Chrome", macApp: "Google Chrome.app", linExec: []string{"google-chrome", "google-chrome-stable"}, privateFlag: "--incognito"},
	{name: "Firefox", macApp: "Firefox.app", linExec: []string{"firefox"}, privateFlag: "--private-window"},
	{name: "Firefox Developer Edition", macApp: "Firefox Developer Edition.app", linExec: []string{"firefox-developer-edition"}, privateFlag: "--private-window"},
	{name: "Arc", macApp: "Arc.app", linExec: []string{"arc"}}, // no CLI private mode
	{name: "Brave Browser", macApp: "Brave Browser.app", linExec: []string{"brave-browser", "brave"}, privateFlag: "--incognito"},
	{name: "Microsoft Edge", macApp: "Microsoft Edge.app", linExec: []string{"microsoft-edge", "msedge"}, privateFlag: "--inprivate"},
	{name: "Chromium", macApp: "Chromium.app", linExec: []string{"chromium", "chromium-browser"}, privateFlag: "--incognito"},
	{name: "Google Chrome Canary", macApp: "Google Chrome Canary.app", privateFlag: "--incognito"},
	{name: "Vivaldi", macApp: "Vivaldi.app", linExec: []string{"vivaldi", "vivaldi-stable"}, privateFlag: "--incognito"},
}

// browserPrivateFlag returns the private/incognito CLI flag for a
// browser name, and whether that browser supports one at all.
func browserPrivateFlag(name string) (string, bool) {
	for _, b := range knownBrowsers {
		if b.name == name {
			return b.privateFlag, b.privateFlag != ""
		}
	}
	return "", false
}

// discoverBrowsers returns the names of installed browsers on this
// machine, most-common-first. Never includes the "" default sentinel —
// callers prepend that. Falls back to the full known-name list when
// discovery can't run (unknown OS) or finds nothing, so the picker is
// never empty.
func discoverBrowsers() []string {
	var found []string
	switch runtime.GOOS {
	case "darwin":
		found = discoverBrowsersMac()
	case "linux":
		found = discoverBrowsersLinux()
	}
	if len(found) == 0 {
		// Unknown OS or nothing detected — return the full catalogue
		// so the user still has choices (open -a resolves whatever is
		// actually installed; a missing one just errors on launch).
		for _, b := range knownBrowsers {
			found = append(found, b.name)
		}
	}
	return found
}

// discoverBrowsersMac stats each known .app bundle in the system and
// user Applications dirs.
func discoverBrowsersMac() []string {
	dirs := []string{"/Applications"}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}
	var out []string
	for _, b := range knownBrowsers {
		if b.macApp == "" {
			continue
		}
		for _, dir := range dirs {
			if _, err := os.Stat(filepath.Join(dir, b.macApp)); err == nil {
				out = append(out, b.name)
				break
			}
		}
	}
	return out
}

// discoverBrowsersLinux scans the standard .desktop application dirs
// for files whose basename matches a known browser exec stem.
func discoverBrowsersLinux() []string {
	dirs := []string{
		"/usr/share/applications",
		"/usr/local/share/applications",
		"/var/lib/flatpak/exports/share/applications",
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".local/share/applications"),
			filepath.Join(home, ".local/share/flatpak/exports/share/applications"),
		)
	}
	// Gather every .desktop basename (lowercased, sans extension) once.
	present := map[string]bool{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			n := strings.ToLower(e.Name())
			if strings.HasSuffix(n, ".desktop") {
				present[strings.TrimSuffix(n, ".desktop")] = true
			}
		}
	}
	var out []string
	for _, b := range knownBrowsers {
		for _, stem := range b.linExec {
			// Match either an exact stem or a reverse-DNS .desktop id
			// ending in the stem (e.g. org.mozilla.firefox).
			hit := present[stem]
			if !hit {
				for id := range present {
					if strings.HasSuffix(id, "."+stem) || strings.HasSuffix(id, stem) {
						hit = true
						break
					}
				}
			}
			if hit {
				out = append(out, b.name)
				break
			}
		}
	}
	return out
}

// browserChoiceOptions builds the choiceOption list for a browser
// picker: "(system default)" first, then each discovered browser.
// `current` pre-positions the returned cursor. Returns (opts, cursor).
func browserChoiceOptions(current string) ([]choiceOption, int) {
	opts := []choiceOption{
		{Label: "(system default)", Hint: "your OS default browser", Value: ""},
	}
	cursor := 0
	for i, name := range discoverBrowsers() {
		opts = append(opts, choiceOption{Label: name, Value: name})
		if name == current {
			cursor = i + 1 // +1 for the leading default row
		}
	}
	return opts, cursor
}
