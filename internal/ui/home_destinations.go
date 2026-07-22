package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// home_destinations.go — the "Lightning Destinations" catalog
// rendered below the SF-DECK logo on /home → Landing.
//
// Each entry is a curated Lightning Setup path that an admin
// reaches for daily. The catalog is grouped into named sections;
// each section + each item carries a single-letter shortcut for
// two-keystroke navigation:
//
//   <section letter>  → focuses the section, item letters go live
//   <item letter>     → opens that destination in Lightning
//
// Section letters are global on /home Landing; item letters scope
// to the focused section so reuse across sections is fine (s in
// ADMIN = Setup home, s in SECURITY = Setup Audit Trail).
//
// An item letter MAY equal another section's section letter (e.g.
// CODE's "m" Custom Metadata Types vs DEPLOY's "m" section). The
// dispatcher resolves this by precedence, not by forbidding it:
// while a section is focused, its own item letters win, so the
// committed-to item fires rather than teleporting to the colliding
// section. Switching sections is then esc-then-letter (or j/k).
// See onHomeDestinationsKey.

// homeDestination is one row in the Lightning destinations grid.
// Path is instance-relative; the open-menu pipeline prepends the
// org's instance URL via sf.FullURL.
type homeDestination struct {
	Label string
	Path  string
	Key   string // single lowercase letter for the in-section shortcut
}

// homeDestinationSection groups related destinations under a
// section header (ADMIN, DATA, etc.). Letter is the section-focus
// shortcut shown bracketed in the header.
type homeDestinationSection struct {
	Label   string
	Letter  string
	Entries []homeDestination
}

// homeDestinations is the canonical catalog. Order here is the
// render order on /home. Section letters audited against keymap
// defaults (Quit/Refresh/OpenDefault/YankDefault/SearchToggle/
// FocusOrgs/FocusBookmarks/Back/Activate/RecordEditField/etc) —
// see commands.go for the global key list. Letters chosen:
//
//	a  ADMIN
//	d  DATA
//	t  AUTOMATION    (a taken, t for auTomation)
//	u  USERS & PERMISSIONS
//	c  CODE
//	s  SECURITY & AUDIT
//	m  DEPLOY & MONITOR
//
// `f` reserved (FocusBookmarks legacy alias), `r` reserved
// (Refresh), `o`/`y` reserved (OpenDefault/YankDefault), `q`
// reserved (Quit), `n` reserved (search-next on most surfaces).
var homeDestinations = []homeDestinationSection{
	{
		Label:  "ADMIN",
		Letter: "a",
		Entries: []homeDestination{
			{Label: "Setup home", Path: "/lightning/setup/SetupOneHome/home", Key: "s"},
			{Label: "Object Manager", Path: "/lightning/setup/ObjectManager/home", Key: "o"},
			{Label: "Schema Builder", Path: "/lightning/setup/SchemaBuilder/home", Key: "b"},
			{Label: "System Overview", Path: "/lightning/setup/SystemOverview/home", Key: "v"},
		},
	},
	{
		Label:  "DATA",
		Letter: "d",
		Entries: []homeDestination{
			{Label: "Data Import Wizard", Path: "/lightning/setup/DataManagementDataImporter/home", Key: "i"},
			{Label: "Data Export", Path: "/lightning/setup/DataManagementExport/home", Key: "e"},
			{Label: "Mass Transfer", Path: "/lightning/setup/DataManagementMassTransfer/home", Key: "t"},
			{Label: "Mass Delete", Path: "/lightning/setup/DataManagementMassDelete/home", Key: "d"},
			{Label: "Storage Usage", Path: "/lightning/setup/CompanyResourceDisk/home", Key: "s"},
		},
	},
	{
		Label:  "AUTOMATION",
		Letter: "f", // not "t" — that's the global Tag picker
		Entries: []homeDestination{
			{Label: "All Flows", Path: "/lightning/setup/Flows/home", Key: "f"},
			{Label: "Process Builder", Path: "/lightning/setup/ProcessAutomation/home", Key: "p"},
			{Label: "Approval Processes", Path: "/lightning/setup/ApprovalProcesses/home", Key: "a"},
			{Label: "Scheduled Jobs", Path: "/lightning/setup/ScheduledJobs/home", Key: "s"},
			{Label: "Apex Jobs", Path: "/lightning/setup/AsyncApexJobs/home", Key: "j"},
		},
	},
	{
		Label:  "USERS & PERMISSIONS",
		Letter: "u",
		Entries: []homeDestination{
			{Label: "Users", Path: "/lightning/setup/ManageUsers/home", Key: "u"},
			{Label: "Profiles", Path: "/lightning/setup/EnhancedProfiles/home", Key: "p"},
			{Label: "Permission Sets", Path: "/lightning/setup/PermSets/home", Key: "s"},
			{Label: "Permission Set Groups", Path: "/lightning/setup/PermSetGroups/home", Key: "g"},
			{Label: "Roles", Path: "/lightning/setup/Roles/home", Key: "r"},
			{Label: "Queues", Path: "/lightning/setup/Queues/home", Key: "q"},
			{Label: "Public Groups", Path: "/lightning/setup/PublicGroups/home", Key: "b"},
		},
	},
	{
		Label:  "CODE",
		Letter: "x", // not "c" — that's the global Clear-committed-search
		Entries: []homeDestination{
			{Label: "Apex Classes", Path: "/lightning/setup/ApexClasses/home", Key: "a"},
			{Label: "Triggers", Path: "/lightning/setup/ApexTriggers/home", Key: "t"},
			{Label: "Lightning Components", Path: "/lightning/setup/LightningComponentBundles/home", Key: "l"},
			{Label: "Static Resources", Path: "/lightning/setup/StaticResources/home", Key: "r"},
			{Label: "Custom Settings", Path: "/lightning/setup/CustomSettings/home", Key: "s"},
			{Label: "Custom Metadata Types", Path: "/lightning/setup/CustomMetadata/home", Key: "m"},
			{Label: "Custom Labels", Path: "/lightning/setup/ExternalStrings/home", Key: "b"},
		},
	},
	{
		Label:  "SECURITY & AUDIT",
		Letter: "s",
		Entries: []homeDestination{
			{Label: "Login History", Path: "/lightning/setup/LoginHistory/home", Key: "l"},
			{Label: "Setup Audit Trail", Path: "/lightning/setup/SecurityEvents/home", Key: "a"},
			{Label: "Connected Apps", Path: "/lightning/setup/ConnectedApps/home", Key: "c"},
			{Label: "Named Credentials", Path: "/lightning/setup/NamedCredential/home", Key: "n"},
			{Label: "Auth Providers", Path: "/lightning/setup/AuthProviders/home", Key: "p"},
		},
	},
	{
		Label:  "DEPLOY & MONITOR",
		Letter: "m",
		Entries: []homeDestination{
			{Label: "Deployment Status", Path: "/lightning/setup/DeployStatus/home", Key: "d"},
			{Label: "Outbound Change Sets", Path: "/lightning/setup/OutboundChangeSet/home", Key: "o"},
			{Label: "Inbound Change Sets", Path: "/lightning/setup/InboundChangeSet/home", Key: "i"},
			{Label: "Installed Packages", Path: "/lightning/setup/ImportedPackage/home", Key: "p"},
			{Label: "Apex Debug Logs", Path: "/lightning/setup/ApexDebugLogs/home", Key: "l"},
			{Label: "Real-Time Event Monitor", Path: "/lightning/setup/EventManager/home", Key: "e"},
		},
	},
}

// homeDestinationSectionByLetter returns the section whose letter
// matches, or nil + false.
func homeDestinationSectionByLetter(letter string) (*homeDestinationSection, bool) {
	for i := range homeDestinations {
		if homeDestinations[i].Letter == letter {
			return &homeDestinations[i], true
		}
	}
	return nil, false
}

// homeDestinationByItemLetter returns the destination inside the
// given section whose Key matches.
func homeDestinationByItemLetter(section *homeDestinationSection, letter string) (*homeDestination, bool) {
	if section == nil {
		return nil, false
	}
	for i := range section.Entries {
		if section.Entries[i].Key == letter {
			return &section.Entries[i], true
		}
	}
	return nil, false
}

// onHomeDestinationsKey is the keystroke dispatcher for the /home
// Landing destinations grid. Returns (model, cmd, consumed). When
// not on TabHome → SubtabHomeLanding, returns consumed=false so the
// global handlers run.
//
// Keystroke regimes:
//   - Section letter (a/d/t/u/c/s/m) with no focus → focuses that
//     section, cursor lands on first entry, item letters go live.
//   - Section letter while already focused on a section → switch
//     focus to the new section.
//   - Item letter while a section is focused → open that
//     destination in Lightning.
//   - j / k / down / up → move cursor across the flat grid; focus
//     follows whichever section the cursor lands in.
//   - enter → open the cursored destination.
//   - esc → clear section focus + cursor (only when no other esc-
//     consumer is in scope; the global Back handler runs after us).
func (m Model) onHomeDestinationsKey(key string) (Model, tea.Cmd, bool) {
	if m.tab() != TabHome || m.homeSubtab() < 0 {
		return m, nil, false
	}
	subs := homeSubtabs()
	if m.homeSubtab() >= len(subs) || subs[m.homeSubtab()].ID != SubtabHomeLanding {
		return m, nil, false
	}
	switch key {
	case "esc":
		// Only consume esc when a section is focused — that's the
		// state esc can usefully un-do. Otherwise let the global
		// Back handler run so esc still exits the surface.
		if m.homeFocusedSectionLetter != "" {
			mm := m
			mm.homeFocusedSectionLetter = ""
			return mm, nil, true
		}
		return m, nil, false
	case "j", "down":
		mm := m
		mm.moveHomeDestCursor(1)
		return mm, nil, true
	case "k", "up":
		mm := m
		mm.moveHomeDestCursor(-1)
		return mm, nil, true
	case "enter":
		s, e, ok := homeDestSectionEntryAtIndex(m.homeDestCursor)
		if !ok || e == nil {
			return m, nil, false
		}
		_ = s
		return m, m.fireHomeDestination(e), true
	}
	// Single-letter keys: section letter or item letter (when a
	// section is focused). Reject anything that isn't a single
	// printable letter so multi-char keys (ctrl+x, etc.) fall
	// through to the global handler.
	if len(key) != 1 {
		return m, nil, false
	}
	c := key[0]
	if c < 'a' || c > 'z' {
		return m, nil, false
	}
	// Item letters of the FOCUSED section win first. Many item
	// letters collide with some OTHER section's section letter
	// (e.g. CODE's "m" Custom Metadata Types vs DEPLOY's "m"
	// section). The user has already committed to a section, so
	// the in-section item is the intent; without this priority the
	// keystroke would teleport to the colliding section instead.
	// To switch sections, press esc first (or navigate with j/k),
	// then the section letter focuses the new section.
	if m.homeFocusedSectionLetter != "" {
		if sec, ok := homeDestinationSectionByLetter(m.homeFocusedSectionLetter); ok {
			if e, ok := homeDestinationByItemLetter(sec, key); ok {
				return m, m.fireHomeDestination(e), true
			}
		}
	}
	// Otherwise treat a section letter as section-focus. This is
	// the only regime when no section is focused, and the
	// non-colliding fallback once one is (a letter that isn't an
	// item of the focused section can still hop to its section).
	if sec, ok := homeDestinationSectionByLetter(key); ok {
		mm := m
		mm.homeFocusedSectionLetter = sec.Letter
		mm.homeDestCursor = homeDestFlatIndex(homeDestSectionIndex(sec.Letter), 0)
		return mm, nil, true
	}
	return m, nil, false
}

// syncHomeDestFocus updates homeFocusedSectionLetter to match the
// section the cursor currently sits in. Called after j/k movement
// so the focused section visual highlight follows the cursor.
func (m *Model) syncHomeDestFocus() {
	s, _, ok := homeDestSectionEntryAtIndex(m.homeDestCursor)
	if !ok || s == nil {
		return
	}
	m.homeFocusedSectionLetter = s.Letter
}

// moveHomeDestCursor moves the Landing destinations cursor by delta,
// clamped to the catalog. Shared by the j/k key handler and the
// TabSpec.MoveCursor hook (so the scroll wheel + arrow keys + j/k all
// drive the same cursor and the view scrolls to follow it). No-op when
// the active subtab isn't Landing.
func (m *Model) moveHomeDestCursor(delta int) {
	subs := homeSubtabs()
	if m.homeSubtab() >= len(subs) || subs[m.homeSubtab()].ID != SubtabHomeLanding {
		return
	}
	n := homeDestTotalRows()
	if n == 0 {
		return
	}
	m.homeDestCursor = clampDelta(m.homeDestCursor, delta, n)
	m.syncHomeDestFocus()
}

// homeDestSectionIndex returns the position of the section with
// the given letter in the catalog, or -1.
func homeDestSectionIndex(letter string) int {
	for i, s := range homeDestinations {
		if s.Letter == letter {
			return i
		}
	}
	return -1
}

// fireHomeDestination dispatches an open-in-browser command for
// the chosen destination. Composes the full URL via the standard
// openInBrowserCmd pipeline so user browser settings are honored.
func (m Model) fireHomeDestination(e *homeDestination) tea.Cmd {
	if e == nil {
		return nil
	}
	o, ok := m.currentOrg()
	if !ok {
		return nil
	}
	t := sfOpenTargetFromHomeDest(*e)
	return m.openInBrowserCmd(o, t)
}

// sfOpenTargetFromHomeDest wraps a destination as a sf.OpenTarget
// so the existing browser-open pipeline handles it identically to
// every other OpenTarget. Avoids duplicating URL composition +
// browser-settings plumbing.
func sfOpenTargetFromHomeDest(e homeDestination) sf.OpenTarget {
	return sf.OpenTarget{ID: "home_dest_" + e.Key, Label: e.Label, Path: e.Path}
}
