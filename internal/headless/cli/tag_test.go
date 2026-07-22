package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

// newTagTestApp opens a real devproject store under a temp HOME and
// wires it into a stub App. SaveSettings is a no-op (chips path is
// not exercised). Cleanup closes the store.
func newTagTestApp(t *testing.T) *app.App {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dp, err := devproject.Open()
	if err != nil {
		t.Fatalf("devproject.Open: %v", err)
	}
	t.Cleanup(func() { _ = dp.Close() })
	return &app.App{
		Settings:     &settings.Settings{},
		Projects:     dp,
		SaveSettings: func() error { return nil },
	}
}

func runTagCLI(t *testing.T, a *app.App, argv ...string) (int, map[string]any) {
	t.Helper()
	args := Parse(argv)
	if !args.IsHeadless() {
		t.Fatalf("argv %v not recognized as headless", argv)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Dispatch(a, args, &stdout, &stderr)
	if !args.JSON {
		return code, nil
	}
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("JSON unmarshal: %v\nstdout: %s", err, stdout.String())
	}
	return code, got
}

func TestTagCreate_JSONHappyPath(t *testing.T) {
	a := newTagTestApp(t)
	code, got := runTagCLI(t, a, "--json", "tag", "create",
		"--name", "renewals", "--color", "blue", "--icon", "🔁")
	if code != headless.ExitOK {
		t.Fatalf("exit = %d (%+v)", code, got)
	}
	if got["command"] != "tag.create" {
		t.Errorf("command = %v", got["command"])
	}
	if got["changed"] != true {
		t.Errorf("changed = %v", got["changed"])
	}
	data, _ := got["data"].(map[string]any)
	tag, _ := data["tag"].(map[string]any)
	if tag["name"] != "renewals" {
		t.Errorf("tag.name = %v", tag["name"])
	}
	if tag["color"] != "blue" {
		t.Errorf("tag.color = %v", tag["color"])
	}
	if _, ok := tag["id"]; !ok {
		t.Error("tag.id missing")
	}
}

func TestTagCreate_DuplicateMapsInvalidArg(t *testing.T) {
	a := newTagTestApp(t)
	_, _ = runTagCLI(t, a, "--json", "tag", "create", "--name", "x")
	code, got := runTagCLI(t, a, "--json", "tag", "create", "--name", "X")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrInvalidArgument {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

func TestTagShow_NotFound(t *testing.T) {
	a := newTagTestApp(t)
	code, got := runTagCLI(t, a, "--json", "tag", "show", "--name", "missing")
	if code != headless.ExitNotFound {
		t.Errorf("exit = %d, want %d", code, headless.ExitNotFound)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrNotFound {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

func TestTagUpdate_PartialAndIdempotent(t *testing.T) {
	a := newTagTestApp(t)
	_, created := runTagCLI(t, a, "--json", "tag", "create", "--name", "old")
	id := tagIDFromCreate(t, created)

	// First update — Changed=true.
	code, got := runTagCLI(t, a, "--json", "tag", "update",
		"--id", fmt.Sprint(id), "--name", "new")
	if code != headless.ExitOK {
		t.Fatalf("update exit = %d (%+v)", code, got)
	}
	if got["changed"] != true {
		t.Errorf("changed = %v", got["changed"])
	}

	// Same name — Changed=false.
	code, got = runTagCLI(t, a, "--json", "tag", "update",
		"--id", fmt.Sprint(id), "--name", "new")
	if code != headless.ExitOK {
		t.Fatalf("idempotent update exit = %d", code)
	}
	if changed, ok := got["changed"]; ok && changed == true {
		t.Errorf("idempotent changed = %v", changed)
	}
}

func TestTagDelete_Roundtrip(t *testing.T) {
	a := newTagTestApp(t)
	_, created := runTagCLI(t, a, "--json", "tag", "create", "--name", "x")
	id := tagIDFromCreate(t, created)

	code, got := runTagCLI(t, a, "--json", "tag", "delete",
		"--id", fmt.Sprint(id))
	if code != headless.ExitOK {
		t.Fatalf("delete exit = %d (%+v)", code, got)
	}
	if got["changed"] != true {
		t.Errorf("changed = %v", got["changed"])
	}

	// Second delete — not_found.
	code, _ = runTagCLI(t, a, "--json", "tag", "delete",
		"--id", fmt.Sprint(id))
	if code != headless.ExitNotFound {
		t.Errorf("second delete exit = %d", code)
	}
}

func TestTagApply_Idempotent(t *testing.T) {
	a := newTagTestApp(t)
	_, created := runTagCLI(t, a, "--json", "tag", "create", "--name", "x")
	id := tagIDFromCreate(t, created)

	code, got := runTagCLI(t, a, "--json", "tag", "apply",
		"--id", fmt.Sprint(id),
		"--kind", "record", "--ref", "001A", "--org-user", "dev@x")
	if code != headless.ExitOK {
		t.Fatalf("apply exit = %d", code)
	}
	if got["changed"] != true {
		t.Errorf("first apply changed = %v", got["changed"])
	}

	// Re-apply — Changed=false.
	code, got = runTagCLI(t, a, "--json", "tag", "apply",
		"--id", fmt.Sprint(id),
		"--kind", "record", "--ref", "001A", "--org-user", "dev@x")
	if code != headless.ExitOK {
		t.Fatalf("re-apply exit = %d", code)
	}
	if changed, ok := got["changed"]; ok && changed == true {
		t.Errorf("re-apply changed = %v", changed)
	}
}

func TestTagApply_BadKind(t *testing.T) {
	a := newTagTestApp(t)
	_, created := runTagCLI(t, a, "--json", "tag", "create", "--name", "x")
	id := tagIDFromCreate(t, created)

	code, got := runTagCLI(t, a, "--json", "tag", "apply",
		"--id", fmt.Sprint(id), "--kind", "bogus", "--ref", "Y")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "bogus") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestTagSet_ReplacesBindings(t *testing.T) {
	a := newTagTestApp(t)
	_, c1 := runTagCLI(t, a, "--json", "tag", "create", "--name", "a")
	_, c2 := runTagCLI(t, a, "--json", "tag", "create", "--name", "b")
	idA := tagIDFromCreate(t, c1)
	idB := tagIDFromCreate(t, c2)

	code, got := runTagCLI(t, a, "--json", "tag", "set",
		"--kind", "record", "--ref", "001A",
		"--ids", fmt.Sprintf("%d,%d", idA, idB))
	if code != headless.ExitOK {
		t.Fatalf("set exit = %d (%+v)", code, got)
	}
	if got["changed"] != true {
		t.Errorf("changed = %v", got["changed"])
	}

	// Same set — idempotent.
	code, got = runTagCLI(t, a, "--json", "tag", "set",
		"--kind", "record", "--ref", "001A",
		"--ids", fmt.Sprintf("%d,%d", idA, idB))
	if code != headless.ExitOK {
		t.Fatalf("idempotent set exit = %d", code)
	}
	if changed, ok := got["changed"]; ok && changed == true {
		t.Errorf("idempotent set changed = %v", changed)
	}
}

func TestTagOf_ListsBoundTags(t *testing.T) {
	a := newTagTestApp(t)
	_, c1 := runTagCLI(t, a, "--json", "tag", "create", "--name", "a")
	idA := tagIDFromCreate(t, c1)
	_, _ = runTagCLI(t, a, "--json", "tag", "apply",
		"--id", fmt.Sprint(idA), "--kind", "record", "--ref", "001A")

	code, got := runTagCLI(t, a, "--json", "tag", "of",
		"--kind", "record", "--ref", "001A")
	if code != headless.ExitOK {
		t.Fatalf("of exit = %d", code)
	}
	data, _ := got["data"].(map[string]any)
	if data["count"] != float64(1) {
		t.Errorf("count = %v, want 1", data["count"])
	}
	tags, _ := data["tags"].([]any)
	if len(tags) != 1 {
		t.Errorf("tags len = %d", len(tags))
	}
}

func TestTagItems_NotFound(t *testing.T) {
	a := newTagTestApp(t)
	code, got := runTagCLI(t, a, "--json", "tag", "items", "--id", "99")
	if code != headless.ExitNotFound {
		t.Fatalf("exit = %d, want %d (%+v)", code, headless.ExitNotFound, got)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrNotFound {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

func TestTagList_DefaultsToListVerb(t *testing.T) {
	a := newTagTestApp(t)
	_, _ = runTagCLI(t, a, "--json", "tag", "create", "--name", "x")
	code, got := runTagCLI(t, a, "--json", "tag")
	if code != headless.ExitOK {
		t.Fatalf("default-verb exit = %d", code)
	}
	if got["command"] != "tag.list" {
		t.Errorf("command = %v", got["command"])
	}
}

func TestTag_UnknownVerb(t *testing.T) {
	a := newTagTestApp(t)
	code, _ := runTagCLI(t, a, "--json", "tag", "weird")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
}

func TestTag_NilProjectsRendersTypedError(t *testing.T) {
	a := &app.App{Settings: &settings.Settings{}} // no Projects
	code, got := runTagCLI(t, a, "--json", "tag", "list")
	if code != headless.ExitInternal {
		t.Errorf("exit = %d, want %d", code, headless.ExitInternal)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrInternal {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

// tagIDFromCreate extracts the int id out of a tag.create response's
// data.tag.id field. The Go JSON decoder returns numbers as float64
// by default so we round-trip via fmt.
func tagIDFromCreate(t *testing.T, resp map[string]any) int64 {
	t.Helper()
	data, _ := resp["data"].(map[string]any)
	tag, _ := data["tag"].(map[string]any)
	idF, ok := tag["id"].(float64)
	if !ok {
		t.Fatalf("data.tag.id missing or non-numeric: %v", tag)
	}
	return int64(idF)
}
