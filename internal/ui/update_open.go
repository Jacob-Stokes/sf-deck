package ui

import (
	"net/url"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

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

func instanceHost(instanceURL string) string {
	u, err := url.Parse(instanceURL)
	if err == nil && u.Host != "" {
		return u.Host
	}
	s := strings.TrimPrefix(instanceURL, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	return s
}

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
		name := t.MasterLabel
		if name == "" {
			name = t.DeveloperName
		}
		m.rememberRecent(orgUser, RecentKindFlow, t.DefinitionID, name, t.DeveloperName)

	case sf.FlowVersion:
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
		m.rememberRecent(orgUser, RecentKindSObject, t.Name, t.Name, "")

	case sf.FieldRef:
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

func recordSObject(rec map[string]any) (string, bool) {
	attrs, ok := rec["attributes"].(map[string]any)
	if !ok {
		return "", false
	}
	t, _ := attrs["type"].(string)
	return t, t != ""
}

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

func copyRecordWithAttrs(rec map[string]any, sobj string) map[string]any {
	out := make(map[string]any, len(rec)+1)
	for k, v := range rec {
		out[k] = v
	}
	out["attributes"] = map[string]any{
		"type": sobj,
	}
	return out
}

func (m Model) cursorOpenable() sf.Openable {
	if m.focus == focusOrgs {
		if o, ok := m.currentOrg(); ok {
			return o
		}
		return nil
	}
	if id, ok := m.resolveItemIdentity(); ok && id.Openable != nil {
		return id.Openable
	}
	if surf := m.resolveOpenSurface(); surf != nil && surf.Openable != nil {
		if op := surf.Openable(m); op != nil {
			return op
		}
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
