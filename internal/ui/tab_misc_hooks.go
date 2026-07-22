package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// activateExecRun handles Enter on the /exec tab. On the Editor
// subtab it submits the textarea body for execution; on Saved /
// History it loads the cursored row into the editor and flips back
// to the Editor subtab. The Output subtab is read-only so Enter
// no-ops there.
//
// On production orgs the run is gated by a confirmation modal —
// anonymous Apex can DELETE records, fire flows, hit triggers, etc.
// SBX runs straight through (no gate).
func (m *Model) activateExecRun() tea.Cmd {
	switch m.currentSubtab() {
	case SubtabExecSaved:
		m.loadCursoredExecSavedEntry()
		return nil
	case SubtabExecHistory:
		m.loadCursoredExecHistoryEntry()
		return nil
	case SubtabExecOutput:
		return nil
	}
	body := strings.TrimSpace(m.execInput.Value())
	if body == "" {
		m.flash("nothing to run — write some Apex first (e to edit)")
		return nil
	}
	if len(m.orgs) == 0 {
		m.flash("no org selected")
		return nil
	}
	o := m.orgs[m.selected]
	// Safety gate first: anonymous Apex can do anything, so the org
	// must allow WriteAnonymous (only the `full` level does). This is
	// the real boundary; the prod-confirmation modal below is an extra
	// speed-bump on top of it, not a substitute.
	if ok, reason := m.canWriteOrg(o, settings.WriteAnonymous); !ok {
		m.flash(reason)
		return nil
	}
	if isProductionOrg(o) {
		return m.openExecProdGate(o, body)
	}
	return m.runExecConfirmed(o, body)
}

// runExecConfirmed is the actual launch path — sets the running
// flag, clears prior result, fires runExecCmd. Split out so the
// prod-gate's confirm callback can invoke it without going back
// through activateExecRun (which would re-trigger the gate).
func (m *Model) runExecConfirmed(o sf.Org, body string) tea.Cmd {
	m.execRunning = true
	m.execErr = nil
	m.execResult = sf.ExecuteAnonymousResult{}
	userID := ""
	if d := m.activeOrgData(); d != nil {
		userID = d.Home.Value().UserID
	}
	return m.runExecCmd(o, body, m.execCaptureLog, userID)
}

// isProductionOrg reports whether o is NOT a sandbox / scratch /
// dev-hub. We treat anything-not-sandbox as prod for the gate
// purpose: false-positives prompt an extra confirmation, false-
// negatives skip the prompt — only the second is dangerous.
func isProductionOrg(o sf.Org) bool {
	return !o.IsSandbox
}

// loadCursoredExecSavedEntry + loadCursoredExecHistoryEntry are
// declared in tab_exec_library.go (the Real variants). These thin
// wrappers preserve the original name activateExecRun calls into.
func (m *Model) loadCursoredExecSavedEntry()   { m.loadCursoredExecSavedEntryReal() }
func (m *Model) loadCursoredExecHistoryEntry() { m.loadCursoredExecHistoryEntryReal() }

func (m *Model) activateSOQLResult() tea.Cmd {
	// Library subtabs: Enter loads the cursored query into the
	// editor and flips back to the Editor subtab so the user lands
	// ready to tweak / run.
	switch m.currentSubtab() {
	case SubtabSOQLSaved:
		m.loadCursoredSavedEntry()
		return nil
	case SubtabSOQLHistory:
		m.loadCursoredHistoryEntry()
		return nil
	}
	if len(m.soqlResult.Records) == 0 {
		return nil
	}
	rec, ok := m.soqlSelectedRecord()
	if !ok {
		return nil
	}
	id, _ := rec["Id"].(string)
	if id == "" {
		// SOQL projections that don't include Id can't drill — most
		// often a GROUP BY query or a SELECT that omits Id by
		// accident. Surface so the user knows to add Id rather than
		// pressing Enter into nothing.
		m.flash("can't drill — row has no Id (add it to the SELECT)")
		return nil
	}
	sobject, _ := recordSObject(rec)
	if sobject == "" {
		// SF includes attributes.type on every record fetched via
		// REST, so missing here is unusual — most likely an
		// aggregated row or a relationship-traversed sub-row that
		// got promoted to the top. Surface rather than silent
		// no-op so debugging is faster.
		m.flash("can't drill — row has no sObject type")
		return nil
	}
	name := recordDisplayName(rec)
	return m.triggerRecordDrill(sobject, id, name, TabSOQL)
}

func (m *Model) resetSOQLCursor() {
	if d, ok := m.activeOrgState(); ok {
		switch m.currentSubtab() {
		case SubtabSOQLSaved:
			d.SOQLSavedList.ResetCursor()
			return
		case SubtabSOQLHistory:
			d.SOQLHistoryList.ResetCursor()
			return
		}
	}
	d, _ := m.activeOrgState()
	entry := soqlProjectionFor(d, m.soqlResult.Records, m.soqlSearchPtr(), theme.Current.ID, m.soqlInput.Value())
	if entry == nil || len(entry.filtered) == 0 {
		m.soqlRowCur = 0
		return
	}
	soqlTableAdapter(m, entry).ResetDisplayTop()
}

func (m *Model) moveSOQLCursor(delta int) {
	if d, ok := m.activeOrgState(); ok {
		switch m.currentSubtab() {
		case SubtabSOQLSaved:
			d.SOQLSavedList.MoveBy(delta)
			return
		case SubtabSOQLHistory:
			d.SOQLHistoryList.MoveBy(delta)
			return
		}
	}
	d, _ := m.activeOrgState()
	entry := soqlProjectionFor(d, m.soqlResult.Records, m.soqlSearchPtr(), theme.Current.ID, m.soqlInput.Value())
	if entry == nil || len(entry.filtered) == 0 {
		m.soqlRowCur = 0
		return
	}
	soqlTableAdapter(m, entry).MoveDisplay(delta)
}

func (m Model) setupSearchPtr() *searchState { return m.setupList.SearchPtr() }

func (m *Model) moveSetupCursor(delta int) { m.setupList.MoveBy(delta) }

func (m *Model) resetSetupCursor() { m.setupList.ResetCursor() }

func (m *Model) ensureSystemData(d *orgData, _ sf.Org) tea.Cmd {
	switch m.currentSubtab() {
	case SubtabSystemDeploys:
		return d.Deploys.Ensure(m.cache)
	case SubtabSystemAudit:
		return d.SetupAudit.Ensure(m.cache)
	case SubtabSystemInterviews:
		return d.FlowInterviews.Ensure(m.cache)
	case SubtabSystemAsyncJobs:
		return d.AsyncJobs.Ensure(m.cache)
	case SubtabSystemScheduled:
		return d.ScheduledJobs.Ensure(m.cache)
	case SubtabSystemAPI:
		return nil
	case SubtabSystemLogs:
		fallthrough
	default:
		return d.ApexLogs.Ensure(m.cache)
	}
}

func (m Model) refreshSystemData(d *orgData) tea.Cmd {
	switch m.currentSubtab() {
	case SubtabSystemDeploys:
		return d.Deploys.Refresh(m.cache)
	case SubtabSystemAudit:
		return d.SetupAudit.Refresh(m.cache)
	case SubtabSystemInterviews:
		return d.FlowInterviews.Refresh(m.cache)
	case SubtabSystemAsyncJobs:
		return d.AsyncJobs.Refresh(m.cache)
	case SubtabSystemScheduled:
		return d.ScheduledJobs.Refresh(m.cache)
	case SubtabSystemAPI:
		return nil
	case SubtabSystemLogs:
		fallthrough
	default:
		return d.ApexLogs.Refresh(m.cache)
	}
}
