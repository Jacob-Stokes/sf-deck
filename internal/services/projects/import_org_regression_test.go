package projects

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportBundleKeepsSameMetadataFromDifferentOrgs(t *testing.T) {
	s := newTestStore(t)
	p, err := Create(s, CreateInput{Name: "Import test"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "package.xml")
	if err := os.WriteFile(path, []byte(`<Package><types><members>Example</members><name>ApexClass</name></types></Package>`), 0600); err != nil {
		t.Fatal(err)
	}
	for i, org := range []string{"a@example.test", "b@example.test", "b@example.test"} {
		result, err := ImportBundle(s, ImportBundleInput{ProjectID: p.Project.ID, Path: path, OrgUser: org})
		if err != nil {
			t.Fatal(err)
		}
		want := 1
		if i == 2 {
			want = 0
		}
		if result.Added != want {
			t.Fatalf("import %d added %d, want %d", i, result.Added, want)
		}
	}
}
