package settings

import "strings"

// Chip / lens / object-filter + tree-chip persistence accessors.
// Split out of settings.go.

// DefaultChipLimit resolves the effective chip-fetch row cap —
// settings override > package default. Returns >= 1.
func (s *Settings) DefaultChipLimit() int {
	if s == nil {
		return DefaultChipLimitFallback
	}
	if s.UI.ChipDefaults.DefaultLimit > 0 {
		return s.UI.ChipDefaults.DefaultLimit
	}
	if s.UI.ChipDefaults.DefaultLimit < 0 {
		return 1
	}
	return DefaultChipLimitFallback
}

// SetDefaultChipLimit writes the user-set chip cap. Pass 0 to clear
// the override (back to DefaultChipLimitFallback).
func (s *Settings) SetDefaultChipLimit(n int) {
	if s == nil {
		return
	}
	if n < 0 {
		n = 0
	}
	s.UI.ChipDefaults.DefaultLimit = n
}

// ObjectFilters / FlowFilters / Lenses — legacy accessors kept for one
// release while the UI migrates to the unified Chips list.
func (s *Settings) ObjectFilters() []FilterConfig {
	if s == nil {
		return nil
	}
	return s.UI.ObjectFilters
}

func (s *Settings) SetObjectFilters(fs []FilterConfig) { s.UI.ObjectFilters = fs }

func (s *Settings) UpsertObjectFilter(f FilterConfig) {
	for i, x := range s.UI.ObjectFilters {
		if x.ID == f.ID {
			s.UI.ObjectFilters[i] = f
			return
		}
	}
	s.UI.ObjectFilters = append(s.UI.ObjectFilters, f)
}

func (s *Settings) DeleteObjectFilter(id string) {
	out := s.UI.ObjectFilters[:0]
	for _, x := range s.UI.ObjectFilters {
		if x.ID != id {
			out = append(out, x)
		}
	}
	s.UI.ObjectFilters = out
}

func (s *Settings) Lenses() []LensConfig {
	if s == nil {
		return nil
	}
	return s.UI.Lenses
}

func (s *Settings) SetLenses(ls []LensConfig) { s.UI.Lenses = ls }

func (s *Settings) UpsertLens(l LensConfig) {
	for i, existing := range s.UI.Lenses {
		if existing.ID == l.ID {
			s.UI.Lenses[i] = l
			return
		}
	}
	s.UI.Lenses = append(s.UI.Lenses, l)
}

func (s *Settings) DeleteLens(id string) {
	out := s.UI.Lenses[:0]
	for _, existing := range s.UI.Lenses {
		if existing.ID != id {
			out = append(out, existing)
		}
	}
	s.UI.Lenses = out
}

// ClearLegacyChips drops all three legacy slices. Called by the UI
// migrator after entries have been converted into ChipConfig so the
// next Save() drops the old sections from disk.
func (s *Settings) ClearLegacyChips() {
	if s == nil {
		return
	}
	s.UI.Lenses = nil
	s.UI.ObjectFilters = nil
	s.UI.FlowFilters = nil
}

// Chips returns the unified user chip slice (built-ins live in code).
func (s *Settings) Chips() []ChipConfig {
	if s == nil {
		return nil
	}
	return s.UI.Chips
}

// ChipsForDomain returns chips matching the given domain — "records",
// "objects", "flows".
func (s *Settings) ChipsForDomain(domain string) []ChipConfig {
	if s == nil {
		return nil
	}
	out := make([]ChipConfig, 0, len(s.UI.Chips))
	for _, c := range s.UI.Chips {
		if c.Domain == domain {
			out = append(out, c)
		}
	}
	return out
}

// SetChips replaces the entire unified slice. Callers own Save().
// Each entry is normalised so any legacy OrgUser is rewritten into Share
// — keeps the on-disk shape uniform after a bulk replace.
func (s *Settings) SetChips(cs []ChipConfig) {
	for i := range cs {
		cs[i].NormaliseShare()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UI.Chips = cs
}

// UpsertChip adds-or-replaces by ID. Domain is part of the identity
// key (records.recent ≠ objects.recent), so we match on both. Legacy
// OrgUser is migrated to Share before write so freshly-saved chips
// never carry both shapes.
func (s *Settings) UpsertChip(c ChipConfig) {
	c.NormaliseShare()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, x := range s.UI.Chips {
		if x.ID == c.ID && x.Domain == c.Domain {
			s.UI.Chips[i] = c
			return
		}
	}
	s.UI.Chips = append(s.UI.Chips, c)
}

// DeleteChip removes by (domain, id). No-op when absent.
func (s *Settings) DeleteChip(domain, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]ChipConfig, 0, len(s.UI.Chips))
	for _, c := range s.UI.Chips {
		if c.Domain == domain && c.ID == id {
			continue
		}
		out = append(out, c)
	}
	s.UI.Chips = out
}

// ChipFavouriteOverridesFor returns the slice of "<domain>.<chip-id>"
// keys → bool mappings that apply to the given domain. The map is
// flat per-key but the domain prefix lets one settings.toml store
// overrides for records / objects / flows in the same map.
func (s *Settings) ChipFavouriteOverridesFor(domain string) map[string]bool {
	if s == nil || len(s.UI.ChipBuiltinFavOverrides) == 0 {
		return nil
	}
	prefix := domain + "."
	out := map[string]bool{}
	for k, v := range s.UI.ChipBuiltinFavOverrides {
		if strings.HasPrefix(k, prefix) {
			out[strings.TrimPrefix(k, prefix)] = v
		}
	}
	return out
}

// SetChipFavouriteOverridesFor replaces the per-domain entries in the
// override map. Keys are stored as "<domain>.<chip-id>" so a single
// settings file holds every domain's overrides without nesting.
func (s *Settings) SetChipFavouriteOverridesFor(domain string, overrides map[string]bool) {
	if s == nil {
		return
	}
	if s.UI.ChipBuiltinFavOverrides == nil {
		s.UI.ChipBuiltinFavOverrides = map[string]bool{}
	}

	prefix := domain + "."
	for k := range s.UI.ChipBuiltinFavOverrides {
		if strings.HasPrefix(k, prefix) {
			delete(s.UI.ChipBuiltinFavOverrides, k)
		}
	}
	for k, v := range overrides {
		s.UI.ChipBuiltinFavOverrides[prefix+k] = v
	}
}

// TreeChipForOrg returns the persisted treechip state for an
// (org, domain) pair — pinned node IDs + last visited path. Both
// slices are empty when nothing's been recorded yet.
func (s *Settings) TreeChipForOrg(orgUser, domain string) TreeChipConfig {
	if s == nil || orgUser == "" || domain == "" {
		return TreeChipConfig{}
	}
	per, ok := s.UI.TreeChipByOrg[orgUser]
	if !ok {
		return TreeChipConfig{}
	}
	cfg, ok := per[domain]
	if !ok {
		return TreeChipConfig{}
	}
	out := TreeChipConfig{}
	if len(cfg.Pins) > 0 {
		out.Pins = make([]string, len(cfg.Pins))
		copy(out.Pins, cfg.Pins)
	}
	if len(cfg.LastPath) > 0 {
		out.LastPath = make([]string, len(cfg.LastPath))
		copy(out.LastPath, cfg.LastPath)
	}
	return out
}

// SetTreeChipForOrg replaces the persisted treechip state for one
// (org, domain). Empty pins + empty path clears the entry so
// settings.toml stays tidy.
func (s *Settings) SetTreeChipForOrg(orgUser, domain string, cfg TreeChipConfig) {
	if s == nil || orgUser == "" || domain == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.UI.TreeChipByOrg == nil {
		s.UI.TreeChipByOrg = map[string]map[string]TreeChipConfig{}
	}
	per, ok := s.UI.TreeChipByOrg[orgUser]
	if !ok {
		per = map[string]TreeChipConfig{}
		s.UI.TreeChipByOrg[orgUser] = per
	}
	if len(cfg.Pins) == 0 && len(cfg.LastPath) == 0 {
		delete(per, domain)
		if len(per) == 0 {
			delete(s.UI.TreeChipByOrg, orgUser)
		}
		return
	}
	cp := TreeChipConfig{}
	if len(cfg.Pins) > 0 {
		cp.Pins = make([]string, len(cfg.Pins))
		copy(cp.Pins, cfg.Pins)
	}
	if len(cfg.LastPath) > 0 {
		cp.LastPath = make([]string, len(cfg.LastPath))
		copy(cp.LastPath, cfg.LastPath)
	}
	per[domain] = cp
}
