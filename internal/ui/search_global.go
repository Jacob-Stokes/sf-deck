package ui

// Global search modal — a Force-Navigator-style "start typing to jump
// anywhere" overlay.
//
// Opens on ctrl+k (configurable via Keys.GlobalSearch). Indexes what's
// currently loaded for the active org: sObject list (always cached),
// per-sobject fields (cached after the user visits an object), flows,
// validation rules, record types, and triggers (cached after their
// subtabs load).
//
// Each match is typed so the result row shows what it is:
//   [object]      Account
//   [field]       Account.Name
//   [validation]  Account / Required_Region
//   [trigger]     Account / AccountTrigger
//   [flow]        MyFlow
//
// Enter drills to that entity's normal view (the same tab + drill-in
// path used outside of search).
//
// Tab on an object-scoped result pushes it as a search scope: results
// from then on are restricted to children of that scope. The scope
// chain renders as breadcrumbs above the input, e.g.
// "All > Account >". Esc peels one scope level; esc at root closes.

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/recent"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/resource"
)

// recentBoostMax is the peak recency bump applied to a search hit
// the moment a record is visited; decays linearly to 0 over the
// configured decay window (settings.RecentBoostDecayHours()).
//
// Kept hardcoded because it's the SHAPE of the boost, not the
// magnitude — users tune the decay window to push items out of the
// "fresh" tier sooner / later, but the peak height is just "1.5x
// the typical match score" and re-tuning that without re-tuning
// scoreEntry's match weights would be confusing.
const recentBoostMax = 1.5

// recentBoostFor returns a decayed boost — fresh visits get the
// full recentBoostMax, older ones decay linearly to 0 over
// `decayWindow`. Caller computes decayWindow from
// settings.RecentBoostDecayHours().
func recentBoostFor(visitedAt time.Time, decayWindow time.Duration) float64 {
	age := time.Since(visitedAt)
	if age <= 0 {
		return recentBoostMax
	}
	if age >= decayWindow {
		return 0
	}
	frac := 1.0 - float64(age)/float64(decayWindow)
	return recentBoostMax * frac
}

// globalSearchKind names one result category. Drives the badge label
// that appears on every row and gates which scope pushes are
// meaningful (only objects/flows can be scoped into).
type globalSearchKind string

const (
	gsKindObject      globalSearchKind = "object"
	gsKindField       globalSearchKind = "field"
	gsKindFlow        globalSearchKind = "flow"
	gsKindValidation  globalSearchKind = "validation"
	gsKindRecordType  globalSearchKind = "recordtype"
	gsKindTrigger     globalSearchKind = "trigger"
	gsKindApexClass   globalSearchKind = "apex_class"
	gsKindApexTrigger globalSearchKind = "apex_trigger"
	gsKindLWC         globalSearchKind = "lwc"
	gsKindAura        globalSearchKind = "aura"
	gsKindPermSet     globalSearchKind = "permset"
	gsKindPSG         globalSearchKind = "psg"
	gsKindProfile     globalSearchKind = "profile"
	gsKindQueue       globalSearchKind = "queue"
	gsKindPublicGroup globalSearchKind = "public_group"
	gsKindReport      globalSearchKind = "report"
	gsKindRecent      globalSearchKind = "recent"
	gsKindDevProject  globalSearchKind = "dev_project"
	gsKindTag         globalSearchKind = "tag"
	// gsKindRecord is the badge for hits coming from records mode
	// (live SOSL search across the org).  Sobject API name lives
	// in entry.Secondary so the modal row reads:
	//   [record]  Acme Corp                       Account
	gsKindRecord globalSearchKind = "record"
)

// globalSearchMode selects how globalSearchState.hits is populated:
// the local fuzzy index (metadata mode, default) or a live SOSL
// query against the active org (records mode).  Toggled by
// Keys.SearchToggleMode while the modal is open.  See
// docs/global-record-search-plan.md.
type globalSearchMode int

const (
	gsModeMetadata globalSearchMode = iota
	gsModeRecords
)

func (m globalSearchMode) String() string {
	switch m {
	case gsModeRecords:
		return "records"
	}
	return "metadata"
}

// globalSearchEntry is one searchable thing in the index. `Key` is
// what we match against — the caller folds lowercased name / label /
// apiname into it.  `Enter` is called when the user presses enter;
// it typically mutates the Model and returns a tea.Cmd to kick a
// navigation. `ScopeInto`, when non-nil, installs a new scope layer
// when the user presses Tab on this entry.
type globalSearchEntry struct {
	Kind      globalSearchKind
	Label     string // primary display label
	Secondary string // right-side dim hint (e.g. object name for a field)
	Key       string // pre-lowercased match target ("account name")
	Enter     func(m *Model) tea.Cmd
	ScopeInto *globalSearchScope // non-nil for drill-in-capable rows

	// Openable wraps the underlying sf domain type so ctrl+o on
	// the row pops the same open-menu the row would show when
	// cursored elsewhere in the app. The shape is defined ONCE per
	// type via sf.Openable.Targets() in internal/sf/openable.go;
	// global search just exposes the existing destination set
	// rather than declaring its own. Nil = ctrl+o is a no-op on
	// this row (synthetic entries like the "[recent]" header).
	Openable sf.Openable

	// RefKind + Ref identify the underlying domain item so the row
	// can pull tag / project bindings via the devproject store. Same
	// (Kind, Ref) shape that the tag picker + sidebar pills use, so
	// a Tag applied via t lands consistently on the search row.
	// Empty = no tag/project lookup (e.g. for a "[recent]" entry
	// whose ref is implicit).
	RefKind devproject.ItemKind
	Ref     string

	// Tags + Projects are the resolved bindings, populated once at
	// index-build time so per-row render is a no-store-call render
	// of pre-cached strings. Empty slices = unbound or store
	// unavailable.
	Tags     []devproject.Tag
	Projects []devproject.DevProject

	// Boost is a silent score-bump applied at rank time. Used by the
	// post-build pass to push entries the user is likely working on
	// right now (loaded-project members, recently-visited records)
	// to the top of relevant matches, without surfacing an explicit
	// "Recents" or "In project" header in the modal. Zero = no
	// boost; multiple sources stack.
	Boost float64
}

// globalSearchScope describes one level of restriction applied to the
// search space. A chain of these narrows what the index emits.
type globalSearchScope struct {
	Kind  globalSearchKind // object or flow, today
	Key   string           // the scope's identifier (sobject api name, flow dev name)
	Label string           // display label in the breadcrumb
}

// globalSearchHit is a scored result row ready for rendering.
type globalSearchHit struct {
	Entry globalSearchEntry
	Score float64
}

// globalSearchState is the active modal. Scope chain + input widget +
// scored hits + cursor. Nil on m.globalSearch = not visible.
type globalSearchState struct {
	input  textinput.Model
	scopes []globalSearchScope
	hits   []globalSearchHit
	cursor int
	// cached index for the current scope chain. Rebuilt when scopes
	// change; filtered on every keystroke.
	index []globalSearchEntry

	// urlMode is non-nil when the current input parses as a Salesforce
	// URL or bare Id. Enter then jumps to the parsed resource instead
	// of opening the cursored search hit. Recomputed on every keystroke
	// in handleGlobalSearchKey.
	urlMode *globalSearchURL

	// mode selects how hits is populated.  Default = metadata (local
	// index, instant).  Records mode = live SOSL against the active
	// org.  Toggled in-modal by Keys.SearchToggleMode.
	mode globalSearchMode

	// recordsLastTerm caches the last term records-mode queried with
	// so the modal doesn't re-fire SOSL when the user moves the
	// cursor or toggles back into records mode without changing the
	// term.  Empty when no records query has happened yet.
	recordsLastTerm string

	// recordsDebounceGen increments on every keystroke in records
	// mode.  Tick callbacks carry the generation they were scheduled
	// under; on fire they compare against this and discard themselves
	// if a newer keystroke has happened.  Lets fast typists avoid
	// firing one SOSL per character without the complexity of
	// cancellable timers.
	recordsDebounceGen uint64

	// recordsCache memoises (term → hits) for the current session.
	// Cursor moves and mode toggles within the same term reuse the
	// cached slice.  Cleared on modal close + on org switch (the
	// modal lives on Model so org changes wipe state implicitly).
	recordsCache map[string][]globalSearchHit

	// recordsLoading flags an in-flight SOSL fetch so the renderer
	// can show a "searching…" hint.  Cleared when the result lands.
	recordsLoading bool

	// recordsErr captures the most recent SOSL failure so the
	// renderer can surface it instead of silently showing zero
	// hits.  Cleared on each new fetch attempt.
	recordsErr error
}

// globalSearchURL is the resolved-paste state for the URL/Id detection
// path. nil → fuzzy search; non-nil → render a recognition pill and
// dispatch its Enter on enter.
type globalSearchURL struct {
	// Label is what the recognition pill shows ("RECORD · Account" or
	// "FLOW" or "UNKNOWN ID 0011…").
	Label string
	// Enter is the navigation closure. Non-nil when the parser
	// resolved to a kind sf-deck can navigate to. Nil when we
	// recognised a Salesforce URL but can't route it (e.g. setup
	// page we don't model) — pill still renders so the user knows
	// the URL was detected.
	Enter func(m *Model) tea.Cmd
}

// openGlobalSearch installs a fresh modal at the root scope.
//
// Also kicks the underlying Resources (sObjects + Flows for the
// active org) so a cold-launched user typing into ctrl+space sees
// real entries appear as the data lands. Without this, the index
// builds from whatever happens to be cached at modal-open time —
// often empty after a fresh start. The rebuild-on-resource-update
// hook (see globalSearchRebuildOnResourceUpdate) populates the
// modal as ensures resolve.
func (m *Model) openGlobalSearch() tea.Cmd {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 0
	resource.StyleInput(&ti)
	ti.Placeholder = "type to search…"
	ti.Focus()

	s := &globalSearchState{
		input:  ti,
		scopes: nil,
	}
	// Default mode is user-configurable ([ui.startup] global_search_mode);
	// built-in default is metadata (local index).
	if m.settings.StartupGlobalSearchRecordsMode() {
		s.mode = gsModeRecords
	}
	s.index = m.buildGlobalSearchIndex(s.scopes)
	s.hits = rankGlobalSearch(s.index, "")
	m.globalSearch = s

	// Warm every searchable resource for the active org. ctrl+f might
	// be the user's first action, so we can't assume the relevant tabs
	// have been visited. buildRootIndex indexes apex/LWC/aura/perms/
	// reports/etc. "when the cache is loaded" — without warming them
	// here, those kinds silently never appear unless the user happened
	// to open their tab first. The rebuild-on-resource-update hook
	// (update.go → rebuildGlobalSearchIndexIfActive) folds each set into
	// the open modal as its Ensure resolves, so results stream in.
	if len(m.orgs) == 0 {
		return nil
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	return tea.Batch(m.warmGlobalSearchResources(d)...)
}

// warmGlobalSearchResources returns the Ensure commands for every
// resource buildRootIndex reads. Kept as the single source of truth so
// the warm set can't drift from the index set — if buildRootIndex
// learns to index a new kind, add its Ensure here too.
func (m *Model) warmGlobalSearchResources(d *orgData) []tea.Cmd {
	if d == nil {
		return nil
	}
	return []tea.Cmd{
		d.SObjects.Ensure(m.cache),
		d.Flows.Ensure(m.cache),
		d.ApexClasses.Ensure(m.cache),
		d.ApexTriggersFlat.Ensure(m.cache),
		d.LWCBundles.Ensure(m.cache),
		d.AuraBundles.Ensure(m.cache),
		d.PermSets.Ensure(m.cache),
		d.PSGs.Ensure(m.cache),
		d.Profiles.Ensure(m.cache),
		d.PublicGroups.Ensure(m.cache),
		d.Queues.Ensure(m.cache),
		d.Reports.Ensure(m.cache),
	}
}

// renderGlobalSearch draws the modal — breadcrumb, input, result
// list. Layout mirrors the other overlays (info/edit/choice).
func (m Model) renderGlobalSearch() string {
	if m.globalSearch == nil {
		return ""
	}
	// Sized larger than the default modal: ~80% of terminal width
	// (clamped to 80..140) so result rows have room for kind badge
	// + name + sObject hint + URL on one line. Height tracks the
	// terminal so a 40-row terminal shows ~22 results, an 80-row
	// terminal shows ~50 — matching what users expect from a "find
	// anything" surface like VS Code's command palette.
	w := modalWidth(m.width, 80, 140)
	inner := w - 4
	s := m.globalSearch

	// Compute how many result rows to show. The non-result chrome is
	// title (1) + separator (1) + scope (1) + input (1) + blank (1)
	// + footer-blank (1) + hint (1) + border (2) + body-title (1) =
	// 10 rows of chrome the modal always emits regardless of hit
	// count.
	//
	// Modal height MUST fit the terminal: m.height - 2 leaves a
	// little breathing room for status bar / fudge. The 70%-of-
	// terminal soft cap is preserved as a UX nicety so the modal
	// doesn't dominate huge terminals (~100 rows), but it's bounded
	// from above by the hard "must fit" constraint either way.
	const chromeRows = 10
	hardMax := m.height - 2
	if hardMax < 12 {
		hardMax = 12 // degenerate tiny-terminal case
	}
	softMax := m.height * 7 / 10
	if softMax < 18 {
		softMax = 18
	}
	maxModalH := softMax
	if maxModalH > hardMax {
		maxModalH = hardMax
	}
	maxShown := maxModalH - chromeRows
	if maxShown < 5 {
		maxShown = 5
	}
	// Absolute cap to keep the list scannable on huge terminals;
	// users with > 40 visible rows should refine the query rather
	// than drown.
	if rowCap := m.settings.LayoutGlobalSearchRows(); maxShown > rowCap {
		maxShown = rowCap
	}

	// Column budgets for tabulated rows. The pills + projects bug
	// (rows soft-wrapping when content overflowed) was caused by
	// trying to fit pills inline with the label; tabulated columns
	// truncate within their budget, never wrap.
	tagsW := 28
	projectsW := 20
	if inner < 80 {
		// Tiny modal: shrink the right side, give label more room.
		tagsW = 16
		projectsW = 12
	}
	labelW := inner - tagsW - projectsW - 6 // 6 = column gaps
	if labelW < 30 {
		labelW = 30
	}

	var lines []string
	// Title sits in the body (above the mode label) so the modal
	// matches the other overlays' visual rhythm. The mode label + toggle
	// hint stays as a separate body line so it tracks state (search
	// count, error, etc.).
	titleStyle := lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true)
	lines = append(lines, titleStyle.Render("global search"))
	modeLabel := s.mode.String()
	if s.mode == gsModeRecords {
		if s.recordsLoading {
			modeLabel += " · searching…"
		} else if s.recordsErr != nil {
			modeLabel += " · error"
		} else if len(s.hits) > 0 {
			modeLabel += fmt.Sprintf(" · %d hits", len(s.hits))
		}
	}
	headerParts := []string{
		lipgloss.NewStyle().Foreground(theme.Muted).Render(modeLabel),
		lipgloss.NewStyle().Foreground(theme.FgDim).Render(
			"   " + firstPretty(Keys.SearchToggleMode) + " toggle mode"),
	}
	lines = append(lines, strings.Join(headerParts, ""))

	// Scope breadcrumb line — "All > Account >".  Append a "fetching…"
	// tag when any scope-in Ensure is in flight so the user knows more
	// results are on the way.
	scopeLine := renderGlobalSearchScope(s, inner)
	if m.globalSearchBusy() {
		scopeLine += "  " + lipgloss.NewStyle().
			Foreground(theme.Yellow).Italic(true).Render("fetching…")
	}
	lines = append(lines, scopeLine)

	// Input. Size to inner so cursor scrolls if the query gets long.
	inputW := inner - 4
	if inputW < 10 {
		inputW = 10
	}
	s.input.SetWidth(inputW)
	lines = append(lines, "  "+lipgloss.NewStyle().Foreground(theme.BorderHi).Render("> ")+s.input.View())
	// URL/Id recognition pill, or a blank gap when no detection. Same
	// height either way so the modal's chrome count stays fixed.
	if s.urlMode != nil {
		pillStyle := lipgloss.NewStyle().Foreground(theme.Bg).
			Background(theme.Cyan).Bold(true).Padding(0, 1)
		marker := pillStyle.Render(s.urlMode.Label)
		hint := "   ↵ go"
		if s.urlMode.Enter == nil {
			hint = "   not routable — refine"
		}
		lines = append(lines, "  "+marker+lipgloss.NewStyle().
			Foreground(theme.FgDim).Render(hint))
	} else {
		lines = append(lines, "")
	}

	// Render exactly maxShown result rows so the modal's height
	// doesn't jump as the user types and the result count changes.
	// Empty rows pad to the bottom; the "no matches" row sits in the
	// first slot so the user sees feedback at the same eye line.
	rendered := 0
	overflowLine := ""
	switch {
	case s.urlMode != nil:
		// URL/Id paste: the recognition pill above IS the result.
		// Show a subtle confirmation line in the results area instead
		// of "(no matches)" — pressing Enter is what the user wants
		// here, not refining the query.
		hint := "  press ↵ to open"
		if s.urlMode.Enter == nil {
			hint = "  recognised but not navigable — refine the query to fuzzy-search instead"
		}
		lines = append(lines, theme.Subtle.Render(hint))
		rendered = 1
	case s.mode == gsModeRecords && s.recordsErr != nil:
		// Records-mode SOSL failure — surface the SF error text so
		// the user can tell what went wrong (typically one of the
		// requested sObjects isn't present in this org).  Without
		// this the modal just shows "(no matches)" which is wrong
		// AND undiagnosable.
		errStyle := lipgloss.NewStyle().Foreground(theme.Red)
		lines = append(lines, "  "+errStyle.Render(s.recordsErr.Error()))
		rendered = 1
	case len(s.hits) == 0:
		lines = append(lines, theme.Subtle.Render("  (no matches)"))
		rendered = 1
	default:
		sel := s.cursor
		if sel < 0 {
			sel = 0
		}
		if sel >= len(s.hits) {
			sel = len(s.hits) - 1
		}
		total := len(s.hits)
		size := maxShown
		if total > size {
			// Reserve the bottom row for the overflow indicator so
			// the cursor never lands on it.
			size = maxShown - 1
		}
		if size > total {
			size = total
		}
		start := sel - size/3
		if start < 0 {
			start = 0
		}
		end := start + size
		if end > total {
			end = total
			start = end - size
			if start < 0 {
				start = 0
			}
		}
		for i := start; i < end; i++ {
			lines = append(lines, renderGlobalSearchRow(s.hits[i].Entry, i == sel, inner, labelW, tagsW, projectsW))
			rendered++
		}
		if total > size {
			overflowLine = fmt.Sprintf("  showing %d–%d of %d  ·  refine to narrow",
				start+1, end, total)
		}
	}
	// Pad blank rows so the modal height stays constant regardless
	// of result count. Slot count = maxShown total (results +
	// padding + optional overflow row).
	pad := maxShown - rendered
	if overflowLine != "" {
		pad--
	}
	for i := 0; i < pad; i++ {
		lines = append(lines, "")
	}
	if overflowLine != "" {
		lines = append(lines, theme.Subtle.Render(overflowLine))
	}

	lines = append(lines, "")
	hint := "↑/↓ move · ↵ open · tab scope-in · esc back"
	lines = append(lines,
		lipgloss.NewStyle().Foreground(theme.FgDim).Render(hint))

	// Double border distinguishes global search from the other
	// overlays (info/edit/choice) at a glance — it's the "search
	// anywhere" surface so it earns a stronger frame.
	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(theme.BorderHi).
		Padding(0, 1).
		Width(w - 2).
		Render(strings.Join(lines, "\n"))
}

// renderGlobalSearchScope draws the scope breadcrumb — "All > X > Y >".
// Dims "All" and separators so the active scope name reads.
func renderGlobalSearchScope(s *globalSearchState, inner int) string {
	dim := lipgloss.NewStyle().Foreground(theme.FgDim)
	scope := lipgloss.NewStyle().Foreground(theme.Cyan).Bold(true)
	parts := []string{dim.Render("  All")}
	for _, sc := range s.scopes {
		parts = append(parts, dim.Render(" > "))
		parts = append(parts, scope.Render(sc.Label))
	}
	return ansi.Truncate(strings.Join(parts, ""), inner, "…")
}

// renderGlobalSearchRow is one result-list row. Badge + label + dim
// secondary (e.g. the object name for a field). Tag + project pills
// flush right when the entry has any.
// renderGlobalSearchRow renders one result row as three fixed-width
// columns: label (left, flex), tags (middle, fixed budget), projects
// (right, fixed budget). Each column truncates independently so a
// long label doesn't push pills off the right edge, and lots of
// tags don't push projects to a new line. Total width = inner.
//
// Bug history: previously rendered pills inline with truncate-on-
// overflow logic, which broke when `ansi.StringWidth` of pills
// disagreed with `lipgloss.Width` of the rendered output by 1-2
// cells. The terminal then soft-wrapped pills onto their own line,
// breaking the modal box's alignment. Tabulated columns avoid the
// arithmetic entirely.
func renderGlobalSearchRow(e globalSearchEntry, selected bool, inner, labelW, tagsW, projectsW int) string {
	badge := renderKindBadge(e.Kind)
	labelStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	secondaryStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	prefix := "  "
	if selected {
		prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " "
		labelStyle = labelStyle.Bold(true)
	}

	// LEFT column: badge + label (+ secondary if it fits).
	leftCol := prefix + badge + " " + labelStyle.Render(e.Label)
	if e.Secondary != "" {
		leftCol += "  " + secondaryStyle.Render(e.Secondary)
	}
	leftCol = padOrTrunc(leftCol, labelW)

	// MIDDLE column: tag pills, space-joined, truncated to tagsW.
	tagCol := joinTruncated(tagPillSlice(e.Tags), tagsW)

	// RIGHT column: project pills, space-joined, truncated to
	// projectsW.
	projParts := make([]string, 0, len(e.Projects))
	for _, p := range e.Projects {
		projParts = append(projParts, lipgloss.NewStyle().
			Background(projectColorFor(p.ID)).
			Foreground(theme.Bg).
			Bold(true).
			Padding(0, 1).
			Render(p.Name))
	}
	projCol := joinTruncated(projParts, projectsW)

	// Total = labelW + 2 + tagsW + 2 + projectsW + (any rounding
	// slack) ≤ inner. Trim trailing whitespace defensively.
	row := leftCol + "  " + tagCol + "  " + projCol
	if w := lipgloss.Width(row); w > inner {
		// Last-resort truncation; should never fire if the column
		// budgets are right, but the cell-width measurer can disagree
		// by 1 cell on emoji.
		row = ansi.Truncate(row, inner, "…")
	}
	return row
}

// tagPillSlice renders the tag list as a slice of styled pill
// strings (one per tag). Distinct from renderTagPills (which
// space-joins them into a single string) — this caller wants the
// granularity to truncate the LAST pill cleanly.
func tagPillSlice(tags []devproject.Tag) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		out = append(out, renderTagPill(t))
	}
	return out
}

// joinTruncated joins parts with a single space and truncates the
// result to width cells with a "…" suffix. Returns the original
// joined string padded to width if it fits.
func joinTruncated(parts []string, width int) string {
	joined := strings.Join(parts, " ")
	w := lipgloss.Width(joined)
	if w <= width {
		// Pad on the right.
		if w < width {
			joined += strings.Repeat(" ", width-w)
		}
		return joined
	}
	// Drop parts from the end until it fits, with "…" suffix.
	for keep := len(parts) - 1; keep >= 0; keep-- {
		candidate := strings.Join(parts[:keep], " ")
		if keep < len(parts) {
			candidate = strings.TrimRight(candidate, " ") + " " +
				lipgloss.NewStyle().Foreground(theme.FgDim).Render("…")
		}
		if lipgloss.Width(candidate) <= width {
			cw := lipgloss.Width(candidate)
			if cw < width {
				candidate += strings.Repeat(" ", width-cw)
			}
			return candidate
		}
	}
	// Even zero parts + "…" doesn't fit — empty pad.
	return strings.Repeat(" ", width)
}

// padOrTrunc pads a styled string to exactly width cells, or
// truncates with a "…" suffix when too wide.
func padOrTrunc(s string, width int) string {
	w := lipgloss.Width(s)
	if w == width {
		return s
	}
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return ansi.Truncate(s, width, "…")
}

// renderKindBadge is a small color-coded "[kind]" tag. Fixed-width so
// labels align vertically across rows. Width fits the longest kind
// name we emit ("[public_group]" = 14 chars).
func renderKindBadge(k globalSearchKind) string {
	var c color.Color
	switch k {
	case gsKindObject:
		c = theme.Cyan
	case gsKindField:
		c = theme.Blue
	case gsKindFlow:
		c = theme.Magenta
	case gsKindValidation:
		c = theme.Yellow
	case gsKindRecordType:
		c = theme.Green
	case gsKindTrigger:
		c = theme.Orange
	case gsKindApexClass, gsKindApexTrigger:
		c = theme.Orange
	case gsKindLWC, gsKindAura:
		c = theme.Cyan
	case gsKindPermSet, gsKindPSG, gsKindProfile, gsKindQueue, gsKindPublicGroup:
		c = theme.Yellow
	case gsKindReport:
		c = theme.Green
	case gsKindRecent:
		c = theme.Muted
	case gsKindDevProject:
		c = theme.Blue
	case gsKindTag:
		c = theme.Magenta
	default:
		c = theme.Muted
	}
	label := fmt.Sprintf("%-15s", "["+string(k)+"]")
	return lipgloss.NewStyle().Foreground(c).Render(label)
}

// recordsSearchResultMsg carries the result of a records-mode SOSL
// fetch back to the modal.  Includes the term that was queried so
// late-arriving results don't overwrite hits from a newer term.
type recordsSearchResultMsg struct {
	term string
	hits []sf.GlobalSearchHit
	err  error
}

// recordsDebounceTickMsg is the tick callback for the records-mode
// debounce.  Carries the generation it was scheduled under so stale
// ticks (newer keystrokes since) discard themselves.
type recordsDebounceTickMsg struct {
	gen uint64
}

// recordsSearchDebounce is the wait between the last keystroke and
// the SOSL fire.  Long enough to coalesce fast typists' bursts,
// short enough that a deliberate keystroke feels immediate.
const recordsSearchDebounce = 200 * time.Millisecond

// handleGlobalSearchKey is the reducer for the modal while it's open.
// Special keys: esc (peel scope / close), enter (open), tab (scope
// in), up/down (cursor), Keys.SearchToggleMode (metadata/records
// flip). Everything else goes to the textinput.
func (m Model) handleGlobalSearchKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.globalSearch == nil {
		return m, nil
	}
	s := m.globalSearch
	if matches(msg.String(), Keys.SearchToggleMode) {
		if s.mode == gsModeMetadata {
			s.mode = gsModeRecords
		} else {
			s.mode = gsModeMetadata
		}
		s.cursor = 0
		// Re-resolve hits for the new mode.  Metadata mode uses the
		// existing local index; records mode either re-uses cached
		// hits for the current term or kicks a fresh SOSL.
		s.urlMode = nil
		if s.mode == gsModeMetadata {
			s.hits = rankGlobalSearch(s.index, s.input.Value())
			return m, nil
		}
		// Records mode: clear local hits so the modal doesn't
		// briefly show metadata hits while SOSL is in flight.
		s.hits = nil
		s.recordsErr = nil
		term := strings.TrimSpace(s.input.Value())
		if len(term) < 2 {
			// Empty / very short term: show recently-visited records
			// as warm-up suggestions.  Costs zero API calls and
			// matches user intent ("things I was just working on")
			// without waiting for them to type.
			s.hits = m.recordsWarmupHits()
			return m, nil
		}
		// Cache hit — reuse without another SOSL.
		if hits, ok := s.recordsCache[term]; ok {
			s.hits = hits
			s.recordsLastTerm = term
			s.recordsLoading = false
			return m, nil
		}
		// Toggling into records is an explicit user action — fire
		// the SOSL immediately rather than waiting for the
		// keystroke-debounce window.
		return m, m.startRecordsSearch(term)
	}
	switch msg.String() {
	case "esc":
		// Peel one scope level; at root, close.
		if len(s.scopes) == 0 {
			m.globalSearch = nil
			return m, nil
		}
		s.scopes = s.scopes[:len(s.scopes)-1]
		s.index = m.buildGlobalSearchIndex(s.scopes)
		s.hits = rankGlobalSearch(s.index, s.input.Value())
		s.cursor = 0
		return m, nil
	case "ctrl+c":
		m.globalSearch = nil
		return m, nil
	case "ctrl+o":
		// Surface the cursored hit's standard open menu without
		// leaving global search. The menu is built from the row's
		// Openable (populated at index-build time); rows whose
		// underlying type doesn't implement Openable are no-ops.
		//
		// Hide global search while the open menu is up so the user
		// sees a single focused overlay (esc on the open menu
		// restores the search state verbatim — input value, scope
		// chain, cursor position — so the user resumes where they
		// were).
		if len(s.hits) == 0 {
			return m, nil
		}
		entry := s.hits[s.cursor].Entry
		if entry.Openable == nil {
			return m, nil
		}
		o, ok := m.currentOrg()
		if !ok {
			return m, nil
		}
		targets := entry.Openable.Targets()
		if len(targets) == 0 {
			return m, nil
		}
		stashed := *s
		m.globalSearch = nil
		title := "Open · " + entry.Label
		m.openMenu = &openMenuState{
			title:               title,
			mode:                menuOpen,
			org:                 o,
			source:              entry.Openable,
			targets:             targets,
			cursor:              0,
			restoreGlobalSearch: &stashed,
		}
		return m, nil
	case "enter":
		// URL/Id pre-empts the fuzzy-search Enter path: if the input
		// parsed as a Salesforce URL or bare Id, jump straight to
		// the recognised resource. Falls through to the normal hit
		// path when there's no urlMode or its Enter is nil
		// (recognised-but-not-routable, e.g. a setup page sf-deck
		// doesn't model — let the user refine instead of silently
		// no-op'ing).
		if s.urlMode != nil && s.urlMode.Enter != nil {
			cmd := s.urlMode.Enter(&m)
			m.globalSearch = nil
			return m, cmd
		}
		if len(s.hits) == 0 {
			return m, nil
		}
		entry := s.hits[s.cursor].Entry
		m.globalSearch = nil
		if entry.Enter != nil {
			return m, entry.Enter(&m)
		}
		return m, nil
	case "tab":
		if len(s.hits) == 0 {
			return m, nil
		}
		entry := s.hits[s.cursor].Entry
		if entry.ScopeInto == nil {
			return m, nil
		}
		s.scopes = append(s.scopes, *entry.ScopeInto)
		s.input.SetValue("")
		s.index = m.buildGlobalSearchIndex(s.scopes)
		s.hits = rankGlobalSearch(s.index, "")
		s.cursor = 0
		// Lazily kick Ensure* for un-cached children so the index
		// fills without the user having to visit each subtab. Returns
		// cmds for Bubble Tea to schedule; applyResourceMsg will
		// re-rebuild our index when each resource lands.
		return m, m.kickScopeInFetches(*entry.ScopeInto)
	case "up":
		if s.cursor > 0 {
			s.cursor--
		}
		return m, nil
	case "down":
		if s.cursor < len(s.hits)-1 {
			s.cursor++
		}
		return m, nil
	}

	before := s.input.Value()
	newInput, cmd := s.input.Update(msg)
	s.input = newInput
	if s.input.Value() != before {
		s.cursor = 0
		// URL/Id detection runs in both modes — pasting a Salesforce
		// URL or bare Id is a valid shortcut even from records mode.
		s.urlMode = recognizeURL(s.input.Value())
		if s.mode == gsModeRecords {
			term := strings.TrimSpace(s.input.Value())
			if len(term) < 2 {
				// Too short to be useful for SOSL; fall back to the
				// warm-up suggestions (recently-visited records)
				// rather than an empty list.
				s.hits = m.recordsWarmupHits()
				s.recordsErr = nil
				s.recordsLoading = false
			} else if hits, ok := s.recordsCache[term]; ok {
				// Cache hit — same term was fetched recently in this
				// session.  Reuse the hits without another SOSL.
				s.hits = hits
				s.recordsLastTerm = term
				s.recordsLoading = false
				s.recordsErr = nil
			} else if term != s.recordsLastTerm {
				// New term — schedule a debounced tick.  Generation
				// bump invalidates any in-flight tick from earlier
				// keystrokes so only the most recent pause fires.
				s.recordsDebounceGen++
				s.recordsLoading = true
				s.recordsErr = nil
				gen := s.recordsDebounceGen
				cmd = tea.Batch(cmd, tea.Tick(recordsSearchDebounce, func(time.Time) tea.Msg {
					return recordsDebounceTickMsg{gen: gen}
				}))
			}
		} else {
			s.hits = rankGlobalSearch(s.index, s.input.Value())
		}
	}
	return m, cmd
}

// startRecordsSearch kicks an async SOSL fetch for the given term
// against the active org's curated target set + the user's recently-
// visited objects.  Returns a tea.Cmd that produces a
// recordsSearchResultMsg when the SOSL response lands.
//
// Sets s.recordsLoading + s.recordsLastTerm synchronously so the
// renderer can show a "searching…" hint and so the input-change
// handler doesn't re-fire the same term.
func (m *Model) startRecordsSearch(term string) tea.Cmd {
	if m.globalSearch == nil {
		return nil
	}
	s := m.globalSearch
	s.recordsLastTerm = term
	s.recordsLoading = true
	s.recordsErr = nil
	if len(m.orgs) == 0 {
		s.recordsLoading = false
		return nil
	}
	if Demo {
		// SOSL needs a live org; substring-scan the seeded records
		// instead so the records tier of global search demos too.
		return func() tea.Msg {
			return recordsSearchResultMsg{term: term, hits: demoRecordHits(term)}
		}
	}
	target := targetArg(m.orgs[m.selected])
	targets := m.globalSearchTargetsForActiveOrg()
	limit := m.settings.LimitGlobalSearch()
	return func() tea.Msg {
		hits, err := sf.GlobalSearchAlias(target, term, targets, limit)
		return recordsSearchResultMsg{term: term, hits: hits, err: err}
	}
}

// globalSearchTargetsForActiveOrg composes the SOSL target list from
// the curated default set + any sObjects the user has recently
// visited (pulled from the local visit log).  Caps the total at 30
// to stay safely under SF's RETURNING-clause limit.
//
// Critical filter: only includes sObjects that are actually present
// in this org's cached SObjects list.  Without that, a developer-
// edition org without (say) Quote or Asset gets a SOSL that fails
// the entire query with INVALID_TYPE rather than just dropping the
// missing target.  Curated defaults skew toward standard objects
// most orgs have, but Quote / Order / Asset / Contract / Campaign
// are gated by org edition + licences.
//
// If the SObjects list isn't loaded yet, falls back to the curated
// defaults unfiltered — the user will see the error if any target
// is missing, but that's better than blocking the modal entirely.
func (m Model) globalSearchTargetsForActiveOrg() []sf.GlobalSearchTarget {
	defaults := sf.DefaultGlobalSearchTargets()
	if len(m.orgs) == 0 {
		return defaults
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil {
		return defaults
	}
	// Build the org's present-sObjects set from the cached SObjects
	// describe.  Empty when not yet loaded — fall through to the
	// unfiltered defaults (best-effort).
	present := make(map[string]bool)
	for _, so := range d.SObjects.Value() {
		if so.Name != "" {
			present[so.Name] = true
		}
	}
	filter := func(t sf.GlobalSearchTarget) bool {
		if len(present) == 0 {
			return true // no describe yet; let everything through
		}
		return present[t.Sobject]
	}
	targets := make([]sf.GlobalSearchTarget, 0, len(defaults))
	seen := make(map[string]bool, len(defaults))
	for _, t := range defaults {
		if !filter(t) {
			continue
		}
		targets = append(targets, t)
		seen[t.Sobject] = true
	}
	// Walk the local visit log in MRU order, adding any sObjects we
	// haven't already covered AND that the org actually has.
	for _, e := range d.Recent {
		if e.Kind != RecentKindRecord || e.Type == "" {
			continue
		}
		if seen[e.Type] {
			continue
		}
		if !filter(sf.GlobalSearchTarget{Sobject: e.Type}) {
			continue
		}
		seen[e.Type] = true
		targets = append(targets, sf.GlobalSearchTarget{Sobject: e.Type})
		if len(targets) >= 30 {
			break
		}
	}
	return targets
}

// applyRecordsSearchResult merges a landed SOSL response into the
// modal state.  Stale results (term no longer matches the current
// input) are discarded so a fast typist's older queries don't
// overwrite newer ones.
func (m *Model) applyRecordsSearchResult(msg recordsSearchResultMsg) {
	if m.globalSearch == nil {
		return
	}
	s := m.globalSearch
	if s.mode != gsModeRecords {
		return
	}
	currentTerm := strings.TrimSpace(s.input.Value())
	if msg.term != currentTerm {
		return // user has moved on
	}
	s.recordsLoading = false
	s.recordsErr = msg.err
	if msg.err != nil {
		s.hits = nil
		return
	}
	hits := make([]globalSearchHit, 0, len(msg.hits))
	for _, h := range msg.hits {
		hits = append(hits, globalSearchHit{
			Entry: globalSearchEntry{
				Kind:      gsKindRecord,
				Label:     h.Name,
				Secondary: h.Sobject,
				Key:       strings.ToLower(h.Name + " " + h.Sobject),
				RefKind:   devproject.KindRecord,
				Ref:       h.ID,
				Enter:     openRecordHitFromSOSL(h.Sobject, h.ID, h.Name),
				Openable:  recordRefForSearchHit(h.Sobject, h.ID),
			},
		})
	}
	s.hits = hits
	if s.cursor >= len(hits) {
		s.cursor = 0
	}
	// Cache the result so subsequent toggles / cursor moves on the
	// same term don't re-fire SOSL.  Bounded implicitly by the
	// modal's session lifetime — closed-and-reopened modals start
	// fresh.
	if s.recordsCache == nil {
		s.recordsCache = make(map[string][]globalSearchHit)
	}
	s.recordsCache[msg.term] = hits
}

// applyRecordsDebounceTick fires the actual SOSL when the debounce
// window has elapsed without a newer keystroke.  Stale ticks (older
// generation than the current state) discard themselves.
func (m *Model) applyRecordsDebounceTick(msg recordsDebounceTickMsg) tea.Cmd {
	if m.globalSearch == nil {
		return nil
	}
	s := m.globalSearch
	if s.mode != gsModeRecords {
		return nil
	}
	if msg.gen != s.recordsDebounceGen {
		// User typed more characters since this tick was scheduled
		// — let the newer tick handle it.
		return nil
	}
	term := strings.TrimSpace(s.input.Value())
	if len(term) < 2 {
		s.recordsLoading = false
		return nil
	}
	if term == s.recordsLastTerm && !s.recordsLoading {
		// Already showing results for this term (rare — happens if
		// the user types and then un-types in <200ms).
		return nil
	}
	// Cache hit?  Skip the network entirely.
	if hits, ok := s.recordsCache[term]; ok {
		s.hits = hits
		s.recordsLastTerm = term
		s.recordsLoading = false
		return nil
	}
	return m.startRecordsSearch(term)
}

// openRecordHitFromSOSL builds the Enter closure for a records-mode
// hit.  Routes through drillByKind so Esc returns to the tab the
// user was on before opening the modal — same return-tab plumbing
// the metadata-mode entries already use.
func openRecordHitFromSOSL(sobject, id, name string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		cmd, _ := drillByKind(m, "record", id, sobject, name, m.tab())
		return cmd
	}
}

// recordRefForSearchHit wraps a (sObject, id) pair from global search
// as a sf.RecordRef so its standard open menu (Record detail / Edit /
// Inspector / list / Object Manager) becomes available via ctrl+o on
// the search row. Builds a synthetic attributes block since the SOSL
// hit doesn't carry one — RecordRef.Targets() consults
// attributes.type to identify the sObject.
func recordRefForSearchHit(sobject, id string) sf.RecordRef {
	return sf.RecordRef{
		Record: map[string]any{
			"Id": id,
			"attributes": map[string]any{
				"type": sobject,
			},
		},
	}
}

// recordsWarmupHits returns the user's local visit log (records
// only) shaped as globalSearchHit so records mode has something to
// show on first entry / empty input.  Zero network calls — purely
// local data the user has already seen.
//
// Returns an empty slice when no visits exist (fresh org); the
// renderer's empty-state hint covers that.
func (m Model) recordsWarmupHits() []globalSearchHit {
	if len(m.orgs) == 0 {
		return nil
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil {
		return nil
	}
	items := d.RecentList.Items()
	hits := make([]globalSearchHit, 0, len(items))
	for _, e := range items {
		if e.Kind != RecentKindRecord || e.ID == "" || e.Type == "" {
			continue
		}
		hits = append(hits, globalSearchHit{
			Entry: globalSearchEntry{
				Kind:      gsKindRecord,
				Label:     e.Name,
				Secondary: e.Type,
				Key:       strings.ToLower(e.Name + " " + e.Type),
				RefKind:   devproject.KindRecord,
				Ref:       e.ID,
				Enter:     openRecordHitFromSOSL(e.Type, e.ID, e.Name),
				Openable:  recordRefForSearchHit(e.Type, e.ID),
			},
		})
	}
	return hits
}

// rankGlobalSearch scores the index against the query. Empty query
// returns a stable lexical order so the user sees something on first
// open. Force-Navigator-style scoring: substring match per query
// token, bonus for ordered prefix match, tiebreak on label length.
func rankGlobalSearch(index []globalSearchEntry, query string) []globalSearchHit {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		// Lexical by Label, capped to keep the idle state snappy.
		entries := make([]globalSearchEntry, len(index))
		copy(entries, index)
		sort.SliceStable(entries, func(i, j int) bool {
			return strings.ToLower(entries[i].Label) < strings.ToLower(entries[j].Label)
		})
		hits := make([]globalSearchHit, 0, len(entries))
		for _, e := range entries {
			hits = append(hits, globalSearchHit{Entry: e, Score: 0})
		}
		return hits
	}
	terms := strings.Fields(q)
	var out []globalSearchHit
	for _, e := range index {
		score := scoreEntry(e, terms, q)
		if score <= 0 {
			continue
		}
		out = append(out, globalSearchHit{Entry: e, Score: score})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return len(out[i].Entry.Label) < len(out[j].Entry.Label)
	})
	return out
}

// scoreEntry returns a positive match score or 0 for no match. All
// tokens must substring-match. Bonus when the full query appears
// as a prefix of the key, plus a smaller bonus for appearing as a
// whole word. Prefer higher-value kinds (objects) via a small
// per-kind weight so they surface above their own children.
func scoreEntry(e globalSearchEntry, terms []string, fullQuery string) float64 {
	key := e.Key
	for _, t := range terms {
		if !strings.Contains(key, t) {
			return 0
		}
	}
	score := float64(len(terms))
	if strings.HasPrefix(key, fullQuery) {
		score += 2
	}
	// Whole-word bonus: " <query>" or "<query>" at start.
	if strings.HasPrefix(" "+key, " "+fullQuery) {
		score += 1
	}
	// Silent context boost — loaded project membership + recency.
	// See applySilentBoosts; entry carries the value pre-computed so
	// scoring stays a pure function on the entry.
	score += e.Boost
	// Kind weight — objects feel like a "front door" so rank them up.
	switch e.Kind {
	case gsKindObject:
		score += 0.5
	case gsKindFlow:
		score += 0.2
	}
	return score
}

// buildGlobalSearchIndex walks orgData caches to build the current
// search space under the given scope chain. Empty scope = everything;
// scope[0] Kind=Object restricts to that object's children (fields,
// validation rules, record types, triggers).
func (m Model) buildGlobalSearchIndex(scopes []globalSearchScope) []globalSearchEntry {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	var entries []globalSearchEntry
	// Root scope: sobject list + flows + any already-loaded children
	// across every object.
	if len(scopes) == 0 {
		entries = m.buildRootIndex(d)
	} else {
		// Scoped: dispatch by the current (outermost) scope's kind.
		top := scopes[len(scopes)-1]
		switch top.Kind {
		case gsKindObject:
			entries = m.buildObjectScopedIndex(d, top.Key)
		}
	}
	if len(entries) > 0 {
		decay := time.Duration(m.settings.RecentBoostDecayHours()) * time.Hour
		applySilentBoosts(d, m.devProjects, entries, m.settings.LoadedProjectBoost(), decay)
		hydrateTagsAndProjects(m, d, entries)
	}
	return entries
}

// hydrateTagsAndProjects bulk-resolves tag + project bindings for
// every entry that carries (RefKind, Ref). Done once at index-build
// time so the per-row render is a no-store-call paint of the
// pre-cached slices. Mirrors the bulk-fetch pattern used by list-
// table gutters.
func hydrateTagsAndProjects(m Model, d *orgData, entries []globalSearchEntry) {
	if m.devProjects == nil || d == nil {
		return
	}
	o, ok := m.currentOrg()
	if !ok {
		return
	}
	keys := make([]devproject.TagLookupKey, 0, len(entries))
	for _, e := range entries {
		if e.Ref == "" || e.RefKind == "" {
			continue
		}
		keys = append(keys, devproject.TagLookupKey{Kind: e.RefKind, Ref: e.Ref})
	}
	if len(keys) == 0 {
		return
	}
	tagMap, _ := m.devProjects.TagsForItems(o.Username, keys)
	projMap, _ := m.devProjects.ProjectsForItems(o.Username, keys)
	if len(tagMap) == 0 && len(projMap) == 0 {
		return
	}
	for i := range entries {
		e := &entries[i]
		if e.Ref == "" || e.RefKind == "" {
			continue
		}
		key := string(e.RefKind) + ":" + e.Ref
		if tags, ok := tagMap[key]; ok {
			e.Tags = tags
		}
		if projects, ok := projMap[key]; ok {
			e.Projects = projects
		}
	}
}

// buildRootIndex emits one entry per cached sobject + one per cached
// flow. Fields, validation rules, record types, and triggers are NOT
// emitted at the root because the signal-to-noise ratio tanks with
// thousands of fields across every object — users should scope into
// an object to search its children. Same UX as Force Navigator.
func (m Model) buildRootIndex(d *orgData) []globalSearchEntry {
	var out []globalSearchEntry
	for _, so := range d.SObjects.Value() {
		so := so
		apiName := so.Name
		label := so.Label
		if label == "" {
			label = apiName
		}
		secondary := ""
		if label != apiName {
			secondary = apiName
		}
		scope := &globalSearchScope{
			Kind: gsKindObject, Key: apiName, Label: apiName,
		}
		out = append(out, globalSearchEntry{
			Kind:      gsKindObject,
			RefKind:   devproject.KindSObject,
			Ref:       apiName,
			Label:     label,
			Secondary: secondary,
			Key:       strings.ToLower(apiName + " " + label),
			Enter:     openObjectCmd(apiName),
			ScopeInto: scope,
			Openable:  so,
		})
	}
	for _, f := range d.Flows.Value() {
		f := f
		label := f.MasterLabel
		if label == "" {
			label = f.DeveloperName
		}
		out = append(out, globalSearchEntry{
			Kind:      gsKindFlow,
			RefKind:   devproject.KindFlow,
			Ref:       f.DefinitionID,
			Label:     label,
			Secondary: f.DeveloperName,
			Key:       strings.ToLower(f.DeveloperName + " " + label),
			Enter:     openFlowCmd(f.DefinitionID),
			Openable:  f,
		})
	}
	// Apex Classes — searchable when the cache is loaded.
	for _, a := range d.ApexClasses.Value() {
		a := a
		out = append(out, globalSearchEntry{
			Kind:      gsKindApexClass,
			RefKind:   devproject.KindApexClass,
			Ref:       a.ID,
			Label:     a.Name,
			Secondary: dashIfEmpty(a.Status),
			Key:       strings.ToLower(a.Name + " apex class"),
			Enter:     openApexClassCmd(a.ID),
			Openable:  a,
		})
	}
	// Apex Triggers (cross-sObject flat list).
	for _, t := range d.ApexTriggersFlat.Value() {
		t := t
		out = append(out, globalSearchEntry{
			Kind:      gsKindApexTrigger,
			RefKind:   devproject.KindApexTrigger,
			Ref:       t.ID,
			Label:     t.Name,
			Secondary: t.Table,
			Key:       strings.ToLower(t.Name + " " + t.Table + " trigger"),
			Enter:     openApexTriggerCmd(t.Table, t.ID),
			// TriggerRow doesn't implement Openable; the row's
			// ctrl+o is a no-op until we add Targets() on it.
		})
	}
	// LWC bundles.
	for _, b := range d.LWCBundles.Value() {
		b := b
		label := b.MasterLabel
		if label == "" || label == b.DeveloperName {
			label = ""
		}
		out = append(out, globalSearchEntry{
			Kind:      gsKindLWC,
			RefKind:   devproject.KindLWC,
			Ref:       b.ID,
			Label:     b.DeveloperName,
			Secondary: label,
			Key:       strings.ToLower(b.DeveloperName + " " + label + " lwc"),
			Enter:     openLWCBundleCmd(b.ID),
			Openable:  b,
		})
	}
	// Aura bundles.
	for _, b := range d.AuraBundles.Value() {
		b := b
		label := b.MasterLabel
		if label == "" || label == b.DeveloperName {
			label = ""
		}
		out = append(out, globalSearchEntry{
			Kind:      gsKindAura,
			RefKind:   devproject.KindAura,
			Ref:       b.ID,
			Label:     b.DeveloperName,
			Secondary: label,
			Key:       strings.ToLower(b.DeveloperName + " " + label + " aura"),
			Enter:     openAuraBundleCmd(b.ID),
			Openable:  b,
		})
	}
	// Permission Sets.
	for _, p := range d.PermSets.Value() {
		p := p
		label := p.Label
		if label == "" {
			label = p.Name
		}
		out = append(out, globalSearchEntry{
			Kind:      gsKindPermSet,
			RefKind:   devproject.KindPermissionSet,
			Ref:       p.ID,
			Label:     p.Name,
			Secondary: label,
			Key:       strings.ToLower(p.Name + " " + label + " permset"),
			Enter:     openPermSetCmd(p.ID),
			Openable:  p,
		})
	}
	// PSGs.
	for _, g := range d.PSGs.Value() {
		g := g
		label := g.MasterLabel
		if label == "" {
			label = g.DeveloperName
		}
		out = append(out, globalSearchEntry{
			Kind:      gsKindPSG,
			RefKind:   devproject.KindPermissionSetGroup,
			Ref:       g.ID,
			Label:     g.DeveloperName,
			Secondary: label,
			Key:       strings.ToLower(g.DeveloperName + " " + label + " psg permission set group"),
			Enter:     openPSGCmd(g.ID),
			Openable:  g,
		})
	}
	// Profiles.
	for _, p := range d.Profiles.Value() {
		p := p
		out = append(out, globalSearchEntry{
			Kind:      gsKindProfile,
			RefKind:   devproject.KindProfile,
			Ref:       p.ID,
			Label:     p.Name,
			Secondary: dashIfEmpty(p.UserType),
			Key:       strings.ToLower(p.Name + " " + p.UserType + " profile"),
			Enter:     openProfileCmd(p.ID, p.PermissionSetID),
			Openable:  p,
		})
	}
	// Queues.
	for _, q := range d.Queues.Value() {
		q := q
		out = append(out, globalSearchEntry{
			Kind:      gsKindQueue,
			RefKind:   devproject.KindQueue,
			Ref:       q.ID,
			Label:     q.Name,
			Secondary: q.DeveloperName,
			Key:       strings.ToLower(q.Name + " " + q.DeveloperName + " queue"),
			Enter:     openQueueCmd(q.ID),
			Openable:  q,
		})
	}
	// Public Groups.
	for _, g := range d.PublicGroups.Value() {
		g := g
		out = append(out, globalSearchEntry{
			Kind:      gsKindPublicGroup,
			RefKind:   devproject.KindPublicGroup,
			Ref:       g.ID,
			Label:     g.Name,
			Secondary: g.DeveloperName,
			Key:       strings.ToLower(g.Name + " " + g.DeveloperName + " public group"),
			Enter:     openPublicGroupCmd(g.ID),
			Openable:  g,
		})
	}
	// Reports.
	for _, r := range d.Reports.Value() {
		r := r
		secondary := r.FolderName
		if secondary == "" {
			secondary = r.Format
		}
		out = append(out, globalSearchEntry{
			Kind:      gsKindReport,
			RefKind:   devproject.KindReport,
			Ref:       r.ID,
			Label:     r.Name,
			Secondary: secondary,
			Key:       strings.ToLower(r.Name + " " + secondary + " report"),
			Enter:     openReportCmd(r.ID),
			Openable:  r,
		})
	}
	// Recently-visited records used to live here as [recent]
	// suggestions on the metadata index.  Records mode (ctrl+r in
	// the modal) is the canonical surface for recently-visited
	// records now — see docs/global-record-search-plan.md — so they
	// no longer pollute metadata mode results.
	//
	// Dev Projects (global — store-backed, not per-org).
	if m.devProjects != nil {
		if projects, err := m.devProjects.ListDevProjects(); err == nil {
			for _, p := range projects {
				p := p
				out = append(out, globalSearchEntry{
					Kind:      gsKindDevProject,
					Label:     p.Name,
					Secondary: p.Description,
					Key:       strings.ToLower(p.Name + " " + p.Description + " dev project"),
					Enter:     openDevProjectCmd(p.ID),
				})
			}
		}
		// Tags.
		if tags, err := m.devProjects.ListTags(); err == nil {
			for _, t := range tags {
				t := t
				secondary := t.Color
				if t.Icon != "" {
					secondary = t.Icon + "  " + secondary
				}
				out = append(out, globalSearchEntry{
					Kind:      gsKindTag,
					Label:     "#" + t.Name,
					Secondary: secondary,
					Key:       strings.ToLower("#" + t.Name + " " + t.Name + " tag"),
					Enter:     openTagCmd(t.ID),
				})
			}
		}
	}
	return out
}

// buildObjectScopedIndex emits every cached child entity of the given
// sobject: fields (from the describe, if cached), validation rules,
// record types, triggers. Nothing is fetched here — a scope-in that
// has no cache yet simply returns nothing until the user loads it
// via the normal drill navigation. Future: lazy Tooling SOQL here.
func (m Model) buildObjectScopedIndex(d *orgData, sobject string) []globalSearchEntry {
	var out []globalSearchEntry

	if r, ok := d.Describes[sobject]; ok && !r.FetchedAt().IsZero() {
		for _, f := range r.Value().Fields {
			f := f
			label := f.Label
			if label == "" {
				label = f.Name
			}
			out = append(out, globalSearchEntry{
				Kind:      gsKindField,
				RefKind:   devproject.KindField,
				Ref:       sobject + "." + f.Name,
				Label:     sobject + "." + f.Name,
				Secondary: label,
				Key:       strings.ToLower(f.Name + " " + label),
				Enter:     openFieldCmd(sobject, f.Name),
				Openable:  sf.FieldRef{SObjectName: sobject, Field: f},
			})
		}
	}
	if r, ok := d.ValidationRules.Lists[sobject]; ok && !r.FetchedAt().IsZero() {
		for _, v := range r.Value() {
			v := v
			out = append(out, globalSearchEntry{
				Kind:      gsKindValidation,
				RefKind:   devproject.KindValidationRule,
				Ref:       v.ID,
				Label:     sobject + " / " + v.ValidationName,
				Secondary: v.Description,
				Key:       strings.ToLower(v.ValidationName + " " + v.Description),
				Enter:     openValidationCmd(sobject, v.ID),
			})
		}
	}
	if r, ok := d.RecordTypes.Lists[sobject]; ok && !r.FetchedAt().IsZero() {
		for _, rt := range r.Value() {
			rt := rt
			out = append(out, globalSearchEntry{
				Kind:      gsKindRecordType,
				RefKind:   devproject.KindRecordType,
				Ref:       rt.ID,
				Label:     sobject + " / " + rt.DeveloperName,
				Secondary: rt.Name,
				Key:       strings.ToLower(rt.DeveloperName + " " + rt.Name),
				Enter:     openRecordTypeCmd(sobject, rt.ID),
			})
		}
	}
	if r, ok := d.Triggers.Lists[sobject]; ok && !r.FetchedAt().IsZero() {
		for _, t := range r.Value() {
			t := t
			out = append(out, globalSearchEntry{
				Kind:      gsKindTrigger,
				RefKind:   devproject.KindApexTrigger,
				Ref:       t.ID,
				Label:     sobject + " / " + t.Name,
				Secondary: t.Status,
				Key:       strings.ToLower(t.Name + " " + t.Status),
				Enter:     openTriggerCmd(sobject, t.ID),
			})
		}
	}

	// Always include an "Open object" row at the top so users can
	// scope in then open without extra keystrokes.
	header := globalSearchEntry{
		Kind:      gsKindObject,
		RefKind:   devproject.KindSObject,
		Ref:       sobject,
		Label:     sobject,
		Secondary: "open object",
		Key:       strings.ToLower("open " + sobject),
		Enter:     openObjectCmd(sobject),
	}
	return append([]globalSearchEntry{header}, out...)
}

// kickScopeInFetches triggers async Ensure* calls for every child-
// entity type we index under a scope. For an Object scope that's
// describe (fields), validation rules, record types, triggers. The
// resource layer debounces — if something's already cached and
// fresh, the Ensure returns nil and nothing else happens.
//
// Each fetch returns a resourceUpdatedMsg that applyResourceMsg
// dispatches + rebuildGlobalSearchIndexIfActive re-ranks.
func (m Model) kickScopeInFetches(scope globalSearchScope) tea.Cmd {
	if scope.Kind != gsKindObject {
		return nil
	}
	o, ok := m.currentOrg()
	if !ok {
		return nil
	}
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	alias := orgAlias(o)
	sobject := scope.Key
	var cmds []tea.Cmd
	cmds = append(cmds, d.EnsureDescribe(alias, sobject).Ensure(m.cache))
	cmds = append(cmds, d.EnsureValidationRules(alias, sobject).Ensure(m.cache))
	cmds = append(cmds, d.EnsureRecordTypes(alias, sobject).Ensure(m.cache))
	cmds = append(cmds, d.EnsureTriggers(alias, sobject).Ensure(m.cache))
	out := cmds[:0]
	for _, c := range cmds {
		if c != nil {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return tea.Batch(out...)
}

// rebuildGlobalSearchIndexIfActive is called from applyResourceMsg
// after any per-org resource lands. If the global-search modal is
// open, rebuild its (metadata) index so toggling back into metadata
// mode picks up new data without the user having to re-type.
//
// Critical: only the index gets rebuilt unconditionally; s.hits is
// only re-ranked when METADATA mode is active.  Records mode owns
// its own hits via SOSL responses; clobbering them every time a
// background resource fetch lands made records-mode flash for one
// frame then revert to metadata hits — the bug this docstring exists
// to prevent recurring.
// globalSearchIndexKeys is the set of resource keys the metadata search
// index actually reads (see buildRootIndex). A resourceUpdatedMsg for
// any OTHER key (home, deploys, packages, labels, …) can't change the
// index, so rebuilding on it is wasted work — and during cold-start
// warm-up ~12 resources land one-by-one, so the un-gated rebuild caused
// visible jank when the search modal was open. Gate on this set.
var globalSearchIndexKeys = map[string]bool{
	"sobjects_v5": true, "flows_v2": true, "apex_classes_v2": true,
	"apex_triggers_flat_v2": true, "lwc_bundles_v2": true, "aura_bundles_v2": true,
	"permsets": true, "profiles_v2": true, "public_groups_v2": true,
	"queues_v2": true, "reports": true,
}

// rebuildGlobalSearchIndexForKey rebuilds only when the landed resource
// feeds the index (or key=="" for an unconditional rebuild, e.g. on
// modal open / scope change).
func (m *Model) rebuildGlobalSearchIndexForKey(key string) {
	if key != "" && !globalSearchIndexKeys[key] {
		return
	}
	m.rebuildGlobalSearchIndexIfActive()
}

func (m *Model) rebuildGlobalSearchIndexIfActive() {
	if m.globalSearch == nil {
		return
	}
	s := m.globalSearch
	s.index = m.buildGlobalSearchIndex(s.scopes)
	if s.mode != gsModeMetadata {
		return
	}
	s.hits = rankGlobalSearch(s.index, s.input.Value())
	if s.cursor >= len(s.hits) {
		if len(s.hits) == 0 {
			s.cursor = 0
		} else {
			s.cursor = len(s.hits) - 1
		}
	}
}

// globalSearchBusy reports whether any scope-in fetch is still in
// flight. Used by the render to show a small hint so the user knows
// more results are on the way.
func (m Model) globalSearchBusy() bool {
	if m.globalSearch == nil {
		return false
	}
	d := m.activeOrgData()
	if d == nil {
		return false
	}
	for _, sc := range m.globalSearch.scopes {
		if sc.Kind != gsKindObject {
			continue
		}
		s := sc.Key
		if r, ok := d.Describes[s]; ok && r.Busy() {
			return true
		}
		if r, ok := d.ValidationRules.Lists[s]; ok && r.Busy() {
			return true
		}
		if r, ok := d.RecordTypes.Lists[s]; ok && r.Busy() {
			return true
		}
		if r, ok := d.Triggers.Lists[s]; ok && r.Busy() {
			return true
		}
	}
	return false
}

// --- Enter-action helpers ----------------------------------------------
//
// Each returns a closure that performs the equivalent of manually
// navigating to the target. Reuses existing setTab / tab-change hooks
// so refresh + caching works just like a manual drill.

func openObjectCmd(apiName string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		d := m.activeOrgData()
		if d == nil {
			return nil
		}
		d.DescribeCur = apiName
		m.objectActionCur = 0
		m.setTab(TabObjectDetail)
		return m.onTabChanged()
	}
}

func openFieldCmd(apiName, fieldName string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		d := m.activeOrgData()
		if d == nil {
			return nil
		}
		d.DescribeCur = apiName
		d.FieldCur = fieldName
		m.fieldActionCur = 0
		m.setTab(TabFieldDetail)
		return tea.Batch(m.onTabChanged(), m.ensureFieldDescriptionCmd())
	}
}

func openValidationCmd(apiName, ruleID string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		d := m.activeOrgData()
		if d == nil {
			return nil
		}
		d.DescribeCur = apiName
		d.ValidationRules.DrillID = ruleID
		m.validationActionCur = 0
		m.setTab(TabValidationDetail)
		return m.onTabChanged()
	}
}

func openRecordTypeCmd(apiName, rtID string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		d := m.activeOrgData()
		if d == nil {
			return nil
		}
		d.DescribeCur = apiName
		d.RecordTypes.DrillID = rtID
		m.recordTypeActionCur = 0
		m.setTab(TabRecordTypeDetail)
		return m.onTabChanged()
	}
}

func openTriggerCmd(apiName, id string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		d := m.activeOrgData()
		if d == nil {
			return nil
		}
		return m.triggerDetailDrill(apiName, id, TabObjectDetail)
	}
}

func openFlowCmd(definitionID string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		d := m.activeOrgData()
		if d == nil {
			return nil
		}
		d.FlowCur = definitionID
		m.setTab(TabFlowDetail)
		return m.onTabChanged()
	}
}

// openApexClassCmd drills straight into the class detail (the code),
// matching what Enter on the /apex list does. The list cursor + subtab
// are positioned first so esc-back lands on the right row.
func openApexClassCmd(id string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		d := m.activeOrgData()
		if d == nil {
			return nil
		}
		for i, a := range d.ApexClassList.Items() {
			if a.ID == id {
				d.ApexClassList.SetCursor(i)
				break
			}
		}
		m.setApexSubtab(0) // Classes — esc-back stem
		return m.triggerOpenApexClass(id)
	}
}

// openApexTriggerCmd routes through the existing trigger-detail
// drill so opening behaves identically to clicking from the
// Triggers list.
func openApexTriggerCmd(sobject, id string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		if sobject == "" || id == "" {
			return nil
		}
		return m.triggerDetailDrill(sobject, id, TabApex)
	}
}

// openLWCBundleCmd drills straight into the bundle detail (the code),
// matching what Enter on the /components list does. The list cursor +
// subtab are positioned first so esc-back lands on the right row.
func openLWCBundleCmd(id string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		d := m.activeOrgData()
		if d == nil {
			return nil
		}
		for i, b := range d.LWCBundleList.Items() {
			if b.ID == id {
				d.LWCBundleList.SetCursor(i)
				break
			}
		}
		m.setComponentsSubtab(0) // LWC — esc-back stem
		return m.triggerOpenLWCBundle(id)
	}
}

// openAuraBundleCmd — same pattern, Aura drill.
func openAuraBundleCmd(id string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		d := m.activeOrgData()
		if d == nil {
			return nil
		}
		for i, b := range d.AuraBundleList.Items() {
			if b.ID == id {
				d.AuraBundleList.SetCursor(i)
				break
			}
		}
		m.setComponentsSubtab(1) // Aura — esc-back stem
		return m.triggerOpenAuraBundle(id)
	}
}

// openPermSetCmd / openPSGCmd / openProfileCmd / openQueueCmd /
// openPublicGroupCmd drill straight into the row's detail tab (perm
// parent / queue members / group members), matching what Enter on the
// /perms lists does. The subtab + cursor are positioned first so the
// stem list is right when navigating back.
func openPermSetCmd(id string) func(m *Model) tea.Cmd {
	return openPermsRowCmd(0, recent.KindPermSet, id, "", func(d *orgData) []string {
		ids := make([]string, 0, d.PermSetList.Len())
		for _, p := range d.PermSetList.Items() {
			ids = append(ids, p.ID)
		}
		return ids
	}, func(d *orgData, idx int) { d.PermSetList.SetCursor(idx) })
}

func openPSGCmd(id string) func(m *Model) tea.Cmd {
	return openPermsRowCmd(1, recent.KindPermSetGroup, id, "", func(d *orgData) []string {
		ids := make([]string, 0, d.PSGList.Len())
		for _, g := range d.PSGList.Items() {
			ids = append(ids, g.ID)
		}
		return ids
	}, func(d *orgData, idx int) { d.PSGList.SetCursor(idx) })
}

// permSetID is the profile's implicit PermissionSet Id — the perm
// parent detail reads FLS/object/system perms through it. Callers that
// don't have it (URL paste) pass "" and it back-fills from the loaded
// Profiles list at drill time.
func openProfileCmd(id, permSetID string) func(m *Model) tea.Cmd {
	return openPermsRowCmd(2, recent.KindProfile, id, permSetID, func(d *orgData) []string {
		ids := make([]string, 0, d.ProfileList.Len())
		for _, p := range d.ProfileList.Items() {
			ids = append(ids, p.ID)
		}
		return ids
	}, func(d *orgData, idx int) { d.ProfileList.SetCursor(idx) })
}

func openQueueCmd(id string) func(m *Model) tea.Cmd {
	return openPermsRowCmd(3, recent.KindQueue, id, "", func(d *orgData) []string {
		ids := make([]string, 0, d.QueueList.Len())
		for _, q := range d.QueueList.Items() {
			ids = append(ids, q.ID)
		}
		return ids
	}, func(d *orgData, idx int) { d.QueueList.SetCursor(idx) })
}

func openPublicGroupCmd(id string) func(m *Model) tea.Cmd {
	return openPermsRowCmd(4, recent.KindPublicGroup, id, "", func(d *orgData) []string {
		ids := make([]string, 0, d.PublicGroupList.Len())
		for _, g := range d.PublicGroupList.Items() {
			ids = append(ids, g.ID)
		}
		return ids
	}, func(d *orgData, idx int) { d.PublicGroupList.SetCursor(idx) })
}

// openPermsRowCmd is the shared shape for all five /perms helpers —
// positions the subtab + cursor (so the stem list is right on the way
// back), then drills into the row's detail via drillByKind, which also
// records the search-origin tab for esc-return. Unknown kind falls
// back to landing on the /perms list.
func openPermsRowCmd(subtabIdx int, kind, id, typeField string,
	idsOf func(*orgData) []string,
	setCursor func(*orgData, int)) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		d := m.activeOrgData()
		if d != nil {
			for i, candidate := range idsOf(d) {
				if candidate == id {
					setCursor(d, i)
					break
				}
			}
		}
		m.setPermsDashboardSubtab(subtabIdx)
		if kind == recent.KindProfile && typeField == "" && d != nil {
			typeField = profilePermSetID(d, id)
		}
		if cmd, ok := drillByKind(m, kind, id, typeField, "", m.tab()); ok {
			return cmd
		}
		m.setTab(TabPerms)
		return m.onTabChanged()
	}
}

// openReportCmd jumps to /reports and opens the cursored row's
// report-detail (the cached run). Falls back to /reports landing
// when the id isn't in cache yet.
// openReportCmd drills into the report detail (which runs the report)
// — same as Enter on the /reports row. Cursor positioned first for the
// way back.
func openReportCmd(id string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		d := m.activeOrgData()
		if d != nil {
			for i, r := range d.ReportList.Items() {
				if r.ID == id {
					d.ReportList.SetCursor(i)
					break
				}
			}
		}
		if cmd, ok := drillByKind(m, recent.KindReport, id, "", "", m.tab()); ok {
			return cmd
		}
		m.setTab(TabReports)
		return m.onTabChanged()
	}
}

// openDevProjectCmd jumps to TabDevProjectDetail for the project.
func openDevProjectCmd(projectID string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		m.setActiveDevProject(projectID)
		m.setTab(TabDevProjectDetail)
		return m.onTabChanged()
	}
}

// openTagCmd opens the tag manager and positions the cursor on the
// chosen tag. Useful for "I want to manage / rename / delete this
// tag" flow that starts from a search.
func openTagCmd(tagID int64) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		if m.devProjects != nil {
			tags, err := m.devProjects.ListTagsWithUsage()
			if err == nil {
				for i, t := range tags {
					if t.ID == tagID {
						m.tagsCursor = i
						break
					}
				}
			}
		}
		m.setTab(TabTags)
		return m.onTabChanged()
	}
}

// --- silent boost computation ---------------------------------------

// applySilentBoosts walks the index once and bumps the Boost field on
// any entry that should rank higher because of recent context. Two
// signals stack additively:
//
//   - loaded-project membership: any entry whose underlying item is in
//     the active org's loaded dev project gets +loadedProjectBoost.
//     Bumps "things I'm working on right now" to the top of relevant
//     matches without a visible "in project" header.
//   - recent visit: records that appear in the per-org recent-visits
//     log get a decayed boost — fresh visits up to ~recentBoostMax,
//     decaying linearly to 0 over recentDecayDur.
//
// Boosts are silent: no kind separator, no headers, no badges. The
// user only notices that "the thing I want is at the top" without
// having to think about why.
func applySilentBoosts(d *orgData, store *devproject.Store, entries []globalSearchEntry, projectBoost float64, decayWindow time.Duration) {
	scope := d.LoadedScope
	// Build a recency map keyed by "<sObject>:<Id>" for fast lookup.
	recency := map[string]float64{}
	for _, r := range d.RecentList.Items() {
		if r.Type == "" || r.ID == "" {
			continue
		}
		key := r.Type + ":" + r.ID
		boost := recentBoostFor(r.VisitedAt, decayWindow)
		if boost > recency[key] {
			recency[key] = boost
		}
	}
	for i := range entries {
		e := &entries[i]
		// Project-membership boost. The check varies by kind because
		// each entry caries a different identifying string in its
		// closure context — we recover it from Label/Secondary
		// where possible.
		switch e.Kind {
		case gsKindObject:
			if scope.Loaded() && scope.HasObject(e.Label) {
				e.Boost += projectBoost
			}
		case gsKindFlow:
			// Flow entries' Secondary is the DeveloperName; we don't
			// have the DefinitionID handy at boost time. Skip the
			// project boost for flows for now — the recency boost
			// still applies once flows land in /recent.
		case gsKindRecent:
			// Recents always get a recency boost; lookup is the
			// label/secondary combo. Use the entry's Label + Secondary
			// (sObject) to construct the key when available.
			//
			// (Recent items already carry the freshest-visit value
			// because they ARE the recent list, so the recency map
			// hit covers it.)
		}
		// Recency lookup — works for any record-shaped entry whose
		// Label is the record name and whose underlying ref is
		// "<sObject>:<Id>". We approximate via the Secondary +
		// (when available) the per-record key carried elsewhere.
		// For now, hit the recency map for kinds that map cleanly:
		switch e.Kind {
		case gsKindRecent:
			// Recent provider already encodes the visit; a generic
			// recency boost on top would double-count. Skip.
		}
		// All entries: if any recency key matches a token in their
		// search Key (broad heuristic), bump. Cheap and avoids
		// per-kind plumbing.
		for k, b := range recency {
			if b > 0 && strings.Contains(e.Key, strings.ToLower(k)) {
				e.Boost += b
				break
			}
		}
	}
	_ = store // reserved for future tag-based boosts
}

// profilePermSetID resolves a profile's implicit PermissionSet Id from
// the loaded Profiles list. "" when the list isn't loaded or the id is
// unknown — the perm parent detail's Overview still renders; the
// perms subtabs fill in once the id is known.
func profilePermSetID(d *orgData, profileID string) string {
	for _, p := range d.Profiles.Value() {
		if p.ID == profileID {
			return p.PermissionSetID
		}
	}
	return ""
}
