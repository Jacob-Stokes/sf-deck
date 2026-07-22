package ui

// Top chrome row: the persistent header bar at the top of the TUI.
//
// Rendered as two rows (filled bar + separator) so the body sits between
// two persistent chrome rows (this + the status bar at the bottom).
// Three zones, all driven by Model state:
//
//   LEFT    logo · org pill · view/breadcrumb
//   MIDDLE  live activity ("⟳ syncing X…" / red error)
//   RIGHT   tooling badge · API usage · clock
//
// Narrowing policy:
//   width < 110  → drop the sf-deck API count
//   width < 100  → drop clock
//   width < 95   → drop "refreshed Xm ago"
//   width < 90   → drop API bar
//   width < 70   → drop middle activity (errors still surface via banner)
//
// Left segment truncates last so ambient info stays visible.

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func (m Model) renderHeader() string {
	logoText := "◈"
	// Instance >1 gets a number badge after the diamond so users can
	// tell windows apart at a glance and external agents have a
	// stable visual identifier to ask the user about ("the sf-deck 2
	// window"). Instance 1 stays bare so the common single-window
	// case looks unchanged.
	if n := m.InstanceNumber(); n > 1 {
		logoText += fmt.Sprintf("%d", n)
	}
	logo := lipgloss.NewStyle().
		Foreground(theme.Blue).
		Bold(true).
		Render(logoText)
	title := lipgloss.NewStyle().
		Foreground(theme.Fg).
		Bold(true).
		Render("sf-deck")

	left := logo + " " + title
	if Demo {
		// Always-on badge so no frame of a recording (or a trial
		// session) can be mistaken for a real org connection.
		left += " " + lipgloss.NewStyle().
			Foreground(theme.Bg).
			Background(theme.Magenta).
			Bold(true).
			Render(" DEMO ")
	}
	if pill := m.headerOrgPill(); pill != "" {
		left += "  " + pill
	}
	// The loaded-project pill used to live here as a "📁 <name>" chip
	// next to the org pill. It moved into the right-rail nav cluster
	// (see renderRightNavPills in render_tabs.go) so the load
	// affordance + the keyboard shortcut + the visual indicator all
	// live in one place. Header stays focused on the org identity.
	if crumb := m.headerBreadcrumb(); crumb != "" {
		left += "  " + theme.Subtle.Render("·") + "  " + crumb
	}

	middle := m.headerActivity()
	right := m.headerRight()

	// Narrowing — drop the least essential bits first so ambient info
	// stays visible on tight terminals.
	if m.width < 110 {
		right = stripUsage(right)
	}
	if m.width < 100 {
		right = stripClock(right)
	}
	if m.width < 95 {
		right = stripFreshness(right)
	}
	if m.width < 90 {
		right = stripAPIBar(right)
	}
	if m.width < 70 {
		middle = ""
	}

	bar := composeBar(m.width, left, middle, right)
	style := lipgloss.NewStyle().
		Foreground(theme.Fg).
		Background(theme.Panel).
		Width(m.width).
		MaxWidth(m.width)
	// No separator row below the header — the tab bar (render_tabs.go)
	// sits directly underneath and provides its own visual boundary.
	return style.Render(bar)
}

// headerOrgPill renders the selected org as a status-colored pill:
// e.g. " ● acme-dev  DEV ".
func (m Model) headerOrgPill() string {
	if len(m.orgs) == 0 {
		if m.orgsRes.Busy() {
			return lipgloss.NewStyle().Foreground(theme.Muted).Render("loading orgs…")
		}
		return lipgloss.NewStyle().Foreground(theme.Red).Render("no orgs")
	}
	o := m.orgs[m.selected]
	dot := statusDot(o.Status)
	label := o.Display()
	if label == "" {
		label = "(no alias)"
	}
	name := lipgloss.NewStyle().Foreground(theme.Fg).Bold(true).Render(label)

	// Org-kind badge.
	var badgeFg, badgeBg color.Color
	var badgeText string
	switch {
	case o.IsScratch:
		badgeText = "SCR"
		badgeFg = theme.Bg
		badgeBg = theme.Cyan
	case o.IsSandbox:
		badgeText = "SBX"
		badgeFg = theme.Bg
		badgeBg = theme.Yellow
	case o.IsDevHub:
		badgeText = "HUB"
		badgeFg = theme.Bg
		badgeBg = theme.Magenta
	default:
		badgeText = "PROD"
		badgeFg = theme.Bg
		badgeBg = theme.Red
	}
	badge := lipgloss.NewStyle().
		Foreground(badgeFg).
		Background(badgeBg).
		Bold(true).
		Padding(0, 1).
		Render(badgeText)

	// Safety pill: never hidden, its color tells you at a glance
	// whether the current org will accept any writes (green = locked
	// down, red = wide open, amber/yellow for the middle tiers).
	lvl := m.safetyFor(o)
	safetyBadge := renderSafetyPill(lvl)

	// "0 orgs" — the focus-orgs key, advertised where the org
	// identity lives now that the footer hint moved out of the bar.
	// Keycap + dim label, same chip language as the footer, so the
	// bare key isn't cryptic ("what does 0 do?" — field feedback
	// 2026-06-12 on the label-less first version).
	keycap := lipgloss.NewStyle().
		Foreground(theme.Fg).
		Background(theme.Border).
		Bold(true).
		Render(" " + firstPretty(Keys.FocusOrgs) + " ")
	keyLabel := lipgloss.NewStyle().Foreground(theme.Muted).Render("orgs")

	// "; cmd" — command-menu key, paired with the orgs hint so the
	// two primary cross-cutting affordances live next to each other.
	// Previously this was a footer chip; moved here so the footer
	// has room for tab-local hints and the cmd menu reads as
	// associated with navigation, not utility chrome.
	cmdKeycap := lipgloss.NewStyle().
		Foreground(theme.Fg).
		Background(theme.Border).
		Bold(true).
		Render(" " + firstPretty(Keys.CommandPalette) + " ")
	cmdLabel := lipgloss.NewStyle().Foreground(theme.Muted).Render("cmd")

	return keycap + " " + keyLabel + "  " + cmdKeycap + " " + cmdLabel + "  " + dot + " " + name + " " + badge + " " + safetyBadge
}

// renderSafetyPill returns a colored inline badge for the given
// SafetyLevel. Matches the visual language of the kind badge (fg on bg
// with 1-col padding). Color is semantic: green = safe/read-only,
// red = no guardrails.
func renderSafetyPill(lvl settings.SafetyLevel) string {
	var fg, bg color.Color
	switch lvl {
	case settings.SafetyReadOnly:
		fg, bg = theme.Bg, theme.Green
	case settings.SafetyRecords:
		fg, bg = theme.Bg, theme.Yellow
	case settings.SafetyMetadata:
		fg, bg = theme.Bg, lipgloss.Color("208") // amber
	case settings.SafetyFull:
		fg, bg = theme.Bg, theme.Red
	default:
		fg, bg = theme.Fg, theme.Muted
	}
	return lipgloss.NewStyle().
		Foreground(fg).
		Background(bg).
		Bold(true).
		Padding(0, 1).
		Render(lvl.Label())
}

// headerBreadcrumb renders "view › segment › segment" with the view name
// colored magenta. Per-view logic reaches into whatever state carries
// the "current drill target" for that view.
func (m Model) headerBreadcrumb() string {
	view := lipgloss.NewStyle().
		Foreground(theme.Magenta).
		Bold(true).
		Render("/" + m.tab().String())
	segs := m.resolveBreadcrumb()
	out := view
	sep := theme.Subtle.Render(" › ")
	for _, s := range segs {
		out += sep + lipgloss.NewStyle().Foreground(theme.Fg).Render(s)
	}
	return out
}

// resolveBreadcrumb walks the spec resolver chain (subtab → tab) and
// returns the first non-nil Breadcrumb closure's segment list. Empty
// when no resolver applies — the header still renders the tab name.
func (m Model) resolveBreadcrumb() []string {
	spec, sub := m.activeSpec()
	if sub != nil && sub.Breadcrumb != nil {
		return sub.Breadcrumb(m)
	}
	if spec != nil && spec.Breadcrumb != nil {
		return spec.Breadcrumb(m)
	}
	return nil
}

// headerActivity renders the "what's happening right now" middle zone.
// Empty when nothing is active, so narrow terminals collapse gracefully.
func (m Model) headerActivity() string {
	var parts []string

	if len(m.orgs) > 0 {
		// d may be nil on a cold launch — the token bootstrap runs
		// before the org's data map is populated. currentTabSyncingLabel
		// tolerates a nil d (its BusyLabel resolvers guard) and still
		// returns the "getting new token…" note from the process-wide
		// gauge, so the slow first fetch is explained even pre-data.
		d := m.data[m.orgs[m.selected].Username]
		if syncing := currentTabSyncingLabel(m, m.tab(), d, m.soqlRunning); syncing != "" {
			parts = append(parts, lipgloss.NewStyle().
				Foreground(theme.Yellow).
				Render("⟳ "+syncing))
		}
	} else if m.soqlRunning {
		parts = append(parts, lipgloss.NewStyle().Foreground(theme.Yellow).Render("⟳ running query…"))
	}

	if err := currentTabError(m); err != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(theme.Red).Render("! "+err))
	}

	return strings.Join(parts, "   ")
}

// headerRight renders the ambient-info right zone.
func (m Model) headerRight() string {
	var parts []string

	if m.tab() == TabSOQL && m.soqlTooling {
		parts = append(parts, lipgloss.NewStyle().
			Foreground(theme.Magenta).
			Bold(true).
			Render("[tooling]"))
	}
	if m.tab() == TabSOQL && m.soqlBulk {
		parts = append(parts, lipgloss.NewStyle().
			Foreground(theme.Cyan).
			Bold(true).
			Render("[bulk]"))
	}

	if fresh := m.headerFreshness(); fresh != "" {
		parts = append(parts, fresh)
	}

	// Notifications indicator — sits just before the API summary on the
	// top line so unread org notifications are always in view.
	if notif := m.headerNotifications(); notif != "" {
		parts = append(parts, notif)
	}

	// Merged "API calls today" — sf-deck's own count plus the org's
	// daily quota figure, on one label. Previously two adjacent
	// chunks ("sf-deck API calls today: N" and "API used/max")
	// duplicated the word "API" and split the same concept across
	// two visual landmarks.
	if api := m.headerAPISummary(); api != "" {
		parts = append(parts, api)
	}

	// Wall-clock time used to live here. Dropped: the user has a
	// system clock already; the header is finite-width chrome and
	// time was the least useful thing in it.

	return strings.Join(parts, "   ")
}

// headerFreshness surfaces the current view's primary resource's
// fetch age ("loaded 3m ago"). Picked per-tab/subtab so drilling
// into a field shows the describe's age; drilling into a trigger
// shows the trigger body's age. Returns "" when the view doesn't
// have a single obvious primary resource yet (SOQL, projects, etc).
func (m Model) headerFreshness() string {
	t := m.primaryFetchedAt()
	if t.IsZero() {
		return ""
	}
	label := lipgloss.NewStyle().Foreground(theme.Muted).Render("refreshed")
	age := lipgloss.NewStyle().Foreground(theme.Cyan).Render(humanAge(t))
	return label + " " + age
}

// primaryFetchedAt returns the fetch time of whichever resource is
// the "main data on screen" for the current view. Returns zero time
// when the view doesn't map to a single resource or nothing's loaded.
//
// Drives off TabSpec.PrimaryFetchedAt (subtab variant takes precedence)
// — see internal/ui/tab_registry.go for the hook. Each tab registers a
// one-liner there instead of a case arm here.
func (m Model) primaryFetchedAt() time.Time {
	if len(m.orgs) == 0 {
		return time.Time{}
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil {
		return time.Time{}
	}
	spec, sub := m.activeSpec()
	if sub != nil && sub.PrimaryFetchedAt != nil {
		return sub.PrimaryFetchedAt(m, d)
	}
	if spec != nil && spec.PrimaryFetchedAt != nil {
		return spec.PrimaryFetchedAt(m, d)
	}
	return time.Time{}
}

// headerAPISummary combines sf-deck's own call count for today with
// the org's DailyApiRequests quota into one label:
//
//	"API calls today · sf-deck: 59 · org: 151.8k/15.6m"
//
// Either half is optional — when no usage tracker is installed the
// sf-deck count drops; when Home data hasn't loaded the org quota
// drops; both missing returns "". The org figure colour-codes by
// quota usage (green / yellow / red) so a glance still flags
// "you're burning through this org's daily budget."
func (m Model) headerAPISummary() string {
	if len(m.orgs) == 0 {
		return ""
	}
	muted := lipgloss.NewStyle().Foreground(theme.Muted)
	mine := ""
	if Usage != nil {
		// Count under BOTH the org's short alias and its username — calls
		// get recorded under either depending on the code path (e.g.
		// /compare records under the username), so summing both keys keeps
		// the counter honest.
		o := m.orgs[m.selected]
		if n := Usage.TodayForOrgKeys(o.Alias, o.Username); n > 0 {
			val := lipgloss.NewStyle().Foreground(theme.Cyan).Bold(true).Render(shortCount(n))
			mine = muted.Render("sf-deck: ") + val
		}
	}
	orgQuota := ""
	if d := m.data[m.orgs[m.selected].Username]; d != nil {
		for _, l := range d.Home.Value().KeyLimits {
			if l.Name != "DailyApiRequests" || l.Max == 0 {
				continue
			}
			used := l.Max - l.Remaining
			pct := float64(used) / float64(l.Max)
			fg := theme.Green
			switch {
			case pct > 0.8:
				fg = theme.Red
			case pct > 0.5:
				fg = theme.Yellow
			}
			val := lipgloss.NewStyle().Foreground(fg).
				Render(fmt.Sprintf("%s/%s", shortCount(used), shortCount(l.Max)))
			orgQuota = muted.Render("org: ") + val
			break
		}
	}
	if mine == "" && orgQuota == "" {
		return ""
	}
	prefix := muted.Render("API calls today")
	body := []string{prefix}
	if mine != "" {
		body = append(body, mine)
	}
	if orgQuota != "" {
		body = append(body, orgQuota)
	}
	return strings.Join(body, muted.Render(" · "))
}

// headerNotifications is the compact unread-notifications indicator on
// the top line (next to the API summary). "⌁ N" in the alert accent
// when the active org has unread notifications; empty otherwise — no
// org, count not loaded yet, or zero unread. We deliberately show
// nothing at zero rather than a muted "⌁ 0": the indicator is an alert,
// and a persistent zero-badge is just noise in the common case.
// Non-emoji glyph to match the app's clean unicode style.
func (m Model) headerNotifications() string {
	if len(m.orgs) == 0 {
		return ""
	}
	d := m.activeOrgData()
	if d == nil || d.Notifications.FetchedAt().IsZero() {
		return ""
	}
	unread := d.Notifications.Value().UnreadCount
	if unread > 0 {
		return lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true).
			Render(fmt.Sprintf("⌁ %d", unread))
	}
	return ""
}

// currentTabSyncingLabel returns a "syncing X…" label if the active
// view has an in-flight Resource refresh. Returns "" otherwise.
//
// Resolves through TabSpec.BusyLabel / SubtabSpec.BusyLabel — each
// tab declares which resources to surface. The legacy soqlRunning
// param is preserved on the signature for callers that don't yet
// have an org loaded; SOQL's resolver reads m.soqlRunning directly.
func currentTabSyncingLabel(m Model, _ Tab, d *orgData, soqlRunning bool) string {
	label := ""
	spec, sub := m.activeSpec()
	if sub != nil && sub.BusyLabel != nil {
		label = sub.BusyLabel(m, d)
	} else if spec != nil && spec.BusyLabel != nil {
		label = spec.BusyLabel(m, d)
	}
	_ = soqlRunning // SOQL's resolver reads m.soqlRunning directly.

	// When the slowness is a live token bootstrap (the ~2.5s `sf`
	// round-trip on redacting CLIs), say so — a slow refresh is then
	// explained rather than mysterious. Composes with the tab's own
	// label: "syncing flows… · getting new token" when both are live,
	// or just the token note when the fetch precedes any data sync.
	if sf.TokenFetchInFlight() {
		if label == "" {
			return "getting new token…"
		}
		return label + " · getting new token…"
	}
	return label
}

// currentTabError returns the most relevant error for the active
// view, or "" if none. Resolves through TabSpec.ErrorLabel /
// SubtabSpec.ErrorLabel.
func currentTabError(m Model) string {
	if len(m.orgs) == 0 {
		// The /home onboarding panel handles the "no orgs / sf
		// missing" messaging in-pane; suppressing the header error
		// here avoids duplicating the same message twice on screen.
		// Other tabs still show empty-state hints in their own bodies.
		return ""
	}
	d := m.data[m.orgs[m.selected].Username]
	spec, sub := m.activeSpec()
	if sub != nil && sub.ErrorLabel != nil {
		return sub.ErrorLabel(m, d)
	}
	if spec != nil && spec.ErrorLabel != nil {
		return spec.ErrorLabel(m, d)
	}
	return ""
}

// --- layout primitives (composeBar + narrow-strippers) ------------------

// composeBar lays out left / middle / right into exactly `width` visible
// columns (including 1-col gutters on each end). Truncates left first
// when content overflows so ambient info (right) and activity (middle)
// stay visible. Measured with ansi.StringWidth to avoid ANSI codes
// polluting the width count.
func composeBar(width int, left, middle, right string) string {
	const gutter = 1
	avail := width - 2*gutter
	if avail < 4 {
		return strings.Repeat(" ", width)
	}

	lw := ansi.StringWidth(left)
	mw := ansi.StringWidth(middle)
	rw := ansi.StringWidth(right)

	// The middle zone is the live activity/status ("syncing…", "getting
	// new token…") — more valuable than the breadcrumb when the terminal
	// is tight, since it explains what the app is doing right now. So when
	// things don't fit, truncate `left` (the breadcrumb) to make room for
	// `middle` rather than dropping `middle` outright. Only drop `middle`
	// when even a minimally-truncated left can't coexist with it. The
	// caller (renderHeader) already zeroes middle below width 70, so this
	// path only fires on wider terminals where a long breadcrumb — not raw
	// width — was crowding the status out.
	const minLeft = 4 // room for at least "x…" of breadcrumb
	if middle != "" && lw+mw+rw+2 > avail {
		// Can middle survive if we shrink left to its floor?
		if minLeft+mw+rw+2 <= avail {
			keep := avail - mw - rw - 2
			left = ansi.Truncate(left, keep, "…")
			lw = ansi.StringWidth(left)
		} else {
			// Not even a stub of left fits alongside middle — drop middle.
			middle = ""
			mw = 0
		}
	}
	if lw+rw+1 > avail {
		keep := avail - rw - 1
		if keep < minLeft {
			keep = minLeft
		}
		left = ansi.Truncate(left, keep, "…")
		lw = ansi.StringWidth(left)
	}

	var body string
	if middle == "" {
		gap := avail - lw - rw
		if gap < 1 {
			gap = 1
		}
		body = left + strings.Repeat(" ", gap) + right
	} else {
		leftGap := (avail - lw - mw - rw) / 2
		if leftGap < 1 {
			leftGap = 1
		}
		rightGap := avail - lw - leftGap - mw - rw
		if rightGap < 1 {
			rightGap = 1
		}
		body = left + strings.Repeat(" ", leftGap) + middle + strings.Repeat(" ", rightGap) + right
	}
	bw := ansi.StringWidth(body)
	if bw < avail {
		body += strings.Repeat(" ", avail-bw)
	} else if bw > avail {
		body = ansi.Truncate(body, avail, "")
	}
	return strings.Repeat(" ", gutter) + body + strings.Repeat(" ", gutter)
}

// stripAPIBar removes the API segment from the right zone when the
// terminal is too narrow to show it. Identified by literal "API "
// substring in the segment's visible text.
func stripAPIBar(right string) string {
	parts := strings.Split(right, "   ")
	out := parts[:0]
	for _, p := range parts {
		if strings.Contains(ansi.Strip(p), "API ") {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, "   ")
}

// stripUsage drops the "sf-deck NNN" count segment. Identified by the
// literal "sf-deck" substring.
func stripUsage(right string) string {
	parts := strings.Split(right, "   ")
	out := parts[:0]
	for _, p := range parts {
		if strings.Contains(ansi.Strip(p), "sf-deck") {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, "   ")
}

// stripFreshness drops the "refreshed Xm ago" segment. Identified
// by the "refreshed" label substring.
func stripFreshness(right string) string {
	parts := strings.Split(right, "   ")
	out := parts[:0]
	for _, p := range parts {
		if strings.Contains(ansi.Strip(p), "refreshed") {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, "   ")
}

// stripClock drops the clock (always the last segment).
func stripClock(right string) string {
	parts := strings.Split(right, "   ")
	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts[:len(parts)-1], "   ")
}

// shortCount formats large integers as 1.2k / 3.4m so "API 412/15k" fits.
func shortCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fm", float64(n)/1_000_000)
	case n >= 1_000:
		if n%1_000 == 0 {
			return fmt.Sprintf("%dk", n/1_000)
		}
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// --- header-freshness hooks (PrimaryFetchedAt) ---------------------------
//
// Extracted per the registry purity rule (closures max 5 lines). Each
// resolves the drilled-in entity's own resource, falling back to the
// list the row came from where that's the honest age.

func deployDetailFetchedAt(m Model, d *orgData) time.Time {
	if r, ok := d.DeployDetailMap[d.DeployCur]; ok && r != nil {
		return r.FetchedAt()
	}
	return d.Deploys.FetchedAt()
}

func apexDetailFetchedAt(m Model, d *orgData) time.Time {
	if r, ok := d.ApexClassDetail[d.ApexCur]; ok && r != nil {
		return r.FetchedAt()
	}
	return d.ApexClasses.FetchedAt()
}

// componentsDetailFetchedAt mirrors renderComponentsDetail's kind pick:
// presence in the AuraDetail map wins (LWCCur holds either kind's Id).
func componentsDetailFetchedAt(m Model, d *orgData) time.Time {
	if r, ok := d.AuraDetail[d.LWCCur]; ok && r != nil {
		return r.FetchedAt()
	}
	if r, ok := d.LWCDetail[d.LWCCur]; ok && r != nil {
		return r.FetchedAt()
	}
	return time.Time{}
}

func metaTypeDetailFetchedAt(m Model, d *orgData) time.Time {
	if r, ok := d.MetaTypeItems[d.MetaTypeCur]; ok && r != nil {
		return r.FetchedAt()
	}
	return time.Time{}
}

func userSessionsFetchedAt(m Model, d *orgData) time.Time {
	if r, ok := d.UserSessions[d.UserCur]; ok && r != nil {
		return r.FetchedAt()
	}
	return time.Time{}
}

func communityDetailFetchedAt(m Model, d *orgData) time.Time {
	if r, ok := d.CommunityPages[communityPageKey(d.CommunityCur)]; ok && r != nil {
		return r.FetchedAt()
	}
	return d.Community.FetchedAt()
}

// allUsersFetchedAt: the All-users subtab is chip-driven — the age is
// the ACTIVE chip's resource, matching what's actually on screen.
func allUsersFetchedAt(m Model, d *orgData) time.Time {
	if r, ok := d.ChipUsers[activeUsersChipID(d)]; ok && r != nil {
		return r.FetchedAt()
	}
	return time.Time{}
}
