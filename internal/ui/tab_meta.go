package ui

// /meta browses less common metadata types.

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func (m Model) renderMeta(w, innerH int) string {
	d := m.activeOrgData()
	if d == nil {
		return theme.Subtle.Render("  org data not loaded")
	}
	return m.dispatchSubtab(w, innerH, metaSubtabs(), m.metaSubtab(),
		map[Subtab]subtabBranch{
			SubtabMetaCustomMetadata: {Render: func(w, h int) string {
				return m.renderMetaList(&cmtListSurface,
					d.CMTTypes.FetchedAt().IsZero(), d.CMTTypes.Busy(),
					"custom metadata types", w, h)
			}},
			SubtabMetaCustomLabels: {Render: func(w, h int) string {
				return m.renderMetaList(&customLabelsListSurface,
					d.CustomLabels.FetchedAt().IsZero(), d.CustomLabels.Busy(),
					"custom labels", w, h)
			}},
			SubtabMetaCustomSettings: {Render: func(w, h int) string {
				return m.renderMetaList(&customSettingsListSurface,
					d.CustomSettings.FetchedAt().IsZero(), d.CustomSettings.Busy(),
					"custom settings", w, h)
			}},
			SubtabMetaStaticResources: {Render: func(w, h int) string {
				return m.renderMetaList(&staticResourcesListSurface,
					d.StaticResources.FetchedAt().IsZero(), d.StaticResources.Busy(),
					"static resources", w, h)
			}},
			SubtabMetaNamedCredentials: {Render: func(w, h int) string {
				return m.renderMetaList(&namedCredsListSurface,
					d.NamedCreds.FetchedAt().IsZero(), d.NamedCreds.Busy(),
					"named credentials", w, h)
			}},
			SubtabMetaRemoteSiteSettings: {Render: func(w, h int) string {
				return m.renderMetaList(&remoteSitesListSurface,
					d.RemoteSites.FetchedAt().IsZero(), d.RemoteSites.Busy(),
					"remote sites", w, h)
			}},
		},
		subtabBranch{Render: m.renderMetaBrowse},
	)
}

func (m Model) renderMetaList(surf *listSurface, notFetched, busy bool, noun string, w, innerH int) string {
	inner := w - 4
	d := m.activeOrgData()
	if d == nil {
		return theme.Subtle.Render("  org data not loaded")
	}
	if notFetched {
		if busy {
			return dimLine("  loading "+noun+"…", inner)
		}
		return dimLine("  press "+firstPretty(Keys.Refresh)+" to load "+noun, inner)
	}
	body := renderListSurface(m, surf, w, innerH, d)
	if body == "" {
		return dimLine("  loading…", inner)
	}
	return body
}

func (m Model) renderMetaBrowse(w, innerH int) string {
	d := m.activeOrgData()
	if d == nil {
		return theme.Subtle.Render("  org data not loaded")
	}
	inner := w - 4
	if d.MetaTypes.FetchedAt().IsZero() {
		if d.MetaTypes.Busy() {
			return dimLine("  describing metadata types…", inner)
		}
		return dimLine("  press "+firstPretty(Keys.Refresh)+" to load the type catalogue", inner)
	}
	body := renderListSurface(m, &metaTypesListSurface, w, innerH, d)
	if body == "" {
		return dimLine("  loading…", inner)
	}
	return body
}

func (m Model) renderMetaTypeDetail(w, innerH int) string {
	d := m.activeOrgData()
	if d == nil || d.MetaTypeCur == "" {
		return theme.Subtle.Render("  no type selected")
	}
	inner := w - 4
	res := d.MetaTypeItems[d.MetaTypeCur]
	if res == nil || (res.FetchedAt().IsZero() && res.Err() == nil) {
		return dimLine("  listing "+d.MetaTypeCur+" components…", inner)
	}
	if err := res.Err(); err != nil && res.FetchedAt().IsZero() {
		return strings.Join([]string{
			sectionTitle("  " + d.MetaTypeCur),
			"",
			theme.Subtle.Render("  listMetadata failed: " + err.Error()),
		}, "\n")
	}
	body := renderListSurface(m, &metaTypeItemsListSurface, w, innerH, d)
	if body == "" {
		return dimLine("  loading…", inner)
	}
	return body
}

func (m *Model) drillIntoMetaType() tea.Cmd {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	t, ok := d.MetaTypesList.Selected()
	if !ok {
		return nil
	}
	d.MetaTypeCur = t.XMLName
	d.SyncMetaTypeItemList()
	d.MetaTypeItemList.ResetCursor()
	m.setTab(TabMetaTypeDetail)
	res := d.metaTypeItemsRes(m.orgs[m.selected].Alias, t.XMLName)
	if res == nil {
		return m.onTabChanged()
	}
	return tea.Batch(m.onTabChanged(), res.Ensure(m.cache))
}

func (d *orgData) metaTypeItemsRes(alias, metaType string) *Resource[[]sf.MetadataItem] {
	if metaType == "" {
		return nil
	}
	if d.MetaTypeItems == nil {
		d.MetaTypeItems = map[string]*Resource[[]sf.MetadataItem]{}
	}
	if r, ok := d.MetaTypeItems[metaType]; ok {
		return r
	}
	target := alias
	if target == "" {
		target = d.username
	}
	mt := metaType
	r := &Resource[[]sf.MetadataItem]{
		Scope: d.username, Key: "metalist:" + mt, TTL: 30 * time.Minute,
		Fetch: func() ([]sf.MetadataItem, error) {
			return sf.ListMetadataComponents(target, mt)
		},
	}
	d.MetaTypeItems[mt] = r
	return r
}

func (m *Model) ensureMetaData(d *orgData, _ sf.Org) tea.Cmd {
	switch m.currentSubtab() {
	case SubtabMetaCustomMetadata:
		return d.CMTTypes.Ensure(m.cache)
	case SubtabMetaCustomLabels:
		return d.CustomLabels.Ensure(m.cache)
	case SubtabMetaCustomSettings:
		return d.CustomSettings.Ensure(m.cache)
	case SubtabMetaStaticResources:
		return d.StaticResources.Ensure(m.cache)
	case SubtabMetaNamedCredentials:
		return d.NamedCreds.Ensure(m.cache)
	case SubtabMetaRemoteSiteSettings:
		return d.RemoteSites.Ensure(m.cache)
	}
	return d.MetaTypes.Ensure(m.cache)
}

func (m Model) refreshMetaData(d *orgData) tea.Cmd {
	switch m.currentSubtab() {
	case SubtabMetaCustomMetadata:
		return d.CMTTypes.Refresh(m.cache)
	case SubtabMetaCustomLabels:
		return d.CustomLabels.Refresh(m.cache)
	case SubtabMetaCustomSettings:
		return d.CustomSettings.Refresh(m.cache)
	case SubtabMetaStaticResources:
		return d.StaticResources.Refresh(m.cache)
	case SubtabMetaNamedCredentials:
		return d.NamedCreds.Refresh(m.cache)
	case SubtabMetaRemoteSiteSettings:
		return d.RemoteSites.Refresh(m.cache)
	}
	return d.MetaTypes.Refresh(m.cache)
}

func (m *Model) ensureMetaTypeDetailData(d *orgData, o sf.Org) tea.Cmd {
	if d.MetaTypeCur == "" {
		return nil
	}
	res := d.metaTypeItemsRes(targetArg(o), d.MetaTypeCur)
	if res == nil {
		return nil
	}
	return res.Ensure(m.cache)
}

func (m Model) refreshMetaTypeDetailData(d *orgData) tea.Cmd {
	if d.MetaTypeCur == "" {
		return nil
	}
	if res := d.MetaTypeItems[d.MetaTypeCur]; res != nil {
		return res.Refresh(m.cache)
	}
	return nil
}
