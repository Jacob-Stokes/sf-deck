package ui

import (
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/services/userops"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func userWriteService(m *Model) *userops.Service {
	if m == nil {
		return userops.New(nil)
	}
	if m.users != nil {
		return m.users
	}
	org, ok := m.currentOrg()
	if !ok {
		return userops.New(nil)
	}
	gate := orgwrite.NewGate(func(string) (sf.Org, error) { return org, nil },
		func(resolved sf.Org) settings.SafetyLevel {
			return m.settings.Resolve(resolved.Username, settings.OrgKind(resolved.Kind()), resolved.Alias)
		})
	return userops.New(gate)
}
