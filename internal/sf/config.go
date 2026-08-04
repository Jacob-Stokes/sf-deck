package sf

import (
	"sync"
	"time"
)

// Tunable client config.
//
// The sf package is a lower layer than internal/settings (settings reads
// user TOML; sf does the actual API work), so sf must NOT import
// settings — that would invert the dependency and risk a cycle. Instead
// the UI reads the user's preferences from settings at startup and pushes
// them down here via ApplyConfig. Everything has a sane default so sf
// works standalone (tests, headless callers) without any ApplyConfig
// call.
//
// Reads go through the accessor funcs (cfgHTTPTimeout, etc.) which take
// the lock, so a startup ApplyConfig racing the first API call is safe.

var (
	cfgMu sync.RWMutex
	cfg   = clientConfig{
		HTTPTimeout: 30 * time.Second,
		CLITimeout:  30 * time.Second,
		// RetrieveTimeout caps `sf project retrieve start` /
		// `sf project deploy start` / `sf project deploy validate`
		// shell-outs. Salesforce-side validates with Apex test
		// runs routinely take 5-15 min on busy orgs (queue time +
		// test execution); the previous 5min cap killed sf-deck's
		// client before SF returned for any non-trivial manifest.
		// 20min covers ~99% of real-world validates while still
		// catching truly hung processes.
		RetrieveTimeout: 20 * time.Minute,
		DeployDeadline:  60 * time.Second,
		// Steady-state deploy poll (after the 500ms fast-start in
		// pollDeploy). Mirrors settings.APIDeployPollMsFallback.
		DeployPoll: 5 * time.Second,
		BulkPoll:   5 * time.Second,
		APIVersion: "", // "" = use the org-reported version
	}
)

// clientConfig holds the tunable knobs. Zero-value durations are
// rejected by ApplyConfig (it keeps the current value), so a partial
// Config from the UI never clobbers a default with 0.
type clientConfig struct {
	HTTPTimeout     time.Duration
	CLITimeout      time.Duration
	RetrieveTimeout time.Duration
	DeployDeadline  time.Duration
	DeployPoll      time.Duration
	BulkPoll        time.Duration
	APIVersion      string
	FlowOpenVersion string
}

// Config is the public shape the UI fills from settings and hands to
// ApplyConfig. Any zero-value field is left at its current value, so
// callers only set what they want to override.
type Config struct {
	HTTPTimeout     time.Duration
	CLITimeout      time.Duration
	RetrieveTimeout time.Duration
	DeployDeadline  time.Duration
	DeployPoll      time.Duration
	BulkPoll        time.Duration
	APIVersion      string // "" leaves the org-reported version in effect
	FlowOpenVersion string // "active" = flows-list `o` opens the active version; anything else = latest
}

// ApplyConfig merges c into the package config. Zero-value duration
// fields are ignored (left at the current value); APIVersion is always
// applied (so "" can be set to clear a forced version). Safe to call
// once at startup before any client work; concurrent reads take the
// RLock.
func ApplyConfig(c Config) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	if c.HTTPTimeout > 0 {
		cfg.HTTPTimeout = c.HTTPTimeout
	}
	if c.CLITimeout > 0 {
		cfg.CLITimeout = c.CLITimeout
	}
	if c.RetrieveTimeout > 0 {
		cfg.RetrieveTimeout = c.RetrieveTimeout
	}
	if c.DeployDeadline > 0 {
		cfg.DeployDeadline = c.DeployDeadline
	}
	if c.DeployPoll > 0 {
		cfg.DeployPoll = c.DeployPoll
	}
	if c.BulkPoll > 0 {
		cfg.BulkPoll = c.BulkPoll
	}
	cfg.APIVersion = c.APIVersion
	cfg.FlowOpenVersion = c.FlowOpenVersion
}

func cfgHTTPTimeout() time.Duration { cfgMu.RLock(); defer cfgMu.RUnlock(); return cfg.HTTPTimeout }
func cfgCLITimeout() time.Duration  { cfgMu.RLock(); defer cfgMu.RUnlock(); return cfg.CLITimeout }
func cfgRetrieveTimeout() time.Duration {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return cfg.RetrieveTimeout
}
func cfgDeployDeadline() time.Duration {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return cfg.DeployDeadline
}
func cfgDeployPoll() time.Duration { cfgMu.RLock(); defer cfgMu.RUnlock(); return cfg.DeployPoll }
func cfgBulkPoll() time.Duration   { cfgMu.RLock(); defer cfgMu.RUnlock(); return cfg.BulkPoll }

// cfgAPIVersion returns the user-forced API version, or "" when unset
// (caller should fall back to the org-reported / defaultAPIVersion).
func cfgAPIVersion() string { cfgMu.RLock(); defer cfgMu.RUnlock(); return cfg.APIVersion }

// cfgFlowOpenActive reports whether the user prefers the flows-list `o`
// to open the ACTIVE flow version instead of the latest. Default (unset
// or any other value) is latest-first — the most recent version
// regardless of status, matching Setup's own flow list.
func cfgFlowOpenActive() bool {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return cfg.FlowOpenVersion == "active"
}
