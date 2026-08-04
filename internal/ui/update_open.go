package ui

// Open / yank default-target actions + the cursor→Openable dispatcher.
//
// The Openable pattern (see internal/sf/openable.go) means every type
// that has a Lightning URL declares its own Targets(). `o` fires the
// default target; `ctrl+o` pops the menu to pick any. `y` / `ctrl+y`
// do the same but copy the URL to the clipboard.

import (
	"net/url"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// newRecordRef builds a RecordRef populated with the user's Inspector
// extension URL + the current org's host, so the returned ref's
// Targets() surfaces an "Inspector — Show all data" target when the
// user has configured one in settings.toml. No-op (blank Inspector
// fields) when the setting is unset.
//
// Also enriches ExtraTargets with sObject-specific actions that need
// org state to build — currently the Contact "Log in to community as
// user" targets.
func (m Model) newRecordRef(rec map[string]any) sf.RecordRef {
	ref := sf.RecordRef{Record: rec}
	o, hasOrg := m.currentOrg()
	if hasOrg && o.InstanceURL != "" && m.settings != nil {
		if base := m.settings.InspectorURL(); base != "" {
			ref.InspectorBase = base
			ref.InstanceHost = instanceHost(o.InstanceURL)
		}
	}
	if hasOrg {
		ref.ExtraTargets = append(ref.ExtraTargets, m.contactCommunityLoginTargets(rec, o)...)
		ref.ExtraTargets = append(ref.ExtraTargets, m.userLoginAsTargets(rec, o)...)
		ref.ExtraTargets = append(ref.ExtraTargets, m.relatedRecordOpenTarget()...)
	}
	return ref
}

// instanceHost strips scheme + trailing slash from an instance URL
// so Inspector's host= query param gets a bare hostname (what the
// extension expects — matches the pattern the user copies out of
// their browser's URL bar).
func instanceHost(instanceURL string) string {
	u, err := url.Parse(instanceURL)
	if err == nil && u.Host != "" {
		return u.Host
	}
	// Last-resort manual trim for malformed URLs.
	s := strings.TrimPrefix(instanceURL, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	return s
}

// openDefault fires the default target for whatever's under the cursor.
//
// Special case: on the /reports surfaces, when the user has at least
// one completed report export in history, `o` opens the most-recent
// saved file in the OS default app. Newer exports take precedence
// automatically (the registry stores history newest-first). To reach
// the SF report viewer instead, use the open-menu (O / Y).
func (m Model) openDefault() (tea.Model, tea.Cmd) {
	if m.exports != nil && (m.onReportsBrowser() || m.tab() == TabReportDetail) {
		if j := m.exports.mostRecentDone(exportKindReport); j != nil {
			path := j.Path
			m.flash("opening " + filepath.Base(path) + "…")
			return m, func() tea.Msg {
				_ = openPath(path)
				return nil
			}
		}
	}
	o, ok := m.currentOrg()
	if !ok {
		return m, nil
	}
	target := m.cursorOpenable()
	if target == nil {
		m.flash("nothing to open here")
		return m, nil
	}
	targets := target.Targets()
	if len(targets) == 0 {
		m.flash("no targets")
		return m, nil
	}
	m.recordRecentVisit(o.Username, target)
	m.flash("opening " + targets[0].Label + "…")
	return m, m.openInBrowserCmd(o, targets[0])
}

// recordRecentVisit lifts a visit-worthy Openable into the per-org
// "recently visited" log. Each case maps the openable's identity
// into a RecentEntry (Kind, ID, Name, Type) and funnels through
// rememberRecent for persistence.
//
// Skipped kinds (limits, async jobs, notifications) are transient
// metrics, not navigable destinations — recording them clutters the
// list without helping the user navigate.
func (m *Model) recordRecentVisit(orgUser string, target sf.Openable) {
	switch t := target.(type) {
	case sf.RecordRef:
		id, _ := t.Record["Id"].(string)
		if id == "" {
			return
		}
		sobject, _ := recordSObject(t.Record)
		name := recordDisplayName(t.Record)
		m.rememberRecent(orgUser, RecentKindRecord, id, name, sobject)

	case sf.ReportSummary:
		m.rememberRecent(orgUser, RecentKindReport, t.ID, t.Name, "")

	case sf.Flow:
		// DefinitionID is the stable identifier — versions share it.
		// Use MasterLabel as the display name; fall back to the
		// developer name when missing.
		name := t.MasterLabel
		if name == "" {
			name = t.DeveloperName
		}
		m.rememberRecent(orgUser, RecentKindFlow, t.DefinitionID, name, t.DeveloperName)

	case sf.FlowVersion:
		// Versions roll up to the parent flow for de-duplication —
		// the user reaches the version through its definition.
		// FlowVersion has no DeveloperName field; use the flow's
		// definition id as the secondary slot.
		m.rememberRecent(orgUser, RecentKindFlow, t.DefinitionID, t.MasterLabel, "")

	case sf.ApexClassRow:
		name := t.Name
		ns := t.NamespacePrefix
		m.rememberRecent(orgUser, RecentKindApexClass, t.ID, name, ns)

	case sf.LWCBundle:
		name := t.MasterLabel
		if name == "" {
			name = t.DeveloperName
		}
		m.rememberRecent(orgUser, RecentKindLWC, t.ID, name, t.DeveloperName)

	case sf.AuraBundle:
		name := t.MasterLabel
		if name == "" {
			name = t.DeveloperName
		}
		m.rememberRecent(orgUser, RecentKindAura, t.ID, name, t.DeveloperName)

	case sf.SObject:
		// SObject's ID isn't a 15/18-char Salesforce ID — its API name
		// is the stable identifier. Use that as the entry's ID so
		// re-visits collapse correctly.
		m.rememberRecent(orgUser, RecentKindSObject, t.Name, t.Name, "")

	case sf.FieldRef:
		// Field identity = "<sobject>.<fieldName>"; not a real SF Id.
		id := t.SObjectName + "." + t.Field.Name
		m.rememberRecent(orgUser, RecentKindField, id, t.Field.Name, t.SObjectName)

	case sf.PermissionSet:
		label := t.Label
		if label == "" {
			label = t.Name
		}
		m.rememberRecent(orgUser, RecentKindPermSet, t.ID, label, t.Name)

	case sf.PermissionSetGroup:
		label := t.MasterLabel
		if label == "" {
			label = t.DeveloperName
		}
		m.rememberRecent(orgUser, RecentKindPermSetGroup, t.ID, label, t.DeveloperName)

	case sf.Profile:
		m.rememberRecent(orgUser, RecentKindProfile, t.ID, t.Name, "")

	case sf.UserRow:
		name := t.Name
		if name == "" {
			name = t.Username
		}
		m.rememberRecent(orgUser, RecentKindUser, t.ID, name, t.Username)

	case sf.DeployRow:
		name := t.CreatedByName
		if name == "" {
			name = t.ID
		}
		m.rememberRecent(orgUser, RecentKindDeploy, t.ID, name, t.Status)

	case sf.InstalledPackage:
		m.rememberRecent(orgUser, RecentKindPackage, t.SubscriberPackageID, t.SubscriberPackageName, t.SubscriberPackageNamespace)

	case sf.QueueRow:
		m.rememberRecent(orgUser, RecentKindQueue, t.ID, t.Name, t.DeveloperName)

	case sf.PublicGroupRow:
		m.rememberRecent(orgUser, RecentKindPublicGroup, t.ID, t.Name, t.DeveloperName)

	case sf.ApexLogRow:
		m.rememberRecent(orgUser, RecentKindApexLog, t.ID, t.Operation, t.Status)
	}
}

// recordSObject extracts the sObject API name from a record's
// `attributes.type` field. Returns "" when missing — callers should
// treat that as "unknown" rather than a hard error.
func recordSObject(rec map[string]any) (string, bool) {
	attrs, ok := rec["attributes"].(map[string]any)
	if !ok {
		return "", false
	}
	t, _ := attrs["type"].(string)
	return t, t != ""
}

// recordDisplayName picks the most-useful label for a record row.
// Prefers Name, then a few common alternatives, then the Id.
func recordDisplayName(rec map[string]any) string {
	for _, k := range []string{"Name", "Subject", "CaseNumber", "DeveloperName", "Title"} {
		if v, ok := rec[k].(string); ok && v != "" {
			return v
		}
	}
	if id, ok := rec["Id"].(string); ok {
		return id
	}
	return ""
}

// yankDefault copies the default target's URL to the clipboard.
func (m Model) yankDefault() (tea.Model, tea.Cmd) {
	o, ok := m.currentOrg()
	if !ok {
		return m, nil
	}
	target := m.cursorOpenable()
	if target == nil {
		m.flash("nothing to yank here")
		return m, nil
	}
	targets := target.Targets()
	if len(targets) == 0 {
		m.flash("no targets")
		return m, nil
	}
	m.flash("url copied: " + targets[0].Label)
	return m, yankURLCmd(o, targets[0])
}

// copyRecordWithAttrs returns a shallow copy of rec with an
// `attributes` block set so sf.RecordRef.LightningPath can infer the
// sObject + record ID. Used when the record came from a ListView
// `/results` response, which doesn't include `attributes` on rows.
func copyRecordWithAttrs(rec map[string]any, sobj string) map[string]any {
	out := make(map[string]any, len(rec)+1)
	for k, v := range rec {
		out[k] = v
	}
	out["attributes"] = map[string]any{
		"type": sobj,
		// `url` isn't needed — Openable falls back to reading
		// rec["Id"] when the URL parse yields nothing.
	}
	return out
}

// cursorOpenable returns whatever thing the cursor is currently pointing
// at, as an sf.Openable. Returns nil if there's nothing meaningful to
// open from the current view/cursor. Used by the global `o` / `ctrl+o`
// (open) and `y` / `ctrl+y` (yank) shortcuts so every view gets those
// keys for free.
func (m Model) cursorOpenable() sf.Openable {
	if m.focus == focusOrgs {
		if o, ok := m.currentOrg(); ok {
			return o
		}
		return nil
	}
	// Identity-first: the registry-driven cursored-item resolver is
	// the canonical "what's under the cursor" answer. When the
	// surface's Identity closure returns an Openable we end the
	// lookup here. This dedupes simple openSurface.Openable entries
	// that just walk the same Selected() call and return the same
	// item — they can leave openSurface.Openable nil entirely.
	if id, ok := m.resolveItemIdentity(); ok && id.Openable != nil {
		return id.Openable
	}
	// openSurface fallback: surfaces without Identity (or whose
	// Identity didn't carry an Openable) resolve through the
	// declarative open-surface registry.
	if surf := m.resolveOpenSurface(); surf != nil && surf.Openable != nil {
		if op := surf.Openable(m); op != nil {
			return op
		}
		// Home tabs with no row (empty list state) fall back to the
		// org itself so o still does *something*.
		if m.tab() == TabHome {
			if o, ok := m.currentOrg(); ok {
				return o
			}
		}
		return nil
	}
	// Every per-tab openable now lives on the registry: either the
	// surface's Identity closure (consulted above) or an
	// openSurface.Openable on the TabSpec/SubtabSpec (see
	// open_surface.go). No legacy per-tab fallback remains —
	// TestNoTabSwitchesOutsideRegistry keeps it that way.
	return nil
}
