package ui

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

type comparePayload struct {
	Version int           `json:"version"`
	SnapA   diff.Snapshot `json:"snapA"`
	SnapB   diff.Snapshot `json:"snapB"`
	Rows    []diff.Row    `json:"rows"`
	Errors  []string      `json:"errors,omitempty"`
}

const comparePayloadVersion = 1

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
