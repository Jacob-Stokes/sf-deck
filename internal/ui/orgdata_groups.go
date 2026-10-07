package ui

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

type orgDataCore struct {
	username string
	target   string // sf CLI / REST target captured by Fetch closures (alias when present, else username)
	cache    *cache.Cache
	settings *settings.Settings
}

type orgDataTopLists struct {
	SObjects       Resource[[]sf.SObject]
	ApexLogs       Resource[[]sf.ApexLogRow]
	SetupAudit     Resource[[]sf.SetupAuditRow]
	FlowInterviews Resource[[]sf.FlowInterviewRow]
	ActiveUsers    Resource[[]sf.ActiveUserRow]
	AsyncJobs      Resource[[]sf.AsyncJobRow]
	ScheduledJobs  Resource[[]sf.CronTriggerRow]

	SessionUserID   string
	SessionUserName string
	UserSessions    map[string]*Resource[[]sf.SessionRow]
	Deploys         Resource[[]sf.DeployRow]
	Packages        Resource[[]sf.InstalledPackage]
	Flows           Resource[[]sf.Flow]
	Queues          Resource[[]sf.QueueRow]
	PublicGroups    Resource[[]sf.PublicGroupRow]
	Community       Resource[[]sf.CommunityRow]

	CommunityCur     string
	CommunityCurName string
	CommunityCurID   string
	CommunityPages   map[string]*Resource[[]sf.CommunityPageRow]

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

	DeploysChipIdx     int
	ActiveUsersChipIdx int
	DeployCur          string
	DeployDetailCursor int
	DeployDetailMap    map[string]*Resource[sf.DeployDetail]
}

type orgDataRecordsData struct {
	Records map[string]*Resource[sf.RecordsList]

	ChipRecords map[string]*Resource[sf.RecordsList]

	ChipRecordsSearch map[string]*searchState

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

	RecordDetails map[string]*Resource[map[string]any]

	RecordReferenceNames map[string]*Resource[map[string]string]

	RecordChildCounts map[string]*Resource[map[string]int]
	RecordDetailCur   string

	EditSessions map[string]*recordEditSession

	RecordFieldCursor map[string]string

	RecordFindBuffer map[string]string

	RecordFindActive map[string]bool

	RecordsPickerSearch searchState // search within the sobject picker
	RecordsSObjectCur   string      // "" = in picker mode; else name of drilled-in sObject

	ListViewsPerSObject map[string]*Resource[[]sf.ListView]

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

	Networks *Resource[[]sf.Network]

	// CommunityUserByContact memoises the per-session lookup of "does
	// this Contact have an active community User?" keyed by ContactId.
	// Empty value = checked, no active community user; missing key =
	// never checked. Backs the gating of "Log in to community as user"
	// targets so we don't enqueue per-network targets for contacts
	// without a portal user.
	CommunityUserByContact map[string]string

	ListViewCur map[string]string

	ChipMode map[string]ChipMode
}

type orgDataMetadata struct {
	Describes      map[string]*Resource[sf.SObjectDescribe]
	DescribeFields map[string]*describeFieldState

	CustomObjectBaselines map[string]*Resource[*sf.CustomObjectBaseline]

	FlowVersions map[string]*Resource[[]sf.FlowVersion]

	FlowVersionDetail map[string]*Resource[map[string]any]

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

	FieldDescriptions map[string]string

	// CustomObjectIDs caches Tooling-API CustomObject.Id lookups
	// per sobject API name. Same rationale + lifespan as CustomFieldIDs.
	// Guarded by customIDMu — access via customObjectIDCached or with
	// the lock held.
	CustomObjectIDs map[string]string
}

type orgDataPerms struct {
	// PermissionSets is the shared picker scope for the FLS grid
	// (every profile + permset in the org). Cached org-wide
	// because every object's FLS grid uses the same list.
	PermissionSets Resource[[]sf.FLSPickerEntry]
	FLS            map[string]*Resource[[]sf.FieldPermissionRow]
	FLSParentID    string

	PermSets Resource[[]sf.PermissionSet]
	PSGs     Resource[[]sf.PermissionSetGroup]
	Profiles Resource[[]sf.Profile]

	PermSetList ListView[sf.PermissionSet]
	PSGList     ListView[sf.PermissionSetGroup]
	ProfileList ListView[sf.Profile]

	PermSetsTableState uilayout.ListTableState
	PSGsTableState     uilayout.ListTableState
	ProfilesTableState uilayout.ListTableState

	PermParentKind      string
	PermParentID        string
	PermParentPermSetID string

	PermParentSubtab int

	ObjectPerms map[string]*Resource[[]sf.ObjectPermission]

	// SystemPerms holds per-parentID system-permission lists.
	// Key: the PermissionSet Id (PermParentPermSetID).
	// Lazily allocated on first System subtab view.
	SystemPerms map[string]*Resource[[]sf.SystemPermission]

	GroupMembers     map[string]*Resource[[]sf.GroupMemberRow]
	GroupMemberList  map[string]ListView[sf.GroupMemberRow]
	GroupMemberState map[string]*uilayout.ListTableState
	GroupMemberKind  string
	GroupMemberID    string

	AssignedUsers map[string]*Resource[[]sf.PermissionSetAssignment]

	PermFieldsSObject string

	ObjPermSearch map[string]*searchState
	SysPermSearch map[string]*searchState

	PermsDashboardSubtab int
}

type orgDataUsers struct {
	ChipUsers           map[string]*Resource[sf.UsersList]
	ChipUsersList       map[string]*ListView[sf.UserRow]
	ChipUsersTableState map[string]*uilayout.ListTableState

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
	userScore         func(sf.UserRow, string) int

	userMatch func(sf.UserRow, string) bool

	UserCur string
	// UserActionCur is the highlighted row in the User detail action
	// menu. Bounded by the renderer.
	UserActionCur  int
	UserDetailRows map[string]sf.UserRow
	// UserLoginRows caches per-user UserLogin rows (Freeze state +
	// row Id needed to PATCH). Empty UserLoginRow means we tried but
	// the row doesn't exist (user has never logged in); a missing
	// map entry means we haven't fetched yet.
	UserLoginRows map[string]sf.UserLoginRow
	UserLoginHist map[string][]sf.LoginHistoryRow
	UserAccessMap map[string]sf.UserAccess

	UsersSubtab int // /users subtab index
}

type orgDataHome struct {
	Home    Resource[HomeData]
	OrgInfo Resource[sf.OrgInfo] // Organization sObject — singleton metadata, drives the home tab's identity card

	Notifications Resource[sf.NotificationsList]

	HomeNotifList   ListView[sf.Notification]
	HomeLimitList   ListView[KeyLimit]
	HomeUserList    ListView[sf.UserRow]
	HomeLicenseList ListView[homeLicenseRow]

	HomeNotifTableState   uilayout.ListTableState
	HomeLimitTableState   uilayout.ListTableState
	HomeUserTableState    uilayout.ListTableState
	HomeLicenseTableState uilayout.ListTableState

	HomeSubtab int

	HomeRecentMode ChipMode
}

type orgDataRecent struct {
	RecentlyViewed     Resource[[]sf.RecentlyViewedRow]
	RecentlyViewedList ListView[sf.RecentlyViewedRow]

	Recent       []RecentEntry
	RecentList   ListView[RecentEntry]
	RecentLoaded bool // lazy-load guard so settings hits at most once per org per session

	RecentSFList ListView[RecentEntry]
	// recentSFGen tracks the d.recentGen value at the time
	// RecentSFList was last refreshed.  When recentGen advances
	// past this we know the list needs a rebuild.  Lets us avoid
	// the per-render rebuild that the old merged stream paid for.
	recentSFGen uint64

	recentGen uint64

	RecentChipIdx int
}

type orgDataCode struct {
	ApexClasses      Resource[[]sf.ApexClassRow]
	ApexTriggersFlat Resource[[]sf.TriggerRow] // flat cross-sObject list for /apex's Triggers chip
	LWCBundles       Resource[[]sf.LWCBundle]
	AuraBundles      Resource[[]sf.AuraBundle]

	ApexClassList   ListView[sf.ApexClassRow]
	ApexTriggerList ListView[sf.TriggerRow] // populated when /apex's Triggers chip is on
	LWCBundleList   ListView[sf.LWCBundle]
	AuraBundleList  ListView[sf.AuraBundle]

	ApexClassesTableState  uilayout.ListTableState
	ApexTriggersTableState uilayout.ListTableState
	LWCBundlesTableState   uilayout.ListTableState
	AuraBundlesTableState  uilayout.ListTableState

	ApexClassDetail map[string]*Resource[sf.ApexClassDetail]
	LWCDetail       map[string]*Resource[sf.LWCBundleDetail]
	AuraDetail      map[string]*Resource[sf.AuraBundleDetail]
	ApexCur         string
	LWCCur          string // the active drill ID — applies to whichever kind is selected
	ComponentsKind  string
	ApexKind        string

	BodyCursor  map[string]int
	BodyScroll  map[string]int
	BodyHScroll map[string]int

	// CodeFind is the per-body in-code find state (/ on a code
	// surface): query buffer, input focus, current match index, plus
	// the match memo so the per-frame render never re-scans the body.
	CodeFind map[string]*codeFindState

	CodeViewLast codeViewLastPaint

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

type orgDataReports struct {
	Reports     Resource[[]sf.ReportSummary]
	Dashboards  Resource[[]sf.DashboardRow]
	ReportTypes Resource[[]sf.ReportTypeRow]
	ReportList  ListView[sf.ReportSummary]

	ReportRuns map[string]*Resource[sf.ReportRun]
	ReportCur  string

	ReportFolders       *treechip.Registry
	ReportFoldersSrc    *sources.ReportFolderSource
	ReportFoldersLoaded bool // lazy-init guard

	ReportsSubtab int
}

type orgDataMeta struct {
	MetaTypes        Resource[[]sf.MetadataTypeInfo]
	MetaTypesList    ListView[sf.MetadataTypeInfo]
	MetaTypeCur      string // drilled type (TabMetaTypeDetail)
	MetaTypeItems    map[string]*Resource[[]sf.MetadataItem]
	MetaTypeItemList ListView[sf.MetadataItem]

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

type orgDataDevProjects struct {
	LoadedDevProjectID string
	LoadedScope        *orgproject.Scope
	ReportsProjectMode bool

	BundleCursor int

	// DevProjectsSubtab + DevProjectDetailSubtab — index into
	// devProjectsSubtabs() / devProjectDetailSubtabs() respectively.
	// Per-org so two orgs can be on different tabs concurrently.
	DevProjectsSubtab      int
	DevProjectDetailSubtab int

	AllBundlesCursor int
}

type orgDataSOQLLibrary struct {
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

type orgDataExecLibrary struct {
	ExecSavedList     ListView[devproject.SavedApex]
	ExecSavedLoaded   bool
	ExecSavedTable    uilayout.ListTableState
	ExecHistoryList   ListView[devproject.ApexHistoryEntry]
	ExecHistoryLoaded bool
	ExecHistoryTable  uilayout.ListTableState

	ExecSavedChipIdx   int
	ExecHistoryChipIdx int
}

type orgDataNav struct {
	SObjectFilter         SObjectFilter
	DescribeCur           string // API name of the currently-open describe
	FieldCur              string // API name of the currently-open field (on the field-detail page)
	FlowCur               string // DefinitionId of the currently-open flow
	FlowVersionCur        string // Tooling Id of the flow version drilled into (version viewer)
	flowVersionsLoadedFor string

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

	LastTabInStem map[Tab]Tab

	DrillReturnTab map[Tab]Tab

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
