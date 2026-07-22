package sf

// TraceFlag management for anonymous-Apex log capture.
//
// Anonymous Apex executions only land in ApexLog when the running
// user has an *active* TraceFlag pointing at a DebugLevel during
// the call. Without one, executeAnonymous succeeds but no log row
// is ever created — exactly the "nothing comes up in log" case.
//
// Workbench / Apex Replay / VS Code's Salesforce Extensions all
// auto-set one when running anonymous Apex. We do the same:
//
//	1. EnsureTraceFlagForUser(userID) — find or create a DebugLevel
//	   with a sensible default + a TraceFlag covering userID for
//	   the next ~30 minutes. Idempotent: re-uses an existing
//	   active TraceFlag when one is already in effect.
//	2. The caller fires executeAnonymous.
//	3. FetchLatestApexLog reads the resulting row.
//
// We don't clean the TraceFlag up after the run — leaving it
// active for 30 minutes lets the user run a series of snippets
// without re-priming every time. The flag expires on its own.

import (
	"encoding/json"
	"fmt"
	"time"
)

// TraceFlagResult summarises what EnsureTraceFlagForUser did. Used
// only for telemetry / debug; callers normally ignore the return.
type TraceFlagResult struct {
	DebugLevelID string
	TraceFlagID  string
	Created      bool // true if a new TraceFlag was inserted; false if reused
}

// EnsureTraceFlagForUser ensures the given user has an active
// TraceFlag pointing at a sensible DebugLevel. If one's already
// active (ExpirationDate > now), returns it. Otherwise creates a
// DebugLevel (if none with the canonical name exists) + a fresh
// TraceFlag good for 30 minutes.
//
// "Sensible default" debug levels: ApexCode=DEBUG, Database=INFO,
// System=DEBUG, Validation=INFO, Visualforce=INFO, Workflow=INFO.
// Matches Inspector's default — DEBUG on the bits anonymous Apex
// usually wants to see (System.debug output, DML), INFO elsewhere.
func EnsureTraceFlagForUser(alias, userID string) (TraceFlagResult, error) {
	if userID == "" {
		return TraceFlagResult{}, fmt.Errorf("user id required")
	}
	c, err := RESTClient(alias)
	if err != nil {
		return TraceFlagResult{}, err
	}
	now := time.Now().UTC()

	// Step 1: check for an existing active TraceFlag on this user.
	soql := fmt.Sprintf(
		"SELECT Id, DebugLevelId, ExpirationDate FROM TraceFlag "+
			"WHERE TracedEntityId = '%s' AND LogType = 'USER_DEBUG' "+
			"AND ExpirationDate > %s ORDER BY ExpirationDate DESC LIMIT 1",
		sqlEscape(userID), now.Format("2006-01-02T15:04:05.000Z"))
	qres, err := c.QueryREST(soql, true)
	if err != nil {
		return TraceFlagResult{}, fmt.Errorf("look up TraceFlag: %w", err)
	}
	if len(qres.Records) > 0 {
		r := qres.Records[0]
		flagID, _ := r["Id"].(string)
		dlID, _ := r["DebugLevelId"].(string)
		return TraceFlagResult{
			DebugLevelID: dlID,
			TraceFlagID:  flagID,
			Created:      false,
		}, nil
	}

	// Step 2: find or create the DebugLevel.
	dlID, err := findOrCreateDebugLevel(c)
	if err != nil {
		return TraceFlagResult{}, err
	}

	// Step 3: create a TraceFlag valid for 30 minutes.
	expires := now.Add(30 * time.Minute)
	body, _ := json.Marshal(map[string]any{
		"TracedEntityId": userID,
		"DebugLevelId":   dlID,
		"LogType":        "USER_DEBUG",
		"StartDate":      now.Format("2006-01-02T15:04:05.000Z"),
		"ExpirationDate": expires.Format("2006-01-02T15:04:05.000Z"),
	})
	raw, err := c.post(c.ToolingPath("sobjects/TraceFlag"), body)
	if err != nil {
		return TraceFlagResult{}, fmt.Errorf("create TraceFlag: %w", err)
	}
	var created struct {
		ID      string `json:"id"`
		Success bool   `json:"success"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		return TraceFlagResult{}, fmt.Errorf("decode TraceFlag insert: %w", err)
	}
	if !created.Success || created.ID == "" {
		return TraceFlagResult{}, fmt.Errorf("TraceFlag insert reported failure")
	}
	return TraceFlagResult{
		DebugLevelID: dlID,
		TraceFlagID:  created.ID,
		Created:      true,
	}, nil
}

// findOrCreateDebugLevel returns the Id of our canonical DebugLevel,
// creating it on first use. We pick a stable DeveloperName ("sfdeck")
// so subsequent invocations re-use the row instead of polluting the
// org with one DebugLevel per session.
func findOrCreateDebugLevel(c *Client) (string, error) {
	const developerName = "sfdeck"
	soql := fmt.Sprintf(
		"SELECT Id FROM DebugLevel WHERE DeveloperName = '%s' LIMIT 1",
		developerName)
	qres, err := c.QueryREST(soql, true)
	if err != nil {
		return "", fmt.Errorf("look up DebugLevel: %w", err)
	}
	if len(qres.Records) > 0 {
		id, _ := qres.Records[0]["Id"].(string)
		if id != "" {
			return id, nil
		}
	}
	body, _ := json.Marshal(map[string]any{
		"DeveloperName": developerName,
		"MasterLabel":   "sf-deck",
		"ApexCode":      "DEBUG",
		"ApexProfiling": "INFO",
		"Callout":       "INFO",
		"Database":      "INFO",
		"System":        "DEBUG",
		"Validation":    "INFO",
		"Visualforce":   "INFO",
		"Workflow":      "INFO",
		"Nba":           "INFO",
	})
	raw, err := c.post(c.ToolingPath("sobjects/DebugLevel"), body)
	if err != nil {
		return "", fmt.Errorf("create DebugLevel: %w", err)
	}
	var created struct {
		ID      string `json:"id"`
		Success bool   `json:"success"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		return "", fmt.Errorf("decode DebugLevel insert: %w", err)
	}
	if !created.Success || created.ID == "" {
		return "", fmt.Errorf("DebugLevel insert reported failure")
	}
	return created.ID, nil
}
