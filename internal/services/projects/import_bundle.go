package projects

// import_bundle.go — parse an existing sfdx project's package.xml and
// add the listed components to a DevProject as items. The reverse
// of the bundle.create writer: agents point us at a `force-app/`
// checkout (a git repo, another tool's export, an existing bundle
// dir) and we infer what kinds of items to register.
//
// Compound shapes (Field = "<sObject>.<Field>") survive the round
// trip because Salesforce's package.xml encodes them the same way.
// Kinds we can't safely round-trip (FlowVersion, Record, the
// org-agnostic soql_query / apex_snippet) are skipped — no
// MetadataAPI equivalent.

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

// ImportBundleInput is the request shape. ProjectID + Path required;
// the path may point at a sfdx project directory (containing
// `package.xml` directly) or at a `package.xml` file. OrgUser is
// stamped on each created item so org-scoped kinds resolve to the
// caller's intended org context.
type ImportBundleInput struct {
	ProjectID string
	Path      string
	OrgUser   string
}

// ImportBundleResult tallies what happened. Added is the count of
// newly-inserted items; Skipped is members the project already had
// (dedup is "same kind + same ref"); Unknown is metadata types
// we can't map back to an ItemKind — the caller surfaces these so
// the user knows what wasn't tracked.
type ImportBundleResult struct {
	Added       int                         `json:"added"`
	Skipped     int                         `json:"skipped"`
	Unknown     []string                    `json:"unknown,omitempty"`
	AddedByKind map[devproject.ItemKind]int `json:"added_by_kind,omitempty"`
}

// ImportBundle reads the package.xml at the given path and inserts
// each member as a project item. Idempotent — items already in the
// project are counted as Skipped and not re-added. Synchronous
// (parsing + a few INSERTs); no shell-out.
func ImportBundle(s *devproject.Store, in ImportBundleInput) (ImportBundleResult, error) {
	if s == nil {
		return ImportBundleResult{}, errors.New("nil store")
	}
	if in.ProjectID == "" {
		return ImportBundleResult{}, errors.New("project id is required")
	}
	if in.Path == "" {
		return ImportBundleResult{}, errors.New("path is required")
	}
	dp, err := s.GetDevProject(in.ProjectID)
	if err != nil {
		return ImportBundleResult{}, err
	}
	if dp == nil {
		return ImportBundleResult{}, ErrNotFound{ID: in.ProjectID}
	}

	manifestPath, err := resolveManifestPath(in.Path)
	if err != nil {
		return ImportBundleResult{}, err
	}
	pkg, err := parsePackageXML(manifestPath)
	if err != nil {
		return ImportBundleResult{}, err
	}

	// Pre-load existing project items keyed by (kind, ref) so we
	// can dedup without an INSERT-then-rollback cycle.
	existing, err := s.ListItems(in.ProjectID, "")
	if err != nil {
		return ImportBundleResult{}, err
	}
	seen := map[string]bool{}
	for _, it := range existing {
		seen[itemKey(it.Kind, it.Ref)] = true
	}

	result := ImportBundleResult{
		AddedByKind: map[devproject.ItemKind]int{},
	}
	unknownSet := map[string]bool{}
	now := time.Now()
	for _, t := range pkg.Types {
		kind, ok := metadataTypeToKind(t.Name)
		if !ok {
			unknownSet[t.Name] = true
			continue
		}
		for _, member := range t.Members {
			ref := strings.TrimSpace(member)
			if ref == "" || ref == "*" {
				// Wildcards aren't trackable as discrete items.
				continue
			}
			key := itemKey(kind, ref)
			if seen[key] {
				result.Skipped++
				continue
			}
			seen[key] = true
			item := devproject.Item{
				DevProjectID: in.ProjectID,
				OrgUser:      in.OrgUser,
				Kind:         kind,
				Ref:          ref,
				// Type / Name aren't recoverable from a bare package.xml
				// member — the manifest is intentionally lean. Best
				// effort: leave Type empty; set Name = Ref so the
				// UI has something to render before the user runs a
				// describe.
				Name:    deriveName(kind, ref),
				Type:    deriveType(kind, ref),
				AddedAt: now,
			}
			if _, err := s.AddItem(item); err != nil {
				return result, fmt.Errorf("add %s %s: %w", kind, ref, err)
			}
			result.Added++
			result.AddedByKind[kind]++
		}
	}
	if len(unknownSet) > 0 {
		result.Unknown = make([]string, 0, len(unknownSet))
		for t := range unknownSet {
			result.Unknown = append(result.Unknown, t)
		}
	}
	return result, nil
}

// resolveManifestPath accepts either a sfdx-project dir or a direct
// path to package.xml. The TUI / bundle.link surface both treat a
// directory as the canonical shape; we honor that here.
func resolveManifestPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("normalize path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", abs, err)
	}
	if info.IsDir() {
		manifest := filepath.Join(abs, "package.xml")
		if _, err := os.Stat(manifest); err != nil {
			return "", fmt.Errorf("missing package.xml in %s", abs)
		}
		return manifest, nil
	}
	return abs, nil
}

// packageXML is the minimal subset of the MetadataAPI manifest we
// care about — list of types, each with members + name.
type packageXML struct {
	XMLName xml.Name         `xml:"Package"`
	Types   []packageXMLType `xml:"types"`
}

type packageXMLType struct {
	Members []string `xml:"members"`
	Name    string   `xml:"name"`
}

func parsePackageXML(path string) (packageXML, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return packageXML{}, fmt.Errorf("read %s: %w", path, err)
	}
	var pkg packageXML
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return packageXML{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return pkg, nil
}

// metadataTypeToKind is the inverse of metadataMember. Limited to
// shapes we can confidently round-trip — KindFlowVersion (folded
// into Flow at write time), KindRecord (not a metadata type),
// soql_query / apex_snippet (sf-deck local only) are intentionally
// not represented. Unknown types surface as result.Unknown so the
// caller can flag them.
func metadataTypeToKind(name string) (devproject.ItemKind, bool) {
	switch name {
	case "CustomObject":
		return devproject.KindSObject, true
	case "CustomField":
		return devproject.KindField, true
	case "Flow":
		return devproject.KindFlow, true
	case "ApexClass":
		return devproject.KindApexClass, true
	case "ApexTrigger":
		return devproject.KindApexTrigger, true
	case "Report":
		return devproject.KindReport, true
	case "PermissionSet":
		return devproject.KindPermissionSet, true
	case "PermissionSetGroup":
		return devproject.KindPermissionSetGroup, true
	case "Profile":
		return devproject.KindProfile, true
	case "ValidationRule":
		return devproject.KindValidationRule, true
	case "RecordType":
		return devproject.KindRecordType, true
	case "LightningComponentBundle":
		return devproject.KindLWC, true
	case "AuraDefinitionBundle":
		return devproject.KindAura, true
	case "Queue":
		return devproject.KindQueue, true
	case "Group":
		return devproject.KindPublicGroup, true
	}
	return "", false
}

// deriveName is the best-effort guess at what to stamp on Item.Name
// when the manifest only gave us a ref. The UI shows Name when set,
// falling back to Ref. For most kinds Ref IS the canonical
// human-readable identifier (an Account.Phone field reads the same
// in any UI), so echoing it as Name keeps the list legible.
//
// For fields we use the bare field portion (after the dot) so the
// UI doesn't render "Account.Phone Account.Phone" — the Type column
// already carries the parent sobject.
func deriveName(kind devproject.ItemKind, ref string) string {
	if kind == devproject.KindField {
		if i := strings.IndexByte(ref, '.'); i > 0 && i < len(ref)-1 {
			return ref[i+1:]
		}
	}
	if kind == devproject.KindReport {
		// Reports are "Folder/DeveloperName" — show just the dev name.
		if i := strings.IndexByte(ref, '/'); i > 0 && i < len(ref)-1 {
			return ref[i+1:]
		}
	}
	return ref
}

// deriveType captures the parent context where the ref encodes it.
// CustomField "<sObject>.<Field>" → Type = sObject; Report
// "<Folder>/<Name>" → Type = Folder. Other kinds leave Type empty;
// the bundle import can't recover info the manifest didn't carry.
func deriveType(kind devproject.ItemKind, ref string) string {
	switch kind {
	case devproject.KindField:
		if i := strings.IndexByte(ref, '.'); i > 0 {
			return ref[:i]
		}
	case devproject.KindReport:
		if i := strings.IndexByte(ref, '/'); i > 0 {
			return ref[:i]
		}
	}
	return ""
}

func itemKey(kind devproject.ItemKind, ref string) string {
	return string(kind) + "|" + ref
}
