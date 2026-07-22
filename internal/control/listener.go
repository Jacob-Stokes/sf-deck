package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/Jacob-Stokes/sf-deck/internal/instance"
)

// Backend is the contract the UI layer fulfils for the control
// channel. Keeping it abstract (string-keyed snapshots, write ops
// returning typed errors) means internal/control doesn't import
// internal/ui — that direction would create a cycle.
//
// State() is called for state.get. Subscribe() is called for
// state.subscribe; it returns a channel that emits JSON-marshalable
// snapshots whenever the UI's observable state changes, plus a
// cancel func the listener calls on client disconnect.
//
// OpenTab / ApplyChip are write verbs. They synthesize the same
// effect the user's keystrokes would produce. Returning a typed
// error lets the listener map error code → JSON envelope.
type Backend interface {
	StateBackend
	NavigationBackend
	BundleBackend
	ProjectBackend
	SOQLBackend
	ApexBackend
	RecordBackend
	MetadataBackend
	ObjectBackend
	VerbsBackend
	ReportBackend
	TagBackend
	SafetyBackend
}

type StateBackend interface {
	State() (map[string]any, error)
	Subscribe() (<-chan map[string]any, func(), error)
}

type NavigationBackend interface {
	OpenTab(args OpenTabArgs) error
	ApplyChip(args ApplyChipArgs) error
	SwitchOrg(args SwitchOrgArgs) error
	LoadProject(args LoadProjectArgs) error
	PreviewChip(args PreviewChipArgs) (PreviewChipResult, error)
	PreviewSaveChip(args PreviewSaveChipArgs) error
	PreviewDismissChip(args PreviewDismissChipArgs) error
}

type BundleBackend interface {
	BundleList(args BundleListArgs) ([]any, error)
	BundleShow(args BundleShowArgs) (any, error)
	BundleCreate(args BundleCreateArgs) (any, error)
	BundleLink(args BundleLinkArgs) (any, error)
	BundleRetrieve(args BundleRetrieveArgs) (any, error)
	BundleValidate(args BundleValidateArgs) (any, error)
	BundleDeploy(args BundleDeployArgs) (any, error)
	BundleReport(args BundleReportArgs) (any, error)
	BundleDelete(args BundleDeleteArgs) error
	ProjectImportBundle(args ProjectImportBundleArgs) (any, error)
}

type ProjectBackend interface {
	ProjectList(args ProjectListArgs) ([]any, error)
	ProjectShow(args ProjectShowArgs) (any, error)
	ProjectCreate(args ProjectCreateArgs) (any, error)
	ProjectUpdate(args ProjectUpdateArgs) (any, error)
	ProjectDelete(args ProjectDeleteArgs) (any, error)
	ProjectAddItem(args ProjectAddItemArgs) (any, error)
	ProjectRemoveItem(args ProjectRemoveItemArgs) (any, error)
	ProjectItems(args ProjectItemsArgs) ([]any, error)
}

type SOQLBackend interface {
	SOQLRun(args SOQLRunArgs) (any, error)
	SOQLSeed(args SOQLSeedArgs) error
	SOQLHistoryList(args SOQLHistoryListArgs) ([]any, error)
	SOQLSavedList(args SOQLSavedListArgs) ([]any, error)
	SOQLSavedShow(args SOQLSavedShowArgs) (any, error)
	SOQLSavedCreate(args SOQLSavedCreateArgs) (any, error)
	SOQLSavedUpdate(args SOQLSavedUpdateArgs) (any, error)
	SOQLSavedDelete(args SOQLSavedDeleteArgs) (any, error)
}

type ApexBackend interface {
	ApexRun(args ApexRunArgs) (any, error)
}

type RecordBackend interface {
	RecordGet(args RecordGetArgs) (any, error)
	RecordRecent(args RecordRecentArgs) (any, error)
	RecordCreate(args RecordCreateArgs) (any, error)
	RecordUpdate(args RecordUpdateArgs) (any, error)
	RecordDelete(args RecordDeleteArgs) (any, error)
}

type MetadataBackend interface {
	MetadataGet(args MetadataGetArgs) (any, error)
	MetadataCreate(args MetadataCreateArgs) (any, error)
	MetadataUpdate(args MetadataUpdateArgs) (any, error)
	MetadataDelete(args MetadataDeleteArgs) (any, error)
}

type ObjectBackend interface {
	ObjectDescribe(args ObjectDescribeArgs) (any, error)
}

type VerbsBackend interface {
	VerbsList(args VerbsListArgs) ([]any, error)
}

type ReportBackend interface {
	ReportList(args ReportListArgs) ([]any, error)
	ReportRun(args ReportRunArgs) (any, error)
}

type TagBackend interface {
	TagList(args TagListArgs) ([]any, error)
	TagShow(args TagShowArgs) (any, error)
	TagCreate(args TagCreateArgs) (any, error)
	TagUpdate(args TagUpdateArgs) (any, error)
	TagDelete(args TagDeleteArgs) (any, error)
	TagApply(args TagApplyArgs) (any, error)
	TagRemove(args TagRemoveArgs) (any, error)
	TagSet(args TagSetArgs) (any, error)
}

type SafetyBackend interface {
	OrgSafetyGet(args OrgSafetyGetArgs) (any, error)
	OrgSafetySet(args OrgSafetySetArgs) (any, error)
}

// OpenTabArgs is the JSON shape of tab.open.
type OpenTabArgs struct {
	Tab     string `json:"tab"`
	SObject string `json:"sobject,omitempty"`
	OrgUser string `json:"org_user,omitempty"`
}

// ApplyChipArgs is the JSON shape of chip.apply.
type ApplyChipArgs struct {
	Domain string `json:"domain"`
	Scope  string `json:"scope,omitempty"`
	ID     string `json:"id"`
}

// SwitchOrgArgs is the JSON shape of org.switch.
type SwitchOrgArgs struct {
	// One of OrgUser or Alias must be set. OrgUser takes precedence
	// when both are supplied.
	OrgUser string `json:"org_user,omitempty"`
	Alias   string `json:"alias,omitempty"`
}

// BundleListArgs / etc are the JSON shapes for the bundle.* verbs.
// Fields mirror the headless CLI flags so the controller skill and
// operator skill use the same vocabulary.
type BundleListArgs struct {
	ProjectID string `json:"project_id,omitempty"`
}

type BundleShowArgs struct {
	ID string `json:"id"`
}

type BundleCreateArgs struct {
	ProjectID    string `json:"project_id"`
	Path         string `json:"path,omitempty"`
	OrgAlias     string `json:"org_alias,omitempty"`
	OrgUser      string `json:"org_user,omitempty"`
	FullProject  bool   `json:"full_project,omitempty"`
	Retrieve     bool   `json:"retrieve,omitempty"`
	ScopeAllOrgs bool   `json:"all_orgs,omitempty"`
	Force        bool   `json:"force,omitempty"`
}

type BundleLinkArgs struct {
	ProjectID string `json:"project_id"`
	Path      string `json:"path"`
	OrgAlias  string `json:"org_alias,omitempty"`
}

type BundleRetrieveArgs struct {
	ID       string `json:"id"`
	OrgAlias string `json:"org_alias,omitempty"`
}

// BundleValidateArgs / BundleDeployArgs mirror their CLI counterparts.
// Async defaults to true for IPC since the listener can't usefully
// hold a 5-15min request open.
//
// Tests / TestClasses mirror the CLI --tests / --test-classes flags.
// Skip the org's full Apex suite when a sandbox has unrelated broken
// tests blocking validates ("NoTestRun" — sandbox only). RunLocal /
// RunAll forces the org's tests. Empty Tests lets Salesforce pick
// its default for the target org.
type BundleValidateArgs struct {
	ID          string   `json:"id"`
	OrgAlias    string   `json:"org_alias,omitempty"`
	Async       bool     `json:"async,omitempty"`
	Tests       string   `json:"tests,omitempty"`
	TestClasses []string `json:"test_classes,omitempty"`
}

type BundleDeployArgs struct {
	ID          string   `json:"id"`
	OrgAlias    string   `json:"org_alias,omitempty"`
	Async       bool     `json:"async,omitempty"`
	Tests       string   `json:"tests,omitempty"`
	TestClasses []string `json:"test_classes,omitempty"`
}

type BundleReportArgs struct {
	ID       string `json:"id"`
	OrgAlias string `json:"org_alias,omitempty"`
	DeployID string `json:"deploy_id"`
}

type BundleDeleteArgs struct {
	ID string `json:"id"`
}

type ProjectImportBundleArgs struct {
	ProjectID string `json:"project_id"`
	Path      string `json:"path"`
	OrgUser   string `json:"org_user,omitempty"`
	OrgAlias  string `json:"org_alias,omitempty"`
}

// ProjectListArgs is a deliberately empty placeholder — project.list
// takes no args. Kept as its own type so the dispatch contract stays
// symmetric with the other verbs.
type ProjectListArgs struct{}

type ProjectShowArgs struct {
	// One of ID or Name. ID wins when both are set.
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type ProjectCreateArgs struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type ProjectUpdateArgs struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

type ProjectDeleteArgs struct {
	ID    string `json:"id"`
	Force bool   `json:"force,omitempty"`
}

// ProjectAddItemArgs mirrors `sf-deck project add-item` exactly.
// OrgAlias is resolved to a canonical username so org-scoped kinds
// match consistently with other verbs.
type ProjectAddItemArgs struct {
	ProjectID string `json:"project_id"`
	Kind      string `json:"kind"`
	Ref       string `json:"ref"`
	OrgUser   string `json:"org_user,omitempty"`
	OrgAlias  string `json:"org_alias,omitempty"`
	Type      string `json:"type,omitempty"`
	Name      string `json:"name,omitempty"`
	Notes     string `json:"notes,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type ProjectRemoveItemArgs struct {
	ProjectID string `json:"project_id"`
	Kind      string `json:"kind"`
	Ref       string `json:"ref"`
	OrgUser   string `json:"org_user,omitempty"`
	OrgAlias  string `json:"org_alias,omitempty"`
}

type ProjectItemsArgs struct {
	ID      string `json:"id"`
	OrgUser string `json:"org_user,omitempty"`
}

// SOQLRunArgs mirrors `sf-deck soql run`. Either Query or QueryFile
// must be supplied (QueryFile = path on the sf-deck host).
type SOQLRunArgs struct {
	OrgAlias  string `json:"org_alias,omitempty"`
	OrgUser   string `json:"org_user,omitempty"`
	Query     string `json:"query,omitempty"`
	QueryFile string `json:"query_file,omitempty"`
	Tooling   bool   `json:"tooling,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

// SOQLSeedArgs pushes a query string into the TUI's SOQL editor.
// When Run is true the editor also fires the query — same as the
// user pressing the run keybind after typing it. When Open is
// true the TUI navigates to /soql first so the editor is visible.
type SOQLSeedArgs struct {
	Query string `json:"query"`
	Open  bool   `json:"open,omitempty"` // navigate to /soql before seeding (default: true)
	Run   bool   `json:"run,omitempty"`  // also fire the query immediately
}

// SOQLHistoryListArgs reads recent SOQL runs from soql_history.
// OrgUser empty = all orgs. Limit 0 = default cap (200).
type SOQLHistoryListArgs struct {
	OrgUser string `json:"org_user,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

// SOQLSavedListArgs / ShowArgs / CreateArgs / UpdateArgs / DeleteArgs
// mirror the CLI soql_saved verbs. Saved-query IDs are sq_<ulid>
// strings (created via Store.NewSavedQueryID), not integers.
type SOQLSavedListArgs struct{}

type SOQLSavedShowArgs struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type SOQLSavedCreateArgs struct {
	Name        string `json:"name"`
	Body        string `json:"body"`
	Description string `json:"description,omitempty"`
}

type SOQLSavedUpdateArgs struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Body        *string `json:"body,omitempty"`
	Description *string `json:"description,omitempty"`
}

type SOQLSavedDeleteArgs struct {
	ID string `json:"id"`
}

// ApexRunArgs runs anonymous Apex. Source comes from Body or BodyFile
// or SnippetID (loaded from the saved-apex library).
type ApexRunArgs struct {
	OrgAlias  string `json:"org_alias,omitempty"`
	OrgUser   string `json:"org_user,omitempty"`
	Body      string `json:"body,omitempty"`
	BodyFile  string `json:"body_file,omitempty"`
	SnippetID string `json:"snippet_id,omitempty"`
}

// RecordGetArgs is a per-record REST GET. SObject is required.
type RecordGetArgs struct {
	OrgAlias string   `json:"org_alias,omitempty"`
	OrgUser  string   `json:"org_user,omitempty"`
	SObject  string   `json:"sobject"`
	ID       string   `json:"id"`
	Fields   []string `json:"fields,omitempty"` // empty = all queryable
}

// RecordUpdateArgs mirrors `sf-deck record update`. Fields is the
// JSON body of fields → values.
type RecordUpdateArgs struct {
	OrgAlias string         `json:"org_alias,omitempty"`
	OrgUser  string         `json:"org_user,omitempty"`
	SObject  string         `json:"sobject"`
	ID       string         `json:"id"`
	Fields   map[string]any `json:"fields"`
}

type RecordRecentArgs struct {
	OrgAlias string `json:"org_alias,omitempty"`
	OrgUser  string `json:"org_user,omitempty"`
	SObject  string `json:"sobject"`
	Limit    int    `json:"limit,omitempty"`
}

type RecordCreateArgs struct {
	OrgAlias string         `json:"org_alias,omitempty"`
	OrgUser  string         `json:"org_user,omitempty"`
	SObject  string         `json:"sobject"`
	Fields   map[string]any `json:"fields"`
}

type RecordDeleteArgs struct {
	OrgAlias string `json:"org_alias,omitempty"`
	OrgUser  string `json:"org_user,omitempty"`
	SObject  string `json:"sobject"`
	ID       string `json:"id"`
}

// MetadataGetArgs / MetadataCreateArgs / MetadataUpdateArgs /
// MetadataDeleteArgs mirror the CLI metadata verbs. Type is the
// MetadataAPI type ("CustomField", "ValidationRule", etc.); FullName
// is its API name (e.g. "Account.Phone_v2__c").
type MetadataGetArgs struct {
	OrgAlias string `json:"org_alias,omitempty"`
	OrgUser  string `json:"org_user,omitempty"`
	Type     string `json:"type"`
	ID       string `json:"id,omitempty"`        // Tooling Id
	FullName string `json:"full_name,omitempty"` // alternate lookup
}

type MetadataCreateArgs struct {
	OrgAlias string         `json:"org_alias,omitempty"`
	OrgUser  string         `json:"org_user,omitempty"`
	Type     string         `json:"type"`
	FullName string         `json:"full_name"`
	Patch    map[string]any `json:"patch"`
}

type MetadataUpdateArgs struct {
	OrgAlias string         `json:"org_alias,omitempty"`
	OrgUser  string         `json:"org_user,omitempty"`
	Type     string         `json:"type"`
	ID       string         `json:"id"`
	Patch    map[string]any `json:"patch"`
}

type MetadataDeleteArgs struct {
	OrgAlias string `json:"org_alias,omitempty"`
	OrgUser  string `json:"org_user,omitempty"`
	Type     string `json:"type"`
	ID       string `json:"id"`
}

// ObjectDescribeArgs returns the cached SObjectDescribe for the
// supplied SObject. Empty SObject is rejected by the handler.
// VerbsListArgs filters the registry. Empty Surface returns every
// Spec; "cli"/"ipc"/"tui" returns only specs binding that surface.
type VerbsListArgs struct {
	Surface string `json:"surface,omitempty"`
}

type ObjectDescribeArgs struct {
	OrgAlias string `json:"org_alias,omitempty"`
	OrgUser  string `json:"org_user,omitempty"`
	SObject  string `json:"sobject"`
}

type ReportListArgs struct {
	OrgAlias string `json:"org_alias,omitempty"`
	OrgUser  string `json:"org_user,omitempty"`
	Contains string `json:"contains,omitempty"`
	Folder   string `json:"folder,omitempty"`
}

type ReportRunArgs struct {
	OrgAlias   string `json:"org_alias,omitempty"`
	OrgUser    string `json:"org_user,omitempty"`
	ID         string `json:"id"`
	ForceRerun bool   `json:"force_rerun,omitempty"`
}

// TagListArgs / TagApplyArgs mirror the CLI. UsageOnly filters the
// list to tags actually applied to at least one item.
type TagListArgs struct {
	UsageOnly bool `json:"usage_only,omitempty"`
}

type TagApplyArgs struct {
	TagID   int64  `json:"tag_id"` // tag id (must already exist)
	Kind    string `json:"kind"`   // ItemKind
	Ref     string `json:"ref"`    // item ref (kind-specific shape)
	OrgUser string `json:"org_user"`
}

type TagShowArgs struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type TagCreateArgs struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
	Icon  string `json:"icon,omitempty"`
}

type TagUpdateArgs struct {
	ID    int64   `json:"id"`
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
	Icon  *string `json:"icon,omitempty"`
}

type TagDeleteArgs struct {
	ID int64 `json:"id"`
}

type TagRemoveArgs struct {
	TagID   int64  `json:"tag_id"`
	Kind    string `json:"kind"`
	Ref     string `json:"ref"`
	OrgUser string `json:"org_user"`
}

type TagSetArgs struct {
	Kind    string  `json:"kind"`
	Ref     string  `json:"ref"`
	OrgUser string  `json:"org_user"`
	TagIDs  []int64 `json:"tag_ids"`
}

// OrgSafetyGetArgs / OrgSafetySetArgs handle per-org safety overrides.
// Level is one of "read_only", "records", "metadata", "full".
// Clear=true reverts the override to whatever the global default is.
type OrgSafetyGetArgs struct {
	OrgAlias string `json:"org_alias,omitempty"`
	OrgUser  string `json:"org_user,omitempty"`
}

type OrgSafetySetArgs struct {
	OrgAlias string `json:"org_alias,omitempty"`
	OrgUser  string `json:"org_user,omitempty"`
	Level    string `json:"level,omitempty"`
	Clear    bool   `json:"clear,omitempty"`
}

// LoadProjectArgs is the JSON shape of project.load.  ID may be
// empty, in which case project.load behaves as project.unload
// (mirrors the in-TUI affordance of pressing _ on /dev-projects
// with no selection).
type LoadProjectArgs struct {
	ID string `json:"id,omitempty"`
}

// PreviewChipArgs is the JSON shape of chip.preview. The agent
// supplies a chip body; the backend mints an id, drops the chip
// onto the active org's strip as a session-only entry, and
// (unless Activate=false) makes it the active view in one go.
type PreviewChipArgs struct {
	Domain   string   `json:"domain"`
	Scope    string   `json:"scope,omitempty"`
	Label    string   `json:"label"`
	Columns  []string `json:"columns,omitempty"`
	Clauses  string   `json:"clauses,omitempty"`
	Limit    int      `json:"limit,omitempty"`
	Activate *bool    `json:"activate,omitempty"` // nil = default true
}

// PreviewChipResult is the data block returned by chip.preview.
// Carries the minted id so the agent can save / dismiss it later
// without juggling its own correlator.
type PreviewChipResult struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
	Scope  string `json:"scope,omitempty"`
	Label  string `json:"label"`
}

// PreviewSaveChipArgs is the JSON shape of chip.preview.save. The
// caller picks new_id so settings.toml doesn't end up with the
// ULID-flavoured ephemeral key.
type PreviewSaveChipArgs struct {
	ID        string `json:"id"`
	NewID     string `json:"new_id"`
	Favourite bool   `json:"favourite,omitempty"`
}

// PreviewDismissChipArgs is the JSON shape of chip.preview.dismiss.
type PreviewDismissChipArgs struct {
	ID string `json:"id"`
}

// Server owns the listener loop and the per-instance socket file.
// It can be in three states:
//   - constructed but not Listen()ing  → no socket, no claim
//   - Listening                        → socket bound, instance claimed
//   - Closed                           → socket removed, claim released
//
// Single-writer enforcement is process-wide: only one client at a
// time can issue write commands (tab.open / chip.apply). Reads
// (state.get / state.subscribe) are unlimited.
type Server struct {
	Backend Backend
	Label   string

	mu       sync.Mutex
	listener net.Listener
	entry    instance.Entry
	closed   atomic.Bool

	writerHeld atomic.Int32 // 0 = free, 1 = held
}

// Listen claims an instance number, mints a socket at
// ~/.sf-deck/control-<N>.sock, and starts accepting connections in
// a background goroutine. Returns the claimed Entry so the caller
// can render the instance badge.
//
// Pass a context that cancels on app shutdown; the listener exits
// when the context is Done and Close() is called.
func (s *Server) Listen(ctx context.Context) (instance.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.entry, errors.New("control: already listening")
	}
	if s.Backend == nil {
		return instance.Entry{}, errors.New("control: nil Backend")
	}
	dir, err := defaultDir()
	if err != nil {
		return instance.Entry{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return instance.Entry{}, err
	}
	// Claim with a placeholder socket path so the registry picks
	// the slot number; we then mint the socket using that number
	// and re-claim with the real path.
	pid := os.Getpid()
	prelim, err := instance.Claim(pid, "", s.Label)
	if err != nil {
		return instance.Entry{}, err
	}
	socketPath := filepath.Join(dir, fmt.Sprintf("control-%d.sock", prelim.Number))
	// Drop any stale socket file from a prior crash. Best-effort —
	// if the file's owned by another process, Listen will fail
	// loudly below and the user can clear it manually.
	_ = os.Remove(socketPath)
	lst, err := net.Listen("unix", socketPath)
	if err != nil {
		_ = instance.Release(pid)
		return instance.Entry{}, err
	}
	// Lock down to 0600 so other users can't reach the socket. The
	// kernel already enforces directory perms via ~/.sf-deck/, but
	// belt and braces.
	_ = os.Chmod(socketPath, 0o600)
	entry, err := instance.Claim(pid, socketPath, s.Label)
	if err != nil {
		_ = lst.Close()
		_ = os.Remove(socketPath)
		_ = instance.Release(pid)
		return instance.Entry{}, err
	}
	s.listener = lst
	s.entry = entry
	go s.acceptLoop(ctx, lst)
	go func() {
		<-ctx.Done()
		s.Close()
	}()
	return entry, nil
}

// Close stops the listener, removes the socket file, and releases
// the instance number. Idempotent.
func (s *Server) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	s.mu.Lock()
	lst := s.listener
	entry := s.entry
	s.listener = nil
	s.mu.Unlock()
	if lst != nil {
		_ = lst.Close()
	}
	if entry.Socket != "" {
		_ = os.Remove(entry.Socket)
	}
	_ = instance.Release(os.Getpid())
	return nil
}

// Entry returns the claimed registry entry. Useful for the UI badge.
func (s *Server) Entry() instance.Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.entry
}

func (s *Server) acceptLoop(ctx context.Context, lst net.Listener) {
	for {
		conn, err := lst.Accept()
		if err != nil {
			// Listener closed → exit cleanly.
			return
		}
		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	// Allow long lines for subscribe-stream cancellation frames or
	// future inline-Apex args. 1 MiB is more than enough.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	writer := json.NewEncoder(conn)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = writer.Encode(fail(Request{}, ErrInvalidArgument,
				"malformed request: "+err.Error(), nil))
			continue
		}
		s.dispatch(ctx, req, conn, writer)
	}
}

// dispatch routes one request to its handler. Subscribe-style verbs
// take over the connection for streaming output.
func (s *Server) dispatch(ctx context.Context, req Request, conn net.Conn, w *json.Encoder) {
	switch req.Command {
	case "state.get":
		s.handleStateGet(req, w)
	case "state.subscribe":
		s.handleStateSubscribe(ctx, req, conn, w)
	case "tab.open":
		s.handleTabOpen(req, w)
	case "chip.apply":
		s.handleChipApply(req, w)
	case "org.switch":
		s.handleOrgSwitch(req, w)
	case "project.load":
		s.handleProjectLoad(req, w)
	case "project.unload":
		s.handleProjectUnload(req, w)
	case "chip.preview":
		s.handlePreviewChip(req, w)
	case "chip.preview.save":
		s.handlePreviewSaveChip(req, w)
	case "chip.preview.dismiss":
		s.handlePreviewDismissChip(req, w)
	case "bundle.list":
		s.handleBundleList(req, w)
	case "bundle.show":
		s.handleBundleShow(req, w)
	case "bundle.create":
		s.handleBundleCreate(req, w)
	case "bundle.link":
		s.handleBundleLink(req, w)
	case "bundle.retrieve":
		s.handleBundleRetrieve(req, w)
	case "bundle.validate":
		s.handleBundleValidate(req, w)
	case "bundle.deploy":
		s.handleBundleDeploy(req, w)
	case "bundle.report":
		s.handleBundleReport(req, w)
	case "bundle.delete":
		s.handleBundleDelete(req, w)
	case "project.import-bundle":
		s.handleProjectImportBundle(req, w)
	case "project.list":
		s.handleProjectList(req, w)
	case "project.show":
		s.handleProjectShow(req, w)
	case "project.create":
		s.handleProjectCreate(req, w)
	case "project.update":
		s.handleProjectUpdate(req, w)
	case "project.delete":
		s.handleProjectDelete(req, w)
	case "project.add-item":
		s.handleProjectAddItem(req, w)
	case "project.remove-item":
		s.handleProjectRemoveItem(req, w)
	case "project.items":
		s.handleProjectItems(req, w)
	case "soql.run":
		s.handleSOQLRun(req, w)
	case "soql.seed":
		s.handleSOQLSeed(req, w)
	case "soql.history.list":
		s.handleSOQLHistoryList(req, w)
	case "soql.saved.list":
		s.handleSOQLSavedList(req, w)
	case "soql.saved.show":
		s.handleSOQLSavedShow(req, w)
	case "soql.saved.create":
		s.handleSOQLSavedCreate(req, w)
	case "soql.saved.update":
		s.handleSOQLSavedUpdate(req, w)
	case "soql.saved.delete":
		s.handleSOQLSavedDelete(req, w)
	case "apex.run":
		s.handleApexRun(req, w)
	case "record.get":
		s.handleRecordGet(req, w)
	case "record.recent":
		s.handleRecordRecent(req, w)
	case "record.create":
		s.handleRecordCreate(req, w)
	case "record.update":
		s.handleRecordUpdate(req, w)
	case "record.delete":
		s.handleRecordDelete(req, w)
	case "metadata.get":
		s.handleMetadataGet(req, w)
	case "metadata.create":
		s.handleMetadataCreate(req, w)
	case "metadata.update":
		s.handleMetadataUpdate(req, w)
	case "metadata.delete":
		s.handleMetadataDelete(req, w)
	case "object.describe":
		s.handleObjectDescribe(req, w)
	case "verbs.list":
		s.handleVerbsList(req, w)
	case "report.list":
		s.handleReportList(req, w)
	case "report.run":
		s.handleReportRun(req, w)
	case "tag.list":
		s.handleTagList(req, w)
	case "tag.show":
		s.handleTagShow(req, w)
	case "tag.create":
		s.handleTagCreate(req, w)
	case "tag.update":
		s.handleTagUpdate(req, w)
	case "tag.delete":
		s.handleTagDelete(req, w)
	case "tag.apply":
		s.handleTagApply(req, w)
	case "tag.remove":
		s.handleTagRemove(req, w)
	case "tag.set":
		s.handleTagSet(req, w)
	case "org.safety.get":
		s.handleOrgSafetyGet(req, w)
	case "org.safety.set":
		s.handleOrgSafetySet(req, w)
	default:
		_ = w.Encode(fail(req, ErrMethodNotImplemented,
			"unknown command "+req.Command, map[string]any{"command": req.Command}))
	}
}

// withWriteLock guards a write command so only one client at a time
// can mutate state. The lock is per-call (acquire→write→release) so
// long-running subscriptions don't starve writers.
func (s *Server) withWriteLock(fn func() error) error {
	if !s.writerHeld.CompareAndSwap(0, 1) {
		return errBusy
	}
	defer s.writerHeld.Store(0)
	return fn()
}

var errBusy = errors.New("instance_busy")

func defaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sf-deck"), nil
}
