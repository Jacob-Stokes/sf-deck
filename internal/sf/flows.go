package sf

// Flow is the "header" row for a flow in the browser. One Flow per
// FlowDefinition — we aggregate the active-version metadata in so the
// list has useful info without a second round-trip per row.
type Flow struct {
	DefinitionID     string `json:"defId"`
	DeveloperName    string `json:"devName"`
	MasterLabel      string `json:"label"`
	Description      string `json:"desc"`
	Namespace        string `json:"ns,omitempty"`
	ActiveVersionID  string `json:"activeId,omitempty"`
	LatestVersionID  string `json:"latestId,omitempty"`
	ActiveVersionNum int    `json:"activeN,omitempty"`
	LatestVersionNum int    `json:"latestN,omitempty"`
	ProcessType      string `json:"ptype,omitempty"`
	APIVersion       int    `json:"api,omitempty"`
	Status           string `json:"status,omitempty"` // Active / Draft / Obsolete / InvalidDraft
	// LatestVersionStatus is the status of the LATEST version, which
	// differs from Status (the active version's) when a newer version
	// exists — e.g. active v3 with a v4 Draft. Drives the status-
	// accurate "(v4) = newer Draft" footer hint. Empty when the latest
	// version wasn't fetched or equals the active one.
	LatestVersionStatus string `json:"latestStatus,omitempty"`
	LastModifiedDate    string `json:"mod,omitempty"`
	LastModifiedBy      string `json:"modBy,omitempty"`
	// CreatedDate / CreatedBy come from the active (or latest) Flow
	// version row — same source as LastModified*. We always fetched
	// them in the version SOQL but discarded them until chips on
	// "Mine" / "Created by X" called for filtering on author.
	CreatedDate string `json:"crt,omitempty"`
	CreatedBy   string `json:"crtBy,omitempty"`
}

// Field implements the structural query.Row interface. Column names
// match what `FROM Flow` would expose in SOQL so an imported list
// view evaluates against the same identifiers — with the convenience
// alias "Status" mapping to the active version's status (which is
// what the list shows).
func (f Flow) Field(name string) (any, bool) {
	switch name {
	case "Id", "DefinitionId":
		return f.DefinitionID, true
	case "DeveloperName":
		return f.DeveloperName, true
	case "MasterLabel", "Label":
		return f.MasterLabel, true
	case "Description":
		return f.Description, true
	case "NamespacePrefix", "Namespace":
		return f.Namespace, true
	case "ActiveVersionId":
		return f.ActiveVersionID, true
	case "LatestVersionId":
		return f.LatestVersionID, true
	case "ProcessType":
		return f.ProcessType, true
	case "ApiVersion", "APIVersion":
		return f.APIVersion, true
	case "Status":
		return f.Status, true
	case "LastModifiedDate":
		return f.LastModifiedDate, true
	case "LastModifiedBy", "LastModifiedBy.Name", "LastModifiedByName":
		return f.LastModifiedBy, true
	case "CreatedDate":
		return f.CreatedDate, true
	case "CreatedBy", "CreatedBy.Name", "CreatedByName":
		return f.CreatedBy, true
	}
	return nil, false
}

// FlowVersion is one row per historical Flow-version record.
type FlowVersion struct {
	ID               string `json:"id"`
	DefinitionID     string `json:"defId"`
	VersionNumber    int    `json:"n"`
	MasterLabel      string `json:"label"`
	Description      string `json:"desc,omitempty"`
	ProcessType      string `json:"ptype,omitempty"`
	APIVersion       int    `json:"api,omitempty"`
	Status           string `json:"status,omitempty"`
	CreatedDate      string `json:"c,omitempty"`
	CreatedBy        string `json:"cby,omitempty"`
	LastModifiedDate string `json:"m,omitempty"`
	LastModifiedBy   string `json:"mby,omitempty"`
}

// ListFlows returns one Flow per FlowDefinition, joined with the
// active-version's process type / API version / status via a second
// query against the Flow object. All read-only Tooling-API SOQL.
func ListFlows(orgAlias string) ([]Flow, error) {
	// Page FlowDefinition by Id so we don't trip the 2000-row queryMore
	// limit on Tooling-API views. In practice orgs rarely have more than
	// a couple thousand flows, but the code handles it cleanly regardless.
	defs, err := listFlowDefinitions(orgAlias)
	if err != nil {
		return nil, err
	}
	if len(defs) == 0 {
		return nil, nil
	}

	// Fetch the "active or latest" version for each definition so we can
	// show ProcessType, APIVersion, Status, VersionNumber, LastModified
	// without a round-trip per row.
	byID := map[string]*Flow{}
	versionIDs := make([]string, 0, len(defs)*2)
	for i := range defs {
		f := &defs[i]
		byID[f.DefinitionID] = f
		if f.ActiveVersionID != "" {
			versionIDs = append(versionIDs, f.ActiveVersionID)
		}
		if f.LatestVersionID != "" && f.LatestVersionID != f.ActiveVersionID {
			versionIDs = append(versionIDs, f.LatestVersionID)
		}
	}
	// fetchFlowVersions returns whatever it managed to collect even on
	// error — earlier versions of this code dropped partial results
	// when ANY chunk failed, leaving the entire flow list with no
	// version metadata. Apply what we got and just log the error away
	// (best-effort: the user sees most rows populated rather than
	// none).
	versions, _ := fetchFlowVersions(orgAlias, versionIDs)
	vByID := map[string]FlowVersion{}
	for _, v := range versions {
		vByID[v.ID] = v
	}
	for i := range defs {
		f := &defs[i]
		// Prefer the active version's metadata; fall back to the latest.
		v, ok := vByID[f.ActiveVersionID]
		if !ok {
			v = vByID[f.LatestVersionID]
		}
		if v.ID != "" {
			f.ProcessType = v.ProcessType
			f.APIVersion = v.APIVersion
			f.Status = v.Status
			// CreatedDate/CreatedBy intentionally NOT taken from the
			// version — they're the flow's original author from the
			// FlowDefinition (seeded in listFlowDefinitions). The
			// version's creator is whoever saved that version, which is
			// a different person for any flow edited after creation.
			if f.Description == "" {
				f.Description = v.Description
			}
		}
		// Reconcile "last modified" to the NEWEST of the definition
		// (seeded above), the active version, and the latest version.
		// A draft saved after the active version — or an activation
		// change that touched only the definition — must still surface
		// as the flow's modified date, matching Salesforce's Flows
		// setup list. Preferring the active version alone (the old
		// behaviour) made such flows look stale and sort too low.
		adoptNewerModified(f, vByID[f.ActiveVersionID])
		adoptNewerModified(f, vByID[f.LatestVersionID])
		if av, ok := vByID[f.ActiveVersionID]; ok {
			f.ActiveVersionNum = av.VersionNumber
		}
		if lv, ok := vByID[f.LatestVersionID]; ok {
			f.LatestVersionNum = lv.VersionNumber
			f.LatestVersionStatus = lv.Status
		}
		if f.ActiveVersionID == "" {
			f.Status = "Inactive"
		}
	}
	return defs, nil
}

func listFlowDefinitions(orgAlias string) ([]Flow, error) {
	// QueryREST follows nextRecordsUrl so a single query returns every
	// row regardless of Tooling's 500-per-response cap.
	// FlowDefinition.LastModifiedDate updates on activation changes and
	// on saves that don't bump the active version (e.g. a new draft) —
	// the same "Last Modified" Salesforce's Flows setup list shows. We
	// pull it here and later reconcile it against the version rows so a
	// draft saved after the active version still floats the flow to the
	// top by modified date (see the reconciliation in ListFlows).
	// CreatedDate/CreatedBy come from FlowDefinition — the flow's
	// original author, matching what Salesforce's Flows setup list
	// shows. This must NOT come from a Flow *version*: whoever saved
	// the active/latest version (often a later editor) is not the
	// flow's creator, and using the version made "Created by" — and
	// the "Created by me" lens — attribute the flow to the wrong user.
	soql := "SELECT Id, DeveloperName, MasterLabel, NamespacePrefix, " +
		"ActiveVersionId, LatestVersionId, Description, " +
		"CreatedDate, CreatedBy.Name, " +
		"LastModifiedDate, LastModifiedBy.Name FROM FlowDefinition " +
		"ORDER BY Id"
	q, err := Query(orgAlias, soql, true)
	if err != nil {
		return nil, err
	}
	out := make([]Flow, 0, len(q.Records))
	for _, r := range q.Records {
		f := Flow{
			DefinitionID:    asString(r["Id"]),
			DeveloperName:   asString(r["DeveloperName"]),
			MasterLabel:     asString(r["MasterLabel"]),
			Description:     asString(r["Description"]),
			Namespace:       asString(r["NamespacePrefix"]),
			ActiveVersionID: asString(r["ActiveVersionId"]),
			LatestVersionID: asString(r["LatestVersionId"]),
			// Seed modified* from the definition; ListFlows overwrites
			// with the newest of {definition, active, latest version}.
			LastModifiedDate: asString(r["LastModifiedDate"]),
			// Created* come from the definition and are authoritative —
			// ListFlows must not overwrite them from a version row.
			CreatedDate: asString(r["CreatedDate"]),
		}
		if u, ok := r["LastModifiedBy"].(map[string]any); ok {
			f.LastModifiedBy = asString(u["Name"])
		}
		if u, ok := r["CreatedBy"].(map[string]any); ok {
			f.CreatedBy = asString(u["Name"])
		}
		out = append(out, f)
	}
	return out, nil
}

// adoptNewerModified sets f's LastModifiedDate/By to v's when v is
// strictly newer than what f already holds. All three timestamp
// sources (FlowDefinition, active Flow version, latest Flow version)
// come back from the Tooling API in the same ISO-8601 UTC format
// (2026-07-07T11:36:08.000+0000), so a lexicographic compare orders
// them chronologically — no parse needed. Empty candidate is ignored.
func adoptNewerModified(f *Flow, v FlowVersion) {
	if v.ID == "" || v.LastModifiedDate == "" {
		return
	}
	if v.LastModifiedDate > f.LastModifiedDate {
		f.LastModifiedDate = v.LastModifiedDate
		f.LastModifiedBy = v.LastModifiedBy
	}
}

// fetchFlowVersions fetches per-version rows for every id in the batch,
// using SOQL IN (..). The SOQL itself is capped only by the parser
// (~100K chars), but Salesforce REST queries go through GET with the
// SOQL in the URL query string — and the URL is capped at ~16K chars
// in practice. Each quoted-and-comma-separated 18-char Id costs 21
// chars, so 400 ids → ~8.4K chars of IN list, plus ~250 chars for the
// SELECT/WHERE = ~9K total URL. Comfortably under the limit.
//
// Bumping to 800 made some orgs return 0 rows per chunk (URL exceeded
// the limit, query rejected, partial results dropped on the caller
// side because of the legacy "best effort, return defs unchanged on
// error" path — see ListFlows).
const flowVersionChunk = 400

func fetchFlowVersions(orgAlias string, ids []string) ([]FlowVersion, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	chunk := flowVersionChunk
	var out []FlowVersion
	for start := 0; start < len(ids); start += chunk {
		end := start + chunk
		if end > len(ids) {
			end = len(ids)
		}
		in := soqlIDList(ids[start:end])
		soql := "SELECT Id, DefinitionId, MasterLabel, Description, ApiVersion, Status, VersionNumber, ProcessType, CreatedDate, CreatedBy.Name, LastModifiedDate, LastModifiedBy.Name FROM Flow WHERE Id IN (" + in + ")"
		q, err := Query(orgAlias, soql, true)
		if err != nil {
			return out, err
		}
		for _, r := range q.Records {
			v := FlowVersion{
				ID:               asString(r["Id"]),
				DefinitionID:     asString(r["DefinitionId"]),
				MasterLabel:      asString(r["MasterLabel"]),
				Description:      asString(r["Description"]),
				APIVersion:       asInt(r["ApiVersion"]),
				Status:           asString(r["Status"]),
				VersionNumber:    asInt(r["VersionNumber"]),
				ProcessType:      asString(r["ProcessType"]),
				CreatedDate:      asString(r["CreatedDate"]),
				LastModifiedDate: asString(r["LastModifiedDate"]),
			}
			if u, ok := r["CreatedBy"].(map[string]any); ok {
				v.CreatedBy = asString(u["Name"])
			}
			if u, ok := r["LastModifiedBy"].(map[string]any); ok {
				v.LastModifiedBy = asString(u["Name"])
			}
			out = append(out, v)
		}
	}
	return out, nil
}

// FlowVersions returns every version record for a single FlowDefinition,
// ordered newest first. Read-only.
func FlowVersions(orgAlias, definitionID string) ([]FlowVersion, error) {
	soql := "SELECT Id, DefinitionId, MasterLabel, Description, ApiVersion, Status, VersionNumber, ProcessType, CreatedDate, CreatedBy.Name, LastModifiedDate, LastModifiedBy.Name FROM Flow WHERE DefinitionId = '" + sqlEscape(definitionID) + "' ORDER BY VersionNumber DESC"
	q, err := Query(orgAlias, soql, true)
	if err != nil {
		return nil, err
	}
	var out []FlowVersion
	for _, r := range q.Records {
		v := FlowVersion{
			ID:               asString(r["Id"]),
			DefinitionID:     asString(r["DefinitionId"]),
			MasterLabel:      asString(r["MasterLabel"]),
			Description:      asString(r["Description"]),
			APIVersion:       asInt(r["ApiVersion"]),
			Status:           asString(r["Status"]),
			VersionNumber:    asInt(r["VersionNumber"]),
			ProcessType:      asString(r["ProcessType"]),
			CreatedDate:      asString(r["CreatedDate"]),
			LastModifiedDate: asString(r["LastModifiedDate"]),
		}
		if u, ok := r["CreatedBy"].(map[string]any); ok {
			v.CreatedBy = asString(u["Name"])
		}
		if u, ok := r["LastModifiedBy"].(map[string]any); ok {
			v.LastModifiedBy = asString(u["Name"])
		}
		out = append(out, v)
	}
	return out, nil
}

// RenameFlow updates a FlowDefinition's display label (the "Flow
// Label" Salesforce Setup edits). Writes Metadata.masterLabel on the
// FlowDefinition via the generic read-modify-write helper, so the
// existing activeVersionNumber / description are preserved — a rename
// never changes which version is active. The DeveloperName (API name)
// is intentionally left untouched; renaming the label can't break
// references. Safety gating is the caller's job.
func RenameFlow(target, definitionID, newLabel string) error {
	return UpdateToolingMetadata(target, "FlowDefinition", definitionID, map[string]any{
		"masterLabel": newLabel,
	})
}

// DeleteFlowVersion permanently deletes one Flow version by its
// Tooling Id. Salesforce refuses to delete the ACTIVE version (the
// caller must block that case first for a clean message — the raw API
// error is opaque). Deleting an inactive version is irreversible.
// Safety gating is the caller's job.
func DeleteFlowVersion(target, versionID string) error {
	return DeleteToolingMetadata(target, "Flow", versionID)
}

// FlowVersionMetadata returns the full definition of one Flow version
// (its Tooling `Metadata` object) — every element, decision, assignment,
// etc. as the structured map the Tooling API returns. Read-only; the
// caller renders it (e.g. pretty-printed JSON) for the in-terminal
// version viewer.
func FlowVersionMetadata(target, versionID string) (map[string]any, error) {
	return GetToolingMetadata(target, "Flow", versionID)
}

func soqlIDList(ids []string) string {
	var out string
	for i, id := range ids {
		if i > 0 {
			out += ","
		}
		out += "'" + sqlEscape(id) + "'"
	}
	return out
}
