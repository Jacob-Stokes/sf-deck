package sf

// Object-level Tooling metadata helpers — the CustomObject sibling
// of fields.go's CustomField helpers. Structurally identical: an Id
// lookup with the naming-quirk fallback chain, plus a thin wrapper
// over UpdateToolingMetadata for readability at call sites.

import (
	"encoding/json"
	"fmt"
	"sync"
)

// CustomObjectID resolves the Tooling CustomObject.Id for an sObject.
// Returns an error for non-custom objects (Salesforce has no
// CustomObject row for Account, Contact, etc. — you'd edit those via
// EntityDefinition instead, which has a different shape).
//
// Naming handling matches the CustomField equivalent: strips
// namespace prefix + __c suffix to get the bare DeveloperName. Keeps
// a fallback to the full API name for weird managed-package cases.
//
// Process-wide cache: the Id is immutable for the life of the object,
// so once we've resolved (target, sobject) keep the answer forever.
// Observed firing 3× during a single "edit object label" gesture
// (preview-prepare, deploy-prepare, post-deploy refresh) when each
// path called this independently — the cache makes the 2nd and 3rd
// calls free.
func CustomObjectID(target, sobject string) (string, error) {
	key := target + "|" + sobject
	customObjectIDCacheMu.RLock()
	cached, ok := customObjectIDCache[key]
	customObjectIDCacheMu.RUnlock()
	if ok && cached != "" {
		return cached, nil
	}

	c, err := RESTClient(target)
	if err != nil {
		return "", err
	}
	for _, dev := range customObjectLookupCandidates(sobject) {
		soql := fmt.Sprintf(
			"SELECT Id FROM CustomObject WHERE DeveloperName = '%s'",
			sqlEscape(dev))
		q, err := c.QueryREST(soql, true)
		if err != nil {
			return "", fmt.Errorf("lookup CustomObject id: %w", err)
		}
		if len(q.Records) > 0 {
			if id := asString(q.Records[0]["Id"]); id != "" {
				customObjectIDCacheMu.Lock()
				customObjectIDCache[key] = id
				customObjectIDCacheMu.Unlock()
				return id, nil
			}
		}
	}
	return "", fmt.Errorf(
		"no CustomObject row for %s — standard objects can't be edited via CustomObject",
		sobject)
}

// Process-wide CustomObject.Id cache. RWMutex because reads dominate
// writes (every lookup hits read; only one write per (target, sobject)
// per process lifetime). Cleared by InvalidateRESTClients so tests +
// alias-rebuild paths get a clean slate.
var (
	customObjectIDCacheMu sync.RWMutex
	customObjectIDCache   = map[string]string{}
)

// invalidateCustomObjectIDCache resets the process-wide cache.
// Called from InvalidateRESTClients so tests start clean and alias
// teardown doesn't leak Ids across orgs that may have been re-auth'd.
func invalidateCustomObjectIDCache() {
	customObjectIDCacheMu.Lock()
	customObjectIDCache = map[string]string{}
	customObjectIDCacheMu.Unlock()
}

// customObjectLookupCandidates returns DeveloperName variants to try
// in order. Bare (namespace + __c stripped) first, then full API name
// as a fallback.
func customObjectLookupCandidates(sobject string) []string {
	out := []string{developerNameFromAPIName(sobject)}
	if developerNameFromAPIName(sobject) != sobject {
		out = append(out, sobject)
	}
	return out
}

// GetCustomObjectDescription reads the Description column from the
// Tooling CustomObject sobject. This works for sync pre-population
// in the edit-description modal even though the Tooling CustomObject
// is read-only via PATCH — SELECTs on it are fine for most columns.
// Returns "" when the column is null/empty. Errors bubble up.
func GetCustomObjectDescription(target, customObjectID string) (string, error) {
	c, err := RESTClient(target)
	if err != nil {
		return "", err
	}
	path := c.ToolingPath("sobjects/CustomObject/" + customObjectID)
	raw, err := c.get(path, nil)
	if err != nil {
		return "", upgradeToSFError(err)
	}
	var row struct {
		Description *string `json:"Description"`
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		return "", fmt.Errorf("decode CustomObject: %w", err)
	}
	if row.Description == nil {
		return "", nil
	}
	return *row.Description, nil
}
