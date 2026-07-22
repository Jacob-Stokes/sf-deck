package ui

// Bundle FILES view: a cd-style file browser of the bundle's
// on-disk directory. Sibling to list_surface_bundle_detail.go's
// COMPONENTS view; both live under TabBundleDetail and the user
// flips between them with `[` / `]`.
//
// Mode is intentionally simple: ReadDir(b.Path + cwd) each time
// the cwd changes. No watcher. The .. row appears at the top of
// every non-root view so Enter on it pops one level. Real
// directories show `<dir>` markers + size 0 to keep parity with
// regular files in the same table.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/tablemodel"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// bundleFileRow is one entry in the FILES view's list.
// IsParent flags the synthetic ".." row at the top of any
// non-root listing; the activate hook treats it as "pop one
// segment of cwd."
type bundleFileRow struct {
	Name     string
	IsDir    bool
	IsParent bool
	Size     int64  // bytes; 0 for dirs (file system doesn't tell us "tree weight" cheaply)
	Modified string // pre-formatted "2006-01-02 15:04"
}

// bundleFileColumnSchema is the column spec for the FILES view.
// Name / kind / size / modified — same shape any file explorer
// shows. Kind is rendered as a small inline tag so the user can
// see at a glance whether a row is a dir without having to read
// the size column.
func bundleFileColumnSchema() tablemodel.Schema[bundleFileRow] {
	return tablemodel.Schema[bundleFileRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Kind", "Size", "Modified"}
		},
		Columns: map[string]tablemodel.ColumnDef[bundleFileRow]{
			"Name": {
				Header: "NAME",
				Width:  tablemodel.Width{Min: 20, Ideal: 38},
				Render: func(r bundleFileRow) string {
					if r.IsParent {
						return ".."
					}
					return r.Name
				},
			},
			"Kind": {
				Header: "KIND",
				Width:  tablemodel.Width{Min: 6, Ideal: 8},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: func(r bundleFileRow) string {
					if r.IsParent {
						return "parent"
					}
					if r.IsDir {
						return "dir"
					}
					return "file"
				},
			},
			"Size": {
				Header: "SIZE",
				Width:  tablemodel.Width{Min: 8, Ideal: 10},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: func(r bundleFileRow) string {
					if r.IsParent || r.IsDir {
						return "—"
					}
					return humanBytes(int(r.Size))
				},
			},
			"Modified": {
				Header: "MODIFIED",
				Width:  tablemodel.Width{Min: 16, Ideal: 16},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: func(r bundleFileRow) string {
					if r.IsParent {
						return ""
					}
					return r.Modified
				},
			},
		},
	}
}

// bundleFileListCols is the canonical column spec returned by
// the TabSpec.ListTable hook for the files view.
func bundleFileListCols() []uilayout.ListColumn {
	return mustResolveColumns(bundleFileColumnSchema()).ListColumns()
}

// readBundleDir lists the bundle's working directory (b.Path
// joined with the relative cwd). Sort order: dirs first
// alphabetical, then files alphabetical — same as most file
// browsers. A leading ".." row is prepended when cwd is non-
// empty so Enter has somewhere to go up.
//
// Errors fall through as an empty slice + a flash on the caller's
// side. The renderer treats "no rows" as empty-state — the
// distinction between "real empty dir" and "ReadDir errored" isn't
// surfaced in v1 because the empty-dir case is far more common
// (mid-folders the user just made).
func readBundleDir(bundleRoot, relCwd string) ([]bundleFileRow, error) {
	full := filepath.Join(bundleRoot, relCwd)
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}
	rows := make([]bundleFileRow, 0, len(entries)+1)
	if relCwd != "" {
		rows = append(rows, bundleFileRow{Name: "..", IsParent: true})
	}
	for _, e := range entries {
		info, ierr := e.Info()
		if ierr != nil {
			// Skip rows we can't stat — usually a transient
			// permission flake. Don't fail the whole listing.
			continue
		}
		rows = append(rows, bundleFileRow{
			Name:     e.Name(),
			IsDir:    e.IsDir(),
			Size:     info.Size(),
			Modified: info.ModTime().Format("2006-01-02 15:04"),
		})
	}
	// Sort dirs first, then files, both alpha.
	parentRow := -1
	for i, r := range rows {
		if r.IsParent {
			parentRow = i
			break
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		// Keep .. at the top.
		if rows[i].IsParent {
			return true
		}
		if rows[j].IsParent {
			return false
		}
		if rows[i].IsDir != rows[j].IsDir {
			return rows[i].IsDir
		}
		return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
	})
	_ = parentRow
	return rows, nil
}
