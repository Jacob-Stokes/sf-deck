package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// TestRecordsRenders verifies both modes of the Records tab render
// without panicking on seeded data.
func TestRecordsRenders(t *testing.T) {
	c, err := cache.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	m := New(c)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 40})
	mm := nm.(Model)
	mm.orgs = []sf.Org{{
		Alias: "t", Username: "u@t.com",
		InstanceURL: "https://x.my.salesforce.com",
		Status:      "Connected", LastUsed: time.Now().Format(time.RFC3339),
	}}
	mm.focus = focusMain
	// queryLineHidden defaults to true; this test asserts the SOQL
	// line IS visible, so flip it back for the assertion below.
	mm.queryLineHidden = false

	// Picker mode — seed sobjects.
	d := mm.ensureOrgDataRef("u@t.com")
	d.Tab = TabRecords
	sobs := []sf.SObject{{Name: "Account", Label: "Account", IsCustomizable: true}}
	d.SObjects.Apply(resourceUpdatedMsg{
		Scope: "u@t.com", Key: "sobjects_v5", Payload: &sobs,
	})
	d.SyncListViews()
	out := mm.View().Content
	if !strings.Contains(out, "VIEWS") {
		t.Errorf("picker mode should show the VIEWS dashboard header; got:\n%s", out)
	}
	if !strings.Contains(out, "PICK AN SOBJECT") {
		t.Errorf("picker mode should show the PICK AN SOBJECT title; got:\n%s", out)
	}
	if !strings.Contains(out, "Account") {
		t.Error("picker mode should list Account")
	}

	// Record-list mode — drill into Account.
	d.RecordsSObjectCur = "Account"
	// Default chip is now Recently viewed; pin "Changed" explicitly
	// so the test exercises the legacy records-list rendering path
	// (the "Changed" chip drives d.EnsureRecords / d.Records below).
	d.ListViewCur["Account"] = syntheticRecentID
	list := sf.RecordsList{
		SObject: "Account", HasName: true, HasModDate: true,
		Records: []map[string]any{
			{
				"attributes":       map[string]any{"type": "Account", "url": "/services/data/v66.0/sobjects/Account/001xx"},
				"Id":               "001xx",
				"Name":             "Acme Inc",
				"LastModifiedDate": time.Now().Add(-3 * time.Hour).Format(time.RFC3339),
			},
		},
		Query: "SELECT Id, Name FROM Account",
	}
	r := d.EnsureRecords("t", "Account")
	r.Apply(resourceUpdatedMsg{Scope: "u@t.com", Key: "records:Account", Payload: &list})

	out2 := mm.View().Content
	if !strings.Contains(out2, "Acme Inc") {
		t.Errorf("record-list mode should show the Name; got:\n%s", out2)
	}
	if !strings.Contains(out2, "SELECT Id") {
		t.Error("record-list mode should echo the query")
	}
}

func TestRecordsFetchColumnsAlwaysIncludeId(t *testing.T) {
	got := recordsFetchColumns([]string{"Name", "Status__c", "Name"})
	want := []string{"Id", "Name", "Status__c"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("recordsFetchColumns = %v, want %v", got, want)
		}
	}
}

func TestSalesforceRecentlyViewedChipUsesChipRecordsForRows(t *testing.T) {
	m := New(nil)
	m.orgs = []sf.Org{{
		Alias:       "t",
		Username:    "u@t.com",
		InstanceURL: "https://x.my.salesforce.com",
		Status:      "Connected",
		LastUsed:    time.Now().Format(time.RFC3339),
	}}
	m.selected = 0
	m.setTab(TabObjectDetail)

	d := m.ensureOrgDataRef("u@t.com")
	d.DescribeCur = "Account"
	d.RecordsSObjectCur = "Account"
	d.ObjectSubtab = 2 // Records
	setChipMode(d, "Account", ChipModeSalesforce)

	list := sf.RecordsList{
		SObject: "Account",
		HasName: true,
		Records: []map[string]any{{
			"Id":   "001xx",
			"Name": "Acme Inc",
		}},
		Columns: []string{"Id", "Name"},
		Query:   "SELECT Id, Name FROM Account WHERE Id IN ('001xx')",
	}
	r := &Resource[sf.RecordsList]{
		Scope: "u@t.com",
		Key:   "chiprecords:Account:" + sfRecentlyViewedChipID,
	}
	r.Set(list)
	d.ChipRecords["Account:"+sfRecentlyViewedChipID] = r

	visible, visibleIdx := visibleRecordsAndIdx(d, "Account")
	if len(visible) != 1 || len(visibleIdx) != 1 {
		t.Fatalf("visible rows = %d/%d, want 1/1", len(visible), len(visibleIdx))
	}
	if got := visible[0]["Name"]; got != "Acme Inc" {
		t.Fatalf("visible Name = %v, want Acme Inc", got)
	}
	if cols, _, _, ok := recordsSortContext(d, "Account", visible); !ok || len(cols) == 0 {
		t.Fatalf("recordsSortContext ok=%v cols=%d, want records-backed columns", ok, len(cols))
	}
	if _, cols := m.recordsListTable(); len(cols) == 0 {
		t.Fatal("recordsListTable returned no columns for SF Recently Viewed chip records")
	}
}

func TestListViewColumnsCanGrowPastContentWidth(t *testing.T) {
	rows := []map[string]any{{
		"Id":   "001xx",
		"Name": "Acme",
	}}
	cols := buildListViewCols([]sf.ListViewColumn{{
		Name:  "Name",
		Label: "Name",
	}}, rows)
	if len(cols) != 1 {
		t.Fatalf("cols = %d, want 1", len(cols))
	}

	state := &uilayout.ListTableState{}
	spec := uilayout.ListTableSpec{
		Cols: cols,
		N:    len(rows),
		Cell: func(row, col int) string {
			return "Acme"
		},
	}
	res := uilayout.LayoutListTable(spec, state, 120)
	before := res.Widths[0]
	uilayout.StepResize(spec, state, res, 0, +1, 4)
	if got := state.UserWidths["Name"]; got <= before {
		t.Fatalf("list-view column width after grow = %d, want > %d", got, before)
	}
}

func TestRecordColumnsCanResizePastDefaultWidth(t *testing.T) {
	rows := []map[string]any{{
		"Id":               "a3p000000001AAA",
		"Name":             "577076",
		"LastModifiedDate": time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
	}}
	list := sf.RecordsList{
		SObject: "Request__c",
		Columns: []string{"Id", "Name", "LastModifiedDate"},
		Records: rows,
	}
	cols := buildRecordListCols(list, rows)
	if len(cols) != 3 {
		t.Fatalf("cols = %d, want 3", len(cols))
	}

	state := &uilayout.ListTableState{}
	spec := uilayout.ListTableSpec{
		Cols: cols,
		N:    len(rows),
		Cell: func(row, col int) string {
			return renderRecordCell(rows[row], cols[col].Name)
		},
	}
	res := uilayout.LayoutListTable(spec, state, 120)
	for i, col := range cols {
		before := res.Widths[i]
		uilayout.StepResize(spec, state, res, i, +1, 4)
		if got := state.UserWidths[col.Name]; got <= before {
			t.Fatalf("%s width after grow = %d, want > %d", col.Name, got, before)
		}
		delete(state.UserWidths, col.Name)
	}
}

func TestObjectRecordsResizeKeyBeatsSubtabShortcut(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c, err := cache.Open()
	if err != nil {
		t.Fatalf("cache open: %v", err)
	}
	defer c.Close()

	m := New(c)
	m.width = 180
	m.height = 40
	m.orgs = []sf.Org{{
		Alias:       "t",
		Username:    "u@t.com",
		InstanceURL: "https://x.my.salesforce.com",
		Status:      "Connected",
		LastUsed:    time.Now().Format(time.RFC3339),
	}}
	m.selected = 0
	m.focus = focusMain
	m.setTab(TabObjectDetail)

	d := m.ensureOrgDataRef("u@t.com")
	d.DescribeCur = "Request__c"
	d.RecordsSObjectCur = "Request__c"
	d.ObjectSubtab = 2 // Records
	d.ListViewCur["Request__c"] = "mine"

	list := sf.RecordsList{
		SObject: "Request__c",
		HasName: true,
		Records: []map[string]any{{
			"Id":               "a3p000000001AAA",
			"Name":             "577076",
			"LastModifiedDate": time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
		}},
		Columns: []string{"Id", "Name", "LastModifiedDate"},
		Query:   "SELECT Id, Name, LastModifiedDate FROM Request__c",
	}
	r := &Resource[sf.RecordsList]{
		Scope: "u@t.com",
		Key:   "chiprecords:Request__c:mine",
	}
	r.Set(list)
	d.ChipRecords["Request__c:mine"] = r

	state, cols := (&m).activeListTable()
	if state == nil || len(cols) == 0 {
		t.Fatalf("active list table state=%v cols=%d", state, len(cols))
	}
	beforeSubtab := d.ObjectSubtab

	nextModel, _ := m.handleKey(fakeKey(">"))
	next := nextModel.(Model)
	nextData := next.data["u@t.com"]
	nextState := nextData.RecordsTableStatePtr("Request__c", "mine")
	if got := nextState.UserWidths["Id"]; got == 0 {
		t.Fatalf("grow key did not set Id width; widths=%#v", nextState.UserWidths)
	}
	if nextData.ObjectSubtab != beforeSubtab {
		t.Fatalf("> switched subtab to %d, want records subtab %d", nextData.ObjectSubtab, beforeSubtab)
	}
}
