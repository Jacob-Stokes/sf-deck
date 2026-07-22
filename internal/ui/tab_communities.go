package ui

// /communities — Experience sites (Networks) list + a drill into one
// community's pages and config.
//
// Layer 1 (solid): the community list with config + member count, and
// o → open menu (live site / Experience Builder / Administration /
// Setup). Layer 2 (best-effort): Enter drills into a community-detail
// view that lists the org's community-type FlexiPages. FlexiPages don't
// carry a foreign key to their Network, so the pages are grouped by
// name prefix as a best-effort approximation — the detail view says so.
// Layer 3 (page CONTENT via ExperienceBundle) is a documented TODO.

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func (m Model) renderCommunities(w, innerH int) string {
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	d := m.ensureOrgDataRef(o.Username)
	if d.Community.FetchedAt().IsZero() {
		inner := w - 4
		if d.Community.Busy() {
			return dimLine("  loading communities…", inner)
		}
		return dimLine("  press "+firstPretty(Keys.Refresh)+" to load communities", inner)
	}
	return renderListSurface(m, &communitiesListSurface, w, innerH, d)
}

// activateCommunities is Enter on a community row: drill into its pages.
func (m *Model) activateCommunities() tea.Cmd {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	r, ok := d.CommunityList.Selected()
	if !ok {
		return nil
	}
	d.CommunityCur = r.URLPathPrefix
	d.CommunityCurName = r.Name
	d.CommunityCurID = r.ID
	d.CommunityPageList.ResetCursor()
	if s := d.CommunityList.SearchPtr(); s.Active {
		s.Active = false
		s.Committed = s.Buffer() != ""
	}
	m.setTab(TabCommunityDetail)
	return m.onTabChanged()
}

func (m Model) renderCommunityDetail(w, innerH int) string {
	inner := w - 4
	d := m.activeOrgData()
	if d == nil {
		return noOrgPlaceholder()
	}
	if d.CommunityCurName == "" {
		return dimLine("  no community drilled in", inner)
	}
	res := d.CommunityPages[communityPageKey(d.CommunityCur)]
	var lines []string
	lines = append(lines, "")
	lines = append(lines, sectionTitle("  "+d.CommunityCurName+" · pages"))
	lines = append(lines, sideDim("  ⚠ org-wide community pages, best-effort grouping · full page content is a TODO", inner))
	if res == nil || res.FetchedAt().IsZero() {
		if res != nil && res.Busy() {
			lines = append(lines, dimLine("  loading pages…", inner))
		} else {
			lines = append(lines, dimLine("  press "+firstPretty(Keys.Refresh)+" to load pages", inner))
		}
		return strings.Join(lines, "\n")
	}
	d.SyncCommunityPageList()
	body := renderListSurface(m, &communityPagesListSurface, w, innerH-usedLines(lines), d)
	lines = append(lines, body)
	return strings.Join(lines, "\n")
}

func (d *orgData) SyncCommunityPageList() {
	if res := d.CommunityPages[communityPageKey(d.CommunityCur)]; res != nil {
		d.CommunityPageList.Set(res.Value())
		return
	}
	d.CommunityPageList.Set(nil)
}

func communityPageKey(prefix string) string {
	if prefix == "" {
		return "__default__"
	}
	return prefix
}

// EnsureCommunityPages lazily wires the keyed per-community page list.
func (d *orgData) EnsureCommunityPages(alias, prefix string) *Resource[[]sf.CommunityPageRow] {
	key := communityPageKey(prefix)
	return ensureKeyed(&d.CommunityPages, key, func() *Resource[[]sf.CommunityPageRow] {
		target := alias
		if target == "" {
			target = d.username
		}
		p := prefix
		return &Resource[[]sf.CommunityPageRow]{
			Scope: d.username, Key: "communitypages:" + key, TTL: 10 * time.Minute,
			Fetch: func() ([]sf.CommunityPageRow, error) {
				return sf.ListCommunityPages(target, p)
			},
		}
	})
}

func (m *Model) ensureCommunityDetailData(d *orgData, o sf.Org) tea.Cmd {
	if d.CommunityCurName == "" {
		return nil
	}
	return d.EnsureCommunityPages(targetArg(o), d.CommunityCur).Ensure(m.cache)
}

func (m Model) refreshCommunityDetailData(d *orgData) tea.Cmd {
	if d.CommunityCurName == "" || len(m.orgs) == 0 {
		return nil
	}
	return d.EnsureCommunityPages(targetArg(m.orgs[m.selected]), d.CommunityCur).Refresh(m.cache)
}
