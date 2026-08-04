package sf

import (
	"fmt"
	"time"
)

// flow_interviews.go — reader for the FlowInterview standard object.
//
// FlowInterview holds every in-flight or stuck flow run: interviews
// waiting on a Pause element, and — more usefully — interviews that
// ERRORED mid-run and are sitting there invisible. The web UI buries
// these under Setup > Paused Flow Interviews; nobody sees an errored
// interview until a user complains something didn't happen. Surfacing
// them as a live, status-filterable list (with the CurrentElement the
// run died on) turns a silent backlog into something you can triage.

// FlowInterviewRow is one in-flight / paused / errored flow run.
type FlowInterviewRow struct {
	ID          string
	Label       string    // InterviewLabel — flow name + start timestamp
	Status      string    // InterviewStatus: Started / Paused / Error / …
	Element     string    // CurrentElement — where the run is (or died)
	PauseLabel  string    // PauseLabel — set when paused at a named Pause
	CreatedByID string    // starter's User Id (for the o → user drill)
	CreatedBy   string    // starter's display name
	CreatedDate time.Time // when the interview began
}

// Field implements query.Row so interview rows flow through the generic
// list/search/sort engine.
func (r FlowInterviewRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return r.ID, true
	case "InterviewLabel", "Label":
		return r.Label, true
	case "InterviewStatus", "Status":
		return r.Status, true
	case "CurrentElement", "Element":
		return r.Element, true
	case "PauseLabel":
		return r.PauseLabel, true
	case "CreatedBy", "CreatedBy.Name", "CreatedByName":
		return r.CreatedBy, true
	case "CreatedDate":
		return r.CreatedDate, true
	}
	return nil, false
}

// Targets: open the starter's user record when known, then the Paused
// Flow Interviews Setup page (where errored/paused interviews live).
func (r FlowInterviewRow) Targets() []OpenTarget {
	t := []OpenTarget{
		{ID: "setup", Label: "Paused Flow Interviews (Setup)",
			Path: "/lightning/setup/PausedFlowInterviews/home"},
	}
	if r.CreatedByID != "" {
		t = append([]OpenTarget{{
			ID: "user", Label: "Started by — user detail",
			Path: "/lightning/r/User/" + r.CreatedByID + "/view",
		}}, t...)
	}
	return t
}

// YankTargets exposes the interview label (the flow + when it ran), the
// element it's stuck on, and the row Id.
func (r FlowInterviewRow) YankTargets() []YankTarget {
	var ts []YankTarget
	if r.Label != "" {
		ts = append(ts, YankTarget{ID: "label", Label: "Interview label", Value: r.Label, Shortcut: "l"})
	}
	if r.Element != "" {
		ts = append(ts, YankTarget{ID: "element", Label: "Current element", Value: r.Element, Shortcut: "e"})
	}
	if r.ID != "" {
		ts = append(ts, YankTarget{ID: "id", Label: "Id", Value: r.ID, Shortcut: "i"})
	}
	return ts
}

// FlowInterviews returns in-flight / paused / errored flow interviews,
// newest first. Read-only. Capped at cap rows (0 → sensible default).
func FlowInterviews(target string, cap int) ([]FlowInterviewRow, error) {
	if cap <= 0 {
		cap = 1000
	}
	return queryRows(target,
		fmt.Sprintf(
			"SELECT Id, InterviewLabel, InterviewStatus, CurrentElement, "+
				"PauseLabel, CreatedDate, CreatedById, CreatedBy.Name "+
				"FROM FlowInterview ORDER BY CreatedDate DESC LIMIT %d", cap),
		false, mapFlowInterviewRow)
}

func mapFlowInterviewRow(r map[string]any) FlowInterviewRow {
	return FlowInterviewRow{
		ID:          asString(r["Id"]),
		Label:       asString(r["InterviewLabel"]),
		Status:      asString(r["InterviewStatus"]),
		Element:     asString(r["CurrentElement"]),
		PauseLabel:  asString(r["PauseLabel"]),
		CreatedByID: asString(r["CreatedById"]),
		CreatedBy:   relationName(r, "CreatedBy"),
		CreatedDate: parseSFDate(r["CreatedDate"]),
	}
}
