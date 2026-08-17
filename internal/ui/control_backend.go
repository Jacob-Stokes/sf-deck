package ui

// control_backend.go — adapter between the running Bubble Tea Model
// and the internal/control listener. Lives here so it can read Model
// directly; the control package itself stays UI-agnostic.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/control"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/services/apexops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/bundles"
	"github.com/Jacob-Stokes/sf-deck/internal/services/chips"
	"github.com/Jacob-Stokes/sf-deck/internal/services/metadataops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/services/records"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
	"github.com/Jacob-Stokes/sf-deck/internal/verbs"
)

// ControlState is the shared snapshot the UI publishes to and the
// control listener reads from. Concurrent: protected by an internal
// mutex. Initialised once at startup; the publisher is the only
// writer.
type ControlState struct {
	mu          sync.RWMutex
	snapshot    map[string]any
	subs        []chan map[string]any
	writes      chan tea.Msg
	devProjects *devproject.Store
	// resolveOrg resolves an alias / username via app.App. The
	// listener uses it to validate org refs in bundle args. nil-safe
	// — callers that don't have an org-context skip the resolve.
	resolveOrg func(target string) (sf.Org, error)
	// safetyFor returns the current SafetyLevel for the resolved org.
	// Used by org.safety.get. nil-safe — callers handle a nil ptr.
	safetyFor func(o sf.Org) settings.SafetyLevel
	// settings is a reference to the live Settings struct the TUI
	// also reads/writes. Setting per-org safety mutates it in place
	// and we follow with saveSettings() to persist.
	settings *settings.Settings
	// saveSettings persists settings to disk. nil-safe — when nil,
	// org.safety.set returns an error rather than silently dropping
	// the write.
	saveSettings func() error
	// metadata delegates Tooling metadata writes to the same safety-enforced
	// service used by the headless CLI.
	metadata *metadataops.Service
	apex     *apexops.Service
	records  *records.Service
	bundles  *bundles.Service
}

// ControlServices holds data-plane services used by IPC methods that do not
// need to enter the Bubble Tea update loop. It is optional for compatibility
// with embedded/test construction; missing entries are built from the legacy
// resolver/safety dependencies where possible.
type ControlServices struct {
	Metadata *metadataops.Service
	Apex     *apexops.Service
	Records  *records.Service
	Bundles  *bundles.Service
}

// NewControlState constructs an empty shared state with a buffered
// write channel. Buffer size 16 is plenty — agents rarely chain
// more than a handful of writes before reading state back.
//
// Callers wire devProjects + the App-level helpers (org resolver,
// safety reader, settings + saver) so the data-plane IPC verbs hit
// the same surface area the CLI does. Any may be nil; the verb
// returns an error directing the caller to launch sf-deck with
// the proper deps in place.
func NewControlState(
	devProjects *devproject.Store,
	resolveOrg func(string) (sf.Org, error),
	safetyFor func(o sf.Org) settings.SafetyLevel,
	st *settings.Settings,
	saveSettings func() error,
	services ...ControlServices,
) *ControlState {
	metadata := metadataops.New(orgwrite.NewGate(resolveOrg, safetyFor))
	apex := apexops.New(orgwrite.NewGate(resolveOrg, safetyFor))
	recordWrites := records.New(orgwrite.NewGate(resolveOrg, safetyFor))
	bundleOps := bundles.New(devProjects, orgwrite.NewGate(resolveOrg, safetyFor))
	if len(services) > 0 && services[0].Metadata != nil {
		metadata = services[0].Metadata
	}
	if len(services) > 0 && services[0].Apex != nil {
		apex = services[0].Apex
	}
	if len(services) > 0 && services[0].Records != nil {
		recordWrites = services[0].Records
	}
	if len(services) > 0 && services[0].Bundles != nil {
		bundleOps = services[0].Bundles
	}
	return &ControlState{
		snapshot:     map[string]any{},
		writes:       make(chan tea.Msg, 16),
		devProjects:  devProjects,
		resolveOrg:   resolveOrg,
		safetyFor:    safetyFor,
		settings:     st,
		saveSettings: saveSettings,
		metadata:     metadata,
		apex:         apex,
		records:      recordWrites,
		bundles:      bundleOps,
	}
}

// Publish replaces the snapshot and fans it out to any subscribers.
// Called from the Tea update loop after each frame's state shifts.
// Drop-newest backpressure: a subscriber that can't keep up loses
// the intermediate frames but always sees the latest one.
func (s *ControlState) Publish(snap map[string]any) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.snapshot = snap
	subs := s.subs
	s.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- snap:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- snap:
			default:
			}
		}
	}
}

// Writes returns the channel the Tea program reads from to apply
// inbound control commands. Each tea.Msg goes through the standard
// Update path so the same code drives both keystrokes and IPC.
func (s *ControlState) Writes() <-chan tea.Msg { return s.writes }

// State returns the latest published snapshot. Called from the
// control listener goroutine.
func (s *ControlState) State() (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]any, len(s.snapshot))
	for k, v := range s.snapshot {
		out[k] = v
	}
	return out, nil
}

// Subscribe registers a snapshot channel and returns it plus a
// cancel func that removes it. Buffered to size 1 — the publisher
// uses drop-newest semantics so subscribers always see the freshest
// available frame.
func (s *ControlState) Subscribe() (<-chan map[string]any, func(), error) {
	ch := make(chan map[string]any, 1)
	s.mu.Lock()
	s.subs = append(s.subs, ch)
	s.mu.Unlock()
	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, c := range s.subs {
			if c == ch {
				s.subs = append(s.subs[:i], s.subs[i+1:]...)
				close(ch)
				return
			}
		}
	}
	return ch, cancel, nil
}

// OpenTab forwards a tab-open request to the Tea program. Returns
// once the message is queued. Resolution / safety checking happens
// inside the update path — failures propagate back via the snapshot
// stream (state.last_error) rather than the OpenTab error return,
// because applying a tab change is async by nature.
func (s *ControlState) OpenTab(args control.OpenTabArgs) error {
	if s == nil {
		return errors.New("control backend not initialised")
	}
	select {
	case s.writes <- controlOpenTabMsg{args: args}:
		return nil
	default:
		return ErrBusy
	}
}

// SOQLSeed forwards a seed-the-SOQL-editor request. Runs only
// when args.Open==true OR args.Run==true (a bare seed doesn't
// require nav). The Tea update path is responsible for the
// actual editor mutation + optional run kickoff.

// ApplyChip forwards a chip-apply request the same way.
func (s *ControlState) ApplyChip(args control.ApplyChipArgs) error {
	if s == nil {
		return errors.New("control backend not initialised")
	}
	select {
	case s.writes <- controlChipApplyMsg{args: args}:
		return nil
	default:
		return ErrBusy
	}
}

// SwitchOrg forwards an org-switch request.
func (s *ControlState) SwitchOrg(args control.SwitchOrgArgs) error {
	if s == nil {
		return errors.New("control backend not initialised")
	}
	select {
	case s.writes <- controlSwitchOrgMsg{args: args}:
		return nil
	default:
		return ErrBusy
	}
}

// LoadProject forwards a project-load (or unload, when args.ID="").
func (s *ControlState) LoadProject(args control.LoadProjectArgs) error {
	if s == nil {
		return errors.New("control backend not initialised")
	}
	select {
	case s.writes <- controlLoadProjectMsg{args: args}:
		return nil
	default:
		return ErrBusy
	}
}

// PreviewChip queues a chip.preview request and waits synchronously
// for the apply func to mint an id. Different from the other write
// verbs because the response carries the minted id in its data —
// the agent uses it later for save/dismiss without correlating
// against args.
func (s *ControlState) PreviewChip(args control.PreviewChipArgs) (control.PreviewChipResult, error) {
	if s == nil {
		return control.PreviewChipResult{}, errors.New("control backend not initialised")
	}
	resp := make(chan controlPreviewResp, 1)
	select {
	case s.writes <- controlPreviewChipMsg{args: args, resp: resp}:
	default:
		return control.PreviewChipResult{}, ErrBusy
	}
	r := <-resp
	return r.result, r.err
}

// PreviewSaveChip queues a chip.preview.save request.
func (s *ControlState) PreviewSaveChip(args control.PreviewSaveChipArgs) error {
	if s == nil {
		return errors.New("control backend not initialised")
	}
	resp := make(chan error, 1)
	select {
	case s.writes <- controlPreviewSaveChipMsg{args: args, resp: resp}:
	default:
		return ErrBusy
	}
	return <-resp
}

// PreviewDismissChip queues a chip.preview.dismiss request.
func (s *ControlState) PreviewDismissChip(args control.PreviewDismissChipArgs) error {
	if s == nil {
		return errors.New("control backend not initialised")
	}
	resp := make(chan error, 1)
	select {
	case s.writes <- controlPreviewDismissChipMsg{args: args, resp: resp}:
	default:
		return ErrBusy
	}
	return <-resp
}

type errBusy struct{}

func (errBusy) Error() string { return "instance busy" }
func (errBusy) Code() string  { return control.ErrInstanceBusy }

type controlServiceError struct {
	code string
	err  error
}

func (e controlServiceError) Error() string { return e.err.Error() }
func (e controlServiceError) Unwrap() error { return e.err }
func (e controlServiceError) Code() string  { return e.code }

func encodeControlServiceError(err error) error {
	var blocked orgwrite.BlockedError
	if errors.As(err, &blocked) {
		return controlServiceError{code: control.ErrSafetyBlocked, err: err}
	}
	var invalidType metadataops.ErrInvalidType
	if errors.As(err, &invalidType) {
		return controlServiceError{code: control.ErrInvalidArgument, err: err}
	}
	return err
}

// ErrBusy is the sentinel returned when the write channel is full.
var ErrBusy = errBusy{}

type controlSeedSOQLMsg struct{ args control.SOQLSeedArgs }

type controlOpenTabMsg struct{ args control.OpenTabArgs }

type controlChipApplyMsg struct{ args control.ApplyChipArgs }

type controlSwitchOrgMsg struct{ args control.SwitchOrgArgs }

type controlLoadProjectMsg struct{ args control.LoadProjectArgs }

type controlPreviewResp struct {
	result control.PreviewChipResult
	err    error
}

type controlPreviewChipMsg struct {
	args control.PreviewChipArgs
	resp chan<- controlPreviewResp
}

type controlPreviewSaveChipMsg struct {
	args control.PreviewSaveChipArgs
	resp chan<- error
}

type controlPreviewDismissChipMsg struct {
	args control.PreviewDismissChipArgs
	resp chan<- error
}

// AttachControl wires a ControlState onto a Model. Called from
// cmd/sf-deck after Listen() succeeds. Also stamps the instance
// number so the badge renderer can show it.
func (m *Model) AttachControl(cs *ControlState, instanceNumber int) {
	m.control = cs
	m.instanceNumber = instanceNumber
}

// InstanceNumber returns the instance number assigned at startup.
// Always >= 1.
func (m Model) InstanceNumber() int {
	if m.instanceNumber <= 0 {
		return 1
	}
	return m.instanceNumber
}

// ControlWritesCmd returns a tea.Cmd that blocks on the next inbound
// control message and surfaces it as a tea.Msg the Update loop can
// dispatch. Re-issued from Update on each control message so the
// program keeps draining new ones. Nil when control isn't attached.
func (m Model) ControlWritesCmd() tea.Cmd {
	if m.control == nil {
		return nil
	}
	ch := m.control.Writes()
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// PublishControlSnapshot is called from the Update loop on every
// render. No-op when control isn't attached.
func (m Model) PublishControlSnapshot() {
	if m.control == nil {
		return
	}
	m.control.Publish(m.snapshotForControl())
}

func (m Model) snapshotForControl() map[string]any {
	snap := map[string]any{
		"instance_number": m.InstanceNumber(),
		"tab":             tabName(m.tab()),
		"subtab":          string(m.currentSubtab()),
	}
	if d := m.activeOrgData(); d != nil {
		if d.DescribeCur != "" {
			snap["sobject"] = d.DescribeCur
		}
		if d.LoadedDevProjectID != "" {
			snap["active_project"] = d.LoadedDevProjectID
		}
	}
	if len(m.orgs) > 0 {
		snap["active_org"] = m.orgs[m.selected].Username
	}
	// Surface IPC-spawned ephemerals so a reconnecting agent can see
	// what it (or a previous agent) spun up without having to track
	// ids in its own memory. Cross-org previews (OriginOrgUser != "ipc")
	// are deliberately excluded — those belong to the user's
	// preview/widen-scope flow, not the controller surface.
	type ephemeralEntry struct {
		ID     string `json:"id"`
		Domain string `json:"domain"`
		Scope  string `json:"scope,omitempty"`
		Label  string `json:"label"`
	}
	var ephemerals []ephemeralEntry
	for _, slot := range m.chipPreviews {
		for _, p := range slot {
			if p.OriginOrgUser != chipPreviewOriginIPC {
				continue
			}
			ephemerals = append(ephemerals, ephemeralEntry{
				ID:     p.Chip.ID,
				Domain: string(p.Domain),
				Scope:  p.Scope,
				Label:  p.Chip.Label,
			})
		}
	}
	if len(ephemerals) > 0 {
		snap["ephemeral_chips"] = ephemerals
	}
	return snap
}

func tabName(t Tab) string {
	for name, tt := range controlTabNames {
		if tt == t {
			if name == "records" {
				continue
			}
			return name
		}
	}
	return ""
}

func (m Model) applyControlOpenTab(msg controlOpenTabMsg) (Model, tea.Cmd) {
	tab, ok := tabByName(msg.args.Tab)
	if !ok {
		m.flash("control: unknown tab " + msg.args.Tab)
		return m, nil
	}
	m.setTab(tab)
	if msg.args.SObject != "" {
		d := m.activeOrgData()
		if d != nil {
			d.DescribeCur = msg.args.SObject
		}
	}
	return m, m.onTabChanged()
}

func (m Model) applyControlSeedSOQL(msg controlSeedSOQLMsg) (Model, tea.Cmd) {
	q := strings.TrimSpace(msg.args.Query)
	if q == "" {
		return m, nil
	}
	openSOQL := msg.args.Open || msg.args.Run || m.tab() != TabSOQL
	if openSOQL && m.tab() != TabSOQL {
		m.setTab(TabSOQL)
	}

	// Push the value into the textarea. Don't move focus
	// implicitly; if Run==true we fire the query immediately
	// (focus state doesn't matter), otherwise we leave the user
	// in whatever mode they were in. Note: SetValue resets the
	// cursor to position 0 by design — fine for seeded content
	// since the user typically wants to read the whole query
	// from the start anyway.
	m.soqlInput.SetValue(q)

	if !msg.args.Run {
		var cmds []tea.Cmd
		if openSOQL {
			cmds = append(cmds, m.onTabChanged())
		}
		return m, tea.Batch(cmds...)
	}

	if len(m.orgs) == 0 {
		return m, nil
	}
	m.soqlEditing = false
	m.soqlRunning = true
	m.soqlErr = nil
	m.soqlInput.Blur()
	if m.autocomplete != nil {
		m.autocomplete.Items = nil
	}
	o := m.orgs[m.selected]
	ctx, cancel := context.WithCancel(context.Background())
	m.soqlCancel = cancel
	m.soqlRunGen++
	runCmd := m.runSOQLCmd(o, m.soqlInput.Value(), m.soqlTooling, m.soqlBulk, ctx, m.soqlRunGen, soqlSessionTab, m.soqlSession.id)
	var cmds []tea.Cmd
	if openSOQL {
		cmds = append(cmds, m.onTabChanged())
	}
	cmds = append(cmds, runCmd)
	return m, tea.Batch(cmds...)
}

func (m Model) applyControlChipApply(msg controlChipApplyMsg) (Model, tea.Cmd) {
	domain := chipDomain(msg.args.Domain)
	reg := m.registryFor(domain)
	if reg == nil {
		m.flash(fmt.Sprintf("control: unknown chip domain %q", msg.args.Domain))
		return m, nil
	}
	// Resolve the chip from either the registry (persisted chips) or
	// m.chipPreviews (session-only ephemerals + cross-org previews).
	// The original code only consulted the registry, which silently
	// no-op'd ephemerals: chip.apply returned ok:true while the
	// predicate never landed.
	var c qchip.Chip
	if found, ok := reg.FindByID(msg.args.ID); ok {
		c = found
	} else if p, ok := m.findChipPreview(msg.args.ID); ok {
		c = p.Chip
	} else {
		m.flash(fmt.Sprintf("control: chip %q not found in domain %s", msg.args.ID, msg.args.Domain))
		return m, nil
	}
	d := m.activeOrgData()
	if d == nil {
		return m, nil
	}
	surf := chipSurfaceForDomain(domain)
	if surf == nil || surf.ApplyChip == nil {
		return m, nil
	}
	if surf.SetChipIdx != nil {
		scope := c.Scope
		if scope == "" && domain != domainRecords {
			scope = "*"
		}
		strip := m.stripRows(domain, scope)
		for i, row := range strip {
			if row.ID == msg.args.ID {
				surf.SetChipIdx(&m, i)
				break
			}
		}
	}
	surf.ApplyChip(d, c)
	return m, nil
}

func (m Model) findChipPreview(id string) (chipPreview, bool) {
	for _, slot := range m.chipPreviews {
		for _, p := range slot {
			if p.Chip.ID == id {
				return p, true
			}
		}
	}
	return chipPreview{}, false
}

func (m Model) applyControlSwitchOrg(msg controlSwitchOrgMsg) (Model, tea.Cmd) {
	target := msg.args.OrgUser
	if target == "" {
		target = msg.args.Alias
	}
	if target == "" {
		return m, nil
	}
	for i, o := range m.orgs {
		if o.Username == target || o.Alias == target {
			(&m).setSelectedOrg(i)
			return m, m.onTabChanged()
		}
	}
	m.flash(fmt.Sprintf("control: org %q not found", target))
	return m, nil
}

func (m Model) applyControlLoadProject(msg controlLoadProjectMsg) (Model, tea.Cmd) {
	if len(m.orgs) == 0 {
		return m, nil
	}
	orgUser := m.orgs[m.selected].Username
	label := ""
	if msg.args.ID != "" {
		if dp, ok := m.devProjectByID(msg.args.ID); ok {
			label = dp.Name
		}
	}
	(&m).loadDevProject(orgUser, msg.args.ID, label)
	return m, nil
}

// applyControlPreviewChip handles a chip.preview coming in over the
// control channel. Mints an ephemeral id, builds a qchip.Chip from
// the supplied args, drops it onto the active org's strip via the
// existing addChipPreview path, and (unless Activate=false) makes
// it the active view in one step.
//
// Sends the minted id back on msg.resp so the IPC handler can echo
// it in the response. We do the send unconditionally — error path
// included — so the caller never hangs.
func (m Model) applyControlPreviewChip(msg controlPreviewChipMsg) (Model, tea.Cmd) {
	reply := func(r control.PreviewChipResult, err error) {
		if msg.resp == nil {
			return
		}
		msg.resp <- controlPreviewResp{result: r, err: err}
	}
	if len(m.orgs) == 0 {
		reply(control.PreviewChipResult{}, errors.New("no active org"))
		return m, nil
	}
	domain := chipDomain(msg.args.Domain)
	if !validChipDomain(domain) {
		reply(control.PreviewChipResult{}, fmt.Errorf("unknown chip domain %q", msg.args.Domain))
		return m, nil
	}
	if domain == domainRecords && msg.args.Scope == "" {
		reply(control.PreviewChipResult{}, errors.New("scope is required for records domain"))
		return m, nil
	}
	// Normalise scope: non-records surfaces key their strip on "*",
	// so an empty scope from the IPC client (which is natural — no
	// sObject context for /flows etc.) must be mapped onto the
	// surface-wide bucket or the chip would sit in an unreachable
	// slot and never render.
	scope := msg.args.Scope
	if domain != domainRecords && scope == "" {
		scope = "*"
	}
	id := newEphemeralChipID()
	var qquery query.Query
	if msg.args.Clauses != "" {
		parsed, err := chips.ParseClauses(msg.args.Clauses)
		if err != nil {
			reply(control.PreviewChipResult{},
				fmt.Errorf("parse clauses: %w", err))
			return m, nil
		}
		qquery = qchip.QueryFromConfig(parsed)
	}
	c := qchip.Chip{
		ID:     id,
		Label:  msg.args.Label,
		Scope:  scope,
		Origin: qchip.OriginUser,
		Query:  qquery,
	}
	surfBefore := chipSurfaceForDomain(domain)
	var priorChipID string
	if surfBefore != nil && surfBefore.ChipIdx != nil {
		stripBefore := m.stripRows(domain, scope)
		if idx := surfBefore.ChipIdx(m); idx >= 0 && idx < len(stripBefore) {
			priorChipID = stripBefore[idx].ID
		}
	}
	(&m).pushChipPreview(chipPreview{
		Domain:        domain,
		Scope:         scope,
		Chip:          c,
		OriginOrgUser: chipPreviewOriginIPC,
		Columns:       msg.args.Columns,
		Limit:         msg.args.Limit,
		Clauses:       msg.args.Clauses,
	})
	activate := true
	if msg.args.Activate != nil {
		activate = *msg.args.Activate
	}
	if !activate && priorChipID != "" && surfBefore != nil && surfBefore.SetChipIdx != nil {
		stripAfter := m.stripRows(domain, scope)
		for i, row := range stripAfter {
			if row.ID == priorChipID {
				surfBefore.SetChipIdx(&m, i)
				break
			}
		}
	}
	if activate {
		d := m.activeOrgData()
		if d != nil {
			surf := chipSurfaceForDomain(domain)
			if surf != nil && surf.ApplyChip != nil {
				surf.ApplyChip(d, c)
			}
			if surf != nil && surf.SetChipIdx != nil {
				strip := m.stripRows(domain, scope)
				for i, row := range strip {
					if row.ID == id {
						surf.SetChipIdx(&m, i)
						break
					}
				}
				m.applySelectedChipMatcher(d)
			}
		}
	}
	reply(control.PreviewChipResult{
		ID:     id,
		Domain: string(domain),
		Scope:  msg.args.Scope,
		Label:  msg.args.Label,
	}, nil)
	return m, nil
}

func (m Model) applyControlPreviewSaveChip(msg controlPreviewSaveChipMsg) (Model, tea.Cmd) {
	reply := func(err error) {
		if msg.resp == nil {
			return
		}
		msg.resp <- err
	}
	if m.settings == nil {
		reply(errors.New("settings not initialised"))
		return m, nil
	}
	var found *chipPreview
	for _, slot := range m.chipPreviews {
		for i := range slot {
			if slot[i].Chip.ID == msg.args.ID {
				found = &slot[i]
				break
			}
		}
		if found != nil {
			break
		}
	}
	if found == nil {
		reply(fmt.Errorf("ephemeral chip %q not found", msg.args.ID))
		return m, nil
	}
	in := chips.CreateInput{
		ID:        msg.args.NewID,
		Domain:    string(found.Domain),
		Scope:     found.Scope,
		Label:     found.Chip.Label,
		Favourite: msg.args.Favourite,
		Columns:   found.Columns,
		Limit:     found.Limit,
		Clauses:   found.Clauses,
	}
	persist := func() error {
		(&m).saveSettings("")
		return nil
	}
	if _, err := chips.Create(m.settings, in, persist); err != nil {
		reply(err)
		return m, nil
	}
	(&m).removeChipPreview(found.Domain, found.Scope, msg.args.ID)
	reply(nil)
	return m, nil
}

func (m Model) applyControlPreviewDismissChip(msg controlPreviewDismissChipMsg) (Model, tea.Cmd) {
	reply := func(err error) {
		if msg.resp == nil {
			return
		}
		msg.resp <- err
	}
	var found *chipPreview
	for _, slot := range m.chipPreviews {
		for i := range slot {
			if slot[i].Chip.ID == msg.args.ID {
				found = &slot[i]
				break
			}
		}
		if found != nil {
			break
		}
	}
	if found == nil {
		reply(fmt.Errorf("ephemeral chip %q not found", msg.args.ID))
		return m, nil
	}
	(&m).removeChipPreview(found.Domain, found.Scope, msg.args.ID)
	reply(nil)
	return m, nil
}

func validChipDomain(d chipDomain) bool {
	return chipSurfaceForDomain(d) != nil
}

var controlTabNames = map[string]Tab{
	"home":          TabHome,
	"soql":          TabSOQL,
	"objects":       TabObjects,
	"flows":         TabFlows,
	"apex":          TabApex,
	"users":         TabUsers,
	"perms":         TabPerms,
	"dev-projects":  TabDevProjects,
	"object-detail": TabObjectDetail,
	"records":       TabObjectDetail,
}

func tabByName(name string) (Tab, bool) {
	t, ok := controlTabNames[name]
	return t, ok
}

func (s *ControlState) ensureStore() error {
	if s == nil || s.devProjects == nil {
		return errors.New("devprojects store not initialised — relaunch sf-deck without --demo")
	}
	return nil
}

func (s *ControlState) resolveBundleOrg(target string) (alias, username string, err error) {
	if target == "" {
		return "", "", nil
	}
	if s.resolveOrg == nil {
		return "", "", errors.New("org resolver unavailable")
	}
	o, err := s.resolveOrg(target)
	if err != nil {
		return "", "", err
	}
	return o.Alias, o.Username, nil
}

func translateDeployOpts(testsFlag string, classes []string) (sf.DeployOpts, error) {
	level := strings.TrimSpace(testsFlag)
	if level == "" {
		if len(classes) > 0 {
			return sf.DeployOpts{}, errors.New(
				"test_classes requires tests=RunSpecifiedTests")
		}
		return sf.DeployOpts{}, nil
	}
	var normalized sf.DeployTestLevel
	switch strings.ToLower(level) {
	case "notestrun", "no-test-run":
		normalized = sf.TestLevelNoTestRun
	case "runspecifiedtests", "run-specified", "specified":
		normalized = sf.TestLevelRunSpecified
	case "runlocaltests", "run-local", "local":
		normalized = sf.TestLevelRunLocalTests
	case "runalltestsintorg", "runalltestsinorg", "run-all", "all":
		normalized = sf.TestLevelRunAllTestsInOrg
	default:
		return sf.DeployOpts{}, fmt.Errorf(
			"unknown tests value %q (expected NoTestRun / RunSpecifiedTests / RunLocalTests / RunAllTestsInOrg)",
			level)
	}
	opts := sf.DeployOpts{TestLevel: normalized}
	if normalized == sf.TestLevelRunSpecified {
		if len(classes) == 0 {
			return sf.DeployOpts{}, errors.New(
				"tests=RunSpecifiedTests requires test_classes")
		}
		opts.TestClasses = append(opts.TestClasses, classes...)
	} else if len(classes) > 0 {
		return sf.DeployOpts{}, errors.New(
			"test_classes only meaningful with tests=RunSpecifiedTests")
	}
	return opts, nil
}

// ----- soql / apex / record / metadata / object / tag / safety Backend impls --------

func (s *ControlState) resolveTargetForIPC(alias, user string) (target, username string, err error) {
	if s.resolveOrg == nil {
		return "", "", errors.New("org resolver unavailable")
	}
	want := alias
	if want == "" {
		want = user
	}
	o, err := s.resolveOrg(want)
	if err != nil {
		return "", "", err
	}
	target = o.Alias
	if target == "" {
		target = o.Username
	}
	return target, o.Username, nil
}

func (s *ControlState) ApexRun(args control.ApexRunArgs) (any, error) {
	src := args.Body
	if src == "" && args.BodyFile != "" {
		body, rerr := readFileTrim(args.BodyFile)
		if rerr != nil {
			return nil, rerr
		}
		src = body
	}
	if strings.TrimSpace(src) == "" {
		return nil, errors.New("apex body required")
	}
	target := args.OrgAlias
	if target == "" {
		target = args.OrgUser
	}
	serviceResult, err := s.apex.Execute(context.Background(), apexops.ExecuteInput{
		Target: target, Body: src,
	})
	if err != nil {
		return nil, encodeControlServiceError(err)
	}
	res := serviceResult.Execution
	return map[string]any{
		"compiled":          res.Compiled,
		"success":           res.Success,
		"compile_problem":   res.CompileProblem,
		"exception_message": res.ExceptionMessage,
		"line":              res.Line,
		"column":            res.Column,
		"took_ms":           res.Took.Milliseconds(),
	}, nil
}

func (s *ControlState) ObjectDescribe(args control.ObjectDescribeArgs) (any, error) {
	target, _, err := s.resolveTargetForIPC(args.OrgAlias, args.OrgUser)
	if err != nil {
		return nil, err
	}
	desc, err := sf.Describe(target, args.SObject)
	if err != nil {
		return nil, err
	}
	return desc, nil
}

func (s *ControlState) VerbsList(args control.VerbsListArgs) ([]any, error) {
	var specs []verbs.Spec
	switch args.Surface {
	case "":
		specs = verbs.Specs()
	case "cli":
		specs = verbs.SpecsForSurface(verbs.SurfaceCLI)
	case "ipc":
		specs = verbs.SpecsForSurface(verbs.SurfaceIPC)
	case "tui":
		specs = verbs.SpecsForSurface(verbs.SurfaceTUI)
	default:
		return nil, fmt.Errorf("unknown surface %q (want cli|ipc|tui)", args.Surface)
	}
	out := make([]any, 0, len(specs))
	for _, s := range specs {
		out = append(out, verbSpecToMap(s))
	}
	return out, nil
}

// verbSpecToMap projects a verbs.Spec into the same JSON shape the
// CLI verbsToJSON uses. Duplicated rather than imported because
// internal/headless/cli can't be imported from internal/ui without
// creating a cycle.
func verbSpecToMap(s verbs.Spec) map[string]any {
	entry := map[string]any{
		"noun":      s.Noun,
		"verb":      s.Verb,
		"qualified": s.Qualified(),
		"summary":   s.Summary,
		"stability": s.Stability,
	}
	if s.Safety != "" {
		entry["safety"] = string(s.Safety)
	}
	if s.Notes != "" {
		entry["notes"] = s.Notes
	}
	if s.TUIOnly {
		entry["tui_only"] = true
	}
	if s.CLI != nil {
		entry["cli"] = map[string]any{
			"usage":    s.CLI.Usage,
			"flags":    verbFlagsToMap(s.CLI.Flags),
			"examples": s.CLI.Examples,
		}
	}
	if s.IPC != nil {
		entry["ipc"] = map[string]any{
			"command":  s.IPC.Command,
			"args":     verbFieldsToMap(s.IPC.Args),
			"examples": s.IPC.Examples,
			"async":    s.IPC.Async,
		}
	}
	return entry
}

func verbFlagsToMap(fs []verbs.FlagSpec) []map[string]any {
	out := make([]map[string]any, 0, len(fs))
	for _, f := range fs {
		out = append(out, map[string]any{
			"name":        f.Name,
			"type":        f.Type,
			"required":    f.Required,
			"description": f.Description,
		})
	}
	return out
}

func verbFieldsToMap(fs []verbs.FieldSpec) []map[string]any {
	out := make([]map[string]any, 0, len(fs))
	for _, f := range fs {
		out = append(out, map[string]any{
			"name":        f.Name,
			"type":        f.Type,
			"required":    f.Required,
			"description": f.Description,
		})
	}
	return out
}

func (s *ControlState) ReportList(args control.ReportListArgs) ([]any, error) {
	target, _, err := s.resolveTargetForIPC(args.OrgAlias, args.OrgUser)
	if err != nil {
		return nil, err
	}
	all, err := sf.ListAllReports(target)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(args.Contains)
	folderNeedle := strings.ToLower(args.Folder)
	out := make([]any, 0, len(all))
	for _, rep := range all {
		if needle != "" && !strings.Contains(strings.ToLower(rep.Name), needle) {
			continue
		}
		if folderNeedle != "" &&
			!strings.EqualFold(rep.FolderName, args.Folder) &&
			!strings.Contains(strings.ToLower(rep.FolderName), folderNeedle) {
			continue
		}
		out = append(out, rep)
	}
	return out, nil
}

func (s *ControlState) ReportRun(args control.ReportRunArgs) (any, error) {
	target, _, err := s.resolveTargetForIPC(args.OrgAlias, args.OrgUser)
	if err != nil {
		return nil, err
	}
	run, err := sf.RunReport(target, args.ID, args.ForceRerun)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":         run.ID,
		"name":       run.Name,
		"format":     run.Format,
		"columns":    run.Columns,
		"rows":       run.Rows,
		"row_count":  len(run.Rows),
		"all_data":   run.AllData,
		"cached":     run.Cached,
		"ran_at":     run.RanAt,
		"aggregates": run.Aggregates,
	}, nil
}

func (s *ControlState) OrgSafetyGet(args control.OrgSafetyGetArgs) (any, error) {
	if s.resolveOrg == nil || s.safetyFor == nil {
		return nil, errors.New("safety lookup unavailable")
	}
	want := args.OrgAlias
	if want == "" {
		want = args.OrgUser
	}
	o, err := s.resolveOrg(want)
	if err != nil {
		return nil, err
	}
	level := s.safetyFor(o)
	return map[string]any{
		"org_user": o.Username,
		"safety":   level.String(),
	}, nil
}

func (s *ControlState) OrgSafetySet(args control.OrgSafetySetArgs) (any, error) {
	if s.resolveOrg == nil || s.safetyFor == nil || s.settings == nil || s.saveSettings == nil {
		return nil, errors.New("safety mutation unavailable")
	}
	want := args.OrgAlias
	if want == "" {
		want = args.OrgUser
	}
	o, err := s.resolveOrg(want)
	if err != nil {
		return nil, err
	}
	prior := s.safetyFor(o)
	priorOverride, hadPriorOverride := s.settings.OrgSafetyOverride(o.Username)
	restorePrior := func() {
		if hadPriorOverride {
			s.settings.SetOrg(o.Username, settings.ParseSafetyLevel(priorOverride), false)
		} else {
			s.settings.SetOrg(o.Username, settings.SafetyReadOnly, true)
		}
	}
	if args.Clear {
		next := s.settings.ResolveAfterClear(o.Username, settings.OrgKind(o.Kind()), o.Alias)
		if next > prior {
			return nil, controlServiceError{code: control.ErrSafetyBlocked, err: fmt.Errorf(
				"IPC can only lower safety; clearing %s would raise it from %s to %s",
				o.Username, prior.String(), next.String())}
		}
		s.settings.SetOrg(o.Username, settings.SafetyReadOnly, true)
	} else {
		switch strings.ToLower(strings.TrimSpace(args.Level)) {
		case "read_only", "records", "metadata", "full":
		default:
			return nil, controlServiceError{code: control.ErrInvalidArgument, err: fmt.Errorf(
				"invalid safety level %q (want read_only|records|metadata|full)", args.Level)}
		}
		lvl := settings.ParseSafetyLevel(args.Level)
		// IPC may only LOWER an org's safety, never raise it. Otherwise
		// a socket client could self-escalate a read-only org to full
		// and then run Apex / DML — defeating the write gates entirely.
		// Raising safety is a deliberate, risk-accepting act that must
		// happen at the keyboard in the TUI, not over the wire.
		if lvl > prior {
			return nil, controlServiceError{code: control.ErrSafetyBlocked, err: fmt.Errorf(
				"IPC can only lower safety; raising %s from %s to %s must be done in the TUI",
				o.Username, prior.String(), lvl.String())}
		}
		s.settings.SetOrg(o.Username, lvl, false)
	}
	if err := s.saveSettings(); err != nil {
		restorePrior()
		return nil, err
	}
	now := s.safetyFor(o)
	return map[string]any{
		"org_user":     o.Username,
		"safety":       now.String(),
		"prior_safety": prior.String(),
		"cleared":      args.Clear,
	}, nil
}

func readFileTrim(path string) (string, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

var _ control.Backend = (*ControlState)(nil)

var _ = devproject.Bundle{}
