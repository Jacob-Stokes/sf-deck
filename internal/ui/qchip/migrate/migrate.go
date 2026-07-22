// Package migrate carries the one-time conversion from the
// pre-unified settings sections (lens / object_filters / flow_filters)
// into the unified [[ui.chips]] format.
//
// Carved out of qchip itself so it's deletable in one move once the
// migration window closes — at that point the legacy fields can be
// dropped from settings.UIConfig and this package goes with them.
//
// The Run function is the only public entry point. Caller flow is
// always:
//
//	if migrate.Run(st) > 0 {
//	    st.ClearLegacyChips()
//	    _ = st.Save()
//	}
//
// model.go's New() has the only call site today.
package migrate

import (
	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

// Run scans the legacy lens / object_filters / flow_filters sections
// of `s` and converts every entry into a unified ChipConfig with the
// matching domain. Idempotent: a domain that already has at least one
// chip is left alone, so re-running the migration is a no-op.
//
// Returns the number of entries migrated this call. Zero when the
// settings file is already on the unified format.
//
// Conversion is approximate for FilterConfig (the flat-Spec form
// becomes an AND of CompareNodes). A post-migration round-trip
// through the wizard yields the same predicate.
func Run(s *settings.Settings) int {
	if s == nil {
		return 0
	}
	migrated := 0
	hasDomain := func(domain string) bool {
		for _, c := range s.Chips() {
			if c.Domain == domain {
				return true
			}
		}
		return false
	}
	if !hasDomain("records") {
		for _, l := range s.Lenses() {
			s.UpsertChip(lensConfigToChipConfig(l))
			migrated++
		}
	}
	if !hasDomain("objects") {
		for _, f := range s.ObjectFilters() {
			s.UpsertChip(filterConfigToChipConfig(f, "objects"))
			migrated++
		}
	}
	if !hasDomain("flows") {
		for _, f := range s.FlowFilters() {
			s.UpsertChip(filterConfigToChipConfig(f, "flows"))
			migrated++
		}
	}
	return migrated
}

// lensConfigToChipConfig converts a legacy LensConfig (free-form SOQL
// strings) into a ChipConfig. Where clause runs through the parser;
// on parse failure we fall back to "match everything" rather than
// dropping the entry.
func lensConfigToChipConfig(l settings.LensConfig) settings.ChipConfig {
	q := query.Query{Limit: l.Limit, Columns: l.Columns}
	if l.SOQLWhere != "" {
		// Trick the parser by handing it a synthetic SELECT so the
		// WHERE-only string parses through the full pipeline.
		synthetic := "SELECT Id FROM " + nonEmpty(l.Scope, "X") + " WHERE " + l.SOQLWhere
		if l.OrderBy != "" {
			synthetic += " ORDER BY " + l.OrderBy
		}
		parsed, _, err := query.Parse(synthetic)
		if err == nil {
			q = parsed
			if l.Limit > 0 {
				q.Limit = l.Limit
			}
			if len(l.Columns) > 0 {
				q.Columns = l.Columns
			}
		}
	} else if l.OrderBy != "" {
		// Just an ORDER BY, no WHERE.
		obs, err := parseOrderBy(l.OrderBy)
		if err == nil {
			q.OrderBy = obs
		}
	}
	origin := "user"
	if l.Origin == "imported" {
		origin = "imported"
	}
	return settings.ChipConfig{
		ID:         l.ID,
		Label:      l.Label,
		Scope:      l.Scope,
		Domain:     "records",
		Origin:     origin,
		Query:      qchip.QueryToConfig(q),
		SourceID:   l.SourceID,
		SourceName: l.SourceName,
		ImportedAt: l.ImportedAt,
	}
}

// filterConfigToChipConfig maps the per-field FilterSpecYAML onto an
// AND of CompareNodes. Each non-empty field becomes one comparison;
// empty fields drop out.
func filterConfigToChipConfig(f settings.FilterConfig, domain string) settings.ChipConfig {
	var children []query.Node
	add := func(field string, op query.Op, val any) {
		children = append(children, query.Cmp(field, op, val))
	}
	s := f.Spec
	if s.NameContains != "" {
		add("Name", query.OpContains, s.NameContains)
	}
	if s.LabelContains != "" {
		add("Label", query.OpContains, s.LabelContains)
	}
	if s.DescriptionContains != "" {
		add("Description", query.OpContains, s.DescriptionContains)
	}
	if s.Suffix != "" {
		add("Name", query.OpEndsWith, s.Suffix)
	}
	if s.Prefix != "" {
		add("Name", query.OpStartsWith, s.Prefix)
	}
	if s.NamespaceEquals != "" {
		add("Namespace", query.OpEq, s.NamespaceEquals)
	}
	if s.StatusEquals != "" {
		add("Status", query.OpEq, s.StatusEquals)
	}
	if s.CategoryEquals != "" {
		// CategoryEquals is domain-specific; for flows it's ProcessType,
		// for objects it's a derived bucket. For flows we map cleanly;
		// for objects we leave it as a "Category" virtual field —
		// the migration is best-effort and the user can edit later.
		switch domain {
		case "flows":
			add("ProcessType", query.OpEq, s.CategoryEquals)
		default:
			// Best-effort: convert to a known suffix predicate when we
			// recognise the category, otherwise keep the literal as a
			// virtual "Category" field (Eval ignores unknown fields).
			switch s.CategoryEquals {
			case "platform-event":
				add("Name", query.OpEndsWith, "__e")
			case "change-event":
				add("Name", query.OpEndsWith, "ChangeEvent")
			case "custom-metadata":
				add("Name", query.OpEndsWith, "__mdt")
			case "custom":
				add("IsCustom", query.OpEq, true)
			default:
				add("Category", query.OpEq, s.CategoryEquals)
			}
		}
	}
	if s.DeploymentStatusEquals != "" {
		add("DeploymentStatus", query.OpEq, s.DeploymentStatusEquals)
	}
	if s.KeyPrefixEquals != "" {
		add("KeyPrefix", query.OpEq, s.KeyPrefixEquals)
	}
	if s.APIVersionGTE != 0 {
		add("ApiVersion", query.OpGTE, s.APIVersionGTE)
	}
	if s.APIVersionLTE != 0 {
		add("ApiVersion", query.OpLTE, s.APIVersionLTE)
	}
	if s.ModifiedAfter != "" {
		add("LastModifiedDate", query.OpGT, s.ModifiedAfter)
	}
	if s.ModifiedBefore != "" {
		add("LastModifiedDate", query.OpLT, s.ModifiedBefore)
	}
	if s.ModifiedBy != "" {
		add("LastModifiedBy", query.OpContains, s.ModifiedBy)
	}
	if s.IsCustom != nil {
		add("IsCustom", query.OpEq, *s.IsCustom)
	}
	if s.IsApexTriggerable != nil {
		add("IsApexTriggerable", query.OpEq, *s.IsApexTriggerable)
	}
	if s.IsWorkflowEnabled != nil {
		add("IsWorkflowEnabled", query.OpEq, *s.IsWorkflowEnabled)
	}
	if s.HasActiveVersion != nil {
		if *s.HasActiveVersion {
			children = append(children, query.Not(query.Cmp("ActiveVersionId", query.OpIsNull, nil)))
		} else {
			add("ActiveVersionId", query.OpIsNull, nil)
		}
	}
	q := query.Query{Where: query.And(children...)}
	origin := "user"
	if f.Origin == "imported" {
		origin = "imported"
	}
	return settings.ChipConfig{
		ID:     f.ID,
		Label:  f.Label,
		Scope:  nonEmpty(f.Scope, "*"),
		Domain: domain,
		Origin: origin,
		Query:  qchip.QueryToConfig(q),
	}
}

// parseOrderBy is a thin wrapper around the query package's parser
// for the ORDER BY suffix.
func parseOrderBy(s string) ([]query.OrderBy, error) {
	q, _, err := query.Parse("SELECT Id FROM X ORDER BY " + s)
	if err != nil {
		return nil, err
	}
	return q.OrderBy, nil
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
