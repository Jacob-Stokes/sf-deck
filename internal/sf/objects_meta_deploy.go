package sf

// Object-level edits via the Metadata API.
//
// Why this file exists: CustomObject is read-only via Tooling (see
// README_API_ROUTING.md). Every user-visible object-level edit —
// label, plural label, description, feature toggles — goes through
// the Metadata API instead.
//
// IMPORTANT invariant: Salesforce validates a CustomObject deploy
// as a complete standalone definition. Required top-level elements
// include <label>, <pluralLabel>, <nameField>, <deploymentStatus>,
// and <sharingModel>. Shipping a partial XML that omits them
// triggers "Must specify a non-empty X" errors even when X is
// already set on the org.
//
// So this file does a round-trip:
//   1. Read current state (describe for label/pluralLabel/nameField;
//      Tooling CustomObject for Description + SharingModel).
//   2. Build a COMPLETE CustomObject XML with those values.
//   3. Overlay the user's patch on top (only fields they changed).
//   4. Deploy.
//
// This costs 1-2 extra API calls per edit but it's the only
// reliable way to deploy a single-field change through the
// Metadata API without managing the full object source locally.

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

	// nameField metadata — identifies the object's primary "Name"
	// field so the deploy XML carries it. Text + "Account Name" is
	// the normal shape; AutoNumber objects have a different sub-tree.
	NameFieldLabel string
	NameFieldType  string // "Text" or "AutoNumber"

	// SharingModel — Tooling reports this per-org, Metadata API
	// requires it on deploy.  One of: "Read", "ReadWrite",
	// "Private", "ControlledByParent".
	SharingModel string

	// Feature toggles — current state. Decoded from the Tooling
	// CustomObject GET (same call we make for Description /
	// SharingModel). When SF doesn't return a flag (e.g. on certain
	// standard objects whose toggle is implicitly true / managed
	// elsewhere) the pointer stays nil and the UI shows "current
	// state unknown."
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

	// Tooling CustomObject carries SharingModel + Description on
	// the sobject row itself. Read them directly.
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
	// CustomObject record exposes only Description + SharingModel.
	// The metadata-level feature toggles (enableReports etc.) are
	// NOT on this endpoint — they live on EntityDefinition under
	// Is*-prefixed columns (mapped in fetchEntityDefinitionToggles).
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
	// Layer in the toggle flags from a SOQL against EntityDefinition.
	// Best-effort — failures here just leave the *bool fields nil
	// (UI surfaces as "current state unknown" rather than blocking
	// the rest of the baseline.)
	if toggles, terr := fetchEntityDefinitionToggles(c, apiName); terr == nil {
		base.EnableReports = toggles.EnableReports
		base.EnableActivities = toggles.EnableActivities
		base.EnableHistory = toggles.EnableHistory
		base.EnableFeeds = toggles.EnableFeeds
		base.EnableSearch = toggles.EnableSearch
	}
	// Tooling reports SharingModel as "Edit" / "Read" / "Private" /
	// "ControlledByParent"; Metadata API wants "ReadWrite" where
	// Tooling says "Edit". Translate.
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

// objectToggles is the subset of EntityDefinition columns we map
// onto the CustomObject metadata enable* flags. SF's column
// naming is non-obvious — verified empirically (see the
// describe-EntityDefinition probe history) that these five
// columns correspond to the five metadata flags one-to-one.
type objectToggles struct {
	EnableReports    *bool
	EnableActivities *bool
	EnableHistory    *bool
	EnableFeeds      *bool
	EnableSearch     *bool
}

// fetchEntityDefinitionToggles runs a single Tooling SOQL against
// EntityDefinition for the metadata-level feature flags. Mapping:
//
//	enableReports     → IsReportingEnabled
//	enableActivities  → IsActivityTrackable
//	enableHistory     → IsFieldHistoryTracked
//	enableFeeds       → IsFeedEnabled
//	enableSearch      → IsSearchable
//
// EntityDefinition is queryable for both standard and custom
// objects, so this works on Account / Contact / etc. as well as
// __c objects — unlike the CustomObject endpoint which is
// custom-only.
//
// All fields come back as plain bool from EntityDefinition (no
// nullability indicator), but we expose them as *bool so the
// caller can distinguish "Salesforce gave us a value" from
// "we didn't fetch / it errored." Set to a non-nil pointer for
// every successful row.
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

// boolPtr returns &b for value b. Used so we can put plain bool
// query results behind *bool baseline fields (nil = unknown).
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

// applyPatch overwrites baseline fields with patch values where the
// patch has them set. String patches use empty-means-skip; pointer
// bool patches are left to the XML builder to emit separately.
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

// buildCustomObjectXML emits a complete CustomObject XML. Required
// elements (label, pluralLabel, nameField, deploymentStatus,
// sharingModel) come from the baseline + patch overlay; feature-
// toggle bools come from the patch directly (only emitted when set).
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

	// Feature toggles — only the ones the user set.
	writeBool("enableReports", p.EnableReports)
	writeBool("enableActivities", p.EnableActivities)
	writeBool("enableHistory", p.EnableHistory)
	writeBool("enableFeeds", p.EnableFeeds)
	writeBool("enableSearch", p.EnableSearch)

	// Required identity.
	writeStr("label", b.Label)
	writeStr("pluralLabel", b.PluralLabel)
	writeStr("description", b.Description)

	// Required nameField sub-tree.
	sb.WriteString("  <nameField>\n")
	sb.WriteString("    <label>")
	sb.WriteString(xmlEscape(b.NameFieldLabel))
	sb.WriteString("</label>\n")
	sb.WriteString("    <type>")
	sb.WriteString(xmlEscape(b.NameFieldType))
	sb.WriteString("</type>\n")
	sb.WriteString("  </nameField>\n")

	// Required scope.
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
