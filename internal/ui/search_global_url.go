package ui

// URL/Id recognition for the global search modal.
//
// Pasting a Salesforce URL or bare Id into ctrl+f shouldn't make the
// user think — sf-deck recognises the shape, shows a small pill so
// they see what was detected, and Enter navigates straight to the
// resource instead of running a fuzzy search.
//
// Detection is per-keystroke: every input change runs sfurl.Parse. A
// successful parse pre-empts the fuzzy-search Enter path. Failure
// (or text that doesn't look URL/Id-shaped) leaves the modal in
// normal search mode.

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

// urlPillLabel formats a one-line summary the modal renders above
// the input — "RECORD · Account" / "FLOW" / "FIELD · Account.<id>"
// etc. Stays compact; the user just needs to see "yes the parse
// recognised this."
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

// navigateFromParsed returns the Enter closure that jumps to the
// parsed resource. Reuses the open*Cmd helpers the existing search
// hits already use, so URL navigation lands on the same surfaces as
// fuzzy-search navigation.
//
// Returns nil when the parsed kind has no navigation contract today
// (or when required fields are missing — record without an Id, for
// instance). Caller renders the recognition pill but Enter is a
// no-op.
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
		// Field URLs identify the field by a 15/18-char Id, not by
		// API name. Until we wire an Id→name lookup, route to the
		// parent sObject's Fields subtab and let the user pick.
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
