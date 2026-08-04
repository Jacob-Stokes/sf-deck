package ui

// Value pickers — the "what should I type here" dropdowns that open
// when a criterion has a fixed-vocabulary value (picklist) or a
// reference into another sObject (RecordTypeId, OwnerId, parent
// lookups). Generic over a small valueSource interface so new
// sources slot in without touching the wizard / picker plumbing.
//
// New sources implement valueSource and add a case in
// valueSourceFor. Existing sources today:
//
//   picklistValueSource — values come from describe.PicklistValues
//   recordTypeValueSource — RecordType rows for the parent sObject
//   userValueSource     — the org's User list (via the home cache),
//                         with a $userId literal pinned to the top
//   genericLookupValueSource — fallback for any reference field that
//                         isn't User/RecordType — runs a basic
//                         `SELECT Id, Name FROM <referent>` query
//
// All sources share the same anchored picker overlay (picker.go) so
// the user gets identical UX regardless of which field they activated.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// valueOption is one row in any value picker. Value is the literal
// that gets typed into the criterion buffer when the user picks the
// row; Label + Hint are display-only.
type valueOption struct {
	Value string
	Label string
	Hint  string
}

// valueSource is the contract every "what value should this be"
// dropdown implements. Two-stage: Loading tells the picker whether
// to show a spinner placeholder, Items returns whatever's currently
// available (may grow as an async fetch completes).
type valueSource interface {
	Title() string
	Items() []valueOption
	Loading() bool
}

// valueSourceFor returns the right source for a focused criterion,
// or nil when the field has no constrained vocabulary (free text /
// integer / date) and the user should just type.
//
// The decision tree:
//
//  1. Field is a picklist / multipicklist → picklistValueSource
//     using the cached describe's PicklistValues.
//  2. Field is a reference to RecordType → recordTypeValueSource
//     using the cached RecordType resource for the parent sObject.
//  3. Field is a reference to User → userValueSource (TODO: today
//     falls through to generic lookup; can specialise once the
//     User cache lands).
//  4. Field is any other reference → genericLookupValueSource
//     (TODO: same — needs a per-target lookup resource).
//
// 3+4 are scaffolded but not wired in this commit — the file's
// shape is set up so adding them is one new source type plus one
// case in valueSourceFor.
func (m Model) valueSourceFor(parentSObject string, criterion cwField) valueSource {
	if criterion.Field == "" {
		return nil
	}
	desc, ok := m.cachedDescribe(parentSObject)
	if !ok {
		return nil
	}
	field := lookupDescribeField(desc, criterion.Field)
	if field == nil {
		return nil
	}
	switch {
	case field.Type == "picklist" || field.Type == "multipicklist" || field.Type == "combobox":
		return newPicklistValueSource(*field)
	case field.Type == "reference" && referencesRecordType(*field):
		return m.newRecordTypeValueSource(parentSObject)
	}
	// Free text / integer / date / id → no source, user just types.
	return nil
}

// lookupDescribeField finds the named field in a describe, or returns
// nil. Centralised so future sources can share the lookup.
func lookupDescribeField(desc sf.SObjectDescribe, name string) *sf.Field {
	for i := range desc.Fields {
		if desc.Fields[i].Name == name {
			return &desc.Fields[i]
		}
	}
	return nil
}

// referencesRecordType reports whether the given reference field
// points at the RecordType sObject. Used to route to the
// RecordType-specific source (which is cached, scoped to the parent
// sObject, and renders Name + DeveloperName).
func referencesRecordType(f sf.Field) bool {
	for _, t := range f.ReferenceTo {
		if t == "RecordType" {
			return true
		}
	}
	return false
}

// ---- picklist value source -----------------------------------------

// picklistValueSource serves values from the cached describe's
// PicklistValues. Synchronous — never loading.
type picklistValueSource struct {
	field   sf.Field
	options []valueOption
}

func newPicklistValueSource(f sf.Field) *picklistValueSource {
	opts := make([]valueOption, 0, len(f.PicklistValues))
	for _, pv := range f.PicklistValues {
		if !pv.Active {
			continue
		}
		opt := valueOption{Value: pv.Value, Label: pv.Label}
		if pv.Label == "" {
			opt.Label = pv.Value
		}
		if pv.DefaultValue {
			opt.Hint = "default"
		}
		opts = append(opts, opt)
	}
	return &picklistValueSource{field: f, options: opts}
}

func (p *picklistValueSource) Title() string        { return p.field.Label + " · pick a value" }
func (p *picklistValueSource) Items() []valueOption { return p.options }
func (p *picklistValueSource) Loading() bool        { return false }

// ---- record-type value source --------------------------------------

// recordTypeValueSource pulls from the cached RecordType resource
// for a given parent sObject. The resource is part of the standard
// orgData layer so it's already cached + refreshed via the same
// path as the rest of the object's metadata — no new cache logic.
type recordTypeValueSource struct {
	parent string
	res    *Resource[[]sf.RecordTypeRow]
}

func (m Model) newRecordTypeValueSource(parentSObject string) *recordTypeValueSource {
	if len(m.orgs) == 0 {
		return nil
	}
	o := m.orgs[m.selected]
	d := m.data[o.Username]
	if d == nil {
		return nil
	}
	// Reuse the existing per-sObject cache — same Resource the
	// RecordTypes subtab uses, so a refresh there benefits the
	// picker and vice versa. EnsureList is idempotent: if the
	// resource is already there, we get the cached pointer; if
	// not, it's allocated. The Fetch fires lazily when something
	// actually calls Ensure(), which the picker invocation does
	// via the wizard's command.
	res := d.EnsureRecordTypes(targetArg(o), parentSObject)
	return &recordTypeValueSource{parent: parentSObject, res: res}
}

func (r *recordTypeValueSource) Title() string {
	return r.parent + " RecordType · pick"
}

func (r *recordTypeValueSource) Loading() bool {
	if r == nil || r.res == nil {
		return false
	}
	return r.res.Busy()
}

func (r *recordTypeValueSource) Items() []valueOption {
	if r == nil || r.res == nil {
		return nil
	}
	rows := r.res.Value()
	out := make([]valueOption, 0, len(rows))
	for _, rt := range rows {
		opts := valueOption{
			Value: rt.ID,
			Label: rt.Name,
			Hint:  rt.DeveloperName,
		}
		// Active is the default — only flag non-active rows so the
		// hint doesn't clutter every row.
		if !rt.Active {
			opts.Hint += " · inactive"
		}
		out = append(out, opts)
	}
	return out
}

// ---- picker invocation --------------------------------------------

// openValuePicker opens the right anchored dropdown for the focused
// criterion's value field. No-op when the field is free-text /
// numeric / date — those let the user type freely.
//
// For sources backed by a Resource (RecordTypes etc.), we kick the
// resource's Ensure so the cache layer fetches if needed; the picker
// shows a "loading…" flash and the user re-presses enter once the
// fetch lands. No new cache logic — same Resource the rest of the
// app uses.
func (m *Model) openValuePicker() tea.Cmd {
	st := m.chipWizard
	if st == nil || st.Cursor < 0 || st.Cursor >= len(st.criteria) {
		return nil
	}
	criterion := st.criteria[st.Cursor]
	src := m.valueSourceFor(st.Scope, criterion)
	if src == nil {
		// Nothing constrained about this field — fall through to text edit.
		return nil
	}

	// If the source is backed by a Resource we haven't loaded yet,
	// kick the fetch and tell the user to retry. The picker is
	// modal, so we can't both fetch + open at the same time without
	// a "loading" placeholder; v1 keeps it simple — flash + return.
	if rt, ok := src.(*recordTypeValueSource); ok && rt != nil && rt.res != nil {
		if rt.res.FetchedAt().IsZero() && !rt.res.Busy() {
			cmd := rt.res.Ensure(m.cache)
			m.flash("loading record types…")
			return cmd
		}
		if rt.res.Busy() {
			m.flash("loading record types…")
			return nil
		}
	}

	// Anchor the picker like the field picker — under the wizard's
	// criterion row. Same trick as openCriterionFieldPicker.
	wW := modalWidth(m.width, 72, 110)
	wX := (m.width - wW) / 2
	pickerW := wW * 2 / 3
	if pickerW < 48 {
		pickerW = 48
	}
	if pickerW > m.width-4 {
		pickerW = m.width - 4
	}
	anchorX := wX + 4
	anchorY := (m.height / 2) + 2

	if src.Loading() {
		// Show a spinner placeholder until the resource lands. The
		// picker re-opens itself when the user presses enter again
		// after the data arrives.
		m.flash("loading " + src.Title() + "…")
		return nil
	}
	options := src.Items()
	if len(options) == 0 {
		m.flash("no values available for " + criterion.Label)
		return nil
	}

	criterionIdx := st.Cursor
	return openPicker(m, pickerSpec[valueOption]{
		Title:       src.Title(),
		Items:       options,
		Width:       pickerW,
		AnchorX:     anchorX,
		AnchorY:     anchorY,
		Placeholder: "type to filter…",
		Match: func(o valueOption, q string) bool {
			lq := strings.ToLower(q)
			return strings.Contains(strings.ToLower(o.Value), lq) ||
				strings.Contains(strings.ToLower(o.Label), lq) ||
				strings.Contains(strings.ToLower(o.Hint), lq)
		},
		RenderRow: func(o valueOption, focused bool) string {
			label := o.Label
			if label == "" {
				label = o.Value
			}
			line := "  " + label
			if focused {
				line = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " " +
					lipgloss.NewStyle().Foreground(theme.Fg).Bold(true).Render(label)
			}
			if o.Hint != "" {
				line += "  " + lipgloss.NewStyle().Foreground(theme.FgDim).Render(o.Hint)
			}
			return line
		},
		OnPick: func(o valueOption) tea.Cmd {
			return func() tea.Msg {
				return valuePickedMsg{criterionIdx: criterionIdx, value: o.Value}
			}
		},
	})
}

// valuePickedMsg lands on the main loop after the user picks a value.
type valuePickedMsg struct {
	criterionIdx int
	value        string
}

// applyValuePicked writes the picked value into the targeted criterion
// row's textinput (or tristate, in case future sources surface a
// boolean — today only string-valued sources exist).
func (m Model) applyValuePicked(msg valuePickedMsg) (Model, tea.Cmd) {
	st := m.chipWizard
	if st == nil || msg.criterionIdx < 0 || msg.criterionIdx >= len(st.criteria) {
		return m, nil
	}
	cur := &st.criteria[msg.criterionIdx]
	switch cur.Kind {
	case cwText, cwInt, cwDate:
		cur.input.SetValue(msg.value)
		cur.input.CursorEnd()
	}
	return m, nil
}
