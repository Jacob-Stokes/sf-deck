package sf

import "testing"

func TestFlowInterviewRowFieldAndYank(t *testing.T) {
	r := FlowInterviewRow{
		ID:          "0Fo1",
		Label:       "Carrier Onboarding Processor",
		Status:      "Error",
		Element:     "GetObjectType",
		CreatedByID: "005DM0000000001AAA",
		CreatedBy:   "Demo Author",
	}

	// Columns the interviews surface renders must resolve via Field().
	for _, name := range []string{"InterviewStatus", "InterviewLabel", "CurrentElement", "CreatedBy.Name", "CreatedDate"} {
		if _, ok := r.Field(name); !ok {
			t.Errorf("Field(%q) not resolvable", name)
		}
	}

	ys := r.YankTargets()
	var hasElement bool
	for _, y := range ys {
		if y.ID == "element" && y.Value == "GetObjectType" {
			hasElement = true
		}
	}
	if !hasElement {
		t.Errorf("expected an 'element' yank target, got %+v", ys)
	}

	ts := r.Targets()
	if len(ts) == 0 || ts[0].ID != "user" {
		t.Errorf("Targets should lead with the starter user link, got %+v", ts)
	}

	bare := FlowInterviewRow{ID: "0Fo2", Label: "x"}.Targets()
	if len(bare) != 1 || bare[0].ID != "setup" {
		t.Errorf("no-starter row should offer only the setup target, got %+v", bare)
	}
}
