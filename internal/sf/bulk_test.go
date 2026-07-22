package sf

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestBulkQuery_Lifecycle drives a fake Salesforce through submit →
// poll-poll-complete → 2-chunk download. Verifies the result file is a
// valid CSV with one header row and rows from both chunks.
func TestBulkQuery_Lifecycle(t *testing.T) {
	var (
		mu              sync.Mutex
		submitCount     int
		statusCount     int
		downloadCount   int
		statusSequence  = []string{"UploadComplete", "InProgress", "JobComplete"}
		downloadPayload = [][]string{
			{"Id,Name", "0011,Acme", "0012,Globex"},   // first chunk
			{"Id,Name", "0013,Initech", "0014,Hooli"}, // second chunk (header stripped by caller)
		}
		jobID = "750xx0000000FAKE"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/jobs/query"):
			submitCount++
			body, _ := json.Marshal(map[string]string{"id": jobID, "state": "UploadComplete"})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/jobs/query/"+jobID):
			idx := statusCount
			if idx >= len(statusSequence) {
				idx = len(statusSequence) - 1
			}
			statusCount++
			state := statusSequence[idx]
			body, _ := json.Marshal(map[string]string{"state": state})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/jobs/query/"+jobID+"/results"):
			idx := downloadCount
			if idx >= len(downloadPayload) {
				idx = len(downloadPayload) - 1
			}
			downloadCount++
			payload := downloadPayload[idx]
			w.Header().Set("Sforce-NumberOfRecords", "2")
			if idx < len(downloadPayload)-1 {
				w.Header().Set("Sforce-Locator", "loc-"+payload[1])
			}
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte(strings.Join(payload, "\n")))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &Client{
		alias:       "test",
		accessToken: "token",
		instanceURL: srv.URL,
		apiVersion:  "62.0",
		http:        srv.Client(),
	}

	// Tighten poll cadence for the test — overriding via a tiny
	// retry loop driver isn't worth a Client field; just accept the
	// 2s first-poll and run the test in parallel-able mode.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var out bytes.Buffer
	progress := make(chan BulkQueryProgress, 32)
	res, err := c.BulkQuery(ctx, "SELECT Id, Name FROM Account", &out, progress)
	close(progress)
	if err != nil {
		t.Fatalf("BulkQuery: %v", err)
	}

	if res.JobID != jobID {
		t.Errorf("JobID = %q, want %q", res.JobID, jobID)
	}
	if res.Chunks != 2 {
		t.Errorf("Chunks = %d, want 2", res.Chunks)
	}
	if res.Polls < 3 {
		t.Errorf("Polls = %d, want >=3", res.Polls)
	}

	got := out.String()
	// Must contain header row exactly once.
	if got == "" {
		t.Fatal("output empty")
	}
	if c := strings.Count(got, "Id,Name"); c != 1 {
		t.Errorf("Id,Name header appears %d times, want 1", c)
	}
	// Must contain data rows from BOTH chunks.
	for _, expect := range []string{"Acme", "Globex", "Initech", "Hooli"} {
		if !strings.Contains(got, expect) {
			t.Errorf("output missing %q\noutput=\n%s", expect, got)
		}
	}
}

// TestBulkQuery_JobFailed asserts that a SF "Failed" status surfaces as
// a Go error rather than being silently swallowed.
func TestBulkQuery_JobFailed(t *testing.T) {
	jobID := "750xx0000000FAIL"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/jobs/query"):
			_, _ = w.Write([]byte(`{"id":"` + jobID + `","state":"UploadComplete"}`))
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/jobs/query/"+jobID):
			_, _ = w.Write([]byte(`{"state":"Failed"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &Client{
		alias:       "test",
		accessToken: "token",
		instanceURL: srv.URL,
		apiVersion:  "62.0",
		http:        srv.Client(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out bytes.Buffer
	_, err := c.BulkQuery(ctx, "SELECT Id FROM Account", &out, nil)
	if err == nil {
		t.Fatal("expected error for Failed state, got nil")
	}
	if !strings.Contains(err.Error(), "Failed") {
		t.Errorf("error %q should mention 'Failed'", err)
	}
}

func TestParseBulkCSV(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"empty body", "", 0},
		{"header only", "Id,Name\n", 0},
		{"basic rows", "Id,Name\n001,Acme\n002,Globex\n", 2},
		{"quoted comma", "Id,Name\n001,\"Acme, Inc.\"\n", 1},
		{"dotted header", "Id,Account.Name\n001,Acme\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := parseBulkCSV([]byte(tc.body))
			if err != nil {
				t.Fatalf("parseBulkCSV: %v", err)
			}
			if got := len(res.Records); got != tc.want {
				t.Errorf("rows: got %d want %d", got, tc.want)
			}
			if !res.Done {
				t.Errorf("Done should be true for Bulk results")
			}
		})
	}

	// Spot-check field plumbing on a representative row.
	res, err := parseBulkCSV([]byte("Id,Account.Name\n001,Acme\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Records[0]["Id"]; got != "001" {
		t.Errorf("Id: got %v want 001", got)
	}
	if got := res.Records[0]["Account.Name"]; got != "Acme" {
		t.Errorf("Account.Name: got %v want Acme", got)
	}
}
