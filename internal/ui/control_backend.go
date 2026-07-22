package ui

// control_backend.go — adapter between the running Bubble Tea Model
// and the internal/control listener. Lives here so it can read Model
// directly; the control package itself stays UI-agnostic.
//
// The Tea update loop publishes a snapshot to the shared
// ControlState on every successful render. The control listener
// reads from ControlState concurrently. Writes (tab.open, chip.apply)
// flow back the other way via a tea.Cmd channel that the Tea
// program drains in its update loop.

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
	mu       sync.RWMutex
	snapshot map[string]any
	subs     []chan map[string]any
	// Write channel: control handlers push tea.Msg values here; the
	// Tea program reads from it via a tea.Cmd subscription set up at
	// startup. Buffered so a single in-flight write doesn't block
	// the control goroutine.
	writes chan tea.Msg
	// devProjects gives the bundle / project.import-bundle IPC
	// handlers a direct path to the persistence layer. Bundle ops
	// don't drive the TUI (they shell out to sf), so they bypass
	// the writes channel.
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
			// Subscriber is behind; drop the older frame in favour of
			// the newer one by reading and re-sending.
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

// ----- control.Backend implementation -----------------------------

// State returns the latest published snapshot. Called from the
// control listener goroutine.
func (s *ControlState) State() (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Shallow copy so the caller can mutate freely (e.g. add an
	// instance number) without racing the publisher.
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
		// Backlogged; the Tea loop hasn't drained recent writes. Tell
		// the agent to back off rather than queueing arbitrarily.
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

// ErrBusy implements the Code() interface the control package
// recognises for the instance_busy error envelope.
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

// encodeControlServiceError preserves the control protocol's stable error
// codes while keeping service packages independent of the IPC wire layer.
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

// ----- Tea messages flowing from control → update loop -----------

// controlSeedSOQLMsg is queued by the control backend's SOQLSeed
// hook. The Tea update path pushes the query into the SOQL editor
// (m.soqlInput) and optionally fires runSOQLCmd to execute it —
// same path the user takes by typing into the editor and pressing
// Enter.
type controlSeedSOQLMsg struct{ args control.SOQLSeedArgs }

// controlOpenTabMsg is queued by the control backend's OpenTab
// handler. The Tea update loop translates it into the same
// navigation a user keystroke would produce.
type controlOpenTabMsg struct{ args control.OpenTabArgs }

// controlChipApplyMsg is queued by the control backend's ApplyChip
// handler.
type controlChipApplyMsg struct{ args control.ApplyChipArgs }

// controlSwitchOrgMsg is queued by the control backend's SwitchOrg
// handler.
type controlSwitchOrgMsg struct{ args control.SwitchOrgArgs }

// controlLoadProjectMsg is queued by the control backend's
// LoadProject handler. Empty args.ID = unload.
type controlLoadProjectMsg struct{ args control.LoadProjectArgs }

// controlPreviewResp is the channel value the synchronous
// PreviewChip apply func sends back to the IPC caller. Carries the
// minted id (or an error) so the response envelope can echo it.
type controlPreviewResp struct {
	result control.PreviewChipResult
	err    error
}

// controlPreviewChipMsg is queued by the control backend's
// PreviewChip handler. Synchronous — the agent waits for the apply
// func to mint an id before the response goes out.
type controlPreviewChipMsg struct {
	args control.PreviewChipArgs
	resp chan<- controlPreviewResp
}

// controlPreviewSaveChipMsg promotes an ephemeral to a persisted
// chip via the existing chips.Create service. Synchronous so the
// IPC envelope reflects validation errors (duplicate id, bad
// column, etc.) rather than swallowing them in a flash banner.
type controlPreviewSaveChipMsg struct {
	args control.PreviewSaveChipArgs
	resp chan<- error
}

// controlPreviewDismissChipMsg drops an ephemeral. Synchronous so
// the IPC envelope can carry "not found" cleanly.
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

// snapshotForControl assembles the read-only view of Model the
// control listener exposes. Kept small + stable; agents shouldn't
// need to know about internal field names.
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

// tabName is the inverse of tabByName for snapshot rendering. Falls
// back to "" for tabs we don't have a stable string for. Single-pass
// scan over the same map so the two stay in sync; the map is small
// enough (<20 entries) that a linear walk is cheaper than maintaining
// a second inverted index.
func tabName(t Tab) string {
	for name, tt := range controlTabNames {
		if tt == t {
			// Prefer the canonical name when there's an alias. The
			// canonical names sort before their aliases lexically,
			// but rather than depend on map iteration order we hard-
			// code the one aliased pair.
			if name == "records" {
				continue
			}
			return name
		}
	}
	return ""
}

// applyControlOpenTab handles a tab-open coming in over the control
// channel. Resolves the requested tab name to a Tab enum, applies
// it, and (when an sObject is specified) drills into the matching
// records view.
func (m Model) applyControlOpenTab(msg controlOpenTabMsg) (Model, tea.Cmd) {
	tab, ok := tabByName(msg.args.Tab)
	if !ok {
		// Unknown tab — surface via flash so the user can see what
		// the external agent tried to do.
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

// applyControlSeedSOQL handles a seed request coming in over the
// control channel. Pushes the supplied query into the SOQL editor
// (m.soqlInput), optionally navigates to /soql so the editor is
// visible, and optionally fires runSOQLCmd to execute it.
//
// Run is the agent's "auto-execute" lever — when true we mirror
// exactly what the in-editor Enter handler does so history /
// retry / cancel all work the same.
func (m Model) applyControlSeedSOQL(msg controlSeedSOQLMsg) (Model, tea.Cmd) {
	q := strings.TrimSpace(msg.args.Query)
	if q == "" {
		return m, nil
	}
	// Optional nav. Default to opening /soql when neither Open
	// nor Run is set so the seed is actually visible — a bare
	// seed with the tab elsewhere is rarely what the agent wants.
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

	// Run path: mirror the editor's Enter handler.
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

// applyControlChipApply handles a chip-apply coming in over the
// control channel. Resolves the chip via the existing registry and
// applies it the same way a user's keystroke would.
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
	// Move the chip-strip cursor onto the requested chip so the
	// next applySelectedChipMatcher tick (triggered by tab change /
	// render / cycle) reads the same chip rather than reverting to
	// whatever index used to be there. Same shape as
	// applyControlPreviewChip's activation path.
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

// findChipPreview is a small lookup helper used by chip.apply (and
// related verbs) so the registry-vs-preview lookup logic isn't
// inlined at each call site. Linear walk; the preview map holds at
// most a handful of entries per session.
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

// applyControlSwitchOrg handles an org.switch coming in over the
// control channel. Resolves OrgUser preferentially; falls back to
// Alias. Triggers the same post-switch flow a manual ' ' selection
// in the org panel does.
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

// applyControlLoadProject handles a project.load (or project.unload
// when args.ID is empty) coming in over the control channel.
// Resolves the active org's username from m.orgs[m.selected] so the
// scope hydrator filters items correctly.
func (m Model) applyControlLoadProject(msg controlLoadProjectMsg) (Model, tea.Cmd) {
	if len(m.orgs) == 0 {
		return m, nil
	}
	orgUser := m.orgs[m.selected].Username
	label := ""
	if msg.args.ID != "" {
		// Pull the project name for the loaded-project pill so the UI
		// renders "Project: <name>" without an extra round-trip.
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
	// Parse Clauses into the qchip Query AST so the runtime
	// chip-apply path (chipMatcherFor → query.Eval) has the predicate
	// available. Without this the ephemeral chip's filter would be
	// an empty AndNode that matches everything — visually
	// indistinguishable from "no chip selected" once the user
	// navigates away and back and applySelectedChipMatcher reruns.
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
	// Capture the chip the user was looking at BEFORE the prepend.
	// The new ephemeral lands at the start of the strip (after the
	// synthetic Project/Recent chips), so the index of every chip
	// after that point shifts by one. With activate=false the user
	// expects to keep looking at whatever they had selected — without
	// this compensation they'd silently jump to a neighbouring chip.
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
	// Activate default is true; nil pointer treats unset args as
	// "yes, switch to it" so the common case is one round trip.
	activate := true
	if msg.args.Activate != nil {
		activate = *msg.args.Activate
	}
	if !activate && priorChipID != "" && surfBefore != nil && surfBefore.SetChipIdx != nil {
		// Re-find the prior chip's new index after the prepend, so the
		// cursor stays on the same chip the user was viewing.
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
			// Move the chip-strip cursor onto the new ephemeral so
			// the user sees it highlighted as the active view (not
			// just having its predicate silently installed under a
			// different chip's highlight). Find the chip's index in
			// the freshly-rebuilt strip and call the surface's
			// SetChipIdx hook.
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
		// Echo the original scope so the client sees what it sent;
		// the canonical "*" for non-records is an internal detail.
		Scope: msg.args.Scope,
		Label: msg.args.Label,
	}, nil)
	return m, nil
}

// applyControlPreviewSaveChip promotes the named ephemeral chip to
// a persisted one. Resolves the in-memory chip, builds a
// chips.CreateInput, and calls chips.Create — every existing
// validation (id shape, collision check, column shape) runs
// through that single code path.
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
	// Find the ephemeral chip by id. Search every slot — the agent
	// doesn't have to tell us the (domain, scope) tuple again.
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
	// Adapt m.saveSettings (which takes a success-flash arg + returns
	// bool) to the chips.Persister shape (no args, returns error).
	persist := func() error {
		(&m).saveSettings("")
		return nil
	}
	if _, err := chips.Create(m.settings, in, persist); err != nil {
		reply(err)
		return m, nil
	}
	// Promotion succeeded — drop the ephemeral so the strip doesn't
	// double-render the same view.
	(&m).removeChipPreview(found.Domain, found.Scope, msg.args.ID)
	reply(nil)
	return m, nil
}

// applyControlPreviewDismissChip drops one ephemeral chip. Returns
// not-found through the IPC envelope rather than silently
// no-op-ing, so a confused agent doesn't think it succeeded.
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

// validChipDomain reports whether the given domain has a chip
// surface registered. Wraps chipSurfaceForDomain so callers don't
// have to import the surface type.
func validChipDomain(d chipDomain) bool {
	return chipSurfaceForDomain(d) != nil
}

// controlTabNames is the stable string ↔ Tab mapping the IPC layer
// exposes to agents. Single source of truth so tabName and tabByName
// don't drift, and the architectural "no scattered tab switches"
// guard treats this as one entry, not two.
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
	// "records" is the agent-facing alias for the records drill — same
	// Tab as object-detail in this codebase.
	"records": TabObjectDetail,
}

// tabByName resolves the public string identifier of a tab to its
// Tab enum value.
func tabByName(name string) (Tab, bool) {
	t, ok := controlTabNames[name]
	return t, ok
}

// ----- Bundle / project.import-bundle Backend impls --------------
//
// All of these bypass the TUI tea.Cmd channel: bundle ops talk
// directly to services/bundles + services/projects against
// devprojects.db, optionally shelling out to `sf project ...`.
// The TUI doesn't need to render anything; the agent gets a sync
// response (or a deploy_id to poll via BundleReport for async ops).

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

// Resolve the org for both the retrieve alias and the username
// stamped on items via OrgUser filtering.

// A deploy ships metadata to the org — gate it at the metadata
// write level, same as the CLI bundle deploy path.

// translateDeployOpts maps the IPC arg shape (string Tests + []string
// TestClasses) into sf.DeployOpts. Same accepted vocabulary as the
// CLI helper; lives here instead of being shared so the control
// package doesn't need to import cli.
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

// ----- project.* Backend impls -----------------------------------

// Resolve --org alias to a canonical username when caller didn't
// pass org_user directly — matches the CLI behaviour.

// ----- soql / apex / record / metadata / object / tag / safety Backend impls --------

// resolveTargetForIPC resolves an org for an IPC call. Prefers
// args' alias, falls back to org_user, and finally to the
// instance's pinned default org. Returns the canonical
// alias-or-username string passed to sf shell-outs + the
// canonical username for the response envelope.
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

// Log to soql_history so IPC-driven runs share the same history
// surface the TUI's editor populates. Best-effort: a missing
// store (rare — only when --control is on but devprojects.db
// couldn't open) shouldn't fail the query response.

// SOQLSeed is the queue method; the actual editor mutation
// happens in applyControlSeedSOQL via the tea.Msg.
// (Method defined alongside OpenTab earlier.)

// Lookup by name — walk the list. Saved-query names are
// expected to be unique but the store doesn't enforce it,
// so first match wins (same as the CLI).

// UpdateSavedQuery takes full strings, not pointers — fetch
// current state, overlay the supplied fields, then write.

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

// Lookup id by FullName via Tooling SOQL — same pattern the
// CLI uses when only --full-name is supplied.

// Custom fields use Account.Phone_v2__c shape; the Tooling
// table for them is CustomField but the lookup query is
// different. Don't over-engineer — agents that need this
// can SOQL-lookup the id themselves before calling
// metadata.get.

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

// readFileTrim reads a file (or stdin via "-") and trims trailing
// whitespace. Same shape the CLI's resolve helpers use.
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

// _ enforces at compile time that ControlState implements Backend.
var _ control.Backend = (*ControlState)(nil)

// silence: keep devproject import in case bundle deps relocate later.
var _ = devproject.Bundle{}
