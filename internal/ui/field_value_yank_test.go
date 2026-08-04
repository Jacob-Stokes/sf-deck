package ui

import (
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// TestFieldValueYankOptionsPicklist covers the four picklist formats
// plus that non-picklist value entries (formula/default/help/refs) only
// appear when the field has them.
func TestFieldValueYankOptionsPicklist(t *testing.T) {
	f := sf.Field{
		Name: "Stage__c",
		PicklistValues: []sf.PicklistValue{
			{Label: "Open", Value: "open", Active: true, DefaultValue: true},
			{Label: "Closed Won", Value: "closed_won", Active: true},
		},
	}
	opts := fieldValueYankOptions(f)
	byLabel := map[string]string{}
	for _, o := range opts {
		byLabel[o.Label] = o.Value
	}
	if got := byLabel["Picklist values (comma)"]; got != "open,closed_won" {
		t.Errorf("comma values = %q", got)
	}
	if got := byLabel["Picklist labels (comma)"]; got != "Open,Closed Won" {
		t.Errorf("comma labels = %q", got)
	}
	if got := byLabel["Picklist values (newline)"]; got != "open\nclosed_won" {
		t.Errorf("newline values = %q", got)
	}
	table := byLabel["Picklist table (Label/Value/Active/Default)"]
	if !strings.Contains(table, "Label\tValue\tActive\tDefault") {
		t.Errorf("table missing header: %q", table)
	}
	if !strings.Contains(table, "Open\topen\tyes\tyes") {
		t.Errorf("table missing default row: %q", table)
	}
	// A plain picklist has no formula/help/etc.
	for _, unexpected := range []string{"Formula", "Help text", "Reference target(s)"} {
		if _, ok := byLabel[unexpected]; ok {
			t.Errorf("plain picklist should not offer %q", unexpected)
		}
	}
}

// TestFieldValueYankOptionsOtherValues: formula, default, help, and
// references appear only when present.
func TestFieldValueYankOptionsOtherValues(t *testing.T) {
	f := sf.Field{
		Name:              "Amount_x2__c",
		CalculatedFormula: "Amount__c * 2",
		InlineHelpText:    "double the amount",
		ReferenceTo:       nil,
	}
	byLabel := map[string]string{}
	for _, o := range fieldValueYankOptions(f) {
		byLabel[o.Label] = o.Value
	}
	if byLabel["Formula"] != "Amount__c * 2" {
		t.Errorf("formula = %q", byLabel["Formula"])
	}
	if byLabel["Help text"] != "double the amount" {
		t.Errorf("help = %q", byLabel["Help text"])
	}
	if _, ok := byLabel["Reference target(s)"]; ok {
		t.Error("no references → should not offer reference target")
	}

	// Reference field
	ref := sf.Field{Name: "AccountId", Type: "reference", ReferenceTo: []string{"Account"}}
	byLabel = map[string]string{}
	for _, o := range fieldValueYankOptions(ref) {
		byLabel[o.Label] = o.Value
	}
	if byLabel["Reference target(s)"] != "Account" {
		t.Errorf("reference = %q", byLabel["Reference target(s)"])
	}
}

// TestFieldValueYankOptionsEmpty: a plain text field with no special
// values yields no options (so "Field values…" isn't offered).
func TestFieldValueYankOptionsEmpty(t *testing.T) {
	f := sf.Field{Name: "Name", Type: "string", Label: "Name"}
	if opts := fieldValueYankOptions(f); len(opts) != 0 {
		t.Errorf("plain field should yield no value options, got %d: %+v", len(opts), opts)
	}
}
