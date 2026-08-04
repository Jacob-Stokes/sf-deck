package sf

// Report Types — the catalogue a new report starts from, fetched via
// the analytics REST endpoint (GET /analytics/reportTypes). Returns
// categories each holding a list of types; flattened here to one row
// per type for the list surface.

import "encoding/json"

// ReportTypeRow is one report type in the flattened catalogue.
type ReportTypeRow struct {
	Category       string
	Label          string
	Type           string // API name, e.g. AccountList or CustomEntity__c
	Description    string
	Custom         bool
	SupportsJoined bool
}

// Field implements query.Row for chip predicates.
func (r ReportTypeRow) Field(name string) (any, bool) {
	switch name {
	case "Category":
		return r.Category, true
	case "Label", "Name":
		return r.Label, true
	case "Type":
		return r.Type, true
	case "Description":
		return r.Description, true
	case "Custom", "IsCustom":
		return r.Custom, true
	case "SupportsJoined":
		return r.SupportsJoined, true
	}
	return nil, false
}

// Targets implements Openable. Custom report types manage from Setup;
// standard types have no useful destination beyond the same page.
func (r ReportTypeRow) Targets() []OpenTarget {
	return []OpenTarget{
		{ID: "report_types", Label: "Report Types · Setup",
			Path: "/lightning/setup/CustomReportTypes/home"},
	}
}

type reportTypeCategory struct {
	Label       string `json:"label"`
	ReportTypes []struct {
		Label          string `json:"label"`
		Type           string `json:"type"`
		Description    string `json:"description"`
		IsCustom       bool   `json:"isCustomReportType"`
		IsHidden       bool   `json:"isHidden"`
		SupportsJoined bool   `json:"supportsJoinedFormat"`
	} `json:"reportTypes"`
}

// ListReportTypes fetches + flattens the report-type catalogue.
// Hidden types are dropped — they're not offerable in the report
// builder either.
func ListReportTypes(target string) ([]ReportTypeRow, error) {
	c, err := RESTClient(target)
	if err != nil {
		return nil, err
	}
	body, err := c.get(c.APIPath("analytics/reportTypes"), nil)
	if err != nil {
		return nil, upgradeToSFError(err)
	}
	var cats []reportTypeCategory
	if err := json.Unmarshal(body, &cats); err != nil {
		return nil, err
	}
	var out []ReportTypeRow
	for _, cat := range cats {
		for _, t := range cat.ReportTypes {
			if t.IsHidden {
				continue
			}
			out = append(out, ReportTypeRow{
				Category:       cat.Label,
				Label:          t.Label,
				Type:           t.Type,
				Description:    t.Description,
				Custom:         t.IsCustom,
				SupportsJoined: t.SupportsJoined,
			})
		}
	}
	return out, nil
}
