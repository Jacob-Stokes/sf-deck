package settings

import "testing"

func TestSortPerView(t *testing.T) {
	s := &Settings{}
	if s.SortPerView() {
		t.Error("default should be shared (false)")
	}
	s.SetSortPerView(true)
	if !s.SortPerView() {
		t.Error("SetSortPerView(true) not reflected")
	}
	s.SetSortPerView(false)
	if s.SortPerView() {
		t.Error("SetSortPerView(false) should be shared")
	}
	var n *Settings
	if n.SortPerView() {
		t.Error("nil should be shared")
	}
}
