package settings

// Feature-area accessors: /compare tuning, org-group organisation,
// and /home banner options. Split out of settings.go.

// OrgGroups returns the user's persisted org groups in render order.
// Returns nil for an unconfigured user (everything renders under
// the synthetic "Ungrouped" section).
func (s *Settings) OrgGroups() []OrgGroupConfig {
	if s == nil || len(s.UI.OrgGroups.Groups) == 0 {
		return nil
	}
	byID := make(map[string]OrgGroupConfig, len(s.UI.OrgGroups.Groups))
	for _, g := range s.UI.OrgGroups.Groups {
		byID[g.ID] = g
	}
	out := make([]OrgGroupConfig, 0, len(s.UI.OrgGroups.Groups))
	seen := map[string]bool{}
	for _, id := range s.UI.OrgGroups.Order {
		if g, ok := byID[id]; ok && !seen[id] {
			out = append(out, cloneOrgGroup(g))
			seen[id] = true
		}
	}
	for _, g := range s.UI.OrgGroups.Groups {
		if seen[g.ID] {
			continue
		}
		out = append(out, cloneOrgGroup(g))
		seen[g.ID] = true
	}
	return out
}

// SetOrgGroups replaces the persisted groups + order. Order is
// derived from the slice itself; callers that need explicit control
// can pass groups in render order. Caller owns Save().
func (s *Settings) SetOrgGroups(groups []OrgGroupConfig) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(groups) == 0 {
		s.UI.OrgGroups = OrgGroupsConfig{}
		return
	}
	out := make([]OrgGroupConfig, 0, len(groups))
	order := make([]string, 0, len(groups))
	seen := map[string]bool{}
	for _, g := range groups {
		if g.ID == "" || seen[g.ID] {
			continue
		}
		seen[g.ID] = true
		out = append(out, cloneOrgGroup(g))
		order = append(order, g.ID)
	}
	s.UI.OrgGroups = OrgGroupsConfig{Order: order, Groups: out}
}

// OrgGroupForUsername returns the id of the group that owns the
// given org username, or "" when the org is in no group (renders
// under "Ungrouped"). First match wins — schema invariant is one
// group per org but we don't trust the file blindly.
func (s *Settings) OrgGroupForUsername(username string) string {
	if s == nil || username == "" {
		return ""
	}
	for _, g := range s.UI.OrgGroups.Groups {
		for _, m := range g.Members {
			if m == username {
				return g.ID
			}
		}
	}
	return ""
}

// PruneOrgGroupMembers drops any usernames from group members that
// aren't in the supplied authed-orgs set. Returns true when something
// was removed (caller saves on true). The set is "what `sf` knows
// about right now" — orgs the user has logged out of via the CLI
// while sf-deck wasn't running.
//
// SAFETY: an EMPTY authed set is treated as "we don't currently know
// which orgs exist" — NOT "every org is logged out". Pruning on an
// empty set would wipe every group's membership, which happens during
// transient states (cache clear, startup before the org list lands, a
// failed `sf org list`). We refuse to prune in that case: losing the
// real signal momentarily must never destroy persisted assignments.
// Without this guard, clearing the cache wiped all group memberships.
func (s *Settings) PruneOrgGroupMembers(authedUsernames map[string]bool) bool {
	if s == nil || len(s.UI.OrgGroups.Groups) == 0 {
		return false
	}
	if len(authedUsernames) == 0 {

		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for i, g := range s.UI.OrgGroups.Groups {

		removed := false
		kept := make([]string, 0, len(g.Members))
		for _, m := range g.Members {
			if authedUsernames[m] {
				kept = append(kept, m)
			} else {
				removed = true
			}
		}
		if removed {
			s.UI.OrgGroups.Groups[i].Members = kept
			changed = true
		}
	}
	return changed
}

// CompareConcurrency returns the configured parallel-retrieve cap,
// clamped to a sane range, falling back to the default when unset.
func (s *Settings) CompareConcurrency() int {
	if s == nil {
		return defaultCompareConcurrency
	}
	n := s.UI.Compare.Concurrency
	if n <= 0 {
		return defaultCompareConcurrency
	}
	if n > 32 {
		n = 32
	}
	return n
}

// CompareBodyCapBytes returns the per-component retain cap in BYTES,
// clamped, falling back to the default when unset.
func (s *Settings) CompareBodyCapBytes() int {
	kb := defaultCompareBodyCapKB
	if s != nil && s.UI.Compare.BodyCapKB > 0 {
		kb = s.UI.Compare.BodyCapKB
	}
	if kb < 8 {
		kb = 8
	}
	if kb > 1<<20 {
		kb = 1 << 20
	}
	return kb * 1024
}

// CompareRetainCeilingBytes returns the total retained-body ceiling in
// BYTES, clamped, falling back to the default when unset.
func (s *Settings) CompareRetainCeilingBytes() int64 {
	mb := defaultCompareRetainCeilingMB
	if s != nil && s.UI.Compare.RetainCeilingMB > 0 {
		mb = s.UI.Compare.RetainCeilingMB
	}
	return int64(mb) * 1024 * 1024
}

// CompareDefs returns a copy of the saved comparison definitions.
func (s *Settings) CompareDefs() []CompareDef {
	if s == nil || len(s.UI.Compare.Defs) == 0 {
		return nil
	}
	out := make([]CompareDef, 0, len(s.UI.Compare.Defs))
	for _, d := range s.UI.Compare.Defs {
		out = append(out, cloneCompareDef(d))
	}
	return out
}

// SetCompareDefs replaces the saved comparison definitions. Caller owns
// Save(). Dedupes by Name (last wins), drops nameless entries.
func (s *Settings) SetCompareDefs(defs []CompareDef) {
	if s == nil {
		return
	}
	seen := map[string]int{}
	var out []CompareDef
	for _, d := range defs {
		if d.Name == "" {
			continue
		}
		if i, ok := seen[d.Name]; ok {
			out[i] = cloneCompareDef(d)
			continue
		}
		seen[d.Name] = len(out)
		out = append(out, cloneCompareDef(d))
	}
	s.UI.Compare.Defs = out
}

// HomeBannerIntervalMs returns the banner-animation tick interval
// in ms. 0 falls back to the default; values below 50ms clamp up
// (faster than that wastes CPU). When DisableHomeBanner is true
// the caller skips animation entirely; this getter still returns a
// value to keep the type contract simple.
func (s *Settings) HomeBannerIntervalMs() int {
	if s == nil || s.UI.Home.BannerIntervalMs == 0 {
		return DefaultHomeBannerIntervalMs
	}
	if s.UI.Home.BannerIntervalMs < 50 {
		return 50
	}
	return s.UI.Home.BannerIntervalMs
}

// SetHomeBannerIntervalMs persists the tick interval. n <= 0 resets.
func (s *Settings) SetHomeBannerIntervalMs(n int) {
	if s == nil {
		return
	}
	if n <= 0 {
		s.UI.Home.BannerIntervalMs = 0
		return
	}
	s.UI.Home.BannerIntervalMs = n
}

// DisableHomeBanner reports whether the banner animation is off.
func (s *Settings) DisableHomeBanner() bool {
	if s == nil {
		return false
	}
	return s.UI.Home.DisableBanner
}

// SetDisableHomeBanner persists the banner-disable flag.
func (s *Settings) SetDisableHomeBanner(v bool) {
	if s == nil {
		return
	}
	s.UI.Home.DisableBanner = v
}

// HideHomeBanner reports whether the /home cloud banner is hidden
// entirely (vs DisableHomeBanner which only freezes its animation).
func (s *Settings) HideHomeBanner() bool {
	if s == nil {
		return false
	}
	return s.UI.Home.HideBanner
}

// SetHideHomeBanner persists the banner-hide flag.
func (s *Settings) SetHideHomeBanner(v bool) {
	if s == nil {
		return
	}
	s.UI.Home.HideBanner = v
}
