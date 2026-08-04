package ui

// Field-value yank sub-modal — the "Field values…" entry in a field's
// ctrl+y menu opens a second picker of everything worth copying OUT of
// a field's definition: picklist values (four formats), the formula,
// the default, help text, and reference targets. Kept as a sub-modal so
// the top-level yank menu stays short (API name / Object.Field / Label)
// rather than sprouting a dozen field-specific rows.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// fieldValuesYankTargetID marks the synthetic "Field values…" yank
// target that opens the sub-modal (no YankValue — fireMenuTarget
// intercepts it). UI-side since the sub-modal is a UI concern.
const fieldValuesYankTargetID = "__field_values_submenu__"

// fieldValueYankOption is one copyable value with its menu label.
type fieldValueYankOption struct {
	Label string
	Value string
}

// fieldValueYankOptions builds the sub-modal contents for a field —
// only the entries that actually apply (a non-picklist field skips the
// picklist formats, a field with no formula skips FORMULA, etc.). Empty
// slice → the "Field values…" entry isn't offered.
func fieldValueYankOptions(f sf.Field) []fieldValueYankOption {
	var out []fieldValueYankOption

	if len(f.PicklistValues) > 0 {
		values := make([]string, 0, len(f.PicklistValues))
		labels := make([]string, 0, len(f.PicklistValues))
		lines := make([]string, 0, len(f.PicklistValues))
		var table strings.Builder
		table.WriteString("Label\tValue\tActive\tDefault\n")
		for _, pv := range f.PicklistValues {
			values = append(values, pv.Value)
			labels = append(labels, pv.Label)
			lines = append(lines, pv.Value)
			table.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\n",
				pv.Label, pv.Value, yesNo(pv.Active), yesNo(pv.DefaultValue)))
		}
		out = append(out,
			fieldValueYankOption{"Picklist values (comma)", strings.Join(values, ",")},
			fieldValueYankOption{"Picklist labels (comma)", strings.Join(labels, ",")},
			fieldValueYankOption{"Picklist values (newline)", strings.Join(lines, "\n")},
			fieldValueYankOption{"Picklist table (Label/Value/Active/Default)", strings.TrimRight(table.String(), "\n")},
		)
	}

	if f.CalculatedFormula != "" {
		out = append(out, fieldValueYankOption{"Formula", f.CalculatedFormula})
	}
	if dv := fieldDefaultValueString(f); dv != "" {
		out = append(out, fieldValueYankOption{"Default value", dv})
	}
	if f.InlineHelpText != "" {
		out = append(out, fieldValueYankOption{"Help text", f.InlineHelpText})
	}
	if len(f.ReferenceTo) > 0 {
		out = append(out, fieldValueYankOption{"Reference target(s)", strings.Join(f.ReferenceTo, ", ")})
	}
	return out
}

// fieldDefaultValueString renders a field's default (a formula default
// takes precedence over a literal). Empty when the field has none.
func fieldDefaultValueString(f sf.Field) string {
	if f.DefaultValueFormula != "" {
		return f.DefaultValueFormula
	}
	if f.DefaultValue != nil {
		if s := fmt.Sprintf("%v", f.DefaultValue); s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

// openFieldValuesYankModal opens the sub-modal listing every copyable
// value for a field; selecting one copies it to the clipboard.
func (m *Model) openFieldValuesYankModal(f sf.Field) tea.Cmd {
	opts := fieldValueYankOptions(f)
	if len(opts) == 0 {
		m.flash("no field values to copy")
		return nil
	}
	options := make([]choiceOption, 0, len(opts)+1)
	for _, o := range opts {
		preview := o.Value
		if len(preview) > 40 || strings.ContainsRune(preview, '\n') || strings.ContainsRune(preview, '\t') {
			preview = fmt.Sprintf("%d chars", len(o.Value))
		}
		options = append(options, choiceOption{
			Label: o.Label,
			Hint:  preview,
			Value: o.Value,
		})
	}
	options = append(options, choiceOption{Label: "Cancel", Cancel: true})

	return m.openChoiceModal(choiceModalState{
		Title:      "Copy field value",
		Hint:       "Enter to copy · Esc to cancel",
		Options:    options,
		Cursor:     0,
		Searchable: len(options) > 8,
		Save:       func(any) error { return nil },
		OnSuccessTyped: func(val any) tea.Cmd {
			s, _ := val.(string)
			if s == "" {
				return nil
			}
			label := "value"
			if len(s) <= 60 && !strings.ContainsRune(s, '\n') {
				label = s
			} else {
				label = fmt.Sprintf("%d chars", len(s))
			}
			m.flash("copied: " + label)
			return yankValueCmd(s)
		},
	})
}
