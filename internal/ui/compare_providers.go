package ui

// Compare providers + route selection — the bridge between the
// type-agnostic diff/compare engine (internal/diff) and sf-deck's
// Salesforce layer (internal/sf).

import (
	"fmt"
	"sync"

	"github.com/Jacob-Stokes/sf-deck/internal/diff"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

var toolingCompareTypes = map[string]func() diff.Provider{
	"ApexClass":   func() diff.Provider { return apexClassProvider{} },
	"ApexTrigger": func() diff.Provider { return apexTriggerProvider{} },
}

var mdapiCompareTypes = []string{
	"ApexClass", "ApexTrigger", "ApexPage", "ApexComponent",
	"CustomField", "ValidationRule", "RecordType", "Flow",
	"Layout", "PermissionSet", "Profile", "CustomObject",
	"WorkflowRule", "CustomLabels", "StaticResource", "EmailTemplate",
	"FlexiPage", "QuickAction", "CustomMetadata", "CustomApplication",
}

func allCompareTypes() []string {
	return append([]string(nil), mdapiCompareTypes...)
}

// unsupportedCompareTypes are types our retrieve lanes can't yet handle
// and so are excluded from the scope picker even if the org supports
// them: folder-based (need folder traversal) and bundle-based (need
// bundle assembly). Tracked for a future follow-up.
var unsupportedCompareTypes = map[string]bool{
	"Report": true, "Dashboard": true, "Document": true, "EmailTemplate": true,
	"LightningComponentBundle": true, "AuraDefinitionBundle": true,
	"ExperienceBundle": true, "DigitalExperienceBundle": true,
	"WaveTemplateBundle": true, "LightningTypeBundle": true,
}

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

	if refreshedThisSession && m.cache != nil {
		var cached []string
		if _, ok, _ := m.cache.GetJSON(alias, compareTypesCacheKey, &cached); ok && len(cached) > 0 {
			return cached, nil
		}
	}

	infos, err := sf.DescribeMetadataTypes(alias)
	if err != nil || len(infos) == 0 {
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

func compareProviders() []diff.Provider {
	return providersForMethod(compareMethodAuto)
}

func providersForMethod(method compareMethod) []diff.Provider {
	switch method {
	case compareMethodTooling:
		var out []diff.Provider
		for _, label := range toolingTypeOrder() {
			out = append(out, toolingCompareTypes[label]())
		}
		return out
	case compareMethodMetadataAPI:
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

func toolingTypeOrder() []string {
	return []string{"ApexClass", "ApexTrigger"}
}

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
			Type:    p.typeLabel,
			Key:     it.FullName,
			ID:      it.FullName,
			Summary: mdapiSummary(it),
		})
	}
	return out, nil
}

func (p *mdapiProvider) Body(alias, id string) (string, error) {
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
