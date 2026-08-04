package sf

// Manifest-driven preview fallback for non-source-tracked orgs.
//
// `sf project retrieve preview` only works on orgs with source
// tracking enabled — scratch orgs, Developer/Developer Pro
// sandboxes (after explicit opt-in). Production, Partial Copy and
// Full sandboxes can never enable it. For those orgs we compute a
// best-effort diff ourselves: read the bundle's package.xml, query
// the Tooling API for each component's LastModifiedDate, compare
// against the local files' mtimes.
//
// Limitations vs the real preview:
//   - No conflict detection (the real preview tracks who-changed-what
//     server-side; we can only see "are mtimes different")
//   - Can't detect deletions in the org (a component that existed
//     when retrieved but was deleted later shows as "still in your
//     bundle"; the real preview would flag it for delete)
//   - Some metadata types don't expose LastModifiedDate via Tooling
//     (most do, but exceptions are silently treated as unchanged)
//
// In exchange: works on every org regardless of tracking state, and
// makes ~one Tooling query per metadata type rather than per file.

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ManifestPreviewFallback is the alternative preview generator used
// when the target org doesn't support source tracking. Returns the
// same ManifestPreview shape as RetrievePreview / DeployPreview so
// the rendering code stays uniform — callers just notice that the
// `Conflicts` slice will always be empty.
//
// orgAlias is required (we hit the Tooling API for it).
// lastRetrievedAt is the timestamp of the bundle's most recent
// retrieve — used as the "floor" for both sides of the comparison:
//
//   - ToRetrieve: org.LastModifiedDate > lastRetrievedAt
//     (the org has changed since you last pulled)
//   - ToDeploy:   local mtime > lastRetrievedAt
//     (you've changed the file since you last pulled)
//
// Comparing local mtime against org.LastModifiedDate directly
// produces noise: sf writes every file with a current mtime on
// retrieve, so immediately after retrieve every file appears
// "newer than the org". The floor avoids that — a freshly retrieved
// file has mtime ≈ lastRetrievedAt, so no false positives.
//
// Pass time.Time{} (zero) when the bundle hasn't been retrieved yet
// (first run, or freshly created); the floor is then "any change
// counts" which matches the intuitive first-time UX.
//
// Other slices:
//   - ToDelete:  empty (we can't detect org-side deletions)
//   - Conflicts: empty (we can't detect "both sides edited")
//   - Ignored:   components in package.xml that the org doesn't
//     have at all (mismatch between manifest and org state)
func ManifestPreviewFallback(bundleDir, orgAlias string, lastRetrievedAt time.Time) (ManifestPreview, error) {
	pkgPath := filepath.Join(bundleDir, "package.xml")
	manifest, err := parsePackageXML(pkgPath)
	if err != nil {
		return ManifestPreview{}, fmt.Errorf("read package.xml: %w", err)
	}
	if len(manifest.Types) == 0 {
		return ManifestPreview{}, nil
	}
	c, err := RESTClient(orgAlias)
	if err != nil {
		return ManifestPreview{}, err
	}

	// One goroutine per metadata type so big bundles stay snappy.
	// Max ~20 types in a typical bundle, so we don't bother with a
	// pool — just unbounded goroutines + WaitGroup.
	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		toRetrieve []ManifestPreviewItem
		toDeploy   []ManifestPreviewItem
		ignored    []ManifestPreviewItem
		firstErr   error
	)
	for _, t := range manifest.Types {
		t := t
		wg.Add(1)
		go func() {
			defer wg.Done()
			ret, dep, ign, err := compareTypeAgainstOrg(c, bundleDir, t, lastRetrievedAt)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			toRetrieve = append(toRetrieve, ret...)
			toDeploy = append(toDeploy, dep...)
			ignored = append(ignored, ign...)
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return ManifestPreview{}, firstErr
	}
	// Stable ordering: type then fullName so the rendered table is
	// deterministic between refreshes.
	sortPreviewItems(toRetrieve)
	sortPreviewItems(toDeploy)
	sortPreviewItems(ignored)
	return ManifestPreview{
		ToRetrieve: toRetrieve,
		ToDeploy:   toDeploy,
		Ignored:    ignored,
	}, nil
}

// packageManifest is the parsed shape of a sfdx package.xml.
type packageManifest struct {
	Types []manifestType `xml:"types"`
}

// manifestType is one <types> block: a metadata type + the list of
// component fullNames in scope.
type manifestType struct {
	Name    string   `xml:"name"`
	Members []string `xml:"members"`
}

func parsePackageXML(path string) (packageManifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return packageManifest{}, err
	}
	var m packageManifest
	if err := xml.Unmarshal(body, &m); err != nil {
		return packageManifest{}, err
	}
	return m, nil
}

// compareTypeAgainstOrg queries the org for the LastModifiedDate of
// every component of the given metadata type, then walks the local
// force-app/ tree to find the matching files' mtimes. Returns three
// slices: (changed-in-org, changed-locally, in-manifest-but-not-in-org).
//
// floor is the bundle's last_retrieved_at timestamp. Both sides of
// the comparison are anchored against it:
//   - "changed in org" = orgTime > floor
//   - "changed locally" = localTime > floor
//
// When floor is zero (first run / never retrieved) we fall back to
// the symmetric comparison (org vs local) which is correct for that
// case.
func compareTypeAgainstOrg(c *Client, bundleDir string, t manifestType, floor time.Time) (
	toRetrieve, toDeploy, ignored []ManifestPreviewItem, err error,
) {
	if len(t.Members) == 0 {
		return nil, nil, nil, nil
	}
	tooling, supported := toolingTableFor(t.Name)
	if !supported {
		// Not all metadata types have a Tooling-API counterpart.
		// Treat as "unknown state" by reporting them as unchanged
		// — a future iteration could fall back to MetadataAPI's
		// listMetadata for these.
		return nil, nil, nil, nil
	}

	// Build the SOQL: fetch every member's LastModifiedDate.
	// "WHERE DeveloperName IN (...)" works for most types; CustomObject
	// uses Name. The toolingTableFor mapping declares which.
	nameCol := tooling.NameColumn
	quoted := make([]string, 0, len(t.Members))
	memberIndex := map[string]string{} // lookup-name → original member
	for _, mem := range t.Members {
		key := tooling.NameKeyFor(mem)
		quoted = append(quoted, "'"+sqlEscape(key)+"'")
		memberIndex[strings.ToLower(key)] = mem
	}
	// NamespacePrefix exists on every Tooling API type that supports
	// managed packages. Querying it always (vs conditionally) is
	// cheaper than maintaining a per-type allow-list and a missing
	// column at query time errors loudly anyway.
	soql := fmt.Sprintf(
		"SELECT %s, LastModifiedDate, NamespacePrefix FROM %s WHERE %s IN (%s)",
		nameCol, tooling.Object, nameCol, strings.Join(quoted, ","),
	)
	res, err := c.QueryREST(soql, true)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%s query: %w", t.Name, err)
	}

	// Map org-side records by their lookup name so we can pair them
	// with manifest entries. Captures both LastModifiedDate (for the
	// diff comparison) + NamespacePrefix (for managed-package
	// detection).
	type orgRecord struct {
		modified  time.Time
		namespace string
	}
	orgByName := map[string]orgRecord{}
	for _, row := range res.Records {
		key := strings.ToLower(asString(row[nameCol]))
		orgByName[key] = orgRecord{
			modified:  parseLastModified(asString(row["LastModifiedDate"])),
			namespace: asString(row["NamespacePrefix"]),
		}
	}

	for _, mem := range t.Members {
		key := strings.ToLower(tooling.NameKeyFor(mem))
		rec, inOrg := orgByName[key]
		if !inOrg {
			ignored = append(ignored, ManifestPreviewItem{
				FullName: mem,
				Type:     t.Name,
			})
			continue
		}
		// Managed-package items get bucketed into Ignored with the
		// namespace tagged. The renderer can pull them out and show
		// in a dedicated "Managed (not retrievable)" section so users
		// understand why the file isn't on disk.
		if rec.namespace != "" {
			ignored = append(ignored, ManifestPreviewItem{
				FullName:  mem,
				Type:      t.Name,
				Namespace: rec.namespace,
			})
			continue
		}
		orgTime := rec.modified
		localPath, localTime, found := findLocalFile(bundleDir, t.Name, mem)
		if !found {
			// In the manifest + org but no local file. Common right
			// after a fresh export but before retrieve runs; treat as
			// "should retrieve".
			toRetrieve = append(toRetrieve, ManifestPreviewItem{
				FullName: mem,
				Type:     t.Name,
				Path:     "",
			})
			continue
		}
		// Round both to the second so sub-millisecond serialization
		// quirks don't cause spurious "newer" reports.
		orgRounded := orgTime.Truncate(time.Second)
		localRounded := localTime.Truncate(time.Second)

		if floor.IsZero() {
			// No floor (bundle never retrieved): fall back to
			// straight comparison. Older first-run path; modern code
			// always passes a floor after a successful retrieve.
			if orgRounded.After(localRounded) {
				toRetrieve = append(toRetrieve, ManifestPreviewItem{
					FullName: mem,
					Type:     t.Name,
					Path:     localPath,
				})
			} else if localRounded.After(orgRounded) {
				toDeploy = append(toDeploy, ManifestPreviewItem{
					FullName: mem,
					Type:     t.Name,
					Path:     localPath,
				})
			}
			continue
		}

		// With floor: both sides must show change since the last
		// retrieve. Otherwise immediately-after-retrieve files (mtime
		// ≈ now) are spuriously flagged as "deploy" because they're
		// always newer than the org's older LastModifiedDate.
		floorRounded := floor.Truncate(time.Second)
		orgChanged := orgRounded.After(floorRounded)
		// Allow a 5s grace on the local side: sf can write files at
		// slightly different mtimes during a single retrieve (some
		// files written first vs last), and we don't want the
		// last-touched files to look modified just because they
		// finished later than the floor was set. 5s is well below any
		// human-edit cadence.
		localChanged := localRounded.After(floorRounded.Add(5 * time.Second))

		if orgChanged {
			toRetrieve = append(toRetrieve, ManifestPreviewItem{
				FullName: mem,
				Type:     t.Name,
				Path:     localPath,
			})
		}
		if localChanged {
			toDeploy = append(toDeploy, ManifestPreviewItem{
				FullName: mem,
				Type:     t.Name,
				Path:     localPath,
			})
		}
		// Both unchanged → omitted from all slices (clean state).
		// Both changed → appears in BOTH retrieve and deploy lists,
		// which is the closest we can get to "conflict" without
		// source-tracking. Renderer can flag dupes in a future pass.
	}
	return toRetrieve, toDeploy, ignored, nil
}

// toolingTypeMap is the per-metadata-type lookup info for the Tooling
// query. Different types live under different sObjects + index by
// different name columns.
type toolingTypeMap struct {
	Object     string                             // tooling sObject (e.g. "Flow", "ApexClass")
	NameColumn string                             // column to filter on
	NameKeyFor func(manifestMember string) string // transform manifest member → query key
}

// toolingTableFor returns the Tooling-API mapping for a metadata
// type, plus a bool indicating whether the fallback is supported
// for that type. Supported list grows as metadata types are added
// to the FormatSfdxProjectRetrieve workflow.
//
// Many SF metadata types use different naming conventions in the
// MetadataAPI (the manifest) vs the Tooling API (the query):
//   - CustomObject: manifest "Account" → Tooling Name "Account"
//   - CustomField: manifest "Account.Industry" → Tooling DeveloperName "Industry" + TableEnumOrId
//     (custom fields are awkward; we treat them as unsupported for now)
//   - Flow: manifest "MyFlow" → Tooling DeveloperName "MyFlow"
//   - ApexClass: manifest "MyClass" → Tooling Name "MyClass"
//
// Identity transform is the default. Add explicit cases when a type
// needs a different Tooling shape.
func toolingTableFor(metadataType string) (toolingTypeMap, bool) {
	identity := func(m string) string { return m }
	switch metadataType {
	case "CustomObject":
		return toolingTypeMap{Object: "CustomObject", NameColumn: "DeveloperName",
			NameKeyFor: func(m string) string {
				// CustomObject's DeveloperName is the API name without
				// the trailing "__c" custom-object suffix.
				return strings.TrimSuffix(m, "__c")
			}}, true
	case "Flow":
		return toolingTypeMap{Object: "FlowDefinition", NameColumn: "DeveloperName", NameKeyFor: identity}, true
	case "ApexClass":
		return toolingTypeMap{Object: "ApexClass", NameColumn: "Name", NameKeyFor: identity}, true
	case "ApexTrigger":
		return toolingTypeMap{Object: "ApexTrigger", NameColumn: "Name", NameKeyFor: identity}, true
	case "ApexComponent":
		return toolingTypeMap{Object: "ApexComponent", NameColumn: "Name", NameKeyFor: identity}, true
	case "ApexPage":
		return toolingTypeMap{Object: "ApexPage", NameColumn: "Name", NameKeyFor: identity}, true
	case "PermissionSet":
		return toolingTypeMap{Object: "PermissionSet", NameColumn: "Name", NameKeyFor: identity}, true
	case "PermissionSetGroup":
		return toolingTypeMap{Object: "PermissionSetGroup", NameColumn: "DeveloperName", NameKeyFor: identity}, true
	case "ValidationRule":
		// "Account.MyRule" → DeveloperName "MyRule". sObject ID is
		// Tooling's EntityDefinitionId. We can't filter by that
		// without a join; for now compare by DeveloperName only and
		// accept a small false-positive risk on rules with the same
		// name across objects.
		return toolingTypeMap{Object: "ValidationRule", NameColumn: "ValidationName",
			NameKeyFor: func(m string) string {
				if i := strings.IndexByte(m, '.'); i >= 0 {
					return m[i+1:]
				}
				return m
			}}, true
	case "LightningComponentBundle":
		return toolingTypeMap{Object: "LightningComponentBundle", NameColumn: "DeveloperName", NameKeyFor: identity}, true
	case "AuraDefinitionBundle":
		return toolingTypeMap{Object: "AuraDefinitionBundle", NameColumn: "DeveloperName", NameKeyFor: identity}, true
	case "Layout":
		return toolingTypeMap{Object: "Layout", NameColumn: "Name", NameKeyFor: identity}, true
	case "RecordType":
		return toolingTypeMap{Object: "RecordType", NameColumn: "DeveloperName",
			NameKeyFor: func(m string) string {
				if i := strings.IndexByte(m, '.'); i >= 0 {
					return m[i+1:]
				}
				return m
			}}, true
	case "Profile":
		return toolingTypeMap{Object: "Profile", NameColumn: "Name", NameKeyFor: identity}, true
	}
	return toolingTypeMap{}, false
}

// findLocalFile walks force-app/ looking for the file that matches a
// (type, fullName) pair from the manifest. Returns the path, mtime,
// and a found bool. Naive — we walk everything and match by suffix
// — but the bundles are typically small enough that it's fine.
//
// File-name conventions per type:
//   - Flow                 → flows/<name>.flow-meta.xml
//   - ApexClass            → classes/<name>.cls
//   - ApexTrigger          → triggers/<name>.trigger
//   - CustomObject         → objects/<name>/<name>.object-meta.xml
//   - LightningComponentBundle → lwc/<name>/ (use directory mtime)
//   - PermissionSet        → permissionsets/<name>.permissionset-meta.xml
//
// Falls back to "first file matching the fullName + a known suffix"
// when an exact rule isn't declared.
func findLocalFile(bundleDir, metadataType, fullName string) (string, time.Time, bool) {
	root := filepath.Join(bundleDir, "force-app")
	if _, err := os.Stat(root); err != nil {
		return "", time.Time{}, false
	}
	suffix := fileSuffixForType(metadataType)
	if suffix == "" {
		return "", time.Time{}, false
	}
	// Most types: look for <fullName><suffix> anywhere under force-app.
	target := fullName + suffix
	var hit string
	var hitTime time.Time
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) == target {
			hit = path
			hitTime = info.ModTime()
			return filepath.SkipAll
		}
		return nil
	})
	if hit == "" {
		return "", time.Time{}, false
	}
	return hit, hitTime, true
}

// fileSuffixForType maps a metadata type to its on-disk filename
// suffix (the bit after fullName). "" means we don't know the
// convention → fall through to "unsupported".
func fileSuffixForType(metadataType string) string {
	switch metadataType {
	case "Flow":
		return ".flow-meta.xml"
	case "ApexClass":
		return ".cls"
	case "ApexTrigger":
		return ".trigger"
	case "ApexComponent":
		return ".component"
	case "ApexPage":
		return ".page"
	case "CustomObject":
		return ".object-meta.xml"
	case "PermissionSet":
		return ".permissionset-meta.xml"
	case "PermissionSetGroup":
		return ".permissionsetgroup-meta.xml"
	case "Profile":
		return ".profile-meta.xml"
	case "Layout":
		return ".layout-meta.xml"
	case "RecordType":
		return ".recordType-meta.xml"
	case "ValidationRule":
		return ".validationRule-meta.xml"
	}
	return ""
}

// sortPreviewItems sorts a slice of preview items by Type then
// FullName for deterministic rendering.
func sortPreviewItems(items []ManifestPreviewItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Type != items[j].Type {
			return items[i].Type < items[j].Type
		}
		return items[i].FullName < items[j].FullName
	})
}

// parseLastModified parses the SF datetime string. SF returns
// timestamps in ISO-8601 UTC ("2026-04-30T12:34:56.000+0000"), but
// the +0000 / Z variants both occur. time.Parse handles both with
// the right layout fallback.
func parseLastModified(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05.000Z",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
