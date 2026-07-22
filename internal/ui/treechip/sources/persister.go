package sources

import (
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/treechip"
)

// SettingsPersister adapts settings.Settings to treechip.Persister.
// Each (orgUser, domain) pair gets its own persister so the registry
// doesn't need to know about org scoping.
type SettingsPersister struct {
	settings *settings.Settings
	orgUser  string
	domain   string
}

// NewSettingsPersister wires up the bridge. Returns nil if any of
// the bits are missing — treechip handles a nil Persister cleanly
// (no persistence, in-memory state only).
func NewSettingsPersister(s *settings.Settings, orgUser, domain string) *SettingsPersister {
	if s == nil || orgUser == "" || domain == "" {
		return nil
	}
	return &SettingsPersister{settings: s, orgUser: orgUser, domain: domain}
}

// Load returns the persisted pins + last-path for this (org, domain).
func (p *SettingsPersister) Load() (pins []string, lastPath []string) {
	if p == nil {
		return nil, nil
	}
	cfg := p.settings.TreeChipForOrg(p.orgUser, p.domain)
	return cfg.Pins, cfg.LastPath
}

// Save persists pins + last-path. Calls settings.Save() so changes
// hit disk immediately — the registry expects persisted state to be
// durable across runs.
func (p *SettingsPersister) Save(pins []string, lastPath []string) {
	if p == nil {
		return
	}
	p.settings.SetTreeChipForOrg(p.orgUser, p.domain, settings.TreeChipConfig{
		Pins:     pins,
		LastPath: lastPath,
	})
	_ = p.settings.Save()
}

// Compile-time assertion: SettingsPersister implements treechip.Persister.
var _ treechip.Persister = (*SettingsPersister)(nil)
