package sf

// `sf project retrieve` / `sf project deploy` shell-outs for the
// in-TUI bundle workflow.
//
// All commands here run synchronously and require being executed from
// inside a directory that looks like a Salesforce DX project
// (sfdx-project.json present). Callers pass the bundle dir as
// bundleDir; the commands cd into that directory before exec'ing sf.
//
// The retrieve / deploy commands take 30-120s for medium projects;
// callers run them from a goroutine via tea.Cmd. Preview commands
// are fast (typically < 5s) but still off the UI thread for
// consistency.
//
// Source tracking caveat: the *preview* commands (RetrievePreview,
// DeployPreview) require source tracking to be enabled on the target
// org. Production orgs and most non-default sandboxes return
// NonSourceTrackedOrgError. Callers should fall back to a plain
// "show what's in the manifest" listing in that case.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ProjectSFTimeout caps long-running project retrieve/deploy shell-outs.
// These are expected to take longer than ordinary metadata reads, but
// should still eventually return control to the TUI if the sf process hangs.
const ProjectSFTimeout = 5 * time.Minute

// RetrieveProject runs `sf project retrieve start --manifest package.xml
// --target-org <alias>` from inside `bundleDir`. The bundle directory
// must already contain sfdx-project.json + package.xml + a force-app/
// skeleton (the FormatSfdxProjectRetrieve writer makes all three
// before this function is called).
//
// Returns the raw stdout (sf JSON output) on success, or an error
// built the same way as every other runSF call (parses typed errors
// from sf's JSON envelope when present).
func RetrieveProject(bundleDir, orgAlias string) ([]byte, error) {
	if bundleDir == "" || orgAlias == "" {
		return nil, fmt.Errorf("retrieve: bundle dir and org required")
	}
	return runSFInDir(bundleDir, orgAlias,
		"project", "retrieve", "start",
		"--manifest", "package.xml",
		"--target-org", orgAlias,
		"--json",
	)
}

// DeployProject runs `sf project deploy start --manifest package.xml`
// from inside `bundleDir`. Pushes whatever's in the bundle's
// force-app/ to the target org. Synchronous; caller runs from a
// goroutine.
//
// Returns the raw stdout (sf JSON output, includes deploy id +
// component results) on success.
func DeployProject(bundleDir, orgAlias string, opts DeployOpts) ([]byte, error) {
	if bundleDir == "" || orgAlias == "" {
		return nil, fmt.Errorf("deploy: bundle dir and org required")
	}
	args := []string{
		"project", "deploy", "start",
		"--manifest", "package.xml",
		"--target-org", orgAlias,
		"--json",
	}
	args = append(args, opts.toFlags()...)
	return runSFInDir(bundleDir, orgAlias, args...)
}

// DeployTestLevel mirrors Salesforce's testLevel enum. Empty value
// means "let Salesforce choose the default" — RunSpecifiedTests
// without classes is rejected by the platform; NoTestRun is
// sandbox-only; the two run-all variants drive the Apex suite.
type DeployTestLevel string

const (
	TestLevelDefault          DeployTestLevel = ""
	TestLevelNoTestRun        DeployTestLevel = "NoTestRun"
	TestLevelRunSpecified     DeployTestLevel = "RunSpecifiedTests"
	TestLevelRunLocalTests    DeployTestLevel = "RunLocalTests"
	TestLevelRunAllTestsInOrg DeployTestLevel = "RunAllTestsInOrg"
)

// DeployOpts collects the optional knobs the validate/deploy shell-
// outs share. Test level + class list let agents skip the broken
// Apex suite when validating against a sandbox; the other fields
// stay zero-value in most calls.
type DeployOpts struct {
	TestLevel   DeployTestLevel
	TestClasses []string // only honored when TestLevel == RunSpecifiedTests
}

func (o DeployOpts) toFlags() []string {
	if o.TestLevel == TestLevelDefault {
		return nil
	}
	flags := []string{"--test-level", string(o.TestLevel)}
	if o.TestLevel == TestLevelRunSpecified {
		for _, t := range o.TestClasses {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			flags = append(flags, "--tests", t)
		}
	}
	return flags
}

// ValidateDeployProject runs `sf project deploy validate --manifest
// package.xml` — a server-side check that runs validation rules
// (and Apex tests if applicable) without actually committing the
// changes. Same shape as DeployProject but with --validate.
//
// Useful for CI-style "is this deployable" checks before the user
// commits to an actual deploy.
func ValidateDeployProject(bundleDir, orgAlias string, opts DeployOpts) ([]byte, error) {
	if bundleDir == "" || orgAlias == "" {
		return nil, fmt.Errorf("validate: bundle dir and org required")
	}
	args := []string{
		"project", "deploy", "validate",
		"--manifest", "package.xml",
		"--target-org", orgAlias,
		"--json",
	}
	args = append(args, opts.toFlags()...)
	return runSFInDir(bundleDir, orgAlias, args...)
}

// ValidateDeployProjectAsync is the --async variant. Returns
// immediately after Salesforce accepts the job — the response
// carries DeployRequest.Id which callers poll via DeployReport.
// Use this for headless agents where the synchronous timeout
// can't accommodate a 15-minute test run.
func ValidateDeployProjectAsync(bundleDir, orgAlias string, opts DeployOpts) ([]byte, error) {
	if bundleDir == "" || orgAlias == "" {
		return nil, fmt.Errorf("validate: bundle dir and org required")
	}
	args := []string{
		"project", "deploy", "validate",
		"--manifest", "package.xml",
		"--target-org", orgAlias,
		"--async",
		"--json",
	}
	args = append(args, opts.toFlags()...)
	return runSFInDir(bundleDir, orgAlias, args...)
}

// DeployProjectAsync is the --async variant of DeployProject. Same
// shape; sf returns the DeployRequest.Id immediately so callers
// poll via DeployReport.
func DeployProjectAsync(bundleDir, orgAlias string, opts DeployOpts) ([]byte, error) {
	if bundleDir == "" || orgAlias == "" {
		return nil, fmt.Errorf("deploy: bundle dir and org required")
	}
	args := []string{
		"project", "deploy", "start",
		"--manifest", "package.xml",
		"--target-org", orgAlias,
		"--async",
		"--json",
	}
	args = append(args, opts.toFlags()...)
	return runSFInDir(bundleDir, orgAlias, args...)
}

// DeployReport runs `sf project deploy report --job-id <id>` to
// check the status of an async deploy / validate. Returns the raw
// JSON envelope from sf, which carries Status (Pending / InProgress
// / Succeeded / Failed / Canceled), component results, and any
// test failures. Caller parses what they care about.
//
// bundleDir is required because sf needs to run inside a project
// directory; the id itself isn't tied to the bundle but sf's CLI
// command shape demands the dir context.
func DeployReport(bundleDir, orgAlias, jobID string) ([]byte, error) {
	if bundleDir == "" || orgAlias == "" || jobID == "" {
		return nil, fmt.Errorf("deploy.report: bundle dir, org, and job id required")
	}
	return runSFInDir(bundleDir, orgAlias,
		"project", "deploy", "report",
		"--job-id", jobID,
		"--target-org", orgAlias,
		"--json",
	)
}

// ManifestPreviewItem is one row from `sf project retrieve preview`
// or `sf project deploy preview`. Both commands return the same
// shape; the consumer interprets the lists differently based on
// which command produced the output.
//
// Field names mirror the sfdx --json schema. Path is relative to
// the bundle dir; "" when the item exists only in the org / only
// in the manifest.
type ManifestPreviewItem struct {
	FullName string `json:"fullName"`
	Type     string `json:"type"`
	Path     string `json:"path,omitempty"`
	// Namespace is the managed-package prefix when the item belongs
	// to a managed package. Set by the fallback diff (which queries
	// NamespacePrefix from Tooling); always "" for `sf project
	// retrieve preview` results since those are about your project,
	// not the org's package inventory. Renderer uses this to badge
	// managed items + bucket them separately.
	Namespace string `json:"namespace,omitempty"`
}

// ManifestPreview is the parsed form of `sf project retrieve preview
// --json` (or the deploy variant). NonSourceTracked is true when sf
// returned a NonSourceTrackedOrgError — caller should fall back to
// the plain manifest listing in that case.
type ManifestPreview struct {
	NonSourceTracked bool                  `json:"-"`
	ToRetrieve       []ManifestPreviewItem `json:"toRetrieve"`
	ToDelete         []ManifestPreviewItem `json:"toDelete"`
	ToDeploy         []ManifestPreviewItem `json:"toDeploy"`
	Conflicts        []ManifestPreviewItem `json:"conflicts"`
	Ignored          []ManifestPreviewItem `json:"ignored"`
}

// RetrievePreview runs `sf project retrieve preview` inside bundleDir
// and parses the JSON. Empty preview (everything in sync) returns a
// ManifestPreview with all-empty slices.
//
// On NonSourceTrackedOrgError returns a ManifestPreview with
// NonSourceTracked=true and no error — that's an expected workflow
// branch the UI handles, not a failure.
func RetrievePreview(bundleDir, orgAlias string) (ManifestPreview, error) {
	out, err := runSFInDir(bundleDir, orgAlias,
		"project", "retrieve", "preview",
		"--target-org", orgAlias,
		"--json",
	)
	return parsePreviewOutput(out, err)
}

// DeployPreview is RetrievePreview's sibling — what would be deployed
// if you ran `sf project deploy start` against this bundle now.
func DeployPreview(bundleDir, orgAlias string) (ManifestPreview, error) {
	out, err := runSFInDir(bundleDir, orgAlias,
		"project", "deploy", "preview",
		"--manifest", "package.xml",
		"--target-org", orgAlias,
		"--json",
	)
	return parsePreviewOutput(out, err)
}

// parsePreviewOutput unpacks the --json envelope from
// retrieve/deploy preview. The shape is:
//
//	{
//	  "status": 0,
//	  "result": {
//	    "toRetrieve":   [...],
//	    "toDelete":     [...],
//	    "conflicts":    [...],
//	    "ignored":      [...]
//	  }
//	}
//
// On NonSourceTrackedOrgError the call already errored; we detect
// the named SFError + return a sentinel preview rather than
// propagating the failure.
func parsePreviewOutput(out []byte, err error) (ManifestPreview, error) {
	if err != nil {
		if isNonSourceTracked(err) {
			return ManifestPreview{NonSourceTracked: true}, nil
		}
		return ManifestPreview{}, err
	}
	var env struct {
		Result ManifestPreview `json:"result"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		return ManifestPreview{}, fmt.Errorf("parse preview json: %w", err)
	}
	return env.Result, nil
}

// isNonSourceTracked reports whether err is the typed SFError that
// preview commands return when the target org doesn't have source
// tracking enabled.
func isNonSourceTracked(err error) bool {
	var se *SFError
	if !errors.As(err, &se) {
		return false
	}
	return se.Code == "NonSourceTrackedOrgError"
}

// runSFInDir is runSF with cmd.Dir set. Used by every project-level
// sf invocation since they all need to be run inside an sfdx project
// directory.
func runSFInDir(bundleDir, orgAlias string, args ...string) ([]byte, error) {
	return runSFInDirWithTimeout(bundleDir, orgAlias, cfgRetrieveTimeout(), args...)
}

func runSFInDirWithTimeout(bundleDir, orgAlias string, timeout time.Duration, args ...string) ([]byte, error) {
	if DemoMode {
		return nil, errors.New("demo mode: sf CLI calls are disabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := newSFCommand(ctx, args...)
	cmd.Dir = bundleDir
	start := time.Now()
	out, err := cmd.Output()
	fireOnCall(orgAlias, args, err, time.Since(start))
	if err == nil {
		return out, nil
	}
	if ctxErr := ctx.Err(); errors.Is(ctxErr, context.DeadlineExceeded) {
		return nil, fmt.Errorf("sf %s timed out after %s", argsLabel(args), timeout)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil, fmt.Errorf("sf %s cancelled", argsLabel(args))
	}
	if typed := parseCLIError(out); typed != nil {
		return nil, typed
	}
	if msg := parseStructuredError(out); msg != "" {
		return nil, fmt.Errorf("%s", msg)
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return nil, fmt.Errorf("%s", cleanStderr(string(ee.Stderr)))
	}
	return nil, err
}
