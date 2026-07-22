package sf

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// objectXMLWithAllChildren is a CustomObject readMetadata record with one
// of each child type, to verify extractObjectChildren covers them all.
const objectXMLWithAllChildren = `<fullName>Acct__c</fullName>` +
	`<fields><fullName>F1__c</fullName><type>Text</type></fields>` +
	`<validationRules><fullName>VR1</fullName><active>true</active></validationRules>` +
	`<recordTypes><fullName>RT1</fullName></recordTypes>` +
	`<compactLayouts><fullName>CL1</fullName></compactLayouts>` +
	`<webLinks><fullName>WL1</fullName></webLinks>` +
	`<listViews><fullName>LV1</fullName></listViews>` +
	`<fieldSets><fullName>FS1</fullName></fieldSets>` +
	`<indexes><fullName>IDX1</fullName></indexes>` +
	`<businessProcesses><fullName>BP1</fullName></businessProcesses>` +
	`<sharingReasons><fullName>SR1</fullName></sharingReasons>`

func TestExtractObjectChildrenAllTypes(t *testing.T) {
	snap := SOAPSnapshot{}
	extractObjectChildren("Acct__c", objectXMLWithAllChildren, snap)

	want := map[string]string{
		"CustomField":     "Acct__c.F1__c",
		"ValidationRule":  "Acct__c.VR1",
		"RecordType":      "Acct__c.RT1",
		"CompactLayout":   "Acct__c.CL1",
		"WebLink":         "Acct__c.WL1",
		"ListView":        "Acct__c.LV1",
		"FieldSet":        "Acct__c.FS1",
		"Index":           "Acct__c.IDX1",
		"BusinessProcess": "Acct__c.BP1",
		"SharingReason":   "Acct__c.SR1",
	}
	for typeLabel, key := range want {
		bucket, ok := snap[typeLabel]
		if !ok {
			t.Errorf("%s: no bucket created", typeLabel)
			continue
		}
		if _, ok := bucket[key]; !ok {
			t.Errorf("%s: missing key %q (got keys %v)", typeLabel, key, keysOf2(bucket))
		}
	}
}

func TestRetrieveViaSOAPGatedBoundsReadMetadataCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		_, _ = w.Write([]byte(sampleReadResp))
	}))
	defer srv.Close()

	InvalidateRESTClients()
	c := &Client{
		alias:       "gated",
		accessToken: "TOK",
		instanceURL: srv.URL,
		apiVersion:  "62.0",
		http:        srv.Client(),
	}
	clientsMu.Lock()
	entry := &clientEntry{client: c}
	entry.once.Do(func() {})
	clients["gated"] = entry
	clientsMu.Unlock()
	defer InvalidateRESTClients()

	sem := make(chan struct{}, 2)
	var inFlight, peak int64
	acquire := func() {
		sem <- struct{}{}
		n := atomic.AddInt64(&inFlight, 1)
		for {
			p := atomic.LoadInt64(&peak)
			if n <= p || atomic.CompareAndSwapInt64(&peak, p, n) {
				break
			}
		}
	}
	release := func() {
		atomic.AddInt64(&inFlight, -1)
		<-sem
	}

	names := []string{
		"A", "B", "C", "D", "E", "F", "G", "H", "I", "J",
		"K", "L", "M", "N", "O", "P", "Q", "R", "S", "T",
		"U", "V", "W", "X", "Y",
	}
	if _, err := RetrieveViaSOAPGated("gated", "Flow", names, false, acquire, release); err != nil {
		t.Fatal(err)
	}
	if peak > 2 {
		t.Fatalf("peak gated readMetadata calls = %d, want <= 2", peak)
	}
	if peak < 2 {
		t.Fatalf("peak gated readMetadata calls = %d, want to observe contention", peak)
	}
}

func keysOf2(m map[string]string) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	return k
}
