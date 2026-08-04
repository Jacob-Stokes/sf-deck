package settings

import "time"

// Cache-TTL and mouse-wheel tuning accessors. Split out of settings.go.

// CacheTTL resolves the effective TTL for a resource key. Per-resource
// override beats default_ttl; default_ttl beats the hardcoded fallback
// which the caller passes as fallback. Hardcoded fallbacks stay at the
// call site (each Resource[T] already declares its natural TTL) so a
// missing config silently behaves the same as before.
func (s *Settings) CacheTTL(key string, fallback time.Duration) time.Duration {
	if s == nil {
		return fallback
	}
	if v, ok := s.UI.Cache.TTL[key]; ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	if s.UI.Cache.DefaultTTL != "" {
		if d, err := time.ParseDuration(s.UI.Cache.DefaultTTL); err == nil {
			return d
		}
	}
	return fallback
}

// CacheTTLOverride returns the raw override string set by the user
// for a given key, or "" when no override is configured. Surfaces
// in the cache-settings modal so users can tell at a glance which
// rows are theirs vs the shipped defaults.
func (s *Settings) CacheTTLOverride(key string) string {
	if s == nil {
		return ""
	}
	return s.UI.Cache.TTL[key]
}

// SetCacheTTLOverride writes (or clears with an empty value) the
// per-key TTL override. Caller owns Save().
func (s *Settings) SetCacheTTLOverride(key, value string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.UI.Cache.TTL == nil {
		s.UI.Cache.TTL = map[string]string{}
	}
	if value == "" {
		delete(s.UI.Cache.TTL, key)
		return
	}
	s.UI.Cache.TTL[key] = value
}

// WheelQuietGapMs returns the wheel-throttle idle window. Defaults
// to DefaultWheelQuietGapMs when unset; clamps negatives to 1.
func (s *Settings) WheelQuietGapMs() int {
	if s == nil || s.UI.Input.WheelQuietGapMs == 0 {
		return DefaultWheelQuietGapMs
	}
	if s.UI.Input.WheelQuietGapMs < 1 {
		return 1
	}
	return s.UI.Input.WheelQuietGapMs
}

// SetWheelQuietGapMs persists the idle window. n <= 0 resets to default.
func (s *Settings) SetWheelQuietGapMs(n int) {
	if s == nil {
		return
	}
	if n <= 0 {
		s.UI.Input.WheelQuietGapMs = 0
		return
	}
	s.UI.Input.WheelQuietGapMs = n
}

// WheelMinIntervalMs returns the minimum gap between accepted ticks.
// Defaults to DefaultWheelMinIntervalMs when unset; clamps negatives
// to 1.
func (s *Settings) WheelMinIntervalMs() int {
	if s == nil || s.UI.Input.WheelMinIntervalMs == 0 {
		return DefaultWheelMinIntervalMs
	}
	if s.UI.Input.WheelMinIntervalMs < 1 {
		return 1
	}
	return s.UI.Input.WheelMinIntervalMs
}

// SetWheelMinIntervalMs persists the min interval. n <= 0 resets.
func (s *Settings) SetWheelMinIntervalMs(n int) {
	if s == nil {
		return
	}
	if n <= 0 {
		s.UI.Input.WheelMinIntervalMs = 0
		return
	}
	s.UI.Input.WheelMinIntervalMs = n
}

// WheelMaxStep returns the cap on cursor delta per accepted wheel.
// Defaults to DefaultWheelMaxStep when unset; clamps negatives to 1.
func (s *Settings) WheelMaxStep() int {
	if s == nil || s.UI.Input.WheelMaxStep == 0 {
		return DefaultWheelMaxStep
	}
	if s.UI.Input.WheelMaxStep < 1 {
		return 1
	}
	return s.UI.Input.WheelMaxStep
}

// SetWheelMaxStep persists the per-tick cursor cap. n <= 0 resets.
func (s *Settings) SetWheelMaxStep(n int) {
	if s == nil {
		return
	}
	if n <= 0 {
		s.UI.Input.WheelMaxStep = 0
		return
	}
	s.UI.Input.WheelMaxStep = n
}
