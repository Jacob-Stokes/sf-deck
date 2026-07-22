package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

// captureBackend is a richer test backend than fakeBackend in
// listener_test.go: it records every call's args, exposes per-method
// return-value/error knobs, and can drive coded errors. Used for the
// handler-table tests that exercise dispatch + validation + error
// propagation.
//
// Methods left as inert stubs in fakeBackend (BundleList etc.) get
// real implementations here. The two coexist: fakeBackend stays for
// the listener-level integration tests; captureBackend backs the
// handler-table tests.
type captureBackend struct {
	mu sync.Mutex

	// Call counters keyed by method name. Tests assert "this verb
	// caused exactly one call to X".
	calls map[string]int

	// Last args seen per method. Tests assert dispatch shape.
	lastArgs map[string]any

	// Knobs: per-method error + return value. The handler under
	// test is named by the verb (e.g. "project.list").
	errors map[string]error
	values map[string]any
}

func newCapture() *captureBackend {
	return &captureBackend{
		calls:    map[string]int{},
		lastArgs: map[string]any{},
		errors:   map[string]error{},
		values:   map[string]any{},
	}
}

func (b *captureBackend) record(method string, args any) (any, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls[method]++
	b.lastArgs[method] = args
	if err := b.errors[method]; err != nil {
		return nil, err
	}
	return b.values[method], nil
}

func (b *captureBackend) recordList(method string, args any) ([]any, error) {
	v, err := b.record(method, args)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	return v.([]any), nil
}

// --- Backend interface ---

func (b *captureBackend) State() (map[string]any, error) {
	v, err := b.record("state.get", nil)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return map[string]any{}, nil
	}
	return v.(map[string]any), nil
}

func (b *captureBackend) Subscribe() (<-chan map[string]any, func(), error) {
	ch := make(chan map[string]any, 1)
	close(ch)
	return ch, func() {}, nil
}

func (b *captureBackend) OpenTab(a OpenTabArgs) error {
	_, err := b.record("tab.open", a)
	return err
}

func (b *captureBackend) ApplyChip(a ApplyChipArgs) error {
	_, err := b.record("chip.apply", a)
	return err
}

func (b *captureBackend) SwitchOrg(a SwitchOrgArgs) error {
	_, err := b.record("org.switch", a)
	return err
}

func (b *captureBackend) LoadProject(a LoadProjectArgs) error {
	_, err := b.record("project.load", a)
	return err
}

func (b *captureBackend) PreviewChip(a PreviewChipArgs) (PreviewChipResult, error) {
	_, err := b.record("chip.preview", a)
	if err != nil {
		return PreviewChipResult{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if v, ok := b.values["chip.preview"]; ok && v != nil {
		return v.(PreviewChipResult), nil
	}
	return PreviewChipResult{ID: "__eph_test__", Domain: a.Domain, Scope: a.Scope, Label: a.Label}, nil
}

func (b *captureBackend) PreviewSaveChip(a PreviewSaveChipArgs) error {
	_, err := b.record("chip.preview.save", a)
	return err
}

func (b *captureBackend) PreviewDismissChip(a PreviewDismissChipArgs) error {
	_, err := b.record("chip.preview.dismiss", a)
	return err
}

func (b *captureBackend) BundleList(a BundleListArgs) ([]any, error) {
	return b.recordList("bundle.list", a)
}
func (b *captureBackend) BundleShow(a BundleShowArgs) (any, error) { return b.record("bundle.show", a) }
func (b *captureBackend) BundleCreate(a BundleCreateArgs) (any, error) {
	return b.record("bundle.create", a)
}
func (b *captureBackend) BundleLink(a BundleLinkArgs) (any, error) { return b.record("bundle.link", a) }
func (b *captureBackend) BundleRetrieve(a BundleRetrieveArgs) (any, error) {
	return b.record("bundle.retrieve", a)
}
func (b *captureBackend) BundleValidate(a BundleValidateArgs) (any, error) {
	return b.record("bundle.validate", a)
}
func (b *captureBackend) BundleDeploy(a BundleDeployArgs) (any, error) {
	return b.record("bundle.deploy", a)
}
func (b *captureBackend) BundleReport(a BundleReportArgs) (any, error) {
	return b.record("bundle.report", a)
}
func (b *captureBackend) BundleDelete(a BundleDeleteArgs) error {
	_, err := b.record("bundle.delete", a)
	return err
}
func (b *captureBackend) ProjectImportBundle(a ProjectImportBundleArgs) (any, error) {
	return b.record("project.import-bundle", a)
}

func (b *captureBackend) ProjectList(a ProjectListArgs) ([]any, error) {
	return b.recordList("project.list", a)
}
func (b *captureBackend) ProjectShow(a ProjectShowArgs) (any, error) {
	return b.record("project.show", a)
}
func (b *captureBackend) ProjectCreate(a ProjectCreateArgs) (any, error) {
	return b.record("project.create", a)
}
func (b *captureBackend) ProjectUpdate(a ProjectUpdateArgs) (any, error) {
	return b.record("project.update", a)
}
func (b *captureBackend) ProjectDelete(a ProjectDeleteArgs) (any, error) {
	return b.record("project.delete", a)
}
func (b *captureBackend) ProjectAddItem(a ProjectAddItemArgs) (any, error) {
	return b.record("project.add-item", a)
}
func (b *captureBackend) ProjectRemoveItem(a ProjectRemoveItemArgs) (any, error) {
	return b.record("project.remove-item", a)
}
func (b *captureBackend) ProjectItems(a ProjectItemsArgs) ([]any, error) {
	return b.recordList("project.items", a)
}

func (b *captureBackend) SOQLRun(a SOQLRunArgs) (any, error) { return b.record("soql.run", a) }
func (b *captureBackend) SOQLSeed(a SOQLSeedArgs) error      { _, e := b.record("soql.seed", a); return e }
func (b *captureBackend) SOQLHistoryList(a SOQLHistoryListArgs) ([]any, error) {
	return b.recordList("soql.history.list", a)
}
func (b *captureBackend) SOQLSavedList(a SOQLSavedListArgs) ([]any, error) {
	return b.recordList("soql.saved.list", a)
}
func (b *captureBackend) SOQLSavedShow(a SOQLSavedShowArgs) (any, error) {
	return b.record("soql.saved.show", a)
}
func (b *captureBackend) SOQLSavedCreate(a SOQLSavedCreateArgs) (any, error) {
	return b.record("soql.saved.create", a)
}
func (b *captureBackend) SOQLSavedUpdate(a SOQLSavedUpdateArgs) (any, error) {
	return b.record("soql.saved.update", a)
}
func (b *captureBackend) SOQLSavedDelete(a SOQLSavedDeleteArgs) (any, error) {
	return b.record("soql.saved.delete", a)
}
func (b *captureBackend) ApexRun(a ApexRunArgs) (any, error) { return b.record("apex.run", a) }
func (b *captureBackend) RecordGet(a RecordGetArgs) (any, error) {
	return b.record("record.get", a)
}
func (b *captureBackend) RecordRecent(a RecordRecentArgs) (any, error) {
	return b.record("record.recent", a)
}
func (b *captureBackend) RecordCreate(a RecordCreateArgs) (any, error) {
	return b.record("record.create", a)
}
func (b *captureBackend) RecordUpdate(a RecordUpdateArgs) (any, error) {
	return b.record("record.update", a)
}
func (b *captureBackend) RecordDelete(a RecordDeleteArgs) (any, error) {
	return b.record("record.delete", a)
}
func (b *captureBackend) MetadataGet(a MetadataGetArgs) (any, error) {
	return b.record("metadata.get", a)
}
func (b *captureBackend) MetadataCreate(a MetadataCreateArgs) (any, error) {
	return b.record("metadata.create", a)
}
func (b *captureBackend) MetadataUpdate(a MetadataUpdateArgs) (any, error) {
	return b.record("metadata.update", a)
}
func (b *captureBackend) MetadataDelete(a MetadataDeleteArgs) (any, error) {
	return b.record("metadata.delete", a)
}
func (b *captureBackend) ObjectDescribe(a ObjectDescribeArgs) (any, error) {
	return b.record("object.describe", a)
}
func (b *captureBackend) VerbsList(a VerbsListArgs) ([]any, error) {
	return b.recordList("verbs.list", a)
}
func (b *captureBackend) TagList(a TagListArgs) ([]any, error) { return b.recordList("tag.list", a) }
func (b *captureBackend) TagShow(a TagShowArgs) (any, error)   { return b.record("tag.show", a) }
func (b *captureBackend) TagCreate(a TagCreateArgs) (any, error) {
	return b.record("tag.create", a)
}
func (b *captureBackend) TagUpdate(a TagUpdateArgs) (any, error) {
	return b.record("tag.update", a)
}
func (b *captureBackend) TagDelete(a TagDeleteArgs) (any, error) {
	return b.record("tag.delete", a)
}
func (b *captureBackend) TagApply(a TagApplyArgs) (any, error) { return b.record("tag.apply", a) }
func (b *captureBackend) TagRemove(a TagRemoveArgs) (any, error) {
	return b.record("tag.remove", a)
}
func (b *captureBackend) TagSet(a TagSetArgs) (any, error) { return b.record("tag.set", a) }
func (b *captureBackend) ReportList(a ReportListArgs) ([]any, error) {
	return b.recordList("report.list", a)
}
func (b *captureBackend) ReportRun(a ReportRunArgs) (any, error) {
	return b.record("report.run", a)
}
func (b *captureBackend) OrgSafetyGet(a OrgSafetyGetArgs) (any, error) {
	return b.record("org.safety.get", a)
}
func (b *captureBackend) OrgSafetySet(a OrgSafetySetArgs) (any, error) {
	return b.record("org.safety.set", a)
}

// --- helpers ---

// dispatch runs a request through the listener's command switch
// directly (no socket). Captures the first response the handler
// writes and returns it. State-subscribe-style streaming verbs
// aren't safe to call this way — they need a real net.Conn and a
// loop — but every other verb writes exactly one Response then
// returns, so a bytes.Buffer + a single Decode is enough.
func dispatch(t *testing.T, srv *Server, req Request) Response {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	srv.dispatch(context.Background(), req, nil, enc)
	dec := json.NewDecoder(&buf)
	var resp Response
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

// codedError is a Backend error type that surfaces a custom Code()
// through the encodeBackendErr coded-interface path.
type codedError struct{ code, msg string }

func (e *codedError) Error() string { return e.msg }
func (e *codedError) Code() string  { return e.code }

// ------ Validation tests: missing required fields ------

func TestHandler_ValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		command string
		args    any // nil = empty args
	}{
		{"tab.open missing tab", "tab.open", nil},
		{"chip.apply missing domain+id", "chip.apply", nil},
		{"chip.apply missing id", "chip.apply", ApplyChipArgs{Domain: "records"}},
		{"org.switch missing both", "org.switch", nil},
		{"project.load missing id", "project.load", nil},
		{"chip.preview missing domain+label", "chip.preview", nil},
		{"chip.preview.save missing ids", "chip.preview.save", nil},
		{"chip.preview.dismiss missing id", "chip.preview.dismiss", nil},

		{"bundle.show missing id", "bundle.show", nil},
		{"bundle.create missing project_id", "bundle.create", nil},
		{"bundle.link missing fields", "bundle.link", nil},
		{"bundle.retrieve missing id", "bundle.retrieve", nil},
		{"bundle.validate missing id", "bundle.validate", nil},
		{"bundle.deploy missing id", "bundle.deploy", nil},
		{"bundle.report missing id", "bundle.report", nil},
		{"bundle.delete missing id", "bundle.delete", nil},

		{"project.show missing both", "project.show", nil},
		{"project.create missing name", "project.create", nil},
		{"project.update missing id", "project.update", nil},
		{"project.delete missing id", "project.delete", nil},
		{"project.add-item missing fields", "project.add-item", nil},
		{"project.remove-item missing fields", "project.remove-item", nil},
		{"project.items missing project_id", "project.items", nil},
		{"project.import-bundle missing fields", "project.import-bundle", nil},

		{"soql.run missing query", "soql.run", nil},
		{"soql.seed missing query", "soql.seed", nil},
		{"soql.saved.show missing id", "soql.saved.show", nil},
		{"soql.saved.create missing fields", "soql.saved.create", nil},
		{"soql.saved.update missing id", "soql.saved.update", nil},
		{"soql.saved.delete missing id", "soql.saved.delete", nil},

		{"apex.run missing body", "apex.run", nil},

		{"record.get missing fields", "record.get", nil},
		{"record.create missing fields", "record.create", nil},
		{"record.recent missing sobject", "record.recent", nil},
		{"record.update missing fields", "record.update", nil},
		{"record.delete missing fields", "record.delete", nil},

		{"metadata.get missing fields", "metadata.get", nil},
		{"metadata.create missing fields", "metadata.create", nil},
		{"metadata.update missing fields", "metadata.update", nil},
		{"metadata.delete missing fields", "metadata.delete", nil},
		{"object.describe missing sobject", "object.describe", nil},

		{"report.run missing id", "report.run", nil},

		{"tag.show missing fields", "tag.show", nil},
		{"tag.create missing fields", "tag.create", nil},
		{"tag.update missing id", "tag.update", nil},
		{"tag.delete missing id", "tag.delete", nil},
		{"tag.apply missing fields", "tag.apply", nil},
		{"tag.remove missing fields", "tag.remove", nil},
		{"tag.set missing fields", "tag.set", nil},

		// org.safety.get allows empty args (caller asks "what's the default?")
		{"org.safety.set missing org", "org.safety.set", nil},
	}

	srv := &Server{Backend: newCapture()}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := Request{Command: tc.command}
			if tc.args != nil {
				b, _ := json.Marshal(tc.args)
				req.Args = b
			}
			resp := dispatch(t, srv, req)
			if resp.OK {
				t.Fatalf("expected failure, got OK with data=%v", resp.Data)
			}
			if resp.Error == nil || resp.Error.Code != ErrInvalidArgument {
				t.Errorf("code = %v, want invalid_argument", resp.Error)
			}
		})
	}
}

// ------ Malformed args (JSON-level) ------

func TestHandler_MalformedArgs(t *testing.T) {
	srv := &Server{Backend: newCapture()}
	// Truly malformed JSON in args: a bare string where a struct is
	// expected.
	req := Request{Command: "tab.open", Args: json.RawMessage(`"not-a-struct"`)}
	resp := dispatch(t, srv, req)
	if resp.OK {
		t.Fatal("expected failure")
	}
	if resp.Error.Code != ErrInvalidArgument {
		t.Errorf("code = %q, want invalid_argument", resp.Error.Code)
	}
}

// ------ Dispatch + response shape ------

func TestHandler_DispatchAndShape(t *testing.T) {
	type tc struct {
		name        string
		command     string
		args        any
		mockValue   any  // value the backend should return
		wantChanged bool // expected Changed flag
		wantKey     string
		// optional: assertion on what the backend captured. Receives the
		// args type the handler unmarshalled into.
		assertArgs func(*testing.T, any)
	}

	cases := []tc{
		// Reads: not Changed; payload usually nested under a noun key.
		{
			name: "state.get returns state", command: "state.get",
			mockValue: map[string]any{"tab": "records"},
		},
		{
			name: "bundle.list returns wrapped list", command: "bundle.list",
			args:      BundleListArgs{ProjectID: "p1"},
			mockValue: []any{map[string]any{"id": "b1"}, map[string]any{"id": "b2"}},
			wantKey:   "bundles",
			assertArgs: func(t *testing.T, a any) {
				if a.(BundleListArgs).ProjectID != "p1" {
					t.Errorf("ProjectID not forwarded: %+v", a)
				}
			},
		},
		{
			name: "project.list returns count", command: "project.list",
			mockValue: []any{map[string]any{"id": "p1"}},
			wantKey:   "projects",
		},
		{
			name: "project.show returns project", command: "project.show",
			args:      ProjectShowArgs{ID: "p1"},
			mockValue: map[string]any{"id": "p1", "name": "demo"},
			wantKey:   "project",
		},
		{
			name: "verbs.list returns verbs", command: "verbs.list",
			mockValue: []any{},
			wantKey:   "verbs",
		},
		{
			name: "object.describe returns describe", command: "object.describe",
			args:      ObjectDescribeArgs{SObject: "Account"},
			mockValue: map[string]any{"name": "Account"},
			wantKey:   "describe",
		},
		{
			name: "tag.list returns tags", command: "tag.list",
			mockValue: []any{},
			wantKey:   "tags",
		},
		{
			name: "soql.history.list returns history", command: "soql.history.list",
			mockValue: []any{},
			wantKey:   "history",
		},
		{
			name: "soql.saved.list returns saved", command: "soql.saved.list",
			mockValue: []any{},
			wantKey:   "saved",
		},
		{
			name: "record.recent returns records", command: "record.recent",
			args:      RecordRecentArgs{SObject: "Account"},
			mockValue: map[string]any{"records": []any{}},
			wantKey:   "result",
		},
		{
			name: "report.list returns reports", command: "report.list",
			mockValue: []any{},
			wantKey:   "reports",
		},
		{
			name: "org.safety.get returns safety", command: "org.safety.get",
			mockValue: map[string]any{"level": "read_only"},
			wantKey:   "safety",
		},

		// Writes: Changed=true; payload wrapped under noun key.
		{
			name: "project.create sets Changed", command: "project.create",
			args:        ProjectCreateArgs{Name: "demo"},
			mockValue:   map[string]any{"id": "p1", "name": "demo"},
			wantChanged: true,
			wantKey:     "project",
		},
		{
			name: "project.update sets Changed", command: "project.update",
			args:        ProjectUpdateArgs{ID: "p1", Name: strPtr("renamed")},
			mockValue:   map[string]any{"id": "p1"},
			wantChanged: true,
			wantKey:     "project",
		},
		{
			name: "project.delete sets Changed", command: "project.delete",
			args:        ProjectDeleteArgs{ID: "p1"},
			mockValue:   map[string]any{"deleted": "p1"},
			wantChanged: true,
		},
		{
			name: "tag.create sets Changed", command: "tag.create",
			args:        TagCreateArgs{Name: "to-review"},
			mockValue:   map[string]any{"id": "t1"},
			wantChanged: true,
			wantKey:     "tag",
		},
		{
			name: "bundle.create sets Changed", command: "bundle.create",
			args:        BundleCreateArgs{ProjectID: "p1"},
			mockValue:   map[string]any{"id": "b1"},
			wantChanged: true,
			wantKey:     "result",
		},
		{
			name: "tab.open sets Changed without payload", command: "tab.open",
			args:        OpenTabArgs{Tab: "records"},
			wantChanged: true,
		},
		{
			name: "chip.apply sets Changed", command: "chip.apply",
			args:        ApplyChipArgs{Domain: "records", ID: "__sf_recent__"},
			wantChanged: true,
		},
		{
			name: "org.switch sets Changed", command: "org.switch",
			args:        SwitchOrgArgs{Alias: "dev"},
			wantChanged: true,
		},
		{
			name: "soql.run returns result", command: "soql.run",
			args:      SOQLRunArgs{Query: "SELECT Id FROM Account"},
			mockValue: map[string]any{"records": []any{}},
			wantKey:   "result",
		},
		{
			name: "record.get returns record", command: "record.get",
			args:      RecordGetArgs{SObject: "Account", ID: "001x"},
			mockValue: map[string]any{"Id": "001x"},
			wantKey:   "record",
		},
		{
			name: "metadata.get returns result", command: "metadata.get",
			args:      MetadataGetArgs{Type: "Flow", FullName: "MyFlow"},
			mockValue: map[string]any{"fullName": "MyFlow"},
			wantKey:   "result",
		},

		// More writes — these go through withWriteLock, so the
		// success-path coverage needs a valid args struct.
		{
			name: "apex.run sets Changed", command: "apex.run",
			args:        ApexRunArgs{Body: "System.debug('x');"},
			mockValue:   map[string]any{"success": true},
			wantChanged: true,
			wantKey:     "result",
		},
		{
			name: "bundle.link sets Changed", command: "bundle.link",
			args:        BundleLinkArgs{ProjectID: "p1", Path: "/tmp/b1"},
			mockValue:   map[string]any{"id": "b1"},
			wantChanged: true,
			wantKey:     "bundle",
		},
		{
			name: "bundle.retrieve sets Changed", command: "bundle.retrieve",
			args:        BundleRetrieveArgs{ID: "b1"},
			mockValue:   map[string]any{"id": "b1"},
			wantChanged: true,
			wantKey:     "result",
		},
		{
			name: "bundle.validate returns result (no Changed: check-only)", command: "bundle.validate",
			args:      BundleValidateArgs{ID: "b1", OrgAlias: "dev"},
			mockValue: map[string]any{"deploy_id": "0Af..."},
			wantKey:   "result",
		},
		{
			name: "bundle.deploy sets Changed", command: "bundle.deploy",
			args:        BundleDeployArgs{ID: "b1", OrgAlias: "dev"},
			mockValue:   map[string]any{"deploy_id": "0Af..."},
			wantChanged: true,
			wantKey:     "result",
		},
		{
			name: "bundle.report returns result", command: "bundle.report",
			args:      BundleReportArgs{ID: "b1", OrgAlias: "dev", DeployID: "0Af..."},
			mockValue: map[string]any{"status": "Succeeded"},
			wantKey:   "result",
		},
		{
			name: "bundle.delete sets Changed", command: "bundle.delete",
			args:        BundleDeleteArgs{ID: "b1"},
			wantChanged: true,
		},
		{
			name: "project.import-bundle sets Changed", command: "project.import-bundle",
			args:        ProjectImportBundleArgs{ProjectID: "p1", Path: "/tmp/b1", OrgUser: "u@x.com"},
			mockValue:   map[string]any{"added": 5},
			wantChanged: true,
			wantKey:     "result",
		},
		{
			name: "project.items returns items", command: "project.items",
			args:      ProjectItemsArgs{ID: "p1"},
			mockValue: []any{},
			wantKey:   "items",
		},
		{
			name: "project.add-item sets Changed", command: "project.add-item",
			args:        ProjectAddItemArgs{ProjectID: "p1", Kind: "flow", Ref: "MyFlow"},
			mockValue:   map[string]any{"id": "i1"},
			wantChanged: true,
			wantKey:     "item",
		},
		{
			name: "project.remove-item sets Changed", command: "project.remove-item",
			args:        ProjectRemoveItemArgs{ProjectID: "p1", Kind: "flow", Ref: "MyFlow"},
			wantChanged: true,
		},
		{
			name: "metadata.create sets Changed", command: "metadata.create",
			args:        MetadataCreateArgs{Type: "ApexClass", FullName: "X"},
			mockValue:   map[string]any{"id": "01p..."},
			wantChanged: true,
			wantKey:     "result",
		},
		{
			name: "metadata.update sets Changed", command: "metadata.update",
			args:        MetadataUpdateArgs{Type: "ApexClass", ID: "01p..."},
			mockValue:   map[string]any{"id": "01p..."},
			wantChanged: true,
			wantKey:     "result",
		},
		{
			name: "metadata.delete sets Changed", command: "metadata.delete",
			args:        MetadataDeleteArgs{Type: "ApexClass", ID: "01p..."},
			mockValue:   map[string]any{"deleted": "01p..."},
			wantChanged: true,
			wantKey:     "result",
		},
		{
			name: "soql.saved.create sets Changed", command: "soql.saved.create",
			args:        SOQLSavedCreateArgs{Name: "Open opps", Body: "SELECT Id FROM Opportunity"},
			mockValue:   map[string]any{"id": "sq1"},
			wantChanged: true,
			wantKey:     "saved",
		},
		{
			name: "soql.saved.update sets Changed", command: "soql.saved.update",
			args:        SOQLSavedUpdateArgs{ID: "sq1", Name: strPtr("renamed")},
			mockValue:   map[string]any{"id": "sq1"},
			wantChanged: true,
			wantKey:     "saved",
		},
		{
			name: "soql.saved.delete sets Changed", command: "soql.saved.delete",
			args:        SOQLSavedDeleteArgs{ID: "sq1"},
			mockValue:   map[string]any{"deleted": "sq1"},
			wantChanged: true,
			wantKey:     "result",
		},
		{
			name: "record.create sets Changed", command: "record.create",
			args:        RecordCreateArgs{SObject: "Account", Fields: map[string]any{"Name": "X"}},
			mockValue:   map[string]any{"id": "001x"},
			wantChanged: true,
			wantKey:     "result",
		},
		{
			name: "record.update sets Changed", command: "record.update",
			args:        RecordUpdateArgs{SObject: "Account", ID: "001x", Fields: map[string]any{"Name": "X"}},
			mockValue:   map[string]any{"id": "001x"},
			wantChanged: true,
			wantKey:     "result",
		},
		{
			name: "record.delete sets Changed", command: "record.delete",
			args:        RecordDeleteArgs{SObject: "Account", ID: "001x"},
			mockValue:   map[string]any{"deleted": "001x"},
			wantChanged: true,
			wantKey:     "result",
		},
		{
			name: "tag.update sets Changed", command: "tag.update",
			args:        TagUpdateArgs{ID: 1, Name: strPtr("renamed")},
			mockValue:   map[string]any{"id": 1},
			wantChanged: true,
			wantKey:     "tag",
		},
		{
			name: "tag.delete sets Changed", command: "tag.delete",
			args:        TagDeleteArgs{ID: 1},
			mockValue:   map[string]any{"deleted": 1},
			wantChanged: true,
			wantKey:     "result",
		},
		{
			name: "tag.apply sets Changed", command: "tag.apply",
			args:        TagApplyArgs{TagID: 1, Kind: "flow", Ref: "MyFlow"},
			mockValue:   map[string]any{"id": 1},
			wantChanged: true,
			wantKey:     "result",
		},
		{
			name: "tag.remove sets Changed", command: "tag.remove",
			args:        TagRemoveArgs{TagID: 1, Kind: "flow", Ref: "MyFlow"},
			mockValue:   map[string]any{"removed": 1},
			wantChanged: true,
			wantKey:     "result",
		},
		{
			name: "tag.set sets Changed", command: "tag.set",
			args:        TagSetArgs{Kind: "flow", Ref: "MyFlow", TagIDs: []int64{1}},
			mockValue:   map[string]any{"tags": []any{1}},
			wantChanged: true,
			wantKey:     "result",
		},
		{
			name: "org.safety.set sets Changed (clear)", command: "org.safety.set",
			args:        OrgSafetySetArgs{OrgAlias: "dev", Clear: true},
			mockValue:   map[string]any{"safety": "read_only"},
			wantChanged: true,
			wantKey:     "safety",
		},
		{
			name: "report.run returns report", command: "report.run",
			args:      ReportRunArgs{ID: "00O..."},
			mockValue: map[string]any{"rows": []any{}},
			wantKey:   "report",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			be := newCapture()
			be.values[tc.command] = tc.mockValue
			srv := &Server{Backend: be}

			req := Request{Command: tc.command}
			if tc.args != nil {
				b, _ := json.Marshal(tc.args)
				req.Args = b
			}
			resp := dispatch(t, srv, req)
			if !resp.OK {
				t.Fatalf("expected OK; got error %+v", resp.Error)
			}
			if resp.Changed != tc.wantChanged {
				t.Errorf("Changed=%v, want %v", resp.Changed, tc.wantChanged)
			}
			if tc.wantKey != "" {
				data, ok := resp.Data.(map[string]any)
				if !ok {
					t.Fatalf("data shape: %T", resp.Data)
				}
				if _, ok := data[tc.wantKey]; !ok {
					t.Errorf("missing key %q in data: %+v", tc.wantKey, data)
				}
			}
			if be.calls[tc.command] != 1 {
				t.Errorf("backend %q called %d times, want 1",
					tc.command, be.calls[tc.command])
			}
			if tc.assertArgs != nil {
				tc.assertArgs(t, be.lastArgs[tc.command])
			}
		})
	}
}

// ------ Error propagation ------

func TestHandler_BackendErrorPropagation(t *testing.T) {
	cases := []struct {
		name     string
		command  string
		args     any
		injected error
		wantCode string
	}{
		{
			"plain backend error -> internal_error",
			"project.list", nil,
			errors.New("db locked"),
			ErrInternal,
		},
		{
			"errBusy -> instance_busy",
			"project.create", ProjectCreateArgs{Name: "x"},
			errBusy,
			ErrInstanceBusy,
		},
		{
			"coded error preserves Code()",
			"record.update", RecordUpdateArgs{SObject: "Account", ID: "001x", Fields: map[string]any{"Name": "x"}},
			&codedError{code: ErrSafetyBlocked, msg: "production is read-only"},
			ErrSafetyBlocked,
		},
		{
			"coded error: not_found",
			"project.show", ProjectShowArgs{ID: "missing"},
			&codedError{code: ErrNotFound, msg: "no such project"},
			ErrNotFound,
		},
		{
			"coded error: confirmation_required",
			"metadata.delete", MetadataDeleteArgs{Type: "ApexClass", ID: "01p..."},
			&codedError{code: ErrConfirmationRequired, msg: "confirm first"},
			ErrConfirmationRequired,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			be := newCapture()
			be.errors[tc.command] = tc.injected
			srv := &Server{Backend: be}

			req := Request{Command: tc.command}
			if tc.args != nil {
				b, _ := json.Marshal(tc.args)
				req.Args = b
			}
			resp := dispatch(t, srv, req)
			if resp.OK {
				t.Fatalf("expected failure; got data %v", resp.Data)
			}
			if resp.Error == nil || resp.Error.Code != tc.wantCode {
				t.Errorf("Error.Code = %v, want %q",
					resp.Error, tc.wantCode)
			}
		})
	}
}

// ------ project.unload uses no args ------

func TestHandler_ProjectUnload_Succeeds(t *testing.T) {
	be := newCapture()
	srv := &Server{Backend: be}
	resp := dispatch(t, srv, Request{Command: "project.unload"})
	if !resp.OK {
		t.Fatalf("expected OK; got %+v", resp.Error)
	}
	if !resp.Changed {
		t.Error("expected Changed=true")
	}
	if be.calls["project.load"] != 1 {
		t.Errorf("expected one project.load invocation, got %d", be.calls["project.load"])
	}
}

// ------ chip.preview returns the minted chip id ------

func TestHandler_PreviewChip_ReturnsMintedID(t *testing.T) {
	be := newCapture()
	srv := &Server{Backend: be}
	args, _ := json.Marshal(PreviewChipArgs{Domain: "records", Scope: "Account", Label: "Recent"})
	resp := dispatch(t, srv, Request{Command: "chip.preview", Args: args})
	if !resp.OK {
		t.Fatalf("expected OK; got %+v", resp.Error)
	}
	data := resp.Data.(map[string]any)
	chip := data["chip"].(map[string]any)
	if chip["id"] != "__eph_test__" {
		t.Errorf("expected minted id, got %v", chip["id"])
	}
	if !resp.Changed {
		t.Error("expected Changed=true")
	}
}

// ------ decodeArgs ------

func TestDecodeArgs_EmptyIsNoop(t *testing.T) {
	var got OpenTabArgs
	if err := decodeArgs(nil, &got); err != nil {
		t.Errorf("nil args should not error: %v", err)
	}
	if err := decodeArgs(json.RawMessage{}, &got); err != nil {
		t.Errorf("empty args should not error: %v", err)
	}
	// Field stays zero.
	if got.Tab != "" {
		t.Errorf("expected zero value, got %+v", got)
	}
}

func TestDecodeArgs_PopulatesFields(t *testing.T) {
	var got OpenTabArgs
	raw := json.RawMessage(`{"tab":"records","sobject":"Account"}`)
	if err := decodeArgs(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Tab != "records" || got.SObject != "Account" {
		t.Errorf("decode shape: %+v", got)
	}
}

func TestDecodeArgs_PropagatesError(t *testing.T) {
	var got OpenTabArgs
	if err := decodeArgs(json.RawMessage(`{not-json`), &got); err == nil {
		t.Error("expected JSON parse error")
	}
}

// ------ Server.Entry exposes the registered instance entry ------

func TestServer_EntryRoundtrip(t *testing.T) {
	t.Setenv("HOME", shortHome(t))
	srv := &Server{Backend: newCapture()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got, err := srv.Listen(ctx)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer srv.Close()
	entry := srv.Entry()
	if entry.Number != got.Number {
		t.Errorf("Entry().Number = %d, want %d", entry.Number, got.Number)
	}
	if entry.Socket != got.Socket {
		t.Errorf("Entry().Socket = %q, want %q", entry.Socket, got.Socket)
	}
}

// ------ helpers used above ------

func strPtr(s string) *string { return &s }
