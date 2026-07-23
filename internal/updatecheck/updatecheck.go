// Package updatecheck discovers newer stable sf-deck releases.
//
// It deliberately does not download or install anything. Automatic callers
// use a small on-disk cache so normal TUI launches make at most one anonymous
// GitHub Releases request per 24 hours.
package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// LatestReleaseAPI is GitHub's stable-release endpoint. GitHub excludes
	// drafts and prereleases from this route; we also check the response flags
	// defensively.
	LatestReleaseAPI = "https://api.github.com/repos/Jacob-Stokes/sf-deck/releases/latest"
	ReleasesURL      = "https://github.com/Jacob-Stokes/sf-deck/releases"
	CheckInterval    = 24 * time.Hour
)

// Options controls one update lookup.
type Options struct {
	// Force bypasses the 24-hour cache. The successful result still refreshes
	// the cache for later automatic checks.
	Force bool
}

// Result is shared by the TUI and the stable CLI JSON envelope.
type Result struct {
	CurrentVersion   string    `json:"current_version"`
	LatestVersion    string    `json:"latest_version,omitempty"`
	UpdateAvailable  bool      `json:"update_available"`
	Kind             string    `json:"kind,omitempty"` // patch | minor | major
	ReleaseURL       string    `json:"release_url,omitempty"`
	PublishedAt      time.Time `json:"published_at,omitzero"`
	CheckedAt        time.Time `json:"checked_at"`
	FromCache        bool      `json:"from_cache"`
	DevelopmentBuild bool      `json:"development_build,omitempty"`
	NoStableRelease  bool      `json:"no_stable_release,omitempty"`
}

// Service is the dependency boundary used by app, CLI, and TUI layers.
type Service interface {
	Check(context.Context, string, Options) (Result, error)
}

// Checker performs and caches GitHub release lookups. Exported fields make it
// straightforward to test with an httptest server and temporary state file.
type Checker struct {
	Client    *http.Client
	URL       string
	StatePath string
	Now       func() time.Time
}

// New returns the production checker.
func New() *Checker {
	return &Checker{
		Client: &http.Client{Timeout: 3 * time.Second},
		URL:    LatestReleaseAPI,
		Now:    time.Now,
	}
}

type release struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
}

type state struct {
	CheckedAt time.Time `json:"checked_at"`
	Release   release   `json:"release"`
}

// Check returns the latest stable release relative to currentVersion.
func (c *Checker) Check(ctx context.Context, currentVersion string, opts Options) (Result, error) {
	if c == nil {
		c = New()
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	checkedAt := now().UTC()

	path := c.StatePath
	if path == "" {
		path, _ = DefaultStatePath()
	}
	if !opts.Force && path != "" {
		if cached, err := loadState(path); err == nil &&
			!cached.CheckedAt.IsZero() &&
			checkedAt.Sub(cached.CheckedAt) >= 0 &&
			checkedAt.Sub(cached.CheckedAt) < CheckInterval {
			return evaluate(currentVersion, cached.Release, cached.CheckedAt, true)
		}
	}

	rel, err := c.fetch(ctx)
	if err != nil {
		return Result{}, err
	}
	st := state{CheckedAt: checkedAt, Release: rel}
	// Cache persistence is best-effort: a read-only home directory should not
	// turn a successful network check into a command failure.
	if path != "" {
		_ = saveState(path, st)
	}
	return evaluate(currentVersion, rel, checkedAt, false)
}

func (c *Checker) fetch(ctx context.Context) (release, error) {
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	url := c.URL
	if url == "" {
		url = LatestReleaseAPI
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return release{}, fmt.Errorf("create update request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	// GitHub requires a User-Agent. Deliberately omit the running version:
	// the request discovers releases; it is not product analytics.
	req.Header.Set("User-Agent", "sf-deck-update-check")

	resp, err := client.Do(req)
	if err != nil {
		return release{}, fmt.Errorf("check for updates: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		// A private repository or a repository with no published releases is
		// a valid pre-launch state, not an application error.
		return release{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		detail := strings.TrimSpace(string(body))
		if detail != "" {
			return release{}, fmt.Errorf("check for updates: GitHub returned %s: %s", resp.Status, detail)
		}
		return release{}, fmt.Errorf("check for updates: GitHub returned %s", resp.Status)
	}
	var rel release
	dec := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := dec.Decode(&rel); err != nil {
		return release{}, fmt.Errorf("decode update response: %w", err)
	}
	if rel.Draft || rel.Prerelease {
		return release{}, nil
	}
	if rel.HTMLURL == "" {
		rel.HTMLURL = ReleasesURL
	}
	return rel, nil
}

func evaluate(currentVersion string, rel release, checkedAt time.Time, fromCache bool) (Result, error) {
	result := Result{
		CurrentVersion: strings.TrimSpace(currentVersion),
		ReleaseURL:     rel.HTMLURL,
		PublishedAt:    rel.PublishedAt,
		CheckedAt:      checkedAt,
		FromCache:      fromCache,
	}
	if isDevelopmentVersion(currentVersion) {
		result.DevelopmentBuild = true
		if rel.TagName != "" {
			latest, err := parseVersion(rel.TagName)
			if err != nil {
				return Result{}, fmt.Errorf("latest release: %w", err)
			}
			result.LatestVersion = latest.String()
		} else {
			result.NoStableRelease = true
		}
		return result, nil
	}
	current, err := parseVersion(currentVersion)
	if err != nil {
		return Result{}, fmt.Errorf("current version: %w", err)
	}
	result.CurrentVersion = current.String()
	if rel.TagName == "" {
		result.NoStableRelease = true
		return result, nil
	}
	latest, err := parseVersion(rel.TagName)
	if err != nil {
		return Result{}, fmt.Errorf("latest release: %w", err)
	}
	result.LatestVersion = latest.String()
	if compareVersion(latest, current) <= 0 {
		return result, nil
	}
	result.UpdateAvailable = true
	switch {
	case latest.major > current.major:
		result.Kind = "major"
	case latest.minor > current.minor:
		result.Kind = "minor"
	default:
		result.Kind = "patch"
	}
	return result, nil
}

var versionPattern = regexp.MustCompile(`^[vV]?([0-9]+)\.([0-9]+)\.([0-9]+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)

type version struct {
	major, minor, patch int
	prerelease          string
}

func parseVersion(raw string) (version, error) {
	raw = strings.TrimSpace(raw)
	m := versionPattern.FindStringSubmatch(raw)
	if m == nil {
		return version{}, fmt.Errorf("%q is not semantic version major.minor.patch", raw)
	}
	vals := [3]int{}
	for i := range vals {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return version{}, fmt.Errorf("parse %q: %w", raw, err)
		}
		vals[i] = n
	}
	return version{major: vals[0], minor: vals[1], patch: vals[2], prerelease: m[4]}, nil
}

func (v version) String() string {
	out := fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch)
	if v.prerelease != "" {
		out += "-" + v.prerelease
	}
	return out
}

func compareVersion(a, b version) int {
	for _, pair := range [][2]int{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	// A stable release is newer than its prerelease. Exact prerelease
	// ordering is unnecessary here because GitHub's /latest route only
	// returns stable releases, but this handles a prerelease current build.
	if a.prerelease == "" && b.prerelease != "" {
		return 1
	}
	if a.prerelease != "" && b.prerelease == "" {
		return -1
	}
	return strings.Compare(a.prerelease, b.prerelease)
}

func isDevelopmentVersion(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "dev", "development", "snapshot":
		return true
	}
	return false
}

// DefaultStatePath returns the daily-check cache location.
func DefaultStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sf-deck", "update-state.json"), nil
}

func loadState(path string) (state, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return state{}, err
	}
	var st state
	if err := json.Unmarshal(b, &st); err != nil {
		return state{}, err
	}
	return st, nil
}

func saveState(path string, st state) error {
	if path == "" {
		return errors.New("empty update state path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".update-state-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Chmod(path, 0o600)
}
