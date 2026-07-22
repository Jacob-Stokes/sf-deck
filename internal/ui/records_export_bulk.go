package ui

// UI-side adapter for internal/exporters/bulk. This file owns the
// modelHost wiring + the dispatcher routing; the actual bulk-export
// pipeline (goroutine, channel pump, msg types) lives in the
// internal/exporters/bulk subpackage.
//
// Pattern: Model satisfies bulk.Host via a handful of small
// methods (Flash, FlashFor, OpenPathPicker, DefaultPath,
// ActiveUsername, Flight, SetFlight). Each of those delegates to
// existing UI primitives. The bulk-export code never reaches into
// Model fields directly — every interaction goes through the
// interface.

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/exporters"
	"github.com/Jacob-Stokes/sf-deck/internal/exporters/bulk"
)

// Flash implements bulk.Host.
func (m *Model) Flash(msg string) { m.flash(msg) }

// FlashFor implements bulk.Host.
func (m *Model) FlashFor(msg string, d time.Duration) { m.flashFor(msg, d) }

// OpenPathPicker implements bulk.Host. Builds the edit-modal
// state on the UI side so internal/exporters/bulk stays ignorant
// of the modal infrastructure.
func (m *Model) OpenPathPicker(title, hint, defaultPath string, onConfirm func(path string) tea.Msg) tea.Cmd {
	var savedPath string
	state := editModalState{
		Title:       title,
		Hint:        hint,
		InitialBody: defaultPath,
		Save: func(val string, _ any) error {
			savedPath = val
			if savedPath == "" {
				return fmt.Errorf("path required")
			}
			return nil
		},
		OnSuccess: func() tea.Cmd {
			return func() tea.Msg {
				return onConfirm(savedPath)
			}
		},
	}
	return m.openEditModal(state)
}

// DefaultPath implements bulk.Host — returns the user-configured
// export directory + a timestamped CSV filename for `label`.
func (m *Model) DefaultPath(label string) string {
	return m.defaultRecordsExportPath(label, exporters.FormatCSV)
}

// ActiveUsername implements bulk.Host.
func (m *Model) ActiveUsername() string {
	if len(m.orgs) == 0 {
		return ""
	}
	return m.orgs[m.selected].Username
}

// Flight + SetFlight implement bulk.Host. The in-flight Flight
// pointer lives on Model.transient.bulkExport so the existing
// ctrl+c intercept + dispatcher re-arm logic can find it.
func (m *Model) Flight() *bulk.Flight     { return m.bulkExport }
func (m *Model) SetFlight(f *bulk.Flight) { m.bulkExport = f }
