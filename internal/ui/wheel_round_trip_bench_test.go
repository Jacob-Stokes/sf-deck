package ui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func BenchmarkWheelRoundTripWithItems(b *testing.B) {
	m := benchmarkWheelModel(b, 800)
	msg := tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next, _ := m.Update(msg)
		m = next.(Model)
		_ = m.viewImpl()
	}
}

func BenchmarkObjectsMoveRenderLargeAlternating(b *testing.B) {
	m := benchmarkWheelModel(b, 5000)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		delta := 1
		if (i/250)%2 == 1 {
			delta = -1
		}
		next, _ := m.moveCursor(delta)
		m = next
		_ = m.viewImpl()
	}
}

func benchmarkWheelModel(b *testing.B, n int) Model {
	b.Helper()
	c, err := cache.Open()
	if err != nil {
		b.Fatalf("cache open: %v", err)
	}
	b.Cleanup(func() { _ = c.Close() })

	m := New(c)
	m.width, m.height = 180, 60
	m.sidebarOpen = true
	m.focus = focusMain
	m.orgs = []sf.Org{{
		Alias:       "bench",
		Username:    "bench@example.com",
		InstanceURL: "https://bench.example.com",
		Status:      "Connected",
		LastUsed:    time.Now().Format(time.RFC3339),
	}}
	m.selected = 0
	m.setTab(TabObjects)

	d := m.ensureOrgData(m.orgs[0].Username)
	items := make([]sf.SObject, n)
	for i := range items {
		name := fmt.Sprintf("BenchObject%03d__c", i)
		if i%3 == 0 {
			name = fmt.Sprintf("BenchSystem%03d", i)
		}
		items[i] = sf.SObject{
			Name:             name,
			Label:            fmt.Sprintf("Bench Object %03d", i),
			IsCustomizable:   i%4 != 0,
			KeyPrefix:        fmt.Sprintf("%03d", i%1000),
			DeploymentStatus: "Deployed",
			ApexTriggerable:  i%5 != 0,
			WorkflowEnabled:  i%7 != 0,
			LastModifiedDate: "2026-01-02T03:04:05.000+0000",
		}
		if i%10 == 0 {
			items[i].Namespace = "pkg"
		}
	}
	d.SObjects.Set(items)
	d.SyncListViews()
	d.SObjectList.SetExtra(func(o sf.SObject) bool { return o.IsCustomizable })

	_ = m.viewImpl()
	return m
}
