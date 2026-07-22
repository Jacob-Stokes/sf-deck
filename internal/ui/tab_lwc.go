package ui

// /components — list of LWC + Aura bundles in the active org. Subtab
// strip toggles between the two; same drill-in tab (TabLWCDetail) is
// reused for both — the renderer picks the data source by inspecting
// LWCCur and falling back to AuraDetail when LWCDetail doesn't have
// the id (cheap to maintain since both bundle kinds use 18-char Ids
// from the same Tooling pool).
//
// Tab constant is still TabLWC for compatibility — the user-visible
// name is now "components".

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/highlight"
)

// bulkTagsForBundles pre-fetches tag bindings for LWC + Aura bundle
// lists. Pass either lwc or aura (the other nil) — kind tells the
// helper which item kind to query against. Memoised on *orgData via
// the gutter cache.
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

// bulkProjectsForBundles mirrors bulkTagsForBundles for the project
// gutter.
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

// bundleLookupKeys is the shared front-end for bulkTagsForBundles +
// bulkProjectsForBundles — guards on store/org availability and
// builds the TagLookupKey slice from whichever bundle slice was
// passed. Returns ok=false when the lookup should bail (no store,
// no org, or no bundles for the requested kind).
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

// renderComponents dispatches between the LWC + Aura subtabs.
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

// renderLWCList draws the LWC bundle list (default subtab).
// dispatchSubtab handles the subtab strip; this owns the chip
// dashboard + busy/loading branch + table.
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

// renderAuraList draws the Aura bundle list (Aura subtab).
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

// renderComponentsDetail dispatches to LWC or Aura detail based on
// which detail-cache holds the cursored Id. Both kinds drill into
// the same TabLWCDetail; we look up by Id rather than by subtab so
// the user can hold a class open across subtab toggles.
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

// bundleFile is a shape-erased view over an LWC or Aura resource
// that the detail renderer + cycle handler can both read from. The
// label is what shows up on the file-strip pill; the source is the
// raw body; the lang is the chroma lexer key.
type bundleFile struct {
	Label  string
	Source string
	Lang   string
}

// lwcBundleFiles returns the files inside an LWC bundle in their
// declared order (preserved from the Tooling fetch).
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

// bundleLabelFromPath shortens a "myComponent/myComponent.html" to
// "myComponent.html" — the bundle name is shown in the title strip
// already, no need to repeat it on every file pill.
func bundleLabelFromPath(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// renderLWCDetail draws one LWC bundle. Files inside the bundle are
// presented as a cycle-able strip (← / → step files); only the
// active file's body is rendered, scrollable via j / k like other
// code-detail surfaces.
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

// renderAuraDetail draws one Aura bundle — same shape as LWC.
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

// bundleHeader is the metadata block shared by LWC + Aura detail.
// LWC has IsExposed; Aura doesn't — the ShowExposed flag picks
// which surfaces include that pill.
type bundleHeader struct {
	Title       string
	Fallback    string // the drill ID, fallback when DeveloperName is empty
	Label       string
	ApiVersion  float64
	Description string
	Exposed     bool
	ShowExposed bool
}

// renderBundleDetail is the shared body of LWC + Aura detail views.
// Header + file strip + body viewport. The cursor / scroll state
// lives on d.BodyCursor / d.BodyScroll keyed by "<bundleId>:<label>"
// so cycling files preserves each file's last position.
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
		// Single-file bundle (or still loading): keep one reserved
		// line so the title doesn't sit under the hit-layer band.
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

// setBundleFileIdx records the active file index for a bundle.
func setBundleFileIdx(d *orgData, bundleID string, idx int) {
	if d == nil || bundleID == "" {
		return
	}
	if d.LWCFileIdx == nil {
		d.LWCFileIdx = map[string]int{}
	}
	d.LWCFileIdx[bundleID] = idx
}

// bundleBodyID is the cache key for a bundle resource's cursor +
// scroll. Per-(bundleID, fileLabel) so each file inside a bundle
// keeps its own viewport position.
func bundleBodyID(bundleID, fileLabel string) string {
	if bundleID == "" || fileLabel == "" {
		return ""
	}
	return "bundle:" + bundleID + ":" + fileLabel
}

// activeBundleFiles returns the file list for the currently-drilled
// bundle, dispatching by which kind (LWC vs Aura) is loaded. Empty
// when nothing is drilled in.
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

// lwcDetailSubtabs returns one subtab per resource file in the
// currently-drilled bundle. Synthetic IDs ("file:" + label) so the
// existing subtab registry / dispatcher machinery (Tab cycle,
// Shift+1..9 jump) Just Works without per-bundle TabSpec entries.
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

// bundleSubtabIdx is the GetSubtabIdx hook for TabLWCDetail. Reads
// the active file index for the currently-drilled bundle.
func (m Model) bundleSubtabIdx() int {
	d := m.activeOrgData()
	if d == nil || d.LWCCur == "" {
		return 0
	}
	files := m.activeBundleFiles(d)
	return bundleFileIdx(d, d.LWCCur, len(files))
}

// setBundleSubtabIdx is the SetSubtabIdx hook for TabLWCDetail.
// Writes the active file index for the currently-drilled bundle.
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

// moveBundleDetailCursor steers the body cursor for the currently
// active file within the drilled-in bundle.
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

// triggerOpenLWCBundle drills into an LWC bundle. Same target tab as
// Aura — TabLWCDetail — and the renderer picks which kind to draw by
// inspecting which detail map holds the id.
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

// triggerOpenAuraBundle is the Aura analog of triggerOpenLWCBundle.
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

// refreshLWCDetailData is the r-key refresh for TabLWCDetail: re-fetch
// the drilled-in bundle's resources (the keyed detail resource) — LWC or
// Aura, whichever the current bundle is. Without this r was a no-op on
// the component detail; only ctrl+r reached it.
func (m Model) refreshLWCDetailData(d *orgData) tea.Cmd {
	if d == nil || d.LWCCur == "" || len(m.orgs) == 0 {
		return nil
	}
	alias := m.orgs[m.selected].Alias
	// The detail view holds LWC and Aura bundles under the same cursor;
	// refresh whichever map owns this id (Aura checked first, mirroring
	// the renderer's dispatch).
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
