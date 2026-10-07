package ui

// Schema-subtab field-filter chips (the VIEWS strip on /objects/<X>/
// Schema). Lightweight by design: stock built-in chips only (see
// qchip.FieldBuiltins) — no project / visited / custom-chip / manager
// plumbing, none of which is meaningful for a per-sObject field list.

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// schemaChipID returns the effective selected chip ID for a field
// state, defaulting to "all" (the locked favourite) when unset.
func schemaChipID(fs *describeFieldState) string {
	if fs == nil || fs.ChipID == "" {
		return "all"
	}
	return fs.ChipID
}

func (m Model) applySchemaChip(fs *describeFieldState) {
	if fs == nil {
		return
	}
	id := schemaChipID(fs)
	c, ok := m.chipRegistry(domainSchemaFields).FindByID(id)
	if !ok {
		fs.List.SetExtra(nil)
		return
	}
	fs.List.SetExtra(chipMatcherFor[sf.Field](c, chipSubs(m.activeOrgData())))
}

func (m *Model) cycleSchemaChip(delta int) {
	d := m.activeOrgData()
	if d == nil || d.DescribeCur == "" {
		return
	}
	fs := d.FieldState(d.DescribeCur)
	chips := stripRowsFor(m.chipRegistry(domainSchemaFields), "*")
	nav := withoutOverflow(chips)
	if len(nav) == 0 {
		return
	}
	cur := findChipIndex(nav, schemaChipID(fs))
	cur = wrapIdx(cur+delta, len(nav))
	fs.ChipID = nav[cur].ID
	m.applySchemaChip(fs)
	fs.List.ResetCursor()
}

// openSchemaChipOverflow shows the non-favourite field chips (External
// ID / Encrypted / Lookup / Master-Detail / …) as a choice modal.
// Picking one selects it for the current sObject's field list. This is
// the lightweight stand-in for the full chip-overflow modal — no
// transient/favourite/persistence machinery, which fields don't need.
func (m *Model) openSchemaChipOverflow() tea.Cmd {
	d := m.activeOrgData()
	if d == nil || d.DescribeCur == "" {
		return nil
	}
	others := m.chipRegistry(domainSchemaFields).OthersFor("*")
	if len(others) == 0 {
		m.flash("no more field filters")
		return nil
	}
	opts := make([]choiceOption, 0, len(others))
	for _, c := range others {
		opts = append(opts, choiceOption{Label: c.Label, Value: c.ID})
	}
	sobj := d.DescribeCur
	return m.openChoiceModal(choiceModalState{
		Title:   "Field filters · " + sobj,
		Hint:    "Enter to apply · Esc to cancel",
		Options: opts,
		OnSuccessTyped: func(val any) tea.Cmd {
			id, _ := val.(string)
			fs := d.FieldState(sobj)
			fs.ChipID = id
			m.applySchemaChip(fs)
			fs.List.ResetCursor()
			return nil
		},
	})
}
