package ui

import (
	"github.com/Jacob-Stokes/sf-deck/internal/services/metadataops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func metadataWriteService(m Model, org sf.Org) *metadataops.Service {
	if m.metadata != nil {
		return m.metadata
	}
	return metadataops.New(metadataGateFor(m, org))
}

func metadataEditorService(m Model, org sf.Org) *metadataops.EditorService {
	if m.metaEditors != nil {
		return m.metaEditors
	}
	return metadataops.NewEditor(metadataGateFor(m, org))
}

func metadataGateFor(m Model, org sf.Org) *orgwrite.Gate {
	return orgwrite.NewGate(func(string) (sf.Org, error) { return org, nil },
		func(resolved sf.Org) settings.SafetyLevel {
			return m.settings.Resolve(resolved.Username, settings.OrgKind(resolved.Kind()), resolved.Alias)
		})
}
