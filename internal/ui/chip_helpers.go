package ui

// Helpers shared by every surface that renders a chip strip from a
// qchip.Registry. Centralised so each surface stays a one-liner
// (chips := chipRowsFromQChips(reg.ChipsFor("*"))).

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/orgproject"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

// chipOverflowID is the sentinel ID used for the "+ N more…" row that
// terminates the chip strip when non-favourite chips exist. Picking
// it (enter / activation) opens the overflow modal where the full
// chip list is browsable. The ID is intentionally garbage so it
// doesn't collide with any real chip ID, and the chip-resolution
// code paths can detect it cheaply.
const chipOverflowID = "__chip_overflow__"

// projectChipID is the sentinel ID for the synthetic loaded-org-
// project chip. Prepended to the strip when m.activeScope().Loaded()
// AND the loaded project has items relevant to the current domain.
// Selection routes through applySelectedChipMatcher's project-chip
// branch, which sets a Go closure on the ListView's Extra slot that
// consults the Scope's HasObject / HasFlow / etc.
const projectChipID = "__project__"

// chipEphemeralIDPrefix marks chips minted by IPC clients via the
// chip.preview verb. Format: "__eph_<ulid>__" so the existing
// sentinel-detection conventions in this package treat them like
// any other synthetic chip (filtered out of registry-pin walks,
// excluded from chip-manager rename/delete, etc.).
const chipEphemeralIDPrefix = "__eph_"

// chipPreviewOriginIPC is the sentinel OriginOrgUser value an IPC-
// spawned ephemeral carries on its chipPreview row. The renderer
// keys off this string to swap "(from <org>)" for "(session)" and
// to prefix the label with the ephemeral glyph.
const chipPreviewOriginIPC = "ipc"

// chipEphemeralGlyph is the visual marker prefixed to ephemeral
// chip labels on the strip. Tilde matches the editor/backup-file
// convention for transient artefacts and stays single-cell-wide so
// the existing column math doesn't shift.
const chipEphemeralGlyph = "~"

// newEphemeralChipID mints a fresh "__eph_<hex>__" id. Uses the
// same ~96-bit random body the dev-project newID uses — plenty of
// entropy to keep two concurrent agents from colliding within a
// session. Reusing the same helper keeps the test-time RNG seam
// consistent across the package.
func newEphemeralChipID() string {
	return chipEphemeralIDPrefix + newID() + "__"
}

// recentlyViewedChipID is the sentinel ID for the synthetic
// "Recently viewed" chip prepended to surfaces that participate
// in the merged-recent pipeline. Filters rows to items the user
// has touched recently — sf-deck local log unioned with
// Salesforce's RecentlyViewed.
//
// ID is left as the legacy `__visited__` so user TOML configs
// that pinned chip preferences (if any) keep working; only the
// label is "Recently viewed".
const recentlyViewedChipID = "__visited__"

// projectChipActive reports whether the synthetic project chip is the
// currently-cursored chip on the active chip-shaped surface. Used by
// the empty-state copy to switch from "no matches" to a more useful
// "no X collected — press ctrl+k to add" hint.
func (m Model) projectChipActive() bool {
	switch m.tab() {
	case TabObjects, TabRecords:
		strip := m.stripRows(domainObjects, "*")
		idx := m.objectsChipIdx()
		if idx >= 0 && idx < len(strip) {
			return strip[idx].ID == projectChipID
		}
	case TabObjectDetail:
		if m.currentSubtab() != SubtabRecords {
			return false
		}
		_, sobj := m.activeRecordsSObject()
		if sobj == "" {
			return false
		}
		return selectedRecordsChip(m.data[m.orgs[m.selected].Username], sobj) == projectChipID
	case TabFlows:
		strip := m.stripRows(domainFlows, "*")
		idx := m.flowsChipIdx()
		if idx >= 0 && idx < len(strip) {
			return strip[idx].ID == projectChipID
		}
	}
	return false
}

// projectEmptyHint returns the empty-state help text shown when the
// project chip is active but the project contains no items of the
// kind the surface filters by. Includes the collect-keybind so the
// user knows how to populate the working set without leaving the
// surface.
func (m Model) projectEmptyHint(label string) string {
	return "  no " + label + " in this project · press " +
		firstPretty(Keys.CollectItem) + " to collect"
}

// domainFromRegistry maps a *qchip.Registry pointer back to its
// domain constant. cycleSimpleChipStrip needs this so it can call
// m.stripRows(domain, "*") instead of stripRowsFor(reg, "*") — the
// former picks up the synthetic project chip prepended in stripRows.
//
// Reverse-lookup walks every chipSurface declared in the TabSpec
// registry; pointer-compare against each surface's Registry getter.
// O(N) over surfaces but N is tiny and this fires on chip-cycle
// only.
func domainFromRegistry(m Model, reg *qchip.Registry) chipDomain {
	for _, s := range allChipSurfaces() {
		if s.Registry(&m) == reg {
			return s.Domain
		}
	}
	return ""
}

// recentlyViewedChipApplies reports whether the synthetic Recently
// viewed chip should appear on the strip for the given (domain,
// scope). True when there's at least one entry in the merged-recent
// stream that the surface's chip predicate would match.
//
// /records (domain=records, scope=<sObject>): visited records for
//
//	that sObject.
//
// /objects (domain=objects, scope=*): any visited sObject — either
//
//	a Kind=sobject entry or a record visit whose Type identifies
//	an sObject we're listing.
//
// Other domains return false — Recently viewed only makes sense on
// surfaces that explicitly opt in via this predicate. New domains
// add a case here when they wire a Recently-viewed filter.
//
// The chip is prepended on every supported surface regardless of
// whether the visit log has entries — matches Lightning behaviour
// ("Recently Viewed" page is always there, just empty until you
// open something). recentlyViewedChipHasEntries below answers the
// "is there anything to show?" question separately, used by the
// default-cursor decision.
func recentlyViewedChipApplies(m Model, domain chipDomain, scope string) bool {
	switch domain {
	case domainRecords:
		// scope is the sObject API name on the per-sObject records
		// strip. Empty scope ("*") happens before drill — no Visited
		// there because there's no anchor sObject to filter against.
		if scope == "" || scope == "*" {
			return false
		}
		return true
	case domainObjects,
		domainFlows,
		domainApex, domainTriggers,
		domainLWC,
		domainAura,
		domainPermSets,
		domainPSGs,
		domainProfiles,
		domainQueues,
		domainPublicGroup:
		return true
	}
	return false
}

// projectChipApplies reports whether the synthetic project chip
// should appear in the strip for the given (domain, scope). The
// rule is uniform across surfaces: only show the chip when the
// loaded project has at least one item of this kind that applies
// to the surface. /objects looks at the global Objects set,
// /flows at FlowIDs, /records at the per-sObject records slice,
// and so on. Hiding the chip when there's nothing in scope is
// the consistent UX — users shouldn't have to click into an
// empty chip to find out it's empty.
func projectChipApplies(domain chipDomain, scope string, s *orgproject.Scope) bool {
	if !s.Loaded() {
		return false
	}
	// Records: per-sObject. The chip applies when the project has
	// at least one record bound to THIS sObject. Different sObjects
	// independently show / hide the chip — drilling into /Account
	// with a project that's only Account records will show the
	// chip; drilling into /Contact with the same project will not.
	if domain == domainRecords {
		return len(s.RecordIDsFor(scope)) > 0
	}
	surf := chipSurfaceForDomain(domain)
	if surf == nil {
		return false
	}
	if surf.ScopeCount == nil {
		// Surface doesn't expose project-chip semantics — never
		// prepend the chip. (Apex / LWC / Aura today: the project
		// chip doesn't make sense there yet.)
		return false
	}
	return surf.ScopeCount(s) > 0
}

// transientSlotKey builds the activeTransient map key — one slot per
// (domain, scope) so per-sObject records surfaces don't share a
// transient with /objects.
func transientSlotKey(domain, scope string) string {
	return domain + "|" + scope
}

// chipRowKindTransient is set on the chipRow.Count field as a sentinel
// so the dashboard renderer can style transient chips differently from
// favourites. Count's actual numeric meaning is "row count" — using -2
// here is a tag, not a number; the renderer detects it before any
// numeric formatting.
const chipRowKindTransient = -2

// chipRowKindPreview is the analogous sentinel for ephemeral cross-org
// previews. Renderer draws these with a dotted border + "(from <origin
// org>)" tag so the user can't mistake them for chips that belong to
// the current org's permanent set.
const chipRowKindPreview = -3

// chipRowsFromQChips renders a qchip.Chip slice as the chipRow slice
// the dashboard renderer consumes. Origin glyph + shared glyph prepend
// to the label so user-defined and cross-org-shared chips are visually
// distinct from built-ins and from chips private to the current org.
//
// This is the unfiltered version — it shows every chip the slice
// contains. Use stripRowsFor instead when rendering the on-screen
// strip; that one filters to favourites + the overflow sentinel.
func chipRowsFromQChips(cs []qchip.Chip) []chipRow {
	out := make([]chipRow, 0, len(cs))
	for _, c := range cs {
		out = append(out, chipRow{
			ID:    c.ID,
			Label: chipDisplayLabel(c),
			Count: -1,
		})
	}
	return out
}

// chipDisplayLabel composes a chip's strip/manager label from its origin
// glyph + cross-org-shared glyph + label. Centralised so every render
// surface stays visually consistent — adding a new badge means one
// edit here, not chasing every renderer.
func chipDisplayLabel(c qchip.Chip) string {
	prefix := c.Origin.Glyph()
	if c.Share.IsShared() {
		prefix += qchip.SharedGlyph
	}
	return prefix + c.Label
}

// stripRows returns the on-screen strip rows for a domain + scope.
// Layout:
//
//	favourites … [transient] [+ N more (M)]
//
// where favourites come from the registry, transient is the chip
// the user activated from the overflow modal (one per surface), and
// the sentinel only shows when non-favourite chips exist. Cycle
// handlers iterate through favourites + transient (transient is
// cycle-able like any favourite); the sentinel is skipped.
//
// stripRowsFor (the legacy function form) is preserved as a
// thin shim because some callers still pass a registry directly.
// Prefer m.stripRows for new call sites — the transient slot is
// only visible to the Model.
func (m Model) stripRows(domain chipDomain, scope string) []chipRow {
	reg := m.registryFor(domain)
	if reg == nil {
		return nil
	}
	favs := reg.FavouritesFor(scope)
	others := reg.OthersFor(scope)
	// Fallback: if nothing's favourited (e.g. a fresh user-only
	// catalogue) show every chip — the strip should never be blank.
	if len(favs) == 0 {
		favs = others
		others = nil
	}
	rows := chipRowsFromQChips(favs)

	// Synthetic chip prepends, in REVERSE display order — each
	// prepend lands at index 0, so the LAST prepend wins position 0.
	// Project belongs first (it's the user's pinned working set);
	// Recently viewed belongs second.

	// Recently viewed chip: prepended when the active surface has
	// anything in the merged-recent stream that matches the
	// surface's filter. /records (domain=records, scope=<sobject>)
	// shows it when the user has visited records for this sObject;
	// /objects (domain=objects, scope=*) shows it when the user has
	// visited any sObject (or any record whose Type identifies one).
	if recentlyViewedChipApplies(m, domain, scope) {
		rows = append([]chipRow{{
			ID:    recentlyViewedChipID,
			Label: "Recently viewed",
			Count: -1,
		}}, rows...)
	}

	// Loaded org-project chip: synthetic, prepended LAST so it
	// lands at position 0. Cyclable like any other chip; selecting
	// it filters the surface to project items via a Go closure
	// (see applySelectedChipMatcher's project-chip branch).
	if scopePill := m.activeScope(); scopePill.Loaded() && projectChipApplies(domain, scope, scopePill) {
		rows = append([]chipRow{{
			ID:    projectChipID,
			Label: "📁 " + scopePill.ProjectName,
			Count: -1,
		}}, rows...)
	}

	// IPC-spawned ephemerals belong at the START of the strip so
	// they get prime visual real estate — they're the user's
	// active working filter, not background decoration. Prepended
	// AFTER Project/Recent so those synthetic chips keep their
	// pinned positions 0/1 and ephemerals land just to their
	// right.
	//
	// Cross-org "Preview here" previews stay in their original
	// position (after favourites + transient, before overflow) —
	// they're cosmetically transient too but semantically
	// background — a peek at another org's chip, not an active
	// session-driver.
	var ipcRows []chipRow
	for _, p := range m.chipPreviewsFor(domain, scope) {
		if p.OriginOrgUser != chipPreviewOriginIPC {
			continue
		}
		if chipIDIn(favs, p.Chip.ID) {
			continue
		}
		ipcRows = append(ipcRows, chipRow{
			ID:    p.Chip.ID,
			Label: chipEphemeralGlyph + " " + p.Chip.Label,
			Count: chipRowKindPreview,
		})
	}
	if len(ipcRows) > 0 {
		rows = append(ipcRows, rows...)
	}

	// Transient slot: one chip the user picked from the overflow
	// modal but hasn't pinned yet. Distinct styling so users see
	// it's not part of their permanent set. Filtered out of the
	// "others" list so it doesn't double-show.
	if id := m.transientID(domain, scope); id != "" {
		// Don't add the transient if it's already a favourite
		// (e.g. user pinned it; transient slot should be cleared
		// but defensive lookup here too).
		if !chipIDIn(favs, id) {
			if c, ok := reg.FindByID(id); ok {
				rows = append(rows, chipRow{
					ID:    c.ID,
					Label: chipDisplayLabel(c),
					Count: chipRowKindTransient,
				})
			}
		}
		// Filter the transient out of "others" so the sentinel
		// count doesn't count it.
		others = filterOutChipID(others, id)
	}

	// Cross-org chip previews: rendered after favourites/transient but
	// BEFORE the overflow sentinel so they're visible inline. Each
	// carries the originating org's friendly name in the label so the
	// user is always reminded these aren't permanent. IPC-origin
	// previews were already handled above (start-of-strip position);
	// this loop is cross-org only.
	for _, p := range m.chipPreviewsFor(domain, scope) {
		if p.OriginOrgUser == chipPreviewOriginIPC {
			continue
		}
		// Filter out chips the user has already pinned for this org —
		// otherwise running "Preview here" on something that's now
		// permanent would double-render.
		if chipIDIn(favs, p.Chip.ID) {
			continue
		}
		rows = append(rows, chipRow{
			ID:    p.Chip.ID,
			Label: p.Chip.Origin.Glyph() + p.Chip.Label + "  (from " + chipShareFriendlyOrg(m, p.OriginOrgUser) + ")",
			Count: chipRowKindPreview,
		})
	}

	if len(others) > 0 {
		// Sentinel renders the keybind so the affordance is
		// discoverable without consulting docs. Cycle handlers skip
		// it (withoutOverflow) so arrows never land on it.
		rows = append(rows, chipRow{
			ID:    chipOverflowID,
			Label: fmt.Sprintf("+ %d more (M)", len(others)),
			Count: -1,
		})
	}
	return rows
}

// pushChipPreview is the variant of addChipPreview used by IPC-
// spawned ephemerals: it accepts a full chipPreview so the caller
// can populate Columns / Limit / Clauses (which the cross-org
// addChipPreview path doesn't need). Same dedupe-by-id semantics
// as addChipPreview.
func (m *Model) pushChipPreview(p chipPreview) {
	if m.chipPreviews == nil {
		m.chipPreviews = map[string][]chipPreview{}
	}
	key := transientSlotKey(string(p.Domain), p.Scope)
	for _, existing := range m.chipPreviews[key] {
		if existing.Chip.ID == p.Chip.ID {
			return
		}
	}
	m.chipPreviews[key] = append(m.chipPreviews[key], p)
}

// transientID returns the chip id currently occupying the transient
// slot for (domain, scope), or "" when none is active.
func (m Model) transientID(domain chipDomain, scope string) string {
	if m.activeTransient == nil {
		return ""
	}
	return m.activeTransient[transientSlotKey(string(domain), scope)]
}

// chipPreviewsFor returns the ephemeral cross-org chip previews registered
// for (domain, scope), or nil. Reads-only — safe to call from render.
func (m Model) chipPreviewsFor(domain chipDomain, scope string) []chipPreview {
	if m.chipPreviews == nil {
		return nil
	}
	return m.chipPreviews[transientSlotKey(string(domain), scope)]
}

// addChipPreview adds a session-only preview of a chip from another org
// at the given (domain, scope). Duplicate previews (same chip ID at the
// same slot) are no-ops — calling "Preview here" twice doesn't multiply
// the row. Mutates Model state in place.
func (m *Model) addChipPreview(domain chipDomain, scope string, c qchip.Chip, originOrgUser string) {
	if m.chipPreviews == nil {
		m.chipPreviews = map[string][]chipPreview{}
	}
	key := transientSlotKey(string(domain), scope)
	for _, p := range m.chipPreviews[key] {
		if p.Chip.ID == c.ID {
			return // already previewing this exact chip here
		}
	}
	m.chipPreviews[key] = append(m.chipPreviews[key], chipPreview{
		Domain: domain, Scope: scope, Chip: c, OriginOrgUser: originOrgUser,
	})
}

// removeChipPreview drops a single preview from a slot. Called when the
// user permanently adds the chip to scope (preview → real) so the strip
// doesn't double-render.
func (m *Model) removeChipPreview(domain chipDomain, scope, chipID string) {
	if m.chipPreviews == nil {
		return
	}
	key := transientSlotKey(string(domain), scope)
	in := m.chipPreviews[key]
	out := in[:0]
	for _, p := range in {
		if p.Chip.ID != chipID {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		delete(m.chipPreviews, key)
	} else {
		m.chipPreviews[key] = out
	}
}

// stripRowsFor is the registry-only legacy entry point. Keeps the
// signature so older call sites compile; prefer m.stripRows.
//
// Returns the same shape minus any transient slot — those need the
// Model.
func stripRowsFor(reg *qchip.Registry, scope string) []chipRow {
	if reg == nil {
		return nil
	}
	favs := reg.FavouritesFor(scope)
	others := reg.OthersFor(scope)
	if len(favs) == 0 {
		favs = others
		others = nil
	}
	rows := chipRowsFromQChips(favs)
	if len(others) > 0 {
		rows = append(rows, chipRow{
			ID:    chipOverflowID,
			Label: fmt.Sprintf("+ %d more (M)", len(others)),
			Count: -1,
		})
	}
	return rows
}

// chipIDIn / filterOutChipID are tiny slice helpers used by stripRows.
func chipIDIn(cs []qchip.Chip, id string) bool {
	for _, c := range cs {
		if c.ID == id {
			return true
		}
	}
	return false
}

func filterOutChipID(cs []qchip.Chip, id string) []qchip.Chip {
	if id == "" {
		return cs
	}
	out := make([]qchip.Chip, 0, len(cs))
	for _, c := range cs {
		if c.ID != id {
			out = append(out, c)
		}
	}
	return out
}

// chipMatcherFor builds the row-level predicate the chip strip applies
// to a cached list. Generic over any row type that satisfies the
// query.Row interface — sf.SObject, sf.Flow, sf.Record all work
// because they each implement Field(name) (any, bool).
//
// Returns nil when the chip has no WHERE clause — a nil predicate is
// the convention ListView.Extra reads as "match every row".
//
// Substitutions ($userId, $userName) are applied before evaluation so
// built-in chips like "Mine" can reference the current user without
// baking a literal into settings.toml. The substitution machinery is
// shared with the server-side ApplyToSOQL path (see qchip/apply.go).
func chipMatcherFor[T query.Row](c qchip.Chip, subs qchip.Substitutions) func(T) bool {
	if c.Query.Where == nil {
		return nil
	}
	// Substitute UP-FRONT, not inside the predicate closure. The chip
	// strip is hit per-row on a hot path; doing the AST walk once at
	// install time and capturing the substituted predicate keeps
	// per-row eval allocation-free.
	pred := qchip.SubstituteWhere(c.Query.Where, subs)
	return func(t T) bool { return query.Eval(pred, t) }
}

// applySelectedChipMatcher writes the active chip's predicate into
// the appropriate ListView.Extra slot, so the cached-list filter
// reflects the current chip selection. Called whenever the chip
// selection might have changed: chip cycle, tab change, settings
// reload after wizard save.
//
// Read-only on the model (we only mutate orgData fields). Idempotent:
// safe to call as often as we like; cheap to skip when nothing has
// changed but cheaper to just overwrite.
func (m Model) applySelectedChipMatcher(d *orgData) {
	if d == nil {
		return
	}
	// Records is the one bespoke surface — its chip predicate is
	// per-(sobject, chip) rather than per-tab, so it stays on its
	// own legacy code path. /objects shares the same domain (you
	// drill from /objects into /records) so we route TabRecords
	// through the Objects surface for the un-drilled case.
	if m.tab() == TabRecords {
		applyChipFromSurface(m, d, objectsChipSurface)
		return
	}
	surf := m.resolveChipSurface()
	if surf == nil {
		return
	}
	applyChipFromSurface(m, d, *surf)
}

// applyChipFromSurface is the shared body of "look up the cursored
// chip on the surface's strip and write its predicate onto the
// surface's ListView". Project chip short-circuits to the surface's
// project predicate; everything else routes through the chip's
// registry entry.
func applyChipFromSurface(m Model, d *orgData, surf chipSurface) {
	reg := surf.Registry(&m)
	if reg == nil {
		return
	}
	strip := m.stripRows(surf.Domain, "*")
	idx := surf.ChipIdx(m)
	if idx < 0 || idx >= len(strip) {
		return
	}
	row := strip[idx]
	if row.ID == projectChipID {
		if surf.ClearVisitedOrder != nil {
			surf.ClearVisitedOrder(d)
		}
		if surf.ApplyProjectChip != nil {
			surf.ApplyProjectChip(d, m.activeScope())
		}
		return
	}
	if row.ID == recentlyViewedChipID {
		if surf.ApplyVisitedChip != nil {
			surf.ApplyVisitedChip(m, d)
		}
		return
	}
	if surf.ClearVisitedOrder != nil {
		surf.ClearVisitedOrder(d)
	}
	if c, ok := reg.FindByID(row.ID); ok {
		surf.ApplyChip(d, c)
		return
	}
	// Ephemeral (session-only) and cross-org preview chips live in
	// m.chipPreviews rather than the registry, so reg.FindByID misses
	// them. Without this fallback, navigating away and back to the
	// strip would silently drop the predicate and re-render with
	// whatever ListView.Extra was set to last. Look the chip up by
	// id across every slot — chipPreviews is small (one entry per
	// active ephemeral / cross-org preview) so the linear walk is
	// fine.
	for _, slot := range m.chipPreviews {
		for _, p := range slot {
			if p.Chip.ID == row.ID {
				surf.ApplyChip(d, p.Chip)
				return
			}
		}
	}
}

// withoutOverflow filters the overflow sentinel out of a strip slice.
// Used by cycle handlers — arrows should only step through real
// chips, never onto the "+ N more…" placeholder.
func withoutOverflow(rows []chipRow) []chipRow {
	out := make([]chipRow, 0, len(rows))
	for _, r := range rows {
		if r.ID == chipOverflowID {
			continue
		}
		out = append(out, r)
	}
	return out
}

// toggleActiveChipFavourite flips the favourite flag on the chip
// currently active on the strip for whatever tab the user is on.
// Promotes a transient chip to a favourite (clears the transient
// slot) or demotes a favourite back to the overflow modal.
// Locked-favourite chips (Recent / All) silently refuse via
// SetFavourite returning false.
func (m *Model) toggleActiveChipFavourite() tea.Cmd {
	domain, scope, ok := m.activeChipContext()
	if !ok {
		m.flash("no chip strip on this tab — nothing to favourite")
		return nil
	}
	reg := m.registryFor(domain)
	if reg == nil {
		return nil
	}
	chipID := m.activeChipID(domain, scope)
	if chipID == "" || chipID == chipOverflowID {
		m.flash("no view selected")
		return nil
	}
	c, ok := reg.FindByID(chipID)
	if !ok {
		return nil
	}
	if c.LockedFavourite {
		m.flash(c.Label + " can't be unfavourited")
		return nil
	}
	newFav := !c.Favourite
	if !reg.SetFavourite(chipID, newFav) {
		return nil
	}
	if newFav {
		// Promote: clear the transient slot if this chip was occupying it.
		key := transientSlotKey(string(domain), scope)
		if m.activeTransient[key] == chipID {
			delete(m.activeTransient, key)
		}
		m.flash("★ pinned " + c.Label)
	} else {
		m.flash("☆ unpinned " + c.Label)
	}
	if m.settings != nil {
		reg.PersistUser(m.settings)
		m.saveSettings("")
	}
	return m.onTabChanged()
}

// editActiveChip opens the wizard for whichever chip is selected on
// the active surface. Returns (newModel, true) when handled (the
// caller should stop further key dispatch); (model, false) when no
// view surface is focused so other 'e' bindings (SOQL edit, report
// export) can still fire.
//
// Built-in chips can't be edited — they're code-defined. Salesforce
// list views aren't editable here either; their definitions live in
// Salesforce, not on disk.
func (m Model) editActiveChip() (tea.Cmd, bool) {
	domain, scope, ok := m.activeChipContext()
	if !ok {
		return nil, false
	}
	// Salesforce-mode list views aren't sf-deck chips — skip.
	if d, sobj := m.activeRecordsSObject(); sobj != "" && currentChipMode(d, sobj) == ChipModeSalesforce {
		m.flash("Salesforce list views are edited in Salesforce, not sf-deck")
		return nil, true
	}
	reg := m.registryFor(domain)
	if reg == nil {
		return nil, true
	}
	chipID := m.activeChipID(domain, scope)
	if chipID == "" || chipID == chipOverflowID {
		m.flash("no view selected")
		return nil, true
	}
	c, ok := reg.FindByID(chipID)
	if !ok {
		return nil, true
	}
	if c.Origin == qchip.OriginBuiltIn {
		m.flash(c.Label + " is built-in — copy to a new view to edit")
		return nil, true
	}
	return m.openChipWizard(domain, c), true
}

// activeChipContext returns the (domain, scope) the F keybind should
// operate on, given the active tab. ok=false when no chip-shaped
// surface is focused.
//
// Routes through resolveChipSurface so every chipped tab/subtab
// registered in the TabSpec registry is covered automatically —
// previously this function had a switch that drifted out of sync as
// new chipped surfaces (apex, components, perms, recent, …) landed.
func (m Model) activeChipContext() (chipDomain, string, bool) {
	// Records-on-detail is the one bespoke case left: the records
	// subtab inside TabObjectDetail uses the records domain with
	// the drilled-in sObject as scope. Everything else falls through
	// to the generic surface resolver.
	if m.tab() == TabObjectDetail && m.currentSubtab() == SubtabRecords {
		_, sobj := m.activeRecordsSObject()
		if sobj != "" {
			return domainRecords, sobj, true
		}
	}
	if surf := m.resolveChipSurface(); surf != nil {
		return surf.Domain, "*", true
	}
	return "", "", false
}

// activeChipID returns the chip id currently selected on the strip
// for the given (domain, scope). Generic over every domain by
// reading the strip + the surface's ChipIdx hook.
func (m Model) activeChipID(domain chipDomain, scope string) string {
	// Records-on-detail still has its own per-sObject cursor that
	// doesn't fit the chipSurface registry's single-cursor model.
	if domain == domainRecords {
		_, sobj := m.activeRecordsSObject()
		if sobj == "" {
			return ""
		}
		d := m.data[m.orgs[m.selected].Username]
		if d == nil {
			return ""
		}
		if id, ok := d.ListViewCur[sobj]; ok {
			return id
		}
		// Default on first visit matches selectedRecordsChip — see
		// tab_records_dashboard.go.  Lands on Recently viewed.
		return recentlyViewedChipID
	}
	strip := m.stripRows(domain, scope)
	if surf := m.resolveChipSurface(); surf != nil && surf.Domain == domain {
		idx := surf.ChipIdx(m)
		if idx >= 0 && idx < len(strip) {
			return strip[idx].ID
		}
	}
	return ""
}

// activeChipScope returns the scope string the resolved chip
// surface is operating under right now.  "*" for surfaces with a
// universal scope (objects, flows, apex, lwc, etc.); the active
// sObject API name for records-shaped surfaces.  Returns ""
// alongside a nil domain when no chip surface is active.
func (m Model) activeChipScope() (chipDomain, string) {
	surf := m.resolveChipSurface()
	if surf == nil {
		// Records surfaces don't go through resolveChipSurface —
		// their scope is per-sobject and tied to the records subtab.
		if _, sobj := m.activeRecordsSObject(); sobj != "" {
			return domainRecords, sobj
		}
		return "", ""
	}
	return surf.Domain, "*"
}

// activeChipIDForRender resolves the current chip ID for whatever
// chip surface is active on the user's tab.  Returns "" when no
// chip surface is active.  Used by the render path to rewrite the
// empty-state message when the user lands on Recently viewed with
// nothing to show.
func (m Model) activeChipIDForRender() string {
	domain, scope := m.activeChipScope()
	if domain == "" {
		return ""
	}
	return m.activeChipID(domain, scope)
}

// recentlyViewedEmptyHintFor returns the empty-state copy shown
// when the Recently viewed chip is active and the visit log is
// empty.  Tailored per domain so the recovery hint mentions the
// right "broader" chip to cycle to.
func recentlyViewedEmptyHintFor(domain chipDomain) string {
	broader := "All"
	if domain == domainRecords {
		broader = "Changed"
	}
	noun := chipDomainNoun(domain)
	return "  no recently-viewed " + noun + " — press → for " + broader
}

// chipDomainNoun returns the user-facing label for a domain — used
// in empty-state hints ("no recently-viewed sObjects").
func chipDomainNoun(domain chipDomain) string {
	switch domain {
	case domainObjects:
		return "sObjects"
	case domainFlows:
		return "flows"
	case domainApex:
		return "apex classes"
	case domainTriggers:
		return "triggers"
	case domainLWC:
		return "LWC bundles"
	case domainAura:
		return "Aura bundles"
	case domainPermSets:
		return "permission sets"
	case domainPSGs:
		return "permission set groups"
	case domainProfiles:
		return "profiles"
	case domainQueues:
		return "queues"
	case domainPublicGroup:
		return "public groups"
	case domainRecords:
		return "records"
	}
	return "items"
}
