package qchip

import "github.com/Jacob-Stokes/sf-deck/internal/query"

// Built-in chip catalogues. Each surface declares its built-ins as
// query.Query literals. Adding a new built-in is one entry; adding a
// new operator or row field reaches the catalogue automatically (via
// the AST + Eval).
//
// Names + IDs are stable on disk — callers reference chips by ID
// (settings.toml keeps a pointer to the active chip per surface) so
// renaming a built-in id is a breaking change for users' configs.

// SObjectBuiltins ships /objects' built-in chips. Replaces the
// hardcoded match closures in internal/ui/filter/sobject.go.
//
// Strip layout (favourites): Browseable → Custom → Standard →
// Unmanaged → Managed. Browseable is the locked default — it
// mirrors what Lightning's Setup → Object Manager shows, which is
// what "I want to see the sObjects" actually means for ~95% of
// browsing intents. The full firehose lives behind "All (incl.
// system)" in the overflow modal.
//
// Why filter so aggressively in the default? A typical org has
// ~800 sObjects when you include Share/History/Feed/ChangeEvent
// companion tables, system metadata, platform events, custom
// metadata types, etc. The user's mental model of "an object" is
// the Object Manager's ~150 — everything else is plumbing.
var SObjectBuiltins = []Chip{
	{
		// Browseable mirrors Object Manager's filter: queryable,
		// customizable, not a Share/History/Feed/ChangeEvent
		// companion. Includes both custom (__c) and standard
		// objects users actually browse — Account, Contact, etc.
		// plus every MyCustom__c. Excludes the platform-events /
		// custom-metadata / big-objects tail because those have
		// their own dedicated overflow chips and don't belong in
		// "I want to see my objects."
		ID: "browseable", Label: "Browseable", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{
			Where: query.And(
				query.Cmp("IsCustomizable", query.OpEq, true),
				query.Not(query.Cmp("Name", query.OpEndsWith, "ChangeEvent")),
				query.Not(query.Cmp("Name", query.OpEndsWith, "Feed")),
				query.Not(query.Cmp("Name", query.OpEndsWith, "History")),
				query.Not(query.Cmp("Name", query.OpEndsWith, "Share")),
				query.Not(query.Cmp("Name", query.OpEndsWith, "__mdt")),
				query.Not(query.Cmp("Name", query.OpEndsWith, "__e")),
				query.Not(query.Cmp("Name", query.OpEndsWith, "__b")),
			),
		},
	},
	{
		ID: "custom", Label: "Custom", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query: query.Query{
			Where: query.Cmp("IsCustom", query.OpEq, true),
		},
	},
	{
		// Standard = SF-shipped object the user can customize.
		// Excludes companion tables for the same reasons Browseable
		// does (otherwise the strip's "Standard" chip surfaces
		// AccountShare, AccountHistory, etc. alongside Account).
		ID: "standard", Label: "Standard", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query: query.Query{
			Where: query.And(
				query.Cmp("IsCustom", query.OpEq, false),
				query.Cmp("IsCustomizable", query.OpEq, true),
				query.Not(query.Cmp("Name", query.OpEndsWith, "ChangeEvent")),
				query.Not(query.Cmp("Name", query.OpEndsWith, "Feed")),
				query.Not(query.Cmp("Name", query.OpEndsWith, "History")),
				query.Not(query.Cmp("Name", query.OpEndsWith, "Share")),
				query.Not(query.Cmp("Name", query.OpEndsWith, "__mdt")),
				query.Not(query.Cmp("Name", query.OpEndsWith, "__e")),
			),
		},
	},
	{
		ID: "unmanaged", Label: "Unmanaged", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		// Name-based check: synthetic companions (ChangeEvent / History /
		// Share / OwnerSharingRule) of managed parents have a null
		// NamespacePrefix on EntityDefinition but still carry the parent's
		// namespace in the API name. NamespacePrefix-only would leak them
		// into Unmanaged.
		Query: query.Query{Where: query.Cmp("IsManaged", query.OpEq, false)},
	},
	{
		ID: "managed", Label: "Managed", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("IsManaged", query.OpEq, true)},
	},
	// --- Overflow (Favourite: false) -----------------------------
	{
		// All = no filter. Renamed from "All" so the strip default
		// (Browseable) doesn't seem misleading by comparison — the
		// system suffix makes it obvious this is the firehose.
		ID: "all", Label: "All (incl. system)", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{},
	},
	{
		// Manageable = old name for what Browseable covers + more.
		// Kept in overflow for users who want IsCustomizable=true
		// without the companion-table filtering.
		ID: "manageable", Label: "Manageable", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{
			Where: query.Cmp("IsCustomizable", query.OpEq, true),
		},
	},
	{
		// System = SF-internal, can't be customized. Hidden in
		// Object Manager. Useful for admins debugging audit
		// trail / setup-history lookups.
		ID: "system", Label: "System", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{
			Where: query.And(
				query.Cmp("IsCustom", query.OpEq, false),
				query.Cmp("IsCustomizable", query.OpEq, false),
			),
		},
	},
	{
		ID: "platform-events", Label: "Platform Events", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("Name", query.OpEndsWith, "__e")},
	},
	{
		ID: "change-events", Label: "Change Events", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("Name", query.OpEndsWith, "ChangeEvent")},
	},
	{
		ID: "custom-metadata", Label: "Custom Metadata", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("Name", query.OpEndsWith, "__mdt")},
	},
	{
		ID: "big-objects", Label: "Big Objects", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("Name", query.OpEndsWith, "__b")},
	},
}

// ApexBuiltins ships /apex's built-in chips.
//
// Test detection is name-based — the body would be the source of
// truth (look for @isTest) but the list query doesn't pull bodies, so
// we approximate via the conventional Test suffix / prefix used in
// pretty much every Salesforce codebase. Users can disable
// non-conforming classes individually via /apex's overflow chip
// modal once the registry exposes them.
var ApexBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{},
	},
	{
		ID: "unmanaged", Label: "Unmanaged", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("NamespacePrefix", query.OpIsNull, nil)},
	},
	{
		ID: "managed", Label: "Managed", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Not(query.Cmp("NamespacePrefix", query.OpIsNull, nil))},
	},
	{
		ID: "tests", Label: "Tests", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query: query.Query{Where: query.Or(
			query.Cmp("Name", query.OpEndsWith, "_Test"),
			query.Cmp("Name", query.OpEndsWith, "Tests"),
			query.Cmp("Name", query.OpEndsWith, "_TEST"),
			query.Cmp("Name", query.OpEndsWith, "Test"),
			query.Cmp("Name", query.OpStartsWith, "Test_"),
			query.Cmp("Name", query.OpStartsWith, "Test"),
		)},
	},
	{
		ID: "non-tests", Label: "Non-tests", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query: query.Query{Where: query.And(
			query.Not(query.Cmp("Name", query.OpEndsWith, "_Test")),
			query.Not(query.Cmp("Name", query.OpEndsWith, "Tests")),
			query.Not(query.Cmp("Name", query.OpEndsWith, "_TEST")),
			query.Not(query.Cmp("Name", query.OpEndsWith, "Test")),
			query.Not(query.Cmp("Name", query.OpStartsWith, "Test_")),
			query.Not(query.Cmp("Name", query.OpStartsWith, "Test")),
		)},
	},
	// --- Overflow (Favourite: false) -----------------------------
	// Active is the implicit default — nearly every class is
	// Active, so the chip adds no signal vs. All. Deleted is a
	// rarely-needed historical view. Invalid is for debugging
	// uncompiled / broken classes after a deploy.
	{
		ID: "active", Label: "Active", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("Status", query.OpEq, "Active")},
	},
	{
		ID: "invalid", Label: "Invalid", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("IsValid", query.OpEq, false)},
	},
	{
		ID: "deleted", Label: "Deleted", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("Status", query.OpEq, "Deleted")},
	},
}

// TriggerBuiltins ships /apex Triggers' built-in chips. Mirrors
// ApexBuiltins where it makes sense (managed/unmanaged + Active
// status); test-detection chips are dropped because the convention
// for tests doesn't apply to triggers.
var TriggerBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{},
	},
	{
		ID: "unmanaged", Label: "Unmanaged", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("NamespacePrefix", query.OpIsNull, nil)},
	},
	{
		ID: "managed", Label: "Managed", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Not(query.Cmp("NamespacePrefix", query.OpIsNull, nil))},
	},
	{
		ID: "active", Label: "Active", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("Status", query.OpEq, "Active")},
	},
	{
		ID: "inactive", Label: "Inactive", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("Status", query.OpEq, "Inactive")},
	},
	{
		ID: "invalid", Label: "Invalid", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("IsValid", query.OpEq, false)},
	},
}

// AuraBuiltins ships /components Aura subtab's built-in chips. Aura
// has no equivalent of LWC's IsExposed flag, so the cuts are
// managed/unmanaged only — same shape as the LWC defaults minus the
// exposure split. Custom chips can fill the gap for users who care
// about API version or namespace specifics.
var AuraBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{},
	},
	{
		ID: "unmanaged", Label: "Unmanaged", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("NamespacePrefix", query.OpIsNull, nil)},
	},
	{
		ID: "managed", Label: "Managed", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Not(query.Cmp("NamespacePrefix", query.OpIsNull, nil))},
	},
}

// LWCBuiltins ships /lwc's built-in chips. IsExposed is the headline
// admin-relevant flag; everything else (api version, etc.) we leave
// to the search bar.
var LWCBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{},
	},
	{
		ID: "unmanaged", Label: "Unmanaged", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("NamespacePrefix", query.OpIsNull, nil)},
	},
	{
		ID: "managed", Label: "Managed", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Not(query.Cmp("NamespacePrefix", query.OpIsNull, nil))},
	},
	{
		ID: "exposed", Label: "Exposed", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("IsExposed", query.OpEq, true)},
	},
	{
		ID: "internal", Label: "Internal", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("IsExposed", query.OpEq, false)},
	},
}

// PermSetBuiltins ships /perms PermSets subtab chips. PermissionSet
// has an IsCustom flag (false for SF-shipped permsets) and a
// IsOwnedByProfile flag (true for the implicit permset behind a
// profile — those are usually noise on this surface, but admins
// occasionally want to see them).
var PermSetBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{},
	},
	{
		ID: "custom", Label: "Custom", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("IsCustom", query.OpEq, true)},
	},
	{
		ID: "standard", Label: "Standard", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("IsCustom", query.OpEq, false)},
	},
	{
		ID: "session", Label: "Session", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("Type", query.OpEq, "Session")},
	},
}

// PSGBuiltins ships /perms PSGs subtab chips. PermissionSetGroup
// has a Status field reflecting whether it's been recalculated.
var PSGBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{},
	},
	{
		ID: "updated", Label: "Updated", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("Status", query.OpEq, "Updated")},
	},
	{
		ID: "outdated", Label: "Outdated", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("Status", query.OpEq, "Outdated")},
	},
	{
		ID: "failed", Label: "Failed", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("Status", query.OpEq, "Failed")},
	},
}

// ProfileBuiltins ships /perms Profiles subtab chips. UserType is
// the headline split — Standard internal users vs guest / partner /
// portal users.
var ProfileBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{},
	},
	{
		ID: "standard", Label: "Internal", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("UserType", query.OpEq, "Standard")},
	},
	{
		ID: "guest", Label: "Guest", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("UserType", query.OpEq, "Guest")},
	},
	{
		ID: "partner", Label: "Partner", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("UserType", query.OpEq, "PowerPartner")},
	},
}

// QueueBuiltins ships /perms Queues subtab chips. Queues are
// usually small in count; the chips help with the "find by sObject"
// case via custom-chip predicates rather than baked-in defaults.
var QueueBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{},
	},
}

// PublicGroupBuiltins — chips on /perms Public Groups. The
// DoesIncludeBosses flag is the headline split; everything else is
// down to user-defined chips via M.
var PublicGroupBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{},
	},
	{
		ID: "with-bosses", Label: "Includes Bosses", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("DoesIncludeBosses", query.OpEq, true)},
	},
}

// FlowBuiltins ships /flows' built-in chips.
//
// Strip favourites:
//
//	All, Active, Inactive, Modified by me, Created by me.
//
// Overflow (Favourite: false):
//
//	Draft, Screen, Auto, Unmanaged, Managed, Process Builder, Workflow.
//
// The synthetic Project + Recently viewed chips prepend to this
// strip at render time (see stripRows), so the user sees
// "📁 Project · Recently viewed · All · Active · …" when a project
// is loaded.
//
// Process Builder + Workflow are obsolete tech (Salesforce has
// deprecated both in favour of Flow) — typically zero or a handful
// of grandfathered entries in modern orgs. Keeping them on the
// strip wastes space; users debugging legacy automation can reach
// them via the overflow modal.
//
// Screen / Auto / Draft / Unmanaged / Managed were favourites in
// the original cut but were demoted to overflow once the
// personal-lens chips (Modified by me, Created by me) joined —
// they're cheap to reach via the overflow modal when needed and
// the daily-driver workflow is much more often "what have I
// touched lately" than "what flavour of flow is this."
//
// Inactive replaces the older "Obsolete" chip: same OR-predicate
// (Status = Obsolete OR Status = Inactive) under a clearer label —
// "Inactive" reads as "anything that isn't running," which is what
// the chip actually filters to.
var FlowBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{},
	},
	{
		ID: "active", Label: "Active", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("Status", query.OpEq, "Active")},
	},
	{
		// Inactive: anything that isn't running, regardless of how
		// it got there. Status = "Obsolete" is the explicit Salesforce
		// state for deprecated flows; "Inactive" is the synthetic
		// state sf-deck stamps when there's no active version at all
		// (see ListFlows). One chip covers both — users typically
		// don't care which flavour of inactive a flow is, just that
		// it isn't running.
		ID: "inactive", Label: "Inactive", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query: query.Query{Where: query.Or(
			query.Cmp("Status", query.OpEq, "Obsolete"),
			query.Cmp("Status", query.OpEq, "Inactive"),
		)},
	},
	{
		// Modified by me: flows whose last editor matches the current
		// user. $userName is substituted at apply time via
		// chipSubs(d).UserName threading through chipMatcherFor so
		// settings.toml stays org-agnostic.
		//
		// Two distinct chips (vs a single "Mine" OR-union) because
		// they answer different questions: "what have I touched
		// recently" vs "what did I originally author." Both are
		// genuinely common asks; the user picks the lens that
		// matches their intent.
		ID: "modified-by-me", Label: "Modified by me", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("LastModifiedBy.Name", query.OpEq, "$userName")},
	},
	{
		ID: "created-by-me", Label: "Created by me", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("CreatedBy.Name", query.OpEq, "$userName")},
	},
	// --- Overflow (Favourite: false) -----------------------------
	{
		ID: "draft", Label: "Draft", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("Status", query.OpEq, "Draft")},
	},
	{
		ID: "screen", Label: "Screen", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("ProcessType", query.OpEq, "Flow")},
	},
	{
		ID: "auto", Label: "Auto", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("ProcessType", query.OpEq, "AutoLaunchedFlow")},
	},
	{
		ID: "unmanaged", Label: "Unmanaged", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("NamespacePrefix", query.OpIsNull, nil)},
	},
	{
		ID: "managed", Label: "Managed", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Not(query.Cmp("NamespacePrefix", query.OpIsNull, nil))},
	},
	{
		ID: "process-builder", Label: "Process Builder", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("ProcessType", query.OpEq, "InvocableProcess")},
	},
	{
		ID: "workflow", Label: "Workflow", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("ProcessType", query.OpEq, "Workflow")},
	},
}

// RecordBuiltins ships /records' built-in chips. Same structure as
// the legacy lens.Builtins. Scope is per-sobject — these are templates
// that the registry expands when applied (the existing lens layer
// templated SOQL `:userId` etc.; we keep that semantics).
//
// :userId expansion happens at engine apply time, not in the AST.
// The AST stores the literal token "$userId" and the engine layer
// substitutes the live id before evaluation/SOQL emission.
var RecordBuiltins = []Chip{
	{
		// Label is "Changed" — the chip surfaces records ordered by
		// LastModifiedDate, i.e. "what's been changed in the org
		// recently." Distinct from /recent which is "where the user
		// has been." ID stays "recent" for back-compat with user
		// settings.toml configs that pin chip preferences by ID.
		//
		// Limit unset → inherits settings.DefaultChipLimit (10000
		// default). Per-chip override available via the chip wizard.
		ID: "recent", Label: "Changed", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{
			OrderBy: []query.OrderBy{{Field: "LastModifiedDate", Direction: query.Descending}},
		},
	},
	{
		// The created-axis counterpart to "Changed": records ordered by
		// CreatedDate descending, i.e. "what's newest in this object."
		// One query on the current object — inherits the Records
		// surface's sort / search / scroll / columns for free.
		ID: "recently-created", Label: "Recently created", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query: query.Query{
			OrderBy: []query.OrderBy{{Field: "CreatedDate", Direction: query.Descending}},
		},
	},
	{
		// Filters + orders by LastModifiedDate, not CreatedDate.
		// Users intuitively read "Today" on a record list as "what
		// happened today" — and "happened" almost always means
		// edits, not just new creations. The default "Changed"
		// chip already orders by LastModifiedDate; keeping the
		// date axis consistent across the strip is the right UX.
		ID: "today", Label: "Today", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{
			Where:   query.Cmp("LastModifiedDate", query.OpDateLiteral, "TODAY"),
			OrderBy: []query.OrderBy{{Field: "LastModifiedDate", Direction: query.Descending}},
		},
	},
	{
		ID: "this-week", Label: "This week", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{
			Where:   query.Cmp("LastModifiedDate", query.OpDateLiteral, "THIS_WEEK"),
			OrderBy: []query.OrderBy{{Field: "LastModifiedDate", Direction: query.Descending}},
		},
	},
	{
		ID: "mine", Label: "Mine", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{
			Where:   query.Cmp("OwnerId", query.OpEq, "$userId"),
			OrderBy: []query.OrderBy{{Field: "LastModifiedDate", Direction: query.Descending}},
		},
	},
	{
		ID: "mine-recent", Label: "Mine, recent", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{
			Where: query.And(
				query.Cmp("OwnerId", query.OpEq, "$userId"),
				query.Cmp("LastModifiedDate", query.OpDateLiteral, "LAST_N_DAYS:7"),
			),
			OrderBy: []query.OrderBy{{Field: "LastModifiedDate", Direction: query.Descending}},
		},
	},
}

// SOQLSavedBuiltins ships /soql Saved subtab built-in chips. Only
// "All" today; HasDescription gives a "documented vs not" split
// users frequently want when curating a library. Other splits
// (pinned-to-project, has-tags) require store lookups so they live
// outside the chip engine — wired as bespoke chips in the surface
// (see tab_soql_library.go).
var SOQLSavedBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{},
	},
	{
		ID: "documented", Label: "Documented", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("HasDescription", query.OpEq, true)},
	},
	{
		ID: "undocumented", Label: "Undocumented", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("HasDescription", query.OpEq, false)},
	},
}

// SOQLHistoryBuiltins ships /soql History subtab built-in chips.
var SOQLHistoryBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{},
	},
	{
		ID: "ok", Label: "OK", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("HasError", query.OpEq, false)},
	},
	{
		ID: "errors", Label: "Errors", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("HasError", query.OpEq, true)},
	},
	{
		ID: "today", Label: "Today", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("ExecutedAt", query.OpDateLiteral, "TODAY")},
	},
	{
		ID: "this-week", Label: "This Week", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("ExecutedAt", query.OpDateLiteral, "THIS_WEEK")},
	},
	// --- Overflow (Favourite: false) -----------------------------
	// Slow is a debugging chip — useful when you're hunting a
	// specific bad query, not part of normal browsing.
	{
		ID: "slow", Label: "Slow (>5s)", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("DurationMs", query.OpGT, 5000)},
	},
}

// RecentBuiltins ships /home → Recent + /recent's built-in chips.
//
// Layout: All as the locked default, then one chip per common kind
// (records, reports, flows, apex, components), then a "Misc" bucket
// catching the long tail of less-frequent kinds (users, deploys,
// packages, queues, etc.) so the strip stays compact at narrow
// widths.
//
// Filter contract: chips match against RecentEntry.Kind via the
// query.Row interface (see RecentEntry.Field). Kind is the stable
// identifier — the human KIND column label (renderer-side) is a
// pretty-print on top.
//
// The synthetic "loaded project" chip is prepended at render time
// when an org has a project loaded — handled by stripRows + the
// recentChipSurface's ApplyProjectChip closure, NOT here.
var RecentBuiltins = []Chip{
	{
		// "All" shows every row from the active source (sf-deck local
		// log or Salesforce RecentlyViewed — picked by L key on
		// /home Recent) EXCEPT list views.  Salesforce auto-tracks
		// list-view visits via the same RecentlyViewed mechanism it
		// uses for records, so without this filter the default view
		// floods with "All Contacts", "My Cases", etc. every time
		// you bounced through an object tab in Lightning.  The
		// "List Views" chip below opts them back in for the rare
		// "what list views was I just using" workflow.
		//
		// sf-deck mode never produces KindListView entries (the local
		// log only records direct drills) so this filter is a no-op
		// in that mode — listview is purely an SF-side concept.
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{Where: query.Not(query.Cmp("Kind", query.OpEq, "listview"))},
	},
	{
		ID: "records", Label: "Records", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("Kind", query.OpEq, "record")},
	},
	{
		ID: "reports", Label: "Reports", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query: query.Query{
			Where: query.Cmp("Kind", query.OpIn, []any{"report", "dashboard"}),
		},
	},
	{
		// ListViews appear in Salesforce's RecentlyViewed when users
		// open a list view in Lightning.  The default "All" chip
		// hides them (see above); this chip is the opt-in.  Pinned
		// to the strip so the round-trip is one keystroke for the
		// rare "what list views was I just using" workflow.
		ID: "listviews", Label: "List Views", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("Kind", query.OpEq, "listview")},
	},
	{
		ID: "flows", Label: "Flows", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("Kind", query.OpEq, "flow")},
	},
	{
		ID: "apex", Label: "Apex", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("Kind", query.OpEq, "apex_class")},
	},
	{
		ID: "components", Label: "Components", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query: query.Query{
			Where: query.Cmp("Kind", query.OpIn, []any{"lwc", "aura"}),
		},
	},
	{
		ID: "schema", Label: "Schema", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{
			Where: query.Cmp("Kind", query.OpIn, []any{"sobject", "field"}),
		},
	},
	{
		ID: "perms", Label: "Perms", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{
			Where: query.Cmp("Kind", query.OpIn, []any{"permset", "permset_group", "profile"}),
		},
	},
	{
		// Misc catches the long tail: users, deploys, packages, queues,
		// public groups, apex logs. NOT in any of the named chips
		// above. Available via the overflow modal — these kinds rarely
		// need a dedicated lane on the strip.
		ID: "misc", Label: "Misc", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{
			Where: query.Cmp("Kind", query.OpIn, []any{
				"user", "deploy", "package", "queue",
				"public_group", "apex_log",
			}),
		},
	},
}

// UserBuiltins ships /users · All Users built-in chips. Each chip
// compiles to SOQL via qchip.ApplyToSOQL — the predicate runs
// server-side so chips like "System admins" return every admin in
// the org, not just whatever sat in the first N-row alphabetical
// slice. Limit caps each chip's result; matches the records-shaped
// surfaces' "every chip is its own bounded fetch" pattern.
var UserBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{
			OrderBy: []query.OrderBy{{Field: "Name", Direction: query.Ascending}},
		},
	},
	{
		ID: "active", Label: "Active", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query: query.Query{
			Where:   query.Cmp("IsActive", query.OpEq, true),
			OrderBy: []query.OrderBy{{Field: "Name", Direction: query.Ascending}},
		},
	},
	{
		ID: "inactive", Label: "Inactive", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query: query.Query{
			Where:   query.Cmp("IsActive", query.OpEq, false),
			OrderBy: []query.OrderBy{{Field: "Name", Direction: query.Ascending}},
		},
	},
	{
		ID: "admins", Label: "System admins", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query: query.Query{
			Where:   query.Cmp("Profile.Name", query.OpEq, "System Administrator"),
			OrderBy: []query.OrderBy{{Field: "Name", Direction: query.Ascending}},
		},
	},
	{
		ID: "standard", Label: "Standard users", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{
			Where:   query.Cmp("Profile.Name", query.OpEq, "Standard User"),
			OrderBy: []query.OrderBy{{Field: "Name", Direction: query.Ascending}},
		},
	},
	{
		ID: "logged-30d", Label: "Logged in 30d", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query: query.Query{
			Where:   query.Cmp("LastLoginDate", query.OpDateLiteral, "LAST_N_DAYS:30"),
			OrderBy: []query.OrderBy{{Field: "LastLoginDate", Direction: query.Descending}},
		},
	},
	{
		ID: "never-logged", Label: "Never logged in", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{
			Where:   query.Cmp("LastLoginDate", query.OpIsNull, nil),
			OrderBy: []query.OrderBy{{Field: "Name", Direction: query.Ascending}},
		},
	},
}

// FieldBuiltins ships the /objects → Schema field-list chips. Slices
// the field list by KIND (picklist / formula / reference / lookup /
// master-detail) and by FLAG (required / unique / external-id /
// encrypted / auto-number / name-field / dependent picklist) — the
// flag chips map 1:1 to the FLAGS column letters. Predicates resolve
// against sf.Field.Field (the query.Row accessor).
//
// Favourites (strip): All · Custom · Required · Picklist · Formula ·
// Reference. Everything else lives in the overflow modal; the user
// promotes what they reach for via F (persists in settings).
var FieldBuiltins = []Chip{
	{
		// All is the locked default — no predicate, matches every field.
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
	},
	{
		ID: "custom", Label: "Custom", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("IsCustom", query.OpEq, true)},
	},
	{
		ID: "required", Label: "Required", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("IsRequired", query.OpEq, true)},
	},
	{
		ID: "picklist", Label: "Picklist", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("IsPicklist", query.OpEq, true)},
	},
	{
		ID: "formula", Label: "Formula", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("IsFormula", query.OpEq, true)},
	},
	{
		ID: "reference", Label: "Reference", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("IsReference", query.OpEq, true)},
	},
	// --- overflow (non-favourite) ---
	{
		ID: "standard", Label: "Standard", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("IsCustom", query.OpEq, false)},
	},
	{
		ID: "lookup", Label: "Lookup", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("IsLookup", query.OpEq, true)},
	},
	{
		ID: "master-detail", Label: "Master-Detail", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("IsMasterDetail", query.OpEq, true)},
	},
	{
		ID: "unique", Label: "Unique", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("IsUnique", query.OpEq, true)},
	},
	{
		ID: "external-id", Label: "External ID", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("IsExternalId", query.OpEq, true)},
	},
	{
		ID: "encrypted", Label: "Encrypted", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("IsEncrypted", query.OpEq, true)},
	},
	{
		ID: "auto-number", Label: "Auto-number", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("IsAutoNumber", query.OpEq, true)},
	},
	{
		ID: "name-field", Label: "Name field", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("IsNameField", query.OpEq, true)},
	},
	{
		ID: "dependent-picklist", Label: "Dependent picklist", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("IsDependentPicklist", query.OpEq, true)},
	},
}

// DeployBuiltins — /deploys chip strip. OpGt on the error counters
// gives "anything broken" without needing a status enumeration.
var DeployBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{},
	},
	{
		ID: "failed", Label: "Failed", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("Status", query.OpEq, "Failed")},
	},
	{
		ID: "succeeded", Label: "Succeeded", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("Status", query.OpEq, "Succeeded")},
	},
	{
		ID: "validations", Label: "Validations", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("CheckOnly", query.OpEq, true)},
	},
	{
		ID: "real", Label: "Real deploys", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("CheckOnly", query.OpEq, false)},
	},
	{
		ID: "testfails", Label: "Test failures", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("TestErrors", query.OpGT, 0)},
	},
}

// ActiveUsersBuiltins are the lenses over the /users → Active subtab
// (one row per user with a live session). Filters run client-side
// against ActiveUserRow.Field. "No MFA" and "API" are the security /
// integration angles; "Recently active" narrows the (up-to-2h) session
// window to who's actually here now.
var ActiveUsersBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{},
	},
	{
		ID: "nomfa", Label: "No MFA", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		// AnyLowMFA is true when any of the user's sessions is below
		// HIGH_ASSURANCE — i.e. didn't clear step-up/MFA this session.
		Query: query.Query{Where: query.Cmp("AnyLowMFA", query.OpEq, true)},
	},
	{
		ID: "recent", Label: "Recently active", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		// RecentMinutes is stamped at fetch time (minutes since last
		// activity); ≤15 ≈ "here right now".
		Query: query.Query{Where: query.Cmp("RecentMinutes", query.OpLTE, 15)},
	},
	{
		ID: "api", Label: "API / integration", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("IsAPI", query.OpEq, true)},
	},
}

// DashboardBuiltins — /reports Dashboards subtab.
var DashboardBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{},
	},
	{
		ID: "dynamic", Label: "Run as viewer", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("Type", query.OpEq, "LoggedInUser")},
	},
	{
		ID: "unmanaged", Label: "Unmanaged", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("NamespacePrefix", query.OpEq, "")},
	},
}

// ReportTypeBuiltins — /reports Report Types subtab.
var ReportTypeBuiltins = []Chip{
	{
		ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true, LockedFavourite: true,
		Query: query.Query{},
	},
	{
		ID: "custom", Label: "Custom", Scope: "*", Origin: OriginBuiltIn,
		Favourite: true,
		Query:     query.Query{Where: query.Cmp("Custom", query.OpEq, true)},
	},
	{
		ID: "standard", Label: "Standard", Scope: "*", Origin: OriginBuiltIn,
		Query: query.Query{Where: query.Cmp("Custom", query.OpEq, false)},
	},
}
