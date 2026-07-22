// Package sfurl parses Salesforce-shaped URLs and bare Ids into a
// kind/ref tuple sf-deck can route on. Used by:
//
//   - Global search modal: "I pasted a Lightning URL, take me there."
//   - Dev-project detail surface: "Add this URL's resource to the
//     current project."
//
// The parser is intentionally pure: it doesn't hit the network, doesn't
// know what's loaded in cache, doesn't decide navigation. It returns a
// shape; callers route on it.
package sfurl

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

// Parsed is the result of parsing a Salesforce URL or bare Id. Callers
// branch on Kind to dispatch — the field semantics mirror what
// devproject.Item* expects so dev-project add can pass straight
// through.
type Parsed struct {
	// Kind is one of the devproject.Kind* values when we recognise a
	// surface sf-deck models. Bare Id with unknown prefix returns
	// kind="record" with SObject="" so the caller can decide whether
	// to look it up or fall back.
	Kind devproject.ItemKind

	// SObject is the parent sObject API name when the URL identifies
	// an sObject context (record, field, validation rule, record type).
	// Empty otherwise.
	SObject string

	// ID is the 15- or 18-char Salesforce Id when the URL carries
	// one. Empty when the URL identifies a kind without an Id (e.g.
	// /lightning/o/Account/list with no filterName).
	ID string

	// Host is the host the URL was parsed from (e.g.
	// "acme.lightning.force.com" or "acme--uat.sandbox.lightning.force.com").
	// Empty for bare Ids. Caller can use this to flag mismatches with
	// the active org's instance URL.
	Host string

	// Sandbox reports whether the host has a sandbox suffix
	// ("--<name>.sandbox.lightning.force.com"). Empty for bare Ids
	// and non-sandbox hosts.
	Sandbox string

	// Extra carries kind-specific bits the primary fields don't model:
	//   - "listViewId" for /lightning/o/<sobj>/list?filterName=<id>
	//   - "fieldId" for ObjectManager field URLs
	//   - "flowVersionId" for FlowVersion URLs
	// Always non-nil when Kind is set.
	Extra map[string]string

	// Raw is the original input string, for error messages and
	// activity logs.
	Raw string
}

// Parse takes either a URL or a bare Salesforce Id and returns a
// Parsed describing what it points at. Returns an error when the
// input is empty, malformed, or recognisable but routes to a kind
// sf-deck doesn't model. Callers should treat error as "couldn't
// route this" — print a toast or fall back to text search.
func Parse(input string) (Parsed, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Parsed{}, fmt.Errorf("empty input")
	}

	// Bare Id path — 15 or 18 alphanumeric chars, no slashes / dots.
	if isBareID(input) {
		return parseBareID(input), nil
	}

	// URL path — must parse as a URL with a Salesforce host.
	u, err := url.Parse(input)
	if err != nil {
		return Parsed{}, fmt.Errorf("not a URL: %w", err)
	}
	host := u.Host
	if !isSalesforceHost(host) {
		return Parsed{}, fmt.Errorf("not a Salesforce URL (host %q)", host)
	}

	p := Parsed{
		Host:    host,
		Sandbox: sandboxName(host),
		Extra:   map[string]string{},
		Raw:     input,
	}

	// Lightning routes: /lightning/{r,o,setup}/...
	switch {
	case strings.HasPrefix(u.Path, "/lightning/r/"):
		return parseLightningRecord(u, p)
	case strings.HasPrefix(u.Path, "/lightning/o/"):
		return parseLightningSObject(u, p)
	case strings.HasPrefix(u.Path, "/lightning/setup/"):
		return parseLightningSetup(u, p)
	}

	// Classic record URL: /<id> with nothing else
	if id := classicRecordID(u.Path); id != "" {
		p.ID = id
		p.Kind = devproject.KindRecord
		return p, nil
	}

	return Parsed{}, fmt.Errorf("unrecognised Salesforce URL path %q", u.Path)
}

// parseLightningRecord handles /lightning/r/<sObject>/<id>/view (and /edit).
func parseLightningRecord(u *url.URL, p Parsed) (Parsed, error) {
	parts := splitPath(u.Path[len("/lightning/r/"):])
	if len(parts) < 2 {
		return Parsed{}, fmt.Errorf("malformed record URL %q", u.Path)
	}
	p.SObject = parts[0]
	p.ID = parts[1]
	p.Kind = devproject.KindRecord
	return p, nil
}

// parseLightningSObject handles /lightning/o/<sObject>/list and friends.
func parseLightningSObject(u *url.URL, p Parsed) (Parsed, error) {
	parts := splitPath(u.Path[len("/lightning/o/"):])
	if len(parts) == 0 {
		return Parsed{}, fmt.Errorf("malformed sObject URL %q", u.Path)
	}
	p.SObject = parts[0]
	if filter := u.Query().Get("filterName"); filter != "" && isIDLike(filter) {
		p.ID = filter
		p.Kind = devproject.KindSObject // listView routing — caller can branch on Extra
		p.Extra["listViewId"] = filter
		return p, nil
	}
	p.Kind = devproject.KindSObject
	return p, nil
}

// parseLightningSetup handles the long tail of /lightning/setup/<x>/page?address=...
// shapes. Salesforce routes most setup pages through this single
// component, so the real target is encoded in the address query param.
func parseLightningSetup(u *url.URL, p Parsed) (Parsed, error) {
	parts := splitPath(u.Path[len("/lightning/setup/"):])
	if len(parts) == 0 {
		return Parsed{}, fmt.Errorf("malformed setup URL %q", u.Path)
	}
	section := parts[0]

	// ObjectManager has its own structure (no address param):
	//   /lightning/setup/ObjectManager/<sObject>/Details/view
	//   /lightning/setup/ObjectManager/<sObject>/FieldsAndRelationships/<fieldId>/view
	if section == "ObjectManager" && len(parts) >= 2 {
		return parseObjectManager(parts[1:], p)
	}

	// Generic setup page — pull the embedded id from ?address=
	if addr := u.Query().Get("address"); addr != "" {
		if id := embeddedID(addr); id != "" {
			p.ID = id
		}
	}

	switch section {
	case "Flows":
		p.Kind = devproject.KindFlow
	case "ApexClasses":
		p.Kind = devproject.KindApexClass
	case "ApexTriggers":
		p.Kind = devproject.KindApexTrigger
	case "PermSets":
		p.Kind = devproject.KindPermissionSet
	case "PermSetGroups":
		p.Kind = devproject.KindPermissionSetGroup
	case "Profiles":
		p.Kind = devproject.KindProfile
	case "Queues":
		p.Kind = devproject.KindQueue
	case "PublicGroups":
		p.Kind = devproject.KindPublicGroup
	case "LightningComponentBundles", "LightningComponents":
		p.Kind = devproject.KindLWC
	case "AuraComponents":
		p.Kind = devproject.KindAura
	default:
		return Parsed{}, fmt.Errorf("unrecognised setup section %q", section)
	}
	return p, nil
}

// parseObjectManager walks the post-/ObjectManager/ path:
//
//	[Account, Details, view]                                 → SObject
//	[Account, FieldsAndRelationships, <fieldId>, view]       → Field
//	[Account, ValidationRules, <id>, view]                   → ValidationRule
//	[Account, RecordTypes, <id>, view]                       → RecordType
//	[Account, ApexTriggers, <id>, view]                      → ApexTrigger (parent-scoped)
func parseObjectManager(parts []string, p Parsed) (Parsed, error) {
	p.SObject = parts[0]
	if len(parts) == 1 || parts[1] == "Details" {
		p.Kind = devproject.KindSObject
		return p, nil
	}
	section := parts[1]
	var subID string
	if len(parts) >= 3 && isIDLike(parts[2]) {
		subID = parts[2]
		p.ID = subID
	}
	switch section {
	case "FieldsAndRelationships":
		p.Kind = devproject.KindField
		if subID != "" {
			p.Extra["fieldId"] = subID
		}
	case "ValidationRules":
		p.Kind = devproject.KindValidationRule
	case "RecordTypes":
		p.Kind = devproject.KindRecordType
	case "ApexTriggers":
		p.Kind = devproject.KindApexTrigger
	case "Layouts":
		// Layouts aren't modelled yet — return SObject so navigation
		// at least lands on the parent.
		p.Kind = devproject.KindSObject
	default:
		// Unknown ObjectManager section — fall back to the parent sObject.
		p.Kind = devproject.KindSObject
	}
	return p, nil
}

// parseBareID classifies a 15/18-char Salesforce Id by its 3-char
// key prefix. Unknown prefixes return KindRecord with SObject="" —
// the caller can either look it up or fall back to "open the record
// detail surface and let it figure out what kind."
func parseBareID(id string) Parsed {
	p := Parsed{
		ID:    id,
		Extra: map[string]string{},
		Raw:   id,
	}
	if len(id) < 3 {
		p.Kind = devproject.KindRecord
		return p
	}
	switch id[:3] {
	case "00B":
		p.Kind = devproject.KindSObject
		p.Extra["listViewId"] = id
	case "01p":
		p.Kind = devproject.KindApexClass
	case "01q":
		p.Kind = devproject.KindApexTrigger
	case "0H4":
		p.Kind = devproject.KindPermissionSetGroup
	case "0PS":
		p.Kind = devproject.KindPermissionSet
	case "00e":
		p.Kind = devproject.KindProfile
	case "00G":
		// 00G is Group — could be Queue or Public Group. Default to
		// PublicGroup; caller can re-route based on Group.Type if
		// needed.
		p.Kind = devproject.KindPublicGroup
	case "300":
		p.Kind = devproject.KindFlow
	case "301":
		p.Kind = devproject.KindFlowVersion
	case "00D":
		// Org Id — no surface to navigate to; treat as record.
		p.Kind = devproject.KindRecord
	default:
		p.Kind = devproject.KindRecord
	}
	return p
}

// --- helpers ----------------------------------------------------------

// classicRecordID returns the Id when the path is just `/<id>` (with
// optional trailing slash). Empty otherwise.
func classicRecordID(path string) string {
	parts := splitPath(path)
	if len(parts) != 1 {
		return ""
	}
	if !isIDLike(parts[0]) {
		return ""
	}
	return parts[0]
}

// embeddedID pulls a Salesforce Id out of an arbitrary string (used
// for the ?address=%2F<id> setup-page pattern). Returns the first
// 15- or 18-char ID-shaped substring or "".
func embeddedID(s string) string {
	// Cheap scan — Salesforce Ids are alphanumeric, contiguous, fixed
	// length. Walk the string and check 18-char then 15-char windows.
	for i := 0; i+15 <= len(s); i++ {
		if i+18 <= len(s) && isIDLike(s[i:i+18]) {
			return s[i : i+18]
		}
		if isIDLike(s[i : i+15]) {
			return s[i : i+15]
		}
	}
	return ""
}

func isBareID(s string) bool {
	if !isIDLike(s) {
		return false
	}
	return !strings.ContainsAny(s, "/?:.")
}

func isIDLike(s string) bool {
	if len(s) != 15 && len(s) != 18 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return true
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// isSalesforceHost reports whether host looks like a Salesforce
// Lightning or My-Domain URL. Conservative — known suffixes only,
// don't try to infer.
func isSalesforceHost(host string) bool {
	host = strings.ToLower(host)
	return strings.HasSuffix(host, ".lightning.force.com") ||
		strings.HasSuffix(host, ".my.salesforce.com") ||
		strings.HasSuffix(host, ".salesforce.com")
}

// sandboxName returns the sandbox name from a host like
// "acme--uat.sandbox.lightning.force.com" → "uat". Empty for
// production hosts.
func sandboxName(host string) string {
	host = strings.ToLower(host)
	if !strings.Contains(host, ".sandbox.") {
		return ""
	}
	// First label is "<orgname>--<sandbox>"
	first := host
	if dot := strings.Index(host, "."); dot >= 0 {
		first = host[:dot]
	}
	if dd := strings.Index(first, "--"); dd >= 0 {
		return first[dd+2:]
	}
	return ""
}
