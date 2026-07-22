package ui

import (
	"testing"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func TestResourceApplySyncsOnlyLandedList(t *testing.T) {
	c, err := cache.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	m := New(c)
	m.orgs = []sf.Org{{
		Alias:       "t",
		Username:    "u@t.com",
		InstanceURL: "https://x.my.salesforce.com",
		Status:      "Connected",
		LastUsed:    time.Now().Format(time.RFC3339),
	}}
	m.selected = 0
	d := m.ensureOrgData("u@t.com")

	d.SObjectList.Set([]sf.SObject{
		{Name: "Account", Label: "Account", IsCustomizable: true},
		{Name: "Contact", Label: "Contact", IsCustomizable: true},
	})
	d.SObjectList.SetCursor(1)
	if got, want := d.SObjectList.Cursor(), 1; got != want {
		t.Fatalf("test setup SObjectList cursor = %d, want %d", got, want)
	}

	rows := []sf.ApexClassRow{{ID: "01px", Name: "MyClass"}}
	next, _ := m.applyResourceMsg(resourceUpdatedMsg{
		Scope:   "u@t.com",
		Key:     "apex_classes_v2",
		Payload: &rows,
	})
	m = next.(Model)

	if got, want := d.SObjectList.Cursor(), 1; got != want {
		t.Fatalf("SObjectList cursor = %d, want %d; unrelated resource sync reset it", got, want)
	}
	if got, want := d.ApexClassList.Len(), 1; got != want {
		t.Fatalf("ApexClassList len = %d, want %d", got, want)
	}
}
