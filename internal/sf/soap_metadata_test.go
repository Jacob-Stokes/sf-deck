package sf

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const sampleReadResp = `<?xml version="1.0" encoding="UTF-8"?><soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns="http://soap.sforce.com/2006/04/metadata" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><soapenv:Body><readMetadataResponse><result>` +
	`<records xsi:type="CustomObject"><fullName>Request__c</fullName>` +
	`<fields><fullName>Academic_year__c</fullName><label>Academic Year</label><type>Text</type></fields>` +
	`<fields><fullName>Status__c</fullName><label>Status</label><type>Picklist</type></fields>` +
	`<validationRules><fullName>Amount_Positive</fullName><active>true</active></validationRules>` +
	`</records>` +
	`<records xsi:type="CustomObject"><fullName>Other__c</fullName>` +
	`<fields><fullName>Foo__c</fullName><type>Number</type></fields>` +
	`</records>` +
	`<records xsi:nil="true"/>` + // a name that didn't exist
	`</result></readMetadataResponse></soapenv:Body></soapenv:Envelope>`

func TestParseReadMetadataRecords(t *testing.T) {
	recs := parseReadMetadataRecords(sampleReadResp)
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2 (nil record skipped): keys=%v", len(recs), keysOf(recs))
	}
	if _, ok := recs["Request__c"]; !ok {
		t.Errorf("missing Request__c; keys=%v", keysOf(recs))
	}
	if _, ok := recs["Other__c"]; !ok {
		t.Errorf("missing Other__c; keys=%v", keysOf(recs))
	}
	// The Request__c body must contain its fields + VR.
	body := recs["Request__c"]
	if !contains(body, "Academic_year__c") || !contains(body, "Amount_Positive") {
		t.Errorf("Request__c body missing nested content: %q", body)
	}
	// Must NOT bleed into Other__c.
	if contains(recs["Request__c"], "Foo__c") {
		t.Error("Request__c record bled into Other__c content")
	}
}

func TestExtractNestedFields(t *testing.T) {
	body := parseReadMetadataRecords(sampleReadResp)["Request__c"]
	fields := extractBlocks(body, "fields")
	if len(fields) != 2 {
		t.Fatalf("got %d field blocks, want 2", len(fields))
	}
	if innerText(fields[0], "fullName") != "Academic_year__c" {
		t.Errorf("field[0] fullName = %q", innerText(fields[0], "fullName"))
	}
	vrs := extractBlocks(body, "validationRules")
	if len(vrs) != 1 || innerText(vrs[0], "fullName") != "Amount_Positive" {
		t.Errorf("validationRules wrong: %v", vrs)
	}
}

func TestSoapFault(t *testing.T) {
	fault := `<soapenv:Envelope><soapenv:Body><soapenv:Fault><faultcode>sf:INVALID_TYPE</faultcode><faultstring>INVALID_TYPE: not available</faultstring></soapenv:Fault></soapenv:Body></soapenv:Envelope>`
	if got := soapFault(fault); got != "INVALID_TYPE: not available" {
		t.Errorf("soapFault = %q", got)
	}
	if soapFault(sampleReadResp) != "" {
		t.Error("non-fault response reported a fault")
	}
}

func keysOf(m map[string]string) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	return k
}

const sampleListResp = `<?xml version="1.0" encoding="UTF-8"?><soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns="http://soap.sforce.com/2006/04/metadata"><soapenv:Body><listMetadataResponse>` +
	`<result><fullName>MyApp</fullName><type>CustomApplication</type><namespacePrefix></namespacePrefix><lastModifiedDate>2026-01-02T03:04:05.000Z</lastModifiedDate></result>` +
	`<result><fullName>pkg__Managed</fullName><type>CustomApplication</type><namespacePrefix>pkg</namespacePrefix></result>` +
	`<result><fullName>My_Flow</fullName><type>Flow</type></result>` +
	`</listMetadataResponse></soapenv:Body></soapenv:Envelope>`

func TestParseListMetadataResult(t *testing.T) {
	byType := parseListMetadataResult(sampleListResp)
	apps := byType["CustomApplication"]
	if len(apps) != 2 {
		t.Fatalf("CustomApplication: got %d, want 2", len(apps))
	}
	if apps[0].FullName != "MyApp" || apps[0].Type != "CustomApplication" {
		t.Errorf("app[0] = %+v", apps[0])
	}
	if apps[0].LastModifiedDate != "2026-01-02T03:04:05.000Z" {
		t.Errorf("lastModifiedDate = %q", apps[0].LastModifiedDate)
	}
	// Managed component is present with its namespace (callers filter it).
	if apps[1].NamespacePrefix != "pkg" {
		t.Errorf("managed app namespace = %q, want pkg", apps[1].NamespacePrefix)
	}
	if flows := byType["Flow"]; len(flows) != 1 || flows[0].FullName != "My_Flow" {
		t.Errorf("Flow result wrong: %+v", flows)
	}
}

func TestSoapListEnvelope(t *testing.T) {
	env := soapListEnvelope("TOKEN&", "62.0", []string{"Flow", "Layout"})
	for _, want := range []string{
		"listMetadata", "<met:asOfVersion>62.0</met:asOfVersion>",
		"<met:type>Flow</met:type>", "<met:type>Layout</met:type>",
		"<met:sessionId>TOKEN&amp;</met:sessionId>", // token XML-escaped
	} {
		if !contains(env, want) {
			t.Errorf("envelope missing %q:\n%s", want, env)
		}
	}
}

func TestSoapBatchSizeFor(t *testing.T) {
	if soapBatchSizeFor("Profile") != 1 {
		t.Errorf("Profile batch = %d, want 1 (huge payload)", soapBatchSizeFor("Profile"))
	}
	if soapBatchSizeFor("PermissionSet") != 3 {
		t.Errorf("PermissionSet batch = %d, want 3", soapBatchSizeFor("PermissionSet"))
	}
	if soapBatchSizeFor("CustomObject") != soapReadBatchMax {
		t.Errorf("CustomObject batch = %d, want %d", soapBatchSizeFor("CustomObject"), soapReadBatchMax)
	}
}

func TestReadMetadataHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer srv.Close()

	c := soapTestClient(srv, "TOK")
	recs, err := c.ReadMetadata("Flow", []string{"MyFlow"})
	if err == nil {
		t.Fatal("ReadMetadata returned nil error for HTTP 502")
	}
	if len(recs) != 0 {
		t.Fatalf("ReadMetadata returned records on HTTP error: %v", recs)
	}
	if !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("error = %q, want HTTP 502", err.Error())
	}
}

func TestListMetadataHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	c := soapTestClient(srv, "TOK")
	items, err := c.ListMetadata([]string{"Flow"})
	if err == nil {
		t.Fatal("ListMetadata returned nil error for HTTP 403")
	}
	if len(items) != 0 {
		t.Fatalf("ListMetadata returned items on HTTP error: %v", items)
	}
	if !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("error = %q, want HTTP 403", err.Error())
	}
}

func TestReadMetadataRetriesInvalidSession(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		raw, _ := io.ReadAll(r.Body)
		switch n {
		case 1:
			if !strings.Contains(string(raw), "OLD") {
				t.Fatalf("first SOAP request did not use old token: %s", raw)
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`<soapenv:Envelope><soapenv:Body><soapenv:Fault><faultstring>INVALID_SESSION_ID: Session expired</faultstring></soapenv:Fault></soapenv:Body></soapenv:Envelope>`))
		case 2:
			if !strings.Contains(string(raw), "NEW") {
				t.Fatalf("retry SOAP request did not use refreshed token: %s", raw)
			}
			_, _ = w.Write([]byte(sampleReadResp))
		default:
			t.Fatalf("unexpected SOAP call %d", n)
		}
	}))
	defer srv.Close()
	installFakeSF(t, srv.URL)

	c := soapTestClient(srv, "OLD")
	recs, err := c.ReadMetadata("CustomObject", []string{"Request__c"})
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("SOAP calls = %d, want 2", calls)
	}
	if recs["Request__c"] == "" {
		t.Fatalf("retry response not parsed: %v", recs)
	}
}

func soapTestClient(srv *httptest.Server, token string) *Client {
	return &Client{
		alias:       "test-org",
		accessToken: token,
		instanceURL: srv.URL,
		apiVersion:  "62.0",
		http:        srv.Client(),
	}
}

func installFakeSF(t *testing.T, instanceURL string) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "sf")
	body := `#!/bin/sh
printf '{"result":{"accessToken":"NEW","instanceUrl":"%s","apiVersion":"62.0"}}' "$SF_FAKE_INSTANCE"
`
	if err := os.WriteFile(script, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SF_FAKE_INSTANCE", instanceURL)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
