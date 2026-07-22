package ui

// Per-surface sidebars for /perms: the permissions dashboard and the
// permset/PSG/profile parent-detail drill. Split out of sidebar.go.

// sidebarPerms shows the cursored permset/PSG/profile's metadata on
// the /perms top tab. Keeps the body tight — overview-shape, the full
// drill-in view already has a dedicated overview subtab.
func (m Model) sidebarPerms(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}
	switch m.currentSubtab() {
	case SubtabPermSets:
		p, ok := d.PermSetList.Selected()
		if !ok {
			return sideEmpty("—")
		}
		title := p.Label
		if title == "" {
			title = p.Name
		}
		return m.kvPanelPills(inner, title, markPillsForPermSet(p), []kv{
			{"name", p.Name},
			{"type", dashIfEmpty(p.Type)},
			{"custom", boolLabel(p.IsCustom)},
			{"namespace", dashIfEmpty(p.NamespacePrefix)},
			{"license", dashIfEmpty(p.LicenseName)},
			{"id", p.ID},
		})
	case SubtabPSGs:
		g, ok := d.PSGList.Selected()
		if !ok {
			return sideEmpty("—")
		}
		title := g.MasterLabel
		if title == "" {
			title = g.DeveloperName
		}
		return m.kvPanelPills(inner, title, markPillsForPSG(g), []kv{
			{"name", g.DeveloperName},
			{"status", dashIfEmpty(g.Status)},
			{"namespace", dashIfEmpty(g.NamespacePrefix)},
			{"id", g.ID},
		})
	case SubtabProfiles:
		p, ok := d.ProfileList.Selected()
		if !ok {
			return sideEmpty("—")
		}
		return m.kvPanelPills(inner, p.Name, markPillsForProfile(p), []kv{
			{"user type", dashIfEmpty(p.UserType)},
			{"license", dashIfEmpty(p.UserLicenseName)},
			{"implicit permset", dashIfEmpty(p.PermissionSetID)},
			{"id", p.ID},
		})
	}
	return sideEmpty("—")
}

// sidebarPermParent shows the current drilled-in parent's identity
// on every TabPermParentDetail subtab. Per-subtab sidebars (e.g. the
// cursored field on the Fields subtab) land in later phases.
func (m Model) sidebarPermParent(inner int) string {
	kind, id, name, ok := m.currentPermParent()
	if !ok {
		return sideEmpty("—")
	}
	rows := []kv{
		{"kind", kind},
		{"id", id},
	}
	if len(m.orgs) > 0 {
		if d := m.data[m.orgs[m.selected].Username]; d != nil && d.PermParentPermSetID != "" && d.PermParentPermSetID != id {
			rows = append(rows, kv{"permset id", d.PermParentPermSetID})
		}
	}
	return renderKVPanel(inner, name, rows)
}
