package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func (m *Model) moveBundleComponentCursor(delta int) {
	switch m.bundleDetailView {
	case bundleViewFiles:
		m.bundleFilesList.MoveBy(delta)
	default:
		m.bundleDetailList.MoveBy(delta)
	}
}

func (m *Model) resetBundleComponentCursor() {
	switch m.bundleDetailView {
	case bundleViewFiles:
		m.bundleFilesList.ResetCursor()
	default:
		m.bundleDetailList.ResetCursor()
	}
}

func (m Model) bundleDetailSearchPtr() *searchState {
	switch m.bundleDetailView {
	case bundleViewFiles:
		return m.bundleFilesList.SearchPtr()
	default:
		return m.bundleDetailList.SearchPtr()
	}
}

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
		m.flash("press " + firstPretty(Keys.BundleOpen) + " to open the file in the default app")
	}
	return nil
}

func popPathSegment(p string) string {
	if p == "" {
		return ""
	}
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}

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

func sidebarKV(label, value string, inner int) string {
	if value == "" {
		value = "—"
	}
	return fmt.Sprintf("  %s: %s", theme.Subtle.Render(label), value)
}

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
			if _, ensureCmd := m.resolveBundleRowIDFullWithEnsure(kind); ensureCmd != nil {
				m.flash(fmt.Sprintf("loading %s list — press ↵ again when loaded", row.Kind))
				return ensureCmd
			}
			m.flash(fmt.Sprintf("can't resolve %s %q (not in this org's loaded list)", row.Kind, row.Member))
			return nil
		}
		ref = resolvedID
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
		return "field", row.Member, "", row.Member, false
	case "ValidationRule":
		return "", "", "", "", false
	}
	return "", "", "", "", false
}

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
