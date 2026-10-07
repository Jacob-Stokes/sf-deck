package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

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

type scheduledJobClassResolvedMsg struct {
	classID   string
	className string
	err       error
}
