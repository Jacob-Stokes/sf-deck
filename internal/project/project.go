package project

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Project is a local SFDX project rooted at Path.
type Project struct {
	Path             string            `json:"path"`
	Name             string            `json:"name"`
	Namespace        string            `json:"namespace"`
	PackageDirs      []PackageDir      `json:"packageDirectories"`
	SourceAPIVersion string            `json:"sourceApiVersion"`
	PackageAliases   map[string]string `json:"packageAliases"`
}

type PackageDir struct {
	Path          string `json:"path"`
	Default       bool   `json:"default"`
	Package       string `json:"package,omitempty"`
	VersionName   string `json:"versionName,omitempty"`
	VersionNumber string `json:"versionNumber,omitempty"`
}

// projectJSON matches the on-disk sfdx-project.json shape.
type projectJSON struct {
	Namespace        string            `json:"namespace"`
	SourceAPIVersion string            `json:"sourceApiVersion"`
	PackageDirs      []PackageDir      `json:"packageDirectories"`
	PackageAliases   map[string]string `json:"packageAliases"`
}

// Load reads sfdx-project.json from the given directory.
func Load(dir string) (*Project, error) {
	path := filepath.Join(dir, "sfdx-project.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw projectJSON
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	return &Project{
		Path:             dir,
		Name:             filepath.Base(dir),
		Namespace:        raw.Namespace,
		SourceAPIVersion: raw.SourceAPIVersion,
		PackageDirs:      raw.PackageDirs,
		PackageAliases:   raw.PackageAliases,
	}, nil
}

// Discover walks `roots` up to a bounded depth and returns every directory
// that contains an sfdx-project.json. Skips node_modules, .git, dist, etc.
// Pure local FS — no network.
func Discover(roots []string, maxDepth int) ([]*Project, error) {
	if maxDepth <= 0 {
		maxDepth = 4
	}
	var found []*Project
	seen := map[string]bool{}

	for _, root := range roots {
		root = expand(root)
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if depthFrom(root, path) > maxDepth {
					return fs.SkipDir
				}
				if shouldSkipDir(d.Name()) {
					return fs.SkipDir
				}
				return nil
			}
			if d.Name() != "sfdx-project.json" {
				return nil
			}
			dir := filepath.Dir(path)
			if seen[dir] {
				return nil
			}
			seen[dir] = true
			if p, err := Load(dir); err == nil {
				found = append(found, p)
			}
			return nil
		})
	}
	return found, nil
}

// DefaultRoots are the directories we scan for SFDX projects.
func DefaultRoots() []string {
	return []string{"~/", "~/code", "~/work", "~/dev", "~/projects", "~/src"}
}

func expand(p string) string {
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

func depthFrom(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return 99
	}
	if rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

func shouldSkipDir(name string) bool {
	switch name {
	case "node_modules", ".git", "dist", "build", ".sfdx", ".sf",
		"target", "vendor", "Pods", ".gradle", ".cache", ".venv", "venv",
		"__pycache__", ".next", ".nuxt", ".turbo", ".vercel",
		"Library", "Applications", "Downloads":
		return true
	}
	return strings.HasPrefix(name, ".")
}
