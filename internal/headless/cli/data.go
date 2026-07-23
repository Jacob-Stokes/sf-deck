package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/instance"
)

type localDataLocation struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Kind   string `json:"kind"`
}

func dispatchData(args Args, stdout io.Writer, mode headless.WriteMode) int {
	verb := args.Verb
	if verb == "" {
		verb = "inspect"
	}
	switch verb {
	case "inspect":
		return dataInspect(args.Rest, stdout, mode)
	case "erase":
		return dataErase(args.Rest, stdout, mode)
	}
	r := headless.Fail("data."+verb, "", headless.ErrInvalidArgument,
		fmt.Sprintf("unknown data verb %q (expected inspect|erase)", verb), nil)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func localDataPaths() (appDir, bundlesDir string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(home, ".sf-deck"), filepath.Join(home, "sf-deck-bundles"), nil
}

func location(path, kind string) localDataLocation {
	_, err := os.Lstat(path)
	return localDataLocation{Path: path, Exists: err == nil, Kind: kind}
}

func dataInspect(rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("data inspect")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("data.inspect", err, stdout, mode)
	}
	appDir, bundlesDir, err := localDataPaths()
	if err != nil {
		return writeArgErr("data.inspect", err, stdout, mode)
	}
	data := map[string]any{
		"locations": []localDataLocation{
			location(appDir, "settings, metadata cache, histories, logs, and local working state"),
			location(bundlesDir, "default user-created SFDX bundle directory"),
		},
		"record_payloads_persisted": false,
		"note":                      "Exports and bundles created at custom paths remain at those user-selected paths.",
	}
	if mode == headless.TextMode {
		fmt.Fprintf(stdout, "app data: %s\nbundles:  %s\nrecord payloads persisted: no\n",
			appDir, bundlesDir)
		return headless.ExitOK
	}
	r := headless.Success("data.inspect", "", "", false, data)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}

func dataErase(rest []string, stdout io.Writer, mode headless.WriteMode) int {
	fs := newFlagSet("data erase")
	yes := fs.Bool("yes", false, "confirm deletion of sf-deck-owned local state")
	includeBundles := fs.Bool("include-bundles", false, "also delete the default ~/sf-deck-bundles directory")
	if err := fs.Parse(rest); err != nil {
		return writeArgErr("data.erase", err, stdout, mode)
	}
	if !*yes {
		return writeArgErr("data.erase", errors.New("--yes is required"), stdout, mode)
	}
	if running, err := instance.Read(); err != nil {
		return writeArgErr("data.erase", fmt.Errorf("check running instances: %w", err), stdout, mode)
	} else if len(running.Entries) > 0 {
		return writeArgErr("data.erase",
			fmt.Errorf("close %d running sf-deck instance(s) before erasing local data", len(running.Entries)),
			stdout, mode)
	}
	appDir, bundlesDir, err := localDataPaths()
	if err != nil {
		return writeArgErr("data.erase", err, stdout, mode)
	}
	targets := []localDataLocation{location(appDir, "sf-deck application data")}
	if *includeBundles {
		targets = append(targets, location(bundlesDir, "default SFDX bundles"))
	}
	changed := false
	for _, target := range targets {
		if !target.Exists {
			continue
		}
		if err := os.RemoveAll(target.Path); err != nil {
			r := headless.Fail("data.erase", "", headless.ErrInternal,
				"erase "+target.Path+": "+err.Error(), map[string]any{"path": target.Path})
			_ = r.Write(stdout, mode)
			return headless.ExitCodeFor(r)
		}
		changed = true
	}
	data := map[string]any{
		"erased": targets,
		"note":   "Salesforce CLI credentials were not changed. Use sf org logout to revoke a local org session. Custom export and bundle paths were not deleted.",
	}
	if mode == headless.TextMode {
		fmt.Fprintf(stdout, "erased sf-deck local data at %s\n", appDir)
		if *includeBundles {
			fmt.Fprintf(stdout, "erased default bundles at %s\n", bundlesDir)
		}
		fmt.Fprintln(stdout, "Salesforce CLI credentials and custom export paths were not changed.")
		return headless.ExitOK
	}
	r := headless.Success("data.erase", "", "", changed, data)
	_ = r.Write(stdout, mode)
	return headless.ExitCodeFor(r)
}
