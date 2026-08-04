package ui

import (
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/services/permissionops"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func permissionWriteService(m Model, org sf.Org) *permissionops.Service {
	if m.permissions != nil {
		return m.permissions
	}
	gate := orgwrite.NewGate(func(string) (sf.Org, error) { return org, nil },
		func(resolved sf.Org) settings.SafetyLevel {
			return m.settings.Resolve(resolved.Username, settings.OrgKind(resolved.Kind()), resolved.Alias)
		})
	return permissionops.New(gate)
}
