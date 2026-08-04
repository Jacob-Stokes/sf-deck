package ui

// Compare providers + route selection — the bridge between the
// type-agnostic diff/compare engine (internal/diff) and sf-deck's
// Salesforce layer (internal/sf).
//
// Two provider families:
//
//   - Tooling (sync, fast, MORE api calls): ApexClass / ApexTrigger.
//     Body is a direct Tooling column → instant, no async retrieve.
//
//   - Metadata API / SOAP snapshot path: broad metadata support via
//     listMetadata + readMetadata lanes. The legacy generic provider below
//     remains for Tooling drill-in fallbacks, but normal Auto/Metadata runs
//     execute through the comparePlan in tab_compare.go.
//
// The user's chosen route (Auto / Tooling / Metadata API) selects which
// family serves each type:
//
//   - Auto         : Tooling for the fast types, Metadata API for the rest.
//   - Tooling      : only the Tooling-capable types (others omitted).
//   - Metadata API : every type via the generic Metadata API provider.

import (
	"fmt"
	"sync"

	"github.com/Jacob-Stokes/sf-deck/internal/diff"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// toolingCompareTypes are the metadata types we can serve via the fast
// synchronous Tooling path (Body is a direct column). Everything else
// goes through the generic Metadata API provider.
var toolingCompareTypes = map[string]func() diff.Provider{
	"ApexClass":   func() diff.Provider { return apexClassProvider{} },
	"ApexTrigger": func() diff.Provider { return apexTriggerProvider{} },
}

// mdapiCompareTypes are the metadata types offered via the Metadata API
// route (the broad "all metadata" set). Ordered for the scope UI. This
// is a curated common subset — extend freely; each is served by the one
// generic provider, so adding a type is a single line here.
var mdapiCompareTypes = []string{
	"ApexClass", "ApexTrigger", "ApexPage", "ApexComponent",
	"CustomField", "ValidationRule", "RecordType", "Flow",
	"Layout", "PermissionSet", "Profile", "CustomObject",
	"WorkflowRule", "CustomLabels", "StaticResource", "EmailTemplate",
	"FlexiPage", "QuickAction", "CustomMetadata", "CustomApplication",
}

// allCompareTypes is the static fallback set (used when org describe is
// unavailable). The live picker prefers the org-discovered list — see
// Model.loadComparableTypes.
func allCompareTypes() []string {
	return append([]string(nil), mdapiCompareTypes...)
}

// unsupportedCompareTypes are types our retrieve lanes can't yet handle
// and so are excluded from the scope picker even if the org supports
// them: folder-based (need folder traversal) and bundle-based (need
// bundle assembly). Tracked for a future follow-up.
var unsupportedCompareTypes = map[string]bool{
	// folder-based
	"Report": true, "Dashboard": true, "Document": true, "EmailTemplate": true,
	// bundle-based
	"LightningComponentBundle": true, "AuraDefinitionBundle": true,
	"ExperienceBundle": true, "DigitalExperienceBundle": true,
	"WaveTemplateBundle": true, "LightningTypeBundle": true,
	// object-CHILD types are retrieved via their parent CustomObject, not
	// as standalone scope entries (the picker offers CustomObject; its
	// children come down with it).
}

// compareTypesCacheKey is the kv key (per org) for the discovered +
// classified comparable-type list.
const compareTypesCacheKey = "metadata_types_v1"

// loadComparableTypes returns the comparable metadata types for an org.
// Refresh-once-per-session: the FIRST scope-open in a session re-fetches
// via describeMetadata (types change rarely; relaunch is the refresh)
// and writes the kv cache; subsequent opens this session read the cache
// (instant). Cross-session, the first open re-fetches again.
//
// Runs OFF the UI loop (called from a tea.Cmd) — it shells `sf` and hits
// the cache, so it must not block rendering.
func (m *Model) loadComparableTypes(alias string) ([]string, error) {
	refreshedThisSession := m.compareTypesRefreshed[alias]

	// Cached read (only when we've already refreshed this session).
	if refreshedThisSession && m.cache != nil {
		var cached []string
		if _, ok, _ := m.cache.GetJSON(alias, compareTypesCacheKey, &cached); ok && len(cached) > 0 {
			return cached, nil
		}
	}

	infos, err := sf.DescribeMetadataTypes(alias)
	if err != nil || len(infos) == 0 {
		// On failure, serve last-cached if any, else the static fallback.
		if m.cache != nil {
			var cached []string
			if _, ok, _ := m.cache.GetJSON(alias, compareTypesCacheKey, &cached); ok && len(cached) > 0 {
				return cached, nil
			}
		}
		return allCompareTypes(), err
	}
	types := classifyComparableTypes(infos)
	if m.cache != nil {
		_ = m.cache.PutJSON(alias, compareTypesCacheKey, types)
	}
	if m.compareTypesRefreshed == nil {
		m.compareTypesRefreshed = map[string]bool{}
	}
	m.compareTypesRefreshed[alias] = true
	return types, nil
}

// classifyComparableTypes filters a describeMetadata result to the types
// the scope picker offers: drops folder-based + bundle-based (no
// retrieve lane yet) and object-CHILD types (they ride CustomObject).
func classifyComparableTypes(infos []sf.MetadataTypeInfo) []string {
	child := map[string]bool{}
	for _, t := range infos {
		for _, c := range t.ChildXMLNames {
			child[c] = true
		}
	}
	var out []string
	for _, t := range infos {
		name := t.XMLName
		if t.InFolder || unsupportedCompareTypes[name] || child[name] {
			continue
		}
		out = append(out, name)
	}
	return out
}

// compareProviders returns the default (Auto) provider set — used by
// callers that don't specify a method (e.g. the scope-default UI).
func compareProviders() []diff.Provider {
	return providersForMethod(compareMethodAuto)
}

// providersForMethod builds the provider list for a retrieval route.
func providersForMethod(method compareMethod) []diff.Provider {
	switch method {
	case compareMethodTooling:
		// Only the fast Tooling-capable types.
		var out []diff.Provider
		for _, label := range toolingTypeOrder() {
			out = append(out, toolingCompareTypes[label]())
		}
		return out
	case compareMethodMetadataAPI:
		// Everything, via the generic Metadata API provider.
		var out []diff.Provider
		for _, label := range mdapiCompareTypes {
			out = append(out, newMDAPIProvider(label))
		}
		return out
	default: // Auto: Tooling where possible, Metadata API for the rest.
		var out []diff.Provider
		seen := map[string]bool{}
		for _, label := range toolingTypeOrder() {
			out = append(out, toolingCompareTypes[label]())
			seen[label] = true
		}
		for _, label := range mdapiCompareTypes {
			if seen[label] {
				continue
			}
			out = append(out, newMDAPIProvider(label))
		}
		return out
	}
}

// toolingTypeOrder returns the Tooling fast types in a stable order.
func toolingTypeOrder() []string {
	return []string{"ApexClass", "ApexTrigger"}
}

// providerByLabel resolves a provider for a type label, preferring the
// fast Tooling path (used on drill-in body fetch, where speed matters
// and Tooling-served types should use their cheap body column).
func providerByLabel(label string) (diff.Provider, bool) {
	if ctor, ok := toolingCompareTypes[label]; ok {
		return ctor(), true
	}
	for _, t := range mdapiCompareTypes {
		if t == label {
			return newMDAPIProvider(label), true
		}
	}
	return nil, false
}

// --- ApexClass (Tooling) --------------------------------------------------

type apexClassProvider struct{}

func (apexClassProvider) TypeLabel() string { return "ApexClass" }

func (apexClassProvider) List(alias string) ([]diff.Component, error) {
	rows, err := sf.ListApexClasses(alias)
	if err != nil {
		return nil, err
	}
	out := make([]diff.Component, 0, len(rows))
	for _, r := range rows {
		if r.NamespacePrefix != "" {
			continue
		}
		out = append(out, diff.Component{
			Type: "ApexClass", Key: r.Name, ID: r.ID,
			Summary: fmt.Sprintf("v%.0f · %s", r.ApiVersion, dashIfEmpty(r.Status)),
		})
	}
	return out, nil
}

func (apexClassProvider) Body(alias, id string) (string, error) {
	d, err := sf.GetApexClass(alias, id)
	if err != nil {
		return "", err
	}
	return d.Body, nil
}

// --- ApexTrigger (Tooling) ------------------------------------------------

type apexTriggerProvider struct{}

func (apexTriggerProvider) TypeLabel() string { return "ApexTrigger" }

func (apexTriggerProvider) List(alias string) ([]diff.Component, error) {
	rows, err := sf.ListAllTriggers(alias)
	if err != nil {
		return nil, err
	}
	out := make([]diff.Component, 0, len(rows))
	for _, r := range rows {
		if r.NamespacePrefix != "" {
			continue
		}
		summary := fmt.Sprintf("v%.0f · %s", r.ApiVer, dashIfEmpty(r.Status))
		if r.Table != "" {
			summary = r.Table + " · " + summary
		}
		out = append(out, diff.Component{
			Type: "ApexTrigger", Key: r.Name, ID: r.ID, Summary: summary,
		})
	}
	return out, nil
}

func (apexTriggerProvider) Body(alias, id string) (string, error) {
	d, err := sf.GetTrigger(alias, id)
	if err != nil {
		return "", err
	}
	return d.Body, nil
}

// --- generic Metadata API provider ----------------------------------------

// mdapiProvider serves ANY metadata type via the Metadata API retrieve
// path. List enumerates via `sf org list metadata`; Body retrieves the
// component's source XML, caching the per-(alias,type) retrieve so a
// drill-in into several components of the same type only retrieves once.
type mdapiProvider struct {
	typeLabel string

	mu    sync.Mutex
	cache map[string]map[string]string // alias -> (componentKey -> xml)
}

func newMDAPIProvider(typeLabel string) *mdapiProvider {
	return &mdapiProvider{typeLabel: typeLabel, cache: map[string]map[string]string{}}
}

func (p *mdapiProvider) TypeLabel() string { return p.typeLabel }

func (p *mdapiProvider) List(alias string) ([]diff.Component, error) {
	items, err := sf.MetadataListByType(alias, p.typeLabel)
	if err != nil {
		return nil, err
	}
	out := make([]diff.Component, 0, len(items))
	for _, it := range items {
		if it.NamespacePrefix != "" {
			continue
		}
		out = append(out, diff.Component{
			Type: p.typeLabel,
			Key:  it.FullName,
			// ID = fullName so Body() can retrieve by member name (MDAPI
			// has no per-component record id like Tooling).
			ID:      it.FullName,
			Summary: mdapiSummary(it),
		})
	}
	return out, nil
}

func (p *mdapiProvider) Body(alias, id string) (string, error) {
	// id is the component fullName. Retrieve the whole type once per
	// alias (cached), then return the matching file's XML.
	p.mu.Lock()
	byKey, ok := p.cache[alias]
	p.mu.Unlock()
	if !ok {
		retrieved, err := sf.RetrieveMetadataXML(alias, p.typeLabel, nil)
		if err != nil {
			return "", err
		}
		p.mu.Lock()
		p.cache[alias] = retrieved
		byKey = retrieved
		p.mu.Unlock()
	}
	// Component keys from the retrieve may be the base name; the id may
	// be "Object.Field" form. Try exact, then the trailing segment.
	if xml, ok := byKey[id]; ok {
		return xml, nil
	}
	if seg := lastDotSegment(id); seg != id {
		if xml, ok := byKey[seg]; ok {
			return xml, nil
		}
	}
	return "", fmt.Errorf("%s %q not found in retrieved source", p.typeLabel, id)
}

func mdapiSummary(it sf.MetadataItem) string {
	if it.LastModifiedDate != "" {
		return "modified " + it.LastModifiedDate
	}
	return it.Type
}

func lastDotSegment(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return s[i+1:]
		}
	}
	return s
}
