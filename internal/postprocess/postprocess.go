package postprocess

// Post-processor pipeline applied to Salesforce-issued xlsx exports
// before we hand the file to the user. Each transform takes the open
// workbook + a context and mutates the workbook in place; the runner
// composes them in order.
//
// Pattern is intentionally tiny so adding a new transform is a single
// file under this package plus a registration in the transform table.
// See research/reports-feature-research.md "Native xlsx post-processors"
// for the broader plan + transform candidates.

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

// Context carries the bits a transform needs that aren't in the
// workbook itself — instance URL for hyperlinks, the EntityDefinition
// prefix map for SF-Id detection, the report metadata, etc. Add fields
// here as new transforms need them; existing transforms ignore the
// fields they don't read.
type Context struct {
	// InstanceURL is the org's Lightning instance, no trailing slash
	// (e.g. https://acme.lightning.force.com). Used to build
	// HYPERLINK formulas.
	InstanceURL string
	// PrefixToSObject maps a 3-char Salesforce KeyPrefix to the
	// sObject API name (e.g. "001" → "Account"). Built from the
	// EntityDefinition cache the UI already keeps. nil disables the
	// URL post-processor's SF-Id detection.
	PrefixToSObject map[string]string
	// ReportName is the saved report's Name. Used as the default
	// xlsx filename if the runner falls back to one.
	ReportName string
}

// Transform is the unit of post-processing. Implementations should
// be idempotent within a single workbook so re-running the same
// pipeline doesn't double-apply.
type Transform interface {
	// ID is the stable identifier used in settings + the chooser
	// modal. Lowercase, single word.
	ID() string
	// Label is the human-readable name shown in the chooser.
	Label() string
	// Apply mutates the workbook in place. Returning an error stops
	// the pipeline (downstream transforms won't run). Safe to no-op
	// when the workbook doesn't have what the transform needs.
	Apply(wb *excelize.File, ctx Context) error
}

// Run reads xlsx bytes, applies the requested transforms in order,
// and returns the rewritten xlsx bytes. Empty transforms slice or
// nil context fields are fine — a zero-transform run round-trips
// the bytes through excelize but leaves them functionally identical.
func Run(in []byte, transforms []Transform, ctx Context) ([]byte, error) {
	wb, err := excelize.OpenReader(bytesReader(in))
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer wb.Close()
	for _, t := range transforms {
		if err := t.Apply(wb, ctx); err != nil {
			return nil, fmt.Errorf("transform %s: %w", t.ID(), err)
		}
	}
	buf, err := wb.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write xlsx: %w", err)
	}
	return buf.Bytes(), nil
}

// All returns every registered transform. UI surfaces (chooser modal,
// CLI flag parsing) iterate this list to discover available options.
// Order matches the recommended display order — most useful first.
func All() []Transform {
	return []Transform{
		&urlTransform{},
		&DetailsifyTransform{},
		&StripSummaryTransform{},
		&StripFormattingTransform{},
	}
}

// ByIDs filters All() to the transforms with matching IDs, preserving
// the order of `ids`. Unknown ids are silently skipped — caller can
// detect by comparing input vs output length if that matters.
func ByIDs(ids []string) []Transform {
	all := All()
	byID := map[string]Transform{}
	for _, t := range all {
		byID[t.ID()] = t
	}
	out := make([]Transform, 0, len(ids))
	for _, id := range ids {
		if t, ok := byID[id]; ok {
			out = append(out, t)
		}
	}
	return out
}
