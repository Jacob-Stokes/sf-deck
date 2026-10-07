package ui

import (
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

// domainSchemaFields is the /objects Schema field-list chip set. Not a
// full domain — it has no manager, wizard, or user chips — but its
// registry lives in the same map so cross-cutting operations (settings
// reload, active-org gating) can't forget it.
const domainSchemaFields chipDomain = "fields"

type chipDomainDef struct {
	Domain   chipDomain
	Builtins []qchip.Chip
	// WizardFields returns the New-view wizard's "+ Add filter"
	// catalogue. nil means the domain has no wizard (schema fields).
	// Catalogues must only reference fields the domain's row type
	// evaluates via Field(name).
	WizardFields func(m Model, scope string) []cwField
}

func chipDomainDefs() []chipDomainDef {
	static := func(fn func() []cwField) func(Model, string) []cwField {
		return func(Model, string) []cwField { return fn() }
	}
	return []chipDomainDef{
		{Domain: domainRecords, Builtins: qchip.RecordBuiltins,
			WizardFields: func(m Model, scope string) []cwField { return m.recordFields(scope) }},
		{Domain: domainObjects, Builtins: qchip.SObjectBuiltins, WizardFields: static(objectFields)},
		{Domain: domainFlows, Builtins: qchip.FlowBuiltins, WizardFields: static(flowFields)},
		{Domain: domainApex, Builtins: qchip.ApexBuiltins, WizardFields: static(apexClassFields)},
		{Domain: domainTriggers, Builtins: qchip.TriggerBuiltins, WizardFields: static(apexTriggerFields)},
		{Domain: domainLWC, Builtins: qchip.LWCBuiltins, WizardFields: static(lwcFields)},
		{Domain: domainAura, Builtins: qchip.AuraBuiltins, WizardFields: static(auraFields)},
		{Domain: domainPermSets, Builtins: qchip.PermSetBuiltins, WizardFields: static(permSetFields)},
		{Domain: domainPSGs, Builtins: qchip.PSGBuiltins, WizardFields: static(psgFields)},
		{Domain: domainProfiles, Builtins: qchip.ProfileBuiltins, WizardFields: static(profileFields)},
		{Domain: domainQueues, Builtins: qchip.QueueBuiltins, WizardFields: static(queueFields)},
		{Domain: domainPublicGroup, Builtins: qchip.PublicGroupBuiltins, WizardFields: static(publicGroupFields)},
		{Domain: domainSOQLSaved, Builtins: qchip.SOQLSavedBuiltins, WizardFields: static(savedQueryFields)},
		{Domain: domainSOQLHistory, Builtins: qchip.SOQLHistoryBuiltins, WizardFields: static(soqlHistoryFields)},
		{Domain: domainRecent, Builtins: qchip.RecentBuiltins, WizardFields: static(recentFields)},
		{Domain: domainUsers, Builtins: qchip.UserBuiltins, WizardFields: static(userFields)},
		{Domain: domainDeploys, Builtins: qchip.DeployBuiltins, WizardFields: static(deployFields)},
		{Domain: domainDashboards, Builtins: qchip.DashboardBuiltins, WizardFields: static(dashboardFields)},
		{Domain: domainReportTypes, Builtins: qchip.ReportTypeBuiltins, WizardFields: static(reportTypeFields)},
		{Domain: domainActiveUsers, Builtins: qchip.ActiveUsersBuiltins}, // built-in lenses only; no custom-chip wizard
		{Domain: domainSchemaFields, Builtins: qchip.FieldBuiltins},      // no wizard
	}
}

func newChipRegistries() map[chipDomain]*qchip.Registry {
	out := make(map[chipDomain]*qchip.Registry, len(chipDomainDefs()))
	for _, def := range chipDomainDefs() {
		out[def.Domain] = qchip.NewRegistry(string(def.Domain), def.Builtins)
	}
	return out
}

func (m Model) chipRegistry(d chipDomain) *qchip.Registry {
	return m.chipRegistries[d]
}
