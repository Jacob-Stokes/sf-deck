package settings

// Per-surface row/fetch limit accessors (recent records, deploy
// history, notifications, reference picker, …). Split out of
// settings.go.

func (s *Settings) LimitRecentRecords() int {
	if s == nil {
		return LimitRecentRecordsFallback
	}
	return clampLimit(s.UI.Limits.RecentRecords, LimitRecentRecordsFallback)
}

func (s *Settings) LimitNotifications() int {
	if s == nil {
		return LimitNotificationsFallback
	}
	return clampLimit(s.UI.Limits.Notifications, LimitNotificationsFallback)
}

func (s *Settings) LimitRecentLogins() int {
	if s == nil {
		return LimitRecentLoginsFallback
	}
	return clampLimit(s.UI.Limits.RecentLogins, LimitRecentLoginsFallback)
}

func (s *Settings) LimitDeployHistory() int {
	if s == nil {
		return LimitDeployHistoryFallback
	}
	return clampLimit(s.UI.Limits.DeployHistory, LimitDeployHistoryFallback)
}

func (s *Settings) LimitAsyncJobHistory() int {
	if s == nil {
		return LimitAsyncJobHistoryFallback
	}
	return clampLimit(s.UI.Limits.AsyncJobHistory, LimitAsyncJobHistoryFallback)
}

func (s *Settings) LimitReferencePicker() int {
	if s == nil {
		return LimitReferencePickerFallback
	}
	return clampLimit(s.UI.Limits.ReferencePicker, LimitReferencePickerFallback)
}

// LimitGlobalSearch resolves the SOSL result cap, additionally clamped
// to Salesforce's hard max of 50.
func (s *Settings) LimitGlobalSearch() int {
	v := LimitGlobalSearchFallback
	if s != nil {
		v = clampLimit(s.UI.Limits.GlobalSearch, LimitGlobalSearchFallback)
	}
	if v > 50 {
		v = 50
	}
	return v
}

func (s *Settings) SetLimitRecentRecords(n int) {
	if s != nil {
		s.UI.Limits.RecentRecords = n
	}
}

func (s *Settings) SetLimitNotifications(n int) {
	if s != nil {
		s.UI.Limits.Notifications = n
	}
}

func (s *Settings) SetLimitRecentLogins(n int) {
	if s == nil {
		return
	}
	s.UI.Limits.RecentLogins = n
}

func (s *Settings) SetLimitDeployHistory(n int) {
	if s != nil {
		s.UI.Limits.DeployHistory = n
	}
}

func (s *Settings) SetLimitAsyncJobHistory(n int) {
	if s != nil {
		s.UI.Limits.AsyncJobHistory = n
	}
}

func (s *Settings) SetLimitReferencePicker(n int) {
	if s != nil {
		s.UI.Limits.ReferencePicker = n
	}
}

func (s *Settings) SetLimitGlobalSearch(n int) {
	if s != nil {
		s.UI.Limits.GlobalSearch = n
	}
}
