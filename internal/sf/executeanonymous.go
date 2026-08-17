package sf

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ExecuteAnonymousResult is the parsed Tooling-API response. Matches
// the JSON shape Salesforce returns from /tooling/executeAnonymous —
// boolean ok flags plus problem strings + line/column on failure.
type ExecuteAnonymousResult struct {
	Compiled         bool          `json:"compiled"`
	CompileProblem   string        `json:"compileProblem"`
	Success          bool          `json:"success"`
	ExceptionMessage string        `json:"exceptionMessage"`
	ExceptionStack   string        `json:"exceptionStackTrace"`
	Line             int           `json:"line"`
	Column           int           `json:"column"`
	Body             string        `json:"-"`
	LogID            string        `json:"-"`
	LogBody          string        `json:"-"`
	Took             time.Duration `json:"-"`
}

// ExecuteAnonymous runs the given Apex body against the org. Pure
// execute — no log capture; the caller composes that on top via
// FetchLatestApexLog when needed.
func (c *Client) ExecuteAnonymous(body string) (ExecuteAnonymousResult, error) {
	start := time.Now()
	path := c.ToolingPath("executeAnonymous")
	q := url.Values{}
	q.Set("anonymousBody", body)
	raw, err := c.get(path, q)
	took := time.Since(start)
	if err != nil {
		return ExecuteAnonymousResult{Body: body, Took: took}, err
	}
	var res ExecuteAnonymousResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return ExecuteAnonymousResult{Body: body, Took: took},
			fmt.Errorf("decode executeAnonymous: %w", err)
	}
	res.Body = body
	res.Took = took
	return res, nil
}

// ExecuteAnonymousAlias is the alias-flavoured entry point that
// matches the rest of this package's "give me the org name" shape.
func ExecuteAnonymousAlias(alias, body string) (ExecuteAnonymousResult, error) {
	c, err := RESTClient(alias)
	if err != nil {
		return ExecuteAnonymousResult{}, err
	}
	return c.ExecuteAnonymous(body)
}

// FetchLatestApexLog returns the body of the most recent ApexLog row
// for the running user — typically used after an executeAnonymous
// call to surface the debug output. Returns ("", "", nil) when no
// log exists (trace flag wasn't active, log expired, etc.) so
// callers can treat "no log" as a normal case.
//
// userID is the running user's 005… Id; the SOQL filters to logs the
// user themselves authored so a busy org's log table doesn't bury
// the one we want.
//
// since narrows further: only consider logs whose StartTime is at or
// after that instant. Pass the timestamp from just before the
// executeAnonymous call so we don't accidentally pick up a stale log
// from a prior run.
func FetchLatestApexLog(alias, userID string, since time.Time) (logID, body string, err error) {
	c, err := RESTClient(alias)
	if err != nil {
		return "", "", err
	}
	where := []string{}
	if userID != "" {
		where = append(where, "LogUserId = '"+sqlEscape(userID)+"'")
	}
	if !since.IsZero() {
		where = append(where, "StartTime >= "+since.UTC().Format("2006-01-02T15:04:05.000Z"))
	}
	soql := "SELECT Id FROM ApexLog"
	if len(where) > 0 {
		soql += " WHERE " + strings.Join(where, " AND ")
	}
	soql += " ORDER BY StartTime DESC LIMIT 1"
	qres, err := c.QueryREST(soql, true)
	if err != nil {
		return "", "", fmt.Errorf("query ApexLog: %w", err)
	}
	if len(qres.Records) == 0 {
		return "", "", nil
	}
	id, _ := qres.Records[0]["Id"].(string)
	if id == "" {
		return "", "", nil
	}
	bodyBytes, err := c.get(c.ToolingPath("sobjects/ApexLog/"+id+"/Body"), nil)
	if err != nil {
		return id, "", fmt.Errorf("fetch ApexLog body: %w", err)
	}
	return id, string(bodyBytes), nil
}

var _ = strconv.Itoa
