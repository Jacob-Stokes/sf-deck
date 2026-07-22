package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

// newTestApp builds an App with in-memory Settings + a Save that's a
// no-op (so chip writes don't hit disk). Cache / Projects / Usage are
// left nil — chip dispatch doesn't touch them.
//
// Test safety: we explicitly null out the real settings.Save hook by
// installing a no-op SaveSettings. Without this, every test would
// pollute the user's ~/.sf-deck/settings.toml.
func newTestApp() *app.App {
	return &app.App{
		Settings:     &settings.Settings{},
		SaveSettings: func() error { return nil },
	}
}

// runCLI is the test harness: parse argv, dispatch, capture stdout +
// exit code, decode JSON. Mirrors what a script consumer sees.
func runCLI(t *testing.T, a *app.App, argv ...string) (int, map[string]any, string) {
	t.Helper()
	args := Parse(argv)
	if !args.IsHeadless() {
		t.Fatalf("argv %v not recognized as headless", argv)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Dispatch(a, args, &stdout, &stderr)
	out := stdout.String()
	if !args.JSON {
		return code, nil, out
	}
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("JSON unmarshal failed: %v\nstdout: %s", err, out)
	}
	return code, got, out
}

func TestParse_FlagsAndPositionals(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want Args
	}{
		{
			"json before noun",
			[]string{"--json", "chip", "list"},
			Args{JSON: true, Noun: "chip", Verb: "list", Rest: []string{}},
		},
		{
			"json after verb",
			[]string{"chip", "list", "--json"},
			Args{JSON: true, Noun: "chip", Verb: "list", Rest: []string{}},
		},
		{
			"json after subcommand flags",
			[]string{"chip", "create", "--id", "x", "--json", "--label", "X"},
			Args{JSON: true, Noun: "chip", Verb: "create", Rest: []string{"--id", "x", "--label", "X"}},
		},
		{
			"noun without verb",
			[]string{"chip"},
			Args{Noun: "chip", Verb: "", Rest: []string{}},
		},
		{
			"verb plus subcommand flags pass through",
			[]string{"chip", "create", "--id", "x", "--label", "X"},
			Args{Noun: "chip", Verb: "create", Rest: []string{"--id", "x", "--label", "X"}},
		},
		{
			"unknown global flag stops top-level parser",
			[]string{"--foo", "bar"},
			Args{Rest: []string{"--foo", "bar"}},
		},
		{
			"empty argv",
			[]string{},
			Args{Rest: nil},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.argv)
			if got.JSON != c.want.JSON {
				t.Errorf("JSON = %v, want %v", got.JSON, c.want.JSON)
			}
			if got.Noun != c.want.Noun {
				t.Errorf("Noun = %q, want %q", got.Noun, c.want.Noun)
			}
			if got.Verb != c.want.Verb {
				t.Errorf("Verb = %q, want %q", got.Verb, c.want.Verb)
			}
			if !stringSlicesEqual(got.Rest, c.want.Rest) {
				t.Errorf("Rest = %v, want %v", got.Rest, c.want.Rest)
			}
		})
	}
}

func TestParse_IsHeadless(t *testing.T) {
	if !Parse([]string{"chip"}).IsHeadless() {
		t.Error("chip should be headless")
	}
	if Parse([]string{}).IsHeadless() {
		t.Error("empty argv should not be headless")
	}
	if Parse([]string{"-dump-keymap"}).IsHeadless() {
		t.Error("TUI flag should not be headless")
	}
	if Parse([]string{"unknown"}).IsHeadless() {
		t.Error("unknown noun should not be headless")
	}
}

func TestChipCreate_JSONHappyPath(t *testing.T) {
	a := newTestApp()
	code, got, _ := runCLI(t, a, "--json", "chip", "create",
		"--id", "renewals",
		"--domain", "records",
		"--scope", "Account",
		"--label", "Renewals",
		"--columns", "Id,Name,Amount",
		"--limit", "50",
	)
	if code != headless.ExitOK {
		t.Fatalf("exit = %d, want %d (%+v)", code, headless.ExitOK, got)
	}
	if got["ok"] != true {
		t.Errorf("ok = %v", got["ok"])
	}
	if got["command"] != "chip.create" {
		t.Errorf("command = %v", got["command"])
	}
	if got["changed"] != true {
		t.Errorf("changed = %v", got["changed"])
	}
	data, _ := got["data"].(map[string]any)
	chip, _ := data["chip"].(map[string]any)
	if chip["id"] != "renewals" {
		t.Errorf("chip.id = %v", chip["id"])
	}
	if chip["scope"] != "Account" {
		t.Errorf("chip.scope = %v", chip["scope"])
	}
	cols, _ := chip["columns"].([]any)
	if len(cols) != 3 {
		t.Errorf("columns len = %d, want 3", len(cols))
	}
}

func TestChipCreate_WithAdvancedClauses(t *testing.T) {
	a := newTestApp()
	code, got, _ := runCLI(t, a, "--json", "chip", "create",
		"--id", "urgent-requests",
		"--domain", "records",
		"--scope", "Request__c",
		"--label", "Urgent Requests",
		"--columns", "Id,Name,Status__c",
		"--clauses", "WHERE Status__c = 'Open' AND Priority__c = 'High' ORDER BY CreatedDate DESC LIMIT 25",
		"--favourite",
	)
	if code != headless.ExitOK {
		t.Fatalf("exit = %d, want %d (%+v)", code, headless.ExitOK, got)
	}
	data, _ := got["data"].(map[string]any)
	chip, _ := data["chip"].(map[string]any)
	if chip["scope"] != "Request__c" {
		t.Errorf("chip.scope = %v", chip["scope"])
	}
	if chip["favourite"] != true {
		t.Errorf("chip.favourite = %v", chip["favourite"])
	}
	clauses, _ := chip["clauses"].(string)
	if !strings.Contains(clauses, "WHERE") ||
		!strings.Contains(clauses, "ORDER BY CreatedDate DESC") ||
		!strings.Contains(clauses, "LIMIT 25") {
		t.Errorf("chip.clauses = %q", clauses)
	}
}

func TestChipList_JSONAfterVerb(t *testing.T) {
	a := newTestApp()
	code, got, _ := runCLI(t, a, "chip", "list", "--json")
	if code != headless.ExitOK {
		t.Fatalf("exit = %d, want %d (%+v)", code, headless.ExitOK, got)
	}
	if got["command"] != "chip.list" {
		t.Errorf("command = %v", got["command"])
	}
}

func TestChipColumns_StaticCatalog(t *testing.T) {
	a := newTestApp()
	code, got, _ := runCLI(t, a, "--json", "chip", "columns",
		"--domain", "flows",
		"--columns", "Name,Status")
	if code != headless.ExitOK {
		t.Fatalf("exit = %d, want %d (%+v)", code, headless.ExitOK, got)
	}
	data, _ := got["data"].(map[string]any)
	if data["valid"] != true {
		t.Fatalf("valid = %v", data["valid"])
	}
	catalog, _ := data["catalog"].(map[string]any)
	defaults, _ := catalog["default_columns"].([]any)
	if len(defaults) == 0 {
		t.Fatalf("default_columns empty in %+v", catalog)
	}
	cols, _ := catalog["columns"].([]any)
	if len(cols) == 0 {
		t.Fatalf("columns empty in %+v", catalog)
	}
}

func TestChipColumns_UnknownStaticColumn(t *testing.T) {
	a := newTestApp()
	code, got, _ := runCLI(t, a, "--json", "chip", "columns",
		"--domain", "flows",
		"--columns", "Bogus")
	if code != headless.ExitInvalidArg {
		t.Fatalf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrInvalidArgument {
		t.Fatalf("error.code = %v", errObj["code"])
	}
	details, _ := errObj["details"].(map[string]any)
	unknown, _ := details["columns"].([]any)
	if len(unknown) != 1 || unknown[0] != "Bogus" {
		t.Fatalf("details.columns = %+v", details["columns"])
	}
}

func TestChipCreate_DuplicateMapsToInvalidArgument(t *testing.T) {
	a := newTestApp()
	_, _, _ = runCLI(t, a, "--json", "chip", "create",
		"--id", "x", "--domain", "records", "--label", "X")
	code, got, _ := runCLI(t, a, "--json", "chip", "create",
		"--id", "x", "--domain", "records", "--label", "X")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrInvalidArgument {
		t.Errorf("error.code = %v, want %s", errObj["code"], headless.ErrInvalidArgument)
	}
}

func TestChipShow_NotFoundMapsExit4(t *testing.T) {
	a := newTestApp()
	code, got, _ := runCLI(t, a, "--json", "chip", "show",
		"--id", "missing", "--domain", "records")
	if code != headless.ExitNotFound {
		t.Errorf("exit = %d, want %d", code, headless.ExitNotFound)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrNotFound {
		t.Errorf("error.code = %v, want %s", errObj["code"], headless.ErrNotFound)
	}
	details, _ := errObj["details"].(map[string]any)
	if details["domain"] != "records" || details["id"] != "missing" {
		t.Errorf("error.details = %+v", details)
	}
}

func TestChipUpdate_PartialAndIdempotent(t *testing.T) {
	a := newTestApp()
	_, _, _ = runCLI(t, a, "--json", "chip", "create",
		"--id", "x", "--domain", "records", "--label", "Old")

	// First update changes label.
	code, got, _ := runCLI(t, a, "--json", "chip", "update",
		"--id", "x", "--domain", "records", "--label", "New")
	if code != headless.ExitOK {
		t.Fatalf("update exit = %d (%+v)", code, got)
	}
	if got["changed"] != true {
		t.Errorf("first update changed = %v", got["changed"])
	}

	// Same label again → no-op.
	code, got, _ = runCLI(t, a, "--json", "chip", "update",
		"--id", "x", "--domain", "records", "--label", "New")
	if code != headless.ExitOK {
		t.Fatalf("idempotent update exit = %d", code)
	}
	if changed, ok := got["changed"]; ok && changed == true {
		t.Errorf("idempotent update changed = %v, want false/missing", changed)
	}
}

func TestChipUpdate_WithAdvancedClauses(t *testing.T) {
	a := newTestApp()
	_, _, _ = runCLI(t, a, "--json", "chip", "create",
		"--id", "x", "--domain", "records", "--scope", "Account", "--label", "X",
		"--limit", "10")

	code, got, _ := runCLI(t, a, "--json", "chip", "update",
		"--id", "x",
		"--domain", "records",
		"--clauses", "WHERE Active__c = true ORDER BY CreatedDate DESC")
	if code != headless.ExitOK {
		t.Fatalf("update exit = %d (%+v)", code, got)
	}
	data, _ := got["data"].(map[string]any)
	chip, _ := data["chip"].(map[string]any)
	clauses, _ := chip["clauses"].(string)
	if !strings.Contains(clauses, "WHERE Active__c = true") ||
		!strings.Contains(clauses, "ORDER BY CreatedDate DESC") {
		t.Errorf("chip.clauses = %q", clauses)
	}
	if _, ok := chip["limit"]; ok {
		t.Errorf("chip.limit should be omitted after clauses without LIMIT, got %v", chip["limit"])
	}
}

func TestChipUpdate_RequiresFields(t *testing.T) {
	a := newTestApp()
	_, _, _ = runCLI(t, a, "--json", "chip", "create",
		"--id", "x", "--domain", "records", "--label", "X")

	code, got, _ := runCLI(t, a, "--json", "chip", "update",
		"--id", "x", "--domain", "records") // no update fields
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "no update fields") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestChipFavourite_Roundtrip(t *testing.T) {
	a := newTestApp()
	_, _, _ = runCLI(t, a, "--json", "chip", "create",
		"--id", "x", "--domain", "records", "--label", "X")

	code, got, _ := runCLI(t, a, "--json", "chip", "favourite",
		"--id", "x", "--domain", "records", "--on", "true")
	if code != headless.ExitOK {
		t.Fatalf("fav exit = %d", code)
	}
	if got["changed"] != true {
		t.Errorf("changed = %v", got["changed"])
	}
	data, _ := got["data"].(map[string]any)
	chip, _ := data["chip"].(map[string]any)
	if chip["favourite"] != true {
		t.Errorf("chip.favourite = %v", chip["favourite"])
	}
}

func TestChipDelete_Roundtrip(t *testing.T) {
	a := newTestApp()
	_, _, _ = runCLI(t, a, "--json", "chip", "create",
		"--id", "x", "--domain", "records", "--label", "X")

	code, got, _ := runCLI(t, a, "--json", "chip", "delete",
		"--id", "x", "--domain", "records")
	if code != headless.ExitOK {
		t.Fatalf("delete exit = %d", code)
	}
	if got["changed"] != true {
		t.Errorf("changed = %v", got["changed"])
	}

	// Second delete: not found.
	code, _, _ = runCLI(t, a, "--json", "chip", "delete",
		"--id", "x", "--domain", "records")
	if code != headless.ExitNotFound {
		t.Errorf("second delete exit = %d, want %d", code, headless.ExitNotFound)
	}
}

func TestChipList_DefaultsToListVerb(t *testing.T) {
	a := newTestApp()
	_, _, _ = runCLI(t, a, "--json", "chip", "create",
		"--id", "x", "--domain", "records", "--label", "X")

	// No verb → list.
	code, got, _ := runCLI(t, a, "--json", "chip")
	if code != headless.ExitOK {
		t.Fatalf("default-verb exit = %d", code)
	}
	if got["command"] != "chip.list" {
		t.Errorf("command = %v, want chip.list", got["command"])
	}
}

func TestChip_TextMode(t *testing.T) {
	a := newTestApp()
	_, _, out := runCLI(t, a, "chip", "create",
		"--id", "x", "--domain", "records", "--label", "X")
	if !strings.HasPrefix(out, "ok · chip.create") {
		t.Errorf("text output = %q", out)
	}
}

func TestChip_UnknownVerb(t *testing.T) {
	a := newTestApp()
	code, got, _ := runCLI(t, a, "--json", "chip", "bogus")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "bogus") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
