package sf

import (
	"context"
	"encoding/json"
)

// QueryResult is a slim wrapper around records from `sf data query`.
// Each record is a map because SOQL is arbitrary.
type QueryResult struct {
	Records   []map[string]any `json:"records"`
	TotalSize int              `json:"totalSize"`
	Done      bool             `json:"done"`
}

type queryResultWrapper struct {
	Status int         `json:"status"`
	Result QueryResult `json:"result"`
}

// Query runs a SOQL against the named org. If tooling is true, uses the
// Tooling API. Read-only (all SELECT queries).
//
// Fast path: use the REST client (no Node startup). Falls back to
// shelling out to `sf data query` if REST bootstrap fails (rare — only
// when sf itself can't hand us a token).
func Query(target, soql string, tooling bool) (QueryResult, error) {
	if c, err := RESTClient(target); err == nil {
		return c.QueryREST(soql, tooling)
	}
	return queryViaCLI(target, soql, tooling)
}

// QueryCtx is the cancellable twin of Query.  ctx is threaded
// through the REST HTTP layer so cancelling it aborts the in-flight
// request (and any nextRecordsUrl follow-on pages).  Falls back to
// the non-cancellable CLI path when the REST client can't bootstrap;
// CLI callers can't honour ctx mid-execve, but the closure returns
// promptly enough that the modal still feels responsive.
func QueryCtx(ctx context.Context, target, soql string, tooling bool) (QueryResult, error) {
	if c, err := RESTClient(target); err == nil {
		return c.QueryRESTCtx(ctx, soql, tooling)
	}
	return queryViaCLI(target, soql, tooling)
}

// QueryCapped runs the SOQL with a client-side row cap. cap <= 0
// disables the cap (matches Query's full-fetch semantics). Used by
// chip-driven fetches so we stop paging once the user's cap is
// reached even though the SOQL itself has no LIMIT — TotalSize from
// SF's first page still reports the true unbounded count for
// truncation hints.
//
// Falls back to the CLI path when REST bootstrap fails, then applies
// the same final cap locally. The CLI may have fetched more rows than
// needed, but callers still get the bounded result they requested.
func QueryCapped(target, soql string, tooling bool, cap int) (QueryResult, error) {
	if c, err := RESTClient(target); err == nil {
		return c.QueryRESTCapped(soql, tooling, cap)
	}
	q, err := queryViaCLI(target, soql, tooling)
	if err != nil {
		return QueryResult{}, err
	}
	return capQueryResult(q, cap), nil
}

func capQueryResult(q QueryResult, cap int) QueryResult {
	if cap <= 0 || len(q.Records) <= cap {
		return q
	}
	q.Records = q.Records[:cap]
	q.Done = false
	return q
}

// queryViaCLI is the legacy path kept as a fallback for when REST
// bootstrap fails (e.g. sf can't provide a token for this org).
func queryViaCLI(target, soql string, tooling bool) (QueryResult, error) {
	args := []string{"data", "query", "-q", soql, "-o", target, "--json"}
	if tooling {
		args = append(args, "--use-tooling-api")
	}
	out, err := runSF(args...)
	if err != nil {
		return QueryResult{}, err
	}
	var parsed queryResultWrapper
	if err := json.Unmarshal(out, &parsed); err != nil {
		return QueryResult{}, err
	}
	return parsed.Result, nil
}
