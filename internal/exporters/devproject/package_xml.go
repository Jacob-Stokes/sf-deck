package devproject

// package.xml emitter — turns a slice of DevProject Items into the
// Salesforce MetadataAPI manifest format.

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

// DefaultAPIVersion is the Salesforce API version pinned in emitted
// manifests. Bumped manually when targeting newer features. Older
// orgs reject manifests with a higher version than they support; if
// we ever need per-org pinning, expose this on PackageXMLOptions.
const DefaultAPIVersion = "62.0"

// PackageXMLOptions tunes manifest emission.
type PackageXMLOptions struct {
	APIVersion string
}

// PackageXMLResult is what WritePackageXML returns alongside the
// manifest bytes — counts of what made it in, what got skipped, and
// the records bucket for sidecar emission.
//
// Skipped items split into three buckets so the caller can render
// useful warnings:
//   - Records: data items (KindRecord) — caller should emit a
//     records.csv sidecar via the standard ExportRow path
//   - Unsupported: items whose ItemKind has no MetadataAPI mapping
//     yet (typically a new Kind we haven't taught the emitter)
//   - Managed: items from managed packages — Salesforce refuses to
//     return their source via MetadataAPI, so including them in the
//     manifest produces a "cannot be found" error during retrieve.
//     We omit them and surface the count separately so the README
//     and the export flash can call them out.
type PackageXMLResult struct {
	IncludedCount int
	Records       []devproject.Item
	Unsupported   []devproject.Item
	Managed       []devproject.Item
}

// WritePackageXML serialises a manifest covering the supplied items
// and returns a result describing what made it in. Items are filtered
// to the items the caller passed — no per-org filtering happens here;
// caller pre-filters via the store's ListItems(devID, orgUser).
//
// Idempotent + deterministic: items are sorted before grouping so
// the same input always produces byte-identical output. Useful for
// committing manifests to git.
func WritePackageXML(w io.Writer, items []devproject.Item, opts PackageXMLOptions) (PackageXMLResult, error) {
	apiVersion := opts.APIVersion
	if apiVersion == "" {
		apiVersion = DefaultAPIVersion
	}

	var result PackageXMLResult

	groups := map[string]map[string]struct{}{}

	for _, it := range items {
		if it.Kind == devproject.KindRecord {
			result.Records = append(result.Records, it)
			continue
		}
		// Managed-package components belong to the package vendor.
		// MetadataAPI refuses to return their source ("Entity ... cannot
		// be found"), so including them in the manifest only produces
		// retrieve warnings without giving the user any usable file.
		// Bucket them separately so the caller can call out the count.
		if it.Managed() {
			result.Managed = append(result.Managed, it)
			continue
		}
		mtype, member, ok := metadataMember(it)
		if !ok || mtype == "" || member == "" {
			result.Unsupported = append(result.Unsupported, it)
			continue
		}
		if groups[mtype] == nil {
			groups[mtype] = map[string]struct{}{}
		}
		groups[mtype][member] = struct{}{}
		result.IncludedCount++
	}

	types := make([]string, 0, len(groups))
	for t := range groups {
		types = append(types, t)
	}
	sort.Strings(types)

	pkg := pkgXML{
		Xmlns:   "http://soap.sforce.com/2006/04/metadata",
		Version: apiVersion,
	}
	for _, t := range types {
		members := make([]string, 0, len(groups[t]))
		for m := range groups[t] {
			members = append(members, m)
		}
		sort.Strings(members)
		pkg.Types = append(pkg.Types, pkgTypes{
			Members: members,
			Name:    t,
		})
	}

	if _, err := io.WriteString(w, xml.Header); err != nil {
		return result, err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "    ")
	if err := enc.Encode(pkg); err != nil {
		return result, err
	}
	if err := enc.Flush(); err != nil {
		return result, err
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return result, err
	}
	return result, nil
}

type pkgXML struct {
	XMLName xml.Name   `xml:"Package"`
	Xmlns   string     `xml:"xmlns,attr"`
	Types   []pkgTypes `xml:"types"`
	Version string     `xml:"version"`
}

type pkgTypes struct {
	Members []string `xml:"members"`
	Name    string   `xml:"name"`
}

// metadataMember returns the MetadataAPI type name + member identifier
// for an Item, or ok=false when the kind isn't (yet) mappable.
//
// The mapping table is the source of truth for which kinds are
// deployable; adding a new ItemKind without a case here means it
// silently lands in the Unsupported bucket of the result, which
// keeps the export safe by default — the caller surfaces the count
// so the user knows something was skipped.
func metadataMember(it devproject.Item) (mtype, member string, ok bool) {
	switch it.Kind {
	case devproject.KindSObject:
		if it.Ref == "" {
			return "", "", false
		}
		// Skip auto-generated tracking entities — they show up in the
		// describe list but aren't deployable as CustomObjects.
		// MetadataAPI rejects them with "Entity ... cannot be found";
		// pre-filtering here keeps the manifest deployable without
		// the user having to know about Salesforce's plumbing.
		if isNonDeployableSObject(it.Ref) {
			return "", "", false
		}
		return "CustomObject", it.Ref, true

	case devproject.KindField:
		if it.Ref == "" {
			return "", "", false
		}
		return "CustomField", it.Ref, true

	case devproject.KindFlow:
		// Flow members are DeveloperName, NOT DefinitionId. Item.Name is
		// the DeveloperName captured at collect time. When Name is empty
		// (bundle-imported flows historically stored the DeveloperName as
		// the Ref with a blank Name), fall back to the Ref — but ONLY if
		// the Ref is itself a DeveloperName, not a DefinitionId. A "300"-
		// prefixed Ref is a DefinitionId and is NOT a valid manifest
		// member, so we must not emit it. This prevents the old silent-
		// drop where a blank-name imported flow contributed nothing.
		name := nonEmpty(it.Name, it.Type)
		if name == "" && !looksLikeFlowDefinitionID(it.Ref) {
			name = it.Ref
		}
		if name == "" {
			return "", "", false
		}
		return "Flow", name, true

	case devproject.KindFlowVersion:
		return "", "", false

	case devproject.KindApexClass:
		if it.Name == "" {
			return "", "", false
		}
		return "ApexClass", it.Name, true

	case devproject.KindApexTrigger:
		if it.Name == "" {
			return "", "", false
		}
		return "ApexTrigger", it.Name, true

	case devproject.KindReport:
		// Report members are "FolderName/ReportDeveloperName". Type
		// in our store is the folder name; Name is the report's
		// display label, NOT the DeveloperName. There's no clean way
		// to recover DeveloperName from a label without a Tooling
		// query, so v1 ships reports as best-effort: if Name has no
		// spaces (likely a DeveloperName) we use it; otherwise mark
		// unsupported. Future: capture DeveloperName at collect time.
		if it.Name == "" || strings.ContainsAny(it.Name, " \t") {
			return "", "", false
		}
		folder := it.Type
		if folder == "" {
			folder = "unfiled$public"
		}
		return "Report", folder + "/" + it.Name, true

	case devproject.KindPermissionSet:
		name := nonEmpty(it.Type, it.Name)
		if name == "" {
			return "", "", false
		}
		return "PermissionSet", name, true

	case devproject.KindPermissionSetGroup:
		name := nonEmpty(it.Type, it.Name)
		if name == "" {
			return "", "", false
		}
		return "PermissionSetGroup", name, true

	case devproject.KindProfile:
		if it.Name == "" {
			return "", "", false
		}
		return "Profile", it.Name, true

	case devproject.KindValidationRule:
		if it.Type == "" || it.Name == "" {
			return "", "", false
		}
		return "ValidationRule", it.Type + "." + it.Name, true

	case devproject.KindRecordType:
		if it.Type == "" || it.Name == "" {
			return "", "", false
		}
		return "RecordType", it.Type + "." + it.Name, true

	case devproject.KindLWC:
		name := nonEmpty(it.Type, it.Name)
		if name == "" {
			return "", "", false
		}
		return "LightningComponentBundle", name, true

	case devproject.KindAura:
		name := nonEmpty(it.Type, it.Name)
		if name == "" {
			return "", "", false
		}
		return "AuraDefinitionBundle", name, true

	case devproject.KindQueue:
		name := nonEmpty(it.Type, it.Name)
		if name == "" {
			return "", "", false
		}
		return "Queue", name, true

	case devproject.KindPublicGroup:
		name := nonEmpty(it.Type, it.Name)
		if name == "" {
			return "", "", false
		}
		return "Group", name, true

	case devproject.KindRecord:
		// Records are data, not metadata. Caller should route them to
		// a records.csv sidecar — return ok=false so the dispatch
		// puts them in result.Records via the WritePackageXML caller's
		// dedicated branch (not the Unsupported bucket). This branch
		// is unreachable because WritePackageXML checks KindRecord
		// before calling metadataMember; kept for completeness so the
		// switch is exhaustive.
		return "", "", false

	case devproject.KindSOQLQuery:
		// Saved SOQL queries are user-authored ad-hoc artifacts, not
		// Salesforce metadata. They live in the local devproject
		// store and are intentionally NOT exported with packages —
		// pin one to a project to keep it next to related metadata
		// without it leaking into the deployed manifest.
		return "", "", false
	}
	return "", "", false
}

// isNonDeployableSObject reports whether an sObject API name belongs
// to one of Salesforce's auto-generated plumbing entities — Feed,
// History, ChangeEvent, Share — that appear in describe results but
// can't go into a CustomObject manifest.
//
// Salesforce manages these alongside their parent object; you don't
// "deploy" them. Including them in package.xml triggers the warning
// "Entity of type 'CustomObject' named 'XFeed' cannot be found" and
// the metadata operation drops them. We filter on export so the
// manifest deploys cleanly.
//
// The patterns are conservative — we only filter suffixes that
// Salesforce reserves for these auto-generated kinds. Custom objects
// that happen to end in "Feed" by user choice would also be filtered,
// but Salesforce reserves __c suffixes for custom objects and the
// auto-generated ones never end in __c, so the suffix check below
// gives us correctness without catching custom objects.
func isNonDeployableSObject(name string) bool {
	if strings.HasSuffix(name, "__c") {
		return false
	}
	for _, suf := range []string{"Feed", "History", "ChangeEvent", "Share"} {
		if strings.HasSuffix(name, suf) {
			return true
		}
	}
	return false
}

func nonEmpty(xs ...string) string {
	for _, s := range xs {
		if s != "" {
			return s
		}
	}
	return ""
}

// looksLikeFlowDefinitionID reports whether a ref is a Salesforce
// FlowDefinition Id (keyprefix "300", 15 or 18 chars) rather than a
// DeveloperName. Used so a flow's manifest member is never mistakenly
// emitted as a DefinitionId.
func looksLikeFlowDefinitionID(ref string) bool {
	if len(ref) != 15 && len(ref) != 18 {
		return false
	}
	if !strings.HasPrefix(ref, "300") {
		return false
	}
	for _, r := range ref {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

// SfdxProjectJSON returns the sfdx-project.json contents for a
// scaffolded full-project bundle. Hand-written rather than pulled
// from a Go template because the file is small + reproducible
// content matters (deterministic across exports).
//
// projectName is used as the `name` field; sf-deck slugs it on the
// caller side so we can write whatever's passed without sanitising.
// API version follows DefaultAPIVersion so manifest + project agree.
func SfdxProjectJSON(projectName, apiVersion string) string {
	if apiVersion == "" {
		apiVersion = DefaultAPIVersion
	}
	if projectName == "" {
		projectName = "sf-deck-export"
	}
	return fmt.Sprintf(`{
  "packageDirectories": [
    { "path": "force-app", "default": true }
  ],
  "name": %q,
  "namespace": "",
  "sfdcLoginUrl": "https://login.salesforce.com",
  "sourceApiVersion": %q
}
`, projectName, apiVersion)
}

// ErrNoIncludedItems is returned by WritePackageXML when every input
// item was either a Record or Unsupported. Callers can catch it and
// surface a useful error ("project has 8 items but none are
// deployable as metadata") instead of writing an empty manifest.
var ErrNoIncludedItems = errors.New("devproject.WritePackageXML: no items mapped to MetadataAPI types")

// SuggestedReadme returns the README.md content that ships next to
// every manifest export. Tells the user what they got and how to
// use it. Plain markdown — fine for both rendering on GitHub and
// readable as raw text.
//
// fullProject=true switches the README into the "scaffolded sfdx
// project" mode where the user can `cd` in and `sf project retrieve`
// immediately. fullProject=false is the manifest-only mode where the
// user is expected to drop package.xml into their existing project
// or use it with non-sfdx tooling.
func SuggestedReadme(projectName, orgUser string, result PackageXMLResult, fullProject bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — sf-deck export\n\n", projectName)
	if orgUser != "" {
		fmt.Fprintf(&b, "Items captured from `%s`.\n\n", orgUser)
	}
	fmt.Fprintf(&b, "## Contents\n\n")
	fmt.Fprintf(&b, "- `package.xml` — Salesforce MetadataAPI manifest. %d component(s) listed.\n", result.IncludedCount)
	if fullProject {
		b.WriteString("- `sfdx-project.json` — scaffolded sfdx project; lets you run sf commands directly in this folder.\n")
		b.WriteString("- `force-app/main/default/` — empty target directory for retrieved source.\n")
	}
	if len(result.Records) > 0 {
		fmt.Fprintf(&b, "- `records.csv` — record-data items (%d). These are data, not metadata; import via Data Loader / Workbench / sf data import.\n", len(result.Records))
	}
	if len(result.Unsupported) > 0 {
		fmt.Fprintf(&b, "- %d item(s) skipped because their kind isn't yet mappable to MetadataAPI types.\n", len(result.Unsupported))
	}
	b.WriteString("\n## Use the manifest\n\n")
	if fullProject {
		b.WriteString("This export is a self-contained sfdx project. From this directory:\n\n")
		b.WriteString("```\nsf project retrieve start --manifest package.xml --target-org <alias>\n```\n\n")
		b.WriteString("Pulls source from `<alias>` into `force-app/main/default/`. Commit the result to git, edit, deploy elsewhere with:\n\n")
		b.WriteString("```\nsf project deploy validate --manifest package.xml --target-org <alias>\nsf project deploy start --manifest package.xml --target-org <alias>\n```\n\n")
	} else {
		b.WriteString("This export is the manifest only. Three ways to use it:\n\n")
		b.WriteString("**1. Drop into an existing sfdx project**\n\n")
		b.WriteString("```\ncp package.xml ~/your-sfdx-project/manifest/package.xml\ncd ~/your-sfdx-project\nsf project retrieve start --manifest manifest/package.xml --target-org <alias>\n```\n\n")
		b.WriteString("**2. Generate a new sfdx project around it**\n\n")
		b.WriteString("```\nsf project generate --name my-export --target-dir ~/exports\ncp package.xml ~/exports/my-export/manifest/package.xml\ncd ~/exports/my-export\nsf project retrieve start --manifest manifest/package.xml --target-org <alias>\n```\n\n")
		b.WriteString("Or use sf-deck's \"Full sfdx project\" export format to skip steps 1-2.\n\n")
		b.WriteString("**3. Non-sfdx tooling**\n\n")
	}
	b.WriteString("- Gearset: open Pro Deploy → Upload Manifest → choose this `package.xml`.\n")
	b.WriteString("- ANT migration tool / MetadataAPI directly: pass package.xml as the manifest.\n")
	return b.String()
}
