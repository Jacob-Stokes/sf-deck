package ui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/soqlauto"
)

func (m *Model) autocompleteFetchValues(s *soqlSession, target soqlSessionTarget) tea.Cmd {
	if s == nil || s.autocomplete == nil {
		return nil
	}
	ac := s.autocomplete
	cls := ac.Class
	if cls.Context != soqlauto.ContextWhereValue && cls.Context != soqlauto.ContextInWithValues {
		return nil
	}
	if cls.Sobject == "" || cls.WhereField == "" {
		return nil
	}
	// Resolve the LHS field type via the describe cache — must be
	// a text-shaped field for the live-fetch to be meaningful.
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	if !fieldEligibleForLiveFetch(d, cls.Sobject, cls.WhereField) {
		return nil
	}
	if len(m.orgs) == 0 {
		return nil
	}
	o := m.orgs[m.selected]
	soql := buildLiveValuesSOQL(cls.Sobject, cls.WhereField, cls.SearchToken)

	ac.ValuesGen++
	gen := ac.ValuesGen
	ac.ValuesLoading = true
	ac.ValuesField = cls.WhereField
	if ac.ValuesCancel != nil {
		ac.ValuesCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	ac.ValuesCancel = cancel

	return runAutocompleteValuesCmd(ctx, o, soql, cls.WhereField, target, s.id, gen)
}

// fieldEligibleForLiveFetch reports whether the LHS field of the
// active WHERE comparison is the kind we should run a distinct
// query against. Static-value fields (picklist, boolean, date)
// are excluded because their values come from the describe.
func fieldEligibleForLiveFetch(d *orgData, sobject, dottedField string) bool {
	if d == nil || sobject == "" || dottedField == "" {
		return false
	}
	parts := strings.Split(dottedField, ".")
	terminal := parts[len(parts)-1]
	hops := parts[:len(parts)-1]
	current := sobject
	for _, hop := range hops {
		r, ok := d.Describes[current]
		if !ok || r == nil || r.FetchedAt().IsZero() {
			return false
		}
		desc := r.Value()
		found := false
		for _, f := range desc.Fields {
			if strings.EqualFold(f.RelationshipName, hop) && len(f.ReferenceTo) > 0 {
				current = f.ReferenceTo[0]
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	r, ok := d.Describes[current]
	if !ok || r == nil || r.FetchedAt().IsZero() {
		return false
	}
	desc := r.Value()
	for _, f := range desc.Fields {
		if !strings.EqualFold(f.Name, terminal) {
			continue
		}
		switch f.Type {
		case "string", "email", "phone", "url", "textarea", "reference", "id":
			return true
		}
		return false
	}
	return false
}

func buildLiveValuesSOQL(sobject, field, term string) string {
	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString(field)
	b.WriteString(" FROM ")
	b.WriteString(sobject)
	if term != "" {
		b.WriteString(" WHERE ")
		b.WriteString(field)
		b.WriteString(" LIKE '%")
		b.WriteString(escapeSOQLLiteral(term))
		b.WriteString("%'")
	}
	b.WriteString(" GROUP BY ")
	b.WriteString(field)
	b.WriteString(" LIMIT 100")
	return b.String()
}

// escapeSOQLLiteral handles ' and \ — minimal because the term
// comes from the user's keystrokes and is typically short.
func escapeSOQLLiteral(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

func runAutocompleteValuesCmd(ctx context.Context, o sf.Org, soql, field string, target soqlSessionTarget, sessionID, gen uint64) tea.Cmd {
	return func() tea.Msg {
		result, err := sf.QueryCtx(ctx, targetArg(o), soql, false)
		if err != nil {
			return autocompleteValuesMsg{
				session: target, sessionID: sessionID, gen: gen,
				field: field, err: err,
			}
		}
		values := make([]string, 0, len(result.Records))
		for _, rec := range result.Records {
			for k, v := range rec {
				if k == "attributes" {
					continue
				}
				if v == nil {
					continue
				}
				if s, ok := v.(string); ok {
					values = append(values, s)
				}
			}
		}
		return autocompleteValuesMsg{
			session: target, sessionID: sessionID, gen: gen,
			field: field, values: values,
		}
	}
}

func (m *Model) applyAutocompleteValues(msg autocompleteValuesMsg) {
	s := m.soqlSessionForTarget(msg.session)
	if s == nil || s.id != msg.sessionID || s.autocomplete == nil {
		return
	}
	ac := s.autocomplete
	if msg.gen != ac.ValuesGen {
		return
	}
	ac.ValuesLoading = false
	ac.ValuesCancel = nil
	if msg.err != nil {
		ac.ValuesErr = msg.err
		ac.ValuesValues = nil
		return
	}
	ac.ValuesErr = nil
	ac.ValuesValues = msg.values
	if ac.Class.Context == soqlauto.ContextWhereValue || ac.Class.Context == soqlauto.ContextInWithValues {
		items := make([]soqlauto.Suggestion, 0, len(msg.values))
		for _, v := range msg.values {
			items = append(items, soqlauto.Suggestion{
				Value:   "'" + escapeSOQLLiteral(v) + "'",
				Display: v,
				Detail:  "live · existing value",
				Kind:    soqlauto.KindPicklist,
				Rank:    1,
			})
		}
		ac.Items = items
		ac.Cursor = 0
	}
}
