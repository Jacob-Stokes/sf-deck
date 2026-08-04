package ui

import (
	"os"
	"regexp"
	"sort"
	"testing"
)

// TestEveryOrgDataResourceKeyIsRoutable guards against the class of bug
// where a Resource is ensured/refreshed (so it sets Busy and emits an
// UpdatedMsg) but applyResourceMsg has no case for its Key — the msg is
// dropped and the resource stays Busy forever (the original org_info
// bug). Every Key: literal declared on a top-level Resource in
// orgdata_resources.go must appear as a case in the applyResourceMsg
// switch in update.go.
//
// This is a source-level check rather than a behavioural one because
// each Resource has a different payload type T, so driving a real
// UpdatedMsg per key would need 20 hand-written typed payloads — far
// more brittle than asserting the routing table covers the key set.
func TestEveryOrgDataResourceKeyIsRoutable(t *testing.T) {
	declared := keyLiterals(t, "orgdata_resources.go", `Key:\s*"([a-z0-9_]+)"`)
	if len(declared) == 0 {
		t.Fatal("found no Key: literals in orgdata_resources.go — regex stale?")
	}
	routed := switchCaseKeys(t, "update.go")
	if len(routed) == 0 {
		t.Fatal("found no case keys in update.go switch — parser stale?")
	}
	// A resource is routable EITHER via an explicit switch case (bespoke
	// apply logic) OR via a listResourceSpec registration (the generic
	// Apply→sync→refresh path). Fold the registry into the routable set.
	for _, key := range listResourceOrder {
		routed[key] = true
	}

	var missing []string
	for k := range declared {
		if !routed[k] {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("orgData Resource keys ensured but NOT routed in applyResourceMsg: %v\n"+
			"add a `case %q:` (Apply + FromCache refresh) to the switch in update.go",
			missing, missing[0])
	}
}

// TestEveryKeyedResourcePrefixIsRouted is the prefix-routing sibling of
// the test above. The per-key Ensure* methods in model.go build their
// Resource key as `"<prefix>:" + sobject/id` and rely on a matching
// `{Prefix: "<prefix>:"}` entry in update_resource_helpers.go to route
// the UpdatedMsg. Renaming a key (e.g. describe: → describe_v2:) without
// updating the prefix silently drops every msg for that resource, so it
// stays Busy forever — which is exactly how the describe hang happened.
// Assert every prefixed key in model.go has a routing prefix.
func TestEveryKeyedResourcePrefixIsRouted(t *testing.T) {
	keys := keyLiterals(t, "model.go", `Key:\s*"([a-z0-9_]+):"`)
	if len(keys) == 0 {
		t.Fatal("found no prefixed Key: literals in model.go — regex stale?")
	}
	routed := keyLiterals(t, "update_resource_helpers.go", `Prefix:\s*"([a-z0-9_]+):"`)
	if len(routed) == 0 {
		t.Fatal("found no Prefix: literals in update_resource_helpers.go — regex stale?")
	}
	var missing []string
	for k := range keys {
		if !routed[k] {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("Ensure* Resource key prefixes with no routing handler: %v\n"+
			"add a `{Prefix: %q, ...}` entry to resourcePrefixHandlers in update_resource_helpers.go",
			missing, missing[0]+":")
	}
}

// keyLiterals extracts the capture group of pattern from a source file
// into a set.
func keyLiterals(t *testing.T, file, pattern string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	re := regexp.MustCompile(pattern)
	out := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		out[m[1]] = true
	}
	return out
}

// switchCaseKeys collects every `case "<key>":` literal in the file.
// Broad on purpose — we only assert the declared keys are a subset, so
// picking up unrelated case strings is harmless.
func switchCaseKeys(t *testing.T, file string) map[string]bool {
	return keyLiterals(t, file, `case\s+"([a-z0-9_]+)":`)
}
