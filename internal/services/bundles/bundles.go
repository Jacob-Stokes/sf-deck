// Package bundles is the headless surface for sf-deck's bundle
// pipeline. Wraps the devproject store + the exporters/devproject
// package_xml writer + the sf project retrieve/deploy shell-outs
// in a thin service layer the headless CLI consumes.
//
// Why a separate package: the TUI's devproject_export.go does all
// of this through tea.Cmd async work + flash banners + user
// pickers. Headless needs the same operations done synchronously,
// returning typed errors instead of UI messages, so the headless
// envelope can map them to JSON error codes.
package bundles

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	dpexport "github.com/Jacob-Stokes/sf-deck/internal/exporters/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// Bundle is the headless wire shape — same fields as
// devproject.Bundle but exported with JSON tags + path-existence
// computed at projection time.
type Bundle struct {
	ID              string `json:"id"`
	DevProjectID    string `json:"dev_project_id"`
	Path            string `json:"path"`
	DefaultOrgAlias string `json:"default_org_alias,omitempty"`
	CreatedAt       string `json:"created_at"`
	LastRetrievedAt string `json:"last_retrieved_at,omitempty"`
	LastDeployedAt  string `json:"last_deployed_at,omitempty"`
	Stale           bool   `json:"stale,omitempty"`
}

// CreateInput is the bundle.create argument set. ProjectID is
// required; Path defaults to ~/sf-deck-bundles/<project-name>-<ts>/
// when empty; OrgAlias defaults to "" (the caller's chosen org).
//
// FullProject scaffolds sfdx-project.json + the force-app/main/default
// directory tree so a subsequent `sf project retrieve start` can
// land cleanly. The TUI's "Full sfdx project + retrieve" path
// always sets this to true; agents typically want it too.
//
// Retrieve, when true, immediately runs `sf project retrieve start`
// after writing the package.xml — same as the TUI's `RunsRetrieve`
// shortcut. Caller can disable to inspect the manifest first.
type CreateInput struct {
	ProjectID string
	Path      string
	OrgAlias  string
	// OrgUser is the canonical username — what items are stored
	// against in devprojects.db. Defaults to "" when caller doesn't
	// resolve it; the alias-only path then matches no items because
	// items are keyed by username, not alias. Headless CLI resolves
	// this from app.ResolveOrg before calling.
	OrgUser      string
	FullProject  bool
	Retrieve     bool
	ScopeAllOrgs bool // when true, include items from every org; default = this org only
	// Force allows writing into a non-empty target directory. Off by
	// default: bundle write truncates package.xml / sfdx-project.json /
	// README.md, so a mistyped --path pointing at a hand-maintained
	// sfdx project would silently clobber it. Create refuses a
	// non-empty dir unless Force is set.
	Force bool
}

// CreateResult is what bundle.create returns. RetrieveOutput is set
// only when Retrieve=true; on retrieve failure the bundle is still
// created (so the user can inspect/manually retrieve) but the error
// surfaces.
type CreateResult struct {
	Bundle           Bundle
	Included         int
	RecordsExported  int
	UnsupportedKinds []string
	ManagedSkipped   []string
	PackageXMLPath   string
	RetrieveOutput   []byte
	RetrieveErr      error
}

// List returns every bundle linked to the given DevProject,
// most-recently-touched first. ProjectID="" returns the cross-
// project list (used by the operator skill's discovery flow).
func List(store *devproject.Store, projectID string) ([]Bundle, error) {
	if store == nil {
		return nil, errors.New("nil store")
	}
	var raw []devproject.Bundle
	var err error
	if projectID == "" {
		raw, err = store.ListAllBundles()
	} else {
		raw, err = store.ListBundlesFor(projectID)
	}
	if err != nil {
		return nil, err
	}
	out := make([]Bundle, 0, len(raw))
	for _, b := range raw {
		out = append(out, toWire(b))
	}
	return out, nil
}

// Show returns one bundle by id.
func Show(store *devproject.Store, id string) (Bundle, error) {
	if store == nil {
		return Bundle{}, errors.New("nil store")
	}
	b, err := fetchBundle(store, id)
	if err != nil {
		return Bundle{}, err
	}
	return toWire(b), nil
}

// fetchBundle wraps store.GetBundle to translate sql.ErrNoRows into
// the typed ErrNotFound the service exposes. Keeps the headless
// wrapper's error mapping clean.
func fetchBundle(store *devproject.Store, id string) (devproject.Bundle, error) {
	b, err := store.GetBundle(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return devproject.Bundle{}, ErrNotFound{ID: id}
		}
		return devproject.Bundle{}, err
	}
	return b, nil
}

// Create materialises a bundle: writes package.xml (always),
// scaffolds the sfdx project skeleton (when FullProject), records
// the bundle in the store, and optionally fires the initial
// retrieve. Synchronous — returns when the retrieve completes (or
// errors). Returns CreateResult with both the bundle row and the
// manifest counts so the caller can summarise without re-reading
// package.xml.
func Create(store *devproject.Store, in CreateInput) (CreateResult, error) {
	if store == nil {
		return CreateResult{}, errors.New("nil store")
	}
	if in.ProjectID == "" {
		return CreateResult{}, errors.New("project id is required")
	}

	dp, err := store.GetDevProject(in.ProjectID)
	if err != nil {
		return CreateResult{}, err
	}
	if dp == nil {
		return CreateResult{}, ErrNotFound{ID: in.ProjectID}
	}

	// Items are keyed by canonical username, not alias. Caller
	// resolved alias→username; default to the alias as a fallback for
	// tests / callers that already pass a username here.
	orgFilter := in.OrgUser
	if orgFilter == "" {
		orgFilter = in.OrgAlias
	}
	if in.ScopeAllOrgs {
		orgFilter = ""
	}
	// versionOrg drives the manifest's <version>. It's the deploy/
	// retrieve TARGET, which is distinct from orgFilter (the item
	// scope). For an all-orgs bundle orgFilter is "" — but the bundle
	// still targets a specific org, so resolve the version from that
	// target rather than letting APIVersionForAlias("") fall back to
	// the pinned default. Empty only when the caller supplied no org at
	// all (e.g. a scope-only export), where the default is correct.
	versionOrg := in.OrgUser
	if versionOrg == "" {
		versionOrg = in.OrgAlias
	}
	items, err := store.ListItems(in.ProjectID, orgFilter)
	if err != nil {
		return CreateResult{}, err
	}
	if len(items) == 0 {
		return CreateResult{}, fmt.Errorf("project %s has no items%s",
			in.ProjectID, scopeDescription(orgFilter))
	}

	path, err := resolveBundlePath(in.Path, dp.Name)
	if err != nil {
		return CreateResult{}, err
	}
	// Refuse to write into a non-empty directory unless Force is set.
	// Bundle write truncates package.xml / sfdx-project.json / README.md,
	// so a mistyped --path aimed at a real project would destroy it. The
	// default resolveBundlePath target is a fresh timestamped dir (always
	// empty); this only bites a caller-supplied --path.
	if err := ValidateCreateDestination(path, in.Force); err != nil {
		return CreateResult{}, err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return CreateResult{}, fmt.Errorf("create bundle dir: %w", err)
	}

	manifestPath := filepath.Join(path, "package.xml")
	mf, err := os.Create(manifestPath)
	if err != nil {
		return CreateResult{}, fmt.Errorf("create package.xml: %w", err)
	}
	result, perr := dpexport.WritePackageXML(mf, items, dpexport.PackageXMLOptions{APIVersion: sf.APIVersionForAlias(versionOrg)})
	closeErr := mf.Close()
	if perr != nil {
		return CreateResult{}, fmt.Errorf("write package.xml: %w", perr)
	}
	if closeErr != nil {
		return CreateResult{}, fmt.Errorf("close package.xml: %w", closeErr)
	}
	if result.IncludedCount == 0 {
		return CreateResult{}, fmt.Errorf("no items mapped to MetadataAPI types (records / unsupported only)")
	}

	if in.FullProject {
		projectJSON := dpexport.SfdxProjectJSON(dp.Name, sf.APIVersionForAlias(versionOrg))
		if err := os.WriteFile(filepath.Join(path, "sfdx-project.json"),
			[]byte(projectJSON), 0o644); err != nil {
			return CreateResult{}, fmt.Errorf("write sfdx-project.json: %w", err)
		}
		if err := os.MkdirAll(filepath.Join(path, "force-app", "main", "default"),
			0o755); err != nil {
			return CreateResult{}, fmt.Errorf("create force-app dir: %w", err)
		}
	}

	// README — same content the TUI flow writes. Strictly cosmetic
	// but agents inspecting the bundle deserve the same explainer.
	_ = os.WriteFile(filepath.Join(path, "README.md"),
		[]byte(dpexport.SuggestedReadme(dp.Name, orgFilter, result, in.FullProject)),
		0o644)

	bundleRow, err := store.CreateBundle(in.ProjectID, path, in.OrgAlias)
	if err != nil {
		return CreateResult{}, fmt.Errorf("persist bundle row: %w", err)
	}

	cr := CreateResult{
		Bundle:           toWire(bundleRow),
		Included:         result.IncludedCount,
		RecordsExported:  len(result.Records),
		UnsupportedKinds: kindLabels(result.Unsupported),
		ManagedSkipped:   kindLabels(result.Managed),
		PackageXMLPath:   manifestPath,
	}

	if in.Retrieve {
		if in.OrgAlias == "" {
			cr.RetrieveErr = fmt.Errorf("retrieve requested but no --org supplied")
			return cr, nil
		}
		out, rerr := sf.RetrieveProject(path, in.OrgAlias)
		cr.RetrieveOutput = out
		cr.RetrieveErr = rerr
		if rerr == nil {
			_ = store.MarkRetrieved(bundleRow.ID)
		}
	}

	return cr, nil
}

// Link registers an existing on-disk sfdx project directory as a
// bundle without writing or touching any files. Use this for
// hand-authored fixtures (test flows, manually-crafted package.xml)
// where bundle.create would overwrite the curated content.
//
// The dir must already exist and look like an sfdx project
// (sfdx-project.json present); this is what Stale() checks against.
// projectID is required — bundles have a non-null FK to a dev project.
func Link(store *devproject.Store, projectID, path, orgAlias string) (Bundle, error) {
	if store == nil {
		return Bundle{}, errors.New("nil store")
	}
	if projectID == "" {
		return Bundle{}, errors.New("project id is required")
	}
	if path == "" {
		return Bundle{}, errors.New("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Bundle{}, fmt.Errorf("normalize path: %w", err)
	}
	// Pre-check that the dir looks valid — the Stale check would
	// surface this lazily on first retrieve/deploy, but failing fast
	// at link time gives the caller a clear error.
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return Bundle{}, fmt.Errorf("not a directory: %s", abs)
	}
	if _, err := os.Stat(filepath.Join(abs, "sfdx-project.json")); err != nil {
		return Bundle{}, fmt.Errorf("missing sfdx-project.json in %s", abs)
	}
	dp, err := store.GetDevProject(projectID)
	if err != nil {
		return Bundle{}, err
	}
	if dp == nil {
		return Bundle{}, ErrNotFound{ID: projectID}
	}
	bundleRow, err := store.CreateBundle(projectID, abs, orgAlias)
	if err != nil {
		return Bundle{}, err
	}
	return toWire(bundleRow), nil
}

// Delete unlinks a bundle row from the store. Does NOT touch the
// on-disk directory — that's the caller's choice (the directory may
// have user edits / git history they want to preserve). KeepFiles
// is therefore implicit; the function exists only to remove the
// store entry.
func Delete(store *devproject.Store, bundleID string) error {
	if store == nil {
		return errors.New("nil store")
	}
	if _, err := fetchBundle(store, bundleID); err != nil {
		return err
	}
	return store.DeleteBundle(bundleID)
}

// ----- error types -----------------------------------------------

// ErrNotFound is returned when a bundle / project lookup misses.
type ErrNotFound struct{ ID string }

func (e ErrNotFound) Error() string {
	return fmt.Sprintf("not found: %s", e.ID)
}

// ErrStale fires when the bundle's on-disk directory is gone or no
// longer looks like a sfdx project. Caller can choose to relocate
// or delete the row.
type ErrStale struct {
	ID   string
	Path string
}

func (e ErrStale) Error() string {
	return fmt.Sprintf("bundle %s is stale (path %s missing or not a sfdx project)",
		e.ID, e.Path)
}

// ----- helpers ---------------------------------------------------

func toWire(b devproject.Bundle) Bundle {
	return Bundle{
		ID:              b.ID,
		DevProjectID:    b.DevProjectID,
		Path:            b.Path,
		DefaultOrgAlias: b.DefaultOrgAlias,
		CreatedAt:       b.CreatedAt.UTC().Format(time.RFC3339),
		LastRetrievedAt: optTime(b.LastRetrievedAt),
		LastDeployedAt:  optTime(b.LastDeployedAt),
		Stale:           b.Stale(),
	}
}

func optTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// dirHasFiles reports whether path exists and contains any entries. A
// missing path is "empty" (not an error) — MkdirAll will create it.
func dirHasFiles(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect bundle dir %q: %w", path, err)
	}
	return len(entries) > 0, nil
}

// ValidateCreateDestination refuses to write a new bundle into a non-empty
// directory unless the caller made an explicit force decision. Shared by the
// CLI/IPC service and the TUI so neither surface can silently truncate an
// existing Salesforce project.
func ValidateCreateDestination(path string, force bool) error {
	if force {
		return nil
	}
	nonEmpty, err := dirHasFiles(path)
	if err != nil {
		return err
	}
	if nonEmpty {
		return fmt.Errorf(
			"refusing to write bundle into non-empty directory %q "+
				"(it may be an existing project); choose a new directory or explicitly force/update the existing bundle",
			path)
	}
	return nil
}

func resolveBundlePath(suggested, projectName string) (string, error) {
	if suggested != "" {
		return filepath.Abs(suggested)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	// Default convention: ~/sf-deck-bundles/<sanitised-project>-<unix-ts>/
	// — keeps every bundle creation idempotent (the timestamp suffix
	// ensures a fresh dir even when the user re-runs the same project).
	sanitised := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		}
		return '-'
	}, projectName)
	name := fmt.Sprintf("%s-%d", sanitised, time.Now().Unix())
	return filepath.Join(home, "sf-deck-bundles", name), nil
}

func scopeDescription(orgFilter string) string {
	if orgFilter == "" {
		return ""
	}
	return " for org " + orgFilter
}

func kindLabels(items []devproject.Item) []string {
	if len(items) == 0 {
		return nil
	}
	seen := map[devproject.ItemKind]bool{}
	out := make([]string, 0, 4)
	for _, it := range items {
		if seen[it.Kind] {
			continue
		}
		seen[it.Kind] = true
		out = append(out, string(it.Kind))
	}
	return out
}
