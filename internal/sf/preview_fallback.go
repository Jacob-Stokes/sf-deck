package sf

// Manifest-driven preview fallback for non-source-tracked orgs.

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
// orgAlias is required for the Tooling API request.
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
	sortPreviewItems(toRetrieve)
	sortPreviewItems(toDeploy)
	sortPreviewItems(ignored)
	return ManifestPreview{
		ToRetrieve: toRetrieve,
		ToDeploy:   toDeploy,
		Ignored:    ignored,
	}, nil
}

type packageManifest struct {
	Types []manifestType `xml:"types"`
}

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
		return nil, nil, nil, nil
	}

	nameCol := tooling.NameColumn
	quoted := make([]string, 0, len(t.Members))
	memberIndex := map[string]string{} // lookup-name → original member
	for _, mem := range t.Members {
		key := tooling.NameKeyFor(mem)
		quoted = append(quoted, "'"+sqlEscape(key)+"'")
		memberIndex[strings.ToLower(key)] = mem
	}
	soql := fmt.Sprintf(
		"SELECT %s, LastModifiedDate, NamespacePrefix FROM %s WHERE %s IN (%s)",
		nameCol, tooling.Object, nameCol, strings.Join(quoted, ","),
	)
	res, err := c.QueryREST(soql, true)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%s query: %w", t.Name, err)
	}

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
			toRetrieve = append(toRetrieve, ManifestPreviewItem{
				FullName: mem,
				Type:     t.Name,
				Path:     "",
			})
			continue
		}
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
	}
	return toRetrieve, toDeploy, ignored, nil
}

type toolingTypeMap struct {
	Object     string                             // tooling sObject (e.g. "Flow", "ApexClass")
	NameColumn string                             // column to filter on
	NameKeyFor func(manifestMember string) string // transform manifest member → query key
}

func toolingTableFor(metadataType string) (toolingTypeMap, bool) {
	identity := func(m string) string { return m }
	switch metadataType {
	case "CustomObject":
		return toolingTypeMap{Object: "CustomObject", NameColumn: "DeveloperName",
			NameKeyFor: func(m string) string {
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

func findLocalFile(bundleDir, metadataType, fullName string) (string, time.Time, bool) {
	root := filepath.Join(bundleDir, "force-app")
	if _, err := os.Stat(root); err != nil {
		return "", time.Time{}, false
	}
	suffix := fileSuffixForType(metadataType)
	if suffix == "" {
		return "", time.Time{}, false
	}
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

func sortPreviewItems(items []ManifestPreviewItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Type != items[j].Type {
			return items[i].Type < items[j].Type
		}
		return items[i].FullName < items[j].FullName
	})
}

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
