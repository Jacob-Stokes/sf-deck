package projects

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

// ImportBundle parses a package.xml off disk + inserts each
// referenced metadata member as a project item. Tests use a real
// (in-temp-dir) devproject Store and temp manifest files. No org
// contact.

// ----- happy path ------------------------------------------------

func TestImportBundle_HappyPath(t *testing.T) {
	s := newTestStore(t)
	res, err := Create(s, CreateInput{Name: "Import target"})
	if err != nil {
		t.Fatal(err)
	}
	projectID := res.Project.ID

	dir := t.TempDir()
	manifest := filepath.Join(dir, "package.xml")
	body := `<?xml version="1.0" encoding="UTF-8"?>
<Package xmlns="http://soap.sforce.com/2006/04/metadata">
  <types>
    <members>Shipment__c</members>
    <name>CustomObject</name>
  </types>
  <types>
    <members>Account.Phone</members>
    <members>Account.Industry</members>
    <name>CustomField</name>
  </types>
  <types>
    <members>Shipment_Status_Change</members>
    <name>Flow</name>
  </types>
  <types>
    <members>FlightStatusController</members>
    <name>ApexClass</name>
  </types>
  <types>
    <members>UnknownType_That_We_Skip</members>
    <name>SomeFutureMetadataType</name>
  </types>
  <version>62.0</version>
</Package>`
	if err := os.WriteFile(manifest, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := ImportBundle(s, ImportBundleInput{
		ProjectID: projectID,
		Path:      manifest,
		OrgUser:   "user@example.com",
	})
	if err != nil {
		t.Fatalf("ImportBundle: %v", err)
	}
	if out.Added != 5 {
		t.Errorf("Added = %d, want 5 (Shipment__c + Account.Phone + Account.Industry + Flow + Apex)", out.Added)
	}
	if out.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", out.Skipped)
	}
	if len(out.Unknown) != 1 || out.Unknown[0] != "SomeFutureMetadataType" {
		t.Errorf("Unknown = %v, want [SomeFutureMetadataType]", out.Unknown)
	}

	// Verify the items actually landed.
	items, err := ListItems(s, projectID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 5 {
		t.Errorf("ListItems len = %d, want 5", len(items))
	}

	// Field name + type extraction (Account.Phone → Name=Phone, Type=Account)
	var found bool
	for _, it := range items {
		if devproject.ItemKind(it.Kind) == devproject.KindField && it.Ref == "Account.Phone" {
			found = true
			if it.Name != "Phone" {
				t.Errorf("field Name = %q, want Phone", it.Name)
			}
			if it.Type != "Account" {
				t.Errorf("field Type = %q, want Account", it.Type)
			}
		}
	}
	if !found {
		t.Error("Account.Phone item not found")
	}
}

// ----- idempotency ------------------------------------------------

func TestImportBundle_IdempotentSecondRun(t *testing.T) {
	s := newTestStore(t)
	res, _ := Create(s, CreateInput{Name: "X"})
	projectID := res.Project.ID

	manifest := writeTempManifest(t, `<?xml version="1.0"?>
<Package>
  <types>
    <members>Shipment__c</members>
    <name>CustomObject</name>
  </types>
</Package>`)

	first, err := ImportBundle(s, ImportBundleInput{ProjectID: projectID, Path: manifest})
	if err != nil {
		t.Fatal(err)
	}
	if first.Added != 1 || first.Skipped != 0 {
		t.Errorf("first: Added=%d Skipped=%d, want 1/0", first.Added, first.Skipped)
	}

	second, err := ImportBundle(s, ImportBundleInput{ProjectID: projectID, Path: manifest})
	if err != nil {
		t.Fatal(err)
	}
	if second.Added != 0 || second.Skipped != 1 {
		t.Errorf("second: Added=%d Skipped=%d, want 0/1", second.Added, second.Skipped)
	}
}

// ----- directory-path resolution ----------------------------------

func TestImportBundle_DirectoryPath(t *testing.T) {
	s := newTestStore(t)
	res, _ := Create(s, CreateInput{Name: "X"})
	projectID := res.Project.ID

	dir := t.TempDir()
	body := `<?xml version="1.0"?>
<Package>
  <types><members>Shipment__c</members><name>CustomObject</name></types>
</Package>`
	if err := os.WriteFile(filepath.Join(dir, "package.xml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pass the DIR, not the file.
	out, err := ImportBundle(s, ImportBundleInput{ProjectID: projectID, Path: dir})
	if err != nil {
		t.Fatalf("ImportBundle from dir: %v", err)
	}
	if out.Added != 1 {
		t.Errorf("Added = %d, want 1", out.Added)
	}
}

// ----- wildcard members are skipped -------------------------------

func TestImportBundle_WildcardMemberSkipped(t *testing.T) {
	s := newTestStore(t)
	res, _ := Create(s, CreateInput{Name: "X"})
	manifest := writeTempManifest(t, `<?xml version="1.0"?>
<Package>
  <types>
    <members>*</members>
    <members>Shipment__c</members>
    <name>CustomObject</name>
  </types>
</Package>`)
	out, err := ImportBundle(s, ImportBundleInput{ProjectID: res.Project.ID, Path: manifest})
	if err != nil {
		t.Fatal(err)
	}
	if out.Added != 1 {
		t.Errorf("Added = %d, want 1 (wildcard ignored)", out.Added)
	}
}

// ----- error paths -----------------------------------------------

func TestImportBundle_NilStore(t *testing.T) {
	_, err := ImportBundle(nil, ImportBundleInput{ProjectID: "p", Path: "/x"})
	if err == nil || !strings.Contains(err.Error(), "nil") {
		t.Errorf("expected nil-store error; got %v", err)
	}
}

func TestImportBundle_MissingProjectID(t *testing.T) {
	s := newTestStore(t)
	_, err := ImportBundle(s, ImportBundleInput{Path: "/x"})
	if err == nil || !strings.Contains(err.Error(), "project id") {
		t.Errorf("expected project-id err; got %v", err)
	}
}

func TestImportBundle_MissingPath(t *testing.T) {
	s := newTestStore(t)
	_, err := ImportBundle(s, ImportBundleInput{ProjectID: "p1"})
	if err == nil || !strings.Contains(err.Error(), "path") {
		t.Errorf("expected path err; got %v", err)
	}
}

func TestImportBundle_NonexistentPath(t *testing.T) {
	s := newTestStore(t)
	res, _ := Create(s, CreateInput{Name: "X"})
	_, err := ImportBundle(s, ImportBundleInput{
		ProjectID: res.Project.ID,
		Path:      "/nonexistent/package.xml",
	})
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestImportBundle_DirectoryWithoutManifest(t *testing.T) {
	s := newTestStore(t)
	res, _ := Create(s, CreateInput{Name: "X"})
	dir := t.TempDir() // empty
	_, err := ImportBundle(s, ImportBundleInput{ProjectID: res.Project.ID, Path: dir})
	if err == nil || !strings.Contains(err.Error(), "package.xml") {
		t.Errorf("expected 'missing package.xml' err; got %v", err)
	}
}

func TestImportBundle_ProjectNotFound(t *testing.T) {
	s := newTestStore(t)
	manifest := writeTempManifest(t, `<?xml version="1.0"?>
<Package><types><members>X</members><name>CustomObject</name></types></Package>`)
	_, err := ImportBundle(s, ImportBundleInput{
		ProjectID: "nonexistent",
		Path:      manifest,
	})
	if err == nil {
		t.Error("expected not-found error")
	}
}

func TestImportBundle_MalformedXMLFails(t *testing.T) {
	s := newTestStore(t)
	res, _ := Create(s, CreateInput{Name: "X"})
	manifest := writeTempManifest(t, `<Package><not-closed>`)
	_, err := ImportBundle(s, ImportBundleInput{ProjectID: res.Project.ID, Path: manifest})
	if err == nil {
		t.Error("expected parse error")
	}
}

// ----- deriveName / deriveType (pure functions) -------------------

func TestDeriveName_CustomFieldStripsParent(t *testing.T) {
	if got := deriveName(devproject.KindField, "Account.Phone"); got != "Phone" {
		t.Errorf("got %q, want Phone", got)
	}
	// No dot → bare ref echoed
	if got := deriveName(devproject.KindField, "Phone"); got != "Phone" {
		t.Errorf("got %q, want Phone", got)
	}
}

func TestDeriveName_ReportStripsFolder(t *testing.T) {
	if got := deriveName(devproject.KindReport, "Folder/MyReport"); got != "MyReport" {
		t.Errorf("got %q, want MyReport", got)
	}
}

func TestDeriveName_OtherKindsEchoRef(t *testing.T) {
	if got := deriveName(devproject.KindFlow, "MyFlow"); got != "MyFlow" {
		t.Errorf("got %q, want MyFlow", got)
	}
	if got := deriveName(devproject.KindApexClass, "FooController"); got != "FooController" {
		t.Errorf("got %q", got)
	}
}

func TestDeriveType_CustomFieldUsesParent(t *testing.T) {
	if got := deriveType(devproject.KindField, "Account.Phone"); got != "Account" {
		t.Errorf("got %q", got)
	}
	if got := deriveType(devproject.KindField, "Phone"); got != "" {
		t.Errorf("no dot should yield empty; got %q", got)
	}
}

func TestDeriveType_ReportUsesFolder(t *testing.T) {
	if got := deriveType(devproject.KindReport, "Folder/MyReport"); got != "Folder" {
		t.Errorf("got %q, want Folder", got)
	}
}

func TestDeriveType_OtherKindsEmpty(t *testing.T) {
	if got := deriveType(devproject.KindFlow, "AnyRef"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// ----- metadataTypeToKind ----------------------------------------

func TestMetadataTypeToKind_Coverage(t *testing.T) {
	cases := []struct {
		in   string
		want devproject.ItemKind
	}{
		{"CustomObject", devproject.KindSObject},
		{"CustomField", devproject.KindField},
		{"Flow", devproject.KindFlow},
		{"ApexClass", devproject.KindApexClass},
		{"ApexTrigger", devproject.KindApexTrigger},
		{"Report", devproject.KindReport},
		{"PermissionSet", devproject.KindPermissionSet},
		{"PermissionSetGroup", devproject.KindPermissionSetGroup},
		{"Profile", devproject.KindProfile},
		{"ValidationRule", devproject.KindValidationRule},
		{"RecordType", devproject.KindRecordType},
		{"LightningComponentBundle", devproject.KindLWC},
		{"AuraDefinitionBundle", devproject.KindAura},
		{"Queue", devproject.KindQueue},
		{"Group", devproject.KindPublicGroup},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := metadataTypeToKind(c.in)
			if !ok {
				t.Fatal("unknown")
			}
			if got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestMetadataTypeToKind_Unknown(t *testing.T) {
	if _, ok := metadataTypeToKind("ApexLegoSet"); ok {
		t.Error("expected unknown=false")
	}
}

// ----- itemKey ----------------------------------------------------

func TestItemKey_StableShape(t *testing.T) {
	if itemKey(devproject.KindFlow, "MyFlow") != "flow|MyFlow" {
		t.Errorf("got %q", itemKey(devproject.KindFlow, "MyFlow"))
	}
}

// ----- helpers ----------------------------------------------------

func writeTempManifest(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "package.xml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
