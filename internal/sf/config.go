package sf

import (
	"sync"
	"time"
)

// Tunable client config.

var (
	cfgMu sync.RWMutex
	cfg   = clientConfig{
		HTTPTimeout:     30 * time.Second,
		CLITimeout:      30 * time.Second,
		RetrieveTimeout: 20 * time.Minute,
		DeployDeadline:  60 * time.Second,
		DeployPoll:      10 * time.Second,
		BulkPoll:        5 * time.Second,
		APIVersion:      "", // "" = use the org-reported version
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

func cfgAPIVersion() string { cfgMu.RLock(); defer cfgMu.RUnlock(); return cfg.APIVersion }

func cfgFlowOpenActive() bool {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return cfg.FlowOpenVersion == "active"
}
