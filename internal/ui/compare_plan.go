package ui

import (
	"fmt"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/diff"
)

// compareObjectRootedTypeOrder is the set of buckets produced by one
// CustomObject readMetadata lane. The scope picker hides child XML types
// because selecting CustomObject represents the full object surface.
var compareObjectRootedTypeOrder = []string{
	"CustomObject",
	"CustomField",
	"ValidationRule",
	"RecordType",
	"CompactLayout",
	"WebLink",
	"ListView",
	"FieldSet",
	"Index",
	"BusinessProcess",
	"SharingReason",
}

var compareObjectRootedTypes = func() map[string]bool {
	m := map[string]bool{}
	for _, t := range compareObjectRootedTypeOrder {
		m[t] = true
	}
	return m
}()

// compareObjectChildTypes are served by the parent CustomObject retrieve
// (their bodies are extracted from the object's XML), not standalone.
var compareObjectChildTypes = func() map[string]bool {
	m := map[string]bool{}
	for _, t := range compareObjectRootedTypeOrder {
		if t != "CustomObject" {
			m[t] = true
		}
	}
	return m
}()

type comparePlan struct {
	Method compareMethod
	Scope  []string

	Providers          []diff.Provider
	UnsupportedTooling []string
	ObjectTypes        []string
	PerTypes           []string

	EstimatedCalls int
}

func buildComparePlan(scope []string, method compareMethod) comparePlan {
	plan := comparePlan{
		Method: method,
		Scope:  append([]string(nil), scope...),
	}
	if method == compareMethodTooling {
		plan.Providers, plan.UnsupportedTooling = toolingProvidersForScope(scope)
		plan.EstimatedCalls = len(plan.Providers)
		if plan.EstimatedCalls < 1 {
			plan.EstimatedCalls = 1
		}
		return plan
	}

	plan.ObjectTypes, plan.PerTypes = splitObjectChildScope(scope)
	plan.EstimatedCalls = estimateSnapshotPlanCalls(plan)
	return plan
}

func (p comparePlan) validate() error {
	if p.Method != compareMethodTooling || len(p.UnsupportedTooling) == 0 {
		return nil
	}
	return fmt.Errorf("tooling supports only ApexClass and ApexTrigger; switch to Auto/Metadata API or remove %s",
		shortTypeList(p.UnsupportedTooling))
}

func estimateSnapshotPlanCalls(plan comparePlan) int {
	apex, soapTypes := 0, 0
	for _, label := range plan.PerTypes {
		if apexCompareTypes[label] {
			apex++ // one bulk Tooling body query per Apex type
		} else {
			soapTypes++
		}
	}

	// This mirrors the current runner: every non-Apex type performs one
	// listMetadata call, then at least one readMetadata batch. Component
	// counts add more readMetadata calls, so this remains a floor.
	listing := soapTypes
	retrieve := soapTypes
	objects := 0
	if len(plan.ObjectTypes) > 0 {
		listing++   // list CustomObject names
		objects = 5 // rough readMetadata floor for the object lane
	}
	est := listing + retrieve + apex + objects
	if est < 1 {
		est = 1
	}
	return est
}

func toolingProvidersForScope(scope []string) ([]diff.Provider, []string) {
	all := providersForMethod(compareMethodTooling)
	if len(scope) == 0 {
		return all, nil
	}
	want := map[string]bool{}
	for _, s := range scope {
		want[s] = true
	}
	var out []diff.Provider
	for _, p := range all {
		if want[p.TypeLabel()] {
			out = append(out, p)
		}
	}
	var unsupported []string
	for _, s := range scope {
		if _, ok := toolingCompareTypes[s]; !ok {
			unsupported = append(unsupported, s)
		}
	}
	return out, unsupported
}

func shortTypeList(types []string) string {
	if len(types) == 0 {
		return "the unsupported types"
	}
	const max = 3
	if len(types) <= max {
		return strings.Join(types, ", ")
	}
	return strings.Join(types[:max], ", ") + fmt.Sprintf(" (+%d more)", len(types)-max)
}

func hasType(types []string, want string) bool {
	for _, t := range types {
		if t == want {
			return true
		}
	}
	return false
}
