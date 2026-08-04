package ui

// orgData sub-struct decomposition.
//
// orgData was 173 fields. Today's split groups them by concern using
// embedded sub-structs (same pattern as the Model split in
// model_*.go). Embedding means every existing `d.Records`,
// `d.DescribeCur`, etc. access keeps working unchanged — Go promotes
// fields through the embedded type.
//
// Groups follow the dominant access pattern in the codebase:
//
//	orgDataCore          — shared infra (cache, settings, identity)
//	orgDataTopLists      — org-wide list Resources + their ListView
//	                       wrappers + flat list-table states
//	orgDataRecords       — records-shaped surfaces (per-sobject maps,
//	                       chip records, list-view results, drill)
//	orgDataMetadata      — per-sobject describe + custom-object/field
//	                       lookups + flow-version maps + child entities
//	orgDataPerms         — FLS / ObjectPerms / SystemPerms /
//	                       AssignedUsers / GroupMembers / PermParent
//	                       drill / PermissionSets picker
//	orgDataUsers         — /users · All users chip state, User detail
//	                       drill, User * cache
//	orgDataHome          — /home subtab lists + table states
//	orgDataRecent        — recent log (local + server) + merged stream
//	orgDataCode          — Apex / LWC / Aura body + detail caches
//	orgDataReports       — Reports resource + folder tree state
//	orgDataDevProjects   — Loaded DevProject + Bundle drill state
//	orgDataSOQLLibrary   — SOQL Saved / History library
//	orgDataNav           — per-org tab/subtab/chip cursors,
//	                       drill-identity (DescribeCur etc.),
//	                       LastTabInStem, unified Cursors store

import (
	"sync"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/orgproject"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/treechip"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/treechip/sources"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// --- Core infra -----------------------------------------------------------

type orgDataCore struct {
	username string
	target   string // sf CLI / REST target captured by Fetch closures (alias when present, else username)
	cache    *cache.Cache
	// settings is held so per-sobject Resources (Records, FLS, etc.)
	// can resolve their TTLs through st.CacheTTL the same way the
	// startup-loaded ones do. Set in newOrgData.
	settings *settings.Settings
}

// --- Top-level Resource + ListView wrappers ------------------------------

type orgDataTopLists struct {
	SObjects       Resource[[]sf.SObject]
	ApexLogs       Resource[[]sf.ApexLogRow]
	SetupAudit     Resource[[]sf.SetupAuditRow]
	FlowInterviews Resource[[]sf.FlowInterviewRow]
	ActiveUsers    Resource[[]sf.ActiveUserRow]
	AsyncJobs      Resource[[]sf.AsyncJobRow]
	ScheduledJobs  Resource[[]sf.CronTriggerRow]

	// Per-user session drill (/users → Active → Enter). SessionUserID /
	// SessionUserName are the drilled-in user; UserSessions is the
	// lazily-fetched per-user session list, keyed by user Id.
	SessionUserID   string
	SessionUserName string
	UserSessions    map[string]*Resource[[]sf.SessionRow]
	Deploys         Resource[[]sf.DeployRow]
	Packages        Resource[[]sf.InstalledPackage]
	Flows           Resource[[]sf.Flow]
	Queues          Resource[[]sf.QueueRow]
	PublicGroups    Resource[[]sf.PublicGroupRow]
	Community       Resource[[]sf.CommunityRow]

	// /communities drill: CommunityCur is the drilled-in community's
	// url prefix (used to best-effort group its pages); CommunityPages
	// is the lazily-fetched community-page list keyed by that prefix.
	CommunityCur     string
	CommunityCurName string
	CommunityCurID   string
	CommunityPages   map[string]*Resource[[]sf.CommunityPageRow]

	// Browsable list wrappers.
	SObjectList       ListView[sf.SObject]
	ApexLogList       ListView[sf.ApexLogRow]
	SetupAuditList    ListView[sf.SetupAuditRow]
	FlowInterviewList ListView[sf.FlowInterviewRow]
	ActiveUserList    ListView[sf.ActiveUserRow]
	AsyncJobList      ListView[sf.AsyncJobRow]
	ScheduledJobList  ListView[sf.CronTriggerRow]
	UserSessionList   ListView[sf.SessionRow]
	DeployList        ListView[sf.DeployRow]
	PackageList       ListView[sf.InstalledPackage]
	FlowList          ListView[sf.Flow]
	QueueList         ListView[sf.QueueRow]
	PublicGroupList   ListView[sf.PublicGroupRow]
	DashboardList     ListView[sf.DashboardRow]
	ReportTypeList    ListView[sf.ReportTypeRow]
	CommunityList     ListView[sf.CommunityRow]
	CommunityPageList ListView[sf.CommunityPageRow]

	// DevProjectItems is the items list for the currently-drilled
	// dev project on this org. Replaces the legacy []devproject.Item
	// stored on Model so sort/search/cursor machinery (which assumes
	// per-org state) just works.
	DevProjectItems      ListView[devproject.Item]
	DevProjectItemsTable uilayout.ListTableState

	// One ListTableState per "flat" list-table surface so widths /
	// scroll / sort persist across renders without bleeding between
	// surfaces. All per-org.
	ObjectsTableState       uilayout.ListTableState
	FlowsTableState         uilayout.ListTableState
	ApexLogsTableState      uilayout.ListTableState
	SetupAuditTableState    uilayout.ListTableState
	FlowInterviewTableState uilayout.ListTableState
	ActiveUserTableState    uilayout.ListTableState
	AsyncJobTableState      uilayout.ListTableState
	ScheduledJobTableState  uilayout.ListTableState
	UserSessionTableState   uilayout.ListTableState
	DeploysTableState       uilayout.ListTableState
	PackagesTableState      uilayout.ListTableState
	RecentTableState        uilayout.ListTableState
	QueuesTableState        uilayout.ListTableState
	PublicGroupsTableState  uilayout.ListTableState
	DashboardsTableState    uilayout.ListTableState
	ReportTypesTableState   uilayout.ListTableState
	CommunityTableState     uilayout.ListTableState
	CommunityPageTableState uilayout.ListTableState

	// /deploys chip strip + drill-in state. DeployDetailMap is the
	// per-id lazy detail cache (same shape as LWCDetail); DeployCur
	// is the drilled deploy id; DeployDetailCursor is the row cursor
	// inside the detail pane.
	DeploysChipIdx     int
	ActiveUsersChipIdx int
	DeployCur          string
	DeployDetailCursor int
	DeployDetailMap    map[string]*Resource[sf.DeployDetail]
}

// --- Records surfaces -----------------------------------------------------

type orgDataRecordsData struct {
	// Records holds per-sobject record lists; lazily allocated when the
	// user drills into /records → sObject. Short TTL since records move.
	Records map[string]*Resource[sf.RecordsList]

	// ChipRecords holds per-(sobject:lensId) record lists for the
	// active sf-deck lens. Each chip in Local mode gets its own
	// Resource so switching chips doesn't clobber the previous
	// fetch. Same NoCache + short TTL as Records — records are
	// privacy-sensitive live data.
	ChipRecords map[string]*Resource[sf.RecordsList]

	// ChipRecordsSearch is the per-(sobject:chipId) search buffer for
	// records lists. Sticky across navigation: when the user `/`-s a
	// query, switches tabs, and comes back, the buffer (and therefore
	// the filtered view) is still there. Esc / SearchClear empties it.
	// One buffer per chip so each lens remembers its own filter.
	ChipRecordsSearch map[string]*searchState

	// RecordsTableState is the per-(sobject:chipId) list-table view
	// state — horizontal scroll position, user-pinned column widths,
	// zen flag. Persists across renders so resizes / scrolls survive
	// chip switches and tab navigation. See uilayout.ListTableState.
	RecordsTableState map[string]*uilayout.ListTableState

	// visibleRecordsCache memoises (visible, visibleIdx) per
	// (sobject, chipID) so visibleRecordsAndIdx — called multiple
	// times per render via cursor + sidebar + breadcrumb +
	// openable resolution — costs O(1) on a steady-state wheel
	// burst instead of O(N×fields). See tab_records_dashboard.go
	// for the cache-identity rules. In-memory only; never written
	// to the on-disk cache (records are NoCache by design).
	visibleRecordsCache visibleRecordsCache

	// recordsProjectionCache memoises the expensive records table
	// projection: stringified cell matrix + measured ListColumn spec.
	// This is intentionally separate from visibleRecordsCache: knowing
	// which rows are visible is cheap after the memo, but records still
	// need dynamic column widths derived from every visible cell.
	recordsProjectionCache recordsProjectionCache

	// RecordDetails is the per-(sobject:id) Resource map backing the
	// /record drill-in tab. Lazily allocated on first drill. Cached
	// like other Resources (TTL via "record_detail" key); user `r`
	// re-fetches.
	RecordDetails map[string]*Resource[map[string]any]

	// RecordReferenceNames is the per-record map of source-field
	// API name → resolved related-record Name. Populated by one
	// SOQL with relationship traversal alongside the main record
	// fetch. Drives the "→ Account Acme Corp" annotation on the
	// RELATIONSHIPS section of the record detail page.
	RecordReferenceNames map[string]*Resource[map[string]string]

	// RecordChildCounts is the per-record map of child-relationship
	// name → count. Populated by a Composite batch of COUNT()
	// queries (capped at sf.MaxChildRelationshipCounts). Drives
	// the RELATED panel ("5 Opportunities, 12 Contacts").
	RecordChildCounts map[string]*Resource[map[string]int]
	// RecordDetailCur is the composite "<sobject>:<id>" key the drill
	// tab is currently showing. Empty when no record is drilled into.
	RecordDetailCur string

	// EditSessions tracks active inline-edit state per
	// "<sobject>:<id>" — dirty fields, in-progress edit, save state,
	// per-field error messages. Lazily allocated on first edit
	// gesture; cleared after a successful PATCH triggers a record
	// re-fetch. Lives on orgData so multi-org sessions don't bleed
	// edits across orgs.
	EditSessions map[string]*recordEditSession

	// RecordFieldCursor is the API name of the field row currently
	// highlighted on /record. Drives j/k movement + which field
	// the `e` key targets. Empty = no row highlighted (default
	// state on first drill); j/k initialises to the first field.
	RecordFieldCursor map[string]string

	// RecordFindBuffer is the live find-input buffer per record key
	// ("<sobject>:<id>"). Find-next (not filter) semantics: each
	// keystroke jumps the cursor to the next field whose API name,
	// Label, or value contains the substring. Empty = find inactive
	// for that record.
	RecordFindBuffer map[string]string

	// RecordFindActive records whether the find pill is in
	// editing-focus state per record. When true, keystrokes go to
	// the buffer + live-search; when false (e.g. after Enter
	// commits), n / N cycle matches and other keys fire their
	// normal behaviour.
	RecordFindActive map[string]bool

	// /records tab state: the cursored sobject in the picker, and once
	// chosen, the cursored row in the record list.
	RecordsPickerSearch searchState // search within the sobject picker
	RecordsSObjectCur   string      // "" = in picker mode; else name of drilled-in sObject

	// ListViews is the Salesforce-defined list-view catalog per sobject
	// (e.g. "My Accounts", "New Last Week"). Resource[[]sf.ListView].
	// Lazily allocated on first Records subtab view.
	ListViewsPerSObject map[string]*Resource[[]sf.ListView]

	// ListViewResults is the current-user's rendered list-view results,
	// keyed by "<sobject>:<listViewId>". NoCache — results change
	// constantly (same reasoning as records).
	ListViewResults map[string]*Resource[sf.ListViewResult]

	// RecentlyViewedPerSObject is the per-sObject "what has this user
	// recently viewed for this object" slice from Salesforce.  Backs
	// the SF-mode synthetic Recently Viewed chip on /records and
	// /objects/<X>/Records.
	//
	// Distinct from d.RecentlyViewed (the global top-N cross-object
	// payload) because that one's capped at RecentMaxEntries (~50)
	// across ALL sObjects — a user who's viewed dozens of Accounts
	// would find Request__c never appears even though SF has them.
	// This per-sObject resource queries the RecentlyViewed table
	// with WHERE Type = '<sobject>' so the slice is dedicated to one
	// object and never starved by others.
	RecentlyViewedPerSObject map[string]*Resource[[]sf.RecentlyViewedRow]

	// Networks lists the org's Live Experience Cloud sites. Sourced
	// from `SELECT … FROM Network WHERE Status = 'Live'` and used by
	// the Contact ^O menu to offer "Log in to <site> as user" targets
	// that resolve to /servlet/servlet.su with sunetwork* params.
	// Lazily ensured the first time the Contact open-menu is built.
	Networks *Resource[[]sf.Network]

	// CommunityUserByContact memoises the per-session lookup of "does
	// this Contact have an active community User?" keyed by ContactId.
	// Empty value = checked, no active community user; missing key =
	// never checked. Backs the gating of "Log in to community as user"
	// targets so we don't enqueue per-network targets for contacts
	// without a portal user.
	CommunityUserByContact map[string]string

	// ListViewCur tracks which chip is selected per sobject on the
	// Records subtab. Value is a lens id ("recent", "today", …) when
	// the mode is Local, or a Salesforce ListView Id ("00B...") when
	// the mode is Salesforce. The mode itself is in LensMode.
	ListViewCur map[string]string

	// ChipMode picks the chip-strip source per sobject: ChipModeLocal
	// = sf-deck-defined chips, ChipModeSalesforce = the org's own list
	// views. Default Local. Stored per sobject so the user's last
	// choice sticks per object.
	ChipMode map[string]ChipMode
}

// --- Per-sObject metadata ------------------------------------------------

type orgDataMetadata struct {
	// Describes are per-sobject; lazily allocated.
	Describes      map[string]*Resource[sf.SObjectDescribe]
	DescribeFields map[string]*describeFieldState

	// CustomObjectBaselines caches the Tooling-CustomObject record
	// per sObject — gives the boolean object-toggle modals their
	// "current value" so the cursor preselects the right option.
	// Lazily fetched on first need (sidebar render or modal open);
	// the deploy preview path also reads from here when present.
	CustomObjectBaselines map[string]*Resource[*sf.CustomObjectBaseline]

	// FlowVersions holds per-definition version lists; lazily allocated.
	FlowVersions map[string]*Resource[[]sf.FlowVersion]

	// FlowVersionDetail holds per-version definition metadata (the JSON
	// shown by the in-terminal version viewer); keyed by version Id.
	FlowVersionDetail map[string]*Resource[map[string]any]

	// Validation, RecordTypes, Triggers are per-sobject child-entity
	// state — list + cursor + drilled detail + drilled-id. See
	// SObjectChildren. Wired in newOrgData.
	ValidationRules SObjectChildren[sf.ValidationRuleRow, sf.ValidationRuleDetail]
	RecordTypes     SObjectChildren[sf.RecordTypeRow, sf.RecordTypeDetail]
	PageLayouts     SObjectChildren[sf.PageLayoutRow, struct{}]
	ObjectFlows     SObjectChildren[sf.ObjectFlowRow, struct{}]
	Triggers        SObjectChildren[sf.TriggerRow, sf.TriggerDetail]

	// customIDMu guards CustomFieldIDs + CustomObjectIDs — the ONLY
	// orgData maps touched off the Update goroutine. The edit-modal
	// LoadCurrent/Save closures run on tea.Cmd goroutines and resolve
	// Tooling IDs through customFieldIDCached / customObjectIDCached,
	// which read AND write these maps, while the main loop deletes
	// entries (field delete) — an unlocked concurrent map access is a
	// FATAL runtime error, not a recoverable panic. Everything else on
	// orgData stays main-loop-only and needs no lock.
	customIDMu sync.Mutex

	// CustomFieldIDs caches Tooling-API CustomField.Id lookups per
	// "<sobject>.<fieldDevName>" key. The ID is stable for the life
	// of the field, so a single lookup per session is enough.
	// Guarded by customIDMu — access via customFieldIDCached or with
	// the lock held.
	CustomFieldIDs map[string]string

	// FieldDescriptions caches the CustomField.Metadata.description a
	// field carries, keyed "<sobject>.<fieldName>". The describe API
	// doesn't return field descriptions, so the field-detail page
	// fetches them lazily via Tooling on drill-in. Value "" means
	// "fetched, empty"; a missing key means "not fetched yet". Only
	// custom fields are fetched (standard fields have no editable
	// Setup description).
	FieldDescriptions map[string]string

	// CustomObjectIDs caches Tooling-API CustomObject.Id lookups
	// per sobject API name. Same rationale + lifespan as CustomFieldIDs.
	// Guarded by customIDMu — access via customObjectIDCached or with
	// the lock held.
	CustomObjectIDs map[string]string
}

// --- Permissions ---------------------------------------------------------

type orgDataPerms struct {
	// PermissionSets is the shared picker scope for the FLS grid
	// (every profile + permset in the org). Cached org-wide
	// because every object's FLS grid uses the same list.
	PermissionSets Resource[[]sf.FLSPickerEntry]
	// FLS is per-(sobject, permset-id) — the keyed cache map uses
	// "<sobject>:<parentID>" as the key. Lazily allocated on first
	// FLS subtab view for a given combo.
	FLS map[string]*Resource[[]sf.FieldPermissionRow]
	// FLSParentID is the permset-Id the user has picked as the
	// current scope for the FLS grid. Empty = none selected yet.
	FLSParentID string

	// /perms tab state — full-fidelity PermissionSet / PSG / Profile
	// lists. Each has its own ListView wrapper with an org-wide scope.
	PermSets Resource[[]sf.PermissionSet]
	PSGs     Resource[[]sf.PermissionSetGroup]
	Profiles Resource[[]sf.Profile]

	PermSetList ListView[sf.PermissionSet]
	PSGList     ListView[sf.PermissionSetGroup]
	ProfileList ListView[sf.Profile]

	PermSetsTableState uilayout.ListTableState
	PSGsTableState     uilayout.ListTableState
	ProfilesTableState uilayout.ListTableState

	// TabPermParentDetail drill-in identity. PermParentKind is one of
	// "permset" | "psg" | "profile"; PermParentID is the corresponding
	// record's Id; PermParentPermSetID is the PermissionSet Id used as
	// ParentId for ObjectPermissions / FieldPermissions / system
	// perms. For permsets this is equal to PermParentID; for profiles
	// it's the implicit permset Id (resolved at drill time); for PSGs
	// it's blank (a PSG doesn't have a direct perm-parent — writes
	// target its component permsets instead).
	PermParentKind      string
	PermParentID        string
	PermParentPermSetID string

	// Per-parent subtab index on TabPermParentDetail.
	PermParentSubtab int

	// ObjectPerms holds per-(kind:parentID) ObjectPermissions lists.
	// Key shape: "<kind>:<parentID>" e.g. "permset:0PS..."
	// Lazily allocated on first Objects subtab view for a given parent.
	ObjectPerms map[string]*Resource[[]sf.ObjectPermission]

	// SystemPerms holds per-parentID system-permission lists.
	// Key: the PermissionSet Id (PermParentPermSetID).
	// Lazily allocated on first System subtab view.
	SystemPerms map[string]*Resource[[]sf.SystemPermission]

	// GroupMembers holds per-Group.Id member lists. Used by both
	// /perms Queues and /perms Public Groups detail tabs (Queues
	// are stored as Group rows with Type='Queue', so the lookup is
	// uniform). Keyed by Queue.Id / PublicGroup.Id.
	GroupMembers     map[string]*Resource[[]sf.GroupMemberRow]
	GroupMemberList  map[string]ListView[sf.GroupMemberRow]
	GroupMemberState map[string]*uilayout.ListTableState
	// GroupMemberDrill is the (kind, id) the user has drilled
	// into — kind ∈ {"queue", "public_group"}, id is the parent
	// Group/PublicGroup id. Populated on drill, read by the
	// renderer + cursor handlers.
	GroupMemberKind string
	GroupMemberID   string

	// AssignedUsers holds per-parentID assignment lists.
	// Key: the PermissionSet Id (PermParentPermSetID).
	AssignedUsers map[string]*Resource[[]sf.PermissionSetAssignment]

	// PermFieldsSObject is the currently-selected sObject for the
	// Fields subtab on TabPermParentDetail. Empty = no object selected.
	PermFieldsSObject string

	// Per-parent search state for the Object-perms and System-perms
	// grids. Kept separate from the grids' Resource[] so the buffer
	// survives re-fetches. Keys mirror the grid cache keys above.
	ObjPermSearch map[string]*searchState
	SysPermSearch map[string]*searchState

	// /perms dashboard subtab index (which of PermSets/PSGs/Profiles
	// is selected).
	PermsDashboardSubtab int
}

// --- /users · All users + User detail ------------------------------------

type orgDataUsers struct {
	// ChipUsers holds per-chip-id User lists for /users · All users.
	// Each chip's predicate compiles to SOQL via qchip.ApplyToSOQL,
	// so the server filters and the cap (LIMIT in the chip's Query)
	// applies inside the chip's filter rather than across the whole
	// org. Mirrors ChipRecords. NoCache + short TTL — User data is
	// live + privacy-sensitive.
	ChipUsers           map[string]*Resource[sf.UsersList]
	ChipUsersList       map[string]*ListView[sf.UserRow]
	ChipUsersTableState map[string]*uilayout.ListTableState

	// AllUsersChipIdx is the active chip cursor on /users · All
	// users. Per-chip data lives in ChipUsers; this is just the
	// strip's selection.
	AllUsersChipIdx int
	// ActiveUsersChipID is the resolved chip id at AllUsersChipIdx
	// against the live strip (favourites + imports + transient).
	// Written by setAllUsersChipIdx, which has Model access and can
	// walk the registry; read by listSurface closures that only see
	// orgData. Resolving here once on cursor change avoids every
	// ensure/render/resource lookup re-walking the registry, and
	// avoids the index-into-qchip.UserBuiltins bug that misrouted
	// imported / reordered chips.
	ActiveUsersChipID string
	// userScore is the relevance-ranker closure paired with
	// userMatch — both installed on every per-chip ListView so
	// search results in /users · All users get exact-match-first
	// ordering.
	userScore func(sf.UserRow, string) int

	// userMatch is the search-matcher closure shared by every
	// per-chip user ListView wired via EnsureChipUsers. Cached on
	// orgData so each chip's ListView reuses the same matcher
	// without re-allocating the field-resolver chain per fetch.
	userMatch func(sf.UserRow, string) bool

	// UserCur is the User Id drilled into on TabUserDetail.
	UserCur string
	// UserActionCur is the highlighted row in the User detail action
	// menu. Bounded by the renderer.
	UserActionCur int
	// UserDetailRows caches per-user freshly-fetched rows so the
	// detail card reflects the latest server state (e.g. after a
	// deactivate). Lazily allocated; missing entry → fall back to the
	// list-row data already on screen.
	UserDetailRows map[string]sf.UserRow
	// UserLoginRows caches per-user UserLogin rows (Freeze state +
	// row Id needed to PATCH). Empty UserLoginRow means we tried but
	// the row doesn't exist (user has never logged in); a missing
	// map entry means we haven't fetched yet.
	UserLoginRows map[string]sf.UserLoginRow
	// UserLoginHist + UserAccessMap cache the detail drill's audit
	// sections (recent login attempts incl. failures; permset +
	// group memberships). Refetched on every drill, same lifecycle
	// as UserDetailRows.
	UserLoginHist map[string][]sf.LoginHistoryRow
	UserAccessMap map[string]sf.UserAccess

	UsersSubtab int // /users subtab index
}

// --- /home subtab lists + table states -----------------------------------

type orgDataHome struct {
	Home    Resource[HomeData]
	OrgInfo Resource[sf.OrgInfo] // Organization sObject — singleton metadata, drives the home tab's identity card

	// Bell-icon notifications — the unified Connect API stream
	// covering chatter mentions, approvals, shares, custom
	// notifications, etc. Lives on Home as the "Notifications"
	// subtab and exposes the unread count for the header pill.
	Notifications Resource[sf.NotificationsList]

	// Home subtab list wrappers — one ListView per cursored Home
	// subtab so each gets its own search/filter buffer + cursor.
	// Underlying data lives on Home / Packages / Notifications
	// resources; SyncListViews fans the values out into these
	// wrappers on every fresh fetch.
	HomeNotifList   ListView[sf.Notification]
	HomeLimitList   ListView[KeyLimit]
	HomeUserList    ListView[sf.UserRow]
	HomeLicenseList ListView[homeLicenseRow]

	// Per-Home-subtab list-table state (column-mode, sort, scroll).
	HomeNotifTableState   uilayout.ListTableState
	HomeLimitTableState   uilayout.ListTableState
	HomeUserTableState    uilayout.ListTableState
	HomeLicenseTableState uilayout.ListTableState

	HomeSubtab int

	// HomeRecentMode toggles /home → Recent between two sources:
	//
	//   ChipModeLocal      — sf-deck's local visit log (d.Recent).
	//                        Default.  Answers "what was I doing in
	//                        sf-deck recently?"
	//   ChipModeSalesforce — Salesforce's server-side RecentlyViewed
	//                        list.  Answers "what's hot in this org
	//                        right now (Lightning, mobile, API,
	//                        sf-deck combined)?"
	//
	// Flipped by Keys.LensModeToggle when /home → Recent is the
	// active subtab.  Persists per-org so each org remembers its
	// preferred default.  Replaces the previous merged-stream design
	// which forced users to mentally filter by Origin glyph.
	HomeRecentMode ChipMode
}

// --- /recent + RecentlyViewed -------------------------------------------

type orgDataRecent struct {
	// RecentlyViewed is the server-side "things this user recently
	// viewed" log from Salesforce. Records-only (Salesforce only
	// tracks records there), but global — captures Lightning, mobile,
	// API, sf-deck, etc. Surfaced via a chip on /recent so the
	// client-side log + the server's log live side by side.
	RecentlyViewed     Resource[[]sf.RecentlyViewedRow]
	RecentlyViewedList ListView[sf.RecentlyViewedRow]

	// Recent is the per-org "recently visited" log — currently
	// records only; reports / flows / dashboards land here later.
	// MRU order; capped to settings.RecentMaxEntries() on every
	// visit. Loaded from settings on first ensureOrgData; saved on
	// every visit.
	Recent       []RecentEntry
	RecentList   ListView[RecentEntry]
	RecentLoaded bool // lazy-load guard so settings hits at most once per org per session

	// RecentSFList is the ListView wrapping the Salesforce
	// RecentlyViewed payload converted to []RecentEntry.  Drives
	// /home → Recent when d.HomeRecentMode == ChipModeSalesforce.
	// Rebuilt by syncRecentSFList whenever the SF payload changes
	// (Resource.Apply bumps recentGen).
	RecentSFList ListView[RecentEntry]
	// recentSFGen tracks the d.recentGen value at the time
	// RecentSFList was last refreshed.  When recentGen advances
	// past this we know the list needs a rebuild.  Lets us avoid
	// the per-render rebuild that the old merged stream paid for.
	recentSFGen uint64

	// recentGen is a monotonically-increasing counter bumped every
	// time the visit-stream's ORDER could have changed: a new local
	// visit (rememberRecent) or a fresh SF RecentlyViewed payload
	// (recently_viewed Apply branch).  Used as the recency
	// fingerprint by cache keys that need to invalidate when the
	// stream re-orders without changing length — e.g. an existing
	// entry moves to MRU via upsertRecent, or a new visit at cap
	// shifts the tail off without changing len.  Length-based
	// fingerprints miss both.
	recentGen uint64

	RecentChipIdx int
}

// --- Apex / LWC / Aura code + detail caches ------------------------------

type orgDataCode struct {
	// ApexClasses + LWCBundles are list-scoped per org; bodies are
	// fetched lazily on drill-in (see ApexClassDetailRes / LWCDetailRes
	// further down — keyed maps so multiple drills cache side-by-side).
	ApexClasses      Resource[[]sf.ApexClassRow]
	ApexTriggersFlat Resource[[]sf.TriggerRow] // flat cross-sObject list for /apex's Triggers chip
	LWCBundles       Resource[[]sf.LWCBundle]
	AuraBundles      Resource[[]sf.AuraBundle]

	ApexClassList   ListView[sf.ApexClassRow]
	ApexTriggerList ListView[sf.TriggerRow] // populated when /apex's Triggers chip is on
	LWCBundleList   ListView[sf.LWCBundle]
	AuraBundleList  ListView[sf.AuraBundle]

	// /apex + /components list-table state — used by activeListTable
	// so c (column-mode) + z (zen) + s (sort) work on those tabs.
	ApexClassesTableState  uilayout.ListTableState
	ApexTriggersTableState uilayout.ListTableState
	LWCBundlesTableState   uilayout.ListTableState
	AuraBundlesTableState  uilayout.ListTableState

	// Per-id detail caches for drill-in. Keyed by the record Id so
	// switching between classes / bundles doesn't refetch already-loaded
	// bodies. Lazy-allocated.
	ApexClassDetail map[string]*Resource[sf.ApexClassDetail]
	LWCDetail       map[string]*Resource[sf.LWCBundleDetail]
	AuraDetail      map[string]*Resource[sf.AuraBundleDetail]
	// Cursor target for /apex-detail and /components-detail.
	ApexCur string
	LWCCur  string // the active drill ID — applies to whichever kind is selected
	// ComponentsKind toggles between "lwc" and "aura" on the
	// /components surface; "apex" / "triggers" on /apex. Defaults to
	// "" (= "lwc" / "apex" respectively).
	ComponentsKind string
	ApexKind       string

	// Code-body viewport state, keyed by an opaque body id chosen
	// by the renderer. Apex / trigger detail use the entity Id;
	// LWC / Aura use "<bundleId>:<filePath>" so each file in a
	// bundle keeps its own cursor + scroll. Maps are lazy-allocated.
	BodyCursor map[string]int
	BodyScroll map[string]int
	// BodyHScroll is the horizontal scroll offset (display cells) per
	// code body — ←/→ on a code surface shift the view for lines that
	// run off the right edge. Reset is just scrolling back to 0.
	BodyHScroll map[string]int

	// CodeFind is the per-body in-code find state (/ on a code
	// surface): query buffer, input focus, current match index, plus
	// the match memo so the per-frame render never re-scans the body.
	CodeFind map[string]*codeFindState

	// CodeViewLast records which code body the most recent paint
	// rendered (tab + subtab + body id + body). Key handlers act on
	// exactly what is on screen instead of re-deriving each tab's
	// resource lookup; stale entries are gated by the tab/subtab match.
	CodeViewLast codeViewLastPaint

	// LWCFileIdx tracks the active file within an LWC / Aura bundle
	// — index into the resource list as ordered by the renderer.
	// Persisted per bundle so cycling away and back keeps the user
	// on the file they were reading.
	LWCFileIdx map[string]int

	ApexSubtab          int // /apex subtab index
	ComponentsSubtab    int // /components subtab index
	ApexChipIdx         int
	DashboardsChipIdx   int
	ReportTypesChipIdx  int
	ApexTriggersChipIdx int
	LWCChipIdx          int
	AuraChipIdx         int
}

// --- Reports + folder tree state ----------------------------------------

type orgDataReports struct {
	Reports     Resource[[]sf.ReportSummary]
	Dashboards  Resource[[]sf.DashboardRow]
	ReportTypes Resource[[]sf.ReportTypeRow]
	ReportList  ListView[sf.ReportSummary]

	// ReportRuns holds per-report-id cached runs (the inline preview).
	// Lazily allocated when the user drills into a report. Each is its
	// own Resource so switching reports doesn't clobber the previous
	// run; same NoCache discipline as records — runs are live data.
	ReportRuns map[string]*Resource[sf.ReportRun]
	// ReportCur is the report id currently drilled into on the Reports
	// detail tab. Empty when on the list.
	ReportCur string

	// ReportFolders is the treechip Registry that drives the /reports
	// folder navigation. Lazily allocated on first /reports entry per
	// org. The underlying TreeSource (sources.ReportFolderSource)
	// fetches every Folder record once and caches it in memory — see
	// the package's design doc (vault: research/treechip-design.md).
	ReportFolders       *treechip.Registry
	ReportFoldersSrc    *sources.ReportFolderSource
	ReportFoldersLoaded bool // lazy-init guard

	ReportsSubtab int
}

// --- /meta long-tail metadata surfaces -----------------------------------

type orgDataMeta struct {
	// Browse: full type catalogue (one describeMetadata call) +
	// lazily-listed components per type (one SOAP listMetadata each,
	// keyed so revisits don't refetch).
	MetaTypes     Resource[[]sf.MetadataTypeInfo]
	MetaTypesList ListView[sf.MetadataTypeInfo]
	MetaTypeCur   string // drilled type (TabMetaTypeDetail)
	MetaTypeItems map[string]*Resource[[]sf.MetadataItem]
	// MetaTypeItemList holds the drilled type's components; re-Set
	// on every drill (single list reused across types).
	MetaTypeItemList ListView[sf.MetadataItem]

	// Rich subtabs — one SOQL each.
	CustomLabels    Resource[[]sf.CustomLabelRow]
	CMTTypes        Resource[[]sf.MetaEntityRow]
	CustomSettings  Resource[[]sf.MetaEntityRow]
	StaticResources Resource[[]sf.StaticResourceRow]
	NamedCreds      Resource[[]sf.NamedCredentialRow]
	RemoteSites     Resource[[]sf.RemoteSiteRow]

	CustomLabelList    ListView[sf.CustomLabelRow]
	CMTList            ListView[sf.MetaEntityRow]
	CustomSettingList  ListView[sf.MetaEntityRow]
	StaticResourceList ListView[sf.StaticResourceRow]
	NamedCredList      ListView[sf.NamedCredentialRow]
	RemoteSiteList     ListView[sf.RemoteSiteRow]

	MetaTypesTableState       uilayout.ListTableState
	MetaTypeItemsTableState   uilayout.ListTableState
	CustomLabelsTableState    uilayout.ListTableState
	CMTTableState             uilayout.ListTableState
	CustomSettingsTableState  uilayout.ListTableState
	StaticResourcesTableState uilayout.ListTableState
	NamedCredsTableState      uilayout.ListTableState
	RemoteSitesTableState     uilayout.ListTableState
}

// --- DevProjects + bundle drill -----------------------------------------

type orgDataDevProjects struct {
	// LoadedDevProjectID is the dev project the user has explicitly
	// loaded for THIS org. Empty when none. Drives the auto-pinned
	// project chip on records / objects / flows / reports surfaces,
	// the header pill, and the K-collect fast-path. Persisted to
	// settings (per-org) so it survives sf-deck restart.
	//
	// "Per-org" still applies even though the project itself is
	// cross-org: switching from dev sandbox to prod can plausibly
	// mean switching projects too (e.g. "I'm working on Q2 here, on
	// QA-cleanup there"). The Scope it produces is org-filtered.
	LoadedDevProjectID string
	// LoadedScope is the hydrated Scope for LoadedDevProjectID,
	// filtered to this org's items only. Re-hydrated when the loaded
	// id changes or items are added via K-collect. nil when nothing's
	// loaded.
	LoadedScope *orgproject.Scope
	// ReportsProjectMode is the /reports surface flag: when true the
	// report list ignores the folder breadcrumb and shows only the
	// loaded project's reports. Toggled by activating the synthetic
	// 📁 pin in the report-folder strip. Reset on tab change away
	// from /reports or when the project is unloaded.
	ReportsProjectMode bool

	// BundleCursor is the row cursor on /bundles for the active
	// DevProject. Lives on orgData (per-org cursor pool) so revisits
	// restore position. The bundles themselves are global to the
	// DevProject — the cursor is just per-org viewing state.
	BundleCursor int

	// DevProjectsSubtab + DevProjectDetailSubtab — index into
	// devProjectsSubtabs() / devProjectDetailSubtabs() respectively.
	// Per-org so two orgs can be on different tabs concurrently.
	DevProjectsSubtab      int
	DevProjectDetailSubtab int

	// AllBundlesCursor is the cursor on the top-level /dev-projects
	// → Bundles subtab (every bundle across every DevProject).
	AllBundlesCursor int
}

// --- SOQL Saved + History library ---------------------------------------

type orgDataSOQLLibrary struct {
	// SOQL Library state lives per-org rather than on Model so the
	// existing chip + listSurface contracts (which thread *orgData
	// through every callback) work without a special case. The
	// underlying data is org-agnostic — the snapshots are loaded
	// fresh per-org from the same devproject store, and reads are
	// cheap, so duplicating in memory is fine.
	SOQLSavedList     ListView[devproject.SavedQuery]
	SOQLSavedLoaded   bool
	SOQLSavedTable    uilayout.ListTableState
	SOQLHistoryList   ListView[devproject.SOQLHistoryEntry]
	SOQLHistoryLoaded bool
	SOQLHistoryTable  uilayout.ListTableState

	SOQLSavedChipIdx   int
	SOQLHistoryChipIdx int

	// soqlRenderCache memoises the SOQL results-grid projection:
	// dynamic column spec, pre-rendered cell matrix (column-major),
	// and the post-search-filter row slice. Mirrors
	// recordsProjectionCache — same shape, same purpose, same lifetime.
	//
	// Sits on orgData (not Model) so the cache is pointer-stable
	// across the value-receiver Model copy that every Update + render
	// produces. Every per-frame caller of soqlProjectionFor — body
	// renderer, listTableSOQL (wheel routing / sidebar / status /
	// zen check), measureCellSOQL — hits the same cache.
	//
	// In-memory only; never written to the on-disk cache. SOQL
	// results carry record content (privacy-sensitive); the parent
	// Resource is NoCache for that reason and this projection memo
	// preserves the property.
	soqlRenderCache soqlRenderCache
}

// soqlRenderCache is the per-orgData SOQL projection memo. Single-key
// map ("soql") today because we only ever render one SOQL result set
// at a time, but keyed so future "save+reopen N results in tabs" work
// drops in without changing the type. Lazily allocated on first use.
type soqlRenderCache map[string]*soqlRenderEntry

// --- Anonymous-Apex Saved + History library -----------------------------

type orgDataExecLibrary struct {
	// Mirror of orgDataSOQLLibrary for /exec: saved snippets and
	// execution history. Lives per-org for the same listSurface-
	// contract reason — closures that take *orgData need the
	// library wrappers without going through Model.
	ExecSavedList     ListView[devproject.SavedApex]
	ExecSavedLoaded   bool
	ExecSavedTable    uilayout.ListTableState
	ExecHistoryList   ListView[devproject.ApexHistoryEntry]
	ExecHistoryLoaded bool
	ExecHistoryTable  uilayout.ListTableState

	ExecSavedChipIdx   int
	ExecHistoryChipIdx int
}

// --- Per-org nav + cursors ----------------------------------------------

type orgDataNav struct {
	SObjectFilter  SObjectFilter
	DescribeCur    string // API name of the currently-open describe
	FieldCur       string // API name of the currently-open field (on the field-detail page)
	FlowCur        string // DefinitionId of the currently-open flow
	FlowVersionCur string // Tooling Id of the flow version drilled into (version viewer)
	// flowVersionsLoadedFor is the FlowCur whose versions were last
	// force-refreshed on drill-in. ensureFlowDetailData refreshes when
	// FlowCur differs from this (a genuine drill into a NEW flow), and
	// falls back to Ensure (cache-respecting) on intra-family returns
	// like esc-back from the version viewer — so drilling in always
	// shows fresh versions without re-fetching on every navigation.
	flowVersionsLoadedFor string

	// Per-org UI state — persists across org switches so each org
	// remembers where the user was. Defaults mirror Model.noOrgTab etc.
	Tab                 Tab
	ObjectsChipIdx      int
	RecordsChipIdx      int
	FlowsChipIdx        int
	PermSetsChipIdx     int
	PSGsChipIdx         int
	ProfilesChipIdx     int
	QueuesChipIdx       int
	PublicGroupsChipIdx int
	ObjectSubtab        int
	SystemSubtab        int
	MetaSubtab          int

	// LastTabInStem remembers the last tab the user was on for each
	// stem family (TabObjects → TabFieldDetail, TabFlows → TabFlowDetail,
	// etc). Lets number-key nav restore drill-in state when the user
	// returns to a family after visiting another one.
	LastTabInStem map[Tab]Tab

	// DrillReturnTab maps a detail tab → the tab Esc should pop back
	// to.  Set by drillByKind from the originator argument; consumed
	// by the Esc dispatcher.  Lets drill from /home Recent (or future
	// global search) return to the originator instead of the
	// resource's static stem.  Empty map / missing entry → fall back
	// to the static EscBack on the detail tab's TabSpec.
	//
	// Last-write wins per detail tab; drilling the same flow from
	// /home then later from /flows updates the entry so Esc honours
	// the most recent origin.
	DrillReturnTab map[Tab]Tab

	// Cursors is the unified row-index cursor store covering FLS,
	// ObjectPerms, SystemPerms, AssignedUsers, RecordsRow, and
	// FlowVersion cursors. Single source of truth for "which row
	// is highlighted in <list>" — see internal/ui/cursors.go.
	Cursors CursorStore

	// gutterCache memoises the per-render bulk tag/project lookups
	// keyed by the items slice header pointer + the devproject store's
	// mutation generation. Without it, every wheel tick on a 5000-row
	// list re-runs two SQLite queries and allocates 10000 keys to
	// build the wanted-set — visible scroll lag.
	//
	// Cache is invalidated when:
	//   - the items slice address changes (Set on the wrapping
	//     ListView replaced the underlying slice), or
	//   - the store's Generation() advances (a tag was applied/
	//     removed, an item was collected/uncollected, a project was
	//     created/deleted).
	//
	// Per-(domain) entries because each surface's items have a
	// different element type but a common (Kind, Ref) projection.
	// The cache lookup key is "<domain-discriminator>" so /objects
	// and /flows don't collide.
	gutterCache *gutterCacheState

	// noteMemo caches the cursored item's note body so the per-frame
	// sidebar render doesn't hit SQLite on every wheel tick. Keyed by
	// the item identity + store generation — a cursor move or any
	// store write invalidates it. See Model.cursorNoteBody.
	noteMemo *noteMemoEntry
}

// noteMemoEntry is one cached note lookup (see orgDataNav.noteMemo).
type noteMemoEntry struct {
	key        string // kind \x00 ref \x00 orgUser
	generation int
	body       string
}
