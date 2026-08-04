package control

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

// fakeBackend captures invocations so the tests can assert what
// the listener forwarded.
type fakeBackend struct {
	mu sync.Mutex

	state      map[string]any
	stateErr   error
	openTab    []OpenTabArgs
	openTabErr error
	chip       []ApplyChipArgs
	chipErr    error

	subCh   chan map[string]any
	subStop func()
}

func (b *fakeBackend) State() (map[string]any, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stateErr != nil {
		return nil, b.stateErr
	}
	return b.state, nil
}

func (b *fakeBackend) Subscribe() (<-chan map[string]any, func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subCh == nil {
		b.subCh = make(chan map[string]any, 4)
	}
	return b.subCh, func() {
		if b.subStop != nil {
			b.subStop()
		}
	}, nil
}

func (b *fakeBackend) OpenTab(args OpenTabArgs) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.openTab = append(b.openTab, args)
	return b.openTabErr
}

func (b *fakeBackend) ApplyChip(args ApplyChipArgs) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.chip = append(b.chip, args)
	return b.chipErr
}

func (b *fakeBackend) SwitchOrg(args SwitchOrgArgs) error     { return nil }
func (b *fakeBackend) LoadProject(args LoadProjectArgs) error { return nil }

func (b *fakeBackend) PreviewChip(args PreviewChipArgs) (PreviewChipResult, error) {
	return PreviewChipResult{ID: "__eph_test__", Domain: args.Domain, Scope: args.Scope, Label: args.Label}, nil
}
func (b *fakeBackend) PreviewSaveChip(args PreviewSaveChipArgs) error       { return nil }
func (b *fakeBackend) PreviewDismissChip(args PreviewDismissChipArgs) error { return nil }

// Bundle / project.import-bundle stubs. Tests don't exercise them yet
// (they'd need a devproject.Store fixture); the impls below just
// satisfy the interface so the existing tests still build.
func (b *fakeBackend) BundleList(args BundleListArgs) ([]any, error)   { return nil, nil }
func (b *fakeBackend) BundleShow(args BundleShowArgs) (any, error)     { return nil, nil }
func (b *fakeBackend) BundleCreate(args BundleCreateArgs) (any, error) { return nil, nil }
func (b *fakeBackend) BundleLink(args BundleLinkArgs) (any, error)     { return nil, nil }
func (b *fakeBackend) BundleRetrieve(args BundleRetrieveArgs) (any, error) {
	return nil, nil
}
func (b *fakeBackend) BundleValidate(args BundleValidateArgs) (any, error) {
	return nil, nil
}
func (b *fakeBackend) BundleDeploy(args BundleDeployArgs) (any, error) { return nil, nil }
func (b *fakeBackend) BundleReport(args BundleReportArgs) (any, error) { return nil, nil }
func (b *fakeBackend) BundleDelete(args BundleDeleteArgs) error        { return nil }
func (b *fakeBackend) ProjectImportBundle(args ProjectImportBundleArgs) (any, error) {
	return nil, nil
}

// project.* stubs
func (b *fakeBackend) ProjectList(args ProjectListArgs) ([]any, error)     { return nil, nil }
func (b *fakeBackend) ProjectShow(args ProjectShowArgs) (any, error)       { return nil, nil }
func (b *fakeBackend) ProjectCreate(args ProjectCreateArgs) (any, error)   { return nil, nil }
func (b *fakeBackend) ProjectUpdate(args ProjectUpdateArgs) (any, error)   { return nil, nil }
func (b *fakeBackend) ProjectDelete(args ProjectDeleteArgs) (any, error)   { return nil, nil }
func (b *fakeBackend) ProjectAddItem(args ProjectAddItemArgs) (any, error) { return nil, nil }
func (b *fakeBackend) ProjectRemoveItem(args ProjectRemoveItemArgs) (any, error) {
	return nil, nil
}
func (b *fakeBackend) ProjectItems(args ProjectItemsArgs) ([]any, error) { return nil, nil }

// data-plane / meta-plane stubs (soql/apex/record/metadata/object/tag/safety)
func (b *fakeBackend) SOQLRun(args SOQLRunArgs) (any, error)                   { return nil, nil }
func (b *fakeBackend) SOQLSeed(args SOQLSeedArgs) error                        { return nil }
func (b *fakeBackend) SOQLHistoryList(args SOQLHistoryListArgs) ([]any, error) { return nil, nil }
func (b *fakeBackend) SOQLSavedList(args SOQLSavedListArgs) ([]any, error)     { return nil, nil }
func (b *fakeBackend) SOQLSavedShow(args SOQLSavedShowArgs) (any, error)       { return nil, nil }
func (b *fakeBackend) SOQLSavedCreate(args SOQLSavedCreateArgs) (any, error)   { return nil, nil }
func (b *fakeBackend) SOQLSavedUpdate(args SOQLSavedUpdateArgs) (any, error)   { return nil, nil }
func (b *fakeBackend) SOQLSavedDelete(args SOQLSavedDeleteArgs) (any, error)   { return nil, nil }
func (b *fakeBackend) ApexRun(args ApexRunArgs) (any, error)                   { return nil, nil }
func (b *fakeBackend) RecordGet(args RecordGetArgs) (any, error)               { return nil, nil }
func (b *fakeBackend) RecordRecent(args RecordRecentArgs) (any, error)         { return nil, nil }
func (b *fakeBackend) RecordCreate(args RecordCreateArgs) (any, error)         { return nil, nil }
func (b *fakeBackend) RecordUpdate(args RecordUpdateArgs) (any, error)         { return nil, nil }
func (b *fakeBackend) RecordDelete(args RecordDeleteArgs) (any, error)         { return nil, nil }
func (b *fakeBackend) MetadataGet(args MetadataGetArgs) (any, error)           { return nil, nil }
func (b *fakeBackend) MetadataCreate(args MetadataCreateArgs) (any, error)     { return nil, nil }
func (b *fakeBackend) MetadataUpdate(args MetadataUpdateArgs) (any, error)     { return nil, nil }
func (b *fakeBackend) MetadataDelete(args MetadataDeleteArgs) (any, error)     { return nil, nil }
func (b *fakeBackend) ObjectDescribe(args ObjectDescribeArgs) (any, error)     { return nil, nil }
func (b *fakeBackend) VerbsList(args VerbsListArgs) ([]any, error)             { return nil, nil }
func (b *fakeBackend) TagList(args TagListArgs) ([]any, error)                 { return nil, nil }
func (b *fakeBackend) TagShow(args TagShowArgs) (any, error)                   { return nil, nil }
func (b *fakeBackend) TagCreate(args TagCreateArgs) (any, error)               { return nil, nil }
func (b *fakeBackend) TagUpdate(args TagUpdateArgs) (any, error)               { return nil, nil }
func (b *fakeBackend) TagDelete(args TagDeleteArgs) (any, error)               { return nil, nil }
func (b *fakeBackend) TagApply(args TagApplyArgs) (any, error)                 { return nil, nil }
func (b *fakeBackend) TagRemove(args TagRemoveArgs) (any, error)               { return nil, nil }
func (b *fakeBackend) TagSet(args TagSetArgs) (any, error)                     { return nil, nil }
func (b *fakeBackend) ReportList(args ReportListArgs) ([]any, error)           { return nil, nil }
func (b *fakeBackend) ReportRun(args ReportRunArgs) (any, error)               { return nil, nil }
func (b *fakeBackend) OrgSafetyGet(args OrgSafetyGetArgs) (any, error)         { return nil, nil }
func (b *fakeBackend) OrgSafetySet(args OrgSafetySetArgs) (any, error)         { return nil, nil }

// shortHome returns a home dir whose .sf-deck/control-1.sock path
// stays under macOS's 104-byte sun_path limit. t.TempDir() lives
// under /var/folders/... which already blows the budget.
func shortHome(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "sfd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func setupServer(t *testing.T) (*Server, *fakeBackend, net.Conn, context.CancelFunc) {
	t.Helper()
	t.Setenv("HOME", shortHome(t))
	be := &fakeBackend{state: map[string]any{"tab": "home"}}
	srv := &Server{Backend: be}
	ctx, cancel := context.WithCancel(context.Background())
	entry, err := srv.Listen(ctx)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	conn, err := net.Dial("unix", entry.Socket)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		cancel()
		srv.Close()
	})
	return srv, be, conn, cancel
}

func sendReq(t *testing.T, conn net.Conn, req Request) Response {
	t.Helper()
	enc := json.NewEncoder(conn)
	if err := enc.Encode(req); err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Read one response line.
	dec := json.NewDecoder(conn)
	var resp Response
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

func TestStateGet_ReturnsBackendSnapshot(t *testing.T) {
	_, _, conn, _ := setupServer(t)
	resp := sendReq(t, conn, Request{Command: "state.get"})
	if !resp.OK {
		t.Fatalf("not ok: %+v", resp.Error)
	}
	data, _ := resp.Data.(map[string]any)
	if data["tab"] != "home" {
		t.Errorf("expected tab=home, got %v", data["tab"])
	}
}

func TestTabOpen_DispatchesToBackend(t *testing.T) {
	_, be, conn, _ := setupServer(t)
	args, _ := json.Marshal(OpenTabArgs{Tab: "records", SObject: "Account"})
	resp := sendReq(t, conn, Request{Command: "tab.open", Args: args})
	if !resp.OK {
		t.Fatalf("not ok: %+v", resp.Error)
	}
	if !resp.Changed {
		t.Errorf("expected Changed=true")
	}
	be.mu.Lock()
	defer be.mu.Unlock()
	if len(be.openTab) != 1 || be.openTab[0].SObject != "Account" {
		t.Errorf("OpenTab not invoked correctly: %+v", be.openTab)
	}
}

func TestTabOpen_RejectsMissingTab(t *testing.T) {
	_, _, conn, _ := setupServer(t)
	resp := sendReq(t, conn, Request{Command: "tab.open"})
	if resp.OK {
		t.Fatal("expected failure")
	}
	if resp.Error.Code != ErrInvalidArgument {
		t.Errorf("code = %q, want %q", resp.Error.Code, ErrInvalidArgument)
	}
}

func TestChipApply_DispatchesToBackend(t *testing.T) {
	_, be, conn, _ := setupServer(t)
	args, _ := json.Marshal(ApplyChipArgs{Domain: "flows", ID: "__project__"})
	resp := sendReq(t, conn, Request{Command: "chip.apply", Args: args})
	if !resp.OK {
		t.Fatalf("not ok: %+v", resp.Error)
	}
	be.mu.Lock()
	defer be.mu.Unlock()
	if len(be.chip) != 1 || be.chip[0].Domain != "flows" {
		t.Errorf("ApplyChip not invoked: %+v", be.chip)
	}
}

func TestUnknownCommand_ReturnsMethodNotImplemented(t *testing.T) {
	_, _, conn, _ := setupServer(t)
	resp := sendReq(t, conn, Request{Command: "frobnicate"})
	if resp.OK {
		t.Fatal("expected failure")
	}
	if resp.Error.Code != ErrMethodNotImplemented {
		t.Errorf("code = %q", resp.Error.Code)
	}
}

func TestMalformedJSON_ReturnsInvalidArgument(t *testing.T) {
	_, _, conn, _ := setupServer(t)
	// Send a bare invalid line.
	_, _ = conn.Write([]byte("not-a-json\n"))
	dec := json.NewDecoder(conn)
	var resp Response
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.OK || resp.Error.Code != ErrInvalidArgument {
		t.Errorf("expected invalid_argument, got %+v", resp.Error)
	}
}

func TestIDIsEchoed(t *testing.T) {
	_, _, conn, _ := setupServer(t)
	resp := sendReq(t, conn, Request{ID: "abc123", Command: "state.get"})
	if resp.ID != "abc123" {
		t.Errorf("ID round-trip failed: %q", resp.ID)
	}
}

func TestStateSubscribe_StreamsInitialSnapshotThenUpdates(t *testing.T) {
	_, be, conn, _ := setupServer(t)
	// Issue subscribe — should get an initial snapshot promptly.
	enc := json.NewEncoder(conn)
	if err := enc.Encode(Request{Command: "state.subscribe"}); err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bufio.NewReader(conn))
	var resp Response
	if err := dec.Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Command != "state.subscribe" {
		t.Fatalf("unexpected initial: %+v", resp)
	}
	data, _ := resp.Data.(map[string]any)
	if data["tab"] != "home" {
		t.Errorf("initial tab = %v", data["tab"])
	}
	// Push an update via the fake backend.
	be.mu.Lock()
	be.state = map[string]any{"tab": "records"}
	be.subCh <- be.state
	be.mu.Unlock()
	// Read the streamed update.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := dec.Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ = resp.Data.(map[string]any)
	if data["tab"] != "records" {
		t.Errorf("streamed tab = %v", data["tab"])
	}
}

func TestListen_ClaimsInstanceAndRemovesSocketOnClose(t *testing.T) {
	t.Setenv("HOME", shortHome(t))
	srv := &Server{Backend: &fakeBackend{state: map[string]any{}}}
	ctx, cancel := context.WithCancel(context.Background())
	entry, err := srv.Listen(ctx)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	if entry.Number != 1 {
		t.Errorf("expected instance #1, got %d", entry.Number)
	}
	if _, err := net.Dial("unix", entry.Socket); err != nil {
		t.Errorf("socket not bound: %v", err)
	}
	cancel()
	srv.Close()
	// Socket file should be gone.
	if _, err := net.Dial("unix", entry.Socket); err == nil {
		t.Error("expected dial to fail after Close()")
	}
}
