package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/services/tags"
)

// dispatchTag routes `sf-deck tag <verb> ...`. Requires app.Projects
// to be non-nil; we render a clean typed error when it isn't so a
// missing-devproject (e.g. corrupt db / mkdir failure) doesn't crash
// the process.
func dispatchTag(a *app.App, args Args, stdout io.Writer, mode headless.WriteMode) int {
	verb := args.Verb
	if verb == "" {
		verb = "list"
	}
	if a.Projects == nil {
		r := headless.Fail("tag."+verb, "", headless.ErrInternal,
			"devprojects store unavailable", nil)
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	switch verb {
	case "list":
		return tagList(a, args.Rest, stdout, mode)
	case "show":
		return tagShow(a, args.Rest, stdout, mode)
	case "create":
		return tagCreate(a, args.Rest, stdout, mode)
	case "update":
		return tagUpdate(a, args.Rest, stdout, mode)
	case "delete":
		return tagDelete(a, args.Rest, stdout, mode)
	case "apply":
		return tagApply(a, args.Rest, stdout, mode)
	case "remove":
		return tagRemove(a, args.Rest, stdout, mode)
	case "set":
		return tagSet(a, args.Rest, stdout, mode)
	case "items":
		return tagItems(a, args.Rest, stdout, mode)
	case "of":
		return tagOf(a, args.Rest, stdout, mode)
	}
	r := headless.Fail("tag."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown tag verb %q", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func tagList(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("tag list")
	usageOnly := fs.Bool("usage-only", false, "Only return tags with bindings")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("tag.list", err, stdout, mode)
	}
	list, err := tags.List(a.Projects, *usageOnly)
	if err != nil {
		return writeArgErr("tag.list", err, stdout, mode)
	}
	r := headless.Success("tag.list", "", "", false, map[string]any{
		"tags":  list,
		"count": len(list),
	})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func tagShow(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("tag show")
	id := fs.Int64("id", 0, "Tag id")
	name := fs.String("name", "", "Tag name (case-insensitive)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("tag.show", err, stdout, mode)
	}
	tag, err := tags.Show(a.Projects, *id, *name)
	if err != nil {
		return writeTagErr("tag.show", err, stdout, mode)
	}
	r := headless.Success("tag.show", "", "", false, map[string]any{"tag": tag})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func tagCreate(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("tag create")
	name := fs.String("name", "", "Tag name (required)")
	color := fs.String("color", "", "Theme color name (blue, purple, red, …)")
	icon := fs.String("icon", "", "Optional unicode glyph prefix")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("tag.create", err, stdout, mode)
	}
	res, err := tags.Create(a.Projects, tags.CreateInput{
		Name: *name, Color: *color, Icon: *icon,
	})
	if err != nil {
		return writeTagErr("tag.create", err, stdout, mode)
	}
	r := headless.Success("tag.create", "", "", res.Changed,
		map[string]any{"tag": res.Tag})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func tagUpdate(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("tag update")
	id := fs.Int64("id", 0, "Tag id (required)")
	name := fs.String("name", "", "New name")
	color := fs.String("color", "", "New color")
	icon := fs.String("icon", "", "New icon")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("tag.update", err, stdout, mode)
	}
	if *id == 0 {
		return writeArgErr("tag.update", errors.New("--id is required"), stdout, mode)
	}
	in := tags.UpdateInput{}
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	if visited["name"] {
		in.Name = name
	}
	if visited["color"] {
		in.Color = color
	}
	if visited["icon"] {
		in.Icon = icon
	}
	res, err := tags.Update(a.Projects, *id, in)
	if err != nil {
		return writeTagErr("tag.update", err, stdout, mode)
	}
	r := headless.Success("tag.update", "", "", res.Changed,
		map[string]any{"tag": res.Tag})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func tagDelete(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("tag delete")
	id := fs.Int64("id", 0, "Tag id (required)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("tag.delete", err, stdout, mode)
	}
	if *id == 0 {
		return writeArgErr("tag.delete", errors.New("--id is required"), stdout, mode)
	}
	res, err := tags.Delete(a.Projects, *id)
	if err != nil {
		return writeTagErr("tag.delete", err, stdout, mode)
	}
	r := headless.Success("tag.delete", "", "", res.Changed,
		map[string]any{"tag": res.Tag})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// tagApply / tagRemove / tagSet all bind a tag to an item — they
// share the (kind, ref, org_user) input shape via parseBindingFlags.
type bindingFlags struct {
	kind, ref, orgUser string
}

func parseBindingFlags(fs *flag.FlagSet) *bindingFlags {
	b := &bindingFlags{}
	fs.StringVar(&b.kind, "kind", "",
		"Item kind (e.g. record, field, flow, soql_query)")
	fs.StringVar(&b.ref, "ref", "",
		"Item reference (record id, field developer name, flow developer name, …)")
	fs.StringVar(&b.orgUser, "org-user", "",
		"Org username (empty for org-agnostic items like soql_query)")
	return b
}

func tagApply(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("tag apply")
	id := fs.Int64("id", 0, "Tag id (required)")
	b := parseBindingFlags(fs)
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("tag.apply", err, stdout, mode)
	}
	if *id == 0 || b.kind == "" || b.ref == "" {
		return writeArgErr("tag.apply",
			errors.New("--id, --kind, --ref are required"), stdout, mode)
	}
	res, err := tags.Apply(a.Projects, *id, b.kind, b.ref, b.orgUser)
	if err != nil {
		return writeTagErr("tag.apply", err, stdout, mode)
	}
	r := headless.Success("tag.apply", b.orgUser, b.orgUser, res.Changed,
		map[string]any{
			"tag":       res.Tag,
			"item_kind": b.kind,
			"item_ref":  b.ref,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func tagRemove(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("tag remove")
	id := fs.Int64("id", 0, "Tag id (required)")
	b := parseBindingFlags(fs)
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("tag.remove", err, stdout, mode)
	}
	if *id == 0 || b.kind == "" || b.ref == "" {
		return writeArgErr("tag.remove",
			errors.New("--id, --kind, --ref are required"), stdout, mode)
	}
	res, err := tags.Remove(a.Projects, *id, b.kind, b.ref, b.orgUser)
	if err != nil {
		return writeTagErr("tag.remove", err, stdout, mode)
	}
	r := headless.Success("tag.remove", b.orgUser, b.orgUser, res.Changed,
		map[string]any{
			"tag":       res.Tag,
			"item_kind": b.kind,
			"item_ref":  b.ref,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func tagSet(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("tag set")
	ids := fs.String("ids", "", "Comma-separated tag ids (empty = clear)")
	b := parseBindingFlags(fs)
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("tag.set", err, stdout, mode)
	}
	if b.kind == "" || b.ref == "" {
		return writeArgErr("tag.set",
			errors.New("--kind, --ref are required"), stdout, mode)
	}
	parsed, err := parseInt64CSV(*ids)
	if err != nil {
		return writeArgErr("tag.set", err, stdout, mode)
	}
	res, err := tags.Set(a.Projects, b.kind, b.ref, b.orgUser, parsed)
	if err != nil {
		return writeTagErr("tag.set", err, stdout, mode)
	}
	r := headless.Success("tag.set", b.orgUser, b.orgUser, res.Changed,
		map[string]any{
			"item_kind": b.kind,
			"item_ref":  b.ref,
			"tag_ids":   parsed,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// tagItems lists every item bound to one tag (the "show me everything
// tagged X" query). Read-only.
func tagItems(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("tag items")
	id := fs.Int64("id", 0, "Tag id (required)")
	orgUser := fs.String("org-user", "",
		"Filter to one org (empty returns all orgs)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("tag.items", err, stdout, mode)
	}
	if *id == 0 {
		return writeArgErr("tag.items", errors.New("--id is required"), stdout, mode)
	}
	rows, err := tags.ItemsWithTag(a.Projects, *id, *orgUser)
	if err != nil {
		return writeTagErr("tag.items", err, stdout, mode)
	}
	r := headless.Success("tag.items", *orgUser, *orgUser, false,
		map[string]any{
			"tag_id": *id,
			"items":  rows,
			"count":  len(rows),
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// tagOf is the inverse: list every tag bound to one item.
func tagOf(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("tag of")
	b := parseBindingFlags(fs)
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("tag.of", err, stdout, mode)
	}
	if b.kind == "" || b.ref == "" {
		return writeArgErr("tag.of",
			errors.New("--kind and --ref are required"), stdout, mode)
	}
	rows, err := tags.TagsFor(a.Projects, b.kind, b.ref, b.orgUser)
	if err != nil {
		return writeTagErr("tag.of", err, stdout, mode)
	}
	r := headless.Success("tag.of", b.orgUser, b.orgUser, false,
		map[string]any{
			"item_kind": b.kind,
			"item_ref":  b.ref,
			"tags":      rows,
			"count":     len(rows),
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// writeTagErr maps service errors to typed headless codes. Mirrors
// chip.go's writeChipErr — same three categories (not_found,
// already_exists → invalid_argument, validation → invalid_argument).
func writeTagErr(command string, err error, stdout io.Writer, mode headless.WriteMode) int {
	var notFound tags.ErrNotFound
	if errors.As(err, &notFound) {
		details := map[string]any{}
		if notFound.ID != 0 {
			details["id"] = notFound.ID
		}
		if notFound.Name != "" {
			details["name"] = notFound.Name
		}
		r := headless.Fail(command, "", headless.ErrNotFound, err.Error(), details)
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	var dup tags.ErrAlreadyExists
	if errors.As(err, &dup) {
		r := headless.Fail(command, "", headless.ErrInvalidArgument, err.Error(),
			map[string]any{"name": dup.Name})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	var badKind tags.ErrInvalidKind
	if errors.As(err, &badKind) {
		r := headless.Fail(command, "", headless.ErrInvalidArgument, err.Error(),
			map[string]any{"kind": badKind.Kind})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	return writeArgErr(command, err, stdout, mode)
}

func parseInt64CSV(s string) ([]int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid id %q in --ids: %w", p, err)
		}
		out = append(out, v)
	}
	return out, nil
}
