package settings

// Layout dimension accessors (autocomplete rows, resize step, modal
// row caps, pinned subtabs). Split out of settings.go.

func (s *Settings) LayoutObjectPinnedSubtabs() int {
	if s == nil {
		return LayoutObjectPinnedSubtabsFallback
	}
	return layoutVal(s.UI.Layout.ObjectPinnedSubtabs, LayoutObjectPinnedSubtabsFallback, 1)
}

func (s *Settings) LayoutAutocompleteRows() int {
	if s == nil {
		return LayoutAutocompleteRowsFallback
	}
	return layoutVal(s.UI.Layout.AutocompleteRows, LayoutAutocompleteRowsFallback, 3)
}

func (s *Settings) LayoutColumnResizeStep() int {
	if s == nil {
		return LayoutColumnResizeStepFallback
	}
	return layoutVal(s.UI.Layout.ColumnResizeStep, LayoutColumnResizeStepFallback, 1)
}

func (s *Settings) LayoutDownloadsModalRows() int {
	if s == nil {
		return LayoutDownloadsModalRowsFallback
	}
	return layoutVal(s.UI.Layout.DownloadsModalRows, LayoutDownloadsModalRowsFallback, 4)
}

func (s *Settings) LayoutCommandPaletteRows() int {
	if s == nil {
		return LayoutCommandPaletteRowsFallback
	}
	return layoutVal(s.UI.Layout.CommandPaletteRows, LayoutCommandPaletteRowsFallback, 4)
}

func (s *Settings) LayoutGlobalSearchRows() int {
	if s == nil {
		return LayoutGlobalSearchRowsFallback
	}
	return layoutVal(s.UI.Layout.GlobalSearchRows, LayoutGlobalSearchRowsFallback, 5)
}

func (s *Settings) SetLayoutObjectPinnedSubtabs(n int) {
	if s != nil {
		s.UI.Layout.ObjectPinnedSubtabs = n
	}
}

func (s *Settings) SetLayoutAutocompleteRows(n int) {
	if s != nil {
		s.UI.Layout.AutocompleteRows = n
	}
}

func (s *Settings) SetLayoutColumnResizeStep(n int) {
	if s != nil {
		s.UI.Layout.ColumnResizeStep = n
	}
}

func (s *Settings) SetLayoutDownloadsModalRows(n int) {
	if s != nil {
		s.UI.Layout.DownloadsModalRows = n
	}
}

func (s *Settings) SetLayoutCommandPaletteRows(n int) {
	if s != nil {
		s.UI.Layout.CommandPaletteRows = n
	}
}

func (s *Settings) SetLayoutGlobalSearchRows(n int) {
	if s != nil {
		s.UI.Layout.GlobalSearchRows = n
	}
}
