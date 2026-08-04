package ui

// Per-surface sidebars for the operational / system tabs: apex
// logs, setup audit trail, flow interviews, active-user sessions,
// communities, deploys, packages. Split out of sidebar.go (which
// keeps the shared panel primitives + registry dispatch).

import (
	"fmt"
)

func (m Model) sidebarApexLog(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}
	l, ok := d.ApexLogList.Selected()
	if !ok {
		return sideEmpty("—")
	}
	rows := []kv{
		{"id", l.ID},
		{"status", l.Status},
		{"operation", l.Operation},
		{"application", dashIfEmpty(l.Application)},
		{"duration", fmt.Sprintf("%d ms", l.DurationMs)},
		{"size", humanBytes(l.LogLength)},
		{"user", l.LogUser.Name},
		{"when", prettyDate(l.StartTime)},
	}
	extra := []string{"", sideDim("  ↵ view body  ·  "+
		firstPretty(Keys.YankDefault)+" copy id", inner)}
	return renderKVPanel(inner, "Apex Log", rows, extra...)
}

// sidebarSetupAudit shows one Setup-change event in full. The Display
// sentence and Action code are truncated in the table but rendered
// whole here (sideKV word-wraps long values), so this panel — and the
// `i` inspect modal that reuses it — is the only place you see the
// complete change description.
func (m Model) sidebarSetupAudit(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}
	r, ok := d.SetupAuditList.Selected()
	if !ok {
		return sideEmpty("—")
	}
	by := r.CreatedBy
	if r.Delegate != "" {
		by = r.CreatedBy + " (as " + r.Delegate + ")"
	}
	when := "—"
	if !r.CreatedDate.IsZero() {
		when = r.CreatedDate.Format("2006-01-02 15:04:05")
	}
	rows := []kv{
		{"when", when},
		{"by", by},
		{"section", dashIfEmpty(r.Section)},
		{"action", dashIfEmpty(r.Action)},
		// Display last: it's the longest field and wraps to multiple
		// lines, so keeping it at the bottom keeps the short KV rows
		// aligned above it.
		{"change", dashIfEmpty(r.Display)},
	}
	extra := []string{"", sideDim("  "+firstPretty(Keys.OpenDefault)+" open actor  ·  "+
		firstPretty(Keys.YankDefault)+" copy change", inner)}
	return renderKVPanel(inner, "Setup Change", rows, extra...)
}

// sidebarFlowInterview shows one flow interview in full — the flow +
// timestamp label, its status, and (the key diagnostic) the element it
// is currently sitting on or died at. Reused by the `i` inspect modal.
func (m Model) sidebarFlowInterview(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}
	r, ok := d.FlowInterviewList.Selected()
	if !ok {
		return sideEmpty("—")
	}
	when := "—"
	if !r.CreatedDate.IsZero() {
		when = r.CreatedDate.Format("2006-01-02 15:04:05")
	}
	rows := []kv{
		{"status", dashIfEmpty(r.Status)},
		{"element", dashIfEmpty(r.Element)},
		{"pause", dashIfEmpty(r.PauseLabel)},
		{"started", when},
		{"by", dashIfEmpty(r.CreatedBy)},
		{"id", r.ID},
		// Label last — it's the longest and wraps.
		{"flow", dashIfEmpty(r.Label)},
	}
	extra := []string{"", sideDim("  "+firstPretty(Keys.OpenDefault)+" open starter  ·  "+
		firstPretty(Keys.YankDefault)+" copy element", inner)}
	return renderKVPanel(inner, "Flow Interview", rows, extra...)
}

// sidebarActiveUser shows the security detail behind an active-user row
// (which the presence-forward default columns leave off): source IP,
// session security level / MFA, login type, and the session count.
func (m Model) sidebarActiveUser(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}
	r, ok := d.ActiveUserList.Selected()
	if !ok {
		return sideEmpty("—")
	}
	mfa := "high assurance"
	if r.AnyLowMFA {
		mfa = "LOW (no MFA this session)"
	}
	last := "—"
	if !r.LastActive.IsZero() {
		last = r.LastActive.Format("2006-01-02 15:04:05")
	}
	rows := []kv{
		{"user", r.UserName},
		{"last active", last},
		{"location", dashIfEmpty(r.Location)},
		{"source ip", dashIfEmpty(r.SourceIP)},
		{"security", mfa},
		{"login type", dashIfEmpty(r.LoginType)},
		{"session type", dashIfEmpty(r.SessionType)},
		{"sessions", fmt.Sprintf("%d", r.SessionCount)},
	}
	extra := []string{"", sideDim("  "+firstPretty(Keys.Drill)+" sessions  ·  "+
		firstPretty(Keys.OpenDefault)+" open user", inner)}
	return renderKVPanel(inner, "Active User", rows, extra...)
}

// sidebarUserSession shows one session in full — every AuthSession +
// LoginGeo + LoginHistory field the drill carries.
func (m Model) sidebarUserSession(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}
	r, ok := d.UserSessionList.Selected()
	if !ok {
		return sideEmpty("—")
	}
	last := "—"
	if !r.LastActive.IsZero() {
		last = r.LastActive.Format("2006-01-02 15:04:05")
	}
	started := "—"
	if !r.Started.IsZero() {
		started = r.Started.Format("2006-01-02 15:04:05")
	}
	ttl := "—"
	if r.SecondsValid > 0 {
		ttl = fmt.Sprintf("%dm", r.SecondsValid/60)
	}
	rows := []kv{
		{"type", dashIfEmpty(r.SessionType)},
		{"security", dashIfEmpty(r.SecurityLevel)},
		{"source ip", dashIfEmpty(r.SourceIP)},
		{"location", dashIfEmpty(r.Location())},
		{"country", dashIfEmpty(r.Country)},
		{"browser", dashIfEmpty(r.Browser)},
		{"platform", dashIfEmpty(r.Platform)},
		{"application", dashIfEmpty(r.Application)},
		{"login type", dashIfEmpty(r.LoginType)},
		{"started", started},
		{"last active", last},
		{"valid for", ttl},
	}
	extra := []string{"", sideDim("  "+firstPretty(Keys.YankDefault)+" copy IP / location", inner)}
	return renderKVPanel(inner, "Session", rows, extra...)
}

// sidebarCommunity shows one Experience site's config — the settings
// the columns leave off (guest files, internal login, private messages,
// description) plus URL / status / member count / id.
func (m Model) sidebarCommunity(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}
	r, ok := d.CommunityList.Selected()
	if !ok {
		return sideEmpty("—")
	}
	yn := func(b bool) string {
		if b {
			return "enabled"
		}
		return "off"
	}
	rows := []kv{
		{"name", r.Name},
		{"url prefix", dashIfEmpty(r.URLPathPrefix)},
		{"status", dashIfEmpty(r.Status)},
		{"members", itoa(r.Members)},
		{"self-registration", yn(r.SelfReg)},
		{"guest file access", yn(r.GuestFiles)},
		{"internal-user login", yn(r.InternalLogin)},
		{"private messages", yn(r.PrivateMsgs)},
		{"id", r.ID},
		{"description", dashIfEmpty(r.Description)},
	}
	extra := []string{"", sideDim("  "+firstPretty(Keys.Drill)+" pages  ·  "+
		firstPretty(Keys.OpenDefault)+" open (site/builder/setup)", inner)}
	return renderKVPanel(inner, "Community", rows, extra...)
}

func (m Model) sidebarDeploy(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}
	r, ok := d.DeployList.Selected()
	if m.tab() == TabDeployDetail && d.DeployCur != "" {
		// Drilled in: pin the sidebar to the drilled deploy, not
		// whatever the (hidden) list cursor points at.
		if row, found := m.deployRowByID(d.DeployCur); found {
			r, ok = row, true
		}
	}
	if !ok {
		return sideEmpty("—")
	}
	progress := fmt.Sprintf("%d of %d", r.ComponentsDeployed, r.ComponentsTotal)
	if r.ComponentsTotal > 0 {
		pct := r.ComponentsDeployed * 100 / r.ComponentsTotal
		progress += fmt.Sprintf(" (%d%%)", pct)
	}
	if r.ComponentErrors > 0 {
		progress += fmt.Sprintf(" · %d✗", r.ComponentErrors)
	}
	kind := "deploy"
	if r.CheckOnly {
		kind = "validation"
	}
	rows := []kv{
		{"id", r.ID},
		{"status", r.Status},
		{"kind", kind},
		{"components", progress},
	}
	if r.TestsTotal > 0 {
		tests := fmt.Sprintf("%d of %d", r.TestsCompleted, r.TestsTotal)
		if r.TestErrors > 0 {
			tests += fmt.Sprintf(" · %d✗", r.TestErrors)
		}
		rows = append(rows, kv{"tests", tests})
	}
	if r.TestLevel != "" {
		rows = append(rows, kv{"test level", r.TestLevel})
	}
	if r.ChangeSetName != "" {
		rows = append(rows, kv{"change set", r.ChangeSetName})
	}
	if dur := deployDurationLabel(r); dur != "—" {
		rows = append(rows, kv{"took", dur})
	}
	rows = append(rows,
		kv{"by", dashIfEmpty(r.CreatedByName)},
		kv{"when", prettyDate(r.CreatedDate)},
	)
	if r.CanceledByName != "" {
		rows = append(rows, kv{"canceled by", r.CanceledByName})
	}
	extra := []string{}
	if r.ErrorMessage != "" {
		extra = append(extra, "", sideSection("error"),
			sideDim("  "+wrap(r.ErrorMessage, inner-2), inner))
	}
	if r.StateDetail != "" {
		extra = append(extra, "", sideSection("state"),
			sideDim("  "+wrap(r.StateDetail, inner-2), inner))
	}
	hint := "↵ failures + tests  ·  " + firstPretty(Keys.OpenDefault) + " Lightning"
	if m.tab() == TabDeployDetail {
		hint = firstPretty(Keys.OpenDefault) + " Lightning  ·  Esc back"
	}
	extra = append(extra, "", sideDim("  "+hint, inner))
	title := "Deploy"
	if r.CheckOnly {
		title = "Validation"
	}
	return renderKVPanel(inner, title, rows, extra...)
}

func (m Model) sidebarPackage(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}
	p, ok := d.PackageList.Selected()
	if !ok {
		return sideEmpty("—")
	}
	rows := []kv{
		{"namespace", dashIfEmpty(p.SubscriberPackageNamespace)},
		{"version", "v" + p.SubscriberPackageVersionNumber},
		{"version name", dashIfEmpty(p.SubscriberPackageVersionName)},
		{"pkg id", p.SubscriberPackageID},
		{"version id", p.SubscriberPackageVersionID},
	}
	extra := []string{"", sideDim("  "+
		firstPretty(Keys.OpenDefault)+" Lightning  ·  "+
		firstPretty(Keys.YankDefault)+" copy id", inner)}
	return renderKVPanel(inner, p.SubscriberPackageName, rows, extra...)
}
