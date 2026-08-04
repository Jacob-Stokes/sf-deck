package ui

// TabBundleDetail TabSpec hooks. Separated from
// tab_bundle_detail.go (which carries the renderer) so the spec
// glue + cursor/search/activate behaviours have a single home.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// moveBundleComponentCursor walks the row list on /bundle. Routes
// through whichever ListView the current view-mode owns so cursor
// + view stay in sync regardless of mode.
func (m *Model) moveBundleComponentCursor(delta int) {
	switch m.bundleDetailView {
	case bundleViewFiles:
		m.bundleFilesList.MoveBy(delta)
	default:
		m.bundleDetailList.MoveBy(delta)
	}
}

// resetBundleComponentCursor moves the cursor back to row 0 in
// the active view. Called after a search-filter change.
func (m *Model) resetBundleComponentCursor() {
	switch m.bundleDetailView {
	case bundleViewFiles:
		m.bundleFilesList.ResetCursor()
	default:
		m.bundleDetailList.ResetCursor()
	}
}

// bundleDetailSearchPtr exposes the active view's filter state so
// the / sticky-filter buffer + yellow filtered-pane border work
// the same as on every other list surface — and the right buffer
// reads when the user switches view.
func (m Model) bundleDetailSearchPtr() *searchState {
	switch m.bundleDetailView {
	case bundleViewFiles:
		return m.bundleFilesList.SearchPtr()
	default:
		return m.bundleDetailList.SearchPtr()
	}
}

// cycleBundleDetailView toggles between components ↔ files. delta
// is +1 / -1 (matches the chip-cycle API) but with only two
// values the direction doesn't matter today. Kept symmetric so
// future modes don't need a signature change.
//
// Switching mode lazy-loads the destination view's data if it
// hasn't been populated yet — files reads the directory; the
// components view stays whatever applyBundlePreviewLoaded last
// set. Bundle changes reset the cwd to root.
//
// Value receiver matches the cycleChip / cycleDevProjectKindChip
// shape so the call site in update_keys.go can be uniform.
func (m Model) cycleBundleDetailView(delta int) (Model, tea.Cmd) {
	_ = delta
	if m.bundleDetailView == bundleViewComponents {
		m.bundleDetailView = bundleViewFiles
		m.ensureBundleFilesLoaded()
	} else {
		m.bundleDetailView = bundleViewComponents
	}
	return m, nil
}

// activateBundleFile handles Enter on a row in the FILES view.
// - .. row → pop one segment from cwd (back up the tree)
// - dir row → push the dir onto cwd (enter it)
// - file row → no-op (use `o` to open in the default app)
//
// In both navigation cases we reload the directory listing and
// reset the cursor so the user lands on row 0 of the new view.
func (m *Model) activateBundleFile() tea.Cmd {
	row, ok := m.bundleFilesList.Selected()
	if !ok {
		return nil
	}
	switch {
	case row.IsParent:
		m.bundleFilesCwd = popPathSegment(m.bundleFilesCwd)
		m.bundleFilesLoadedFor = ""
		m.ensureBundleFilesLoaded()
	case row.IsDir:
		if m.bundleFilesCwd == "" {
			m.bundleFilesCwd = row.Name
		} else {
			m.bundleFilesCwd = m.bundleFilesCwd + "/" + row.Name
		}
		m.bundleFilesLoadedFor = ""
		m.ensureBundleFilesLoaded()
	default:
		// File row — Enter is a no-op. Flash a tiny hint so the
		// user knows about `o`.
		m.flash("press " + firstPretty(Keys.BundleOpen) + " to open the file in the default app")
	}
	return nil
}

// popPathSegment trims the final `/`-separated segment from p.
// Returns "" when p is empty or has no separator. Pure string
// manipulation — doesn't touch the file system.
func popPathSegment(p string) string {
	if p == "" {
		return ""
	}
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}

// ensureBundleFilesLoaded reads the bundle's cwd from disk into
// bundleFilesList when the (bundle, cwd) combo doesn't match the
// last load. Cheap to call per-frame: the key check is a string
// compare, ReadDir only runs on a real change.
func (m *Model) ensureBundleFilesLoaded() {
	b, err := m.activeBundle()
	if err != nil {
		m.bundleFilesList.Set(nil)
		return
	}
	key := b.ID + "|" + m.bundleFilesCwd
	if key == m.bundleFilesLoadedFor {
		return
	}
	rows, err := readBundleDir(b.Path, m.bundleFilesCwd)
	if err != nil {
		m.flash("bundle files: " + err.Error())
		m.bundleFilesList.Set(nil)
		return
	}
	m.bundleFilesList.Set(rows)
	m.bundleFilesList.ResetCursor()
	m.bundleFilesLoadedFor = key
}

// sidebarBundleDetail renders the per-row info pane shown on the
// right-hand sidebar. When the cursor is on a row, surfaces full
// path / kind / member / action / namespace. Falls back to a
// bundle-level summary when no row is selected (cold load).
func (m Model) sidebarBundleDetail(inner int) string {
	var lines []string
	if b, err := m.activeBundle(); err == nil {
		lines = append(lines, sectionTitle("Bundle"))
		lines = append(lines, sidebarKV("path", b.Path, inner))
		if b.DefaultOrgAlias != "" {
			lines = append(lines, sidebarKV("default org", b.DefaultOrgAlias, inner))
		}
		lines = append(lines, sidebarKV("created", b.CreatedAt.Format("2006-01-02 15:04"), inner))
		if !b.LastRetrievedAt.IsZero() {
			lines = append(lines, sidebarKV("last retrieved", b.LastRetrievedAt.Format("2006-01-02 15:04"), inner))
		}
		if !b.LastDeployedAt.IsZero() {
			lines = append(lines, sidebarKV("last deployed", b.LastDeployedAt.Format("2006-01-02 15:04"), inner))
		}
	}
	if row, ok := m.bundleDetailList.Selected(); ok {
		lines = append(lines, "", sectionTitle("Selected component"))
		lines = append(lines, sidebarKV("action", row.Action, inner))
		lines = append(lines, sidebarKV("kind", row.Kind, inner))
		lines = append(lines, sidebarKV("member", row.Member, inner))
		if row.Path != "" {
			lines = append(lines, sidebarKV("path", row.Path, inner))
		}
		if row.Namespace != "" {
			lines = append(lines, sidebarKV("namespace", row.Namespace, inner))
		}
		lines = append(lines, "")
		lines = append(lines, theme.Subtle.Render("  ↵ open org-side"))
		if row.Path != "" {
			lines = append(lines, theme.Subtle.Render("  "+firstPretty(Keys.BundleOpen)+" reveal on disk"))
		}
	}
	return strings.Join(lines, "\n")
}

// sidebarKV is a small label/value formatter shared with the
// other sidebar renderers. Pulled local to keep the import
// surface small; the project's helpers vary slightly per surface.
func sidebarKV(label, value string, inner int) string {
	if value == "" {
		value = "—"
	}
	return fmt.Sprintf("  %s: %s", theme.Subtle.Render(label), value)
}

// activeBundle resolves the currently drilled-in bundle row.
// Returns an error rather than (Bundle, bool) so the sidebar +
// activate paths can blend with the same fallback logic.
func (m Model) activeBundle() (devproject.Bundle, error) {
	if m.devProjects == nil {
		return devproject.Bundle{}, fmt.Errorf("dev-projects unavailable")
	}
	if m.bundleCur == "" {
		return devproject.Bundle{}, fmt.Errorf("no bundle drilled in")
	}
	b, err := m.devProjects.GetBundle(m.bundleCur)
	if err != nil {
		return devproject.Bundle{}, err
	}
	if b.ID == "" {
		return devproject.Bundle{}, fmt.Errorf("bundle %q not found", m.bundleCur)
	}
	return b, nil
}

// activateBundleDetail handles Enter on a bundle-detail row.
//
// Routes through drillByKind so the user lands in the canonical
// purple-bordered detail surface (TabFlowDetail, TabApexDetail,
// TabFieldDetail, …) with full ESC-return wiring via
// rememberDrillReturn(). Mirrors how global search, /home Recent,
// and /dev-projects items drill into the same destinations — one
// behaviour, one dispatcher.
//
// Bundle preview rows carry FullName (DeveloperName / class name)
// not the platform Id, so for kinds where drillByKind needs an Id
// (Flow / ApexClass / ApexTrigger / LWC / Aura) we resolve via the
// active org's loaded list first. When the list isn't loaded yet
// we kick the ensure + flash a hint pointing the user at the
// parent surface — refusing to drill silently is the right call
// because a deferred drill would surprise the user mid-loading.
func (m *Model) activateBundleDetail() tea.Cmd {
	// Files mode: Enter cd's into a directory (or pops to parent
	// on the .. row). Files themselves are no-op on Enter; `o`
	// is the way to actually open one.
	if m.bundleDetailView == bundleViewFiles {
		return m.activateBundleFile()
	}
	row, ok := m.bundleDetailList.Selected()
	if !ok {
		return nil
	}
	kind, ref, typeField, name, idLookupNeeded := bundleRowDrillTarget(row)
	if kind == "" {
		m.flash(fmt.Sprintf("no detail surface for %s — press %s to open on disk", row.Kind, firstPretty(Keys.BundleOpen)))
		return nil
	}
	if idLookupNeeded {
		resolvedID, resolvedType := m.resolveBundleRowIDFull(kind, row.Member)
		if resolvedID == "" {
			// Cold list — kick the ensure so a retry within a few
			// seconds lands. Flash explains the deferred behaviour
			// so a single Enter doesn't feel like a no-op.
			if _, ensureCmd := m.resolveBundleRowIDFullWithEnsure(kind); ensureCmd != nil {
				m.flash(fmt.Sprintf("loading %s list — press ↵ again when loaded", row.Kind))
				return ensureCmd
			}
			m.flash(fmt.Sprintf("can't resolve %s %q (not in this org's loaded list)", row.Kind, row.Member))
			return nil
		}
		ref = resolvedID
		// Triggers' detail surface is keyed by (sobject, id) — the
		// lookup carries the sobject as resolvedType.
		if resolvedType != "" {
			typeField = resolvedType
		}
	}
	cmd, handled := drillByKind(m, kind, ref, typeField, name, TabBundleDetail)
	if !handled {
		m.flash(fmt.Sprintf("no detail surface for %s — press %s to open on disk", row.Kind, firstPretty(Keys.BundleOpen)))
		return nil
	}
	return cmd
}

// bundleRowDrillTarget maps a bundle preview row (Salesforce
// metadata Type + FullName) into the (kind, ref, type, name)
// quartet drillByKind takes. The idLookupNeeded flag tells the
// caller "ref needs to become an Id before drillByKind is called"
// — true for kinds whose detail surfaces are keyed by platform
// Id (Flow / ApexClass / etc.).
//
// kind = "" means "no detail surface for this metadata type" —
// caller flashes a hint and the user falls back to o (reveal on
// disk) or the bundle-level Retrieve / Deploy keys.
func bundleRowDrillTarget(row bundleDetailRow) (kind, ref, typeField, name string, idLookupNeeded bool) {
	switch row.Kind {
	case "Flow":
		return "flow", row.Member, "", row.Member, true
	case "ApexClass":
		return "apex_class", row.Member, "", row.Member, true
	case "ApexTrigger":
		return "apex_trigger", row.Member, "", row.Member, true
	case "LightningComponentBundle":
		return "lwc", row.Member, "", row.Member, true
	case "AuraDefinitionBundle":
		return "aura", row.Member, "", row.Member, true
	case "CustomObject":
		return "sobject", row.Member, "", row.Member, false
	case "CustomField":
		// row.Member = "<sObject>.<Field>". drillByKind's KindField
		// arm splits this internally; typeField is the fallback when
		// ref is just the field name. We pass the dotted form as ref
		// to hit the split path.
		return "field", row.Member, "", row.Member, false
	case "ValidationRule":
		// Drill keyed by id, but ValidationRule names are sometimes
		// reused per object — we'd need a (sObject, ruleName) →
		// ruleId lookup. Punt to "no detail" until we add that;
		// users can still o to open the file or open the object's
		// Schema subtab.
		return "", "", "", "", false
	}
	return "", "", "", "", false
}

// resolveBundleRowIDFull does the cold lookup against the active
// org's loaded lists. Returns "" when the list isn't loaded or
// the name doesn't match.
func (m *Model) resolveBundleRowIDFull(kind, name string) (string, string) {
	d := m.activeOrgData()
	if d == nil {
		return "", ""
	}
	switch kind {
	case "flow":
		for _, f := range d.FlowList.Items() {
			if f.DeveloperName == name {
				return f.DefinitionID, ""
			}
		}
	case "apex_class":
		for _, a := range d.ApexClassList.Items() {
			if a.Name == name {
				return a.ID, ""
			}
		}
	case "apex_trigger":
		for _, t := range d.ApexTriggerList.Items() {
			if t.Name == name {
				return t.ID, t.Table
			}
		}
	case "lwc":
		for _, b := range d.LWCBundleList.Items() {
			if b.DeveloperName == name {
				return b.ID, ""
			}
		}
	case "aura":
		for _, b := range d.AuraBundleList.Items() {
			if b.DeveloperName == name {
				return b.ID, ""
			}
		}
	}
	return "", ""
}

// resolveBundleRowIDFullWithEnsure picks the right Ensure cmd for
// the requested kind so the caller can kick a load + retry.
func (m *Model) resolveBundleRowIDFullWithEnsure(kind string) (string, tea.Cmd) {
	d := m.activeOrgData()
	if d == nil {
		return "", nil
	}
	switch kind {
	case "flow":
		return "", d.Flows.Ensure(m.cache)
	case "apex_class":
		return "", d.ApexClasses.Ensure(m.cache)
	case "apex_trigger":
		return "", d.ApexTriggersFlat.Ensure(m.cache)
	case "lwc":
		return "", d.LWCBundles.Ensure(m.cache)
	case "aura":
		return "", d.AuraBundles.Ensure(m.cache)
	}
	return "", nil
}
