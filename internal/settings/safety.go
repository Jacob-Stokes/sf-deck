package settings

// SafetyLevel + WriteKind + per-org safety resolution.
//
// Extracted from settings.go to keep the safety policy code (the
// load-bearing guardrail for every write path) self-contained and
// independently testable. settings_test.go covers the lookup ladder;
// the rest of the package writes/reads via the helpers here.

import "strings"

// SafetyLevel is the per-org write policy. Each level is a strict
// superset of the ones above — Records implies ReadOnly, Metadata
// implies Records, etc. Gating logic elsewhere uses CanWrite(kind).
type SafetyLevel int

const (
	SafetyReadOnly SafetyLevel = iota
	SafetyRecords              // record DML allowed
	SafetyMetadata             // + field/object changes, deploys
	SafetyFull                 // + execute-anonymous Apex, destructive ops
)

// String returns the canonical lowercase snake-case name used in TOML
// and surfaced in UI copy.
func (s SafetyLevel) String() string {
	switch s {
	case SafetyReadOnly:
		return "read_only"
	case SafetyRecords:
		return "records"
	case SafetyMetadata:
		return "metadata"
	case SafetyFull:
		return "full"
	}
	return "unknown"
}

// Label returns a short human label for the status bar / header pill.
func (s SafetyLevel) Label() string {
	switch s {
	case SafetyReadOnly:
		return "READ"
	case SafetyRecords:
		return "REC"
	case SafetyMetadata:
		return "META"
	case SafetyFull:
		return "FULL"
	}
	return "?"
}

// ParseSafetyLevel converts the TOML string back to a SafetyLevel.
// Unknown values default to ReadOnly — fail closed.
func ParseSafetyLevel(s string) SafetyLevel {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "read_only", "readonly", "ro":
		return SafetyReadOnly
	case "records", "rec":
		return SafetyRecords
	case "metadata", "meta":
		return SafetyMetadata
	case "full":
		return SafetyFull
	}
	return SafetyReadOnly
}

// WriteKind classifies an action being gated. SafetyLevel.Allows(kind)
// is the single chokepoint the rest of the app calls before doing
// anything that mutates an org.
type WriteKind int

const (
	WriteRecord    WriteKind = iota // DML on data records
	WriteMetadata                   // schema, layouts, flows, etc.
	WriteAnonymous                  // execute-anonymous Apex (can do anything)
)

// Allows reports whether the given SafetyLevel permits the WriteKind.
func (s SafetyLevel) Allows(k WriteKind) bool {
	switch k {
	case WriteRecord:
		return s >= SafetyRecords
	case WriteMetadata:
		return s >= SafetyMetadata
	case WriteAnonymous:
		return s >= SafetyFull
	}
	return false
}

// SetOrg writes a per-org safety override. Passing clear=true removes
// the override (back to defaults), but preserves the Default pin so
// "I want this org to open at startup, but with default safety" stays
// expressible.
func (s *Settings) SetOrg(username string, level SafetyLevel, clear bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Orgs == nil {
		s.Orgs = map[string]OrgConfig{}
	}
	cur := s.Orgs[username]
	if clear {
		// Clear the safety override but keep Default + any future
		// per-org bits. If nothing's left, drop the entry so the
		// TOML stays tidy.
		cur.Safety = ""
		if !cur.Default {
			delete(s.Orgs, username)
			return
		}
		s.Orgs[username] = cur
		return
	}
	cur.Safety = level.String()
	s.Orgs[username] = cur
}

// OrgSafetyOverride returns the explicit safety override for the first
// matching key (username first, then aliases). The locked accessor keeps UI
// and IPC readers from racing SetOrg's map mutation.
func (s *Settings) OrgSafetyOverride(username string, aliases ...string) (string, bool) {
	if s == nil {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if cfg, ok := s.Orgs[username]; ok && cfg.Safety != "" {
		return cfg.Safety, true
	}
	for _, alias := range aliases {
		if alias == "" {
			continue
		}
		if cfg, ok := s.Orgs[alias]; ok && cfg.Safety != "" {
			return cfg.Safety, true
		}
	}
	return "", false
}

// PinDefault sets username as the sf-deck default org. Toggles
// Default=true on the named entry and clears the bit on every other
// entry so the invariant "at most one default" holds. Pass
// username="" to clear the pin entirely (revert to lastUsed order).
//
// Returns true when the in-memory state actually changed; callers
// can skip Save() on no-ops.
func (s *Settings) PinDefault(username string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Orgs == nil {
		s.Orgs = map[string]OrgConfig{}
	}
	changed := false
	// Clear every existing pin that isn't the new target.
	for k, cfg := range s.Orgs {
		if k != username && cfg.Default {
			cfg.Default = false
			changed = true
			if cfg.Safety == "" {
				delete(s.Orgs, k)
			} else {
				s.Orgs[k] = cfg
			}
		}
	}
	if username == "" {
		return changed
	}
	cur := s.Orgs[username]
	if !cur.Default {
		cur.Default = true
		s.Orgs[username] = cur
		changed = true
	}
	return changed
}

// DefaultOrgUsername returns the username pinned via PinDefault, or
// "" when none is set. Callers (sf-deck startup, headless
// ResolveOrg("")) use this to pick the boot org.
func (s *Settings) DefaultOrgUsername() string {
	if s == nil {
		return ""
	}
	// Read-lock: SetOrg/PinDefault mutate s.Orgs from IPC goroutines
	// while the TUI resolves the default org. Without this the map read
	// races the write (Go runtime "concurrent map read and map write").
	s.mu.RLock()
	defer s.mu.RUnlock()
	for u, cfg := range s.Orgs {
		if cfg.Default {
			return u
		}
	}
	return ""
}

// OrgKind lets Resolve stay ignorant of the sf package — callers pass
// whichever kind string Kind() produced.
type OrgKind string

const (
	KindProduction OrgKind = "Production"
	KindSandbox    OrgKind = "Sandbox"
	KindScratch    OrgKind = "Scratch"
	KindDevHub     OrgKind = "DevHub"
)

// Resolve returns the effective SafetyLevel for an org. Lookup keys
// are tried in order (username first — it's stable; alias second —
// user-friendly when editing the file by hand). Precedence:
//
//  1. Explicit override in [orgs."<username>"]
//  2. Explicit override in [orgs."<alias>"]
//  3. Explicit default in [defaults.<kind>]
//  4. Hardcoded safe default: production → read_only,
//     sandbox → records, scratch → full, devhub → records
func (s *Settings) Resolve(username string, kind OrgKind, aliases ...string) SafetyLevel {
	// Read-lock the whole resolution: s.Orgs (and s.Defaults) can be
	// mutated by SetOrg/PinDefault on IPC goroutines concurrently with
	// the TUI resolving a gate decision. RLock keeps the map read from
	// racing those writes and lets concurrent resolvers proceed.
	if s != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}
	if s != nil {
		if cfg, ok := s.Orgs[username]; ok && cfg.Safety != "" {
			return ParseSafetyLevel(cfg.Safety)
		}
		for _, a := range aliases {
			if a == "" {
				continue
			}
			if cfg, ok := s.Orgs[a]; ok && cfg.Safety != "" {
				return ParseSafetyLevel(cfg.Safety)
			}
		}
	}
	return s.resolveDefaultLocked(kind)
}

// ResolveAfterClear returns the effective level that would apply if the
// username override were removed. Alias overrides still participate because
// SetOrg(clear=true) only removes the canonical username entry. Callers use
// this to reject a clear that would accidentally raise safety before mutating
// the live settings object.
func (s *Settings) ResolveAfterClear(username string, kind OrgKind, aliases ...string) SafetyLevel {
	if s == nil {
		return hardcodedSafetyDefault(kind)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, alias := range aliases {
		if alias == "" || alias == username {
			continue
		}
		if cfg, ok := s.Orgs[alias]; ok && cfg.Safety != "" {
			return ParseSafetyLevel(cfg.Safety)
		}
	}
	return s.resolveDefaultLocked(kind)
}

// resolveDefaultLocked resolves configured kind defaults then the hardcoded
// safe ladder. Caller must hold s.mu when s is non-nil.
func (s *Settings) resolveDefaultLocked(kind OrgKind) SafetyLevel {
	if s != nil {
		switch kind {
		case KindProduction:
			if s.Defaults.Production != "" {
				return ParseSafetyLevel(s.Defaults.Production)
			}
		case KindSandbox:
			if s.Defaults.Sandbox != "" {
				return ParseSafetyLevel(s.Defaults.Sandbox)
			}
		case KindScratch:
			if s.Defaults.Scratch != "" {
				return ParseSafetyLevel(s.Defaults.Scratch)
			}
		case KindDevHub:
			if s.Defaults.DevHub != "" {
				return ParseSafetyLevel(s.Defaults.DevHub)
			}
		}
	}
	return hardcodedSafetyDefault(kind)
}

func hardcodedSafetyDefault(kind OrgKind) SafetyLevel {
	// Hardcoded fallback defaults (safe-by-default).
	switch kind {
	case KindProduction:
		return SafetyReadOnly
	case KindSandbox, KindDevHub:
		return SafetyRecords
	case KindScratch:
		return SafetyFull
	}
	return SafetyReadOnly
}
