package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// dispatchObject routes `sf-deck object <verb>`. Read-only — wraps
// sf.ListSObjects + sf.Describe. No cache invalidation flag yet; the
// REST client coalesces concurrent describes internally and the list
// is fetched live each time (typical N is ~500 EntityDefinition rows,
// ~2s round-trip — acceptable for ad-hoc inspection).
func dispatchObject(a *app.App, args Args, stdout io.Writer, mode headless.WriteMode) int {
	verb := args.Verb
	if verb == "" {
		verb = "list"
	}
	switch verb {
	case "list":
		return objectList(a, args.Rest, stdout, mode)
	case "show", "describe":
		return objectShow(a, args.Rest, stdout, mode)
	case "fields":
		return objectFields(a, args.Rest, stdout, mode)
	}
	r := headless.Fail("object."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown object verb %q", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func objectList(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("object list")
	target := fs.String("org", "", "Alias or username (empty = default)")
	customOnly := fs.Bool("custom-only", false,
		"Return only custom objects (ending in __c)")
	contains := fs.String("contains", "",
		"Case-insensitive substring filter on name or label")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("object.list", err, stdout, mode)
	}
	o, err := a.ResolveOrg(*target)
	if err != nil {
		return writeOrgErr("object.list", *target, err, stdout, mode)
	}
	all, err := sf.ListSObjects(app.TargetArg(o))
	if err != nil {
		return writeSOQLErr("object.list", o.Username, err, stdout, mode)
	}
	out := make([]sf.SObject, 0, len(all))
	needle := strings.ToLower(*contains)
	for _, s := range all {
		if *customOnly && !strings.HasSuffix(s.Name, "__c") {
			continue
		}
		if needle != "" &&
			!strings.Contains(strings.ToLower(s.Name), needle) &&
			!strings.Contains(strings.ToLower(s.Label), needle) {
			continue
		}
		out = append(out, s)
	}
	r := headless.Success("object.list", o.Username, app.TargetArg(o), false,
		map[string]any{
			"objects": out,
			"count":   len(out),
			"total":   len(all),
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func objectShow(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("object show")
	target := fs.String("org", "", "Alias or username (empty = default)")
	name := fs.String("name", "", "sObject API name (required)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("object.show", err, stdout, mode)
	}
	if *name == "" {
		return writeArgErr("object.show",
			errors.New("--name is required"), stdout, mode)
	}
	o, err := a.ResolveOrg(*target)
	if err != nil {
		return writeOrgErr("object.show", *target, err, stdout, mode)
	}
	d, err := sf.Describe(app.TargetArg(o), *name)
	if err != nil {
		return writeDescribeErr("object.show", o.Username, *name, err, stdout, mode)
	}
	r := headless.Success("object.show", o.Username, app.TargetArg(o), false,
		map[string]any{"object": d})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// objectFields is a convenience verb — same data as object.show, but
// projected to just the fields slice + a count. Saves agents from
// having to dig into data.object.fields[].
func objectFields(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("object fields")
	target := fs.String("org", "", "Alias or username (empty = default)")
	name := fs.String("name", "", "sObject API name (required)")
	customOnly := fs.Bool("custom-only", false,
		"Return only custom fields (ending in __c)")
	contains := fs.String("contains", "",
		"Case-insensitive substring filter on field name or label")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("object.fields", err, stdout, mode)
	}
	if *name == "" {
		return writeArgErr("object.fields",
			errors.New("--name is required"), stdout, mode)
	}
	o, err := a.ResolveOrg(*target)
	if err != nil {
		return writeOrgErr("object.fields", *target, err, stdout, mode)
	}
	d, err := sf.Describe(app.TargetArg(o), *name)
	if err != nil {
		return writeDescribeErr("object.fields", o.Username, *name, err, stdout, mode)
	}
	needle := strings.ToLower(*contains)
	out := make([]sf.Field, 0, len(d.Fields))
	for _, f := range d.Fields {
		if *customOnly && !f.Custom {
			continue
		}
		if needle != "" &&
			!strings.Contains(strings.ToLower(f.Name), needle) &&
			!strings.Contains(strings.ToLower(f.Label), needle) {
			continue
		}
		out = append(out, f)
	}
	r := headless.Success("object.fields", o.Username, app.TargetArg(o), false,
		map[string]any{
			"object": *name,
			"fields": out,
			"count":  len(out),
			"total":  len(d.Fields),
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// writeDescribeErr maps describe errors. NOT_FOUND from REST becomes
// not_found; auth errors route through the SOQL mapper.
func writeDescribeErr(command, orgUser, name string, err error, stdout io.Writer, mode headless.WriteMode) int {
	low := strings.ToLower(err.Error())
	if strings.Contains(low, "not_found") ||
		strings.Contains(low, "not found") ||
		strings.Contains(low, "404") {
		r := headless.Fail(command, orgUser, headless.ErrNotFound,
			err.Error(), map[string]any{"name": name})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	return writeSOQLErr(command, orgUser, err, stdout, mode)
}
