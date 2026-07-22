package ui

import (
	"os"
	"sort"
	"strings"
	"testing"
)

// TestLoadedResourcesCoversTopLevel guards against a newly-added
// top-level org Resource silently missing global refresh (ctrl+r):
// every `d.<Name> = Resource[...]{` assigned in orgdata_resources.go
// must be referenced by name in loadedResources() (orgdata_refresh.go).
func TestLoadedResourcesCoversTopLevel(t *testing.T) {
	declared := keyLiterals(t, "orgdata_resources.go", `d\.([A-Z][A-Za-z0-9]+)\s*=\s*Resource\[`)
	if len(declared) == 0 {
		t.Fatal("found no `d.X = Resource[` assignments — regex stale?")
	}
	// A resource is covered for global refresh EITHER by an explicit
	// &d.<Name> in loadedResources(), OR by a listResourceSpec
	// registration (loadedResources iterates the registry). Scan both
	// sources so registered resources count as covered.
	src := ""
	for _, f := range []string{"orgdata_refresh.go", "list_resource_registrations.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src += string(b)
	}
	var missing []string
	for name := range declared {
		if !strings.Contains(src, "&d."+name) {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("top-level Resources not covered by loadedResources(): %v\n"+
			"either add `&d.%s` to loadedResources() in orgdata_refresh.go, or\n"+
			"register it via registerListResource() in list_resource_registrations.go",
			missing, missing[0])
	}
}

// TestLoadedResourcesOnlyReturnsLoaded confirms global refresh touches
// only resources that have actually loaded — cold ones are skipped so
// ctrl+r never triggers a surprise mass-fetch of never-opened data.
func TestLoadedResourcesOnlyReturnsLoaded(t *testing.T) {
	d := &orgData{}

	// A bare orgData has loaded nothing.
	if got := d.loadedResources(); len(got) != 0 {
		t.Fatalf("fresh orgData has %d loaded resources, want 0", len(got))
	}

	// Loading one resource (Resource.Set stamps FetchedAt → Loaded) makes
	// exactly that one appear.
	d.SObjects.Set(nil)
	if got := d.loadedResources(); len(got) != 1 {
		t.Fatalf("after loading SObjects, loadedResources() = %d, want 1", len(got))
	}
	if !d.SObjects.Loaded() {
		t.Error("SObjects should report Loaded after Set")
	}
}
