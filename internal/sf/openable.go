package sf

import (
	"fmt"
	"net/url"
	"strings"
)

// ======================================================================
// SSOT: every "open in browser" target, per domain type.
//
// If you want to know, or change, what `O` / `shift+O` / `Y` / `shift+Y`
// do for any kind of thing, this file is where it lives. Each type's
// Targets() returns the ordered list of Lightning / Setup URLs a user
// might want to jump to; index 0 is the default (what bare `O`/`Y` hit).
//
// The UI layer is generic — it never knows what a flow or an sObject
// is, it just asks `Targets()` and dispatches by index.
//
// Add a new type → implement Targets() next to its struct, make sure
// the type appears in the compile-time check list at the bottom of the
// file, and you're done. No UI changes needed.
// ======================================================================

// OpenTarget is one named destination: a human label, a Lightning path
// relative to the instance URL, and a short ID used for display / future
// keybinding hooks.
//
// Most targets live under the target org (Path) and get authenticated
// via `sf org open -p <path>`. Absolute URL is for targets that are
// not Salesforce pages at all — e.g. a browser extension's page that
// takes an external host+record as query params. Absolute URLs are
// opened via the OS default-browser handler (no org auth flow).
type OpenTarget struct {
	ID          string // short key: "manager", "list", "builder", "about", "view"
	Label       string // human label shown in the open-targets menu
	Path        string // instance-relative Lightning or Setup path
	AbsoluteURL string // set when this target is a non-Salesforce URL; Path is ignored
	// AllowBrowserExtension is set only for the configured Salesforce
	// Inspector target. It lets the UI accept a narrowly validated
	// chrome-extension:// or moz-extension:// inspect.html URL without
	// weakening validation for other absolute URLs.
	AllowBrowserExtension bool
	// Shortcut is an optional single-key accelerator shown in the open
	// menu and matched by handleOpenMenuKey. Lowercase ASCII letter
	// ("r", "e", "i", …). When multiple targets share the same
	// shortcut the first match wins.
	Shortcut string
	// YankValue, when non-empty, marks this as a VALUE target in the
	// yank menu: selecting it copies YankValue verbatim instead of
	// building/copying a URL. Set only by the yank-menu builder from a
	// YankTarget; URL targets leave it empty.
	YankValue string
}

// Openable is implemented by every domain type with one or more
// meaningful browser destinations. Implementations must return at least
// one target; Targets()[0] is treated as the default.
type Openable interface {
	Targets() []OpenTarget
}

// YankTarget is one copyable VALUE (not a URL) offered in the ctrl+y
// yank menu — e.g. a record's label, an sObject's API name, an Id, or a
// resource-specific snippet like a SOQL query. The URL targets from
// Targets() are folded into the same menu by the UI layer, so a Yankable
// only needs to declare the non-URL values that make sense for it.
type YankTarget struct {
	ID       string // short key: "label", "api", "id", "soql", …
	Label    string // human label shown in the yank menu ("API name")
	Value    string // the exact string copied to the clipboard
	Shortcut string // optional single lowercase-ASCII accelerator
}

// Yankable is implemented by domain types that have copyable values
// beyond their URL(s). Optional: a type that only implements Openable
// still gets URL yank targets in the ctrl+y menu.
type Yankable interface {
	YankTargets() []YankTarget
}

// nameLabelIDYankTargets is the common value-yank set for metadata
// components identified by an API/developer name plus a friendly label
// and a record Id — Apex classes, LWC/Aura bundles, permsets, groups,
// and the like. Only non-empty values produce a row, and the label row
// is dropped when it merely repeats the API name (no new information).
//
// Shortcuts: a = API name, l = label, i = Id. These never collide with
// URL targets because the menu drops a clashing URL-target shortcut in
// favour of the value target (see requestOpenMenu).
func nameLabelIDYankTargets(apiName, label, id string) []YankTarget {
	var ts []YankTarget
	if apiName != "" {
		ts = append(ts, YankTarget{ID: "api", Label: "API name", Value: apiName, Shortcut: "a"})
	}
	if label != "" && label != apiName {
		ts = append(ts, YankTarget{ID: "label", Label: "Label", Value: label, Shortcut: "l"})
	}
	if id != "" {
		ts = append(ts, YankTarget{ID: "id", Label: "Id", Value: id, Shortcut: "i"})
	}
	return ts
}

// FullURL joins an instance URL and a Lightning path into a shareable
// absolute URL.
func FullURL(instanceURL, path string) string {
	if instanceURL == "" {
		return path
	}
	return strings.TrimRight(instanceURL, "/") + path
}

// ----------------------------------------------------------------------
// sObject — Object Manager first (most common admin / dev intent),
// records list second, then the common sub-tabs.
// ----------------------------------------------------------------------

func (s SObject) Targets() []OpenTarget {
	return []OpenTarget{
		{ID: "manager", Label: "Object Manager — Details",
			Path: "/lightning/setup/ObjectManager/" + s.Name + "/Details/view"},
		{ID: "list", Label: "Records list",
			Path: "/lightning/o/" + s.Name + "/list"},
		{ID: "fields", Label: "Fields & Relationships",
			Path: "/lightning/setup/ObjectManager/" + s.Name + "/FieldsAndRelationships/view"},
		{ID: "layouts", Label: "Page Layouts",
			Path: "/lightning/setup/ObjectManager/" + s.Name + "/PageLayouts/view"},
		{ID: "rectypes", Label: "Record Types",
			Path: "/lightning/setup/ObjectManager/" + s.Name + "/RecordTypes/view"},
		{ID: "validation", Label: "Validation Rules",
			Path: "/lightning/setup/ObjectManager/" + s.Name + "/ValidationRules/view"},
		{ID: "triggers", Label: "Triggers",
			Path: "/lightning/setup/ObjectManager/" + s.Name + "/ApexTriggers/view"},
		{ID: "flows", Label: "Flows on this object",
			Path: "/lightning/setup/ObjectManager/" + s.Name + "/FlowTriggers/view"},
	}
}

func (s SObject) YankTargets() []YankTarget {
	ts := []YankTarget{
		{ID: "api", Label: "API name", Value: s.Name, Shortcut: "a"},
	}
	if s.Label != "" && s.Label != s.Name {
		ts = append(ts, YankTarget{ID: "label", Label: "Label", Value: s.Label, Shortcut: "l"})
	}
	ts = append(ts, YankTarget{ID: "soql", Label: "SOQL skeleton",
		Value: "SELECT Id FROM " + s.Name, Shortcut: "s"})
	if s.KeyPrefix != "" {
		ts = append(ts, YankTarget{ID: "prefix", Label: "Key prefix", Value: s.KeyPrefix})
	}
	return ts
}

// ----------------------------------------------------------------------
// Field — needs parent sObject context, so we expose a FieldRef wrapper
// the UI populates before calling Targets(). Field's DurableId is
// "<SObject>.<FieldApiName>" for standard and custom fields on standard
// objects; namespaced fields already carry their namespace in Name.
// ----------------------------------------------------------------------

type FieldRef struct {
	SObjectName string
	Field       Field
}

// Field URLs in Lightning use the field's DeveloperName segment, NOT its
// QualifiedApiName. The two differ for reference (lookup/master-detail)
// fields: `ParentId` in the API is `Parent` in the URL. For everything
// else (standard and custom) they match, because custom fields store
// DeveloperName as `<name>__c` same as QualifiedApiName.
//
// We derive DeveloperName at URL-build time rather than fetching
// FieldDefinition per-field, so this stays offline-friendly.
func (r FieldRef) Targets() []OpenTarget {
	dev := lightningFieldSegment(r.Field)
	return []OpenTarget{
		{ID: "view", Label: "Field Detail",
			Path: "/lightning/setup/ObjectManager/" + r.SObjectName +
				"/FieldsAndRelationships/" + dev + "/view"},
		{ID: "edit", Label: "Edit Field",
			Path: "/lightning/setup/ObjectManager/" + r.SObjectName +
				"/FieldsAndRelationships/" + dev + "/edit"},
		{ID: "list", Label: "Fields & Relationships (parent)",
			Path: "/lightning/setup/ObjectManager/" + r.SObjectName +
				"/FieldsAndRelationships/view?search=" + url.QueryEscape(r.Field.Name)},
		{ID: "manager", Label: "Parent Object Manager",
			Path: "/lightning/setup/ObjectManager/" + r.SObjectName + "/Details/view"},
		{ID: "records", Label: "Parent records list",
			Path: "/lightning/o/" + r.SObjectName + "/list"},
	}
}

func (r FieldRef) YankTargets() []YankTarget {
	ts := []YankTarget{
		{ID: "api", Label: "API name", Value: r.Field.Name, Shortcut: "a"},
		{ID: "qualified", Label: "Object.Field",
			Value: r.SObjectName + "." + r.Field.Name, Shortcut: "q"},
	}
	if r.Field.Label != "" && r.Field.Label != r.Field.Name {
		ts = append(ts, YankTarget{ID: "label", Label: "Label", Value: r.Field.Label, Shortcut: "l"})
	}
	return ts
}

// lightningFieldSegment maps a Field's API name to the URL segment
// Lightning's Object Manager uses. Reference fields drop the trailing
// "Id" suffix; custom fields keep their __c suffix; everything else is
// returned verbatim.
func lightningFieldSegment(f Field) string {
	name := f.Name
	if f.Type == "reference" && strings.HasSuffix(name, "Id") && !strings.HasSuffix(name, "__c") {
		return strings.TrimSuffix(name, "Id")
	}
	return name
}

// ----------------------------------------------------------------------
// Flow — Flow Builder for the active (or latest) version by default,
// then the Setup about page, then the all-Flows list.
// ----------------------------------------------------------------------

func (f Flow) Targets() []OpenTarget {
	// Which version `o` opens is a user setting (cfgFlowOpenActive,
	// pushed down from [ui.extensions] flow_open_version). Default =
	// LATEST version regardless of status, matching Salesforce Setup's
	// own flow list: when a draft is newer than the active version,
	// opening the flow means editing that draft — opening the active
	// one would edit a stale version (and Builder forces Save-As on
	// change). "active" flips the ordering; either way the other
	// version stays available as a secondary target when it differs.
	latestTarget := func(id string) OpenTarget {
		label := "Flow Builder (latest"
		if f.LatestVersionNum > 0 {
			label += fmt.Sprintf(" v%d", f.LatestVersionNum)
			if f.ActiveVersionID != "" && f.LatestVersionID != f.ActiveVersionID {
				label += " — draft"
			}
		}
		label += ")"
		return OpenTarget{
			Label: label,
			Path:  "/builder_platform_interaction/flowBuilder.app?flowId=" + id,
		}
	}
	activeTarget := func() OpenTarget {
		label := "Flow Builder (active"
		if f.ActiveVersionNum > 0 {
			label += fmt.Sprintf(" v%d", f.ActiveVersionNum)
		}
		label += ")"
		return OpenTarget{
			Label: label,
			Path:  "/builder_platform_interaction/flowBuilder.app?flowId=" + f.ActiveVersionID,
		}
	}

	var t []OpenTarget
	if cfgFlowOpenActive() && f.ActiveVersionID != "" {
		// Active-first: never-activated flows fall through to the
		// latest-first branch below (there's no active version to open).
		tgt := activeTarget()
		tgt.ID = "builder"
		t = append(t, tgt)
		if f.LatestVersionID != "" && f.LatestVersionID != f.ActiveVersionID {
			tgt := latestTarget(f.LatestVersionID)
			tgt.ID = "builder-latest"
			t = append(t, tgt)
		}
	} else {
		id := f.LatestVersionID
		if id == "" {
			id = f.ActiveVersionID
		}
		if id != "" {
			tgt := latestTarget(id)
			tgt.ID = "builder"
			t = append(t, tgt)
		}
		if f.ActiveVersionID != "" && f.ActiveVersionID != id {
			tgt := activeTarget()
			tgt.ID = "builder-active"
			t = append(t, tgt)
		}
	}
	// Setup "about" / activations page for the flow definition. The
	// Lightning setup URL uses the DefinitionId as a path query.
	t = append(t, OpenTarget{
		ID: "about", Label: "Flow definition (Setup)",
		Path: "/lightning/setup/Flows/page?address=%2F" + f.DefinitionID,
	})
	t = append(t, OpenTarget{
		ID: "list", Label: "All Flows",
		Path: "/lightning/setup/Flows/home",
	})
	return t
}

func (f Flow) YankTargets() []YankTarget {
	var ts []YankTarget
	if f.DeveloperName != "" {
		ts = append(ts, YankTarget{ID: "api", Label: "API name (DeveloperName)", Value: f.DeveloperName, Shortcut: "a"})
	}
	if f.MasterLabel != "" && f.MasterLabel != f.DeveloperName {
		ts = append(ts, YankTarget{ID: "label", Label: "Label", Value: f.MasterLabel, Shortcut: "l"})
	}
	if f.DefinitionID != "" {
		ts = append(ts, YankTarget{ID: "defid", Label: "Definition ID", Value: f.DefinitionID, Shortcut: "d"})
	}
	if id := f.ActiveVersionID; id != "" {
		ts = append(ts, YankTarget{ID: "verid", Label: "Active version ID", Value: id, Shortcut: "v"})
	} else if id := f.LatestVersionID; id != "" {
		ts = append(ts, YankTarget{ID: "verid", Label: "Latest version ID", Value: id, Shortcut: "v"})
	}
	return ts
}

// FlowVersion: Flow Builder opened to that specific version, then the
// containing Flow definition's about page, then the list.
// FlowVersionViewDefinitionTargetID marks the synthetic open-menu
// target that drills into the in-terminal definition viewer instead of
// opening a URL. The UI's fireMenuTarget intercepts this ID (it has no
// Path) — same pattern as the community-login picker.
const FlowVersionViewDefinitionTargetID = "view-definition"

func (v FlowVersion) Targets() []OpenTarget {
	var t []OpenTarget
	if v.ID != "" {
		t = append(t, OpenTarget{
			ID: "builder", Label: "Flow Builder (this version)",
			Path: "/builder_platform_interaction/flowBuilder.app?flowId=" + v.ID,
		})
		// In-app: read the raw definition JSON in the terminal. No Path
		// — fireMenuTarget catches this ID and drills instead of opening.
		t = append(t, OpenTarget{
			ID: FlowVersionViewDefinitionTargetID, Label: "View definition (in-terminal)",
			Shortcut: "d",
		})
	}
	if v.DefinitionID != "" {
		t = append(t, OpenTarget{
			ID: "about", Label: "Flow definition (Setup)",
			Path: "/lightning/setup/Flows/page?address=%2F" + v.DefinitionID,
		})
	}
	t = append(t, OpenTarget{
		ID: "list", Label: "All Flows",
		Path: "/lightning/setup/Flows/home",
	})
	return t
}

func (v FlowVersion) YankTargets() []YankTarget {
	var ts []YankTarget
	if v.MasterLabel != "" {
		ts = append(ts, YankTarget{ID: "label", Label: "Label", Value: v.MasterLabel, Shortcut: "l"})
	}
	if v.ID != "" {
		ts = append(ts, YankTarget{ID: "verid", Label: "Version ID", Value: v.ID, Shortcut: "v"})
	}
	if v.DefinitionID != "" {
		ts = append(ts, YankTarget{ID: "defid", Label: "Definition ID", Value: v.DefinitionID, Shortcut: "d"})
	}
	return ts
}

// ----------------------------------------------------------------------
// Records / SOQL rows: default to the Lightning record detail, plus an
// edit path and a classic-UI fallback for when the Lightning page
// misbehaves.
// ----------------------------------------------------------------------

type RecordRef struct {
	Record map[string]any
	// InspectorBase is the user's configured Salesforce Inspector
	// Reloaded inspect.html URL (from settings.toml — varies per
	// browser + per install). When set, RecordRef.Targets() prepends
	// an "Inspector · Show all data" absolute-URL target that opens
	// the inspector against this record's (host, objectType, id).
	InspectorBase string
	// InstanceHost is the org's host (e.g. "my.example.my.salesforce.com")
	// used to build the Inspector URL. Populated by the UI before
	// calling Targets() since the sf.Org metadata isn't on the raw
	// record map.
	InstanceHost string
	// ExtraTargets are appended to Targets() after the standard
	// record targets. Used by the UI to inject context-specific
	// actions that need org state to build (e.g. "Log in to <site>
	// as user" for Contact rows with a community user — needs the
	// org's Network list + a ContactId→UserId lookup, neither of
	// which the bare record map carries).
	ExtraTargets []OpenTarget
}

func (r RecordRef) Targets() []OpenTarget {
	sobj, id := SObjectAndIDFromRecord(r.Record)
	if sobj == "" || id == "" {
		return []OpenTarget{{ID: "home", Label: "Home", Path: "/lightning/page/home"}}
	}
	targets := []OpenTarget{
		{ID: "view", Label: "Record detail", Shortcut: "r",
			Path: "/lightning/r/" + sobj + "/" + id + "/view"},
		{ID: "edit", Label: "Edit record", Shortcut: "e",
			Path: "/lightning/r/" + sobj + "/" + id + "/edit"},
	}
	if r.InspectorBase != "" && r.InstanceHost != "" {
		targets = append(targets, OpenTarget{
			ID:                    "inspector",
			Label:                 "Inspector — Show all data",
			Shortcut:              "i",
			AbsoluteURL:           buildInspectorURL(r.InspectorBase, r.InstanceHost, sobj, id),
			AllowBrowserExtension: true,
		})
	}
	targets = append(targets,
		OpenTarget{ID: "list", Label: sobj + " list",
			Path: "/lightning/o/" + sobj + "/list"},
		OpenTarget{ID: "manager", Label: sobj + " Object Manager",
			Path: "/lightning/setup/ObjectManager/" + sobj + "/Details/view"},
	)
	targets = append(targets, r.ExtraTargets...)
	return targets
}

func (r RecordRef) YankTargets() []YankTarget {
	sobj, id := SObjectAndIDFromRecord(r.Record)
	var ts []YankTarget
	if id != "" {
		ts = append(ts, YankTarget{ID: "id", Label: "Record ID", Value: id, Shortcut: "i"})
	}
	if name := recordName(r.Record); name != "" {
		ts = append(ts, YankTarget{ID: "name", Label: "Name", Value: name, Shortcut: "n"})
	}
	if sobj != "" && id != "" {
		ts = append(ts, YankTarget{ID: "soql", Label: "SOQL by Id",
			Value: "SELECT Id FROM " + sobj + " WHERE Id = '" + id + "'", Shortcut: "s"})
	}
	return ts
}

// recordName returns the record's "Name" field as a string when present,
// for the yank menu's Name target. Empty when the record has no Name
// (some objects key on a different field; the menu just omits the entry).
func recordName(rec map[string]any) string {
	if rec == nil {
		return ""
	}
	if v, ok := rec["Name"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// buildInspectorURL constructs a Salesforce Inspector Reloaded
// inspect.html URL from the user's extension base + a record's
// identity. Base is typically "moz-extension://<guid>/inspect.html"
// (Firefox) or "chrome-extension://<id>/inspect.html" (Chromium).
// Any existing query string on the base is preserved.
func buildInspectorURL(base, host, sobject, id string) string {
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	q := url.Values{}
	q.Set("host", host)
	q.Set("objectType", sobject)
	q.Set("recordId", id)
	return base + sep + q.Encode()
}

func SObjectAndIDFromRecord(rec map[string]any) (string, string) {
	var sobj, id string
	if attrs, ok := rec["attributes"].(map[string]any); ok {
		if t, ok := attrs["type"].(string); ok {
			sobj = t
		}
		if u, ok := attrs["url"].(string); ok {
			parts := strings.Split(u, "/")
			if len(parts) > 0 {
				id = parts[len(parts)-1]
			}
		}
	}
	if id == "" {
		if s, ok := rec["Id"].(string); ok {
			id = s
		}
	}
	return sobj, id
}

// ----------------------------------------------------------------------
// Apex log / Deploy / Package / Org.
// ----------------------------------------------------------------------

func (l ApexLogRow) Targets() []OpenTarget {
	// Lightning has no per-log detail URL. Classic URL still works for
	// deep-linking; fall back to the list.
	t := []OpenTarget{
		{ID: "list", Label: "Apex Debug Logs",
			Path: "/lightning/setup/ApexDebugLogs/home"},
	}
	if l.ID != "" {
		t = append([]OpenTarget{{
			ID:    "classic",
			Label: "Log detail (classic)",
			Path:  "/p/setup/layout/ApexDebugLogDetailEdit?apex_log_id=" + l.ID,
		}}, t...)
	}
	return t
}

func (l ApexLogRow) YankTargets() []YankTarget {
	if l.ID == "" {
		return nil
	}
	return []YankTarget{
		{ID: "id", Label: "Log ID", Value: l.ID, Shortcut: "i"},
	}
}

func (r DeployRow) Targets() []OpenTarget {
	t := []OpenTarget{
		{ID: "status", Label: "Deploy Status",
			Path: "/lightning/setup/DeployStatus/home"},
	}
	if r.ID != "" {
		// Lightning-native wrapper: the Setup page renders the
		// classic monitorDeploymentsDetails.apexp inside its
		// Lightning chrome via the address= query param. Same
		// pattern Apex / Profiles / Perm Sets use to deep-link
		// from a setup-tree node into a record-specific page.
		// Earlier this used the bare classic URL which made
		// Salesforce drop the user out of the Lightning shell.
		t = append([]OpenTarget{{
			ID: "detail", Label: "Deploy detail",
			Path: "/lightning/setup/DeployStatus/page?address=%2Fchangemgmt%2FmonitorDeploymentsDetails.apexp%3FasyncId%3D" + r.ID,
		}}, t...)
	}
	return t
}

func (r DeployRow) YankTargets() []YankTarget {
	if r.ID == "" {
		return nil
	}
	return []YankTarget{
		{ID: "id", Label: "Deploy ID", Value: r.ID, Shortcut: "i"},
		{ID: "report", Label: "sf report command",
			Value: "sf project deploy report --job-id " + r.ID, Shortcut: "c"},
	}
}

func (p InstalledPackage) Targets() []OpenTarget {
	return []OpenTarget{
		{ID: "list", Label: "Installed Packages",
			Path: "/lightning/setup/ImportedPackage/home"},
	}
}

func (p InstalledPackage) YankTargets() []YankTarget {
	var ts []YankTarget
	if p.SubscriberPackageNamespace != "" {
		ts = append(ts, YankTarget{ID: "ns", Label: "Namespace", Value: p.SubscriberPackageNamespace, Shortcut: "n"})
	}
	if p.SubscriberPackageVersionNumber != "" {
		ts = append(ts, YankTarget{ID: "ver", Label: "Version", Value: p.SubscriberPackageVersionNumber, Shortcut: "v"})
	}
	if p.SubscriberPackageVersionID != "" {
		ts = append(ts, YankTarget{ID: "verid", Label: "Version ID (04t)", Value: p.SubscriberPackageVersionID, Shortcut: "d"})
	}
	return ts
}

func (Org) Targets() []OpenTarget {
	return []OpenTarget{
		{ID: "home", Label: "Lightning Home", Path: "/lightning/page/home"},
		{ID: "setup", Label: "Setup", Path: "/lightning/setup/SetupOneHome/home"},
		{ID: "objects", Label: "Object Manager",
			Path: "/lightning/setup/ObjectManager/home"},
		{ID: "flows", Label: "All Flows",
			Path: "/lightning/setup/Flows/home"},
		{ID: "users", Label: "Users",
			Path: "/lightning/setup/ManageUsers/home"},
	}
}

// ----------------------------------------------------------------------
// Compile-time check that each known type satisfies Openable.
// ----------------------------------------------------------------------

var (
	_ Openable = SObject{}
	_ Openable = FieldRef{}
	_ Openable = Flow{}
	_ Openable = FlowVersion{}
	_ Openable = ApexLogRow{}
	_ Openable = DeployRow{}
	_ Openable = InstalledPackage{}
	_ Openable = Org{}
	_ Openable = RecordRef{}
	_ Openable = PermissionSet{}
	_ Openable = PermissionSetGroup{}
	_ Openable = Profile{}
	_ Openable = ReportSummary{}
	_ Openable = ApexClassRow{}
	_ Openable = LWCBundle{}
	_ Openable = AuraBundle{}
	_ Openable = Notification{}
	_ Openable = UserRow{}
	_ Openable = AsyncJobRow{}
	_ Openable = UserLicenseRow{}
	_ Openable = PermSetLicenseRow{}
	_ Openable = Limit{}
	_ Openable = QueueRow{}
	_ Openable = PublicGroupRow{}
)

// ReportSummary: report viewer (the saved report's run page), then
// Reports home (the all-reports list).
func (r ReportSummary) Targets() []OpenTarget {
	var t []OpenTarget
	if r.ID != "" {
		t = append(t, OpenTarget{
			ID:    "view",
			Label: "Report viewer",
			Path:  "/lightning/r/Report/" + r.ID + "/view",
		})
	}
	t = append(t, OpenTarget{
		ID: "list", Label: "All Reports",
		Path: "/lightning/o/Report/home",
	})
	return t
}
