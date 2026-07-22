package ui

// Enter on a Jobs row drills into the Apex class body behind the job —
// the "what is this job actually running?" answer — reusing the existing
// /apex class viewer. Esc returns to the originating Jobs subtab for
// free: the drill only changes m.tab() (→ TabApexDetail) and stamps the
// return via rememberDrillReturn; the per-org SystemSubtab index is
// untouched, so setTab(TabSystem) on esc lands back on the same subtab.

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// activateAsyncJob is Enter on an Async Jobs row: drill into the job's
// Apex class body. No-op for jobs with no Apex class (rare — most async
// jobs are class-backed).
func (m *Model) activateAsyncJob() tea.Cmd {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	job, ok := d.AsyncJobList.Selected()
	if !ok {
		return nil
	}
	if job.ApexClassID == "" {
		m.flash("no Apex class for this job")
		return nil
	}
	rememberDrillReturn(d, TabApexDetail, TabSystem)
	return m.triggerOpenApexClass(job.ApexClassID)
}

// activateScheduledJob is Enter on a Scheduled Jobs row. CronTrigger
// doesn't hold the Apex class directly, so we resolve it via the
// AsyncApexJob bridge (ScheduledJobApexClass) in a command, then drill.
// No-op for schedules that aren't Apex-backed (dashboard refresh, etc.).
func (m *Model) activateScheduledJob() tea.Cmd {
	d := m.activeOrgData()
	if d == nil || len(m.orgs) == 0 {
		return nil
	}
	job, ok := d.ScheduledJobList.Selected()
	if !ok || job.ID == "" {
		return nil
	}
	target := targetArg(m.orgs[m.selected])
	cronID := job.ID
	return func() tea.Msg {
		classID, className, err := sf.ScheduledJobApexClass(target, cronID)
		return scheduledJobClassResolvedMsg{classID: classID, className: className, err: err}
	}
}

// scheduledJobClassResolvedMsg carries the resolved Apex class for a
// scheduled job. Update drills into the class viewer (or flashes when
// the schedule isn't Apex-backed) on the live Model, so the drill lands
// with a fresh return-tab stamp.
type scheduledJobClassResolvedMsg struct {
	classID   string
	className string
	err       error
}
