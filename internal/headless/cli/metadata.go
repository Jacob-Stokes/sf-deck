package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/services/metadataops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// dispatchMetadata routes `sf-deck metadata <verb>` — a generic
// surface over the Tooling-API metadata CRUD primitives. The same
// verbs work against every Tooling-writable type (CustomField,
// ValidationRule, RecordType, ApexTrigger, CustomObject, FlexiPage,
// …) so an agent only learns one shape.
//
// Reads (get) use the read-only path and skip the safety gate.
// Writes route through metadataops: create/update require WriteMetadata;
// destructive delete requires WriteAnonymous/full. Sandbox orgs default to
// "records" so writes are blocked unless the user explicitly raises safety.
func dispatchMetadata(a *app.App, args Args, stdout io.Writer, mode headless.WriteMode) int {
	verb := args.Verb
	if verb == "" {
		verb = "get"
	}
	switch verb {
	case "get":
		return metadataGet(a, args.Rest, stdout, mode)
	case "create":
		return metadataCreate(a, args.Rest, stdout, mode)
	case "update":
		return metadataUpdate(a, args.Rest, stdout, mode)
	case "delete":
		return metadataDelete(a, args.Rest, stdout, mode)
	}
	r := headless.Fail("metadata."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown metadata verb %q", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// knownToolingTypes is the gate for --type. Salesforce will accept
// any string and return INVALID_TYPE for unrecognised ones, but we
// reject up-front so agents get a clean invalid_argument with the
// allowed set listed.
//
// Not exhaustive — Tooling has dozens of metadata sobjects. Add a
// type here when an agent workflow actually needs it; over-listing
// adds noise to the help text and invites typos.
var knownToolingTypes = func() map[string]bool {
	out := make(map[string]bool)
	for _, metadataType := range metadataops.KnownTypes() {
		out[metadataType] = true
	}
	return out
}()

func knownToolingTypesList() []string {
	return metadataops.KnownTypes()
}

// validateToolingType returns invalid_argument-ready error if the
// type isn't in the closed set.
func validateToolingType(t string) error {
	if t == "" {
		return errors.New("--type is required")
	}
	if !knownToolingTypes[t] {
		return fmt.Errorf("unknown --type %q (want one of %s)",
			t, strings.Join(knownToolingTypesList(), ", "))
	}
	return nil
}

// metadataGet reads a single Tooling-sobject row's Metadata map.
// Read-only — no safety gate.
func metadataGet(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("metadata get")
	target := fs.String("org", "", "Alias or username (empty = default)")
	mdType := fs.String("type", "", "Tooling metadata type (required)")
	id := fs.String("id", "", "Record id (required)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("metadata.get", err, stdout, mode)
	}
	if err := validateToolingType(*mdType); err != nil {
		return writeArgErr("metadata.get", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("metadata.get",
			errors.New("--id is required"), stdout, mode)
	}
	o, err := a.ResolveOrg(*target)
	if err != nil {
		return writeOrgErr("metadata.get", *target, err, stdout, mode)
	}
	md, err := sf.GetToolingMetadata(app.TargetArg(o), *mdType, *id)
	if err != nil {
		return writeDescribeErr("metadata.get", o.Username, *id, err, stdout, mode)
	}
	r := headless.Success("metadata.get", o.Username, app.TargetArg(o), false,
		map[string]any{
			"type":     *mdType,
			"id":       *id,
			"metadata": md,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// metadataCreate POSTs a new Tooling-sobject row. Required:
// --full-name (Tooling's "<parent>.<child>" identifier) + --patch
// JSON (the Metadata payload). WriteMetadata gate.
func metadataCreate(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("metadata create")
	target := fs.String("org", "", "Alias or username (empty = default)")
	mdType := fs.String("type", "", "Tooling metadata type (required)")
	fullName := fs.String("full-name", "",
		"Tooling full name, e.g. 'Account.MyField__c' (required)")
	patchRaw := fs.String("patch", "",
		"Metadata as JSON object literal (required, or --patch-file)")
	patchFile := fs.String("patch-file", "",
		"Read JSON Metadata from file ('-' for stdin)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("metadata.create", err, stdout, mode)
	}
	if err := validateToolingType(*mdType); err != nil {
		return writeArgErr("metadata.create", err, stdout, mode)
	}
	if *fullName == "" {
		return writeArgErr("metadata.create",
			errors.New("--full-name is required"), stdout, mode)
	}
	patch, perr := resolvePatch(*patchRaw, *patchFile)
	if perr != nil {
		return writeArgErr("metadata.create", perr, stdout, mode)
	}
	if patch == nil {
		return writeArgErr("metadata.create",
			errors.New("--patch or --patch-file is required"), stdout, mode)
	}
	res, err := a.MetadataWrites().Create(context.Background(), metadataops.CreateInput{
		Target: *target, Type: *mdType, FullName: *fullName, Metadata: patch,
	})
	if err != nil {
		return writeMetadataOperationErr("metadata.create", *target, *fullName, res.Target, err, stdout, mode)
	}
	r := headless.Success("metadata.create", res.Target.Username, res.Target.CLIArg, true,
		map[string]any{
			"type":      *mdType,
			"full_name": *fullName,
			"id":        res.ID,
			"metadata":  patch,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// metadataUpdate PATCHes the Metadata of an existing row. The
// patch is merged on top of the current state by the underlying
// UpdateToolingMetadata — Tooling rejects partial PATCHes, so the
// service does GET-merge-PUT.
func metadataUpdate(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("metadata update")
	target := fs.String("org", "", "Alias or username (empty = default)")
	mdType := fs.String("type", "", "Tooling metadata type (required)")
	id := fs.String("id", "", "Record id (required)")
	patchRaw := fs.String("patch", "",
		"Metadata-fields patch as JSON object literal (required, or --patch-file)")
	patchFile := fs.String("patch-file", "",
		"Read JSON patch from file ('-' for stdin)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("metadata.update", err, stdout, mode)
	}
	if err := validateToolingType(*mdType); err != nil {
		return writeArgErr("metadata.update", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("metadata.update",
			errors.New("--id is required"), stdout, mode)
	}
	patch, perr := resolvePatch(*patchRaw, *patchFile)
	if perr != nil {
		return writeArgErr("metadata.update", perr, stdout, mode)
	}
	if patch == nil {
		return writeArgErr("metadata.update",
			errors.New("--patch or --patch-file is required"), stdout, mode)
	}
	res, err := a.MetadataWrites().Update(context.Background(), metadataops.UpdateInput{
		Target: *target, Type: *mdType, ID: *id, Patch: patch,
	})
	if err != nil {
		return writeMetadataOperationErr("metadata.update", *target, *id, res.Target, err, stdout, mode)
	}
	r := headless.Success("metadata.update", res.Target.Username, res.Target.CLIArg, true,
		map[string]any{
			"type":  *mdType,
			"id":    *id,
			"patch": patch,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// metadataDelete removes a Tooling row by id. Destructive operations
// require the full safety tier until we add a finer destructive
// metadata write kind. No --force flag because Salesforce already
// rejects deletes that would cascade-orphan dependent metadata; the
// SF error becomes invalid_argument.
func metadataDelete(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("metadata delete")
	target := fs.String("org", "", "Alias or username (empty = default)")
	mdType := fs.String("type", "", "Tooling metadata type (required)")
	id := fs.String("id", "", "Record id (required)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("metadata.delete", err, stdout, mode)
	}
	if err := validateToolingType(*mdType); err != nil {
		return writeArgErr("metadata.delete", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("metadata.delete",
			errors.New("--id is required"), stdout, mode)
	}
	res, err := a.MetadataWrites().Delete(context.Background(), metadataops.DeleteInput{
		Target: *target, Type: *mdType, ID: *id,
	})
	if err != nil {
		return writeMetadataOperationErr("metadata.delete", *target, *id, res.Target, err, stdout, mode)
	}
	r := headless.Success("metadata.delete", res.Target.Username, res.Target.CLIArg, true,
		map[string]any{"type": *mdType, "id": *id})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func writeMetadataOperationErr(command, requestedTarget, ref string, target orgwrite.Target, err error,
	stdout io.Writer, mode headless.WriteMode) int {
	var blocked app.BlockedError
	if errors.As(err, &blocked) {
		return writeSafetyBlocked(command, blocked.Username, blocked, stdout, mode)
	}
	var resolveErr orgwrite.ResolutionError
	if errors.As(err, &resolveErr) {
		return writeOrgErr(command, resolveErr.Target, resolveErr.Err, stdout, mode)
	}
	var invalidType metadataops.ErrInvalidType
	if errors.As(err, &invalidType) {
		return writeArgErr(command, err, stdout, mode)
	}
	orgUser := target.Username
	if orgUser == "" {
		orgUser = requestedTarget
	}
	return writeMetadataErr(command, orgUser, ref, err, stdout, mode)
}

// resolvePatch parses --patch / --patch-file into a map[string]any.
// Returns (nil, nil) when both are empty so the caller decides
// whether that's an error.
func resolvePatch(raw, path string) (map[string]any, error) {
	if raw != "" && path != "" {
		return nil, errors.New("--patch and --patch-file are mutually exclusive")
	}
	src := raw
	if src == "" && path != "" {
		var bytes []byte
		var err error
		if path == "-" {
			bytes, err = io.ReadAll(os.Stdin)
		} else {
			bytes, err = os.ReadFile(path)
		}
		if err != nil {
			return nil, fmt.Errorf("read patch: %w", err)
		}
		src = string(bytes)
	}
	if strings.TrimSpace(src) == "" {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(src), &out); err != nil {
		return nil, fmt.Errorf("invalid JSON in --patch: %w", err)
	}
	return out, nil
}

// writeMetadataErr classifies Tooling-API errors. Most rejections
// are FIELD_INTEGRITY / INVALID_FIELD style; map those to
// invalid_argument. Auth + 4xx auth-class go to ErrAuth. 404 goes
// to ErrNotFound.
func writeMetadataErr(command, orgUser, ref string, err error, stdout io.Writer, mode headless.WriteMode) int {
	low := strings.ToLower(err.Error())
	switch {
	case strings.Contains(low, "not_found"),
		strings.Contains(low, "not found"),
		strings.Contains(low, "404"):
		r := headless.Fail(command, orgUser, headless.ErrNotFound, err.Error(),
			map[string]any{"ref": ref})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	case strings.Contains(low, "field_integrity"),
		strings.Contains(low, "invalid_field"),
		strings.Contains(low, "duplicate_value"),
		strings.Contains(low, "malformed"),
		strings.Contains(low, "invalid_input"),
		strings.Contains(low, "missing_argument"):
		r := headless.Fail(command, orgUser, headless.ErrInvalidArgument, err.Error(),
			map[string]any{"ref": ref})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	return writeSOQLErr(command, orgUser, err, stdout, mode)
}
