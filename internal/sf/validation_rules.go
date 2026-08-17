package sf

import "fmt"

// ValidationRuleRow is one row in the list of a sobject's validation
// rules. Light-weight for list rendering — drill into a row to fetch
// the full Metadata (formula body, error message, etc.).
type ValidationRuleRow struct {
	ID             string
	ValidationName string
	Active         bool
	Description    string
}

// ListValidationRules returns every ValidationRule defined on the
// given sobject. Queries Tooling; read-only.
func ListValidationRules(target, sobject string) ([]ValidationRuleRow, error) {
	return queryRows(target,
		fmt.Sprintf(
			"SELECT Id, ValidationName, Active, Description "+
				"FROM ValidationRule "+
				"WHERE EntityDefinition.QualifiedApiName = '%s' "+
				"ORDER BY ValidationName",
			sqlEscape(sobject)),
		true, mapValidationRuleRow)
}

func mapValidationRuleRow(r map[string]any) ValidationRuleRow {
	row := ValidationRuleRow{
		ID:             asString(r["Id"]),
		ValidationName: asString(r["ValidationName"]),
		Description:    asString(r["Description"]),
	}
	if b, ok := r["Active"].(bool); ok {
		row.Active = b
	}
	return row
}

// ValidationRuleDetail is the full Metadata-unwrapped shape of one
// validation rule, used for the drill-in detail view.
type ValidationRuleDetail struct {
	ID                    string
	ValidationName        string
	Active                bool
	Description           string
	ErrorMessage          string
	ErrorDisplayField     string
	ErrorConditionFormula string
}

// GetValidationRule fetches one ValidationRule via Tooling including
// its full Metadata body. Used when the user drills into a row.
func GetValidationRule(target, id string) (ValidationRuleDetail, error) {
	meta, err := GetToolingMetadata(target, "ValidationRule", id)
	if err != nil {
		return ValidationRuleDetail{}, err
	}
	det := ValidationRuleDetail{ID: id}
	if b, ok := meta["active"].(bool); ok {
		det.Active = b
	}
	det.Description = asString(meta["description"])
	det.ErrorMessage = asString(meta["errorMessage"])
	det.ErrorDisplayField = asString(meta["errorDisplayField"])
	det.ErrorConditionFormula = asString(meta["errorConditionFormula"])
	return det, nil
}
