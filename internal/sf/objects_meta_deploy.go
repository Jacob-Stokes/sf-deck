package sf

// Object-level edits via the Metadata API.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// CustomObjectPatch is the per-field delta the caller wants applied.
// String fields empty = unchanged. Pointer-to-bool nil = unchanged,
// else set to the dereferenced value.
type CustomObjectPatch struct {
	Label       string
	PluralLabel string
	Description string

	EnableReports    *bool
	EnableActivities *bool
	EnableHistory    *bool
	EnableFeeds      *bool
	EnableSearch     *bool
}

// HasChanges returns true when at least one field has a value to
// emit. Callers should short-circuit no-op deploys.
func (p CustomObjectPatch) HasChanges() bool {
	return p.Label != "" || p.PluralLabel != "" || p.Description != "" ||
		p.EnableReports != nil || p.EnableActivities != nil ||
		p.EnableHistory != nil || p.EnableFeeds != nil || p.EnableSearch != nil
}

// CustomObjectBaseline is the current on-org state we need to
// produce a complete deploy XML. Populated by FetchCustomObjectBaseline.
// Exported so callers (UI preview flow) can fetch it once, show a
// diff, then hand it back to DeployCustomObjectPatchWithBaseline —
// avoiding a double round-trip between preview and commit.
type CustomObjectBaseline struct {
	Label       string // from describe
	PluralLabel string // from describe
	Description string // from Tooling CustomObject row

	NameFieldLabel string
	NameFieldType  string // "Text" or "AutoNumber"

	// SharingModel — Tooling reports this per-org, Metadata API
	// requires it on deploy.  One of: "Read", "ReadWrite",
	// "Private", "ControlledByParent".
	SharingModel string

	EnableReports    *bool
	EnableActivities *bool
	EnableHistory    *bool
	EnableFeeds      *bool
	EnableSearch     *bool
}

// FetchCustomObjectBaseline reads the minimum fields we need to
// construct a complete CustomObject deploy XML.  Two API calls:
// describe (for label/plural/nameField) and a Tooling GET (for
// Description + SharingModel + the enable* feature toggles — none
// of which the standard describe exposes).
//
// Toggle fields stay as *bool so we can distinguish "currently
// disabled" (false) from "Salesforce didn't return a value"
// (nil). The latter shows up on certain standard objects where
// the toggle is implicit; UI surfaces it as "current state
// unknown" rather than guessing.
func FetchCustomObjectBaseline(target, apiName string) (*CustomObjectBaseline, error) {
	d, err := Describe(target, apiName)
	if err != nil {
		return nil, fmt.Errorf("describe %s: %w", apiName, err)
	}
	return FetchCustomObjectBaselineWithDescribe(target, apiName, d)
}

// FetchCustomObjectBaselineWithDescribe is the no-extra-round-trip
// variant — callers that already have the describe in hand (e.g. the
// UI's EnsureDescribe Resource) pass it in so we don't fire a second
// `sf.Describe` for the same sobject. Identical output to
// FetchCustomObjectBaseline; the describe argument is the only
// difference. Saves one REST call on every object drill-in (visible
// in the api-trace JSONL as duplicate Request__c/describe rows).
func FetchCustomObjectBaselineWithDescribe(target, apiName string, d SObjectDescribe) (*CustomObjectBaseline, error) {
	base := &CustomObjectBaseline{
		Label:       d.Label,
		PluralLabel: d.LabelPlural,
	}
	for _, f := range d.Fields {
		if !f.NameField {
			continue
		}
		base.NameFieldLabel = f.Label
		if f.AutoNumber {
			base.NameFieldType = "AutoNumber"
		} else {
			base.NameFieldType = "Text"
		}
		break
	}
	if base.NameFieldLabel == "" {
		base.NameFieldLabel = d.Label + " Name"
		base.NameFieldType = "Text"
	}

	id, err := CustomObjectID(target, apiName)
	if err != nil {
		return nil, fmt.Errorf("lookup CustomObject id: %w", err)
	}
	c, err := RESTClient(target)
	if err != nil {
		return nil, err
	}
	raw, err := c.get(c.ToolingPath("sobjects/CustomObject/"+id), nil)
	if err != nil {
		return nil, fmt.Errorf("fetch CustomObject: %w", upgradeToSFError(err))
	}
	var row struct {
		Description  *string `json:"Description"`
		SharingModel string  `json:"SharingModel"`
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		return nil, fmt.Errorf("decode CustomObject row: %w", err)
	}
	if row.Description != nil {
		base.Description = *row.Description
	}
	if toggles, terr := fetchEntityDefinitionToggles(c, apiName); terr == nil {
		base.EnableReports = toggles.EnableReports
		base.EnableActivities = toggles.EnableActivities
		base.EnableHistory = toggles.EnableHistory
		base.EnableFeeds = toggles.EnableFeeds
		base.EnableSearch = toggles.EnableSearch
	}
	switch row.SharingModel {
	case "Edit":
		base.SharingModel = "ReadWrite"
	case "":
		base.SharingModel = "ReadWrite"
	default:
		base.SharingModel = row.SharingModel
	}
	return base, nil
}

type objectToggles struct {
	EnableReports    *bool
	EnableActivities *bool
	EnableHistory    *bool
	EnableFeeds      *bool
	EnableSearch     *bool
}

func fetchEntityDefinitionToggles(c *Client, apiName string) (*objectToggles, error) {
	q := "SELECT IsReportingEnabled, IsActivityTrackable, IsFieldHistoryTracked, IsFeedEnabled, IsSearchable " +
		"FROM EntityDefinition WHERE QualifiedApiName='" + sqlEscape(apiName) + "'"
	raw, err := c.get(c.ToolingPath("query?q="+url.QueryEscape(q)), nil)
	if err != nil {
		return nil, fmt.Errorf("query EntityDefinition: %w", upgradeToSFError(err))
	}
	var resp struct {
		Records []struct {
			IsReportingEnabled    bool `json:"IsReportingEnabled"`
			IsActivityTrackable   bool `json:"IsActivityTrackable"`
			IsFieldHistoryTracked bool `json:"IsFieldHistoryTracked"`
			IsFeedEnabled         bool `json:"IsFeedEnabled"`
			IsSearchable          bool `json:"IsSearchable"`
		} `json:"records"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode EntityDefinition: %w", err)
	}
	if len(resp.Records) == 0 {
		return nil, fmt.Errorf("EntityDefinition: no row for %s", apiName)
	}
	r := resp.Records[0]
	return &objectToggles{
		EnableReports:    boolPtr(r.IsReportingEnabled),
		EnableActivities: boolPtr(r.IsActivityTrackable),
		EnableHistory:    boolPtr(r.IsFieldHistoryTracked),
		EnableFeeds:      boolPtr(r.IsFeedEnabled),
		EnableSearch:     boolPtr(r.IsSearchable),
	}, nil
}

func boolPtr(b bool) *bool { return &b }

// DeployCustomObjectPatch is the one-shot entry point for callers
// that don't need the preview step: fetches the baseline, overlays
// the patch, builds the complete XML, deploys, returns result.
func DeployCustomObjectPatch(target, apiName string, patch CustomObjectPatch) (*DeployResult, error) {
	if !patch.HasChanges() {
		return &DeployResult{Success: true, Status: "NoOp"}, nil
	}
	base, err := FetchCustomObjectBaseline(target, apiName)
	if err != nil {
		return nil, err
	}
	return DeployCustomObjectPatchWithBaseline(target, apiName, patch, base)
}

// DeployCustomObjectPatchWithBaseline is the entry point the preview
// flow uses: it takes a pre-fetched baseline (returned by
// FetchCustomObjectBaseline), overlays the patch, and deploys — no
// duplicate round-trip between preview and commit.
func DeployCustomObjectPatchWithBaseline(target, apiName string, patch CustomObjectPatch, base *CustomObjectBaseline) (*DeployResult, error) {
	if !patch.HasChanges() {
		return &DeployResult{Success: true, Status: "NoOp"}, nil
	}
	if base == nil {
		return nil, fmt.Errorf("nil baseline — call DeployCustomObjectPatch if you don't have one")
	}
	applyPatch(base, patch)

	xml := buildCustomObjectXML(base, patch)
	members := []PackageMember{
		{Type: "CustomObject", Members: []string{apiName}},
	}
	files := []MetadataFile{
		{Path: "objects/" + apiName + ".object", Body: []byte(xml)},
	}
	return DeployMetadata(target, "", members, files)
}

func applyPatch(b *CustomObjectBaseline, p CustomObjectPatch) {
	if p.Label != "" {
		b.Label = p.Label
	}
	if p.PluralLabel != "" {
		b.PluralLabel = p.PluralLabel
	}
	if p.Description != "" {
		b.Description = p.Description
	}
}

func buildCustomObjectXML(b *CustomObjectBaseline, p CustomObjectPatch) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n")
	sb.WriteString(`<CustomObject xmlns="http://soap.sforce.com/2006/04/metadata">`)
	sb.WriteString("\n")

	writeStr := func(tag, val string) {
		if val == "" {
			return
		}
		sb.WriteString("  <")
		sb.WriteString(tag)
		sb.WriteString(">")
		sb.WriteString(xmlEscape(val))
		sb.WriteString("</")
		sb.WriteString(tag)
		sb.WriteString(">\n")
	}
	writeBool := func(tag string, val *bool) {
		if val == nil {
			return
		}
		sb.WriteString("  <")
		sb.WriteString(tag)
		sb.WriteString(">")
		if *val {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
		sb.WriteString("</")
		sb.WriteString(tag)
		sb.WriteString(">\n")
	}

	writeBool("enableReports", p.EnableReports)
	writeBool("enableActivities", p.EnableActivities)
	writeBool("enableHistory", p.EnableHistory)
	writeBool("enableFeeds", p.EnableFeeds)
	writeBool("enableSearch", p.EnableSearch)

	writeStr("label", b.Label)
	writeStr("pluralLabel", b.PluralLabel)
	writeStr("description", b.Description)

	sb.WriteString("  <nameField>\n")
	sb.WriteString("    <label>")
	sb.WriteString(xmlEscape(b.NameFieldLabel))
	sb.WriteString("</label>\n")
	sb.WriteString("    <type>")
	sb.WriteString(xmlEscape(b.NameFieldType))
	sb.WriteString("</type>\n")
	sb.WriteString("  </nameField>\n")

	sb.WriteString("  <deploymentStatus>Deployed</deploymentStatus>\n")
	writeStr("sharingModel", b.SharingModel)

	sb.WriteString(`</CustomObject>`)
	sb.WriteString("\n")
	return sb.String()
}

// BoolPtr is a convenience for building CustomObjectPatch's pointer-
// to-bool fields. `sf.BoolPtr(true)` is cleaner at call sites than
// taking the address of a literal.
func BoolPtr(v bool) *bool { return &v }
