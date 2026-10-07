package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

const fieldValuesYankTargetID = "__field_values_submenu__"

type fieldValueYankOption struct {
	Label string
	Value string
}

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
