package sf

import "time"

// DeployRow is a trimmed row from the DeployRequest tooling-API SOQL.
type DeployRow struct {
	ID                 string
	Status             string
	CreatedByName      string
	CreatedDate        string
	StartDate          string
	CompletedDate      string
	CheckOnly          bool   // true = validation only, nothing saved
	Type               string // "Api" / change-set deploys carry ChangeSetName
	ChangeSetName      string
	TestLevel          string
	ErrorMessage       string
	StateDetail        string
	CanceledByName     string
	ComponentsDeployed int
	ComponentsTotal    int
	ComponentErrors    int
	TestsTotal         int
	TestsCompleted     int
	TestErrors         int
}

// Field implements the structural query.Row interface for the deploys
// chip domain. Names match the DeployRequest tooling-SOQL columns,
// with short conveniences for the wizard.
func (r DeployRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return r.ID, true
	case "Status":
		return r.Status, true
	case "CheckOnly", "Validation":
		return r.CheckOnly, true
	case "Type":
		return r.Type, true
	case "ChangeSetName":
		return r.ChangeSetName, true
	case "TestLevel":
		return r.TestLevel, true
	case "ErrorMessage":
		return r.ErrorMessage, true
	case "CreatedBy", "CreatedBy.Name", "CreatedByName", "By":
		return r.CreatedByName, true
	case "CreatedDate":
		return r.CreatedDate, true
	case "CompletedDate":
		return r.CompletedDate, true
	case "NumberComponentsTotal", "ComponentsTotal":
		return r.ComponentsTotal, true
	case "NumberComponentErrors", "ComponentErrors":
		return r.ComponentErrors, true
	case "NumberTestsTotal", "TestsTotal":
		return r.TestsTotal, true
	case "NumberTestErrors", "TestErrors":
		return r.TestErrors, true
	}
	return nil, false
}

// InFlight reports whether the deploy hasn't reached a terminal
// status yet — drives the /deploys live-watch poller.
func (r DeployRow) InFlight() bool {
	switch r.Status {
	// "Finalizing" is the (newer) phase between tests completing and
	// Succeeded. It MUST count as in-flight: the deploys list only
	// re-polls rows that are in flight (delta refresh skips rows by
	// CreatedDate), so a row cached mid-Finalizing would otherwise be
	// stuck at that status forever — across refresh AND restart.
	// Field bug 2026-07-18, same class as the 2026-06-12 InProgress one.
	case "Pending", "InProgress", "Canceling", "Finalizing":
		return true
	}
	return false
}

// Duration returns the wall-clock Start→Completed span, or 0 when
// either timestamp is missing (still running, or never started).
func (r DeployRow) Duration() time.Duration {
	if r.StartDate == "" || r.CompletedDate == "" {
		return 0
	}
	s, err1 := time.Parse("2006-01-02T15:04:05.000-0700", r.StartDate)
	c, err2 := time.Parse("2006-01-02T15:04:05.000-0700", r.CompletedDate)
	if err1 != nil || err2 != nil {
		return 0
	}
	d := c.Sub(s)
	if d < 0 {
		return 0
	}
	return d
}

const deploySOQLFields = `Id, Status, CheckOnly, Type, ChangeSetName, TestLevel, ` +
	`StartDate, CompletedDate, CreatedDate, CreatedBy.Name, CanceledBy.Name, ` +
	`ErrorMessage, StateDetail, ` +
	`NumberComponentsDeployed, NumberComponentsTotal, NumberComponentErrors, ` +
	`NumberTestsTotal, NumberTestsCompleted, NumberTestErrors`

// RecentDeploys runs a read-only SOQL against the Tooling API for recent
// DeployRequest rows.
func RecentDeploys(target string, limit int) ([]DeployRow, error) {
	return RecentDeploysSince(target, limit, "")
}

// RecentDeploysSince is the delta-refresh variant: when since is a
// valid ISO-8601 timestamp, only rows with CreatedDate > since are
// returned. Callers merge the delta into the existing slice rather
// than re-fetching everything. Pass "" to get the full window.
func RecentDeploysSince(target string, limit int, since string) ([]DeployRow, error) {
	if limit <= 0 {
		limit = 10
	}
	soql := `SELECT ` + deploySOQLFields + ` FROM DeployRequest`
	if since != "" {
		// Salesforce SOQL accepts ISO-8601 literals for dateTime fields
		// with no quotes. If the caller passed a malformed value we
		// fall back to the full query.
		soql += " WHERE CreatedDate > " + since
	}
	soql += ` ORDER BY CreatedDate DESC`
	soql += pagingLimit(limit)
	q, err := Query(target, soql, true)
	if err != nil {
		return nil, err
	}
	out := make([]DeployRow, 0, len(q.Records))
	for _, r := range q.Records {
		out = append(out, deployRowFromRecord(r))
	}
	return out, nil
}

// RefreshDeploys re-queries the given deploy IDs and returns fresh
// rows keyed by ID. Used by the live-watch poller to update in-flight
// rows without disturbing the rest of the cached window.
func RefreshDeploys(target string, ids []string) (map[string]DeployRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	in := ""
	for i, id := range ids {
		if i > 0 {
			in += ", "
		}
		in += "'" + sqlEscape(id) + "'"
	}
	soql := `SELECT ` + deploySOQLFields + ` FROM DeployRequest WHERE Id IN (` + in + `)`
	q, err := Query(target, soql, true)
	if err != nil {
		return nil, err
	}
	out := make(map[string]DeployRow, len(q.Records))
	for _, r := range q.Records {
		row := deployRowFromRecord(r)
		out[row.ID] = row
	}
	return out, nil
}

func deployRowFromRecord(r map[string]any) DeployRow {
	row := DeployRow{
		ID:            asString(r["Id"]),
		Status:        asString(r["Status"]),
		CreatedDate:   asString(r["CreatedDate"]),
		StartDate:     asString(r["StartDate"]),
		CompletedDate: asString(r["CompletedDate"]),
		Type:          asString(r["Type"]),
		ChangeSetName: asString(r["ChangeSetName"]),
		TestLevel:     asString(r["TestLevel"]),
		ErrorMessage:  asString(r["ErrorMessage"]),
		StateDetail:   asString(r["StateDetail"]),
	}
	if b, ok := r["CheckOnly"].(bool); ok {
		row.CheckOnly = b
	}
	if u, ok := r["CreatedBy"].(map[string]any); ok {
		row.CreatedByName = asString(u["Name"])
	}
	if u, ok := r["CanceledBy"].(map[string]any); ok {
		row.CanceledByName = asString(u["Name"])
	}
	row.ComponentsDeployed = asInt(r["NumberComponentsDeployed"])
	row.ComponentsTotal = asInt(r["NumberComponentsTotal"])
	row.ComponentErrors = asInt(r["NumberComponentErrors"])
	row.TestsTotal = asInt(r["NumberTestsTotal"])
	row.TestsCompleted = asInt(r["NumberTestsCompleted"])
	row.TestErrors = asInt(r["NumberTestErrors"])
	return row
}

func pagingLimit(n int) string {
	return " LIMIT " + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return sign + string(buf[i:])
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func asInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	}
	return 0
}
