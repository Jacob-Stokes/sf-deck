package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/services/bundles"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// jsonUnmarshal aliases encoding/json.Unmarshal so the file-local
// parse helpers don't pull the package name into every line.
var jsonUnmarshal = json.Unmarshal

func dispatchBundle(a *app.App, args Args, stdout io.Writer, mode headless.WriteMode) int {
	verb := args.Verb
	if verb == "" {
		verb = "list"
	}
	if a.Projects == nil {
		r := headless.Fail("bundle."+verb, "", headless.ErrInternal,
			"devprojects store unavailable", nil)
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	switch verb {
	case "list":
		return bundleList(a, args.Rest, stdout, mode)
	case "show":
		return bundleShow(a, args.Rest, stdout, mode)
	case "create":
		return bundleCreate(a, args.Rest, stdout, mode)
	case "link":
		return bundleLink(a, args.Rest, stdout, mode)
	case "retrieve":
		return bundleRetrieve(a, args.Rest, stdout, mode)
	case "deploy":
		return bundleDeploy(a, args.Rest, stdout, mode)
	case "validate":
		return bundleValidate(a, args.Rest, stdout, mode)
	case "report":
		return bundleReport(a, args.Rest, stdout, mode)
	case "delete":
		return bundleDelete(a, args.Rest, stdout, mode)
	}
	r := headless.Fail("bundle."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown bundle verb %q", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func bundleList(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("bundle list")
	projectID := fs.String("project-id", "", "Limit to bundles linked to this dev project")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("bundle.list", err, stdout, mode)
	}
	list, err := bundles.List(a.Projects, *projectID)
	if err != nil {
		return writeBundleErr("bundle.list", "", err, stdout, mode)
	}
	r := headless.Success("bundle.list", "", "", false, map[string]any{
		"bundles": list,
		"count":   len(list),
	})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func bundleShow(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("bundle show")
	id := fs.String("id", "", "Bundle id (required)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("bundle.show", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("bundle.show",
			errors.New("--id is required"), stdout, mode)
	}
	b, err := bundles.Show(a.Projects, *id)
	if err != nil {
		return writeBundleErr("bundle.show", "", err, stdout, mode)
	}
	r := headless.Success("bundle.show", "", "", false,
		map[string]any{"bundle": b})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func bundleCreate(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("bundle create")
	projectID := fs.String("project-id", "", "Dev project id (required)")
	path := fs.String("path", "", "Bundle directory (default ~/sf-deck-bundles/<project>-<ts>/)")
	target := fs.String("org", "", "Origin org alias for retrieve + default-org marker")
	full := fs.Bool("full-project", true,
		"Scaffold sfdx-project.json + force-app/ so retrieve can run cleanly")
	retrieve := fs.Bool("retrieve", true,
		"Run `sf project retrieve start` after writing package.xml")
	allOrgs := fs.Bool("all-orgs", false,
		"Include items from every org (default: only the active --org)")
	force := fs.Bool("force", false,
		"Write into a non-empty directory (default: refuse, to protect existing projects)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("bundle.create", err, stdout, mode)
	}
	if *projectID == "" {
		return writeArgErr("bundle.create",
			errors.New("--project-id is required"), stdout, mode)
	}
	in := bundles.CreateInput{
		ProjectID:    *projectID,
		Path:         *path,
		OrgAlias:     "",
		FullProject:  *full,
		Retrieve:     false,
		ScopeAllOrgs: *allOrgs,
		Force:        *force,
	}
	orgUser := ""
	if *target != "" {
		o, err := a.ResolveOrg(*target)
		if err != nil {
			return writeOrgErr("bundle.create", *target, err, stdout, mode)
		}
		in.OrgAlias = app.TargetArg(o)
		in.OrgUser = o.Username
		orgUser = o.Username
	}
	res, err := bundles.Create(a.Projects, in)
	if err != nil {
		return writeBundleErr("bundle.create", orgUser, err, stdout, mode)
	}
	if *retrieve {
		retrieved, retrieveErr := a.BundleWrites().Retrieve(context.Background(), bundles.OperationInput{
			BundleID: res.Bundle.ID, Target: in.OrgAlias,
		})
		res.RetrieveOutput = retrieved.Output
		res.RetrieveErr = retrieveErr
	}
	data := map[string]any{
		"bundle":           res.Bundle,
		"package_xml_path": res.PackageXMLPath,
		"included":         res.Included,
		"records_exported": res.RecordsExported,
	}
	if len(res.UnsupportedKinds) > 0 {
		data["unsupported_kinds"] = res.UnsupportedKinds
	}
	if len(res.ManagedSkipped) > 0 {
		data["managed_skipped"] = res.ManagedSkipped
	}
	exitCode := 0
	changed := true
	if res.RetrieveErr != nil {
		data["retrieve_error"] = res.RetrieveErr.Error()
		// Partial success: the bundle row + manifest are written, but
		// the retrieve failed. Map to ExitPartial so scripts can
		// branch cleanly.
		exitCode = headless.ExitPartialSuccess
	}
	r := headless.Success("bundle.create", orgUser, in.OrgAlias, changed, data)
	_ = r.Write(stdout, mode)
	if exitCode != 0 {
		return exitCode
	}
	return headless.ExitCodeFor(r)
}

func bundleLink(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("bundle link")
	projectID := fs.String("project-id", "", "Dev project id (required)")
	path := fs.String("path", "", "Existing sfdx-project directory (required)")
	target := fs.String("org", "", "Default org alias for the bundle (optional)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("bundle.link", err, stdout, mode)
	}
	if *projectID == "" || *path == "" {
		return writeArgErr("bundle.link",
			errors.New("--project-id and --path are required"), stdout, mode)
	}
	alias := ""
	orgUser := ""
	if *target != "" {
		o, err := a.ResolveOrg(*target)
		if err != nil {
			return writeOrgErr("bundle.link", *target, err, stdout, mode)
		}
		alias = app.TargetArg(o)
		orgUser = o.Username
	}
	b, err := bundles.Link(a.Projects, *projectID, *path, alias)
	if err != nil {
		return writeBundleErr("bundle.link", orgUser, err, stdout, mode)
	}
	r := headless.Success("bundle.link", orgUser, alias, true, map[string]any{
		"bundle": b,
	})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func bundleRetrieve(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("bundle retrieve")
	id := fs.String("id", "", "Bundle id (required)")
	target := fs.String("org", "",
		"Target org alias (defaults to bundle's recorded origin)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("bundle.retrieve", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("bundle.retrieve",
			errors.New("--id is required"), stdout, mode)
	}
	result, err := a.BundleWrites().Retrieve(context.Background(), bundles.OperationInput{
		BundleID: *id, Target: *target,
	})
	if err != nil {
		return writeBundleOperationErr("bundle.retrieve", *target, result.Target, err, stdout, mode)
	}
	r := headless.Success("bundle.retrieve", result.Target.Username, result.Target.CLIArg, true, map[string]any{
		"bundle_id": *id,
		"sf_output": string(result.Output),
	})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func bundleDeploy(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("bundle deploy")
	id := fs.String("id", "", "Bundle id (required)")
	target := fs.String("org", "",
		"Target org alias (defaults to bundle's recorded origin)")
	async := fs.Bool("async", false,
		"Fire and return immediately with the DeployRequest.Id — poll via bundle.report")
	tests := fs.String("tests", "",
		"Test level: NoTestRun (sandbox-only), RunSpecifiedTests, RunLocalTests, RunAllTestsInOrg")
	testClasses := fs.String("test-classes", "",
		"Comma-separated Apex test class names (required when --tests=RunSpecifiedTests)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("bundle.deploy", err, stdout, mode)
	}
	opts, err := buildDeployOpts(*tests, *testClasses)
	if err != nil {
		return writeArgErr("bundle.deploy", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("bundle.deploy",
			errors.New("--id is required"), stdout, mode)
	}
	var result bundles.OperationResult
	if *async {
		result, err = a.BundleWrites().DeployAsync(context.Background(), bundles.OperationInput{
			BundleID: *id, Target: *target, Opts: opts,
		})
	} else {
		result, err = a.BundleWrites().Deploy(context.Background(), bundles.OperationInput{
			BundleID: *id, Target: *target, Opts: opts,
		})
	}
	if err != nil {
		return writeBundleOperationErr("bundle.deploy", *target, result.Target, err, stdout, mode)
	}
	data := map[string]any{
		"bundle_id": *id,
		"sf_output": string(result.Output),
	}
	if jobID := parseDeployJobID(result.Output); jobID != "" {
		data["deploy_id"] = jobID
	}
	r := headless.Success("bundle.deploy", result.Target.Username, result.Target.CLIArg, true, data)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func bundleValidate(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("bundle validate")
	id := fs.String("id", "", "Bundle id (required)")
	target := fs.String("org", "",
		"Target org alias (defaults to bundle's recorded origin)")
	async := fs.Bool("async", false,
		"Fire and return immediately with the DeployRequest.Id — poll via bundle.report")
	tests := fs.String("tests", "",
		"Test level: NoTestRun (sandbox-only), RunSpecifiedTests, RunLocalTests, RunAllTestsInOrg")
	testClasses := fs.String("test-classes", "",
		"Comma-separated Apex test class names (required when --tests=RunSpecifiedTests)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("bundle.validate", err, stdout, mode)
	}
	opts, err := buildDeployOpts(*tests, *testClasses)
	if err != nil {
		return writeArgErr("bundle.validate", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("bundle.validate",
			errors.New("--id is required"), stdout, mode)
	}
	var result bundles.OperationResult
	if *async {
		result, err = a.BundleWrites().ValidateAsync(context.Background(), bundles.OperationInput{
			BundleID: *id, Target: *target, Opts: opts,
		})
	} else {
		result, err = a.BundleWrites().Validate(context.Background(), bundles.OperationInput{
			BundleID: *id, Target: *target, Opts: opts,
		})
	}
	if err != nil {
		return writeBundleOperationErr("bundle.validate", *target, result.Target, err, stdout, mode)
	}
	data := map[string]any{
		"bundle_id": *id,
		"sf_output": string(result.Output),
	}
	if jobID := parseDeployJobID(result.Output); jobID != "" {
		data["deploy_id"] = jobID
	}
	r := headless.Success("bundle.validate", result.Target.Username, result.Target.CLIArg, false, data)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func bundleDelete(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("bundle delete")
	id := fs.String("id", "", "Bundle id (required)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("bundle.delete", err, stdout, mode)
	}
	if *id == "" {
		return writeArgErr("bundle.delete",
			errors.New("--id is required"), stdout, mode)
	}
	if err := bundles.Delete(a.Projects, *id); err != nil {
		return writeBundleErr("bundle.delete", "", err, stdout, mode)
	}
	r := headless.Success("bundle.delete", "", "", true, map[string]any{
		"bundle_id": *id,
		"note":      "row removed; on-disk directory left in place",
	})
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func bundleReport(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("bundle report")
	id := fs.String("id", "", "Bundle id (required — sf needs a project dir to anchor the command)")
	jobID := fs.String("deploy-id", "", "DeployRequest.Id from a prior async validate/deploy")
	target := fs.String("org", "",
		"Target org alias (defaults to bundle's recorded origin)")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("bundle.report", err, stdout, mode)
	}
	if *id == "" || *jobID == "" {
		return writeArgErr("bundle.report",
			errors.New("--id and --deploy-id are required"), stdout, mode)
	}
	result, err := a.BundleWrites().Report(context.Background(), bundles.ReportInput{
		BundleID: *id, Target: *target, JobID: *jobID,
	})
	if err != nil {
		return writeBundleOperationErr("bundle.report", *target, result.Target, err, stdout, mode)
	}
	data := map[string]any{
		"bundle_id": *id,
		"deploy_id": *jobID,
		"sf_output": string(result.Output),
	}
	// Surface the terminal status fields so consumers don't have to
	// parse sf_output. Status is the lifecycle field;
	// numberComponentErrors / numberTestErrors are the "did it fail"
	// signals validate cares about.
	if status, ok := parseDeployStatus(result.Output); ok {
		data["status"] = status
	}
	r := headless.Success("bundle.report", result.Target.Username, result.Target.CLIArg, false, data)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

// buildDeployOpts translates the --tests / --test-classes CLI flags
// into the sf.DeployOpts the bundle service consumes.
//
// Accepts the four Salesforce test levels (case-insensitive). Empty
// --tests is "let Salesforce decide" — the platform defaults to
// RunLocalTests for prod, NoTestRun for sandbox unless the
// --tests-level setting overrides. --test-classes is a CSV that
// becomes one --tests flag per class on the sf shell-out.
//
// Returns an error for typos / mismatches so the caller doesn't
// silently send an unexpected level to Salesforce.
func buildDeployOpts(testsFlag, testClasses string) (sf.DeployOpts, error) {
	level := strings.TrimSpace(testsFlag)
	if level == "" {
		if strings.TrimSpace(testClasses) != "" {
			return sf.DeployOpts{}, errors.New(
				"--test-classes requires --tests=RunSpecifiedTests")
		}
		return sf.DeployOpts{}, nil
	}
	normalized := sf.DeployTestLevel("")
	switch strings.ToLower(level) {
	case "notestrun", "no-test-run":
		normalized = sf.TestLevelNoTestRun
	case "runspecifiedtests", "run-specified", "specified":
		normalized = sf.TestLevelRunSpecified
	case "runlocaltests", "run-local", "local":
		normalized = sf.TestLevelRunLocalTests
	case "runalltestsintorg", "runalltestsinorg", "run-all", "all":
		normalized = sf.TestLevelRunAllTestsInOrg
	default:
		return sf.DeployOpts{}, fmt.Errorf(
			"unknown --tests value %q (expected NoTestRun / RunSpecifiedTests / RunLocalTests / RunAllTestsInOrg)",
			level)
	}
	opts := sf.DeployOpts{TestLevel: normalized}
	classes := strings.TrimSpace(testClasses)
	if normalized == sf.TestLevelRunSpecified {
		if classes == "" {
			return sf.DeployOpts{}, errors.New(
				"--tests=RunSpecifiedTests requires --test-classes")
		}
		for _, c := range strings.Split(classes, ",") {
			if c = strings.TrimSpace(c); c != "" {
				opts.TestClasses = append(opts.TestClasses, c)
			}
		}
	} else if classes != "" {
		return sf.DeployOpts{}, errors.New(
			"--test-classes only meaningful with --tests=RunSpecifiedTests")
	}
	return opts, nil
}

// parseDeployJobID extracts the DeployRequest.Id (0Af...) from the sf
// project deploy --json output. Both sync and async modes include
// "id" in the result block; we surface it so a synchronous deploy
// that times out client-side still tells the agent which job to
// poll.
func parseDeployJobID(out []byte) string {
	var env struct {
		Result struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := jsonUnmarshalLenient(out, &env); err != nil {
		return ""
	}
	return env.Result.ID
}

// parseDeployStatus extracts the lifecycle Status (Pending /
// InProgress / Succeeded / Failed / Canceled) plus the test/component
// error counts. Returns ok=false when the JSON didn't parse —
// caller fills in nothing rather than rendering misleading zeros.
func parseDeployStatus(out []byte) (map[string]any, bool) {
	var env struct {
		Result struct {
			Status                string `json:"status"`
			Done                  bool   `json:"done"`
			Success               bool   `json:"success"`
			NumberComponentsTotal int    `json:"numberComponentsTotal"`
			NumberComponentErrors int    `json:"numberComponentErrors"`
			NumberTestErrors      int    `json:"numberTestErrors"`
			CheckOnly             bool   `json:"checkOnly"`
		} `json:"result"`
	}
	if err := jsonUnmarshalLenient(out, &env); err != nil {
		return nil, false
	}
	return map[string]any{
		"status":                  env.Result.Status,
		"done":                    env.Result.Done,
		"success":                 env.Result.Success,
		"number_components_total": env.Result.NumberComponentsTotal,
		"number_component_errors": env.Result.NumberComponentErrors,
		"number_test_errors":      env.Result.NumberTestErrors,
		"check_only":              env.Result.CheckOnly,
	}, true
}

// jsonUnmarshalLenient is encoding/json.Unmarshal with the empty-
// input nil-check — sf can emit stderr-only on failure, in which
// case stdout is empty.
func jsonUnmarshalLenient(out []byte, v any) error {
	if len(out) == 0 {
		return errors.New("empty output")
	}
	return jsonUnmarshal(out, v)
}

// writeBundleErr translates bundle service errors → typed headless
// envelope codes. Bundle-specific (not_found / stale) get their own
// codes; everything else falls through to the generic argerr path.
func writeBundleErr(command, orgUser string, err error, stdout io.Writer, mode headless.WriteMode) int {
	var notFound bundles.ErrNotFound
	if errors.As(err, &notFound) {
		r := headless.Fail(command, orgUser, headless.ErrNotFound, err.Error(),
			map[string]any{"id": notFound.ID})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	var stale bundles.ErrStale
	if errors.As(err, &stale) {
		r := headless.Fail(command, orgUser, headless.ErrInvalidArgument,
			err.Error(),
			map[string]any{"id": stale.ID, "path": stale.Path,
				"hint": "the bundle dir was moved/deleted — re-create the bundle or delete the row"})
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	return writeArgErr(command, err, stdout, mode)
}

func writeBundleOperationErr(command, requestedTarget string, target orgwrite.Target,
	err error, stdout io.Writer, mode headless.WriteMode) int {
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
	return writeBundleErr(command, orgUser, err, stdout, mode)
}
