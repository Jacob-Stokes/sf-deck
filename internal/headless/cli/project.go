package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/services/projects"
)

func dispatchProject(a *app.App, args Args, stdout io.Writer, mode headless.WriteMode) int {
	verb := args.Verb
	if verb == "" {
		verb = "list"
	}
	if a.Projects == nil {
		r := headless.Fail("project."+verb, "", headless.ErrInternal,
			"devprojects store unavailable", nil)
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	switch verb {
	case "list":
		return projectList(a, args.Rest, stdout, mode)
	case "show":
		return projectShow(a, args.Rest, stdout, mode)
	case "create":
		return projectCreate(a, args.Rest, stdout, mode)
	case "update":
		return projectUpdate(a, args.Rest, stdout, mode)
	case "delete":
		return projectDelete(a, args.Rest, stdout, mode)
	case "add-item":
		return projectAddItem(a, args.Rest, stdout, mode)
	case "remove-item":
		return projectRemoveItem(a, args.Rest, stdout, mode)
	case "items":
		return projectItems(a, args.Rest, stdout, mode)
	case "import-bundle":
		return projectImportBundle(a, args.Rest, stdout, mode)
	}
	r := headless.Fail("project."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown project verb %q", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func projectList(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("project list")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("project.list", err, stdout, mode)
	}
	list, err := projects.List(a.Projects)
	if err != nil {
		return writeArgErr("project.list", err, stdout, mode)
	}
	r := headless.Success("project.list", "", "", false, map[string]any{
		"projects": list,
		"count":    len(list),
	})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func projectShow(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("project show")
	id := fs.String("id", "", "Project id")
	name := fs.String("name", "", "Project name (case-insensitive)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("project.show", err, stdout, mode)
	}
	p, err := projects.Show(a.Projects, *id, *name)
	if err != nil {
		return writeProjectErr("project.show", err, stdout, mode)
	}
	r := headless.Success("project.show", "", "", false,
		map[string]any{"project": p})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func projectCreate(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("project create")
	id := fs.String("id", "", "Optional explicit id (default: random)")
	name := fs.String("name", "", "Project name (required)")
	desc := fs.String("description", "", "Project description")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("project.create", err, stdout, mode)
	}
	res, err := projects.Create(a.Projects, projects.CreateInput{
		ID: *id, Name: *name, Description: *desc,
	})
	if err != nil {
		return writeProjectErr("project.create", err, stdout, mode)
	}
	r := headless.Success("project.create", "", "", res.Changed,
		map[string]any{"project": res.Project})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func projectUpdate(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("project update")
	id := fs.String("id", "", "Project id (required)")
	name := fs.String("name", "", "New name")
	desc := fs.String("description", "", "New description")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("project.update", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("project.update",
			errors.New("--id is required"), stdout, mode)
	}
	in := projects.UpdateInput{}
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	if visited["name"] {
		in.Name = name
	}
	if visited["description"] {
		in.Description = desc
	}
	res, err := projects.Update(a.Projects, *id, in)
	if err != nil {
		return writeProjectErr("project.update", err, stdout, mode)
	}
	r := headless.Success("project.update", "", "", res.Changed,
		map[string]any{"project": res.Project})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func projectDelete(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("project delete")
	id := fs.String("id", "", "Project id (required)")
	force := fs.Bool("force", false, "Cascade-delete items if non-empty")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("project.delete", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("project.delete",
			errors.New("--id is required"), stdout, mode)
	}
	res, err := projects.Delete(a.Projects, *id, *force)
	if err != nil {
		return writeProjectErr("project.delete", err, stdout, mode)
	}
	r := headless.Success("project.delete", "", "", res.Changed,
		map[string]any{"project": res.Project})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func projectAddItem(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("project add-item")
	in := projects.AddItemInput{}
	fs.StringVar(&in.ProjectID, "project-id", "", "Project id (required)")
	fs.StringVar(&in.Kind, "kind", "", "Item kind (required)")
	fs.StringVar(&in.Ref, "ref", "", "Item reference (required)")
	fs.StringVar(&in.OrgUser, "org-user", "", "Origin org username")
	fs.StringVar(&in.Type, "type", "", "Type context (parent sobject etc.)")
	fs.StringVar(&in.Name, "name", "", "Display label")
	fs.StringVar(&in.Notes, "notes", "", "Freeform notes")
	fs.StringVar(&in.Namespace, "namespace", "", "Managed-package namespace prefix")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("project.add-item", err, stdout, mode)
	}
	if in.ProjectID == "" || in.Kind == "" || in.Ref == "" {
		return writeArgErr("project.add-item",
			errors.New("--project-id, --kind, --ref are required"), stdout, mode)
	}
	res, err := projects.AddItem(a.Projects, in)
	if err != nil {
		return writeProjectErr("project.add-item", err, stdout, mode)
	}
	r := headless.Success("project.add-item", in.OrgUser, in.OrgUser, res.Changed,
		map[string]any{"item": res.Item, "project_id": in.ProjectID})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func projectRemoveItem(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("project remove-item")
	projectID := fs.String("project-id", "", "Project id (required)")
	kind := fs.String("kind", "", "Item kind (required)")
	ref := fs.String("ref", "", "Item ref (required)")
	orgUser := fs.String("org-user", "", "Origin org username (must match)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("project.remove-item", err, stdout, mode)
	}
	if *projectID == "" || *kind == "" || *ref == "" {
		return writeArgErr("project.remove-item",
			errors.New("--project-id, --kind, --ref are required"), stdout, mode)
	}
	res, err := projects.RemoveItem(a.Projects, *projectID, *orgUser, *kind, *ref)
	if err != nil {
		return writeProjectErr("project.remove-item", err, stdout, mode)
	}
	r := headless.Success("project.remove-item", *orgUser, *orgUser, res.Changed,
		map[string]any{"item": res.Item, "project_id": *projectID})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func projectItems(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("project items")
	id := fs.String("id", "", "Project id (required)")
	orgUser := fs.String("org-user", "", "Filter to one org (empty = all)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("project.items", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("project.items",
			errors.New("--id is required"), stdout, mode)
	}
	rows, err := projects.ListItems(a.Projects, *id, *orgUser)
	if err != nil {
		return writeProjectErr("project.items", err, stdout, mode)
	}
	r := headless.Success("project.items", *orgUser, *orgUser, false,
		map[string]any{
			"project_id": *id,
			"items":      rows,
			"count":      len(rows),
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// projectImportBundle parses a package.xml from an existing sfdx
// project directory and inserts each member as a project item.
// Idempotent — items already in the project are skipped.
func projectImportBundle(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("project import-bundle")
	projectID := fs.String("project-id", "", "Dev project id (required)")
	path := fs.String("path", "",
		"Path to an sfdx project directory or its package.xml (required)")
	target := fs.String("org", "",
		"Org to stamp on the imported items (resolves to --org-user)")
	orgUser := fs.String("org-user", "",
		"Username to stamp on imported items (alternative to --org)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("project.import-bundle", err, stdout, mode)
	}
	if *projectID == "" || *path == "" {
		return writeArgErr("project.import-bundle",
			errors.New("--project-id and --path are required"), stdout, mode)
	}
	stampUser := *orgUser
	stampOrg := ""
	if *target != "" {
		o, err := a.ResolveOrg(*target)
		if err != nil {
			return writeOrgErr("project.import-bundle", *target, err, stdout, mode)
		}
		if stampUser == "" {
			stampUser = o.Username
		}
		stampOrg = o.Username
	}
	in := projects.ImportBundleInput{
		ProjectID: *projectID,
		Path:      *path,
		OrgUser:   stampUser,
	}
	res, err := projects.ImportBundle(a.Projects, in)
	if err != nil {
		return writeProjectErr("project.import-bundle", err, stdout, mode)
	}
	r := headless.Success("project.import-bundle", stampOrg, stampOrg, res.Added > 0,
		map[string]any{
			"project_id":    *projectID,
			"path":          *path,
			"added":         res.Added,
			"skipped":       res.Skipped,
			"unknown":       res.Unknown,
			"added_by_kind": res.AddedByKind,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// writeProjectErr maps service errors → typed headless codes. Three
// special cases: ErrNotFound → not_found, ErrNotEmpty → invalid_argument
// (caller can retry with --force), ErrInvalidKind → invalid_argument.
func writeProjectErr(command string, err error, stdout io.Writer, mode headless.WriteMode) int {
	var notFound projects.ErrNotFound
	if errors.As(err, &notFound) {
		details := map[string]any{}
		if notFound.ID != "" {
			details["id"] = notFound.ID
		}
		if notFound.Name != "" {
			details["name"] = notFound.Name
		}
		r := headless.Fail(command, "", headless.ErrNotFound, err.Error(), details)
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	var notEmpty projects.ErrNotEmpty
	if errors.As(err, &notEmpty) {
		r := headless.Fail(command, "", headless.ErrInvalidArgument, err.Error(),
			map[string]any{
				"id":         notEmpty.ID,
				"items":      notEmpty.Items,
				"force_flag": "--force",
				"force_hint": "re-run with --force to cascade-delete",
			})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	var badKind projects.ErrInvalidKind
	if errors.As(err, &badKind) {
		r := headless.Fail(command, "", headless.ErrInvalidArgument, err.Error(),
			map[string]any{"kind": badKind.Kind})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	var badRef projects.ErrInvalidRef
	if errors.As(err, &badRef) {
		details := map[string]any{
			"kind": badRef.Kind,
			"ref":  badRef.Ref,
		}
		if badRef.Hint != "" {
			details["hint"] = badRef.Hint
		}
		r := headless.Fail(command, "", headless.ErrInvalidArgument, err.Error(), details)
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	return writeArgErr(command, err, stdout, mode)
}
