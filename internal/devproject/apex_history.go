package devproject

// Anonymous-Apex execution history. Every run from /exec lands here
// scoped to (org_user, executed_at). Same shape + invariants as
// soql_history with execute-anonymous-specific result columns
// (compiled / success / compile_problem / exception_message / line /
// column / log body).
//
// History rows are NOT linked to saved_apex by ID — a saved snippet
// and a typed-by-hand one produce equivalent history rows. The Body
// is the source of truth; re-saving from history pulls the body
// into the editor and CreateSavedApex from there.

import (
	"fmt"
	"time"
)

// ApexHistoryEntry is one execution log row.
type ApexHistoryEntry struct {
	ID               int64
	OrgUser          string
	Body             string
	ExecutedAt       time.Time
	DurationMs       int
	Compiled         bool
	Success          bool
	CompileProblem   string
	ExceptionMessage string
	Line             int
	Column           int
	LogID            string
	LogBody          string
}

// Field implements query.Row so chip predicates can filter the
// history list. Mirrors SOQLHistoryEntry.Field with apex-specific
// derived flags.
func (e ApexHistoryEntry) Field(name string) (any, bool) {
	switch name {
	case "OrgUser", "Org":
		return e.OrgUser, true
	case "Body":
		return e.Body, true
	case "ExecutedAt":
		return e.ExecutedAt, true
	case "DurationMs", "Duration":
		return e.DurationMs, true
	case "Compiled":
		return e.Compiled, true
	case "Success":
		return e.Success, true
	case "CompileProblem":
		return e.CompileProblem, true
	case "ExceptionMessage":
		return e.ExceptionMessage, true
	case "Line":
		return e.Line, true
	case "Column":
		return e.Column, true
	case "LogID":
		return e.LogID, true
	case "HasLog":
		return e.LogID != "", true
	case "Status":
		switch {
		case !e.Compiled:
			return "compile_error", true
		case !e.Success:
			return "runtime_error", true
		}
		return "ok", true
	}
	return nil, false
}

// LogApexHistory inserts one history row. Returns the assigned id.
func (s *Store) LogApexHistory(e ApexHistoryEntry) (int64, error) {
	if e.ExecutedAt.IsZero() {
		e.ExecutedAt = time.Now()
	}
	res, err := s.db.Exec(`
		INSERT INTO apex_history (
			org_user, body, executed_at, duration_ms,
			compiled, success, compile_problem, exception_message,
			line, column_, log_id, log_body
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.OrgUser, e.Body, e.ExecutedAt.Unix(), e.DurationMs,
		boolToInt(e.Compiled), boolToInt(e.Success),
		e.CompileProblem, e.ExceptionMessage,
		e.Line, e.Column, e.LogID, e.LogBody)
	if err != nil {
		return 0, fmt.Errorf("insert apex_history: %w", err)
	}
	return res.LastInsertId()
}

// ListApexHistory returns the last `limit` rows for orgUser, newest
// first. orgUser="" returns history across all orgs.
func (s *Store) ListApexHistory(orgUser string, limit int) ([]ApexHistoryEntry, error) {
	q := `SELECT id, org_user, body, executed_at, duration_ms,
	             compiled, success, compile_problem, exception_message,
	             line, column_, log_id, log_body
	        FROM apex_history`
	args := []any{}
	if orgUser != "" {
		q += ` WHERE org_user = ?`
		args = append(args, orgUser)
	}
	q += ` ORDER BY executed_at DESC, id DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ApexHistoryEntry
	for rows.Next() {
		var e ApexHistoryEntry
		var execSec int64
		var compiled, success int
		if err := rows.Scan(&e.ID, &e.OrgUser, &e.Body, &execSec, &e.DurationMs,
			&compiled, &success, &e.CompileProblem, &e.ExceptionMessage,
			&e.Line, &e.Column, &e.LogID, &e.LogBody); err != nil {
			return nil, err
		}
		e.ExecutedAt = time.Unix(execSec, 0)
		e.Compiled = compiled != 0
		e.Success = success != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// TrimApexHistory keeps only the most recent `keep` rows per org.
// Same algorithm + rationale as TrimSOQLHistory.
func (s *Store) TrimApexHistory(keep int) error {
	if keep <= 0 {
		return nil
	}
	_, err := s.db.Exec(`
		DELETE FROM apex_history
		 WHERE id IN (
		     SELECT id FROM apex_history h1
		      WHERE (
		          SELECT COUNT(*) FROM apex_history h2
		           WHERE h2.org_user = h1.org_user
		             AND h2.id > h1.id
		      ) >= ?
		 )`, keep)
	if err != nil {
		return fmt.Errorf("trim apex_history: %w", err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
