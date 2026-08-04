package headless

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// The JSON shape is a public contract — every field name, casing, and
// omitempty behaviour is something downstream skills / scripts depend
// on. These tests pin the shape.

func TestSuccess_JSONShape(t *testing.T) {
	r := Success("chip.create", "dev@example.com", "dev", true, map[string]any{
		"id":   "a01xx0000000001AAA",
		"name": "Acme",
	})

	var buf bytes.Buffer
	if err := r.Write(&buf, JSONMode); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Decode round-trip so we test what callers see, not internal
	// struct layout.
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout: %s", err, buf.String())
	}

	if got["ok"] != true {
		t.Errorf("ok = %v, want true", got["ok"])
	}
	if got["command"] != "chip.create" {
		t.Errorf("command = %v, want chip.create", got["command"])
	}
	if got["org"] != "dev@example.com" {
		t.Errorf("org = %v, want dev@example.com", got["org"])
	}
	if got["target"] != "dev" {
		t.Errorf("target = %v, want dev", got["target"])
	}
	if got["changed"] != true {
		t.Errorf("changed = %v, want true", got["changed"])
	}
	data, ok := got["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing or wrong type: %#v", got["data"])
	}
	if data["id"] != "a01xx0000000001AAA" {
		t.Errorf("data.id = %v", data["id"])
	}
	// Error must NOT be present on success.
	if _, ok := got["error"]; ok {
		t.Errorf("error field should be omitted on success, got: %v", got["error"])
	}
}

func TestFail_JSONShape(t *testing.T) {
	r := Fail("record.update", "prod@example.com", ErrSafetyBlocked,
		"prod is read_only; requires records",
		map[string]any{
			"required_write_kind": "records",
			"effective_safety":    "read_only",
		})

	var buf bytes.Buffer
	if err := r.Write(&buf, JSONMode); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout: %s", err, buf.String())
	}

	if got["ok"] != false {
		t.Errorf("ok = %v, want false", got["ok"])
	}
	if got["command"] != "record.update" {
		t.Errorf("command = %v", got["command"])
	}
	errObj, ok := got["error"].(map[string]any)
	if !ok {
		t.Fatalf("error missing: %#v", got)
	}
	if errObj["code"] != "safety_blocked" {
		t.Errorf("error.code = %v, want safety_blocked", errObj["code"])
	}
	if !strings.Contains(errObj["message"].(string), "read_only") {
		t.Errorf("error.message = %v, want substring 'read_only'", errObj["message"])
	}
	details, ok := errObj["details"].(map[string]any)
	if !ok {
		t.Fatalf("error.details missing: %#v", errObj)
	}
	if details["required_write_kind"] != "records" {
		t.Errorf("details.required_write_kind = %v", details["required_write_kind"])
	}

	// data and changed must be omitted on fail.
	if _, ok := got["data"]; ok {
		t.Errorf("data should be omitted on fail")
	}
	if _, ok := got["changed"]; ok {
		t.Errorf("changed should be omitted when false")
	}
}

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		name string
		r    *Response
		want int
	}{
		{"nil response", nil, ExitInternal},
		{"success", Success("x", "", "", false, nil), ExitOK},
		{"invalid arg", Fail("x", "", ErrInvalidArgument, "bad", nil), ExitInvalidArg},
		{"safety blocked", Fail("x", "", ErrSafetyBlocked, "no", nil), ExitSafetyBlocked},
		{"not found", Fail("x", "", ErrNotFound, "nope", nil), ExitNotFound},
		{"auth", Fail("x", "", ErrAuth, "401", nil), ExitAuthRequired},
		{"partial", Fail("x", "", ErrPartial, "some", nil), ExitPartialSuccess},
		{"internal", Fail("x", "", ErrInternal, "boom", nil), ExitInternal},
		{"unknown code falls back to internal",
			Fail("x", "", "weird_code", "huh", nil), ExitInternal},
		{"ok=false but no error object",
			&Response{OK: false, Command: "x"}, ExitInternal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExitCodeFor(c.r); got != c.want {
				t.Errorf("ExitCodeFor = %d, want %d", got, c.want)
			}
		})
	}
}

func TestErrorImplementsError(t *testing.T) {
	// Should be usable as a plain Go error.
	var err error = &Error{Code: ErrNotFound, Message: "no such org"}
	if err.Error() != "no such org" {
		t.Errorf("Error() = %q, want 'no such org'", err.Error())
	}
	// Nil receiver must not panic.
	var nilErr *Error
	if nilErr.Error() != "" {
		t.Errorf("nil receiver should return empty string")
	}
}

func TestTextMode(t *testing.T) {
	cases := []struct {
		name string
		r    *Response
		want string
	}{
		{
			"ok no change",
			Success("chip.list", "", "", false, nil),
			"ok · chip.list\n",
		},
		{
			"ok changed",
			Success("chip.create", "", "", true, nil),
			"ok · chip.create · changed\n",
		},
		{
			"error with code",
			Fail("record.update", "", ErrSafetyBlocked, "blocked", nil),
			"error · record.update · safety_blocked · blocked\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := c.r.Write(&buf, TextMode); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if got := buf.String(); got != c.want {
				t.Errorf("output = %q, want %q", got, c.want)
			}
		})
	}
}

// Sanity check the field names in JSON tags — if anyone renames a
// struct tag the wire contract breaks silently otherwise.
func TestJSONFieldNamesArePinned(t *testing.T) {
	r := &Response{
		OK:       true,
		Command:  "x",
		Org:      "o",
		Target:   "t",
		Changed:  true,
		Warnings: []string{"w"},
		Data:     "d",
		Error: &Error{
			Code:    "c",
			Message: "m",
			Details: map[string]any{"k": "v"},
		},
	}
	// Force OK=false so Error renders too.
	r.OK = false
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"ok":`,
		`"command":`,
		`"org":`,
		`"target":`,
		`"changed":`,
		`"warnings":`,
		`"data":`,
		`"error":`,
		`"code":`,
		`"message":`,
		`"details":`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing field %s\n%s", want, s)
		}
	}
}
