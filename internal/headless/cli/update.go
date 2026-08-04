package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/buildinfo"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/updatecheck"
)

func dispatchUpdate(a *app.App, args Args, stdout io.Writer, mode headless.WriteMode) int {
	verb := args.Verb
	if verb == "" {
		verb = "check"
	}
	switch verb {
	case "check":
		return updateCheck(a, args.Rest, stdout, mode)
	}
	r := headless.Fail("update."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown update verb %q (expected check)", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func updateCheck(a *app.App, rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("update check")
	force := fs.Bool("force", false, "bypass the 24-hour release cache")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("update.check", err, stdout, mode)
	}
	checker := updatecheck.Service(updatecheck.New())
	if a != nil && a.Updates != nil {
		checker = a.Updates
	}
	info := buildinfo.Current()
	result, err := checker.Check(context.Background(), info.Version, updatecheck.Options{Force: *force})
	if err != nil {
		r := headless.Fail("update.check", "", headless.ErrInternal, err.Error(), nil)
		_ = r.Write(stdout, mode)
		return headless.ExitCodeFor(r)
	}
	if mode == headless.TextMode {
		writeUpdateText(stdout, result)
		return headless.ExitOK
	}
	r := headless.Success("update.check", "", "", false, result)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func writeUpdateText(w io.Writer, result updatecheck.Result) {
	switch {
	case result.DevelopmentBuild && result.LatestVersion != "":
		fmt.Fprintf(w, "development build · latest stable release is %s · %s\n",
			result.LatestVersion, result.ReleaseURL)
	case result.DevelopmentBuild:
		fmt.Fprintln(w, "development build · no stable release published")
	case result.NoStableRelease:
		fmt.Fprintf(w, "%s · no stable release published\n", result.CurrentVersion)
	case result.UpdateAvailable:
		fmt.Fprintf(w, "%s update available · %s → %s · %s\n",
			result.Kind, result.CurrentVersion, result.LatestVersion, result.ReleaseURL)
	default:
		fmt.Fprintf(w, "%s is up to date\n", result.CurrentVersion)
	}
}
