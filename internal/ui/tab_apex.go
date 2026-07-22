package ui

// /apex — list view of every ApexClass in the active org. Drill in to
// /apex-class-detail to read the body.
//
// Mirrors the shape of /flows + /packages: top header with count +
// search pill, table of rows, footer hint. Cursor + filter come from
// the shared ListView wrapper on orgData.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/highlight"
)

// renderApex draws the Apex tab — dispatches between Classes (default
// list-table) and a flat cross-sObject Triggers subtab.
func (m Model) renderApex(w, innerH int) string {
	if len(m.orgs) == 0 {
		return noOrgPlaceholder()
	}
	if m.activeOrgData() == nil {
		return theme.Subtle.Render("  org data not loaded")
	}
	// VF Pages / VF Components subtab stubs were cut 2026-06-13 —
	// /meta Browse lists both types (names + modified) and a
	// dedicated subtab would add nothing but two columns. Same
	// rationale as the /meta stub cut.
	return m.dispatchSubtab(w, innerH, apexSubtabs(), m.apexSubtab(),
		map[Subtab]subtabBranch{
			SubtabApexTriggers: {Render: m.renderApexTriggers},
		},
		subtabBranch{Render: m.renderApexClasses},
	)
}

// renderApexClasses is the default Apex subtab body — chip strip
// over the classes ListView, busy/loading states above the table.
// dispatchSubtab handles the subtab strip; this function only
// owns the chip dashboard + table.
func (m Model) renderApexClasses(w, innerH int) string {
	inner := w - 4
	d := m.activeOrgData()
	if d == nil {
		return theme.Subtle.Render("  org data not loaded")
	}
	chips := m.stripRows(domainApex, "*")
	if len(chips) == 0 {
		chips = []chipRow{{ID: "all", Label: "All", Count: -1}}
	}
	chipSel := m.apexChipIdx()
	if chipSel < 0 || chipSel >= len(chips) {
		chipSel = 0
	}
	dash := m.renderDashboard("VIEWS", chips, chipSel, inner)

	var lines []string
	if dash != "" {
		lines = append(lines, dash)
	}

	if d.ApexClasses.FetchedAt().IsZero() {
		if d.ApexClasses.Busy() {
			lines = append(lines, dimLine("  loading apex classes…", inner))
		} else {
			lines = append(lines, dimLine("  press "+firstPretty(Keys.Refresh)+" to load apex classes", inner))
		}
		return strings.Join(lines, "\n")
	}

	model, ok := apexClassesListSurface.BuildRenderModel(m, d)
	if !ok {
		lines = append(lines, dimLine("  loading…", inner))
		return strings.Join(lines, "\n")
	}
	usedAbove := usedLines(lines)
	budget := innerH - usedAbove
	lines = append(lines, renderListModel(m, model, m.focus, inner, budget)...)
	return strings.Join(lines, "\n")
}

// apexListRowFor returns the cached ApexClassRow for the drilled-in
// class, or (zero, false) when the list hasn't been fetched yet
// (rare — drill-in is gated on having seen the row in the list).
// Used by renderApexDetail to render mark pills against the same
// row data the list-view used.
func apexListRowFor(d *orgData, classID string) (sf.ApexClassRow, bool) {
	for _, a := range d.ApexClassList.Items() {
		if a.ID == classID {
			return a, true
		}
	}
	return sf.ApexClassRow{}, false
}

// renderApexTriggers is the Triggers subtab — flat list across every
// sObject in the org. Mirrors the Classes layout minus the chip strip
// Triggers shares the chipSurface registry — the chip strip drives
// managed/unmanaged + active/inactive filtering. Drill-in routes
// through the parent sObject's per-sObject Triggers detail tab so
// writes go through the existing trigger update path.
func (m Model) renderApexTriggers(w, innerH int) string {
	inner := w - 4
	d := m.activeOrgData()
	if d == nil {
		return theme.Subtle.Render("  org data not loaded")
	}
	chips := m.stripRows(domainTriggers, "*")
	if len(chips) == 0 {
		chips = []chipRow{{ID: "all", Label: "All", Count: -1}}
	}
	chipSel := m.apexTriggersChipIdx()
	if chipSel < 0 || chipSel >= len(chips) {
		chipSel = 0
	}
	dash := m.renderDashboard("VIEWS", chips, chipSel, inner)

	var lines []string
	if dash != "" {
		lines = append(lines, dash)
	}

	if d.ApexTriggersFlat.FetchedAt().IsZero() {
		if d.ApexTriggersFlat.Busy() {
			lines = append(lines, dimLine("  loading triggers…", inner))
		} else {
			lines = append(lines, dimLine("  press "+firstPretty(Keys.Refresh)+" to load triggers", inner))
		}
		return strings.Join(lines, "\n")
	}

	model, ok := apexTriggersListSurface.BuildRenderModel(m, d)
	if !ok {
		lines = append(lines, dimLine("  loading…", inner))
		return strings.Join(lines, "\n")
	}
	usedAbove := usedLines(lines)
	budget := innerH - usedAbove
	lines = append(lines, renderListModel(m, model, m.focus, inner, budget)...)
	return strings.Join(lines, "\n")
}

// renderApexDetail draws one apex class — name + meta + body.
func (m Model) renderApexDetail(w, innerH int) string {
	inner := w - 4
	if len(m.orgs) == 0 {
		return noOrgPlaceholder()
	}
	d := m.activeOrgData()
	if d == nil || d.ApexCur == "" {
		return theme.Subtle.Render("  no apex class drilled in")
	}
	res := d.apexClassDetailRes(m.orgs[m.selected].Alias, d.ApexCur)
	if res == nil {
		return theme.Subtle.Render("  apex class not loaded")
	}
	var lines []string
	val := res.Value()
	title := val.Name
	if title == "" {
		title = d.ApexCur
	}
	lines = append(lines, sectionTitle(title))
	// Pills row: managed-package badge + invalid badge if applicable.
	// Built from the cached list-view ApexClassRow (val is a richer
	// detail struct without NamespacePrefix); fall back gracefully
	// when not in the list cache.
	if cur, ok := apexListRowFor(d, d.ApexCur); ok {
		if pills := renderMarkPills(markPillsForApexClass(cur)); pills != "" {
			lines = append(lines, "  "+pills)
		}
	}

	meta := []string{}
	if val.Status != "" {
		meta = append(meta, "status: "+val.Status)
	}
	if val.ApiVersion > 0 {
		meta = append(meta, fmt.Sprintf("api: v%.1f", val.ApiVersion))
	}
	if val.LengthNoComments > 0 {
		meta = append(meta, fmt.Sprintf("%d lines", val.LengthNoComments))
	}
	if val.IsValid {
		meta = append(meta, "valid")
	} else if !res.FetchedAt().IsZero() {
		meta = append(meta, "INVALID")
	}
	if len(meta) > 0 {
		lines = append(lines, dimLine("  "+strings.Join(meta, " · "), inner))
	}
	lines = append(lines, "")

	if res.FetchedAt().IsZero() {
		if res.Busy() {
			lines = append(lines, dimLine("  loading body…", inner))
		} else {
			lines = append(lines, dimLine("  press "+firstPretty(Keys.Refresh)+" to load class body", inner))
		}
		return strings.Join(lines, "\n")
	}
	if val.Body == "" {
		lines = append(lines, dimLine("  (empty body)", inner))
		return strings.Join(lines, "\n")
	}
	bodyHeight := innerH - len(lines)
	bodyView := m.renderCodeView(d, codeViewSpec{
		BodyID:  apexBodyID(d.ApexCur),
		Body:    val.Body,
		Lang:    highlight.LangApex,
		Inner:   inner,
		Height:  bodyHeight,
		Focused: true, // no action sidebar on apex class detail
	})
	lines = append(lines, bodyView...)
	return strings.Join(lines, "\n")
}

// apexBodyID is the cache key for the apex class body's cursor +
// scroll. Kept small + namespaced so it can never collide with
// trigger / LWC body keys.
func apexBodyID(classID string) string {
	if classID == "" {
		return ""
	}
	return "apex:" + classID
}

// moveApexDetailCursor steers the body cursor on TabApexDetail.
// Wired in via TabSpec.MoveCursor; called by the global j / k / G
// dispatcher in update_nav.go.
func (m *Model) moveApexDetailCursor(delta int) {
	d := m.activeOrgData()
	if d == nil || d.ApexCur == "" {
		return
	}
	r := d.apexClassDetailRes(m.orgs[m.selected].Alias, d.ApexCur)
	if r == nil {
		return
	}
	body := r.Value().Body
	if body == "" {
		return
	}
	m.codeViewMoveCursor(d, apexBodyID(d.ApexCur), lineCount(body), delta)
}

// triggerOpenApexClass moves the cursor onto a specific class and
// switches to TabApexDetail. Mirrors openFieldCmd / openFlowCmd from
// search_global.go; used by Enter on /apex.
func (m *Model) triggerOpenApexClass(id string) tea.Cmd {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	d.ApexCur = id
	m.setTab(TabApexDetail)
	res := d.apexClassDetailRes(m.orgs[m.selected].Alias, id)
	if res == nil {
		return m.onTabChanged()
	}
	return tea.Batch(m.onTabChanged(), res.Ensure(m.cache))
}

// apexClassDetailRes fetches-or-creates the per-id detail Resource.
// Lazy-allocates the map to keep model.go's init terse.
// refreshApexDetailData is the r-key refresh for TabApexDetail: re-fetch
// the drilled-in class's body (the keyed detail resource), not just the
// class list. Without this, r was a no-op on the apex detail — only
// ctrl+r (refresh-all-loaded) reached the body, because it re-fetches
// every resource for the org regardless of tab.
func (m Model) refreshApexDetailData(d *orgData) tea.Cmd {
	if d == nil || d.ApexCur == "" || len(m.orgs) == 0 {
		return nil
	}
	res := d.apexClassDetailRes(m.orgs[m.selected].Alias, d.ApexCur)
	if res == nil {
		return nil
	}
	return res.Refresh(m.cache)
}

func (d *orgData) apexClassDetailRes(alias, id string) *Resource[sf.ApexClassDetail] {
	if id == "" {
		return nil
	}
	if d.ApexClassDetail == nil {
		d.ApexClassDetail = map[string]*Resource[sf.ApexClassDetail]{}
	}
	if r, ok := d.ApexClassDetail[id]; ok {
		return r
	}
	target := alias
	if target == "" {
		target = d.username
	}
	r := &Resource[sf.ApexClassDetail]{
		Scope: d.username, Key: "apex_class:" + id, TTL: 0,
		Fetch: func() (sf.ApexClassDetail, error) {
			return sf.GetApexClass(target, id)
		},
	}
	d.ApexClassDetail[id] = r
	return r
}

// bulkTagsForApexClasses pre-fetches tag bindings for a class list.
// Memoised on *orgData via the gutter cache; nil-returns when the
// gutter is hidden / store unavailable / org missing / list empty.
func (m Model) bulkTagsForApexClasses(items []sf.ApexClassRow) map[string][]devproject.Tag {
	return bulkTagsForItems(m, items, gutterDomainApexClass, devproject.KindApexClass,
		func(a sf.ApexClassRow) string { return a.ID })
}

// bulkTagsForApexTriggers — same pattern for the cross-sObject
// triggers list.
func (m Model) bulkTagsForApexTriggers(items []sf.TriggerRow) map[string][]devproject.Tag {
	return bulkTagsForItems(m, items, gutterDomainApexTrigger, devproject.KindApexTrigger,
		func(t sf.TriggerRow) string { return t.ID })
}

// bulkProjectsForApexClasses mirrors bulkTagsForApexClasses for the
// project gutter.
func (m Model) bulkProjectsForApexClasses(items []sf.ApexClassRow) map[string][]devproject.DevProject {
	return bulkProjectsForItems(m, items, gutterDomainApexClass, devproject.KindApexClass,
		func(a sf.ApexClassRow) string { return a.ID })
}

// bulkProjectsForApexTriggers mirrors bulkTagsForApexTriggers.
func (m Model) bulkProjectsForApexTriggers(items []sf.TriggerRow) map[string][]devproject.DevProject {
	return bulkProjectsForItems(m, items, gutterDomainApexTrigger, devproject.KindApexTrigger,
		func(t sf.TriggerRow) string { return t.ID })
}
