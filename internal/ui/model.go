package ui

import (
	"errors"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/project"
	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
	qchipmigrate "github.com/Jacob-Stokes/sf-deck/internal/ui/qchip/migrate"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/resource"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/treechip"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/treechip/sources"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
	"github.com/Jacob-Stokes/sf-deck/internal/updatecheck"
)

type focus int

const (
	focusOrgs focus = iota
	focusMain
)

// searchState is a package-level alias for resource.SearchState.
// Use resource.SearchState for new code; this alias is kept for
// backward compatibility within the package.
type searchState = resource.SearchState

// Resource is a package-level alias for resource.Resource.
type Resource[T any] = resource.Resource[T]

// ListView is a package-level alias for resource.ListView.
type ListView[T any] = resource.ListView[T]

// SObjectChildren is a package-level alias for resource.SObjectChildren.
type SObjectChildren[Row any, Detail any] = resource.SObjectChildren[Row, Detail]

// resourceUpdatedMsg is a package-level alias for resource.UpdatedMsg.
type resourceUpdatedMsg = resource.UpdatedMsg

// ChipMode picks "where chips come from" on records-shaped surfaces:
// the sf-deck-defined catalogue, or the org's own Salesforce list views.
type ChipMode int

const (
	ChipModeLocal ChipMode = iota
	ChipModeSalesforce
)

// String renders a label for the chip-strip header.
func (m ChipMode) String() string {
	switch m {
	case ChipModeLocal:
		return "sf-deck"
	case ChipModeSalesforce:
		return "Salesforce"
	}
	return "?"
}

// SObjectFilter controls which sobjects show in the Objects view.
type SObjectFilter int

const (
	FilterManageable SObjectFilter = iota
	FilterAll
	FilterCustom
)

func (f SObjectFilter) String() string {
	switch f {
	case FilterManageable:
		return "manageable"
	case FilterAll:
		return "all"
	case FilterCustom:
		return "custom"
	}
	return "?"
}

// describeFieldState holds the field-browser list state for a given
// sobject's describe. Lives separately from the Resource so the user's
// cursor / sort / search survive a re-fetch.
//
// Migrated onto the shared list engine: List is a ListView[sf.Field]
// (cursor + search + ordering) and Table is the ListTableState (column
// sort / resize / horizontal scroll / pagination), so the Schema subtab
// gets the same creature comforts as /flows, /apex, etc. The fields
// themselves are re-Set from the describe on every render via
// syncFieldList.
type describeFieldState struct {
	List  ListView[sf.Field]
	Table uilayout.ListTableState
	// ChipID is the selected field-filter chip for this sobject (one of
	// qchip.FieldBuiltins' IDs). Empty = the default "all" chip. The
	// chip's predicate is applied to List via SetExtra each render.
	ChipID string
}

// orgData holds every piece of per-org state. Each Salesforce data set
// is a Resource[T]; browsable lists also get a ListView[T] wrapper so
// cursor + search behaviour is uniform across views.
//
// Adding a view that shows one more Salesforce thing is now:
//  1. Add a Resource[T] field on the matching orgData<X> sub-struct
//     in orgdata_groups.go.
//  2. Wire its Fetch closure in initOrgDataResources (or newOrgData
//     for ad-hoc state).
//  3. (Optional) wire a ListView with a Match predicate.
//  4. Call d.MyThing.Ensure(c) in ensureDataFor for the relevant view.
//  5. Render from d.MyThing.Value() / d.MyThingList.Filtered().
//
// All field access remains untouched — Go promotes embedded fields,
// so d.Records / d.DescribeCur / d.Home / etc. still resolve the
// same way they did when orgData was one flat struct.
type orgData struct {
	orgDataCore
	orgDataTopLists
	orgDataRecordsData
	orgDataMetadata
	orgDataPerms
	orgDataUsers
	orgDataHome
	orgDataRecent
	orgDataCode
	orgDataReports
	orgDataMeta
	orgDataDevProjects
	orgDataSOQLLibrary
	orgDataExecLibrary
	orgDataCompare
	orgDataNav
}

// gutterCacheState holds the per-orgData bulk tag/project results so
// successive renders within the same generation share one fetch.
type gutterCacheState struct {
	// tags / projects: keyed by domain discriminator → slice-pointer
	// → bulk map. The slice-pointer key catches "items slice was
	// replaced via Set" without needing to walk the items themselves.
	tags     map[string]gutterEntry[map[string][]devproject.Tag]
	projects map[string]gutterEntry[map[string][]devproject.DevProject]
}

// gutterEntry is one cached bulk-lookup result. itemsPtr is the
// header pointer of the items slice the cache was built against;
// generation is the store's mutation counter at fill time.
type gutterEntry[T any] struct {
	itemsPtr   uintptr
	generation int
	value      T
}

// newOrgData wires up the resources for a single org. Fetch closures
// capture the alias so every resource knows how to talk to sf.
// applySFConfig translates the [ui.api] settings (seconds / ms) into
// the sf package's Config (durations) and installs it. Each accessor
// returns its resolved value (override or built-in fallback), so this
// always sends a fully-populated Config.
func applySFConfig(st *settings.Settings) {
	if st == nil {
		return
	}
	sf.ApplyConfig(sf.Config{
		HTTPTimeout:     time.Duration(st.APIHTTPTimeoutSec()) * time.Second,
		CLITimeout:      time.Duration(st.APICLITimeoutSec()) * time.Second,
		RetrieveTimeout: time.Duration(st.APIRetrieveTimeoutSec()) * time.Second,
		DeployDeadline:  time.Duration(st.APIDeployTimeoutSec()) * time.Second,
		DeployPoll:      time.Duration(st.APIDeployPollMs()) * time.Millisecond,
		BulkPoll:        time.Duration(st.APIBulkPollMs()) * time.Millisecond,
		APIVersion:      st.APIVersionOverride(),
		FlowOpenVersion: st.FlowOpenVersion(),
	})
}

// resolveStartTab maps the user's [ui.startup] start_tab preference to
// a Tab, falling back to TabHome when unset or unknown. Drill-only tabs
// (which tabByID rejects) also fall back — you can't open the app
// directly on a per-record drill surface.
func resolveStartTab(st *settings.Settings) Tab {
	if st == nil {
		return TabHome
	}
	id := st.StartupStartTab()
	if id == "" || id == settings.StartupStartTabFallback {
		return TabHome
	}
	if t, ok := tabByID(id); ok {
		return t
	}
	return TabHome
}

func newOrgData(username, alias string, c *cache.Cache, st *settings.Settings) *orgData {
	// ttl resolves the effective TTL for a Resource key, honoring user
	// overrides in settings.toml while defaulting to the fallback the
	// caller passes. Hidden behind a closure so every Resource below
	// stays a one-liner.
	ttl := func(key string, fallback time.Duration) time.Duration {
		return st.CacheTTL(key, fallback)
	}
	// Promoted-field assignments. Each line writes into the matching
	// embedded sub-struct on orgData. Order follows the original flat
	// literal — no semantic change, just the syntactic shape required
	// once orgData became a composition of typed sub-structs (Go's
	// struct literal can't address promoted fields, but assignments
	// can — embedding promotes them to the parent's namespace).
	d := &orgData{}
	d.username = username
	d.target = alias
	d.cache = c
	d.settings = st
	d.Describes = map[string]*Resource[sf.SObjectDescribe]{}
	d.DescribeFields = map[string]*describeFieldState{}
	d.CustomObjectBaselines = map[string]*Resource[*sf.CustomObjectBaseline]{}
	d.CustomFieldIDs = map[string]string{}
	d.FieldDescriptions = map[string]string{}
	d.CustomObjectIDs = map[string]string{}
	d.FlowVersions = map[string]*Resource[[]sf.FlowVersion]{}
	d.ReportRuns = map[string]*Resource[sf.ReportRun]{}
	d.RecordDetails = map[string]*Resource[map[string]any]{}
	d.RecordReferenceNames = map[string]*Resource[map[string]string]{}
	d.RecordChildCounts = map[string]*Resource[map[string]int]{}
	d.ValidationRules = resource.NewSObjectChildren[sf.ValidationRuleRow, sf.ValidationRuleDetail](
		"validationrules:", "validationruledetail:",
		sf.ListValidationRules, sf.GetValidationRule,
	)
	d.RecordTypes = resource.NewSObjectChildren[sf.RecordTypeRow, sf.RecordTypeDetail](
		"recordtypes:", "recordtypedetail:",
		sf.ListRecordTypes, sf.GetRecordType,
	)
	d.Triggers = resource.NewSObjectChildren[sf.TriggerRow, sf.TriggerDetail](
		"triggers:", "triggerdetail:",
		sf.ListTriggers, sf.GetTrigger,
	)
	// Layouts have no drill-in detail — the editor is a Setup iframe
	// — so the detail fetch is a never-called stub.
	d.PageLayouts = resource.NewSObjectChildren[sf.PageLayoutRow, struct{}](
		"objlayouts:", "objlayoutdetail:",
		sf.ListObjectLayouts,
		func(alias, id string) (struct{}, error) {
			return struct{}{}, errors.New("layouts have no detail drill")
		},
	)
	// Object-scoped flows drill into the EXISTING /flow detail tab
	// (DurableId is a FlowDefinition id), so the detail half of the
	// children engine is unused here too.
	d.ObjectFlows = resource.NewSObjectChildren[sf.ObjectFlowRow, struct{}](
		"objflows:", "objflowdetail:",
		sf.ListObjectFlows,
		func(alias, id string) (struct{}, error) {
			return struct{}{}, errors.New("object flows drill via TabFlowDetail")
		},
	)
	d.LastTabInStem = map[Tab]Tab{}
	d.Records = map[string]*Resource[sf.RecordsList]{}
	d.ChipRecords = map[string]*Resource[sf.RecordsList]{}
	d.ListViewsPerSObject = map[string]*Resource[[]sf.ListView]{}
	d.RecentlyViewedPerSObject = map[string]*Resource[[]sf.RecentlyViewedRow]{}
	d.Networks = &Resource[[]sf.Network]{
		Scope:   username,
		Key:     "networks",
		TTL:     ttl("networks", 24*time.Hour),
		NoCache: true,
		Fetch:   func() ([]sf.Network, error) { return sf.ListNetworks(alias) },
	}
	d.CommunityUserByContact = map[string]string{}
	d.ListViewResults = map[string]*Resource[sf.ListViewResult]{}
	d.ListViewCur = map[string]string{}
	d.ChipMode = map[string]ChipMode{}
	d.Cursors = NewCursorStore()
	d.FLS = map[string]*Resource[[]sf.FieldPermissionRow]{}
	d.ObjectPerms = map[string]*Resource[[]sf.ObjectPermission]{}
	d.SystemPerms = map[string]*Resource[[]sf.SystemPermission]{}
	d.AssignedUsers = map[string]*Resource[[]sf.PermissionSetAssignment]{}
	d.GroupMembers = map[string]*Resource[[]sf.GroupMemberRow]{}
	d.GroupMemberList = map[string]ListView[sf.GroupMemberRow]{}
	d.GroupMemberState = map[string]*uilayout.ListTableState{}
	d.ObjPermSearch = map[string]*searchState{}
	d.SysPermSearch = map[string]*searchState{}
	d.Tab = resolveStartTab(st)
	// Continuous-scroll is the default browsing mode. Users can opt
	// surfaces into pagination via Shift+P (per-state, persists for
	// the session). Pagination is genuinely faster on big lists
	// (the per-row cache cuts list_table phase ~8x) but continuous
	// matches the macOS / browser expectation, so we leave the
	// choice to the user.
	// TTL defaults bumped session-length for near-immutable metadata.
	// Users still get manual refresh via `r`; short-TTL views that
	// represent "live" data (apex logs, deploys) keep short windows.
	initOrgDataResources(d, username, alias, st, ttl)
	installOrgDataSearchSpecs(d)
	return d
}

// installSearch wires both the substring matcher and the relevance
// scorer onto a ListView from one MatchSpec. Use this anywhere you'd
// otherwise call lv.SetMatch(uilayout.MakeMatcher(spec)) — the
// scorer is a free upgrade and gets ranking right ("Request__c"
// beats "bt_base__SCH_Schedule_Delta_Request__c" when the user
// types Request__c) when spec.Primary names the row's primary
// identifier field.
func installSearch[T any](lv *ListView[T], spec uilayout.MatchSpec[T]) {
	lv.SetMatch(uilayout.MakeMatcher(spec))
	lv.SetScorer(uilayout.MakeScorer(spec))
}

// EnsureRecords lazily wires a Resource[RecordsList] for the given
// sObject. TTL is short (1 min) because records change often; the cache
// is really there to avoid refetching on same-session re-visits.
// ttl is the orgData-side helper for resolving a Resource's effective
// TTL. Honors user overrides in settings.toml's [ui.cache.ttl] map,
// falls back to the supplied default. Mirrors the closure newOrgData
// uses for the startup-allocated Resources so all Resources resolve
// TTLs through the same path.
func (d *orgData) ttl(key string, fallback time.Duration) time.Duration {
	if d == nil || d.settings == nil {
		return fallback
	}
	return d.settings.CacheTTL(key, fallback)
}

// effectiveChipLimit returns the row cap a chip should pull when its
// Query.Limit is unset (= 0). Walks the same settings the rest of
// the chip plumbing reads, so future per-org overrides surface here
// uniformly. Always returns >= 1 — never zero.
func effectiveChipLimit(d *orgData) int {
	if d == nil || d.settings == nil {
		return settings.DefaultChipLimitFallback
	}
	return d.settings.DefaultChipLimit()
}

func (d *orgData) EnsureRecords(alias, sobject string) *Resource[sf.RecordsList] {
	return ensureKeyed(&d.Records, sobject, func() *Resource[sf.RecordsList] {
		return &Resource[sf.RecordsList]{
			Scope: d.username,
			Key:   "records:" + sobject,
			// First-load-only by default — fetch once on first visit of
			// the session, then never auto-refresh. Press `r` to force
			// a reload. Records are read-many, usually-stable while the
			// user is browsing; auto-refreshing burned ~1k API calls
			// per heavy day. User can lower this in settings.toml's
			// [ui.cache.ttl] under "records" if they want auto-refresh.
			TTL:     d.ttl("records", 24*time.Hour),
			NoCache: true, // records never hit disk; live data + privacy risk
			Fetch: func() (sf.RecordsList, error) {
				limit := d.settings.LimitRecentRecords()
				// Prefer the cached describe if we have it — saves a round-trip.
				if desc, ok := d.Describes[sobject]; ok && !desc.FetchedAt().IsZero() {
					return sf.RecentRecordsWithDescribe(alias, desc.Value(), limit)
				}
				return sf.RecentRecords(alias, sobject, limit)
			},
		}
	})
}

// EnsureChipRecords lazily wires a Resource per (sobject, chip-id)
// combination for records-shaped surfaces. The fetch renders the chip's
// query.Query as SOQL via qchip.ApplyToSOQL — same engine that runs
// client-side filters, just with the SOQL emitter on the output side.
// Each chip gets its own Resource so cycling between chips reuses
// prior fetches instead of clobbering them.
func (d *orgData) EnsureChipRecords(alias, sobject string, c qchip.Chip, subs qchip.Substitutions) *Resource[sf.RecordsList] {
	key := sobject + ":" + c.ID
	return ensureKeyed(&d.ChipRecords, key, func() *Resource[sf.RecordsList] {
		return &Resource[sf.RecordsList]{
			Scope: d.username,
			Key:   "chiprecords:" + key,
			// First-visit-of-session only; manual `r` to refresh.
			// See EnsureRecords for the rationale.
			TTL:     d.ttl("chip_records", 24*time.Hour),
			NoCache: true,
			Fetch: func() (sf.RecordsList, error) {
				// Default columns from the describe — the standard audit
				// set when the object has it:
				//   Id · Name · CreatedDate · CreatedBy · LastModifiedDate · LastModifiedBy
				// falling back to Id (+Name) alone. CreatedDate makes the
				// "Recently created" chip's sort axis visible; the created/
				// modified by columns pair with their dates. CreatedBy /
				// LastModifiedBy are universal relationships, gated on the
				// matching date field's presence as a cheap proxy for "this
				// object exposes standard audit fields."
				var defaultCols []string
				var hasName, hasModDate, hasCreatedDate bool
				if desc, ok := d.Describes[sobject]; ok && !desc.FetchedAt().IsZero() {
					v := desc.Value()
					present := map[string]bool{}
					for _, f := range v.Fields {
						present[f.Name] = true
						if f.Name == "LastModifiedDate" {
							hasModDate = true
						}
						if f.Name == "CreatedDate" {
							hasCreatedDate = true
						}
					}
					defaultCols = append(defaultCols, "Id")
					// Human-readable label column: standard objects use Name;
					// CustomMetadata (__mdt) has no Name — it uses
					// DeveloperName / MasterLabel / Label. Take the first
					// present so every shape shows a readable column, not
					// just the Id.
					for _, nameField := range []string{"Name", "MasterLabel", "Label", "DeveloperName"} {
						if present[nameField] {
							defaultCols = append(defaultCols, nameField)
							hasName = true
							// Mirror the query projection (sf/records.go):
							// MasterLabel/Label shapes also surface
							// DeveloperName — the API identity CMDT
							// developers actually key on.
							if nameField != "Name" && nameField != "DeveloperName" && present["DeveloperName"] {
								defaultCols = append(defaultCols, "DeveloperName")
							}
							break
						}
					}
					if hasCreatedDate {
						defaultCols = append(defaultCols, "CreatedDate")
						if present["CreatedById"] {
							defaultCols = append(defaultCols, "CreatedBy.Name")
						}
					}
					if hasModDate {
						defaultCols = append(defaultCols, "LastModifiedDate")
						if present["LastModifiedById"] {
							defaultCols = append(defaultCols, "LastModifiedBy.Name")
						}
					} else if present["SystemModstamp"] {
						// CustomMetadata (__mdt) has no LastModifiedDate /
						// CreatedDate / audit-by fields — but it DOES expose
						// SystemModstamp. Show it as the modified column so a
						// CMDT record isn't a bare Id + label.
						defaultCols = append(defaultCols, "SystemModstamp")
					}
				} else {
					defaultCols = []string{"Id", "Name"}
					hasName = true
				}
				// If the chip didn't pin its own columns, fill in our
				// defaults — keeps existing UX (Name + LastModifiedDate
				// columns) when the chip is just "Active" / "Recent".
				cc := c
				if len(cc.Query.Columns) == 0 {
					cc.Query.Columns = defaultCols
				}
				displayCols := append([]string(nil), cc.Query.Columns...)
				// Drop ORDER BY clauses targeting fields the sObject
				// doesn't accept as sortable. CustomMetadata records
				// describe LastModifiedDate but reject it in ORDER BY,
				// surfacing as INVALID_FIELD. Any chip's "Recent" /
				// "Mine, recent" / similar reaches this branch on the
				// first visit to a CMT — strip the unsortable column so
				// the SOQL shape stays valid (we trade ordering for
				// reachability; the user can resort client-side).
				if desc, ok := d.Describes[sobject]; ok && !desc.FetchedAt().IsZero() {
					v := desc.Value()
					sortable := map[string]bool{}
					for _, f := range v.Fields {
						if f.Sortable {
							sortable[f.Name] = true
						}
					}
					kept := cc.Query.OrderBy[:0]
					for _, ob := range cc.Query.OrderBy {
						if sortable[ob.Field] {
							kept = append(kept, ob)
						}
					}
					cc.Query.OrderBy = append([]query.OrderBy(nil), kept...)
				}
				// Three-state Limit:
				//
				//   0  → inherit settings.DefaultChipLimit (auto)
				//   -1 → unbounded (manual + blank input in wizard)
				//   >0 → hard cap pinned to that value
				//
				// Auto and pinned both emit a LIMIT clause; unbounded
				// drops the clause and lets QueryREST cursor-follow to
				// completion.
				cap := cc.Query.Limit
				switch {
				case cap > 0:
					cc.Query.Limit = cap
				case cap < 0:
					cc.Query.Limit = 0 // unbounded — strip LIMIT
				default:
					cc.Query.Limit = effectiveChipLimit(d)
				}
				cc.Query.Columns = recordsFetchColumns(displayCols)
				soql := qchip.ApplyToSOQL(cc, sobject, subs)
				// Pass cap=0 so QueryRESTCapped doesn't double-truncate;
				// the SOQL clause (or its absence) already controls bounds.
				rl, err := sf.RecordsForSOQL(alias, sobject, soql, displayCols, hasName, hasModDate, 0)
				rl.Query = soql
				return rl, err
			},
		}
	})
}

func recordsFetchColumns(display []string) []string {
	out := []string{"Id"}
	seen := map[string]bool{"Id": true}
	for _, c := range display {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

// EnsureChipUsers lazily wires a Resource per chip-id for /users ·
// All users. Each chip's predicate compiles to SOQL via
// qchip.ApplyToSOQL — server-side filtering means a chip like "System
// admins" returns every admin in the org, not just whoever was in
// the alphabetical-LIMIT slice. Mirrors EnsureChipRecords.
func (d *orgData) EnsureChipUsers(alias string, c qchip.Chip, subs qchip.Substitutions) *Resource[sf.UsersList] {
	return ensureKeyed(&d.ChipUsers, c.ID, func() *Resource[sf.UsersList] {
		return &Resource[sf.UsersList]{
			Scope:   d.username,
			Key:     "chipusers:" + c.ID,
			TTL:     d.ttl("chip_users", 5*time.Minute),
			NoCache: true,
			Fetch: func() (sf.UsersList, error) {
				cc := c
				// Always project the column set the table renders — chip
				// authors don't need to repeat them, and overriding here
				// keeps the SOQL stable across chip definitions.
				cc.Query.Columns = []string{
					"Id", "Name", "Username",
					"Profile.Name", "UserRole.Name",
					"LastLoginDate", "IsActive",
				}
				// Resolve the effective row cap: chip's explicit Limit
				// wins, otherwise inherit settings.DefaultChipLimit.
				// Three-state Limit (see EnsureChipRecords for the full
				// rationale): 0 = inherit, -1 = unbounded, >0 = pinned.
				cap := cc.Query.Limit
				switch {
				case cap > 0:
					cc.Query.Limit = cap
				case cap < 0:
					cc.Query.Limit = 0 // unbounded — strip LIMIT
				default:
					cc.Query.Limit = effectiveChipLimit(d)
				}
				soql := qchip.ApplyToSOQL(cc, "User", subs)
				return sf.UsersForSOQL(alias, soql, 0)
			},
		}
	})
}

// UsersListPtr returns the per-chip ListView wrapper for /users ·
// All users, lazily allocating. The ListView holds the cursor +
// search buffer per chip; each render syncs the underlying slice
// from the matching ChipUsers resource via SyncChipUsers.
func (d *orgData) UsersListPtr(chipID string) *ListView[sf.UserRow] {
	if d.ChipUsersList == nil {
		d.ChipUsersList = map[string]*ListView[sf.UserRow]{}
	}
	if lv, ok := d.ChipUsersList[chipID]; ok {
		return lv
	}
	lv := &ListView[sf.UserRow]{}
	lv.SetMatch(d.userMatch)
	lv.SetScorer(d.userScore)
	d.ChipUsersList[chipID] = lv
	return lv
}

// SyncChipUsers fans the active chip's User Resource into the
// matching ListView when the underlying slice has actually changed.
// ListView.Set resets the cursor to 0 unconditionally — calling it
// every render (which the renderer does) would silently snap j/k
// back to the top of the list each frame, making cursor movement
// look like it doesn't work at all. We compare the slice header
// (data pointer + len) and only re-Set when the resource published
// fresh data. Cursor + search state survive across renders of the
// same payload.
func (d *orgData) SyncChipUsers(chipID string) {
	if d.ChipUsers == nil {
		return
	}
	r, ok := d.ChipUsers[chipID]
	if !ok {
		return
	}
	rows := r.Value().Rows
	lv := d.UsersListPtr(chipID)
	cur := lv.Items()
	// Same backing slice? No-op. Comparing len + first-element
	// pointer is enough because the resource produces a new slice on
	// each successful fetch; cached fetches return the same slice
	// instance until the next refresh.
	if len(cur) == len(rows) {
		if len(cur) == 0 {
			return
		}
		if &cur[0] == &rows[0] {
			return
		}
	}
	lv.Set(rows)
}

// UsersTableStatePtr returns the per-chip ListTableState pointer for
// /users · All users, lazily allocating. Per-chip so column widths
// and zen flag survive chip switches.
func (d *orgData) UsersTableStatePtr(chipID string) *uilayout.ListTableState {
	if d.ChipUsersTableState == nil {
		d.ChipUsersTableState = map[string]*uilayout.ListTableState{}
	}
	if s, ok := d.ChipUsersTableState[chipID]; ok {
		return s
	}
	s := &uilayout.ListTableState{}
	d.ChipUsersTableState[chipID] = s
	return s
}

// RecordsSearchPtr returns the per-(sobject, chip) search-state
// pointer, lazily allocating the map + entry. Used by currentSearch
// so `/` opens a sticky filter buffer that survives navigation +
// chip switches (each chip has its own buffer).
func (d *orgData) RecordsSearchPtr(sobject, chipID string) *searchState {
	if d.ChipRecordsSearch == nil {
		d.ChipRecordsSearch = map[string]*searchState{}
	}
	key := sobject + ":" + chipID
	if s, ok := d.ChipRecordsSearch[key]; ok {
		return s
	}
	s := &searchState{}
	d.ChipRecordsSearch[key] = s
	return s
}

// RecordsTableStatePtr returns the per-(sobject, chip) ListTableState
// pointer, lazily allocating. Used by the list-table renderer to
// remember horizontal scroll, user-set column widths, and zen flag
// across renders. Defaults: FrozenCols = 1 (the leftmost projected
// column anchors when scrolling).
func (d *orgData) RecordsTableStatePtr(sobject, chipID string) *uilayout.ListTableState {
	if d.RecordsTableState == nil {
		d.RecordsTableState = map[string]*uilayout.ListTableState{}
	}
	key := sobject + ":" + chipID
	if s, ok := d.RecordsTableState[key]; ok {
		return s
	}
	s := &uilayout.ListTableState{FrozenCols: 1}
	d.RecordsTableState[key] = s
	return s
}

// EnsureRecentlyViewedPerSObject lazily wires a per-sObject Resource
// of RecentlyViewedRow. Distinct from d.RecentlyViewed which is a
// single GLOBAL top-N across all sObjects — that one starves per-sObject
// filters when the user has recently viewed many other types. Backs the
// synthetic "Recently Viewed" SF-mode chip on /objects/<X>/Records.
func (d *orgData) EnsureRecentlyViewedPerSObject(alias, sobject string) *Resource[[]sf.RecentlyViewedRow] {
	return ensureKeyed(&d.RecentlyViewedPerSObject, sobject, func() *Resource[[]sf.RecentlyViewedRow] {
		return &Resource[[]sf.RecentlyViewedRow]{
			Scope:   d.username,
			Key:     "recently_viewed_per_sobject:" + sobject,
			TTL:     d.ttl("recently_viewed_per_sobject", 5*time.Minute),
			NoCache: true,
			Fetch: func() ([]sf.RecentlyViewedRow, error) {
				return sf.ListRecentlyViewed(alias, sf.RecentlyViewedOpts{SObject: sobject, Limit: 200})
			},
		}
	})
}

// EnsureListViews lazily wires a Resource[[]sf.ListView] for the given
// sObject. 5-minute TTL — list view metadata doesn't change often but
// users can create new ones mid-session, so we don't cache too long.
// NoCache: true — list view metadata is per-user (sharing rules) and
// persisting across session restarts would be misleading.
func (d *orgData) EnsureListViews(alias, sobject string) *Resource[[]sf.ListView] {
	return ensureKeyed(&d.ListViewsPerSObject, sobject, func() *Resource[[]sf.ListView] {
		return &Resource[[]sf.ListView]{
			Scope:   d.username,
			Key:     "listviews:" + sobject,
			TTL:     d.ttl("list_views", 24*time.Hour),
			NoCache: true,
			Fetch: func() ([]sf.ListView, error) {
				return sf.ListViews(alias, sobject)
			},
		}
	})
}

// EnsureListViewResult lazily wires a Resource[sf.ListViewResult] for
// the specific (sobject, listViewID) pair. Keyed by both because the
// same list-view ID is unique only per-sobject. NoCache for the same
// reasons as records — actual data, privacy-sensitive, moves.
func (d *orgData) EnsureListViewResult(alias, sobject, listViewID string) *Resource[sf.ListViewResult] {
	key := sobject + ":" + listViewID
	return ensureKeyed(&d.ListViewResults, key, func() *Resource[sf.ListViewResult] {
		return &Resource[sf.ListViewResult]{
			Scope: d.username,
			Key:   "listview:" + key,
			// First-visit-of-session only; manual `r` to refresh.
			TTL:     d.ttl("list_view_results", 24*time.Hour),
			NoCache: true,
			Fetch: func() (sf.ListViewResult, error) {
				return sf.RunListView(alias, sobject, listViewID, d.settings.ListViewPreviewLimit())
			},
		}
	})
}

// EnsureFlowVersions lazily wires a resource for the per-definition
// version list.
func (d *orgData) EnsureFlowVersions(alias, definitionID string) *Resource[[]sf.FlowVersion] {
	return ensureKeyed(&d.FlowVersions, definitionID, func() *Resource[[]sf.FlowVersion] {
		return &Resource[[]sf.FlowVersion]{
			Scope: d.username, Key: "flowversions:" + definitionID,
			TTL:   d.ttl("flow_versions", 1*time.Hour),
			Fetch: func() ([]sf.FlowVersion, error) { return sf.FlowVersions(alias, definitionID) },
		}
	})
}

// EnsureFlowVersionDetail lazily wires a resource for one flow version's
// full definition metadata (the JSON the in-terminal viewer renders),
// keyed by the version's Tooling Id.
func (d *orgData) EnsureFlowVersionDetail(alias, versionID string) *Resource[map[string]any] {
	return ensureKeyed(&d.FlowVersionDetail, versionID, func() *Resource[map[string]any] {
		return &Resource[map[string]any]{
			Scope: d.username, Key: "flowversiondef:" + versionID,
			TTL:   d.ttl("flow_version_def", 1*time.Hour),
			Fetch: func() (map[string]any, error) { return sf.FlowVersionMetadata(alias, versionID) },
		}
	})
}

// EnsureRecordDetail lazily wires a Resource per (sobject, id) pair.
// NoCache because record values are privacy-sensitive live data —
// caching to disk between sessions would surprise users when they
// expect a stale-after-restart record list. In-process Resource
// caching still applies (TTL key "record_detail").
func (d *orgData) EnsureRecordDetail(alias, sobject, recordID string) *Resource[map[string]any] {
	key := sobject + ":" + recordID
	// Capture sobject + id by value into the closure so re-firing the
	// fetch later (after a refresh) hits the right record.
	sobj, id := sobject, recordID
	return ensureKeyed(&d.RecordDetails, key, func() *Resource[map[string]any] {
		return &Resource[map[string]any]{
			Scope: d.username, Key: "recorddetail:" + key,
			TTL:     d.ttl("record_detail", 24*time.Hour),
			NoCache: true,
			Fetch: func() (map[string]any, error) {
				return sf.GetRecord(alias, sobj, id)
			},
		}
	})
}

// EnsureRecordReferenceNames lazily wires a Resource per record
// that pulls the resolved Name of every reference field's target.
// Driven by the parent's describe at fetch time so callers must
// have already loaded d.Describes[sobject] before this Resource's
// fetch fires — the dispatcher orchestrates that ordering.
//
// Returns nil when the describe isn't cached yet; caller bails
// gracefully (the renderer falls back to raw Ids until both
// resources land).
func (d *orgData) EnsureRecordReferenceNames(alias, sobject, recordID string) *Resource[map[string]string] {
	key := sobject + ":" + recordID
	desc, ok := d.Describes[sobject]
	if !ok || desc.FetchedAt().IsZero() {
		return nil
	}
	parentDesc := desc.Value()
	sobj, id := sobject, recordID
	return ensureKeyed(&d.RecordReferenceNames, key, func() *Resource[map[string]string] {
		return &Resource[map[string]string]{
			Scope:   d.username,
			Key:     "recordrefs:" + key,
			TTL:     d.ttl("record_reference_names", 24*time.Hour),
			NoCache: true,
			Fetch: func() (map[string]string, error) {
				soql := sf.BuildReferenceNameSOQL(parentDesc, id)
				if soql == "" {
					return nil, nil
				}
				res, err := sf.Query(alias, soql, false)
				if err != nil {
					return nil, err
				}
				if len(res.Records) == 0 {
					return nil, nil
				}
				_ = sobj
				return sf.ParseReferenceNames(parentDesc, res.Records[0]), nil
			},
		}
	})
}

// EnsureRecordChildCounts lazily wires a Resource per record that
// batches one COUNT() per child relationship via Composite. Same
// describe-required precondition as EnsureRecordReferenceNames.
func (d *orgData) EnsureRecordChildCounts(alias, sobject, recordID string) *Resource[map[string]int] {
	key := sobject + ":" + recordID
	desc, ok := d.Describes[sobject]
	if !ok || desc.FetchedAt().IsZero() {
		return nil
	}
	parentDesc := desc.Value()
	id := recordID
	return ensureKeyed(&d.RecordChildCounts, key, func() *Resource[map[string]int] {
		return &Resource[map[string]int]{
			Scope:   d.username,
			Key:     "recordchildcounts:" + key,
			TTL:     d.ttl("record_child_counts", 1*time.Hour),
			NoCache: true,
			Fetch: func() (map[string]int, error) {
				queries := sf.BuildChildCountQueries(parentDesc, id)
				if len(queries) == 0 {
					return nil, nil
				}
				return sf.RunChildCountBatch(alias, queries)
			},
		}
	})
}

// EnsureReportRun lazily wires a Resource per report id. NoCache because
// runs are live data — caching at the local layer would mask SF's own
// snapshot semantics, which is the cache the user actually thinks about.
func (d *orgData) EnsureReportRun(alias, reportID string) *Resource[sf.ReportRun] {
	return ensureKeyed(&d.ReportRuns, reportID, func() *Resource[sf.ReportRun] {
		return &Resource[sf.ReportRun]{
			Scope: d.username, Key: "reportrun:" + reportID,
			// First-visit-of-session only; manual `r` for local refresh,
			// `R` (planned) to force a SF re-run.
			TTL:     d.ttl("report_runs", 24*time.Hour),
			NoCache: true,
			Fetch: func() (sf.ReportRun, error) {
				return sf.RunReport(alias, reportID, false)
			},
		}
	})
}

// EnsureReportFolders lazily creates the treechip Registry that
// drives /reports folder navigation. Idempotent: subsequent calls
// return the same Registry + a nil load cmd.
//
// First call returns a non-nil tea.Cmd that the caller MUST fire —
// the cmd async-fetches the folder list off the render goroutine.
// Without it the registry never hydrates and the tab stays at
// "loading folders…" forever.
//
// The persisted last-path is hydrated lazily inside Apply (when the
// load returns and the source is populated) so HydrateLastPath
// doesn't fail with "folders not loaded yet."
func (d *orgData) EnsureReportFolders(alias string, st *settings.Settings) (*treechip.Registry, tea.Cmd) {
	if d.ReportFoldersLoaded && d.ReportFolders != nil {
		return d.ReportFolders, nil
	}
	src := sources.NewReportFolderSource(alias)
	persist := sources.NewSettingsPersister(st, d.username, "report-folders")
	reg := treechip.NewRegistry("report-folders", src, persist)
	d.ReportFolders = reg
	d.ReportFoldersSrc = src
	d.ReportFoldersLoaded = true
	loadFn := src.LoadAsync()
	if loadFn == nil {
		return reg, nil
	}
	return reg, tea.Cmd(func() tea.Msg { return loadFn() })
}

// EnsureDescribe lazily wires a describe Resource for the given sobject.
func (d *orgData) EnsureDescribe(alias, sobject string) *Resource[sf.SObjectDescribe] {
	return ensureKeyed(&d.Describes, sobject, func() *Resource[sf.SObjectDescribe] {
		// Key is versioned (describe_v2) so the on-disk cache from an
		// older build — which serialized SObjectDescribe BEFORE the
		// MruEnabled field existed — is bypassed rather than served with
		// MruEnabled defaulting to false (which mislabelled every cached
		// object as not-recently-viewable). Bump the suffix whenever the
		// persisted describe shape gains a field readers branch on.
		return &Resource[sf.SObjectDescribe]{
			Scope: d.username, Key: "describe_v3:" + sobject,
			TTL:   d.ttl("describes", time.Hour),
			Fetch: func() (sf.SObjectDescribe, error) { return sf.Describe(alias, sobject) },
		}
	})
}

// EnsureCustomObjectBaseline lazily wires a Tooling-CustomObject
// baseline Resource for the given sobject. The baseline carries the
// metadata-level feature toggles (enableReports, enableActivities,
// etc.) which the standard describe doesn't expose — readers like
// the object-action sidebar use this to show the "current state" of
// each toggle so the modal cursor preselects correctly.
//
// NoCache:true — toggles can change via Setup or other deploys
// outside our control; persisting to the on-disk cache risks
// showing stale state. The in-memory Resource gives session-level
// caching which is plenty.
func (d *orgData) EnsureCustomObjectBaseline(alias, sobject string) *Resource[*sf.CustomObjectBaseline] {
	return ensureKeyed(&d.CustomObjectBaselines, sobject, func() *Resource[*sf.CustomObjectBaseline] {
		return &Resource[*sf.CustomObjectBaseline]{
			Scope: d.username, Key: "object_baseline:" + sobject, TTL: 10 * time.Minute, NoCache: true,
			Fetch: func() (*sf.CustomObjectBaseline, error) {
				return sf.FetchCustomObjectBaseline(alias, sobject)
			},
		}
	})
}

// Back-compat wrappers over SObjectChildren.EnsureList / EnsureDetail.
// Kept so existing callers (update.go ensureDataFor, search_global
// kickScopeInFetches, etc.) don't need to change. New code should use
// d.ValidationRules.EnsureList(d.username, alias, sobject) directly.

func (d *orgData) EnsureValidationRules(alias, sobject string) *Resource[[]sf.ValidationRuleRow] {
	return d.ValidationRules.EnsureList(d.username, alias, sobject)
}
func (d *orgData) EnsureValidationRuleDetail(alias, ruleID string) *Resource[sf.ValidationRuleDetail] {
	return d.ValidationRules.EnsureDetail(d.username, alias, ruleID)
}
func (d *orgData) EnsureRecordTypes(alias, sobject string) *Resource[[]sf.RecordTypeRow] {
	return d.RecordTypes.EnsureList(d.username, alias, sobject)
}
func (d *orgData) EnsureRecordTypeDetail(alias, rtID string) *Resource[sf.RecordTypeDetail] {
	return d.RecordTypes.EnsureDetail(d.username, alias, rtID)
}
func (d *orgData) EnsureTriggers(alias, sobject string) *Resource[[]sf.TriggerRow] {
	return d.Triggers.EnsureList(d.username, alias, sobject)
}
func (d *orgData) EnsureTriggerDetail(alias, id string) *Resource[sf.TriggerDetail] {
	return d.Triggers.EnsureDetail(d.username, alias, id)
}

// EnsureFLS lazily wires a FieldPermissions Resource for one
// (sobject, parent-permset) pair. Key shape "<sobject>:<parentID>".
// No-cache + short TTL — FLS changes often during admin work.
func (d *orgData) EnsureFLS(alias, sobject, parentID string) *Resource[[]sf.FieldPermissionRow] {
	key := sobject + ":" + parentID
	return ensureKeyed(&d.FLS, key, func() *Resource[[]sf.FieldPermissionRow] {
		return &Resource[[]sf.FieldPermissionRow]{
			Scope:   d.username,
			Key:     "fls:" + key,
			TTL:     d.ttl("fls", 1*time.Hour),
			NoCache: true,
			Fetch: func() ([]sf.FieldPermissionRow, error) {
				return sf.ListFieldPermissions(alias, sobject, parentID)
			},
		}
	})
}

// EnsureObjectPerms lazily wires an ObjectPermissions Resource for a
// given parent. Key shape "<kind>:<parentID>".
// No-cache + short TTL — object perms change during admin work.
func (d *orgData) EnsureObjectPerms(alias, kind, parentID string) *Resource[[]sf.ObjectPermission] {
	key := kind + ":" + parentID
	return ensureKeyed(&d.ObjectPerms, key, func() *Resource[[]sf.ObjectPermission] {
		return &Resource[[]sf.ObjectPermission]{
			Scope:   d.username,
			Key:     "objectperms:" + key,
			TTL:     2 * time.Minute,
			NoCache: true,
			Fetch: func() ([]sf.ObjectPermission, error) {
				return sf.ListObjectPermissions(alias, parentID)
			},
		}
	})
}

// EnsureSystemPerms lazily wires a SystemPermissions Resource for a
// given permset Id. Aggressive cache (1 hour, on disk) — the data
// only changes when an admin explicitly toggles a system perm, which
// is rare; the underlying fetch is ~12 composite subrequests per
// visit so amortising across a long TTL is the right move. Press r
// to force-refresh after writes.
func (d *orgData) EnsureSystemPerms(alias, permSetID string) *Resource[[]sf.SystemPermission] {
	return ensureKeyed(&d.SystemPerms, permSetID, func() *Resource[[]sf.SystemPermission] {
		return &Resource[[]sf.SystemPermission]{
			Scope: d.username,
			Key:   "systemperms:" + permSetID,
			TTL:   d.ttl("system_perms", time.Hour),
			Fetch: func() ([]sf.SystemPermission, error) {
				return sf.ListSystemPermissions(alias, permSetID)
			},
		}
	})
}

// EnsureGroupMembers lazily wires a Resource[[]GroupMemberRow] for
// one Queue or Public Group. Used by both the Queue detail and
// Public Group detail surfaces — Salesforce's GroupMember table
// doesn't distinguish by parent kind, so the lookup is uniform.
//
// NoCache: queue/group membership changes more often than the rest
// of /perms data; freshness on drill matters more than persistence.
func (d *orgData) EnsureGroupMembers(alias, groupID string) *Resource[[]sf.GroupMemberRow] {
	return ensureKeyed(&d.GroupMembers, groupID, func() *Resource[[]sf.GroupMemberRow] {
		return &Resource[[]sf.GroupMemberRow]{
			Scope:   d.username,
			Key:     "groupmembers:" + groupID,
			TTL:     2 * time.Minute,
			NoCache: true,
			Fetch: func() ([]sf.GroupMemberRow, error) {
				return sf.ListGroupMembersDetailed(alias, groupID)
			},
		}
	})
}

// EnsureUserSessions lazily wires a Resource for one user's live
// sessions (the /users → Active drill). Keyed by user Id. NoCache +
// short TTL: sessions are live state, freshness on drill is the point.
func (d *orgData) EnsureUserSessions(alias, userID string) *Resource[[]sf.SessionRow] {
	return ensureKeyed(&d.UserSessions, userID, func() *Resource[[]sf.SessionRow] {
		return &Resource[[]sf.SessionRow]{
			Scope:   d.username,
			Key:     "usersessions:" + userID,
			TTL:     30 * time.Second,
			NoCache: true,
			Fetch: func() ([]sf.SessionRow, error) {
				return sf.UserSessions(alias, userID)
			},
		}
	})
}

// EnsureAssignedUsers lazily wires a PermissionSetAssignment Resource.
func (d *orgData) EnsureAssignedUsers(alias, permSetID string) *Resource[[]sf.PermissionSetAssignment] {
	return ensureKeyed(&d.AssignedUsers, permSetID, func() *Resource[[]sf.PermissionSetAssignment] {
		return &Resource[[]sf.PermissionSetAssignment]{
			Scope:   d.username,
			Key:     "assignedusers:" + permSetID,
			TTL:     2 * time.Minute,
			NoCache: true,
			Fetch: func() ([]sf.PermissionSetAssignment, error) {
				return sf.ListAssignedUsers(alias, permSetID)
			},
		}
	})
}

// FieldState lazily allocates per-sobject field-browser list state.
// The ListView matcher mirrors the old filterFields (name/label
// substring); callers re-Set the field slice each render via
// syncFieldList.
func (d *orgData) FieldState(sobject string) *describeFieldState {
	f, ok := d.DescribeFields[sobject]
	if ok {
		return f
	}
	f = &describeFieldState{}
	f.List.SetMatch(func(fld sf.Field, q string) bool {
		return strings.Contains(strings.ToLower(fld.Name), q) ||
			strings.Contains(strings.ToLower(fld.Label), q)
	})
	d.DescribeFields[sobject] = f
	return f
}

// syncFieldList re-Sets the field slice on the per-sobject ListView
// from the (already-loaded) describe. Idempotent + version-aware: Set
// is a no-op when the slice is unchanged, so calling it every render is
// cheap. Returns the FieldState for chaining.
func (d *orgData) syncFieldList(sobject string, fields []sf.Field) *describeFieldState {
	fs := d.FieldState(sobject)
	fs.List.Set(fields)
	return fs
}

// cursoredField returns the field under the Schema list cursor for the
// current describe, syncing the list from the describe first. (ok=false
// when the describe isn't loaded or the filtered list is empty.) Shared
// by the breadcrumb / identity / sidebar / activate read sites so they
// all agree on "which field is selected."
func (d *orgData) cursoredField(sobject string, r *Resource[sf.SObjectDescribe]) (sf.Field, bool) {
	if r == nil || r.FetchedAt().IsZero() {
		return sf.Field{}, false
	}
	fs := d.syncFieldList(sobject, r.Value().Fields)
	rows := fs.List.Filtered()
	cur := fs.List.Cursor()
	if cur < 0 || cur >= len(rows) {
		return sf.Field{}, false
	}
	return rows[cur], true
}

// fetchDeploysDelta backs d.Deploys.FetchWithExisting. It receives a
// snapshot captured before the command goroutine starts, then does a
// full pull on cold-start (no cached rows yet) or a delta pull after
// that. Caps the merged slice at 25 so a long-running session doesn't
// unbounded-grow the list.
func fetchDeploysDelta(alias string, existing []sf.DeployRow, limit int) ([]sf.DeployRow, error) {
	if limit <= 0 {
		limit = 25
	}
	if len(existing) == 0 {
		return sf.RecentDeploys(alias, limit)
	}
	// Rows are sorted by CreatedDate DESC in the SOQL so the first
	// cached row is the most recent one we've seen.
	since := existing[0].CreatedDate
	delta, err := sf.RecentDeploysSince(alias, limit, since)
	if err != nil {
		return nil, err
	}
	if len(delta) == 0 {
		// No new deploys — but in-flight rows still need re-polling
		// or a watched deploy stays InProgress forever (the delta
		// only matches CreatedDate > newest, which by definition
		// never includes the rows whose STATUS is what's changing).
		// Field bug 2026-06-12: list stuck at InProgress across
		// refresh and restart while the drill (live REST) showed
		// Succeeded.
		return refreshInFlightDeploys(alias, existing), nil
	}
	// Delta comes back DESC; prepend to existing and cap to the limit.
	merged := append(delta, existing...)
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return refreshInFlightDeploys(alias, merged), nil
}

// refreshInFlightDeploys re-queries any non-terminal rows in the
// merged window and patches them in place. Without this, the
// delta-merge would keep a Pending/InProgress row frozen forever
// (delta only fetches CreatedDate > newest). Errors are swallowed —
// a failed patch just means the row updates on the next poll.
func refreshInFlightDeploys(alias string, rows []sf.DeployRow) []sf.DeployRow {
	var ids []string
	for _, r := range rows {
		if r.InFlight() {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return rows
	}
	fresh, err := sf.RefreshDeploys(alias, ids)
	if err != nil || len(fresh) == 0 {
		return rows
	}
	for i := range rows {
		if f, ok := fresh[rows[i].ID]; ok {
			rows[i] = f
		}
	}
	return rows
}

// Sync helpers (orgData → ListView) live in model_sync.go.
//
// modelServices lives in model_services.go.

// modelRuntime lives in model_runtime.go.

// modelChips lives in model_chips.go.

// modelOrgs lives in model_orgs.go.

// modelSOQL lives in model_soql.go.

// modelLocalNavigation lives in model_local_nav.go.

// modelTransient lives in model_transient.go.

// modelSurfaceState lives in model_surface_state.go.

// modelDevProjectState lives in model_devproject_state.go.

// modelOrgManagement lives in model_org_management.go.

// Model is the top-level Bubble Tea model.
//
// Concurrency model — read this before adding goroutines:
// ALL mutations to Model and to orgData (m.data[org]) happen on the
// single-threaded Update goroutine. tea.Cmd closures (Resource.Fetch,
// export workers, pollers) run on their own goroutines but MUST be
// read-only with respect to Model/orgData: they do their work against
// captured inputs and communicate results back exclusively by
// returning a tea.Msg, which Update folds in on the main goroutine.
// The one sanctioned exception is the export registry (m.exports),
// which is internally mutex-guarded so workers can advance job phases
// directly. Nothing else has a lock — a fetch closure that writes to
// a ListView, Resource value, or map WILL race. When in doubt, return
// a message.
type Model struct {
	modelServices
	modelRuntime
	modelChips
	modelOrgs
	modelSOQL
	modelExec
	modelCompare
	modelLocalNavigation
	modelTransient
	modelSurfaceState
	modelDevProjectState
	modelBundleDetailState
	modelOrgManagement
}

// openMenuMode distinguishes "select to open" vs "select to yank URL".
type openMenuMode int

const (
	menuOpen openMenuMode = iota
	menuYank
)

// openMenuState is the live state of the open-targets overlay.
type openMenuState struct {
	title   string
	mode    openMenuMode
	org     sf.Org
	source  sf.Openable // original cursored item — used for recent-visit tracking
	targets []sf.OpenTarget
	cursor  int

	// restoreGlobalSearch is set when the open menu was launched
	// from inside the global-search modal (ctrl+o on a hit). Esc
	// on the open menu pops back to this exact search state —
	// same input value, scope chain, cursor, mode — so the user
	// resumes where they were without re-typing. nil = nothing to
	// restore (standard open menu launched from a list surface).
	restoreGlobalSearch *globalSearchState

	// pendingTarget is set on the browser sub-picker menu (opened via
	// b / ctrl+o on a target). It's the ORIGINAL open target the user
	// was on; the sub-picker's rows are browser choices that fire this
	// target with the chosen browser as a one-off override. nil on a
	// normal open menu.
	pendingTarget *sf.OpenTarget
}

func New(c *cache.Cache) Model {
	// Load settings.toml (safety levels etc.). A corrupt or missing
	// file is non-fatal — Load returns an empty *Settings which
	// resolves to safe defaults per org kind.
	st, _ := settings.Load()
	if Demo {
		// settings.Ephemeral guarantees st is a blank slate; layer
		// the fictional orgs' safety levels on top.
		applyDemoSettings(st)
	}

	// Push API-client tuning down into the sf package. sf can't import
	// settings (it's the lower layer), so the UI injects the user's
	// [ui.api] preferences here at startup. Done before any client work.
	applySFConfig(st)

	// Resolve pinned tab order. Falls back to defaults when unset.
	RebuildTabsForNumbers(st.PinnedTabs())

	// Migrate legacy chip sections (lenses / object_filters / flow_filters)
	// into the unified Chips slot. Idempotent — re-runs are no-ops once
	// the domain is populated. The next Save() flushes the legacy
	// sections from disk; until then the user's settings.toml keeps both
	// shapes.
	if migrated := qchipmigrate.Run(st); migrated > 0 {
		st.ClearLegacyChips()
		_ = st.Save()
	}

	// Apply the persisted theme before any rendering happens so first
	// paint matches the user's preference.
	theme.ApplyPalette(st.Theme())
	m := Model{
		modelServices: modelServices{
			cache:    c,
			settings: st,
			exports:  newExportRegistry(st.ExportHistoryMax()),
			updates:  updatecheck.New(),
		},
		modelRuntime: modelRuntime{
			lastCompositor: lipgloss.NewCompositor(),
			renderCache:    newRenderCache(),
			renderTrace:    newRenderTracerFromEnv(),
			wheel:          &wheelRuntime{},
			// Default focus is the main pane.  Used to be focusOrgs +
			// leftOpen=false (left rail collapsed but "focused"),
			// which made Tab on startup silently cycle invisible
			// rail-utilities instead of the active tab's subtabs.
			// Visible focus follows the rail's visibility: open the
			// rail (alt+o or |), focus shifts; close it, focus
			// returns to main.
			focus: focusMain,
			// Startup visibility defaults are user-overridable via
			// [ui.startup] in settings.toml; the literal here is the
			// built-in default each accessor falls back to.
			sidebarOpen:    st.StartupSidebarOpen(true),
			sidebarStacked: st.SidebarStartsStacked(),
			// Hide the SOQL query line under the VIEWS chip strip by
			// default — most users don't read the underlying SOQL on
			// every records page, and showing it eats a row of vertical
			// space. ctrl+- re-reveals it when needed. (Stored as the
			// inverse "visible" preference; default false = hidden.)
			queryLineHidden: !st.StartupQueryLineVisible(false),
			// leftOpen defaults to false — the org panel sits on the
			// main tab bar as a pill (renderTabBar prepends an Orgs
			// pill when the rail is collapsed). Users who want it
			// pinned open can hit `ctrl+\` to toggle it permanently for
			// the session.
			leftOpen:   st.StartupLeftRailOpen(false),
			leftPinned: st.StartupLeftRailOpen(false),
		},
		modelChips: modelChips{
			chipRegistries:  newChipRegistries(),
			activeTransient: map[string]string{},
		},
		modelOrgs: modelOrgs{
			noOrgTab: resolveStartTab(st),
			data:     map[string]*orgData{},
		},
		modelSOQL: modelSOQL{
			soqlSession: newSOQLSession(st.StartupSOQLSeed()),
		},
		modelTransient: modelTransient{
			listTableWidthPrefs:       map[string]*listTableWidthPrefs{},
			listTableWidthPrefsLoaded: map[string]bool{},
			compareTypesRefreshed:     map[string]bool{},
		},
		modelExec: modelExec{
			execInput:      newExecInput("System.debug('hello, world');"),
			execLogSearch:  &searchState{},
			execCaptureLog: true,
		},
		modelSurfaceState: modelSurfaceState{
			bodyFocus: true,
		},
	}
	// Hydrate user-defined chips from settings into each registry.
	// One unified slice on disk; per-domain LoadFromSettings filters.
	m.chipRegistry(domainRecords).LoadFromSettings(st)
	m.chipRegistry(domainObjects).LoadFromSettings(st)
	m.chipRegistry(domainFlows).LoadFromSettings(st)
	m.chipRegistry(domainApex).LoadFromSettings(st)
	m.chipRegistry(domainTriggers).LoadFromSettings(st)
	m.chipRegistry(domainLWC).LoadFromSettings(st)
	m.chipRegistry(domainAura).LoadFromSettings(st)
	m.chipRegistry(domainPermSets).LoadFromSettings(st)
	m.chipRegistry(domainPSGs).LoadFromSettings(st)
	m.chipRegistry(domainProfiles).LoadFromSettings(st)
	m.chipRegistry(domainQueues).LoadFromSettings(st)
	m.chipRegistry(domainPublicGroup).LoadFromSettings(st)
	m.chipRegistry(domainSOQLSaved).LoadFromSettings(st)
	m.chipRegistry(domainSOQLHistory).LoadFromSettings(st)
	m.chipRegistry(domainUsers).LoadFromSettings(st)
	m.orgsRes = Resource[[]sf.Org]{
		Scope: "global", Key: "orgs",
		TTL:   st.CacheTTL("orgs", time.Minute),
		Fetch: func() ([]sf.Org, error) { return sf.ListOrgs() },
	}
	m.projectsRes = Resource[[]*project.Project]{
		Scope: "global", Key: "projects",
		TTL: st.CacheTTL("projects", 10*time.Minute),
		Fetch: func() ([]*project.Project, error) {
			return project.Discover(project.DefaultRoots(), 4)
		},
	}
	m.projectList.SetMatch(func(p *project.Project, q string) bool {
		return strings.Contains(strings.ToLower(p.Name), q) ||
			strings.Contains(strings.ToLower(p.Path), q)
	})
	m.setupList.Set(setupLinks)
	m.setupList.SetMatch(func(l setupLink, q string) bool {
		return strings.Contains(strings.ToLower(l.Name), q) ||
			strings.Contains(strings.ToLower(l.Path), q)
	})
	m.devProjectList.SetMatch(func(p devproject.DevProject, q string) bool {
		return strings.Contains(strings.ToLower(p.Name), q) ||
			strings.Contains(strings.ToLower(p.Description), q)
	})
	return m
}

// WithStartupWarning attaches a short message that will appear in the
// flash banner on first paint. Used to surface non-fatal startup errors
// (e.g. a malformed keybindings file).
func (m Model) WithStartupWarning(msg string) Model {
	m.banner = msg
	m.bannerUntil = time.Now().Add(6 * time.Second)
	return m
}

// WithDevProjects wires the dev-project store onto the model. Called
// from main when the SQLite open succeeds. Nil store is fine — the
// /dev-projects + /org-projects tabs detect it and surface a "feature
// unavailable" hint instead of crashing.
//
// Eagerly loads the dev-project list once so the left-rail panel
// renders meaningfully on first paint (without needing to navigate
// to /dev-projects first).
func (m Model) WithDevProjects(s *devproject.Store) Model {
	m.devProjects = s
	m.reloadDevProjects()
	return m
}

func (m Model) Init() tea.Cmd {
	// Re-register demo targets before any resource commands are built. This is
	// local-only and safe before the real-org acknowledgement gate.
	restoreDemoOrgOnBoot(m.settings.DemoOrgImported())
	if cmd := m.legalTriggerCmd(); cmd != nil {
		return cmd
	}
	return m.runtimeStartupCmd()
}

// runtimeStartupCmd starts work that may discover or contact authenticated
// Salesforce orgs. Keep this behind the versioned legal acknowledgement.
func (m Model) runtimeStartupCmd() tea.Cmd {
	cmds := []tea.Cmd{
		m.orgsRes.Ensure(m.cache),
		m.projectsRes.Ensure(m.cache),
	}
	// Wire the control-channel pump. ControlWritesCmd returns nil
	// when --control wasn't enabled, so this is a no-op then.
	if cmd := m.ControlWritesCmd(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	// First-launch welcome modal (no-op after the first run, or in demo).
	if cmd := m.welcomeTriggerCmd(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	// Cached stable-release discovery runs asynchronously and is skipped for
	// development/demo builds or when the user disables it.
	if cmd := m.updateCheckCmd(false); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

// newSOQLInput builds the SOQL editor widget with our theme + initial
// query. Multi-line textarea so long queries can break across clauses
// (`SELECT ...` / `FROM ...` / `WHERE ...`) instead of horizontal-
// scrolling a 200-char single line. Enter is intercepted by the edit
// handler to run the query; shift+enter / alt+enter insert a newline.
// Starts blurred — handleKey calls Focus() when the user enters edit
// mode.
func newSOQLInput(initial string) textarea.Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	// Bump MaxHeight from the default 99 to a generous 500. We
	// resize the textarea every render to fit its content
	// (SetHeight in renderSOQLSessionBody) so this just removes
	// the upstream clamp that would otherwise cap visual rows.
	// Without raising it, the internal viewport scrolls when the
	// query grows past 99 rows of wrapped content — but more
	// importantly, the viewport's YOffset can land non-zero on
	// shift+enter and stay there until MaxHeight ≥ row count.
	ta.MaxHeight = 500
	// Rebind newline insertion to shift+enter so plain Enter is free
	// for "run query." The default binding maps both Enter and
	// ctrl+m to InsertNewline.
	ta.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("shift+enter", "alt+enter"),
		key.WithHelp("shift+enter", "insert newline"),
	)
	ta.SetValue(initial)
	// Cursor at end of buffer so users land ready to append.
	ta.CursorEnd()
	return ta
}

// newExecInput builds the multi-line anonymous-Apex editor widget.
// Starts blurred — handleExecKey focuses it when the user enters
// edit mode (e key). Multi-line so users can paste / type 5-50 line
// snippets in-app without bouncing to $EDITOR for every tweak.
func newExecInput(initial string) textarea.Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.SetValue(initial)
	ta.ShowLineNumbers = true
	return ta
}
