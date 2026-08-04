package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/diff"
)

func TestSplitObjectChildScope(t *testing.T) {
	obj, per := splitObjectChildScope([]string{
		"ApexClass", "CustomField", "Flow", "ValidationRule", "CustomObject", "RecordType", "Layout",
	})
	wantObj := compareObjectRootedTypeOrder
	wantPer := []string{"ApexClass", "Flow", "Layout"}
	if !reflect.DeepEqual(obj, wantObj) {
		t.Errorf("objChildren = %v, want %v", obj, wantObj)
	}
	if !reflect.DeepEqual(per, wantPer) {
		t.Errorf("perType = %v, want %v", per, wantPer)
	}

	// No object-child types → all per-type, nil objChildren.
	obj, per = splitObjectChildScope([]string{"ApexClass", "Flow"})
	if len(obj) != 0 {
		t.Errorf("objChildren should be empty, got %v", obj)
	}
	if !reflect.DeepEqual(per, []string{"ApexClass", "Flow"}) {
		t.Errorf("perType = %v", per)
	}

	// Explicit child-only scopes stay narrow; selecting CustomObject is what
	// expands to the full object-rooted surface.
	obj, per = splitObjectChildScope([]string{"CustomField", "Flow"})
	if !reflect.DeepEqual(obj, []string{"CustomField"}) {
		t.Errorf("child-only objChildren = %v, want [CustomField]", obj)
	}
	if !reflect.DeepEqual(per, []string{"Flow"}) {
		t.Errorf("child-only perType = %v, want [Flow]", per)
	}
}

func TestCustomObjectPlanRecordsChildBuckets(t *testing.T) {
	plan := buildComparePlan([]string{"CustomObject"}, compareMethodAuto)
	for _, want := range []string{"CustomObject", "CustomField", "FieldSet", "ListView", "SharingReason"} {
		if !hasType(plan.ObjectTypes, want) {
			t.Fatalf("CustomObject plan missing %s: %v", want, plan.ObjectTypes)
		}
	}

	run := &compareRun{
		Phase:    comparePhaseRetrieving,
		snapA:    diff.Snapshot{},
		hashA:    diff.Snapshot{},
		snapB:    diff.Snapshot{},
		hashB:    diff.Snapshot{},
		Progress: map[string]retrieveProgress{},
		expected: len(plan.ObjectTypes) * 2,
	}
	d := &orgData{}
	d.Run = run
	m := &Model{}
	_ = m.applyCompareObjectsDone(d, compareObjectsDoneMsg{
		Side:    "source",
		InScope: plan.ObjectTypes,
		Buckets: map[string]map[string]string{
			"CustomObject": {"Acct__c": "<fullName>Acct__c</fullName>"},
			"FieldSet":     {"Acct__c.FS1": "<fullName>FS1</fullName>"},
			"ListView":     {"Acct__c.LV1": "<fullName>LV1</fullName>"},
		},
	})
	if run.hashA["FieldSet"]["Acct__c.FS1"] == "" {
		t.Fatalf("FieldSet bucket was not recorded: %#v", run.hashA["FieldSet"])
	}
	if run.hashA["ListView"]["Acct__c.LV1"] == "" {
		t.Fatalf("ListView bucket was not recorded: %#v", run.hashA["ListView"])
	}
	if _, ok := run.Progress[progressKey("source", "SharingReason")]; !ok {
		t.Fatalf("expanded child type progress missing; progress=%v", run.Progress)
	}
}

func TestShortErr(t *testing.T) {
	cases := []struct {
		in   error
		want string
	}{
		{nil, "failed"},
		{errStr("readMetadata ActionPlanTemplate: INVALID_CROSS_REFERENCE_KEY: invalid cross reference id"),
			"INVALID_CROSS_REFERENCE_KEY: invalid cross reference id"},
		{errStr("listMetadata: UNKNOWN_EXCEPTION: boom"), "UNKNOWN_EXCEPTION: boom"},
		{errStr("plain message no prefix"), "plain message no prefix"},
	}
	for _, c := range cases {
		if got := shortErr(c.in); got != c.want {
			t.Errorf("shortErr(%v) = %q, want %q", c.in, got, c.want)
		}
	}
	// Long messages are truncated with an ellipsis.
	// Printable padding — the truncator is display-width-aware now,
	// and 200 NUL bytes are zero display cells (the old byte-based
	// version "passed" on them by accident).
	long := errStr("readMetadata X: " + strings.Repeat("x", 200))
	if got := shortErr(long); len([]rune(got)) > 60 {
		t.Errorf("shortErr did not truncate: len=%d", len([]rune(got)))
	}
}

func TestClampRetrieveScroll(t *testing.T) {
	cases := []struct {
		off, total, window, want int
	}{
		{0, 10, 20, 0},   // fits in window → top
		{5, 10, 20, 0},   // fits → forced to top
		{-3, 100, 20, 0}, // negative → 0
		{10, 100, 20, 10},
		{200, 100, 20, 80}, // past end → max (total-window)
	}
	for _, c := range cases {
		if got := clampRetrieveScroll(c.off, c.total, c.window); got != c.want {
			t.Errorf("clampRetrieveScroll(%d,%d,%d) = %d, want %d", c.off, c.total, c.window, got, c.want)
		}
	}
}

type errStr string

func (e errStr) Error() string { return string(e) }
