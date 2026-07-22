package settings

import "strings"

// Startup / launch-state accessors: which tab, sidebar layout, query
// line, SOQL seed, etc. sf-deck restores on launch. Split out of
// settings.go (which keeps the Settings struct + load/save core).

// StartupSidebarOpen / etc. resolve each tri-state bool. def is the
// built-in default the caller (model.go) would otherwise use.
func (s *Settings) StartupSidebarOpen(def bool) bool {
	if s == nil {
		return def
	}
	return boolOr(s.UI.Startup.SidebarOpen, s.UI.Startup.SidebarOpenSet, def)
}

func (s *Settings) StartupSidebarStacked(def bool) bool {
	if s == nil {
		return def
	}
	return boolOr(s.UI.Startup.SidebarStacked, s.UI.Startup.SidebarStackedSet, def)
}

func (s *Settings) StartupQueryLineVisible(def bool) bool {
	if s == nil {
		return def
	}
	return boolOr(s.UI.Startup.QueryLineVisible, s.UI.Startup.QueryLineVisibleSet, def)
}

func (s *Settings) StartupLeftRailOpen(def bool) bool {
	if s == nil {
		return def
	}
	return boolOr(s.UI.Startup.LeftRailOpen, s.UI.Startup.LeftRailOpenSet, def)
}

// StartupAutoLayout resolves whether to auto-decide sidebar placement
// at startup based on terminal width. Built-in default: false — the
// user opts in from Settings → Startup & defaults.
func (s *Settings) StartupAutoLayout() bool {
	if s == nil {
		return false
	}
	return boolOr(s.UI.Startup.AutoLayout, s.UI.Startup.AutoLayoutSet, false)
}

// StartupAutoLayoutMinWidth resolves the width threshold below which
// auto-layout stacks the sidebar. 0 / negative → built-in fallback.
func (s *Settings) StartupAutoLayoutMinWidth() int {
	if s == nil || s.UI.Startup.AutoLayoutMinWidth <= 0 {
		return StartupAutoLayoutMinWidthFallback
	}
	return s.UI.Startup.AutoLayoutMinWidth
}

// StartupStartTab resolves the launch tab ID.
func (s *Settings) StartupStartTab() string {
	if s == nil || s.UI.Startup.StartTab == "" {
		return StartupStartTabFallback
	}
	return s.UI.Startup.StartTab
}

// StartupDefaultSortDesc reports whether list columns default to
// descending. Built-in default is ascending (false).
func (s *Settings) StartupDefaultSortDesc() bool {
	if s == nil {
		return false
	}
	return strings.EqualFold(s.UI.Startup.DefaultSort, "desc")
}

// ChordSortModifiedDesc reports the first-press direction for the q-s
// (sort by Last Modified) chord. Defaults to TRUE (newest-first); only
// an explicit "asc" flips it to oldest-first.
func (s *Settings) ChordSortModifiedDesc() bool {
	if s == nil {
		return true
	}
	return !strings.EqualFold(s.UI.Startup.ChordSortModifiedDesc, "asc")
}

// SetChordSortModified persists the q-s first-press direction ("asc" /
// "desc"). Caller is responsible for Save().
func (s *Settings) SetChordSortModified(dir string) {
	if s != nil {
		s.UI.Startup.ChordSortModifiedDesc = dir
	}
}

// StartupGlobalSearchRecordsMode reports whether the global-search
// modal should open in records (SOSL) mode rather than metadata.
func (s *Settings) StartupGlobalSearchRecordsMode() bool {
	if s == nil {
		return false
	}
	return strings.EqualFold(s.UI.Startup.GlobalSearchMode, "records")
}

// StartupSOQLSeed resolves the editor seed query.
func (s *Settings) StartupSOQLSeed() string {
	if s == nil || strings.TrimSpace(s.UI.Startup.SOQLSeed) == "" {
		return StartupSOQLSeedFallback
	}
	return s.UI.Startup.SOQLSeed
}

func (s *Settings) SetStartupSidebarOpen(v bool) {
	if s == nil {
		return
	}
	s.UI.Startup.SidebarOpen, s.UI.Startup.SidebarOpenSet = v, true
}

func (s *Settings) SetStartupSidebarStacked(v bool) {
	if s == nil {
		return
	}
	s.UI.Startup.SidebarStacked, s.UI.Startup.SidebarStackedSet = v, true
}

func (s *Settings) SetStartupQueryLineVisible(v bool) {
	if s == nil {
		return
	}
	s.UI.Startup.QueryLineVisible, s.UI.Startup.QueryLineVisibleSet = v, true
}

func (s *Settings) SetStartupLeftRailOpen(v bool) {
	if s == nil {
		return
	}
	s.UI.Startup.LeftRailOpen, s.UI.Startup.LeftRailOpenSet = v, true
}

func (s *Settings) SetStartupAutoLayout(v bool) {
	if s == nil {
		return
	}
	s.UI.Startup.AutoLayout, s.UI.Startup.AutoLayoutSet = v, true
}

func (s *Settings) SetStartupAutoLayoutMinWidth(v int) {
	if s == nil {
		return
	}
	if v < 0 {
		v = 0
	}
	s.UI.Startup.AutoLayoutMinWidth = v
}

func (s *Settings) SetStartupStartTab(id string) {
	if s != nil {
		s.UI.Startup.StartTab = id
	}
}

func (s *Settings) SetStartupDefaultSort(dir string) {
	if s != nil {
		s.UI.Startup.DefaultSort = dir
	}
}

func (s *Settings) SetStartupGlobalSearchMode(mode string) {
	if s != nil {
		s.UI.Startup.GlobalSearchMode = mode
	}
}

func (s *Settings) SetStartupSOQLSeed(q string) {
	if s != nil {
		s.UI.Startup.SOQLSeed = q
	}
}
