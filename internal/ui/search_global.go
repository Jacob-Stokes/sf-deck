package ui

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
	gsKindRecord      globalSearchKind = "record"
)

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

type globalSearchEntry struct {
	Kind      globalSearchKind
	Label     string // primary display label
	Secondary string // right-side dim hint (e.g. object name for a field)
	Key       string // pre-lowercased match target ("account name")
	Enter     func(m *Model) tea.Cmd
	ScopeInto *globalSearchScope // non-nil for drill-in-capable rows

	Openable sf.Openable

	RefKind devproject.ItemKind
	Ref     string

	Tags     []devproject.Tag
	Projects []devproject.DevProject

	Boost float64
}

type globalSearchScope struct {
	Kind  globalSearchKind // object or flow, today
	Key   string           // the scope's identifier (sobject api name, flow dev name)
	Label string           // display label in the breadcrumb
}

type globalSearchHit struct {
	Entry globalSearchEntry
	Score float64
}

type globalSearchState struct {
	input  textinput.Model
	scopes []globalSearchScope
	hits   []globalSearchHit
	cursor int
	index  []globalSearchEntry

	urlMode *globalSearchURL

	mode globalSearchMode

	recordsLastTerm string

	// recordsDebounceGen increments on every keystroke in records
	// mode.  Tick callbacks carry the generation they were scheduled
	// under; on fire they compare against this and discard themselves
	// if a newer keystroke has happened.  Lets fast typists avoid
	// firing one SOSL per character without the complexity of
	// cancellable timers.
	recordsDebounceGen uint64

	recordsCache map[string][]globalSearchHit

	recordsLoading bool

	recordsErr error
}

type globalSearchURL struct {
	Label string
	// Enter is the navigation closure. Non-nil when the parser
	// resolved to a kind sf-deck can navigate to. Nil when we
	// recognised a Salesforce URL but can't route it (e.g. setup
	// page we don't model) — pill still renders so the user knows
	// the URL was detected.
	Enter func(m *Model) tea.Cmd
}

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
	if rowCap := m.settings.LayoutGlobalSearchRows(); maxShown > rowCap {
		maxShown = rowCap
	}

	// Column budgets keep pills and projects within independently-truncated
	// regions so rows never soft-wrap.
	tagsW := 28
	projectsW := 20
	if inner < 80 {
		tagsW = 16
		projectsW = 12
	}
	labelW := inner - tagsW - projectsW - 6 // 6 = column gaps
	if labelW < 30 {
		labelW = 30
	}

	var lines []string
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

	scopeLine := renderGlobalSearchScope(s, inner)
	if m.globalSearchBusy() {
		scopeLine += "  " + lipgloss.NewStyle().
			Foreground(theme.Yellow).Italic(true).Render("fetching…")
	}
	lines = append(lines, scopeLine)

	inputW := inner - 4
	if inputW < 10 {
		inputW = 10
	}
	s.input.SetWidth(inputW)
	lines = append(lines, "  "+lipgloss.NewStyle().Foreground(theme.BorderHi).Render("> ")+s.input.View())
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

	rendered := 0
	overflowLine := ""
	switch {
	case s.urlMode != nil:
		hint := "  press ↵ to open"
		if s.urlMode.Enter == nil {
			hint = "  recognised but not navigable — refine the query to fuzzy-search instead"
		}
		lines = append(lines, theme.Subtle.Render(hint))
		rendered = 1
	case s.mode == gsModeRecords && s.recordsErr != nil:
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

	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(theme.BorderHi).
		Padding(0, 1).
		Width(w - 2).
		Render(strings.Join(lines, "\n"))
}

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
// Tabulated columns prevent width-measurement differences from soft-wrapping
// pills and disturbing modal alignment.
func renderGlobalSearchRow(e globalSearchEntry, selected bool, inner, labelW, tagsW, projectsW int) string {
	badge := renderKindBadge(e.Kind)
	labelStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	secondaryStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	prefix := "  "
	if selected {
		prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " "
		labelStyle = labelStyle.Bold(true)
	}

	leftCol := prefix + badge + " " + labelStyle.Render(e.Label)
	if e.Secondary != "" {
		leftCol += "  " + secondaryStyle.Render(e.Secondary)
	}
	leftCol = padOrTrunc(leftCol, labelW)

	tagCol := joinTruncated(tagPillSlice(e.Tags), tagsW)

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

	row := leftCol + "  " + tagCol + "  " + projCol
	if w := lipgloss.Width(row); w > inner {
		// Last-resort truncation; should never fire if the column
		// budgets are right, but the cell-width measurer can disagree
		// by 1 cell on emoji.
		row = ansi.Truncate(row, inner, "…")
	}
	return row
}

func tagPillSlice(tags []devproject.Tag) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		out = append(out, renderTagPill(t))
	}
	return out
}

func joinTruncated(parts []string, width int) string {
	joined := strings.Join(parts, " ")
	w := lipgloss.Width(joined)
	if w <= width {
		if w < width {
			joined += strings.Repeat(" ", width-w)
		}
		return joined
	}
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
	return strings.Repeat(" ", width)
}

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

type recordsSearchResultMsg struct {
	term string
	hits []sf.GlobalSearchHit
	err  error
}

type recordsDebounceTickMsg struct {
	gen uint64
}

// recordsSearchDebounce is the wait between the last keystroke and
// the SOSL fire.  Long enough to coalesce fast typists' bursts,
// short enough that a deliberate keystroke feels immediate.
const recordsSearchDebounce = 200 * time.Millisecond

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
		s.urlMode = nil
		if s.mode == gsModeMetadata {
			s.hits = rankGlobalSearch(s.index, s.input.Value())
			return m, nil
		}
		s.hits = nil
		s.recordsErr = nil
		term := strings.TrimSpace(s.input.Value())
		if len(term) < 2 {
			s.hits = m.recordsWarmupHits()
			return m, nil
		}
		if hits, ok := s.recordsCache[term]; ok {
			s.hits = hits
			s.recordsLastTerm = term
			s.recordsLoading = false
			return m, nil
		}
		return m, m.startRecordsSearch(term)
	}
	switch msg.String() {
	case "esc":
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
		s.urlMode = recognizeURL(s.input.Value())
		if s.mode == gsModeRecords {
			term := strings.TrimSpace(s.input.Value())
			if len(term) < 2 {
				s.hits = m.recordsWarmupHits()
				s.recordsErr = nil
				s.recordsLoading = false
			} else if hits, ok := s.recordsCache[term]; ok {
				s.hits = hits
				s.recordsLastTerm = term
				s.recordsLoading = false
				s.recordsErr = nil
			} else if term != s.recordsLastTerm {
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

func (m Model) globalSearchTargetsForActiveOrg() []sf.GlobalSearchTarget {
	defaults := sf.DefaultGlobalSearchTargets()
	if len(m.orgs) == 0 {
		return defaults
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil {
		return defaults
	}
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

func (m *Model) applyRecordsDebounceTick(msg recordsDebounceTickMsg) tea.Cmd {
	if m.globalSearch == nil {
		return nil
	}
	s := m.globalSearch
	if s.mode != gsModeRecords {
		return nil
	}
	if msg.gen != s.recordsDebounceGen {
		return nil
	}
	term := strings.TrimSpace(s.input.Value())
	if len(term) < 2 {
		s.recordsLoading = false
		return nil
	}
	if term == s.recordsLastTerm && !s.recordsLoading {
		return nil
	}
	if hits, ok := s.recordsCache[term]; ok {
		s.hits = hits
		s.recordsLastTerm = term
		s.recordsLoading = false
		return nil
	}
	return m.startRecordsSearch(term)
}

func openRecordHitFromSOSL(sobject, id, name string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		cmd, _ := drillByKind(m, "record", id, sobject, name, m.tab())
		return cmd
	}
}

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
	if strings.HasPrefix(" "+key, " "+fullQuery) {
		score += 1
	}
	score += e.Boost
	switch e.Kind {
	case gsKindObject:
		score += 0.5
	case gsKindFlow:
		score += 0.2
	}
	return score
}

func (m Model) buildGlobalSearchIndex(scopes []globalSearchScope) []globalSearchEntry {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	var entries []globalSearchEntry
	if len(scopes) == 0 {
		entries = m.buildRootIndex(d)
	} else {
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
		})
	}
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
// Critical: only the index is rebuilt unconditionally; s.hits is re-ranked
// only in metadata mode because records mode owns SOSL hits.
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

func openApexTriggerCmd(sobject, id string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		if sobject == "" || id == "" {
			return nil
		}
		return m.triggerDetailDrill(sobject, id, TabApex)
	}
}

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

func openDevProjectCmd(projectID string) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		m.setActiveDevProject(projectID)
		m.setTab(TabDevProjectDetail)
		return m.onTabChanged()
	}
}

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
		case gsKindRecent:
			// Recents always get a recency boost; lookup is the
			// label/secondary combo. Use the entry's Label + Secondary
			// (sObject) to construct the key when available.
			//
			// (Recent items already carry the freshest-visit value
			// because they ARE the recent list, so the recency map
			// hit covers it.)
		}
		switch e.Kind {
		case gsKindRecent:
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

func profilePermSetID(d *orgData, profileID string) string {
	for _, p := range d.Profiles.Value() {
		if p.ID == profileID {
			return p.PermissionSetID
		}
	}
	return ""
}
