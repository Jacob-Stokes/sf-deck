package qchip

import (
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

// Registry is the per-domain holder of chips. One Registry per surface
// (records / objects / flows) — same type for all three so every surface
// uses identical lookup, scope-filtering, and ordering code.
//
// Splits the chip catalogue into two slices: built-ins (immutable, ship
// in code) and user (loaded from settings.toml). ChipsFor returns
// built-ins first, then user — stable order so the chip strip's cursor
// position holds across renders.
//
// builtinFavOverride is the user's per-chip override of the shipped
// favourite default. Persisted as a tiny extra section so users
// who unfavourite "All" don't end up with All restored every launch.
type Registry struct {
	domain              string // "records" | "objects" | "flows"
	builtins            []Chip
	user                []Chip
	builtinFavOverrides map[string]bool // chip id → favourite override

	// activeOrg gates which user-stored chips are returned by ChipsFor.
	// Set by the UI layer on every org switch via SetActiveOrg. When
	// empty, only globally-shared chips (built-ins + legacy unstamped
	// entries) are returned — a safety floor that avoids accidental
	// cross-org leakage if a caller forgets to set the active org.
	activeOrg string

	// groupMembers resolves "is this org a member of this group?" — used
	// by ChipShareGroup. Injected at SetActiveOrg time so the qchip
	// package stays free of any direct dependency on org-group storage.
	// A nil value means "no group membership info" — group-shared chips
	// will fail closed (not appear), which is the safe default.
	groupMembers func(groupID, username string) bool
}

// NewRegistry returns a Registry preloaded with the given built-ins.
func NewRegistry(domain string, builtins []Chip) *Registry {
	return &Registry{
		domain:              domain,
		builtins:            builtins,
		builtinFavOverrides: map[string]bool{},
	}
}

// effectiveFavourite returns the chip's actual favourite state after
// applying user overrides. User chips don't go through here — they
// store their own state directly in the slice.
func (r *Registry) effectiveFavourite(c Chip) bool {
	if r.builtinFavOverrides != nil {
		if v, ok := r.builtinFavOverrides[c.ID]; ok {
			return v
		}
	}
	return c.Favourite
}

// withEffectiveFavourite returns a copy of c with Favourite set to
// the user's override (if any), or its baseline default. Callers
// render against this so the chip strip reflects what the user
// actually toggled.
func (r *Registry) withEffectiveFavourite(c Chip) Chip {
	c.Favourite = r.effectiveFavourite(c)
	return c
}

// Domain returns the registry's domain string.
func (r *Registry) Domain() string { return r.domain }

// SetActiveOrg gates ChipsFor results to chips matching this org's
// username. Pass "" to expose only globally-shared chips. The UI layer
// calls this on every org switch; pair it with SetGroupMembers so
// group-shared chips can resolve.
func (r *Registry) SetActiveOrg(orgUser string) { r.activeOrg = orgUser }

// SetGroupMembers injects the group-membership resolver used by
// ChipShareGroup. Pass nil to disable group resolution (group-shared
// chips will then fail closed). Callers typically set this once at
// startup against settings.OrgGroupForUsername; the resolver re-runs
// each render, so post-startup edits to org-group config take effect
// immediately without re-registering.
func (r *Registry) SetGroupMembers(fn func(groupID, username string) bool) {
	r.groupMembers = fn
}

// ActiveOrg returns the currently-gating org username, or "" if no
// org filter is active.
func (r *Registry) ActiveOrg() string { return r.activeOrg }

// chipBelongsToActiveOrg reports whether a user-stored chip should be
// returned given the registry's current activeOrg + groupMembers. The
// decision is delegated to the chip's EffectiveShare so the rules live
// in one place (settings.ChipShare.Allows); the registry just supplies
// the active org and the group resolver.
func (r *Registry) chipBelongsToActiveOrg(c Chip) bool {
	share := c.Share
	if share.IsZero() {
		// Runtime chip wasn't migrated (built-in or test fixture). Fall
		// back to the legacy OrgUser rule so behaviour matches what users
		// see today before they next touch settings.
		if c.OrgUser == "" {
			return true
		}
		return c.OrgUser == r.activeOrg
	}
	return share.Allows(r.activeOrg, r.groupMembers)
}

// SetUser replaces the user-defined slice. Callers own settings.Save().
func (r *Registry) SetUser(cs []Chip) { r.user = cs }

// User returns a fresh copy of the user-defined slice.
func (r *Registry) User() []Chip {
	out := make([]Chip, len(r.user))
	copy(out, r.user)
	return out
}

// Builtins returns a fresh copy of the immutable built-in slice.
func (r *Registry) Builtins() []Chip {
	out := make([]Chip, len(r.builtins))
	copy(out, r.builtins)
	return out
}

// ChipsFor returns every chip applicable to the given scope, built-ins
// first then user. Scope "*" / "" on the chip means "any surface";
// otherwise the chip's scope must equal the query scope exactly.
//
// Built-in chips have their Favourite flag adjusted by any user
// override before being returned, so callers see "what the user
// wants on the strip" without needing to consult overrides directly.
func (r *Registry) ChipsFor(scope string) []Chip {
	out := make([]Chip, 0, len(r.builtins)+len(r.user))
	for _, c := range r.builtins {
		if scopeApplies(c.Scope, scope) {
			out = append(out, r.withEffectiveFavourite(c))
		}
	}
	for _, c := range r.user {
		if !r.chipBelongsToActiveOrg(c) {
			continue
		}
		if scopeApplies(c.Scope, scope) {
			out = append(out, c)
		}
	}
	return out
}

// FavouritesFor returns just the chips with Favourite=true that apply
// to the scope. These are the ones rendered on the quick-cycle strip;
// the rest live in the overflow modal.
func (r *Registry) FavouritesFor(scope string) []Chip {
	all := r.ChipsFor(scope)
	out := make([]Chip, 0, len(all))
	for _, c := range all {
		if c.Favourite {
			out = append(out, c)
		}
	}
	return out
}

// OthersFor returns the non-favourite chips for the scope — the
// inverse of FavouritesFor. Used to populate the overflow modal.
func (r *Registry) OthersFor(scope string) []Chip {
	all := r.ChipsFor(scope)
	out := make([]Chip, 0, len(all))
	for _, c := range all {
		if !c.Favourite {
			out = append(out, c)
		}
	}
	return out
}

// SetFavourite toggles the favourite flag on the chip with the given
// id. User chips mutate in place; built-ins use the override map so
// the baseline values in code remain authoritative. LockedFavourite
// chips refuse the toggle silently — the call returns false but
// doesn't error, so callers can treat it the same as "not found".
func (r *Registry) SetFavourite(id string, fav bool) bool {
	for i := range r.user {
		if r.user[i].ID == id {
			if r.user[i].LockedFavourite {
				return false
			}
			r.user[i].Favourite = fav
			return true
		}
	}
	for _, b := range r.builtins {
		if b.ID == id {
			if b.LockedFavourite {
				return false
			}
			if r.builtinFavOverrides == nil {
				r.builtinFavOverrides = map[string]bool{}
			}
			// Only store overrides that differ from the default —
			// avoids growing the map indefinitely with redundant entries.
			if b.Favourite == fav {
				delete(r.builtinFavOverrides, id)
			} else {
				r.builtinFavOverrides[id] = fav
			}
			return true
		}
	}
	return false
}

// BuiltinFavOverrides returns a copy of the user-set built-in
// favourite overrides for serialisation.
func (r *Registry) BuiltinFavOverrides() map[string]bool {
	if r == nil || len(r.builtinFavOverrides) == 0 {
		return nil
	}
	out := make(map[string]bool, len(r.builtinFavOverrides))
	for k, v := range r.builtinFavOverrides {
		out[k] = v
	}
	return out
}

// SetBuiltinFavOverrides replaces the override map. Used by the
// settings load path.
func (r *Registry) SetBuiltinFavOverrides(m map[string]bool) {
	if m == nil {
		r.builtinFavOverrides = map[string]bool{}
		return
	}
	r.builtinFavOverrides = make(map[string]bool, len(m))
	for k, v := range m {
		r.builtinFavOverrides[k] = v
	}
}

// FindByID scans built-ins then user, returning the first match.
// Built-ins have any user favourite override applied so callers see
// the effective state, not the baseline.
func (r *Registry) FindByID(id string) (Chip, bool) {
	for _, c := range r.builtins {
		if c.ID == id {
			return r.withEffectiveFavourite(c), true
		}
	}
	for _, c := range r.user {
		if c.ID == id {
			return c, true
		}
	}
	return Chip{}, false
}

// LoadFromSettings hydrates the user slice for this domain from the
// unified settings.Chips list. Drops anything tagged for other domains
// so each registry only carries what it needs. Also re-applies the
// per-domain built-in favourite overrides.
func (r *Registry) LoadFromSettings(s *settings.Settings) {
	if s == nil {
		return
	}
	user := []Chip{}
	for _, cfg := range s.ChipsForDomain(r.domain) {
		user = append(user, FromConfig(cfg))
	}
	r.user = user
	r.SetBuiltinFavOverrides(s.ChipFavouriteOverridesFor(r.domain))
}

// PersistUser converts the registry's current user chips back into
// ChipConfig and rewrites the settings list. Also writes the
// per-domain built-in favourite override map. Caller owns Save().
func (r *Registry) PersistUser(s *settings.Settings) {
	if s == nil {
		return
	}
	// Replace only this domain's slice; leave other domains alone.
	keep := []settings.ChipConfig{}
	for _, c := range s.Chips() {
		if c.Domain != r.domain {
			keep = append(keep, c)
		}
	}
	for _, c := range r.user {
		keep = append(keep, ToConfig(c, r.domain))
	}
	s.SetChips(keep)
	s.SetChipFavouriteOverridesFor(r.domain, r.BuiltinFavOverrides())
}

// scopeApplies reports whether a chip with chipScope applies to a given
// query scope. Empty / "*" chip scope = universal.
func scopeApplies(chipScope, queryScope string) bool {
	if chipScope == "" || chipScope == "*" {
		return true
	}
	return chipScope == queryScope
}
