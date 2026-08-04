package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/securefile"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// dispatchReport routes `sf-deck report <verb>`. Read-only:
//   - list   : SOQL over the Report sObject, returns id + name + folder
//   - run    : Analytics REST /reports/<id>?includeDetails=true
//   - export : same but writes the xlsx/csv body to a file
//
// Reports can't be mutated through this surface — design intent: agents
// shouldn't be editing report definitions; that's a metadata-editor
// concern (Phase 4 if at all).
func dispatchReport(a *app.App, args Args, stdout io.Writer, mode headless.WriteMode) int {
	verb := args.Verb
	if verb == "" {
		verb = "list"
	}
	switch verb {
	case "list":
		return reportList(a, args.Rest, stdout, mode)
	case "run":
		return reportRun(a, args.Rest, stdout, mode)
	case "export":
		return reportExport(a, args.Rest, stdout, mode)
	}
	r := headless.Fail("report."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown report verb %q", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func reportList(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("report list")
	target := fs.String("org", "", "Alias or username (empty = default)")
	contains := fs.String("contains", "",
		"Case-insensitive substring filter on name")
	folder := fs.String("folder", "",
		"Filter to one folder by name (case-insensitive)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("report.list", err, stdout, mode)
	}
	o, err := a.ResolveOrg(*target)
	if err != nil {
		return writeOrgErr("report.list", *target, err, stdout, mode)
	}
	all, err := sf.ListAllReports(app.TargetArg(o))
	if err != nil {
		return writeSOQLErr("report.list", o.Username, err, stdout, mode)
	}
	needle := strings.ToLower(*contains)
	folderNeedle := strings.ToLower(*folder)
	out := make([]sf.ReportSummary, 0, len(all))
	for _, rep := range all {
		if needle != "" && !strings.Contains(strings.ToLower(rep.Name), needle) {
			continue
		}
		if folderNeedle != "" &&
			!strings.EqualFold(rep.FolderName, *folder) &&
			!strings.Contains(strings.ToLower(rep.FolderName), folderNeedle) {
			continue
		}
		out = append(out, rep)
	}
	r := headless.Success("report.list", o.Username, app.TargetArg(o), false,
		map[string]any{
			"reports": out,
			"count":   len(out),
			"total":   len(all),
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func reportRun(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("report run")
	target := fs.String("org", "", "Alias or username (empty = default)")
	id := fs.String("id", "", "Report id (required, 15 or 18 chars)")
	forceRerun := fs.Bool("force-rerun", false,
		"Force SF to recompute instead of returning the cached run")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("report.run", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("report.run",
			errors.New("--id is required"), stdout, mode)
	}
	o, err := a.ResolveOrg(*target)
	if err != nil {
		return writeOrgErr("report.run", *target, err, stdout, mode)
	}
	run, err := sf.RunReport(app.TargetArg(o), *id, *forceRerun)
	if err != nil {
		return writeDescribeErr("report.run", o.Username, *id, err, stdout, mode)
	}
	r := headless.Success("report.run", o.Username, app.TargetArg(o), false,
		map[string]any{
			"report": map[string]any{
				"id":         run.ID,
				"name":       run.Name,
				"format":     run.Format,
				"columns":    run.Columns,
				"rows":       run.Rows,
				"row_count":  len(run.Rows),
				"all_data":   run.AllData,
				"cached":     run.Cached,
				"ran_at":     run.RanAt,
				"aggregates": run.Aggregates,
			},
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func reportExport(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("report export")
	target := fs.String("org", "", "Alias or username (empty = default)")
	id := fs.String("id", "", "Report id (required)")
	output := fs.String("output", "",
		"Output file path (required; xlsx is the native format)")
	view := fs.String("view", "formatted",
		"View: formatted | details. Details-only post-processed client-side.")
	force := fs.Bool("force", false, "Overwrite an existing output file")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("report.export", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("report.export",
			errors.New("--id is required"), stdout, mode)
	}
	if *output == "" {
		return writeArgErr("report.export",
			errors.New("--output is required"), stdout, mode)
	}
	switch strings.ToLower(*view) {
	case "formatted", "details":
		// ok
	default:
		return writeArgErr("report.export",
			fmt.Errorf("invalid --view %q (want formatted|details)", *view), stdout, mode)
	}
	// File format inferred from --output extension. Salesforce only
	// serves xlsx natively; csv goes through internal/postprocess
	// which is the TUI's path — not wired through to the headless
	// flow yet. Restrict to xlsx for now.
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(*output), "."))
	if ext != "xlsx" {
		return writeArgErr("report.export",
			fmt.Errorf("unsupported output extension %q; only .xlsx is supported today", ext),
			stdout, mode)
	}
	o, err := a.ResolveOrg(*target)
	if err != nil {
		return writeOrgErr("report.export", *target, err, stdout, mode)
	}
	body, err := sf.ExportReport(app.TargetArg(o), *id,
		sf.ReportExportFormat{View: strings.ToLower(*view), File: ext})
	if err != nil {
		return writeDescribeErr("report.export", o.Username, *id, err, stdout, mode)
	}
	if werr := securefile.WriteFile(*output, body, *force); werr != nil {
		return writeArgErr("report.export",
			fmt.Errorf("write output: %w", werr), stdout, mode)
	}
	abs, _ := filepath.Abs(*output)
	r := headless.Success("report.export", o.Username, app.TargetArg(o), true,
		map[string]any{
			"output": abs,
			"bytes":  len(body),
			"view":   strings.ToLower(*view),
			"format": "xlsx",
			"id":     *id,
		})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}
