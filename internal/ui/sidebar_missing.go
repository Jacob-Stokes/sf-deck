package ui

// Info panels for surfaces that previously had a blank right rail.
// Each mirrors the established sidebar shape (kv-panel + tag/project
// sections where the item is collectable + a footer hint), so these
// surfaces now match every other list/detail surface in the app.
//
// Covered here:
//   /users              (TabUsers)              — sidebarUsers
//   /components          (TabLWC)               — sidebarComponents (LWC / Aura)
//   /component detail    (TabLWCDetail)         — sidebarComponentsDetail
//   /meta hub            (TabMeta)              — sidebarMetaHub
//   /meta-type detail    (TabMetaTypeDetail)    — sidebarMetaTypeDetail
//   /queue detail        (TabQueueDetail)       — sidebarQueueDetail
//   /public-group detail (TabPublicGroupDetail) — sidebarPublicGroupDetail

import (
	"fmt"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

// ---- /users -----------------------------------------------------------

// sidebarUsers renders the cursored user's card in the right pane.
func (m Model) sidebarUsers(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}
	u, ok := d.HomeUserList.Selected()
	if !ok {
		return sideEmpty("no matches")
	}
	rows := []kv{
		{"id", u.ID},
		{"username", u.Username},
		{"active", yesNo(u.IsActive)},
	}
	if u.ProfileName != "" {
		rows = append(rows, kv{"profile", u.ProfileName})
	}
	if u.UserRoleName != "" {
		rows = append(rows, kv{"role", u.UserRoleName})
	}
	if u.LastLoginDate != "" {
		rows = append(rows, kv{"last login", prettyDate(u.LastLoginDate)})
	}
	extra := []string{"", sideDim("  ↵ open · "+
		firstPretty(Keys.OpenDefault)+" Lightning", inner)}
	return renderKVPanel(inner, u.Name, rows, extra...)
}

// ---- /apex-class detail -----------------------------------------------

// sidebarApexDetail shows the drilled Apex class's metadata — same card
// as the /apex list sidebar, resolved from the drilled ApexCur id.
func (m Model) sidebarApexDetail(inner int) string {
	d := m.activeOrgData()
	if d == nil || d.ApexCur == "" {
		return sideEmpty("no class drilled in")
	}
	a, ok := apexListRowFor(d, d.ApexCur)
	if !ok {
		// Row not in the list cache (e.g. drilled from search) — show
		// what we have.
		return renderKVPanel(inner, d.ApexCur,
			[]kv{{"id", d.ApexCur}},
			"", sideDim("  ↵ open body · esc back", inner))
	}
	rows := []kv{
		{"id", a.ID},
		{"status", dashIfEmpty(a.Status)},
		{"valid", yesNo(a.IsValid)},
	}
	if a.NamespacePrefix != "" {
		rows = append(rows, kv{"namespace", a.NamespacePrefix})
	}
	if a.ApiVersion > 0 {
		rows = append(rows, kv{"api version", fmt.Sprintf("v%.1f", a.ApiVersion)})
	}
	if a.LengthNoComments > 0 {
		// LengthWithoutComments is a CHARACTER count (body minus
		// comments), not a line count — show it as a size, with the
		// exact figure alongside the compact form.
		rows = append(rows, kv{"size", fmt.Sprintf("%s (%d chars)", compactChars(a.LengthNoComments), a.LengthNoComments)})
	}
	if a.LastModifiedDate != "" {
		rows = append(rows, kv{"modified", prettyDate(a.LastModifiedDate)})
	}
	var extra []string
	o, _ := m.currentOrg()
	extra = append(extra, m.sidebarTagsProjectsExtra(devproject.KindApexClass, a.ID, o.Username, inner)...)
	extra = append(extra, "", sideDim("  ↵ open body · "+
		firstPretty(Keys.OpenDefault)+" Lightning · esc back", inner))
	return m.kvPanelTagged(inner, a.Name, nil,
		devproject.KindApexClass, a.ID, o.Username, rows, extra...)
}

// ---- /components (LWC + Aura) -----------------------------------------

// sidebarComponents dispatches to the LWC or Aura sidebar based on the
// active subtab.
func (m Model) sidebarComponents(inner int) string {
	switch m.currentSubtab() {
	case SubtabComponentsAura:
		return m.sidebarAuraBundle(inner)
	default:
		return m.sidebarLWCBundle(inner)
	}
}

func (m Model) sidebarLWCBundle(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}
	b, ok := d.LWCBundleList.Selected()
	if !ok {
		return sideEmpty("no matches")
	}
	rows := []kv{
		{"id", b.ID},
		{"api name", b.DeveloperName},
		{"exposed", yesNo(b.IsExposed)},
	}
	if b.NamespacePrefix != "" {
		rows = append(rows, kv{"namespace", b.NamespacePrefix})
	}
	if b.ApiVersion > 0 {
		rows = append(rows, kv{"api version", fmt.Sprintf("v%.1f", b.ApiVersion)})
	}
	if b.LastModifiedDate != "" {
		rows = append(rows, kv{"modified", prettyDate(b.LastModifiedDate)})
	}
	if b.LastModifiedByName != "" {
		rows = append(rows, kv{"modified by", b.LastModifiedByName})
	}
	extra := m.componentExtra(devproject.KindLWC, b.ID, o.Username, inner, "resources")
	label := b.MasterLabel
	if label == "" {
		label = b.DeveloperName
	}
	return m.kvPanelTagged(inner, label, nil,
		devproject.KindLWC, b.ID, o.Username, rows, extra...)
}

func (m Model) sidebarAuraBundle(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}
	b, ok := d.AuraBundleList.Selected()
	if !ok {
		return sideEmpty("no matches")
	}
	rows := []kv{
		{"id", b.ID},
		{"api name", b.DeveloperName},
	}
	if b.NamespacePrefix != "" {
		rows = append(rows, kv{"namespace", b.NamespacePrefix})
	}
	if b.ApiVersion > 0 {
		rows = append(rows, kv{"api version", fmt.Sprintf("v%.1f", b.ApiVersion)})
	}
	if b.LastModifiedDate != "" {
		rows = append(rows, kv{"modified", prettyDate(b.LastModifiedDate)})
	}
	if b.LastModifiedByName != "" {
		rows = append(rows, kv{"modified by", b.LastModifiedByName})
	}
	extra := m.componentExtra(devproject.KindAura, b.ID, o.Username, inner, "resources")
	label := b.MasterLabel
	if label == "" {
		label = b.DeveloperName
	}
	return m.kvPanelTagged(inner, label, nil,
		devproject.KindAura, b.ID, o.Username, rows, extra...)
}

// componentExtra builds the shared tag/project sections + footer hint
// for LWC/Aura bundle sidebars.
func (m Model) componentExtra(kind devproject.ItemKind, id, orgUser string, inner int, drillNoun string) []string {
	var extra []string
	extra = append(extra, m.sidebarTagsProjectsExtra(kind, id, orgUser, inner)...)
	extra = append(extra, "", sideDim("  ↵ open "+drillNoun+" · "+
		firstPretty(Keys.OpenDefault)+" Lightning", inner))
	return extra
}

// ---- /component detail ------------------------------------------------

// sidebarComponentsDetail shows the drilled bundle's resource count +
// metadata. Reads whichever of LWC/Aura detail is loaded for LWCCur.
func (m Model) sidebarComponentsDetail(inner int) string {
	d := m.activeOrgData()
	if d == nil || d.LWCCur == "" {
		return sideEmpty("no component drilled in")
	}
	// Aura detail wins when present for this id, else LWC.
	if d.AuraDetail != nil {
		if det, ok := d.AuraDetail[d.LWCCur]; ok && det != nil {
			v := det.Value()
			b := v.Bundle
			rows := []kv{
				{"id", d.LWCCur},
				{"type", "Aura"},
				{"files", fmt.Sprintf("%d", len(v.Resources))},
			}
			if b.ApiVersion > 0 {
				rows = append(rows, kv{"api version", fmt.Sprintf("v%.1f", b.ApiVersion)})
			}
			label := b.MasterLabel
			if label == "" {
				label = b.DeveloperName
			}
			return renderKVPanel(inner, label,
				rows, "", sideDim("  "+firstPretty(Keys.PrevView)+" / "+firstPretty(Keys.NextView)+" cycle files · esc back", inner))
		}
	}
	if d.LWCDetail != nil {
		if det, ok := d.LWCDetail[d.LWCCur]; ok && det != nil {
			v := det.Value()
			b := v.Bundle
			rows := []kv{
				{"id", d.LWCCur},
				{"type", "LWC"},
				{"files", fmt.Sprintf("%d", len(v.Resources))},
			}
			if b.ApiVersion > 0 {
				rows = append(rows, kv{"api version", fmt.Sprintf("v%.1f", b.ApiVersion)})
			}
			if b.IsExposed {
				rows = append(rows, kv{"exposed", "yes"})
			}
			label := b.MasterLabel
			if label == "" {
				label = b.DeveloperName
			}
			return renderKVPanel(inner, label,
				rows, "", sideDim("  "+firstPretty(Keys.PrevView)+" / "+firstPretty(Keys.NextView)+" cycle files · esc back", inner))
		}
	}
	return sideEmpty("loading…")
}

// ---- /meta hub --------------------------------------------------------

// sidebarMetaHub shows the cursored metadata type's describe info.
func (m Model) sidebarMetaHub(inner int) string {
	d := m.activeOrgData()
	if d == nil {
		return sideEmpty("—")
	}
	t, ok := d.MetaTypesList.Selected()
	if !ok {
		return sideEmpty("no matches")
	}
	rows := []kv{
		{"in folder", yesNo(t.InFolder)},
		{"has meta file", yesNo(t.MetaFile)},
	}
	if t.DirectoryName != "" {
		rows = append(rows, kv{"directory", t.DirectoryName})
	}
	if n := len(t.ChildXMLNames); n > 0 {
		rows = append(rows, kv{"child types", fmt.Sprintf("%d", n)})
	}
	extra := []string{"", sideDim("  ↵ browse components", inner)}
	return renderKVPanel(inner, t.XMLName, rows, extra...)
}

// ---- /meta-type detail ------------------------------------------------

// sidebarMetaTypeDetail shows the drilled metadata type + its component
// count.
func (m Model) sidebarMetaTypeDetail(inner int) string {
	d := m.activeOrgData()
	if d == nil || d.MetaTypeCur == "" {
		return sideEmpty("no type drilled in")
	}
	rows := []kv{{"type", d.MetaTypeCur}}
	if res, ok := d.MetaTypeItems[d.MetaTypeCur]; ok && res != nil {
		if !res.FetchedAt().IsZero() {
			rows = append(rows, kv{"components", fmt.Sprintf("%d", len(res.Value()))})
		} else if res.Busy() {
			rows = append(rows, kv{"components", "loading…"})
		}
	}
	extra := []string{"", sideDim("  ↵ open component · esc back", inner)}
	return renderKVPanel(inner, d.MetaTypeCur, rows, extra...)
}

// ---- /queue + /public-group detail ------------------------------------

// sidebarQueueDetail / sidebarPublicGroupDetail render the drilled
// group's metadata + resolved member count.
func (m Model) sidebarQueueDetail(inner int) string {
	return m.sidebarGroupDetail(inner, "Queue")
}

func (m Model) sidebarPublicGroupDetail(inner int) string {
	return m.sidebarGroupDetail(inner, "Public Group")
}

func (m Model) sidebarGroupDetail(inner int, parentLabel string) string {
	d := m.activeOrgData()
	if d == nil || d.GroupMemberID == "" {
		return sideEmpty("no group drilled in")
	}
	id := d.GroupMemberID
	name := groupParentName(d, d.GroupMemberKind, id)
	if name == "" {
		name = id
	}
	rows := []kv{
		{"id", id},
		{"type", parentLabel},
	}
	// Resolved member count from the loaded members resource.
	if res := d.GroupMembers[id]; res != nil && !res.FetchedAt().IsZero() {
		rows = append(rows, kv{"members", fmt.Sprintf("%d", len(res.Value()))})
	}
	// Queue-specific extras from the parent list row.
	if d.GroupMemberKind == "queue" {
		for _, q := range d.QueueList.Items() {
			if q.ID == id {
				if q.Email != "" {
					rows = append(rows, kv{"email", q.Email})
				}
				if n := len(q.SObjects); n > 0 {
					rows = append(rows, kv{"sObjects", fmt.Sprintf("%d", n)})
				}
				break
			}
		}
	}
	extra := []string{"", sideDim("  ↵ open member · esc back", inner)}
	return renderKVPanel(inner, parentLabel+" · "+name, rows, extra...)
}
