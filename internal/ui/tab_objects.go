package ui

import (
	"fmt"
	"image/color"
	"reflect"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// bulkTagsForObjects pre-fetches tag bindings for sObjects so the
// table cell closure can prepend a dot marker without a per-row
// store call. Used by sobjectListTable; mirror of bulkTagsForFlows.
//
// Memoised on *orgData via the gutter cache — successive renders
// within the same generation reuse the result without re-running
// the SQLite query or re-allocating the wanted-set. See
// internal/ui/gutter_cache.go.
//
// Two short-circuits before the fetch:
//   - tag gutter hidden via Ctrl+T → return nil; no caller needs
//     the data and the renderer skips the gutter entirely.
//   - no devproject store / no current org / empty items → nil.
func (m Model) bulkTagsForObjects(items []sf.SObject) map[string][]devproject.Tag {
	return bulkTagsForItems(m, items, gutterDomainSObject, devproject.KindSObject,
		func(it sf.SObject) string { return it.Name })
}

// bulkProjectsForObjects mirrors bulkTagsForObjects for the project
// gutter. Same caching + early-return pattern.
func (m Model) bulkProjectsForObjects(items []sf.SObject) map[string][]devproject.DevProject {
	return bulkProjectsForItems(m, items, gutterDomainSObject, devproject.KindSObject,
		func(it sf.SObject) string { return it.Name })
}

// renderObjects draws the Objects tab: a two-zone layout with a
// dashboard (view chips) on top and the filtered sObject list below.
// Left/right arrows cycle views when the main pane is focused; `=`
// toggles the dashboard on/off for full-height browsing.
func (m Model) renderObjects(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	if !canUseOrg(o) {
		return theme.Subtle.Render("  org disconnected")
	}
	d := m.ensureOrgDataRef(o.Username)
	if d.SObjects.FetchedAt().IsZero() {
		if d.SObjects.Busy() {
			return theme.Subtle.Render("  loading sobjects…")
		}
		if err := d.SObjects.Err(); err != nil {
			return redLine("  error: " + err.Error())
		}
		return theme.Subtle.Render("  no sobjects")
	}

	chips := m.stripRows(domainObjects, "*")
	if len(chips) == 0 {
		chips = []chipRow{{ID: "all", Label: "All", Count: -1}}
	}
	sel := m.objectsChipIdx()
	if sel < 0 || sel >= len(chips) {
		sel = 0
	}
	dash := m.renderDashboard("VIEWS", chips, sel, inner)

	var out []string
	if dash != "" {
		out = append(out, dash)
	}

	// Tab-specific empty-state: project-mode chip needs a tailored
	// hint that the shared renderer's generic "no matches" doesn't
	// cover. Gate on ExtraCount (project predicate only) so an
	// active search inside a non-empty project still falls through
	// to the shared renderer — that one keeps the search box visible
	// and shows "no matches" when the user typed something that filters
	// to zero rows.
	if d.SObjectList.ExtraCount() == 0 && m.projectChipActive() {
		out = append(out, theme.Subtle.Render(m.projectEmptyHint("sObjects")))
		return strings.Join(out, "\n")
	}

	// Hand off to the shared list-table renderer via the surface's
	// BuildRenderModel hook. listSurface owns the data shape; the
	// renderer is generic.
	model, ok := objectsListSurface.BuildRenderModel(m, d)
	if !ok {
		// Surface declined to build a model (data not ready). Fall
		// back to a minimal "loading…" line — should be unreachable
		// given the FetchedAt guard above, but defensive.
		out = append(out, theme.Subtle.Render("  loading…"))
		return strings.Join(out, "\n")
	}
	usedAbove := usedLines(out)
	budget := innerH - usedAbove
	out = append(out, renderListModel(m, model, m.focus, inner, budget)...)
	return strings.Join(out, "\n")
}

// sobjectListTable builds the shared list-table spec + rendered rows
// for an sObject list. Used by /objects AND the /records picker —
// same data shape, same layout, same per-row tints (custom sObjects
// get cyan name tinting via runtime style override).
//
// state is the per-surface ListTableState (each surface keeps its
// own so column widths / scroll / sort don't bleed across views).
// Returns the lines to splice into the surrounding chrome.
func sobjectListTable(m Model, items []sf.SObject, cur int, inner, innerH, reserved, trailing int, state *uilayout.ListTableState) []string {
	cols := []uilayout.ListColumn{
		{Name: "Name", Header: "NAME", Min: 18, Ideal: 32,
			Style: lipgloss.NewStyle().Foreground(theme.Fg)},
		{Name: "Label", Header: "LABEL", Min: 16, Ideal: 36,
			Style: lipgloss.NewStyle().Foreground(theme.Muted)},
		{Name: "Marks", Header: "FLAGS", Min: 8, Ideal: 14, Max: 18},
	}
	cols = m.applyFlagsColumnMode(cols)
	tagMap := m.bulkTagsForObjects(items)
	projMap := m.bulkProjectsForObjects(items)
	marks := marksForSObjectList(items)
	leftGutters, rightGutters := m.listGutters(
		func(row int) string {
			return m.resolveTagGutterCell(devproject.KindSObject, items[row].Name, tagMap)
		},
		func(row int) string {
			return rowProjectGutterFromMap(devproject.KindSObject, items[row].Name, projMap)
		},
	)
	itemsPtr := uintptr(0)
	if len(items) > 0 {
		itemsPtr = reflect.ValueOf(items).Pointer()
	}
	sortDataKey := fmt.Sprintf("sobjects:%d|%d", itemsPtr, len(items))
	spec := uilayout.ListTableSpec{
		Cols:         cols,
		N:            len(items),
		Gutters:      leftGutters,
		RightGutters: rightGutters,
		Marks:        marks,
		Cell: func(row, col int) string {
			it := items[row]
			switch cols[col].Name {
			case "Name":
				return it.Name
			case "Label":
				return dashIfEmpty(it.Label)
			case "Marks":
				return m.renderFlagsCell(marks, row)
			}
			return ""
		},
		// Share the cache slot with cursor-translation calls. Without
		// this the bespoke /records sObject-picker render would build
		// its own ad-hoc key and ping-pong the cache slot, paying for
		// a fresh sort every frame.  See listrender_model.go for the
		// same fix on the main shared renderer.
		SortCacheKey: cursorSortCacheKey(state, cols, len(items), sortDataKey),
	}
	res := uilayout.LayoutListTable(spec, state, inner)
	var sortPerm []int
	if state == nil || !state.RowsOrdered {
		sortPerm = uilayout.SortedIndices(spec, state)
	}
	terms := m.searchTerms()
	out := []string{uilayout.RenderListTableHeader(spec, res, state, inner)}
	out = append(out, renderRows(
		len(items), cur, innerH, reserved+1, trailing, inner,
		func(i int) string {
			row := i
			if sortPerm != nil {
				row = sortPerm[i]
			}
			return uilayout.RenderListTableRow(spec, res, row, i == cur, m.focus == focusMain, inner, terms)
		},
	)...)
	return out
}

// renderObjectDetail draws the fields browser for a selected sObject
// as a proper table: column headers, aligned columns, dim vertical
// rules between them. Columns (l-to-r): API NAME · LABEL · TYPE ·
// FLAGS · DETAIL, where DETAIL picks the most useful single-line
// context per field type (reference targets, picklist count, formula
// marker, default value, inline help).
func (m Model) renderObjectDetail(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	d := m.ensureOrgDataRef(o.Username)
	if d.DescribeCur == "" {
		return theme.Subtle.Render("  press enter on an object in /objects first")
	}
	r, ok := d.Describes[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		if r != nil && r.Err() != nil {
			return m.describeErrorLine(d.DescribeCur, r.Err())
		}
		return theme.Subtle.Render("  describing " + d.DescribeCur + "…")
	}
	v := r.Value()
	// Sync the shared ListView from the describe so cursor / search /
	// sort all run through the same engine as /flows et al.
	fs := d.syncFieldList(d.DescribeCur, v.Fields)
	// Apply the selected field-filter chip (All / Custom / Picklist /
	// Required / …) as the ListView pre-filter, then build the VIEWS
	// strip that drives it.
	m.applySchemaChip(fs)
	chips := stripRowsFor(m.chipRegistry(domainSchemaFields), "*")
	chipSel := findChipIndex(chips, schemaChipID(fs))
	dash := m.renderDashboard("VIEWS", chips, chipSel, inner)

	// Header block above the field table (object identity + caps). The
	// search bar + table + footer come from renderListModel below.
	var head []string
	head = append(head, sectionTitle(v.Name+" — "+v.Label))
	if pills := renderMarkPills(markPillsForSObject(v.Name)); pills != "" {
		head = append(head, "  "+pills)
	}
	head = append(head, dimLine(fmt.Sprintf("  key prefix %s · %d fields · %s",
		v.KeyPrefix, len(v.Fields), capsFlags(v)), inner))
	head = append(head, dimLine("  updated "+humanAge(r.FetchedAt())+stateSuffix(r.Busy(), r.Err()), inner))
	if dash != "" {
		head = append(head, dash)
	}
	head = append(head, "")

	resolved := mustResolveColumns(fieldColumnSchema())
	cols := resolved.ListColumns()
	sobj := v.Name

	// Install the sort order BEFORE snapshotting Filtered(): SetOrder
	// invalidates the filtered cache, so reading items first would
	// capture the previous ordering and the Cell/gutter closures below
	// would render it stale until the next frame's cursor move forced a
	// rebuild. (The canonical /objects surface installs order first for
	// exactly this reason.)
	installListViewOrderRows(&fs.List, &fs.Table, cols,
		func(its []sf.Field, row, col int) string {
			if row < 0 || row >= len(its) || col < 0 || col >= len(cols) {
				return ""
			}
			return resolvedSortCellByID(resolved, its[row], cols[col].Name)
		})

	items := fs.List.Filtered()

	tagMap, projMap := m.bulkTagsAndProjectsForFields(sobj, items)
	left, right := m.listGutters(
		func(row int) string {
			if row < 0 || row >= len(items) {
				return ""
			}
			return m.resolveTagGutterCell(devproject.KindField, sobj+"."+items[row].Name, tagMap)
		},
		func(row int) string {
			if row < 0 || row >= len(items) {
				return ""
			}
			return rowProjectGutterFromMap(devproject.KindField, sobj+"."+items[row].Name, projMap)
		},
	)

	model := listRenderModel{
		Title:  headerWithSearchPill("FIELDS", *fs.List.SearchPtr()),
		State:  &fs.Table,
		Search: fs.List.SearchPtr(),
		Cols:   cols,
		N:      len(items),
		Cursor: fs.List.Cursor(),
		Cell: func(row, col int) string {
			if row < 0 || row >= len(items) || col < 0 || col >= len(cols) {
				return ""
			}
			return resolvedCellByID(resolved, items[row], cols[col].Name)
		},
		Gutters:      left,
		RightGutters: right,
		Recolor: func(row, col int, base lipgloss.Style) lipgloss.Style {
			// Custom fields read in cyan on the NAME column — matches
			// the old renderFieldRow tint.
			if row >= 0 && row < len(items) && col >= 0 && col < len(cols) &&
				cols[col].Name == "Name" && items[row].Custom {
				return base.Foreground(theme.Cyan)
			}
			return base
		},
		Empty:       "  no matching fields",
		DataVersion: listVersionWithStore(fs.List.Version(), m),
	}

	body := renderListModel(m, model, m.focus, inner, innerH-len(head))
	return strings.Join(append(head, body...), "\n")
}

// fieldTypeDisplay formats a field's type for the TYPE column. Adds
// length suffix for strings and picks up virtual types the API hides
// inside `string`: autonumber becomes its own type, encrypted strings
// read as "encrypted", formulas show the computed return type.
func fieldTypeDisplay(f sf.Field) string {
	switch {
	case f.AutoNumber:
		return "autonumber"
	case f.Encrypted:
		if f.Type == "string" {
			return "encrypted"
		}
		return "encrypted " + f.Type
	case f.CalculatedFormula != "" && f.Type != "":
		return f.Type + " (formula)"
	}
	if f.Length > 0 && (f.Type == "string" || f.Type == "textarea") {
		return fmt.Sprintf("%s(%d)", f.Type, f.Length)
	}
	if (f.Type == "double" || f.Type == "currency" || f.Type == "percent") &&
		(f.Precision > 0 || f.Scale > 0) {
		return fmt.Sprintf("%s(%d,%d)", f.Type, f.Precision, f.Scale)
	}
	return f.Type
}

// fieldFlagsIcons renders the 5-slot icon strip for the FLAGS column.
// Each slot is always present (as "·" when inactive) so flags live at
// fixed x-positions and the eye can scan down a column for one letter.
//
//	slot 1 · R — required (not nillable)
//	slot 2 · U — unique
//	slot 3 · X — external ID
//	slot 4 · A — auto-populated (autonumber or calculated formula)
//	slot 5 · B — behavior modifier (encrypted / restricted picklist /
//	              case-sensitive / cascade-delete — whichever hits first)
//
// Inactive slots are dimmed so the active letters pop visually.
func fieldFlagsIcons(f sf.Field) string {
	dim := lipgloss.NewStyle().Foreground(theme.FgDim).Render("·")

	slot := func(active bool, letter string, c color.Color) string {
		if !active {
			return dim
		}
		return lipgloss.NewStyle().Foreground(c).Bold(true).Render(letter)
	}

	// Slot 5 picks the most significant behavior modifier — once we
	// add more this can become a cycling badge, but "first hit" keeps
	// each row predictable for now.
	var behaviorLetter string
	var behaviorColor color.Color
	switch {
	case f.Encrypted:
		behaviorLetter = "E"
		behaviorColor = theme.Magenta
	case f.RestrictedPicklist:
		behaviorLetter = "P" // restricted-Picklist
		behaviorColor = theme.Blue
	case f.CaseSensitive:
		behaviorLetter = "C"
		behaviorColor = theme.Blue
	case f.CascadeDelete:
		behaviorLetter = "D"
		behaviorColor = theme.Red
	}

	parts := []string{
		slot(!f.Nillable, "R", theme.Yellow),
		slot(f.Unique, "U", theme.Cyan),
		slot(f.ExternalID, "X", theme.Green),
		slot(f.AutoNumber || f.CalculatedFormula != "", "A", theme.Yellow),
		slot(behaviorLetter != "", behaviorLetter, behaviorColor),
	}
	return strings.Join(parts, " ")
}

// fieldDetailDisplay picks the single highest-signal thing per field:
// reference targets, picklist count (with restricted marker), or the
// word "formula" for calc fields. Lower-value details (default values,
// help text, full formula body) live in the sidebar instead.
func fieldDetailDisplay(f sf.Field) string {
	if len(f.ReferenceTo) > 0 {
		return "→ " + strings.Join(f.ReferenceTo, ", ")
	}
	if len(f.PicklistValues) > 0 {
		if f.RestrictedPicklist {
			return fmt.Sprintf("%d values · restricted", len(f.PicklistValues))
		}
		return fmt.Sprintf("%d values", len(f.PicklistValues))
	}
	if f.CalculatedFormula != "" {
		return "formula"
	}
	return ""
}
