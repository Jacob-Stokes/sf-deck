package sf

// Record Type Tooling-API helpers — list + read + update. Same shape
// as validation_rules.go: a light list row + a deeper Metadata detail
// + a named wrapper over UpdateToolingMetadata.

import "fmt"

// RecordTypeRow is one row in the record-type list. Lightweight for
// rendering; drill in (GetRecordType) for the full Metadata.
type RecordTypeRow struct {
	ID            string
	Name          string
	DeveloperName string
	Active        bool
	Description   string
}

// ListRecordTypes returns every RecordType defined on the given
// sobject. Uses the regular REST API (not Tooling) — the Tooling
// variant of the RecordType entity doesn't expose DeveloperName, but
// the regular sobject does. Read-only.
func ListRecordTypes(target, sobject string) ([]RecordTypeRow, error) {
	return queryRows(target,
		fmt.Sprintf(
			"SELECT Id, Name, DeveloperName, IsActive, Description "+
				"FROM RecordType "+
				"WHERE SobjectType = '%s' "+
				"ORDER BY DeveloperName",
			sqlEscape(sobject)),
		false, mapRecordTypeRow)
}

func mapRecordTypeRow(r map[string]any) RecordTypeRow {
	row := RecordTypeRow{
		ID:            asString(r["Id"]),
		Name:          asString(r["Name"]),
		DeveloperName: asString(r["DeveloperName"]),
		Description:   asString(r["Description"]),
	}
	if b, ok := r["IsActive"].(bool); ok {
		row.Active = b
	}
	return row
}

// RecordTypeDetail unwraps the full Metadata body of a record type.
type RecordTypeDetail struct {
	ID              string
	Active          bool
	Label           string
	Description     string
	BusinessProcess string
}

// GetRecordType fetches one RecordType's full Metadata via Tooling.
func GetRecordType(target, id string) (RecordTypeDetail, error) {
	meta, err := GetToolingMetadata(target, "RecordType", id)
	if err != nil {
		return RecordTypeDetail{}, err
	}
	det := RecordTypeDetail{ID: id}
	if b, ok := meta["active"].(bool); ok {
		det.Active = b
	}
	det.Label = asString(meta["label"])
	det.Description = asString(meta["description"])
	det.BusinessProcess = asString(meta["businessProcess"])
	return det, nil
}
