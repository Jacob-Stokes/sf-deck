package uilayout

import (
	"strings"
	"testing"
)

func stripAnsi(s string) string {
	out := ""
	in := false
	for _, r := range s {
		if r == 0x1b {
			in = true
			continue
		}
		if in {
			if r == 'm' {
				in = false
			}
			continue
		}
		out += string(r)
	}
	return out
}

// Repro for "expand LABEL fully -> nothing visible": a user-resized
// column wider than the pane.
func TestOverWideUserColumnStillShowsContent(t *testing.T) {
	cols := []ListColumn{
		{Name: "Name", Header: "NAME", Min: 18, Ideal: 28},
		{Name: "Type", Header: "TYPE", Min: 6, Ideal: 10},
		{Name: "Status", Header: "STATUS", Min: 6, Ideal: 10},
		{Name: "Version", Header: "VER", Min: 4, Ideal: 6},
		{Name: "Label", Header: "LABEL", Min: 16, Ideal: 32},
		{Name: "Modified", Header: "MODIFIED", Min: 14, Ideal: 16},
		{Name: "ModifiedBy", Header: "MODIFIED BY", Min: 12, Ideal: 22},
		{Name: "Marks", Header: "FLAGS", Min: 6, Ideal: 10},
	}
	cells := map[string]string{
		"Name": "My_Flow_Name", "Type": "Flow", "Status": "Active",
		"Version": "v12", "Label": "My Flow Label Here",
		"Modified": "2026-06-01 10:00", "ModifiedBy": "Jacob Stokes", "Marks": "M",
	}
	spec := ListTableSpec{
		Cols: cols, N: 1,
		Cell: func(row, col int) string { return cells[cols[col].Name] },
	}
	state := &ListTableState{}
	state.SetUserWidth("Label", 200) // snapped/expanded way past the pane
	inner := 150
	res := LayoutListTable(spec, state, inner)
	hdr := stripAnsi(RenderListTableHeader(spec, res, state, inner))
	row := stripAnsi(RenderListTableRow(spec, res, 0, false, true, inner))
	t.Logf("Overflow=%v HScroll=%d", res.Overflow, res.HScroll)
	t.Logf("HDR: %q", hdr)
	t.Logf("ROW: %q", row)
	if !strings.Contains(hdr, "LABEL") {
		t.Errorf("LABEL header not visible")
	}
	if !strings.Contains(row, "My Flow Label") {
		t.Errorf("label cell content not visible in row")
	}
}
