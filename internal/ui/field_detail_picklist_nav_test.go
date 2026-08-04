package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// TestFieldDetailPicklistRowsNavigable pins that picklist value rows are
// now navigable (cursor can rest on them) and carry the value to yank —
// the fix for "cursor skips over picklist".
func TestFieldDetailPicklistRowsNavigable(t *testing.T) {
	f := sf.Field{
		Name: "Stage__c",
		PicklistValues: []sf.PicklistValue{
			{Label: "Open", Value: "open", Active: true},
			{Label: "Closed", Value: "closed", Active: true},
		},
	}
	rows := fieldDetailRows("Opportunity", f, "", true, 80)

	var valueRows []fieldDetailRow
	for _, r := range rows {
		if r.YankValue != "" {
			valueRows = append(valueRows, r)
		}
	}
	if len(valueRows) != 2 {
		t.Fatalf("expected 2 navigable picklist value rows, got %d", len(valueRows))
	}
	for _, r := range valueRows {
		if !r.Navigable {
			t.Errorf("picklist value row %q should be navigable", r.YankValue)
		}
		if r.ActionIdx != noAction {
			t.Errorf("picklist value row should be read-only (noAction), got ActionIdx=%d", r.ActionIdx)
		}
	}
	if valueRows[0].YankValue != "open" || valueRows[1].YankValue != "closed" {
		t.Errorf("value rows carry wrong values: %q, %q", valueRows[0].YankValue, valueRows[1].YankValue)
	}
}

// TestFieldDetailPicklistYanksLabelWhenNoValue: a value-less picklist
// entry falls back to the label as the yank value.
func TestFieldDetailPicklistYanksLabelWhenNoValue(t *testing.T) {
	f := sf.Field{
		Name:           "P__c",
		PicklistValues: []sf.PicklistValue{{Label: "OnlyLabel", Value: "", Active: true}},
	}
	rows := fieldDetailRows("X", f, "", true, 80)
	for _, r := range rows {
		if r.Navigable && r.YankValue != "" {
			if r.YankValue != "OnlyLabel" {
				t.Errorf("value-less entry should yank the label, got %q", r.YankValue)
			}
			return
		}
	}
	t.Fatal("no navigable value row found")
}
