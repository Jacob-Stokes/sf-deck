// Package buildinfo exposes the metadata stamped into release binaries.
//
// The executable owns the linker-injected variables and calls Set once at
// process startup. Keeping the read side here lets the TUI, headless CLI, and
// update checker share one value without importing package main.
package buildinfo

import (
	"strings"
	"sync"
)

// Info describes the running sf-deck binary.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"built_at"`
}

var (
	mu      sync.RWMutex
	current = Info{Version: "dev", Commit: "none", Date: "unknown"}
)

// Set replaces the process-wide build metadata. Empty values fall back to the
// same development labels used by a bare `go build`.
func Set(version, commit, date string) {
	if strings.TrimSpace(version) == "" {
		version = "dev"
	}
	if strings.TrimSpace(commit) == "" {
		commit = "none"
	}
	if strings.TrimSpace(date) == "" {
		date = "unknown"
	}
	mu.Lock()
	current = Info{Version: version, Commit: commit, Date: date}
	mu.Unlock()
}

// Current returns a snapshot of the running binary's build metadata.
func Current() Info {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// IsDevelopment reports whether this is an unstamped local build.
func (i Info) IsDevelopment() bool {
	v := strings.TrimSpace(strings.ToLower(i.Version))
	return v == "" || v == "dev" || v == "development" || v == "snapshot"
}

// DisplayVersion returns a human-facing version with a v prefix.
func (i Info) DisplayVersion() string {
	v := strings.TrimSpace(i.Version)
	if i.IsDevelopment() || strings.HasPrefix(strings.ToLower(v), "v") {
		return v
	}
	return "v" + v
}

// ShortCommit keeps About compact while preserving enough of a Git SHA to
// identify the build. Non-SHA development labels are returned unchanged.
func (i Info) ShortCommit() string {
	c := strings.TrimSpace(i.Commit)
	if len(c) > 12 {
		return c[:12]
	}
	return c
}
