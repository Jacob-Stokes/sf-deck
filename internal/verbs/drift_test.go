package verbs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// findRepoRoot walks up from the test file to find the go.mod
// boundary. Used by the drift tests to grep the source tree
// without hard-coding paths that break on contributor machines.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("repo root not found")
	return ""
}

func grepFile(t *testing.T, path, needle string) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.Contains(string(data), needle)
}

// TestCLIBindingsHaveDispatchArm checks that every Spec with a
// CLI binding has a dispatch case for its noun. We don't try to
// verify per-verb arms because some nouns use a `switch verb` in
// their per-noun file rather than the top-level dispatcher.
func TestCLIBindingsHaveDispatchArm(t *testing.T) {
	root := findRepoRoot(t)
	dispatchFile := filepath.Join(root, "internal/headless/cli/cli.go")
	for _, s := range Specs() {
		if s.CLI == nil {
			continue
		}
		needle := `case "` + s.Noun + `":`
		if !grepFile(t, dispatchFile, needle) {
			t.Errorf("%s declares CLI binding but cli.go has no dispatch arm for noun %q",
				s.Qualified(), s.Noun)
		}
	}
}

// TestIPCBindingsHaveListenerArm verifies every IPC binding has
// a dispatch arm in the listener. The handler may live in
// handlers_*.go; we only check the listener's outer switch since
// that's where typos would surface.
func TestIPCBindingsHaveListenerArm(t *testing.T) {
	root := findRepoRoot(t)
	listenerFile := filepath.Join(root, "internal/control/listener.go")
	for _, s := range Specs() {
		if s.IPC == nil {
			continue
		}
		needle := `case "` + s.IPC.Command + `":`
		if !grepFile(t, listenerFile, needle) {
			t.Errorf("%s declares IPC binding %q but listener.go has no dispatch arm",
				s.Qualified(), s.IPC.Command)
		}
	}
}

// TestIPCBindingsHaveBackendMethod walks the Backend interface
// definition and confirms each IPC verb's expected Backend method
// name exists. The convention is that command "soql.run" maps to
// method "SOQLRun" — uppercase the parts and concatenate.
//
// We do this textually rather than via reflection because the
// Backend interface is exported and stable; a typo'd method name
// would otherwise only break at the listener_test compile site.
func TestIPCBindingsHaveBackendMethod(t *testing.T) {
	root := findRepoRoot(t)
	listenerFile := filepath.Join(root, "internal/control/listener.go")
	for _, s := range Specs() {
		if s.IPC == nil {
			continue
		}
		method := commandToMethod(s.IPC.Command)
		if method == "" {
			continue // intentional skip — convention doesn't apply
		}
		// Look for "MethodName(" — the leading paren proves
		// it's a method decl or call site rather than a comment
		// referencing the name. Both arg-bearing (Foo(args …))
		// and arg-free (Foo()) methods qualify.
		needle := method + "("
		if !grepFile(t, listenerFile, needle) {
			t.Errorf("%s expects Backend method %q but it's missing from listener.go",
				s.Qualified(), method)
		}
	}
}

// commandToMethod implements the IPC-command → Backend-method
// convention:
//   - dot segments are concatenated (soql.run → SoqlRun)
//   - "soql" / "ipc" / "soap" stay uppercase as acronyms
//   - hyphens are removed (project.add-item → ProjectAddItem)
//   - kebab-case segments are CamelCased (add-item → AddItem)
//
// A handful of verbs intentionally break the convention because
// they map onto existing Model methods chosen before the registry
// existed (state.get → State, tab.open → OpenTab,
// chip.apply → ApplyChip, etc.). methodOverrides catches those.
func commandToMethod(command string) string {
	if m, ok := methodOverrides[command]; ok {
		return m
	}
	if command == "" {
		return ""
	}
	parts := strings.Split(command, ".")
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(camelSegment(p))
	}
	return b.String()
}

// camelSegment uppercases the first rune of a segment, handles
// known acronyms (SOQL, IPC, SOAP, REST, etc.), and CamelCases
// kebab-case segments.
func camelSegment(s string) string {
	if s == "" {
		return ""
	}
	// Known acronym shortcut.
	if upper, ok := acronyms[strings.ToLower(s)]; ok {
		return upper
	}
	// Split on dashes; CamelCase each sub-segment.
	subs := strings.Split(s, "-")
	var b strings.Builder
	for _, sub := range subs {
		if sub == "" {
			continue
		}
		if upper, ok := acronyms[strings.ToLower(sub)]; ok {
			b.WriteString(upper)
			continue
		}
		b.WriteString(strings.ToUpper(sub[:1]))
		if len(sub) > 1 {
			b.WriteString(sub[1:])
		}
	}
	return b.String()
}

var acronyms = map[string]string{
	"soql": "SOQL",
	"ipc":  "IPC",
	"id":   "ID",
}

// methodOverrides catches verbs whose Backend method name doesn't
// follow the soql.run → SOQLRun convention. These mostly predate
// the registry — e.g. `tab.open` is implemented as Backend.OpenTab
// because that name read better at the call site.
var methodOverrides = map[string]string{
	"state.get":            "State",
	"state.subscribe":      "Subscribe",
	"tab.open":             "OpenTab",
	"chip.apply":           "ApplyChip",
	"chip.preview":         "PreviewChip",
	"chip.preview.save":    "PreviewSaveChip",
	"chip.preview.dismiss": "PreviewDismissChip",
	"org.switch":           "SwitchOrg",
	"project.load":         "LoadProject",
	"project.unload":       "LoadProject", // Unload routes through LoadProject(empty)
}
