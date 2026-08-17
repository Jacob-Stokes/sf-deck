// Package sfurl parses Salesforce-shaped URLs and bare Ids into a
// kind/ref tuple sf-deck can route on. Used by:
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
	Kind devproject.ItemKind

	// SObject is the parent sObject API name when the URL identifies
	// an sObject context (record, field, validation rule, record type).
	// Empty otherwise.
	SObject string

	ID string

	Host string

	Sandbox string

	Extra map[string]string

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

	switch {
	case strings.HasPrefix(u.Path, "/lightning/r/"):
		return parseLightningRecord(u, p)
	case strings.HasPrefix(u.Path, "/lightning/o/"):
		return parseLightningSObject(u, p)
	case strings.HasPrefix(u.Path, "/lightning/setup/"):
		return parseLightningSetup(u, p)
	}

	if id := classicRecordID(u.Path); id != "" {
		p.ID = id
		p.Kind = devproject.KindRecord
		return p, nil
	}

	return Parsed{}, fmt.Errorf("unrecognised Salesforce URL path %q", u.Path)
}

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

func parseLightningSetup(u *url.URL, p Parsed) (Parsed, error) {
	parts := splitPath(u.Path[len("/lightning/setup/"):])
	if len(parts) == 0 {
		return Parsed{}, fmt.Errorf("malformed setup URL %q", u.Path)
	}
	section := parts[0]

	if section == "ObjectManager" && len(parts) >= 2 {
		return parseObjectManager(parts[1:], p)
	}

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
		p.Kind = devproject.KindSObject
	default:
		p.Kind = devproject.KindSObject
	}
	return p, nil
}

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
		p.Kind = devproject.KindPublicGroup
	case "300":
		p.Kind = devproject.KindFlow
	case "301":
		p.Kind = devproject.KindFlowVersion
	case "00D":
		p.Kind = devproject.KindRecord
	default:
		p.Kind = devproject.KindRecord
	}
	return p
}

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

func isSalesforceHost(host string) bool {
	host = strings.ToLower(host)
	return strings.HasSuffix(host, ".lightning.force.com") ||
		strings.HasSuffix(host, ".my.salesforce.com") ||
		strings.HasSuffix(host, ".salesforce.com")
}

func sandboxName(host string) string {
	host = strings.ToLower(host)
	if !strings.Contains(host, ".sandbox.") {
		return ""
	}
	first := host
	if dot := strings.Index(host, "."); dot >= 0 {
		first = host[:dot]
	}
	if dd := strings.Index(first, "--"); dd >= 0 {
		return first[dd+2:]
	}
	return ""
}
