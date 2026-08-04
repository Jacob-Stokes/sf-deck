package sf

// Bulk API 2.0 query client. Used by the records "full export" flow
// (ctrl+x in /records when the chip is showing a preview) to pull the
// entire matching dataset to disk without going through the synchronous
// /query + cursor-follow path.
//
// Lifecycle:
//   1. POST /services/data/vXX.0/jobs/query
//      Body: {"operation":"query","query":"SELECT ..."}
//      → 200 with {"id": "<jobId>", ...}
//   2. GET /services/data/vXX.0/jobs/query/<jobId>  (poll)
//      → {"state":"UploadComplete" | "InProgress" | "JobComplete" | "Failed" | "Aborted"}
//      Poll every ~2-5s until JobComplete (or terminal failure).
//   3. GET /services/data/vXX.0/jobs/query/<jobId>/results
//      → text/csv body. Optional response header "Sforce-Locator" gives
//      the cursor for the next chunk (call again with ?locator=<value>).
//      "Sforce-NumberOfRecords" header reports rows in this chunk.
//
// Each network call ticks the usage tracker via fireOnCall, same as
// REST. Polling is NOT free against the daily API limit — every poll
// counts as a call. The caller is responsible for setting a sensible
// poll cadence (we default to 2s, ramping to 10s after the first
// minute) so a 30-second job uses ~10 polls instead of 30.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// BulkQueryResult summarises a completed bulk export. The body has
// already been streamed to the caller's io.Writer; this struct carries
// the metadata.
type BulkQueryResult struct {
	JobID    string
	RowCount int           // total records written across all chunks
	Chunks   int           // how many /results pages we walked
	Polls    int           // how many status polls fired
	Elapsed  time.Duration // wall-clock from submit to last byte
}

// BulkQueryProgress reports per-stage progress so the UI can render a
// "submitting / polling / downloading" affordance instead of a black-box
// wait. Sent over the progress channel passed to BulkQuery; consumers
// can ignore it (nil channel = no progress).
type BulkQueryProgress struct {
	Stage   string // "submit" | "poll" | "download" | "done"
	JobID   string
	State   string // SF-reported state during polling
	Rows    int    // cumulative rows written during download
	Chunks  int    // cumulative chunks downloaded
	Polls   int    // cumulative status polls
	Elapsed time.Duration
}

// BulkQuery submits the given SOQL as a Bulk API 2.0 query job, polls
// it to completion, and streams the CSV result into out. Returns a
// summary or an error. ctx cancellation aborts the job (best-effort;
// SF doesn't always honour DELETE quickly).
//
// progress, if non-nil, receives Stage updates. The channel is NOT
// closed by BulkQuery — the caller owns its lifetime. Sends are
// non-blocking (drop if the consumer can't keep up) so a slow UI
// thread can't stall the network goroutine.
func (c *Client) BulkQuery(ctx context.Context, soql string, out io.Writer, progress chan<- BulkQueryProgress) (BulkQueryResult, error) {
	start := time.Now()
	res := BulkQueryResult{}
	sendProgress := func(p BulkQueryProgress) {
		p.Elapsed = time.Since(start)
		if progress == nil {
			return
		}
		select {
		case progress <- p:
		default:
		}
	}

	// Step 1: submit the job.
	sendProgress(BulkQueryProgress{Stage: "submit"})
	jobID, err := c.bulkSubmit(soql)
	if err != nil {
		return res, fmt.Errorf("bulk submit: %w", err)
	}
	res.JobID = jobID

	// Step 2: poll. Cadence ramps from 2s → 10s so short jobs respond
	// quickly without hammering the API limit on long ones.
	for {
		if err := ctx.Err(); err != nil {
			// Best-effort cancel: tell SF to abort the job so its
			// row-scan stops eating capacity. Ignore the ack error
			// (the user already wants out).
			_ = c.bulkAbort(jobID)
			return res, ctx.Err()
		}
		state, err := c.bulkStatus(jobID)
		res.Polls++
		sendProgress(BulkQueryProgress{Stage: "poll", JobID: jobID, State: state, Polls: res.Polls})
		if err != nil {
			return res, fmt.Errorf("bulk status: %w", err)
		}
		switch state {
		case "JobComplete":
			goto downloadLoop
		case "Failed", "Aborted":
			return res, fmt.Errorf("bulk job %s ended in state %s", jobID, state)
		}
		wait := pollInterval(res.Polls)
		select {
		case <-ctx.Done():
			_ = c.bulkAbort(jobID)
			return res, ctx.Err()
		case <-time.After(wait):
		}
	}

downloadLoop:
	// Step 3: stream results. Honour Sforce-Locator until empty.
	locator := ""
	for {
		if err := ctx.Err(); err != nil {
			return res, ctx.Err()
		}
		rows, nextLoc, err := c.bulkDownloadChunk(jobID, locator, out, res.Chunks == 0)
		res.Chunks++
		res.RowCount += rows
		sendProgress(BulkQueryProgress{
			Stage:  "download",
			JobID:  jobID,
			Rows:   res.RowCount,
			Chunks: res.Chunks,
			Polls:  res.Polls,
		})
		if err != nil {
			return res, fmt.Errorf("bulk download: %w", err)
		}
		if nextLoc == "" || nextLoc == "null" {
			break
		}
		locator = nextLoc
	}
	res.Elapsed = time.Since(start)
	sendProgress(BulkQueryProgress{Stage: "done", JobID: jobID, Rows: res.RowCount, Chunks: res.Chunks, Polls: res.Polls})
	return res, nil
}

// pollInterval returns the wait between status polls. Ramps from a
// fast initial cadence up to a slow steady one so a multi-minute job
// uses far fewer polls than a fixed interval would. The middle/steady
// value is the configurable BulkPoll (default 5s); the fast and slow
// ends are derived as half and double, so tuning one knob scales the
// whole ramp.
func pollInterval(pollCount int) time.Duration {
	steady := cfgBulkPoll()
	fast := steady / 2
	if fast < time.Second {
		fast = time.Second
	}
	slow := steady * 2
	switch {
	case pollCount < 5:
		return fast
	case pollCount < 15:
		return steady
	default:
		return slow
	}
}

// bulkSubmit POSTs the job-create request and returns the new job ID.
func (c *Client) bulkSubmit(soql string) (string, error) {
	path := c.APIPath("jobs/query")
	body, err := json.Marshal(map[string]any{
		"operation": "query",
		"query":     soql,
	})
	if err != nil {
		return "", err
	}
	resp, err := c.post(path, body)
	if err != nil {
		return "", err
	}
	var parsed struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return "", fmt.Errorf("decode submit response: %w", err)
	}
	if parsed.ID == "" {
		return "", fmt.Errorf("bulk submit returned empty job ID")
	}
	return parsed.ID, nil
}

// bulkStatus polls one status update.
func (c *Client) bulkStatus(jobID string) (string, error) {
	path := c.APIPath("jobs/query/" + jobID)
	resp, err := c.get(path, nil)
	if err != nil {
		return "", err
	}
	var parsed struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return "", fmt.Errorf("decode status: %w", err)
	}
	return parsed.State, nil
}

// bulkAbort tells SF to stop processing the job. Best-effort.
func (c *Client) bulkAbort(jobID string) error {
	path := c.APIPath("jobs/query/" + jobID)
	body, _ := json.Marshal(map[string]any{"state": "Aborted"})
	_, err := c.patch(path, body)
	return err
}

// bulkDownloadChunk streams one /results page into out. Returns the
// row count for this chunk and the Sforce-Locator header for the next
// page (or "" if this was the last chunk).
//
// When includeHeader is true, the CSV header row is written; on
// subsequent chunks it's stripped so the output file has one header
// row total.
func (c *Client) bulkDownloadChunk(jobID, locator string, out io.Writer, includeHeader bool) (int, string, error) {
	c.mu.Lock()
	token := c.accessToken
	base := c.instanceURL
	c.mu.Unlock()

	path := c.APIPath("jobs/query/" + jobID + "/results")
	q := url.Values{}
	if locator != "" {
		q.Set("locator", locator)
	}
	u := strings.TrimRight(base, "/") + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	logPath := path
	if len(q) > 0 {
		logPath += "?" + q.Encode()
	}

	startedAt := time.Now()
	var err error
	defer func() { fireOnCall(c.alias, []string{"GET", logPath}, err, time.Since(startedAt)) }()

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "text/csv")
	req.Header.Set("User-Agent", "sf-deck/0.1")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		err = &sfHTTPError{Status: resp.StatusCode, Body: body}
		return 0, "", err
	}

	nextLoc := resp.Header.Get("Sforce-Locator")
	rowsHeader := resp.Header.Get("Sforce-NumberOfRecords")
	rowsFromHeader := 0
	if rowsHeader != "" {
		if n, err := strconv.Atoi(rowsHeader); err == nil {
			rowsFromHeader = n
		}
	}

	// Stream the body line-by-line so we can:
	//   (a) skip the CSV header row on chunks after the first, and
	//   (b) emit progress as we go without buffering everything in
	//       memory.
	br := bufio.NewReaderSize(resp.Body, 32*1024)
	rows := 0
	lineNum := 0
	for {
		line, isPrefix, rerr := br.ReadLine()
		if rerr == io.EOF && len(line) == 0 {
			break
		}
		if rerr != nil && rerr != io.EOF {
			err = rerr
			return rows, "", rerr
		}
		// ReadLine truncates at the newline; rejoin if isPrefix
		// indicates the line was longer than the buffer.
		full := append([]byte(nil), line...)
		for isPrefix {
			var more []byte
			more, isPrefix, rerr = br.ReadLine()
			if rerr != nil && rerr != io.EOF {
				err = rerr
				return rows, "", rerr
			}
			full = append(full, more...)
		}
		lineNum++
		// Skip header on non-first chunks. The very first chunk
		// keeps its header so the output file is valid CSV.
		if lineNum == 1 && !includeHeader {
			continue
		}
		if _, werr := out.Write(full); werr != nil {
			err = werr
			return rows, "", werr
		}
		if _, werr := out.Write([]byte("\n")); werr != nil {
			err = werr
			return rows, "", werr
		}
		if lineNum > 1 || !includeHeader {
			rows++
		}
		if rerr == io.EOF {
			break
		}
	}
	// Prefer the SF-reported header count when it disagrees with our
	// line count (rare: CSV embedded newlines inside quoted strings
	// would let our naive line-count diverge from SF's record count).
	if rowsFromHeader > 0 {
		rows = rowsFromHeader
	}
	return rows, nextLoc, nil
}

// BulkQueryAlias is the alias-flavoured entry point that mirrors the
// rest of this package's "give me the org name, I'll find the client"
// shape. Most call sites use this; tests that want to inject a fake
// client construct one directly.
func BulkQueryAlias(ctx context.Context, orgAlias, soql string, out io.Writer, progress chan<- BulkQueryProgress) (BulkQueryResult, error) {
	c, err := RESTClient(orgAlias)
	if err != nil {
		return BulkQueryResult{}, err
	}
	return c.BulkQuery(ctx, soql, out, progress)
}

// BulkQueryRecords runs the SOQL via Bulk API 2.0 and returns the
// parsed records as a QueryResult — same shape REST queries return,
// so the /soql renderer doesn't need a Bulk-specific code path. Used
// by the editor's Bulk toggle (ctrl+b) to pull large result sets in
// one API call instead of N/2000.
//
// Caveats vs REST:
//   - All cell values come back as strings (Bulk CSV has no native
//     types). Numeric/date cells render as their string form; the
//     SOQL grid's formatCell already handles that gracefully.
//   - Nested relationships ("Account.Name") flatten as dotted column
//     headers — same shape SF returns, so Record.Field's dotted-path
//     traversal still works.
//   - Done is always true (Bulk always returns the full set).
//     TotalSize reports the parsed row count, since Bulk doesn't
//     surface SF's WHERE-clause total separately.
//
// Buffers the entire CSV in memory before parsing — fine up to ~1M
// rows on modern hardware. progress is forwarded to the underlying
// BulkQuery so the UI can show submit/poll/download stages.
func BulkQueryRecords(ctx context.Context, orgAlias, soql string, progress chan<- BulkQueryProgress) (QueryResult, error) {
	var buf bytes.Buffer
	if _, err := BulkQueryAlias(ctx, orgAlias, soql, &buf, progress); err != nil {
		return QueryResult{}, err
	}
	return parseBulkCSV(buf.Bytes())
}

// parseBulkCSV converts a Bulk API 2.0 CSV body into QueryResult
// shape. Empty bodies (zero matches) return an empty result.
func parseBulkCSV(body []byte) (QueryResult, error) {
	r := csv.NewReader(bytes.NewReader(body))
	r.ReuseRecord = false
	r.FieldsPerRecord = -1 // tolerate header/row width mismatches
	header, err := r.Read()
	if err == io.EOF {
		return QueryResult{Records: nil, Done: true}, nil
	}
	if err != nil {
		return QueryResult{}, fmt.Errorf("bulk csv header: %w", err)
	}
	records := make([]map[string]any, 0, 1024)
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return QueryResult{}, fmt.Errorf("bulk csv row %d: %w", len(records)+1, err)
		}
		rec := make(map[string]any, len(header))
		for i, name := range header {
			if i >= len(row) {
				break
			}
			rec[name] = row[i]
		}
		records = append(records, rec)
	}
	return QueryResult{Records: records, TotalSize: len(records), Done: true}, nil
}
