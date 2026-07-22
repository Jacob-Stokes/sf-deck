package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/services/chips"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// dispatchChip routes `sf-deck chip <verb> ...`. Each verb owns its
// own FlagSet so the help text + flag set is local to the command —
// matches how cobra-shaped CLIs feel even though we hand-roll the
// parser.
func dispatchChip(a *app.App, args Args, stdout io.Writer, mode headless.WriteMode) int {
	verb := args.Verb
	if verb == "" {
		verb = "list" // default verb — read-only, safe.
	}
	switch verb {
	case "list":
		return chipList(a, args.Rest, stdout, mode)
	case "show":
		return chipShow(a, args.Rest, stdout, mode)
	case "create":
		return chipCreate(a, args.Rest, stdout, mode)
	case "update":
		return chipUpdate(a, args.Rest, stdout, mode)
	case "delete":
		return chipDelete(a, args.Rest, stdout, mode)
	case "favourite":
		return chipFavourite(a, args.Rest, stdout, mode)
	case "columns":
		return chipColumns(a, args.Rest, stdout, mode)
	}
	r := headless.Fail("chip."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown chip verb %q", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// chipDataPath returns the settings.toml absolute path for inclusion
// in the response. Empty on lookup failure — falling back to omitting
// the field is fine since callers can derive it themselves.
func chipDataPath() string {
	p, err := settings.Path()
	if err != nil {
		return ""
	}
	return p
}

func chipList(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("chip list")
	domain := fs.String("domain", "", "Filter by domain (records|objects|flows)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("chip.list", err, stdout, mode)
	}
	list, err := chips.List(a.Settings, *domain)
	if err != nil {
		return writeArgErr("chip.list", err, stdout, mode)
	}
	r := headless.Success("chip.list", "", "", false, map[string]any{
		"chips":         list,
		"count":         len(list),
		"settings_path": chipDataPath(),
	})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func chipShow(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("chip show")
	domain := fs.String("domain", "", "Chip domain (required)")
	id := fs.String("id", "", "Chip id (required)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("chip.show", err, stdout, mode)
	}
	if *domain == "" || *id == "" {
		return writeArgErr("chip.show",
			errors.New("--domain and --id are required"), stdout, mode)
	}
	chip, err := chips.Show(a.Settings, *domain, *id)
	if err != nil {
		return writeChipErr("chip.show", err, stdout, mode)
	}
	r := headless.Success("chip.show", "", "", false, map[string]any{
		"chip":          chip,
		"settings_path": chipDataPath(),
	})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func chipCreate(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("chip create")
	id := fs.String("id", "", "Chip id (required)")
	domain := fs.String("domain", "", "Chip domain (required)")
	label := fs.String("label", "", "Display label (required)")
	scope := fs.String("scope", "", "sObject API name or *")
	favourite := fs.Bool("favourite", false, "Pin to chip strip")
	columns := fs.String("columns", "", "Comma-separated column API names")
	limit := fs.Int("limit", 0, "Query limit (0 = none)")
	clauses := fs.String("clauses", "", "Post-FROM SOQL clauses: WHERE ... ORDER BY ... LIMIT ...")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("chip.create", err, stdout, mode)
	}
	in := chips.CreateInput{
		ID:        *id,
		Domain:    *domain,
		Scope:     *scope,
		Label:     *label,
		Favourite: *favourite,
		Columns:   splitCSV(*columns),
		Limit:     *limit,
		Clauses:   *clauses,
	}
	res, err := chips.Create(a.Settings, in, a.SaveSettings)
	if err != nil {
		return writeChipErr("chip.create", err, stdout, mode)
	}
	r := headless.Success("chip.create", "", "", res.Changed, map[string]any{
		"chip":          res.Chip,
		"settings_path": chipDataPath(),
	})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func chipUpdate(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("chip update")
	id := fs.String("id", "", "Chip id (required)")
	domain := fs.String("domain", "", "Chip domain (required)")
	label := fs.String("label", "", "New label")
	scope := fs.String("scope", "", "New scope")
	limit := fs.Int("limit", -1, "New limit (-1 = no change)")
	columns := fs.String("columns", "", "New columns (comma-separated; clears when empty + --clear-columns)")
	clauses := fs.String("clauses", "", "Replace post-FROM SOQL clauses: WHERE ... ORDER BY ... LIMIT ...")
	clearColumns := fs.Bool("clear-columns", false, "Treat empty --columns as clear-all")
	favouriteRaw := fs.String("favourite", "", "true|false to update")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("chip.update", err, stdout, mode)
	}
	if *domain == "" || *id == "" {
		return writeArgErr("chip.update",
			errors.New("--domain and --id are required"), stdout, mode)
	}
	in := chips.UpdateInput{}
	// Partial-update semantics: only flags the user actually passed
	// take effect. flag.Visit walks just the touched flags.
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	if visited["label"] {
		in.Label = label
	}
	if visited["scope"] {
		in.Scope = scope
	}
	if visited["limit"] {
		in.Limit = limit
	}
	if visited["columns"] || (*clearColumns && visited["clear-columns"]) {
		c := splitCSV(*columns)
		in.Columns = &c
	}
	if visited["clauses"] {
		in.Clauses = clauses
	}
	if visited["favourite"] {
		v, err := parseBool(*favouriteRaw)
		if err != nil {
			return writeArgErr("chip.update", err, stdout, mode)
		}
		in.Favourite = &v
	}
	if !in.HasAny() {
		return writeArgErr("chip.update",
			errors.New("no update fields specified"), stdout, mode)
	}
	res, err := chips.Update(a.Settings, *domain, *id, in, a.SaveSettings)
	if err != nil {
		return writeChipErr("chip.update", err, stdout, mode)
	}
	r := headless.Success("chip.update", "", "", res.Changed, map[string]any{
		"chip":          res.Chip,
		"settings_path": chipDataPath(),
	})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func chipDelete(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("chip delete")
	domain := fs.String("domain", "", "Chip domain (required)")
	id := fs.String("id", "", "Chip id (required)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("chip.delete", err, stdout, mode)
	}
	if *domain == "" || *id == "" {
		return writeArgErr("chip.delete",
			errors.New("--domain and --id are required"), stdout, mode)
	}
	res, err := chips.Delete(a.Settings, *domain, *id, a.SaveSettings)
	if err != nil {
		return writeChipErr("chip.delete", err, stdout, mode)
	}
	r := headless.Success("chip.delete", "", "", res.Changed, map[string]any{
		"chip":          res.Chip,
		"settings_path": chipDataPath(),
	})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func chipFavourite(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("chip favourite")
	domain := fs.String("domain", "", "Chip domain (required)")
	id := fs.String("id", "", "Chip id (required)")
	on := fs.Bool("on", true, "true to favourite, false to unfavourite")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("chip.favourite", err, stdout, mode)
	}
	if *domain == "" || *id == "" {
		return writeArgErr("chip.favourite",
			errors.New("--domain and --id are required"), stdout, mode)
	}
	res, err := chips.Favourite(a.Settings, *domain, *id, *on, a.SaveSettings)
	if err != nil {
		return writeChipErr("chip.favourite", err, stdout, mode)
	}
	r := headless.Success("chip.favourite", "", "", res.Changed, map[string]any{
		"chip":          res.Chip,
		"settings_path": chipDataPath(),
	})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func chipColumns(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("chip columns")
	domain := fs.String("domain", "", "Chip domain (required)")
	scope := fs.String("scope", "", "sObject API name for records")
	target := fs.String("org", "", "Alias or username for records describe")
	contains := fs.String("contains", "", "Case-insensitive substring filter on id or label")
	customOnly := fs.Bool("custom-only", false, "Return only custom Salesforce fields")
	validate := fs.String("columns", "", "Comma-separated column IDs to validate")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("chip.columns", err, stdout, mode)
	}
	if *domain == "" {
		return writeArgErr("chip.columns", errors.New("--domain is required"), stdout, mode)
	}
	if !chips.IsValidDomain(*domain) {
		return writeArgErr("chip.columns",
			fmt.Errorf("unknown domain %q (want one of %s)", *domain, strings.Join(chips.Domains(), ", ")),
			stdout, mode)
	}

	var (
		catalog   chips.ColumnCatalog
		orgUser   string
		targetArg string
	)
	if *domain == "records" {
		if *scope == "" {
			return writeArgErr("chip.columns", errors.New("--scope is required for records"), stdout, mode)
		}
		o, err := a.ResolveOrg(*target)
		if err != nil {
			return writeOrgErr("chip.columns", *target, err, stdout, mode)
		}
		orgUser = o.Username
		targetArg = app.TargetArg(o)
		d, err := sf.Describe(targetArg, *scope)
		if err != nil {
			return writeDescribeErr("chip.columns", orgUser, *scope, err, stdout, mode)
		}
		catalog = chips.RecordColumnCatalog(d)
	} else {
		static, ok := chips.StaticColumnCatalog(*domain)
		if !ok {
			return writeArgErr("chip.columns",
				fmt.Errorf("columns are not available for domain %q", *domain), stdout, mode)
		}
		catalog = static
	}

	selected := splitCSV(*validate)
	if err := chips.ValidateColumns(catalog, selected); err != nil {
		return writeColumnErr("chip.columns", orgUser, err, stdout, mode)
	}
	filtered := chips.FilterCatalog(catalog, *contains, *customOnly)
	data := map[string]any{
		"catalog":       filtered,
		"count":         len(filtered.Columns),
		"total":         len(catalog.Columns),
		"settings_path": chipDataPath(),
	}
	if len(selected) > 0 {
		data["valid"] = true
		data["selected_columns"] = selected
	}
	r := headless.Success("chip.columns", orgUser, targetArg, false, data)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// writeChipErr maps chip service errors → typed headless errors. Each
// service-error shape gets its own headless error code so script
// consumers can branch on .error.code instead of substring-matching.
func writeChipErr(command string, err error, stdout io.Writer, mode headless.WriteMode) int {
	var notFound chips.ErrNotFound
	if errors.As(err, &notFound) {
		r := headless.Fail(command, "", headless.ErrNotFound, err.Error(),
			map[string]any{"domain": notFound.Domain, "id": notFound.ID})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	var dup chips.ErrAlreadyExists
	if errors.As(err, &dup) {
		// Already-exists is a caller mistake (used create instead of
		// update). Maps to invalid_argument so scripts exit 2.
		r := headless.Fail(command, "", headless.ErrInvalidArgument, err.Error(),
			map[string]any{"domain": dup.Domain, "id": dup.ID})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	return writeArgErr(command, err, stdout, mode)
}

func writeColumnErr(command, orgUser string, err error, stdout io.Writer, mode headless.WriteMode) int {
	var unknown chips.UnknownColumnError
	if errors.As(err, &unknown) {
		r := headless.Fail(command, orgUser, headless.ErrInvalidArgument,
			err.Error(), map[string]any{
				"domain":  unknown.Domain,
				"scope":   unknown.Scope,
				"columns": unknown.Columns,
			})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	return writeArgErr(command, err, stdout, mode)
}

func writeArgErr(command string, err error, stdout io.Writer, mode headless.WriteMode) int {
	r := headless.Fail(command, "", headless.ErrInvalidArgument, err.Error(), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "t", "1", "yes", "y":
		return true, nil
	case "false", "f", "0", "no", "n":
		return false, nil
	}
	return false, fmt.Errorf("invalid bool %q (want true|false)", s)
}
