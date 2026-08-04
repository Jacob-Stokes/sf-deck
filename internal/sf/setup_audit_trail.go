package sf

import (
	"fmt"
	"time"
)

// setup_audit_trail.go — reader for the SetupAuditTrail standard object.
//
// Salesforce records every Setup change (field edits, permset/FLS
// changes, deploys, deactivations, login-as, …) in SetupAuditTrail and
// keeps ~180 days. It's queryable via SOQL but almost nobody knows it —
// the web UI only offers a CSV export buried in Setup. Surfacing it as
// a live, sortable, searchable list answers the "what changed in this
// org and who did it?" question with no good keyboard-driven answer
// anywhere else.

// SetupAuditRow is one Setup change event.
type SetupAuditRow struct {
	ID          string
	Action      string    // machine action code, e.g. "PermSetGroupFlsChanged"
	Section     string    // Setup section, e.g. "Permission Set Group"
	Display     string    // the human-readable sentence describing the change
	CreatedByID string    // actor User Id (for the o → user-detail drill)
	CreatedBy   string    // actor display name
	Delegate    string    // DelegateUser — set when someone acted "as" the user
	CreatedDate time.Time // when the change happened
}

// Field implements query.Row so audit rows flow through the generic
// list/search/sort engine. Names mirror the SOQL fields.
func (r SetupAuditRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return r.ID, true
	case "Action":
		return r.Action, true
	case "Section":
		return r.Section, true
	case "Display":
		return r.Display, true
	case "CreatedBy", "CreatedBy.Name", "CreatedByName":
		return r.CreatedBy, true
	case "DelegateUser", "Delegate":
		return r.Delegate, true
	case "CreatedDate":
		return r.CreatedDate, true
	}
	return nil, false
}

// Targets: open the actor's user record (when known) first, then the
// Setup > Security > View Setup Audit Trail page.
func (r SetupAuditRow) Targets() []OpenTarget {
	t := []OpenTarget{
		{ID: "audit", Label: "View Setup Audit Trail (Setup)",
			Path: "/lightning/setup/SecurityEvents/home"},
	}
	if r.CreatedByID != "" {
		t = append([]OpenTarget{{
			ID: "user", Label: "Actor — user detail",
			Path: "/lightning/r/User/" + r.CreatedByID + "/view",
		}}, t...)
	}
	return t
}

// YankTargets exposes the change sentence (the thing you'd paste into a
// ticket) plus the actor and the row Id.
func (r SetupAuditRow) YankTargets() []YankTarget {
	var ts []YankTarget
	if r.Display != "" {
		ts = append(ts, YankTarget{ID: "change", Label: "Change description", Value: r.Display, Shortcut: "c"})
	}
	if r.CreatedBy != "" {
		ts = append(ts, YankTarget{ID: "actor", Label: "Actor", Value: r.CreatedBy, Shortcut: "a"})
	}
	if r.ID != "" {
		ts = append(ts, YankTarget{ID: "id", Label: "Id", Value: r.ID, Shortcut: "i"})
	}
	return ts
}

// SetupAuditTrail returns recent Setup changes for the org, newest
// first. Read-only. Capped at cap rows (0 → a sensible default) so a
// busy org doesn't pull the whole 180-day window on first paint.
func SetupAuditTrail(target string, cap int) ([]SetupAuditRow, error) {
	if cap <= 0 {
		cap = 1000
	}
	c, err := RESTClient(target)
	if err != nil {
		return nil, err
	}
	soql := fmt.Sprintf(
		"SELECT Id, Action, Section, Display, CreatedDate, CreatedById, "+
			"CreatedBy.Name, DelegateUser FROM SetupAuditTrail "+
			"ORDER BY CreatedDate DESC LIMIT %d", cap)
	q, err := c.QueryREST(soql, false)
	if err != nil {
		return nil, fmt.Errorf("list setup audit trail: %w", err)
	}
	out := make([]SetupAuditRow, 0, len(q.Records))
	for _, r := range q.Records {
		out = append(out, SetupAuditRow{
			ID:          asString(r["Id"]),
			Action:      asString(r["Action"]),
			Section:     asString(r["Section"]),
			Display:     asString(r["Display"]),
			CreatedByID: asString(r["CreatedById"]),
			CreatedBy:   relationName(r, "CreatedBy"),
			Delegate:    asString(r["DelegateUser"]),
			CreatedDate: parseSFDate(r["CreatedDate"]),
		})
	}
	return out, nil
}
