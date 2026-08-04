package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/services/apexops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// runSOQLCmd is the odd-one-out command: SOQL results aren't cached and
// produce their own dedicated message so the SOQL view can track them
// independently of the resource plumbing.
//
// The closure also times the run + captures the org username so the
// reducer can persist a soql_history row when the result lands. We
// time inside the closure (not at runSOQLCmd call time) so the
// duration measures actual execution rather than command-queue
// latency.
//
// bulk routes the run through Bulk API 2.0 instead of REST: 1 API
// call for the whole job vs REST's 1 call per 2000 rows. Async, so
// the user trades latency for API budget. Tooling is ignored when
// bulk is set (Bulk doesn't support Tooling); the editor's toggle
// handler keeps them mutually exclusive so this never matters in
// practice, but the branch is defensive.
func (m Model) runSOQLCmd(o sf.Org, soql string, tooling, bulk bool, ctx context.Context, gen uint64, target soqlSessionTarget, sessionID uint64) tea.Cmd {
	alias := targetArg(o)
	username := o.Username
	return func() tea.Msg {
		t0 := time.Now()
		var (
			res sf.QueryResult
			err error
		)
		if bulk {
			// Bulk supports proper server-side cancel via context —
			// the caller's cancel func aborts the polling loop AND
			// sends a delete-job to SF so the server stops chewing
			// CPU on a query the user doesn't want.
			res, err = sf.BulkQueryRecords(ctx, alias, soql, nil)
		} else {
			// REST path: ctx aborts the in-flight HTTP request +
			// any nextRecordsUrl follow-on pages.  Server doesn't
			// know we cancelled — the query may keep running for a
			// few hundred ms — but the modal returns to idle
			// immediately.
			res, err = sf.QueryCtx(ctx, alias, soql, tooling)
		}
		took := int(time.Since(t0) / time.Millisecond)
		return soqlResultMsg{
			session: target, sessionID: sessionID, soql: soql, data: res, err: err,
			orgUser: username, tookMs: took, gen: gen,
		}
	}
}

// runExecCmd is the /exec equivalent of runSOQLCmd. Submits an
// anonymous-Apex body to the Tooling executeAnonymous endpoint;
// optionally fetches the most recent ApexLog after a successful
// run for the debug-output viewer.
//
// captureLog controls the post-run log fetch — on by default. When
// off, the run is one HTTP call instead of three (status + log row
// SOQL + log body GET). Useful when the user knows the script has
// no System.debug calls or doesn't care about the log.
//
// userID is the running user's 005… Id, needed to filter ApexLog
// rows to ones the running user themselves authored. We pass it
// from the cached Home payload (d.Home.Value().UserID).
func (m Model) runExecCmd(o sf.Org, body string, captureLog bool, userID string) tea.Cmd {
	alias := targetArg(o)
	username := o.Username
	service := m.apex
	if service == nil {
		// Compatibility for tests/embedded callers that build a Model without
		// main's service injection. Execution still goes through apexops and
		// its authoritative gate.
		gate := orgwrite.NewGate(func(string) (sf.Org, error) { return o, nil },
			func(org sf.Org) settings.SafetyLevel {
				return m.settings.Resolve(org.Username, settings.OrgKind(org.Kind()), org.Alias)
			})
		service = apexops.New(gate)
	}
	return func() tea.Msg {
		result, err := service.Execute(context.Background(), apexops.ExecuteInput{
			Target: alias, Body: body, CaptureLog: captureLog, UserID: userID,
		})
		if result.Target.Username != "" {
			username = result.Target.Username
		}
		return execResultMsg{body: body, data: result.Execution, err: err, orgUser: username}
	}
}
