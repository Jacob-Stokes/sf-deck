package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sfurl"
)

// recognizeURL returns a populated globalSearchURL when input parses
// as a Salesforce URL or bare Id, otherwise nil. The returned struct
// carries:
//
//   - Label   — what the recognition pill should display
//   - Enter   — the navigation cmd, or nil when the parsed kind isn't
//     navigable (recognised but unsupported destination, e.g.
//     a setup page sf-deck doesn't model).
func recognizeURL(input string) *globalSearchURL {
	p, err := sfurl.Parse(input)
	if err != nil {
		return nil
	}
	return &globalSearchURL{
		Label: urlPillLabel(p),
		Enter: navigateFromParsed(p),
	}
}

func urlPillLabel(p sfurl.Parsed) string {
	switch p.Kind {
	case devproject.KindRecord:
		if p.SObject != "" {
			return "RECORD · " + p.SObject
		}
		return "RECORD"
	case devproject.KindSObject:
		if id, ok := p.Extra["listViewId"]; ok && id != "" {
			return "LIST VIEW · " + p.SObject
		}
		return "SOBJECT · " + p.SObject
	case devproject.KindField:
		return "FIELD · " + p.SObject
	case devproject.KindValidationRule:
		return "VALIDATION · " + p.SObject
	case devproject.KindRecordType:
		return "RECORD TYPE · " + p.SObject
	case devproject.KindApexClass:
		return "APEX CLASS"
	case devproject.KindApexTrigger:
		if p.SObject != "" {
			return "APEX TRIGGER · " + p.SObject
		}
		return "APEX TRIGGER"
	case devproject.KindFlow:
		return "FLOW"
	case devproject.KindFlowVersion:
		return "FLOW VERSION"
	case devproject.KindPermissionSet:
		return "PERMISSION SET"
	case devproject.KindPermissionSetGroup:
		return "PERMISSION SET GROUP"
	case devproject.KindProfile:
		return "PROFILE"
	case devproject.KindQueue:
		return "QUEUE"
	case devproject.KindPublicGroup:
		return "PUBLIC GROUP"
	case devproject.KindLWC:
		return "LWC"
	case devproject.KindAura:
		return "AURA"
	}
	return "URL"
}

func navigateFromParsed(p sfurl.Parsed) func(m *Model) tea.Cmd {
	switch p.Kind {
	case devproject.KindRecord:
		if p.ID == "" {
			return nil
		}
		// SObject is optional — the record-drill machinery accepts
		// "" and falls back to "open the records list and let the
		// user pick." Better than refusing the navigation.
		sobject := p.SObject
		id := p.ID
		return func(m *Model) tea.Cmd {
			return m.triggerRecordDrill(sobject, id, "", m.tab())
		}
	case devproject.KindSObject:
		if p.SObject == "" {
			return nil
		}
		return openObjectCmd(p.SObject)
	case devproject.KindField:
		if p.SObject == "" {
			return nil
		}
		return openObjectCmd(p.SObject)
	case devproject.KindFlow:
		if p.ID == "" {
			return nil
		}
		return openFlowCmd(p.ID)
	case devproject.KindApexClass:
		if p.ID == "" {
			return nil
		}
		return openApexClassCmd(p.ID)
	case devproject.KindPermissionSet:
		if p.ID == "" {
			return nil
		}
		return openPermSetCmd(p.ID)
	case devproject.KindPermissionSetGroup:
		if p.ID == "" {
			return nil
		}
		return openPSGCmd(p.ID)
	case devproject.KindProfile:
		if p.ID == "" {
			return nil
		}
		return openProfileCmd(p.ID, "")
	case devproject.KindQueue:
		if p.ID == "" {
			return nil
		}
		return openQueueCmd(p.ID)
	case devproject.KindPublicGroup:
		if p.ID == "" {
			return nil
		}
		return openPublicGroupCmd(p.ID)
	}
	return nil
}
