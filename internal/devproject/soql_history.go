package devproject

// SOQL execution history. Every executed query lands here scoped
// to (org_user, executed_at). The history table is INSERT-heavy
// and almost always read with a small limit ("last 50 queries"),
// so the index on (org_user, executed_at DESC) carries every
// query path.
//
// History rows are NOT linked to saved_queries by ID — a saved
// query and a typed-by-hand query produce equivalent history
// rows. The Body string is the source of truth; if the user
// later wants to re-save a one-off, they pull the body from
// history into the editor and CreateSavedQuery from there.

import (
	"fmt"
	"time"
)

// SOQLHistoryEntry is one execution log row.
//
// Error is non-empty when the run failed; in that case RowCount
// is 0 and DurationMs reflects the time spent before failing
// (which is meaningful — slow failures matter).
type SOQLHistoryEntry struct {
	ID         int64
	OrgUser    string
	Body       string
	ExecutedAt time.Time
	DurationMs int
	RowCount   int
	Error      string
}

// Field implements query.Row so chip predicates can filter the
// history list. Surfaces both the raw fields and a couple of
// derived booleans ("HasError" / "Status") for human-readable
// chip queries.
func (e SOQLHistoryEntry) Field(name string) (any, bool) {
	switch name {
	case "OrgUser", "Org":
		return e.OrgUser, true
	case "Body":
		return e.Body, true
	case "ExecutedAt":
		return e.ExecutedAt, true
	case "DurationMs", "Duration":
		return e.DurationMs, true
	case "RowCount", "Rows":
		return e.RowCount, true
	case "Error":
		return e.Error, true
	case "HasError":
		return e.Error != "", true
	case "Status":
		if e.Error != "" {
			return "error", true
		}
		return "ok", true
	}
	return nil, false
}

// LogSOQLHistory inserts one history row. Returns the assigned id.
//
// Body is stored verbatim — substitutions (`$ME` etc) are NOT
// pre-resolved on the way in, because the post-substitution form
// is what executed and that's what the user wants to see when
// they recall it. Callers that want the original form can check
// the corresponding saved query (when there is one).
func (s *Store) LogSOQLHistory(orgUser, body string, durationMs, rowCount int, errMsg string) (int64, error) {
	res, err := s.db.Exec(`
		INSERT INTO soql_history (org_user, body, executed_at, duration_ms, row_count, error)
		VALUES (?, ?, ?, ?, ?, ?)`,
		orgUser, body, time.Now().Unix(), durationMs, rowCount, errMsg)
	if err != nil {
		return 0, fmt.Errorf("insert soql_history: %w", err)
	}
	return res.LastInsertId()
}

// ListSOQLHistory returns the last `limit` rows for orgUser, newest
// first. limit <= 0 means "all" (which the index makes cheap, but
// callers should pass a sensible cap — typical UI shows 50-200).
//
// orgUser="" returns history across all orgs (used by the global
// /soql Library when there's no active org context).
func (s *Store) ListSOQLHistory(orgUser string, limit int) ([]SOQLHistoryEntry, error) {
	q := `SELECT id, org_user, body, executed_at, duration_ms, row_count, error
	        FROM soql_history`
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
	var out []SOQLHistoryEntry
	for rows.Next() {
		var e SOQLHistoryEntry
		var execSec int64
		if err := rows.Scan(&e.ID, &e.OrgUser, &e.Body, &execSec,
			&e.DurationMs, &e.RowCount, &e.Error); err != nil {
			return nil, err
		}
		e.ExecutedAt = time.Unix(execSec, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

// TrimSOQLHistory keeps only the most recent `keep` rows per org.
// Called periodically (on app startup is fine — small table) so
// the history table doesn't grow unboundedly.
//
// Per-org because different orgs have wildly different usage
// patterns; a chatty dev sandbox shouldn't push prod history out.
func (s *Store) TrimSOQLHistory(keep int) error {
	if keep <= 0 {
		return nil
	}
	// Trim by id rather than executed_at: the executed_at column is
	// stored at second resolution, so multiple inserts in the same
	// second get the same timestamp and a count-newer comparison
	// can't distinguish them. The auto-increment id is monotonically
	// increasing per row, so "how many rows in this org have a
	// higher id than me" is a sound rank.
	_, err := s.db.Exec(`
		DELETE FROM soql_history
		 WHERE id IN (
		     SELECT id FROM soql_history h1
		      WHERE (
		          SELECT COUNT(*) FROM soql_history h2
		           WHERE h2.org_user = h1.org_user
		             AND h2.id > h1.id
		      ) >= ?
		 )`, keep)
	if err != nil {
		return fmt.Errorf("trim soql_history: %w", err)
	}
	return nil
}
