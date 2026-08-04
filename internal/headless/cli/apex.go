package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/services/apexops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
)

// dispatchApex routes `sf-deck apex <verb>`:
//
//   - execute  : runs anonymous Apex against the org. WriteAnonymous
//     gate — Apex can do anything, so this is the highest
//     safety tier.
//   - snippet  : nested CRUD over saved-apex (local-only). No org
//     resolution, no safety gate; same pattern as `tag`
//   - `chip` which are also local-only writes.
func dispatchApex(a *app.App, args Args, stdout io.Writer, mode headless.WriteMode) int {
	verb := args.Verb
	if verb == "" {
		verb = "execute"
	}
	switch verb {
	case "execute":
		return apexExecute(a, args.Rest, stdout, mode)
	case "snippet":
		return apexSnippet(a, args.Rest, stdout, mode)
	}
	r := headless.Fail("apex."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown apex verb %q", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// apexExecute runs anonymous Apex. Three input modes:
//   - --body "Apex..."         : inline source
//   - --body-file path|-       : from file or stdin
//   - --snippet-id ax_...      : load from the saved-apex library
//
// Safety: WriteAnonymous. Apex bypasses every other gate (it can
// update records, deploy metadata, call out, drop tables), so even a
// scratch-org default of "full" is required.
//
// Result envelope: success → ok=true, data.compiled + data.success.
// Compile failure or runtime exception → ok=false, error.code =
// invalid_argument with the SF problem string + line/column.
// We DON'T conflate compile vs runtime failures into internal_error;
// both are caller-fixable (bad Apex syntax or runtime exception in
// the supplied script).
func apexExecute(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("apex execute")
	target := fs.String("org", "", "Alias or username (empty = default)")
	body := fs.String("body", "", "Inline Apex source")
	bodyFile := fs.String("body-file", "",
		"Path to a file with Apex source ('-' for stdin)")
	snippetID := fs.String("snippet-id", "",
		"Load body from a saved-apex snippet by id")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("apex.execute", err, stdout, mode)
	}

	src, source, err := resolveApexBody(a, *body, *bodyFile, *snippetID)
	if err != nil {
		return writeArgErr("apex.execute", err, stdout, mode)
	}
	if strings.TrimSpace(src) == "" {
		return writeArgErr("apex.execute",
			errors.New("--body, --body-file, or --snippet-id is required"), stdout, mode)
	}

	serviceResult, err := a.ApexWrites().Execute(context.Background(), apexops.ExecuteInput{
		Target: *target, Body: src,
	})
	if err != nil {
		return writeApexOperationErr(*target, serviceResult.Target, err, stdout, mode)
	}
	res := serviceResult.Execution
	orgUser := serviceResult.Target.Username

	// Body-handling policy:
	//   Success → DROP the body. The agent already has it; round-
	//             tripping bytes pollutes logs + context windows.
	//             Replace with source + body_chars + body_sha256 so
	//             callers can correlate runs with what they sent.
	//   Failure → KEEP the body. line/column references that come
	//             back from SF are unusable to a human reader
	//             without the source they refer to.

	bodyHash := sha256Hex(src)
	bodyChars := len(src)

	// Compile failure → invalid_argument. Body is malformed; the
	// caller fixes it and retries. Body kept so line/column means
	// something.
	if !res.Compiled {
		r := headless.Fail("apex.execute", orgUser,
			headless.ErrInvalidArgument,
			"apex compile error: "+res.CompileProblem,
			map[string]any{
				"compile_problem": res.CompileProblem,
				"line":            res.Line,
				"column":          res.Column,
				"took_ms":         res.Took.Milliseconds(),
				"source":          source,
				"body":            src,
				"body_chars":      bodyChars,
				"body_sha256":     bodyHash,
			})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}

	// Compiled but runtime exception → invalid_argument too. SF
	// catches its own exceptions before they crash the request, so
	// from the caller's perspective the script was bad input. Body
	// kept for the same reason as compile failure.
	if !res.Success {
		r := headless.Fail("apex.execute", orgUser,
			headless.ErrInvalidArgument,
			"apex runtime exception: "+res.ExceptionMessage,
			map[string]any{
				"exception_message": res.ExceptionMessage,
				"exception_stack":   res.ExceptionStack,
				"line":              res.Line,
				"column":            res.Column,
				"took_ms":           res.Took.Milliseconds(),
				"source":            source,
				"body":              src,
				"body_chars":        bodyChars,
				"body_sha256":       bodyHash,
			})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}

	// Success — drop body, keep correlation fields.
	r := headless.Success("apex.execute", orgUser, serviceResult.Target.CLIArg, true,
		map[string]any{
			"compiled":    true,
			"success":     true,
			"took_ms":     res.Took.Milliseconds(),
			"source":      source,
			"body_chars":  bodyChars,
			"body_sha256": bodyHash,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func writeApexOperationErr(requestedTarget string, target orgwrite.Target, err error,
	stdout io.Writer, mode headless.WriteMode) int {
	var blocked app.BlockedError
	if errors.As(err, &blocked) {
		return writeSafetyBlocked("apex.execute", blocked.Username, blocked, stdout, mode)
	}
	var resolveErr orgwrite.ResolutionError
	if errors.As(err, &resolveErr) {
		return writeOrgErr("apex.execute", resolveErr.Target, resolveErr.Err, stdout, mode)
	}
	orgUser := target.Username
	if orgUser == "" {
		orgUser = requestedTarget
	}
	return writeSOQLErr("apex.execute", orgUser, err, stdout, mode)
}

// resolveApexBody fans out the three input modes. Exactly one of
// --body / --body-file / --snippet-id must be set; multiple is a user
// mistake we surface up-front so they don't have to wonder which won.
//
// Returns (body, source, error) where source is a discriminator
// string for the response envelope:
//   - "inline"            : --body passed directly
//   - "file:<path>"       : --body-file path (or "stdin" when path = "-")
//   - "snippet:<id>"      : --snippet-id from the saved library
//
// source lets a successful `apex execute` envelope describe what got
// run without echoing the source — agents can correlate runs with
// audit log entries by source + body_sha256.
func resolveApexBody(a *app.App, inline, path, snippetID string) (body, source string, err error) {
	count := 0
	if inline != "" {
		count++
	}
	if path != "" {
		count++
	}
	if snippetID != "" {
		count++
	}
	switch count {
	case 0:
		return "", "", nil // caller-side decides whether that's an error
	case 1:
		// fallthrough to handler below
	default:
		return "", "",
			errors.New("--body, --body-file, --snippet-id are mutually exclusive")
	}
	if inline != "" {
		return inline, "inline", nil
	}
	if path != "" {
		// Re-use resolveSOQL — it does file + stdin reading; the
		// content interpretation is the caller's job, so the helper
		// is generic enough.
		b, err := resolveSOQL("", path)
		if err != nil {
			return "", "", err
		}
		src := "file:" + path
		if path == "-" {
			src = "stdin"
		}
		return b, src, nil
	}
	if a == nil || a.Projects == nil {
		return "", "", errors.New("devprojects store unavailable; --snippet-id needs it")
	}
	snip, err := a.Projects.GetSavedApex(snippetID)
	if err != nil {
		if errors.Is(err, devproject.ErrSavedApexNotFound) {
			return "", "", fmt.Errorf("snippet %s not found", snippetID)
		}
		return "", "", err
	}
	return snip.Body, "snippet:" + snippetID, nil
}

// apexSnippet routes `sf-deck apex snippet <subverb>`. Local-only —
// snippets live in devprojects.db; no org calls.
func apexSnippet(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	subverb := ""
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		subverb = rest[0]
		rest = rest[1:]
	}
	if subverb == "" {
		subverb = "list"
	}
	if a.Projects == nil {
		r := headless.Fail("apex.snippet."+subverb, "", headless.ErrInternal,
			"devprojects store unavailable", nil)
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	switch subverb {
	case "list":
		return apexSnippetList(a, rest, stdout, mode)
	case "show":
		return apexSnippetShow(a, rest, stdout, mode)
	case "create":
		return apexSnippetCreate(a, rest, stdout, mode)
	case "update":
		return apexSnippetUpdate(a, rest, stdout, mode)
	case "delete":
		return apexSnippetDelete(a, rest, stdout, mode)
	}
	r := headless.Fail("apex.snippet."+subverb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown snippet subverb %q", subverb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func apexSnippetList(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("apex snippet list")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("apex.snippet.list", err, stdout, mode)
	}
	all, err := a.Projects.ListSavedApex()
	if err != nil {
		return writeArgErr("apex.snippet.list", err, stdout, mode)
	}
	// List uses the SUMMARY projection — no body. A library of 200
	// snippets can run hundreds of KB of Apex source; an agent
	// listing snippets shouldn't accidentally exfiltrate everything
	// into a logs / context-window dump. Callers fetch the body via
	// `apex snippet show --id ...`.
	out := make([]map[string]any, 0, len(all))
	for _, s := range all {
		out = append(out, snippetSummary(s))
	}
	r := headless.Success("apex.snippet.list", "", "", false,
		map[string]any{
			"snippets": out,
			"count":    len(out),
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func apexSnippetShow(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("apex snippet show")
	id := fs.String("id", "", "Snippet id (required)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("apex.snippet.show", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("apex.snippet.show",
			errors.New("--id is required"), stdout, mode)
	}
	snip, err := a.Projects.GetSavedApex(*id)
	if err != nil {
		return writeSnippetErr("apex.snippet.show", *id, err, stdout, mode)
	}
	r := headless.Success("apex.snippet.show", "", "", false,
		map[string]any{"snippet": snippetView(snip)})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func apexSnippetCreate(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("apex snippet create")
	name := fs.String("name", "", "Snippet name (required)")
	desc := fs.String("description", "", "Description")
	body := fs.String("body", "", "Inline Apex source")
	bodyFile := fs.String("body-file", "",
		"Path to a file with Apex source ('-' for stdin)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("apex.snippet.create", err, stdout, mode)
	}
	src, _, err := resolveApexBody(nil, *body, *bodyFile, "")
	if err != nil {
		return writeArgErr("apex.snippet.create", err, stdout, mode)
	}
	if strings.TrimSpace(*name) == "" || strings.TrimSpace(src) == "" {
		return writeArgErr("apex.snippet.create",
			errors.New("--name and (--body or --body-file) are required"), stdout, mode)
	}
	snip, err := a.Projects.CreateSavedApex(*name, *desc, src)
	if err != nil {
		return writeSnippetErr("apex.snippet.create", *name, err, stdout, mode)
	}
	r := headless.Success("apex.snippet.create", "", "", true,
		map[string]any{"snippet": snippetView(snip)})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func apexSnippetUpdate(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("apex snippet update")
	id := fs.String("id", "", "Snippet id (required)")
	name := fs.String("name", "", "New name")
	desc := fs.String("description", "", "New description")
	body := fs.String("body", "", "New inline Apex source")
	bodyFile := fs.String("body-file", "", "New body from file")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("apex.snippet.update", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("apex.snippet.update",
			errors.New("--id is required"), stdout, mode)
	}
	cur, err := a.Projects.GetSavedApex(*id)
	if err != nil {
		return writeSnippetErr("apex.snippet.update", *id, err, stdout, mode)
	}
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	newName, newDesc, newBody := cur.Name, cur.Description, cur.Body
	changed := false
	if visited["name"] && *name != newName {
		newName = *name
		changed = true
	}
	if visited["description"] && *desc != newDesc {
		newDesc = *desc
		changed = true
	}
	if visited["body"] || visited["body-file"] {
		src, _, berr := resolveApexBody(nil, *body, *bodyFile, "")
		if berr != nil {
			return writeArgErr("apex.snippet.update", berr, stdout, mode)
		}
		if src != newBody && src != "" {
			newBody = src
			changed = true
		}
	}
	if !changed {
		return writeArgErr("apex.snippet.update",
			errors.New("no update fields specified"), stdout, mode)
	}
	if err := a.Projects.UpdateSavedApex(*id, newName, newDesc, newBody); err != nil {
		return writeSnippetErr("apex.snippet.update", *id, err, stdout, mode)
	}
	fresh, _ := a.Projects.GetSavedApex(*id)
	r := headless.Success("apex.snippet.update", "", "", true,
		map[string]any{"snippet": snippetView(fresh)})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func apexSnippetDelete(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("apex snippet delete")
	id := fs.String("id", "", "Snippet id (required)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("apex.snippet.delete", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("apex.snippet.delete",
			errors.New("--id is required"), stdout, mode)
	}
	snap, err := a.Projects.GetSavedApex(*id)
	if err != nil {
		return writeSnippetErr("apex.snippet.delete", *id, err, stdout, mode)
	}
	if err := a.Projects.DeleteSavedApex(*id); err != nil {
		return writeSnippetErr("apex.snippet.delete", *id, err, stdout, mode)
	}
	r := headless.Success("apex.snippet.delete", "", "", true,
		map[string]any{"snippet": snippetView(snap)})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// snippetView is the DETAIL projection — full body included. Used by
// show, create, update, delete responses where the caller is
// inspecting one specific snippet.
func snippetView(s devproject.SavedApex) map[string]any {
	return map[string]any{
		"id":          s.ID,
		"name":        s.Name,
		"description": s.Description,
		"body":        s.Body,
		"body_chars":  len(s.Body),
		"body_sha256": sha256Hex(s.Body),
		"created_at":  s.CreatedAt,
		"updated_at":  s.UpdatedAt,
	}
}

// snippetSummary is the LIST projection — no body, replaced by size
// + hash so callers can dedupe / detect changes without paying the
// byte cost of every snippet's source. body_sha256 also lets a
// downstream tool ask "did this snippet change?" cheaply.
func snippetSummary(s devproject.SavedApex) map[string]any {
	return map[string]any{
		"id":          s.ID,
		"name":        s.Name,
		"description": s.Description,
		"body_chars":  len(s.Body),
		"body_sha256": sha256Hex(s.Body),
		"created_at":  s.CreatedAt,
		"updated_at":  s.UpdatedAt,
	}
}

// sha256Hex returns the hex SHA-256 of s. Used by list/summary
// projections so a caller can detect "is this snippet still the
// same?" without re-fetching the body. Empty string in → empty hash
// kept for symmetry; a 32-byte hash of empty bytes is fine.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// writeSnippetErr maps devproject.ErrSavedApexNotFound → not_found,
// ErrSavedApexEmpty → invalid_argument, anything else → internal.
func writeSnippetErr(command, ref string, err error, stdout io.Writer, mode headless.WriteMode) int {
	if errors.Is(err, devproject.ErrSavedApexNotFound) {
		r := headless.Fail(command, "", headless.ErrNotFound, err.Error(),
			map[string]any{"id": ref})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	if errors.Is(err, devproject.ErrSavedApexEmpty) {
		r := headless.Fail(command, "", headless.ErrInvalidArgument, err.Error(), nil)
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	return writeArgErr(command, err, stdout, mode)
}
