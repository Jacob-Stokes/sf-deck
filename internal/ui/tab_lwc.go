package ui

// /components — list of LWC + Aura bundles in the active org. Subtab
// strip toggles between the two; same drill-in tab (TabLWCDetail) is
// reused for both — the renderer picks the data source by inspecting
// LWCCur and falling back to AuraDetail when LWCDetail doesn't have
// the id (cheap to maintain since both bundle kinds use 18-char Ids
// from the same Tooling pool).

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/highlight"
)

func (m Model) bulkTagsForBundles(kind devproject.ItemKind, lwc []sf.LWCBundle, aura []sf.AuraBundle) map[string][]devproject.Tag {
	if !m.settings.TagColumnVisible() {
		return nil
	}
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	domain, ptr := bundleCacheKey(kind, lwc, aura)
	return d.memoTagsFor(m.devProjects, domain, ptr, func() map[string][]devproject.Tag {
		keys, orgUser, ok := bundleLookupKeys(m, kind, lwc, aura)
		if !ok {
			return nil
		}
		out, err := m.devProjects.TagsForItems(orgUser, keys)
		if err != nil {
			return nil
		}
		return out
	})
}

func (m Model) bulkProjectsForBundles(kind devproject.ItemKind, lwc []sf.LWCBundle, aura []sf.AuraBundle) map[string][]devproject.DevProject {
	if !m.settings.ProjectColumnVisible() {
		return nil
	}
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	domain, ptr := bundleCacheKey(kind, lwc, aura)
	return d.memoProjectsFor(m.devProjects, domain, ptr, func() map[string][]devproject.DevProject {
		keys, orgUser, ok := bundleLookupKeys(m, kind, lwc, aura)
		if !ok {
			return nil
		}
		out, err := m.devProjects.ProjectsForItems(orgUser, keys)
		if err != nil {
			return nil
		}
		return out
	})
}

// bundleCacheKey returns the (domain, slice-pointer) pair for the
// gutter cache. Kind discriminates LWC vs Aura so the domain key
// can't collide when both surfaces happen to have lists of the same
// length.
func bundleCacheKey(kind devproject.ItemKind, lwc []sf.LWCBundle, aura []sf.AuraBundle) (string, uintptr) {
	switch kind {
	case devproject.KindLWC:
		return gutterDomainLWC, slicePtr(lwc)
	case devproject.KindAura:
		return gutterDomainAura, slicePtr(aura)
	}
	return "", 0
}

func bundleLookupKeys(m Model, kind devproject.ItemKind, lwc []sf.LWCBundle, aura []sf.AuraBundle) ([]devproject.TagLookupKey, string, bool) {
	if m.devProjects == nil {
		return nil, "", false
	}
	o, ok := m.currentOrg()
	if !ok {
		return nil, "", false
	}
	var keys []devproject.TagLookupKey
	switch kind {
	case devproject.KindLWC:
		for _, b := range lwc {
			keys = append(keys, devproject.TagLookupKey{Kind: kind, Ref: b.ID})
		}
	case devproject.KindAura:
		for _, b := range aura {
			keys = append(keys, devproject.TagLookupKey{Kind: kind, Ref: b.ID})
		}
	}
	if len(keys) == 0 {
		return nil, "", false
	}
	return keys, o.Username, true
}

func (m Model) renderComponents(w, innerH int) string {
	if len(m.orgs) == 0 {
		return noOrgPlaceholder()
	}
	if m.activeOrgData() == nil {
		return theme.Subtle.Render("  org data not loaded")
	}
	return m.dispatchSubtab(w, innerH, componentsSubtabs(), m.componentsSubtab(),
		map[Subtab]subtabBranch{
			SubtabComponentsAura: {Render: m.renderAuraList},
		},
		subtabBranch{Render: m.renderLWCList},
	)
}

func (m Model) renderLWCList(w, innerH int) string {
	inner := w - 4
	d := m.activeOrgData()
	if d == nil {
		return theme.Subtle.Render("  org data not loaded")
	}
	chips := m.stripRows(domainLWC, "*")
	if len(chips) == 0 {
		chips = []chipRow{{ID: "all", Label: "All", Count: -1}}
	}
	chipSel := m.lwcChipIdx()
	if chipSel < 0 || chipSel >= len(chips) {
		chipSel = 0
	}
	dash := m.renderDashboard("VIEWS", chips, chipSel, inner)

	var lines []string
	if dash != "" {
		lines = append(lines, dash)
	}

	if d.LWCBundles.FetchedAt().IsZero() {
		if d.LWCBundles.Busy() {
			lines = append(lines, dimLine("  loading LWCs…", inner))
		} else {
			lines = append(lines, dimLine("  press "+firstPretty(Keys.Refresh)+" to load LWCs", inner))
		}
		return strings.Join(lines, "\n")
	}

	model, ok := lwcListSurface.BuildRenderModel(m, d)
	if !ok {
		lines = append(lines, dimLine("  loading…", inner))
		return strings.Join(lines, "\n")
	}
	usedAbove := usedLines(lines)
	budget := innerH - usedAbove
	lines = append(lines, renderListModel(m, model, m.focus, inner, budget)...)
	return strings.Join(lines, "\n")
}

func (m Model) renderAuraList(w, innerH int) string {
	inner := w - 4
	d := m.activeOrgData()
	if d == nil {
		return theme.Subtle.Render("  org data not loaded")
	}
	chips := m.stripRows(domainAura, "*")
	if len(chips) == 0 {
		chips = []chipRow{{ID: "all", Label: "All", Count: -1}}
	}
	chipSel := m.auraChipIdx()
	if chipSel < 0 || chipSel >= len(chips) {
		chipSel = 0
	}
	dash := m.renderDashboard("VIEWS", chips, chipSel, inner)

	var lines []string
	if dash != "" {
		lines = append(lines, dash)
	}

	if d.AuraBundles.FetchedAt().IsZero() {
		if d.AuraBundles.Busy() {
			lines = append(lines, dimLine("  loading Aura bundles…", inner))
		} else {
			lines = append(lines, dimLine("  press "+firstPretty(Keys.Refresh)+" to load Aura bundles", inner))
		}
		return strings.Join(lines, "\n")
	}

	model, ok := auraListSurface.BuildRenderModel(m, d)
	if !ok {
		lines = append(lines, dimLine("  loading…", inner))
		return strings.Join(lines, "\n")
	}
	usedAbove := usedLines(lines)
	budget := innerH - usedAbove
	lines = append(lines, renderListModel(m, model, m.focus, inner, budget)...)
	return strings.Join(lines, "\n")
}

func (m Model) renderComponentsDetail(w, innerH int) string {
	inner := w - 4
	if len(m.orgs) == 0 {
		return noOrgPlaceholder()
	}
	d := m.activeOrgData()
	if d == nil || d.LWCCur == "" {
		return theme.Subtle.Render("  no component drilled in")
	}
	if d.AuraDetail != nil {
		if _, ok := d.AuraDetail[d.LWCCur]; ok {
			return m.renderAuraDetail(d, inner, innerH)
		}
	}
	return m.renderLWCDetail(d, inner, innerH)
}

type bundleFile struct {
	Label  string
	Source string
	Lang   string
}

func lwcBundleFiles(d *sf.LWCBundleDetail) []bundleFile {
	if d == nil {
		return nil
	}
	out := make([]bundleFile, 0, len(d.Resources))
	for _, r := range d.Resources {
		out = append(out, bundleFile{
			Label:  bundleLabelFromPath(r.FilePath),
			Source: r.Source,
			Lang:   highlight.LanguageForFilename(r.FilePath),
		})
	}
	return out
}

// auraBundleFiles returns the files inside an Aura bundle. Aura
// resources don't have a FilePath; the label is composed from
// DefType + Format the same way the legacy section header was.
func auraBundleFiles(d *sf.AuraBundleDetail) []bundleFile {
	if d == nil {
		return nil
	}
	out := make([]bundleFile, 0, len(d.Resources))
	for _, r := range d.Resources {
		header := strings.ToLower(r.DefType)
		if r.Format != "" {
			header += "." + strings.ToLower(r.Format)
		}
		out = append(out, bundleFile{
			Label:  header,
			Source: r.Source,
			Lang:   highlight.LanguageForAuraDefType(r.DefType, r.Format),
		})
	}
	return out
}

func bundleLabelFromPath(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func (m Model) renderLWCDetail(d *orgData, inner, innerH int) string {
	res := d.lwcDetailRes(m.orgs[m.selected].Alias, d.LWCCur)
	if res == nil {
		return theme.Subtle.Render("  LWC not loaded")
	}
	val := res.Value()
	files := lwcBundleFiles(&val)
	return m.renderBundleDetail(d, bundleHeader{
		Title:       val.Bundle.DeveloperName,
		Fallback:    d.LWCCur,
		Label:       val.Bundle.MasterLabel,
		ApiVersion:  val.Bundle.ApiVersion,
		Description: val.Bundle.Description,
		Exposed:     val.Bundle.IsExposed,
		ShowExposed: true,
	}, files, res.FetchedAt().IsZero(), res.Busy(), inner, innerH)
}

func (m Model) renderAuraDetail(d *orgData, inner, innerH int) string {
	res := d.auraDetailRes(m.orgs[m.selected].Alias, d.LWCCur)
	if res == nil {
		return theme.Subtle.Render("  Aura bundle not loaded")
	}
	val := res.Value()
	files := auraBundleFiles(&val)
	return m.renderBundleDetail(d, bundleHeader{
		Title:       val.Bundle.DeveloperName,
		Fallback:    d.LWCCur,
		Label:       val.Bundle.MasterLabel,
		ApiVersion:  val.Bundle.ApiVersion,
		Description: val.Bundle.Description,
	}, files, res.FetchedAt().IsZero(), res.Busy(), inner, innerH)
}

type bundleHeader struct {
	Title       string
	Fallback    string // the drill ID, fallback when DeveloperName is empty
	Label       string
	ApiVersion  float64
	Description string
	Exposed     bool
	ShowExposed bool
}

func (m Model) renderBundleDetail(d *orgData, h bundleHeader, files []bundleFile, loading, busy bool, inner, innerH int) string {
	title := h.Title
	if title == "" {
		title = h.Fallback
	}
	// Per-file subtab strip, rendered inline like every other
	// drill surface (perm parent, object drill, system). The global
	// renderMainHitLayers layer only contributes CLICK zones at the
	// same position — it never paints the strip, so skipping the
	// inline render here left the file switcher invisible (the
	// "drill only shows one file" bug).
	var lines []string
	subs := m.tabSubtabsForStrip()
	if strip := renderSubtabStrip(subs, m.currentSubtabIndex(subs), inner); strip != "" {
		lines = append(lines, strings.Split(strip, "\n")...)
	} else {
		lines = append(lines, "")
	}
	lines = append(lines, sectionTitle(title))

	meta := []string{}
	if h.Label != "" && h.Label != h.Title {
		meta = append(meta, "label: "+h.Label)
	}
	if h.ApiVersion > 0 {
		meta = append(meta, fmt.Sprintf("api: v%.1f", h.ApiVersion))
	}
	if h.ShowExposed && h.Exposed {
		meta = append(meta, "exposed")
	}
	if len(meta) > 0 {
		lines = append(lines, dimLine("  "+strings.Join(meta, " · "), inner))
	}
	if h.Description != "" {
		lines = append(lines, dimLine("  "+h.Description, inner))
	}
	lines = append(lines, "")

	if loading {
		if busy {
			lines = append(lines, dimLine("  loading bundle…", inner))
		} else {
			lines = append(lines, dimLine("  press "+firstPretty(Keys.Refresh)+" to load bundle", inner))
		}
		return strings.Join(lines, "\n")
	}
	if len(files) == 0 {
		lines = append(lines, dimLine("  (no resources)", inner))
		return strings.Join(lines, "\n")
	}

	idx := bundleFileIdx(d, d.LWCCur, len(files))
	cur := files[idx]
	lines = append(lines, sectionTitle(cur.Label))

	bodyHeight := innerH - len(lines)
	bodyView := m.renderCodeView(d, codeViewSpec{
		BodyID:  bundleBodyID(d.LWCCur, cur.Label),
		Body:    cur.Source,
		Lang:    cur.Lang,
		Inner:   inner,
		Height:  bodyHeight,
		Focused: true, // no action sidebar on bundle detail
	})
	lines = append(lines, bodyView...)
	return strings.Join(lines, "\n")
}

// bundleFileIdx reads the active-file index for the bundle from
// orgData, clamping to [0, n) so a stale index from a re-fetch
// (the bundle's file list shrank under us) never reaches the
// renderer's slice.
func bundleFileIdx(d *orgData, bundleID string, n int) int {
	if d == nil || bundleID == "" || n == 0 {
		return 0
	}
	if d.LWCFileIdx == nil {
		return 0
	}
	idx := d.LWCFileIdx[bundleID]
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = 0
	}
	return idx
}

func setBundleFileIdx(d *orgData, bundleID string, idx int) {
	if d == nil || bundleID == "" {
		return
	}
	if d.LWCFileIdx == nil {
		d.LWCFileIdx = map[string]int{}
	}
	d.LWCFileIdx[bundleID] = idx
}

func bundleBodyID(bundleID, fileLabel string) string {
	if bundleID == "" || fileLabel == "" {
		return ""
	}
	return "bundle:" + bundleID + ":" + fileLabel
}

func (m Model) activeBundleFiles(d *orgData) []bundleFile {
	if d == nil || d.LWCCur == "" || len(m.orgs) == 0 {
		return nil
	}
	alias := m.orgs[m.selected].Alias
	if d.AuraDetail != nil {
		if r, ok := d.AuraDetail[d.LWCCur]; ok && r != nil {
			val := r.Value()
			return auraBundleFiles(&val)
		}
	}
	if r := d.lwcDetailRes(alias, d.LWCCur); r != nil {
		val := r.Value()
		return lwcBundleFiles(&val)
	}
	return nil
}

func (m Model) lwcDetailSubtabs() []subtabInfo {
	d := m.activeOrgData()
	if d == nil || d.LWCCur == "" {
		return []subtabInfo{{ID: "", Label: ""}}
	}
	files := m.activeBundleFiles(d)
	if len(files) == 0 {
		return []subtabInfo{{ID: "", Label: ""}}
	}
	out := make([]subtabInfo, len(files))
	for i, f := range files {
		out[i] = subtabInfo{ID: Subtab("file:" + f.Label), Label: f.Label}
	}
	return out
}

func (m Model) bundleSubtabIdx() int {
	d := m.activeOrgData()
	if d == nil || d.LWCCur == "" {
		return 0
	}
	files := m.activeBundleFiles(d)
	return bundleFileIdx(d, d.LWCCur, len(files))
}

func (m *Model) setBundleSubtabIdx(idx int) {
	d := m.activeOrgData()
	if d == nil || d.LWCCur == "" {
		return
	}
	files := m.activeBundleFiles(d)
	if len(files) == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(files) {
		idx = len(files) - 1
	}
	setBundleFileIdx(d, d.LWCCur, idx)
}

func (m *Model) moveBundleDetailCursor(delta int) {
	d := m.activeOrgData()
	if d == nil {
		return
	}
	files := m.activeBundleFiles(d)
	if len(files) == 0 {
		return
	}
	idx := bundleFileIdx(d, d.LWCCur, len(files))
	cur := files[idx]
	if cur.Source == "" {
		return
	}
	m.codeViewMoveCursor(d, bundleBodyID(d.LWCCur, cur.Label), lineCount(cur.Source), delta)
}

func (m *Model) triggerOpenLWCBundle(id string) tea.Cmd {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	d.LWCCur = id
	m.setTab(TabLWCDetail)
	res := d.lwcDetailRes(m.orgs[m.selected].Alias, id)
	if res == nil {
		return m.onTabChanged()
	}
	return tea.Batch(m.onTabChanged(), res.Ensure(m.cache))
}

func (m *Model) triggerOpenAuraBundle(id string) tea.Cmd {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	d.LWCCur = id
	m.setTab(TabLWCDetail)
	res := d.auraDetailRes(m.orgs[m.selected].Alias, id)
	if res == nil {
		return m.onTabChanged()
	}
	return tea.Batch(m.onTabChanged(), res.Ensure(m.cache))
}

func (m Model) refreshLWCDetailData(d *orgData) tea.Cmd {
	if d == nil || d.LWCCur == "" || len(m.orgs) == 0 {
		return nil
	}
	alias := m.orgs[m.selected].Alias
	if _, ok := d.AuraDetail[d.LWCCur]; ok {
		if r := d.auraDetailRes(alias, d.LWCCur); r != nil {
			return r.Refresh(m.cache)
		}
	}
	if r := d.lwcDetailRes(alias, d.LWCCur); r != nil {
		return r.Refresh(m.cache)
	}
	return nil
}

func (d *orgData) lwcDetailRes(alias, id string) *Resource[sf.LWCBundleDetail] {
	if id == "" {
		return nil
	}
	if d.LWCDetail == nil {
		d.LWCDetail = map[string]*Resource[sf.LWCBundleDetail]{}
	}
	if r, ok := d.LWCDetail[id]; ok {
		return r
	}
	target := alias
	if target == "" {
		target = d.username
	}
	r := &Resource[sf.LWCBundleDetail]{
		Scope: d.username, Key: "lwc_bundle:" + id, TTL: 0,
		Fetch: func() (sf.LWCBundleDetail, error) {
			return sf.GetLWCBundle(target, id)
		},
	}
	d.LWCDetail[id] = r
	return r
}

func (d *orgData) auraDetailRes(alias, id string) *Resource[sf.AuraBundleDetail] {
	if id == "" {
		return nil
	}
	if d.AuraDetail == nil {
		d.AuraDetail = map[string]*Resource[sf.AuraBundleDetail]{}
	}
	if r, ok := d.AuraDetail[id]; ok {
		return r
	}
	target := alias
	if target == "" {
		target = d.username
	}
	r := &Resource[sf.AuraBundleDetail]{
		Scope: d.username, Key: "aura_bundle:" + id, TTL: 0,
		Fetch: func() (sf.AuraBundleDetail, error) {
			return sf.GetAuraBundle(target, id)
		},
	}
	d.AuraDetail[id] = r
	return r
}
