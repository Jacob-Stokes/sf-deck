package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/exporters"
	exsoql "github.com/Jacob-Stokes/sf-deck/internal/exporters/soql"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/securefile"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// dispatchSOQL routes `sf-deck soql <verb>`. Read-only in Phase 2 —
// no `--export` flag yet, no Bulk API, no saved-query CRUD (those are
// Phase 3 + their own service package).
func dispatchSOQL(a *app.App, args Args, stdout io.Writer, mode headless.WriteMode) int {
	verb := args.Verb
	if verb == "" {
		verb = "run"
	}
	switch verb {
	case "run":
		return soqlRun(a, args.Rest, stdout, mode)
	case "export":
		return soqlExport(a, args.Rest, stdout, mode)
	case "saved":
		return soqlSaved(a, args.Rest, stdout, mode)
	}
	r := headless.Fail("soql."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown soql verb %q", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// soqlRun executes a single SOQL against the resolved org. Returns
// the records + totalSize in the standard envelope. The actual query
// goes through internal/sf.Query which prefers the REST client and
// falls back to shelling out to `sf data query`.
//
// Read-only — no safety check needed. Bigger reads (Bulk export,
// streaming to file) are Phase 3.
func soqlRun(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("soql run")
	target := fs.String("org", "", "Alias or username (empty = default)")
	query := fs.String("query", "",
		"SOQL string (required, or use --query-file)")
	queryFile := fs.String("query-file", "",
		"Path to a file containing the SOQL (use '-' for stdin)")
	tooling := fs.Bool("tooling", false,
		"Use the Tooling API (for ApexClass, Flow, etc.)")
	limit := fs.Int("limit", 0,
		"Client-side row cap. 0 = no cap. SOQL's own LIMIT still applies.")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("soql.run", err, stdout, mode)
	}

	soql, err := resolveSOQL(*query, *queryFile)
	if err != nil {
		return writeArgErr("soql.run", err, stdout, mode)
	}
	if strings.TrimSpace(soql) == "" {
		return writeArgErr("soql.run",
			errors.New("--query or --query-file is required"), stdout, mode)
	}

	o, err := a.ResolveOrg(*target)
	if err != nil {
		return writeOrgErr("soql.run", *target, err, stdout, mode)
	}

	var (
		result sf.QueryResult
		qerr   error
	)
	if *limit > 0 {
		result, qerr = sf.QueryCapped(app.TargetArg(o), soql, *tooling, *limit)
	} else {
		result, qerr = sf.Query(app.TargetArg(o), soql, *tooling)
	}
	if qerr != nil {
		return writeSOQLErr("soql.run", o.Username, qerr, stdout, mode)
	}

	// truncated reports whether the client cap kicked in. The server
	// always pages; SF returns totalSize as the full count even when
	// it streamed only a partial first page, so cap < TotalSize is the
	// right signal.
	truncated := *limit > 0 && len(result.Records) < result.TotalSize
	r := headless.Success("soql.run", o.Username, app.TargetArg(o), false,
		map[string]any{
			"records":    result.Records,
			"total_size": result.TotalSize,
			"returned":   len(result.Records),
			"done":       result.Done,
			"tooling":    *tooling,
			"truncated":  truncated,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// soqlExport runs a SOQL and writes the result to a file. Read-only
// (no safety gate). Format inferred from --output extension unless
// --format is set explicitly. --bulk routes through Bulk API 2.0 for
// large result sets — same result shape, slower for small queries
// but no 2000-row page limit so it scales.
func soqlExport(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("soql export")
	target := fs.String("org", "", "Alias or username (empty = default)")
	query := fs.String("query", "", "SOQL string")
	queryFile := fs.String("query-file", "", "Path to file with SOQL ('-' for stdin)")
	tooling := fs.Bool("tooling", false, "Use Tooling API")
	output := fs.String("output", "",
		"Output file path (required). Format inferred from extension.")
	formatRaw := fs.String("format", "",
		"Override format: csv | xlsx | json. Inferred from --output extension otherwise.")
	bulk := fs.Bool("bulk", false,
		"Use Bulk API 2.0 (slower for small queries, scales to millions of rows)")
	force := fs.Bool("force", false, "Overwrite an existing output file")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("soql.export", err, stdout, mode)
	}
	soql, err := resolveSOQL(*query, *queryFile)
	if err != nil {
		return writeArgErr("soql.export", err, stdout, mode)
	}
	if strings.TrimSpace(soql) == "" {
		return writeArgErr("soql.export",
			errors.New("--query or --query-file is required"), stdout, mode)
	}
	if strings.TrimSpace(*output) == "" {
		return writeArgErr("soql.export",
			errors.New("--output is required"), stdout, mode)
	}
	if *bulk && *tooling {
		// Bulk API 2.0 doesn't support Tooling — fail fast rather
		// than letting the network round-trip return a confusing
		// error.
		return writeArgErr("soql.export",
			errors.New("--bulk and --tooling are mutually exclusive"), stdout, mode)
	}
	format, ferr := pickExportFormat(*output, *formatRaw)
	if ferr != nil {
		return writeArgErr("soql.export", ferr, stdout, mode)
	}
	o, err := a.ResolveOrg(*target)
	if err != nil {
		return writeOrgErr("soql.export", *target, err, stdout, mode)
	}

	// Fetch records. Either REST (with no client-side cap — exports
	// are explicitly "give me everything") or Bulk.
	var (
		result sf.QueryResult
		qerr   error
	)
	if *bulk {
		result, qerr = sf.BulkQueryRecords(context.Background(),
			app.TargetArg(o), soql, nil)
	} else {
		result, qerr = sf.Query(app.TargetArg(o), soql, *tooling)
	}
	if qerr != nil {
		return writeSOQLErr("soql.export", o.Username, qerr, stdout, mode)
	}

	// Shape + write. Columns slice is nil — soql.Shape will discover
	// columns from the records. Discovery preserves the SF response's
	// natural ordering plus alphabetical for extras.
	headers, rows := exsoql.Shape(result.Records, nil)
	if werr := securefile.Write(*output, *force, func(w io.Writer) error {
		return exporters.Write(w, format, headers, rows, "SOQL")
	}); werr != nil {
		return writeArgErr("soql.export",
			fmt.Errorf("write %s: %w", format, werr), stdout, mode)
	}

	abs, _ := filepath.Abs(*output)
	r := headless.Success("soql.export", o.Username, app.TargetArg(o), true,
		map[string]any{
			"output":     abs,
			"format":     string(format),
			"rows":       len(result.Records),
			"total_size": result.TotalSize,
			"columns":    headers,
			"bulk":       *bulk,
			"tooling":    *tooling,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// pickExportFormat resolves the export format. --format wins; the
// output file's extension is the fallback. Returns an error for
// formats that aren't exportable from SOQL (bundle formats like
// sfdx-project — those need a metadata source, not query rows).
func pickExportFormat(output, raw string) (exporters.Format, error) {
	if raw != "" {
		// Map the abbreviated flag values (csv|xlsx|json) to Format
		// constants. Don't go through FormatFromExtension since the
		// user typed a format, not an extension.
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "csv":
			return exporters.FormatCSV, nil
		case "xlsx", "excel":
			return exporters.FormatXLSX, nil
		case "json":
			return exporters.FormatJSON, nil
		}
		return "", fmt.Errorf("unsupported --format %q (want csv|xlsx|json)", raw)
	}
	ext := strings.TrimPrefix(filepath.Ext(output), ".")
	f, ok := exporters.FormatFromExtension(ext)
	if !ok {
		return "", fmt.Errorf("can't infer format from extension %q; pass --format", ext)
	}
	return f, nil
}

// resolveSOQL pulls the query from --query or --query-file. Special-
// cases "-" as stdin so scripts can pipe.
func resolveSOQL(inline, path string) (string, error) {
	if inline != "" && path != "" {
		return "", errors.New("--query and --query-file are mutually exclusive")
	}
	if inline != "" {
		return inline, nil
	}
	if path == "" {
		return "", nil
	}
	if path == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return string(b), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(b), nil
}

// writeSOQLErr classifies Salesforce errors. INVALID_SESSION_ID +
// network 401 surface as auth_required; SOQL parse errors as
// invalid_argument; everything else as internal_error.
func writeSOQLErr(command, orgUser string, err error, stdout io.Writer, mode headless.WriteMode) int {
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "invalid_session_id"),
		strings.Contains(low, "session expired"),
		strings.Contains(low, "401"):
		r := headless.Fail(command, orgUser, headless.ErrAuth, msg, nil)
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	case strings.Contains(low, "malformed_query"),
		strings.Contains(low, "invalid_field"),
		strings.Contains(low, "invalid_type"),
		strings.Contains(low, "unexpected token"):
		r := headless.Fail(command, orgUser, headless.ErrInvalidArgument, msg, nil)
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	r := headless.Fail(command, orgUser, headless.ErrInternal, msg, nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}
