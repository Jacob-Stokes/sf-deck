package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/services/records"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// dispatchRecord routes `sf-deck record <verb>`. Read-only — `get`
// returns one record, `recent` returns the N most recently modified.
// SOQL-driven custom listings go through `sf-deck soql run`.
func dispatchRecord(a *app.App, args Args, stdout io.Writer, mode headless.WriteMode) int {
	verb := args.Verb
	if verb == "" {
		verb = "recent"
	}
	switch verb {
	case "get":
		return recordGet(a, args.Rest, stdout, mode)
	case "recent":
		return recordRecent(a, args.Rest, stdout, mode)
	case "update":
		return recordUpdate(a, args.Rest, stdout, mode)
	case "create":
		return recordCreate(a, args.Rest, stdout, mode)
	case "delete":
		return recordDelete(a, args.Rest, stdout, mode)
	}
	r := headless.Fail("record."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown record verb %q", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// recordGet fetches one record by ID. Either --object is passed
// explicitly, or we infer it from the 3-char key prefix on the
// record id when --object is empty (matches the TUI's behaviour).
func recordGet(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("record get")
	target := fs.String("org", "", "Alias or username (empty = default)")
	object := fs.String("object", "",
		"sObject API name (inferred from --id key prefix if omitted)")
	id := fs.String("id", "", "Record id (required)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("record.get", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("record.get",
			errors.New("--id is required"), stdout, mode)
	}
	if len(*id) < 15 {
		return writeArgErr("record.get",
			fmt.Errorf("invalid record id %q (must be 15 or 18 chars)", *id),
			stdout, mode)
	}
	o, err := a.ResolveOrg(*target)
	if err != nil {
		return writeOrgErr("record.get", *target, err, stdout, mode)
	}

	sobjectName := *object
	if sobjectName == "" {
		// Resolve via key prefix. sf.ListSObjects returns SObjects
		// with KeyPrefix populated; scan for a match.
		resolved, rerr := resolveSObjectByPrefix(app.TargetArg(o), *id)
		if rerr != nil {
			return writeDescribeErr("record.get", o.Username, *id, rerr, stdout, mode)
		}
		sobjectName = resolved
	}

	rec, err := sf.GetRecord(app.TargetArg(o), sobjectName, *id)
	if err != nil {
		return writeRecordErr("record.get", o.Username, sobjectName, *id, err, stdout, mode)
	}
	r := headless.Success("record.get", o.Username, app.TargetArg(o), false,
		map[string]any{
			"object": sobjectName,
			"id":     *id,
			"record": rec,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func recordRecent(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("record recent")
	target := fs.String("org", "", "Alias or username (empty = default)")
	object := fs.String("object", "", "sObject API name (required)")
	limit := fs.Int("limit", 50,
		"Max rows to return (server may cap further)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("record.recent", err, stdout, mode)
	}
	if *object == "" {
		return writeArgErr("record.recent",
			errors.New("--object is required"), stdout, mode)
	}
	if *limit <= 0 {
		return writeArgErr("record.recent",
			errors.New("--limit must be positive"), stdout, mode)
	}
	o, err := a.ResolveOrg(*target)
	if err != nil {
		return writeOrgErr("record.recent", *target, err, stdout, mode)
	}
	list, err := sf.RecentRecords(app.TargetArg(o), *object, *limit)
	if err != nil {
		return writeDescribeErr("record.recent", o.Username, *object, err, stdout, mode)
	}
	r := headless.Success("record.recent", o.Username, app.TargetArg(o), false,
		map[string]any{
			"object":     list.SObject,
			"records":    list.Records,
			"columns":    list.Columns,
			"total_size": list.TotalSize,
			"returned":   len(list.Records),
			"done":       list.Done,
			"query":      list.Query,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// resolveSObjectByPrefix scans the org's sObject list for an entry
// whose KeyPrefix matches the first 3 chars of recordID. Returns the
// API name. Used by `record get` when --object is omitted — saves the
// caller from having to know which 3-letter prefix maps to Account vs.
// Contact etc.
//
// Falls back to a clear "unable to resolve" error rather than guessing
// when no prefix match exists (e.g. recordID belongs to a managed
// package object not yet enabled in this org).
func resolveSObjectByPrefix(target, recordID string) (string, error) {
	if len(recordID) < 3 {
		return "", fmt.Errorf("record id too short to resolve sObject")
	}
	all, err := sf.ListSObjects(target)
	if err != nil {
		return "", err
	}
	if s, ok := sf.SObjectByKeyPrefix(all, recordID); ok {
		return s.Name, nil
	}
	return "", fmt.Errorf("unable to resolve sObject from key prefix %q; pass --object explicitly", recordID[:3])
}

// recordUpdate PATCHes one record through the shared records service,
// which resolves the exact target and enforces WriteRecord safety.
//
// Field values come in as repeated --field K=V flags. Strings only;
// we don't try to type-coerce client-side (Salesforce handles
// strings → typed values per the field's describe). Sending the
// literal string "null" clears a field.
func recordUpdate(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("record update")
	target := fs.String("org", "", "Alias or username (empty = default)")
	object := fs.String("object", "",
		"sObject API name (inferred from --id key prefix if omitted)")
	id := fs.String("id", "", "Record id (required)")
	// Repeated --field K=V. flag.Var with a custom type captures all
	// occurrences; flag.String only keeps the last.
	fields := &kvFlag{}
	fs.Var(fields, "field",
		"Field assignment in K=V form. Repeat for multiple fields. "+
			"Use --field K=null to clear a value.")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("record.update", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("record.update",
			errors.New("--id is required"), stdout, mode)
	}
	if len(*id) < 15 {
		return writeArgErr("record.update",
			fmt.Errorf("invalid record id %q (must be 15 or 18 chars)", *id),
			stdout, mode)
	}
	if len(fields.values) == 0 {
		return writeArgErr("record.update",
			errors.New("at least one --field K=V is required"), stdout, mode)
	}

	// Materialise the field map. "null" → JSON null (clear).
	payload := make(map[string]any, len(fields.values))
	for k, v := range fields.values {
		if v == "null" {
			payload[k] = nil
		} else {
			payload[k] = v
		}
	}

	result, err := a.RecordWrites().Update(context.Background(), records.UpdateInput{
		Target: *target, SObject: *object, ID: *id, Fields: payload,
	})
	if err != nil {
		return writeRecordOperationErr("record.update", *target, result.Target,
			result.SObject, *id, err, stdout, mode)
	}
	if len(result.FieldErrors) > 0 {
		// Salesforce validation rejection — invalid_argument is the
		// right shape since the caller passed bad data. Include the
		// per-field errors verbatim so agents can correct + retry.
		errs := make([]map[string]any, 0, len(result.FieldErrors))
		for _, fe := range result.FieldErrors {
			errs = append(errs, map[string]any{
				"error_code": fe.ErrorCode,
				"message":    fe.Message,
				"fields":     fe.Fields,
			})
		}
		r := headless.Fail("record.update", result.Target.Username, headless.ErrInvalidArgument,
			fmt.Sprintf("salesforce rejected update: %s", result.FieldErrors[0].String()),
			map[string]any{
				"object":           result.SObject,
				"id":               *id,
				"field_errors":     errs,
				"submitted_fields": kvFlagKeys(fields),
			})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}

	// Success — return the submitted field set so agents can confirm
	// what was actually sent (helpful when an automation chain logs
	// each step).
	r := headless.Success("record.update", result.Target.Username, result.Target.CLIArg, true,
		map[string]any{
			"object": result.SObject,
			"id":     *id,
			"fields": fields.values,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// kvFlag is a flag.Value that collects repeated K=V pairs into a map.
// Used by --field on record update + by future verbs that take
// arbitrary key/value sets (e.g. record create in Phase 4).
type kvFlag struct {
	values map[string]string
}

func (k *kvFlag) String() string {
	if k == nil || len(k.values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(k.values))
	for kk, vv := range k.values {
		parts = append(parts, kk+"="+vv)
	}
	return strings.Join(parts, ",")
}

func (k *kvFlag) Set(s string) error {
	idx := strings.IndexByte(s, '=')
	if idx <= 0 {
		return fmt.Errorf("expected K=V, got %q", s)
	}
	key := strings.TrimSpace(s[:idx])
	val := s[idx+1:]
	if key == "" {
		return fmt.Errorf("field key cannot be empty (got %q)", s)
	}
	if k.values == nil {
		k.values = map[string]string{}
	}
	k.values[key] = val
	return nil
}

// kvFlagKeys returns the keys of a parsed kvFlag's value map in a
// flag.Visit-stable order. Used for diagnostic output.
func kvFlagKeys(k *kvFlag) []string {
	out := make([]string, 0, len(k.values))
	for kk := range k.values {
		out = append(out, kk)
	}
	return out
}

// Compile-time check: kvFlag satisfies flag.Value.
var _ flag.Value = (*kvFlag)(nil)

func writeRecordErr(command, orgUser, objectName, recordID string, err error, stdout io.Writer, mode headless.WriteMode) int {
	low := strings.ToLower(err.Error())
	if strings.Contains(low, "no "+strings.ToLower(objectName)+" with id ") {
		r := headless.Fail(command, orgUser, headless.ErrNotFound,
			err.Error(), map[string]any{"object": objectName, "id": recordID})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	return writeDescribeErr(command, orgUser, recordID, err, stdout, mode)
}

func writeRecordOperationErr(command, requestedTarget string, target orgwrite.Target,
	objectName, recordID string, err error, stdout io.Writer, mode headless.WriteMode) int {
	var blocked app.BlockedError
	if errors.As(err, &blocked) {
		return writeSafetyBlocked(command, blocked.Username, blocked, stdout, mode)
	}
	var resolveErr orgwrite.ResolutionError
	if errors.As(err, &resolveErr) {
		return writeOrgErr(command, resolveErr.Target, resolveErr.Err, stdout, mode)
	}
	orgUser := target.Username
	if orgUser == "" {
		orgUser = requestedTarget
	}
	return writeRecordErr(command, orgUser, objectName, recordID, err, stdout, mode)
}

// recordCreate POSTs a new record. WriteRecord safety gate, same as
// recordUpdate. --object is required (we can't infer for a record
// that doesn't have an id yet). Fields come in as repeated --field
// K=V flags; "null" is honoured as the JSON null literal (rare for
// create but consistent with update).
func recordCreate(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("record create")
	target := fs.String("org", "", "Alias or username (empty = default)")
	object := fs.String("object", "", "sObject API name (required)")
	fields := &kvFlag{}
	fs.Var(fields, "field",
		"Field assignment in K=V form. Repeat for multiple fields.")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("record.create", err, stdout, mode)
	}
	if *object == "" {
		return writeArgErr("record.create",
			errors.New("--object is required"), stdout, mode)
	}
	if len(fields.values) == 0 {
		return writeArgErr("record.create",
			errors.New("at least one --field K=V is required"), stdout, mode)
	}
	payload := make(map[string]any, len(fields.values))
	for k, v := range fields.values {
		if v == "null" {
			payload[k] = nil
		} else {
			payload[k] = v
		}
	}
	result, err := a.RecordWrites().Create(context.Background(), records.CreateInput{
		Target: *target, SObject: *object, Fields: payload,
	})
	if err != nil {
		return writeRecordOperationErr("record.create", *target, result.Target,
			*object, "", err, stdout, mode)
	}
	if len(result.FieldErrors) > 0 {
		errs := make([]map[string]any, 0, len(result.FieldErrors))
		for _, fe := range result.FieldErrors {
			errs = append(errs, map[string]any{
				"error_code": fe.ErrorCode,
				"message":    fe.Message,
				"fields":     fe.Fields,
			})
		}
		r := headless.Fail("record.create", result.Target.Username, headless.ErrInvalidArgument,
			fmt.Sprintf("salesforce rejected create: %s", result.FieldErrors[0].String()),
			map[string]any{
				"object":           *object,
				"field_errors":     errs,
				"submitted_fields": kvFlagKeys(fields),
			})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	r := headless.Success("record.create", result.Target.Username, result.Target.CLIArg, true,
		map[string]any{
			"object": *object,
			"id":     result.ID,
			"fields": fields.values,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// recordDelete removes a record by id. WriteRecord safety gate. The
// sObject can be inferred from the id key prefix when --object is
// omitted, same as record.get.
func recordDelete(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("record delete")
	target := fs.String("org", "", "Alias or username (empty = default)")
	object := fs.String("object", "",
		"sObject API name (inferred from --id key prefix if omitted)")
	id := fs.String("id", "", "Record id (required)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("record.delete", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("record.delete",
			errors.New("--id is required"), stdout, mode)
	}
	if len(*id) < 15 {
		return writeArgErr("record.delete",
			fmt.Errorf("invalid record id %q (must be 15 or 18 chars)", *id),
			stdout, mode)
	}
	result, err := a.RecordWrites().Delete(context.Background(), records.DeleteInput{
		Target: *target, SObject: *object, ID: *id,
	})
	if err != nil {
		return writeRecordOperationErr("record.delete", *target, result.Target,
			result.SObject, *id, err, stdout, mode)
	}
	r := headless.Success("record.delete", result.Target.Username, result.Target.CLIArg, true,
		map[string]any{
			"object": result.SObject,
			"id":     *id,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}
