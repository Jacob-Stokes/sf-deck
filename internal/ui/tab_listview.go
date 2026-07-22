package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/tablemodel"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// renderListViewResult shows a Salesforce list view's results — the
// user's actual columns as they appear in Lightning, rendered as a
// table with the row cursor. Same chip strip + breadcrumb treatment
// as Recent 50, but the data shape is different.
func (m Model) renderListViewResult(d *orgData, sobject, listViewID string, w, innerH int) string {
	inner := w - 4
	resKey := sobject + ":" + listViewID

	// Build the chip strip up-front so loading / error states render
	// WITH the strip visible — same shape as renderRecordsList's
	// sf-deck mode.  Without this, "fetching list view results…"
	// hid the chip bar entirely; the user couldn't see which list
	// view was loading or cycle to a different one.
	chips := recordsChips(m, d, sobject)
	chipSel := findChipIndex(chips, selectedRecordsChip(d, sobject))
	dash := m.renderDashboard("VIEWS", chips, chipSel, inner)
	withStrip := func(body string) string {
		if dash == "" {
			return body
		}
		return dash + "\n\n" + body
	}

	r, ok := d.ListViewResults[resKey]
	if !ok || r.FetchedAt().IsZero() {
		// Kick off the fetch if not already in flight. Normally
		// ensureDataFor wired this already, but rendering is cheap and
		// idempotent.
		if r == nil {
			return withStrip(theme.Subtle.Render("  loading list view…"))
		}
		if r.Busy() {
			return withStrip(theme.Subtle.Render("  fetching list view results…"))
		}
		if err := r.Err(); err != nil {
			return withStrip(redLine("  list view error: " + err.Error()))
		}
		return withStrip(theme.Subtle.Render("  loading list view…"))
	}
	result := r.Value()

	// Title + subtitle (ListView name from the catalog if we have it).
	lvName := listViewID
	if lv, ok := findListView(d, sobject, listViewID); ok {
		lvName = lv.Name
	}
	// Salesforce-mode chip is a discovery preview hard-capped at
	// settings.ListViewPreviewLimit() (default 50). SF's "size"
	// field is unreliable (reports rows-in-response, not unbounded
	// total), so we detect truncation heuristically: if we got back
	// exactly the cap, there were probably more rows.
	previewCap := m.settings.ListViewPreviewLimit()
	count := fmt.Sprintf("%d records", len(result.Records))
	capped := len(result.Records) >= previewCap
	if capped {
		count = fmt.Sprintf("%d records · capped", len(result.Records))
	}
	title := fmt.Sprintf("%s · %s · %s · %s",
		sobject, lvName, count,
		humanAge(r.FetchedAt())+stateSuffix(r.Busy(), r.Err()))

	var lines []string
	if dash != "" {
		lines = append(lines, dash)
	}
	lines = append(lines, sectionTitle(title))
	if capped {
		lines = append(lines, dimLine(
			"  preview capped at "+fmt.Sprintf("%d", previewCap)+
				" · press "+firstPretty(Keys.OpenLensManager)+" → Import from Salesforce to load the full list view as a chip",
			inner))
	}
	lines = append(lines, "")

	if len(result.Records) == 0 {
		lines = append(lines,
			theme.Subtle.Render("  (no records in this list view)"),
			"",
			dimLine("  esc back · "+firstPretty(Keys.Refresh)+" refresh", inner))
		return strings.Join(lines, "\n")
	}

	// Build the ListColumn spec from the SF list view's columns +
	// rendered cell content. Same auto-fit + horizontal scroll story
	// as the records subtab — see uilayout/listtable.go.
	cols := visibleColumns(result.Columns)
	if len(cols) == 0 {
		cols = []sf.ListViewColumn{{Name: "Id", Label: "Id"}}
	}
	resolved := resolveListViewColumns(cols, result.Records)
	listCols := resolved.ListColumns()
	cell := resolved.Cell(result.Records)
	tableState := d.RecordsTableStatePtr(sobject, listViewID)
	tagMap, projMap := m.bulkTagsAndProjectsForRecords(sobject, result.Records)
	leftGutters, rightGutters := m.listGutters(
		func(row int) string {
			id, _ := result.Records[row]["Id"].(string)
			if id == "" {
				return ""
			}
			return m.resolveTagGutterCell(devproject.KindRecord, sobject+":"+id, tagMap)
		},
		func(row int) string {
			id, _ := result.Records[row]["Id"].(string)
			if id == "" {
				return ""
			}
			return rowProjectGutterFromMap(devproject.KindRecord, sobject+":"+id, projMap)
		},
	)
	sortDataKey := fmt.Sprintf("listview:%s|%d|%d|%d", resKey, r.FetchedAt().UnixNano(), len(result.Records), len(listCols))
	spec := uilayout.ListTableSpec{
		Cols:         listCols,
		N:            len(result.Records),
		Gutters:      leftGutters,
		RightGutters: rightGutters,
		Cell:         cell,
		// Share the sort cache slot with cursor-translation calls
		// (line 125 above + drill/open paths).  Without a shared
		// key, the two callers thrash each other on every wheel tick.
		// See listrender_model.go for the same fix on the main path.
		SortCacheKey: cursorSortCacheKey(tableState, listCols, len(result.Records), sortDataKey),
	}
	res := uilayout.LayoutListTable(spec, tableState, inner)

	adapter := tableRowAdapter{
		State:   tableState,
		Cols:    listCols,
		N:       len(result.Records),
		Cell:    spec.Cell,
		DataKey: sortDataKey,
		RawCursor: func() RawRow {
			return RawRow(d.Cursors.Get(cursorKindRecordsRow, len(result.Records), sobject))
		},
		SetRawCursor: func(raw RawRow) {
			d.Cursors.Set(cursorKindRecordsRow, int(raw), len(result.Records), sobject)
		},
	}
	// renderRows consumes display-space ints; convert at the render
	// boundary.
	sel := int(adapter.DisplayCursor())
	lines = append(lines, uilayout.RenderListTableHeader(spec, res, tableState, inner))
	sortPerm := uilayout.SortedIndices(spec, tableState)
	lines = append(lines, renderRows(
		len(result.Records), sel, innerH, len(lines), 2, inner,
		func(i int) string {
			row := i
			if sortPerm != nil {
				row = sortPerm[i]
			}
			return uilayout.RenderListTableRow(spec, res, row, i == sel, m.focus == focusMain, inner, m.searchTerms())
		},
	)...)

	extras := firstPretty(Keys.OpenDefault) + " open · " +
		firstPretty(Keys.YankDefault) + " yank · " +
		firstPretty(Keys.Refresh) + " refresh"
	lines = append(lines, "", m.footerHint(m.listTableHint(tableState, res, len(listCols), nil, extras), inner))
	return strings.Join(lines, "\n")
}

// buildListViewCols mirrors buildRecordListCols but uses the
// SF-supplied label as the header (more user-friendly than the API
// name) and measures cells via formatCell.
func buildListViewCols(cols []sf.ListViewColumn, rows []map[string]any) []uilayout.ListColumn {
	return resolveListViewColumns(cols, rows).ListColumns()
}

func resolveListViewColumns(cols []sf.ListViewColumn, rows []map[string]any) tablemodel.Resolved[map[string]any] {
	defs := make([]tablemodel.ColumnDef[map[string]any], 0, len(cols))
	for _, c := range cols {
		defs = append(defs, listViewColumnDef(c, rows))
	}
	return tablemodel.Resolved[map[string]any]{Defs: defs, RequiredFields: []string{"Id"}}
}

func listViewColumnDef(c sf.ListViewColumn, rows []map[string]any) tablemodel.ColumnDef[map[string]any] {
	header := c.Label
	if header == "" {
		header = c.Name
	}
	min := lipglossWidth(header) + 2
	if min < 8 {
		min = 8
	}
	max := min
	for _, rec := range rows {
		v, _ := sf.Record(rec).Field(c.Name)
		if w := lipglossWidth(formatCell(v)); w > max {
			max = w
		}
	}
	ideal := max
	if ideal > uilayout.AutoMaxIdeal {
		ideal = uilayout.AutoMaxIdeal
	}
	return tablemodel.ColumnDef[map[string]any]{
		ID:          c.Name,
		Header:      header,
		Width:       tablemodel.Width{Min: min, Ideal: ideal},
		Style:       lipgloss.NewStyle().Foreground(theme.Fg),
		FetchFields: []string{c.Name},
		Searchable:  true,
		Exportable:  true,
		Render: func(rec map[string]any) string {
			v, _ := sf.Record(rec).Field(c.Name)
			return formatCell(v)
		},
	}
}

// visibleColumns filters out hidden columns.
func visibleColumns(cols []sf.ListViewColumn) []sf.ListViewColumn {
	var out []sf.ListViewColumn
	for _, c := range cols {
		if c.Hidden {
			continue
		}
		out = append(out, c)
	}
	return out
}
