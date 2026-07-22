package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
)

// soqlSaved routes `sf-deck soql saved <subverb>`. Local-only — the
// saved-query library lives in devprojects.db. Mirrors apex.snippet
// structure deliberately so agents only have to learn one CRUD
// shape across both libraries.
func soqlSaved(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	subverb := ""
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		subverb = rest[0]
		rest = rest[1:]
	}
	if subverb == "" {
		subverb = "list"
	}
	if a.Projects == nil {
		r := headless.Fail("soql.saved."+subverb, "", headless.ErrInternal,
			"devprojects store unavailable", nil)
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	switch subverb {
	case "list":
		return soqlSavedList(a, rest, stdout, mode)
	case "show":
		return soqlSavedShow(a, rest, stdout, mode)
	case "create":
		return soqlSavedCreate(a, rest, stdout, mode)
	case "update":
		return soqlSavedUpdate(a, rest, stdout, mode)
	case "delete":
		return soqlSavedDelete(a, rest, stdout, mode)
	}
	r := headless.Fail("soql.saved."+subverb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown saved subverb %q", subverb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func soqlSavedList(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("soql saved list")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("soql.saved.list", err, stdout, mode)
	}
	all, err := a.Projects.ListSavedQueries()
	if err != nil {
		return writeArgErr("soql.saved.list", err, stdout, mode)
	}
	// Summary projection — no body. Same rationale as apex snippet
	// list: an agent listing the saved-query library shouldn't dump
	// every saved SOQL into stdout. Callers pull the body via
	// `soql saved show --id ...`.
	out := make([]map[string]any, 0, len(all))
	for _, s := range all {
		out = append(out, savedQuerySummary(s))
	}
	r := headless.Success("soql.saved.list", "", "", false,
		map[string]any{"queries": out, "count": len(out)})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func soqlSavedShow(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("soql saved show")
	id := fs.String("id", "", "Saved query id (required)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("soql.saved.show", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("soql.saved.show",
			errors.New("--id is required"), stdout, mode)
	}
	q, err := a.Projects.GetSavedQuery(*id)
	if err != nil {
		return writeSavedQueryErr("soql.saved.show", *id, err, stdout, mode)
	}
	r := headless.Success("soql.saved.show", "", "", false,
		map[string]any{"query": savedQueryView(q)})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func soqlSavedCreate(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("soql saved create")
	name := fs.String("name", "", "Saved-query name (required)")
	desc := fs.String("description", "", "Description")
	body := fs.String("query", "", "Inline SOQL string")
	bodyFile := fs.String("query-file", "",
		"Path to file with SOQL ('-' for stdin)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("soql.saved.create", err, stdout, mode)
	}
	src, err := resolveSOQL(*body, *bodyFile)
	if err != nil {
		return writeArgErr("soql.saved.create", err, stdout, mode)
	}
	if strings.TrimSpace(*name) == "" || strings.TrimSpace(src) == "" {
		return writeArgErr("soql.saved.create",
			errors.New("--name and (--query or --query-file) are required"), stdout, mode)
	}
	q, err := a.Projects.CreateSavedQuery(*name, *desc, src)
	if err != nil {
		return writeSavedQueryErr("soql.saved.create", *name, err, stdout, mode)
	}
	r := headless.Success("soql.saved.create", "", "", true,
		map[string]any{"query": savedQueryView(q)})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func soqlSavedUpdate(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("soql saved update")
	id := fs.String("id", "", "Saved query id (required)")
	name := fs.String("name", "", "New name")
	desc := fs.String("description", "", "New description")
	body := fs.String("query", "", "New SOQL")
	bodyFile := fs.String("query-file", "", "New SOQL from file")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("soql.saved.update", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("soql.saved.update",
			errors.New("--id is required"), stdout, mode)
	}
	cur, err := a.Projects.GetSavedQuery(*id)
	if err != nil {
		return writeSavedQueryErr("soql.saved.update", *id, err, stdout, mode)
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
	if visited["query"] || visited["query-file"] {
		src, berr := resolveSOQL(*body, *bodyFile)
		if berr != nil {
			return writeArgErr("soql.saved.update", berr, stdout, mode)
		}
		if src != newBody && src != "" {
			newBody = src
			changed = true
		}
	}
	if !changed {
		return writeArgErr("soql.saved.update",
			errors.New("no update fields specified"), stdout, mode)
	}
	if err := a.Projects.UpdateSavedQuery(*id, newName, newDesc, newBody); err != nil {
		return writeSavedQueryErr("soql.saved.update", *id, err, stdout, mode)
	}
	fresh, _ := a.Projects.GetSavedQuery(*id)
	r := headless.Success("soql.saved.update", "", "", true,
		map[string]any{"query": savedQueryView(fresh)})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func soqlSavedDelete(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("soql saved delete")
	id := fs.String("id", "", "Saved query id (required)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("soql.saved.delete", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("soql.saved.delete",
			errors.New("--id is required"), stdout, mode)
	}
	snap, err := a.Projects.GetSavedQuery(*id)
	if err != nil {
		return writeSavedQueryErr("soql.saved.delete", *id, err, stdout, mode)
	}
	if err := a.Projects.DeleteSavedQuery(*id); err != nil {
		return writeSavedQueryErr("soql.saved.delete", *id, err, stdout, mode)
	}
	r := headless.Success("soql.saved.delete", "", "", true,
		map[string]any{"query": savedQueryView(snap)})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// savedQueryView is the DETAIL projection — full body. Used by show
// / create / update / delete.
func savedQueryView(s devproject.SavedQuery) map[string]any {
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

// savedQuerySummary is the LIST projection — no body, body_chars +
// body_sha256 for size + change-detection. Mirrors snippetSummary.
func savedQuerySummary(s devproject.SavedQuery) map[string]any {
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

func writeSavedQueryErr(command, ref string, err error, stdout io.Writer, mode headless.WriteMode) int {
	if errors.Is(err, devproject.ErrSavedQueryNotFound) {
		r := headless.Fail(command, "", headless.ErrNotFound, err.Error(),
			map[string]any{"id": ref})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	if errors.Is(err, devproject.ErrSavedQueryEmpty) {
		r := headless.Fail(command, "", headless.ErrInvalidArgument, err.Error(), nil)
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	return writeArgErr(command, err, stdout, mode)
}
