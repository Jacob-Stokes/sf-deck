package ui

// Saved-comparison serialization — converts an in-memory compareRun
// (snapshots + inventory) to/from the gzipped-JSON blob stored in
// devprojects.db (see internal/devproject/saved_comparisons.go).
//
// The store keeps the blob opaque; this is where the diff-domain types
// get marshalled. diff.Inventory.Errors carries a non-serializable
// `error`, so we project to a string-only persisted shape.

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/diff"
)

// comparePayload is the JSON shape inside the gzipped blob. snapA/snapB
// are the retrieved source (type → name → XML); Rows is the inventory.
// Errors are flattened to strings (the live error values don't persist
// and aren't needed once retrieval is done).
type comparePayload struct {
	Version int           `json:"version"`
	SnapA   diff.Snapshot `json:"snapA"`
	SnapB   diff.Snapshot `json:"snapB"`
	Rows    []diff.Row    `json:"rows"`
	Errors  []string      `json:"errors,omitempty"`
}

const comparePayloadVersion = 1

// serializeCompareRun gzips the run's snapshots + inventory into the
// blob stored alongside the scalar columns.
func serializeCompareRun(run *compareRun) ([]byte, error) {
	var errs []string
	for _, e := range run.Inv.Errors {
		errs = append(errs, fmt.Sprintf("%s (%s): %v", e.Type, e.Side, e.Err))
	}
	payload := comparePayload{
		Version: comparePayloadVersion,
		SnapA:   run.snapA,
		SnapB:   run.snapB,
		Rows:    run.Inv.Rows,
		Errors:  errs,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal comparison: %w", err)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(raw); err != nil {
		return nil, fmt.Errorf("gzip comparison: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}
	return buf.Bytes(), nil
}

// deserializeCompareRun rebuilds a compareRun (inventory phase, ready to
// browse offline) from a stored SavedComparison. The reconstructed run
// has snapA/snapB so drill-in diffs work with no API calls.
func deserializeCompareRun(sc devproject.SavedComparison) (*compareRun, error) {
	gz, err := gzip.NewReader(bytes.NewReader(sc.Blob))
	if err != nil {
		return nil, fmt.Errorf("gunzip comparison: %w", err)
	}
	defer gz.Close()
	raw, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("read comparison: %w", err)
	}
	var payload comparePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal comparison: %w", err)
	}
	run := &compareRun{
		Source: orgEndpoint(sc.Source),
		Target: orgEndpoint(sc.Target),
		Scope:  splitScope(sc.Scope),
		Method: parseCompareMethod(sc.Method),
		Phase:  comparePhaseInventory,
		snapA:  payload.SnapA,
		snapB:  payload.SnapB,
		Inv:    diff.Inventory{Rows: payload.Rows},
	}
	return run, nil
}

// splitScope parses the comma-joined scope column back to a slice.
func splitScope(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
