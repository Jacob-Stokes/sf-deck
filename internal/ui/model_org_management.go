package ui

// Overlays that edit org groups + auth lifecycle.
//
// Extracted from model.go. modelOrgManagement is embedded into Model so
// existing field access (m.orgManageModal) keeps working unchanged.

// modelOrgManagement owns overlays that edit org groups and auth lifecycle.
type modelOrgManagement struct {
	// orgManageModal is the live state of the roomy "Org Manager"
	// modal that owns every group / auth-lifecycle edit action. nil
	// when closed. Opened from the rail via OrgManageOpen (ctrl+e
	// by default); the rail itself stays a quick-nav surface and
	// doesn't directly handle edit keys.
	orgManageModal *orgManageModalState
}
