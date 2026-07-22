package ui

// Live distinct-value fetch for SOQL autocomplete. Triggered by
// Ctrl+Space in a WhereValue context on a string/reference field:
// runs `SELECT <field> FROM <sobject> WHERE <field> LIKE '%term%'
// GROUP BY <field> LIMIT 100` against the active org and pipes the
// returned values back into the popup as KindPicklist suggestions.
//
// Mirrors Inspector Reloaded's "Ctrl+Space for picklists" feature.
// Cancel-on-keystroke: every new keystroke bumps a generation
// counter; in-flight fetches against an outdated gen drop their
// results silently.

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/soqlauto"
)

// autocompleteValuesGen lives on the soqlSession so cancellation
// is per-session. Bumped on every Ctrl+Space and on every
// keystroke that closes the popup.
//
// Tracked as a field on autocompleteState to keep the runtime
// state cluster cohesive — see autocompleteState.ValuesGen.

// autocompleteFetchValues handles the Ctrl+Space gesture in a
// WhereValue context on a text/reference field. Returns a tea.Cmd
// to dispatch; caller (the key handler) emits it on the next round.
//
// Pre-conditions:
//   - Caret is in ContextWhereValue or ContextInWithValues
//   - WhereField is non-empty
//   - The field's type is one of: string, email, phone, url,
//     textarea, reference, id (i.e. no static picklist values)
//
// Out-of-band: bumps ValuesGen so any prior in-flight fetch's
// result is ignored. Sets ValuesLoading true so the popup can
// show "fetching values…" while we wait.
func (m *Model) autocompleteFetchValues(s *soqlSession, target soqlSessionTarget) tea.Cmd {
	if s == nil || s.autocomplete == nil {
		return nil
	}
	ac := s.autocomplete
	cls := ac.Class
	// Only WhereValue / InWithValues contexts trigger this.
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

	// Bump generation, mark loading, cancel any prior fetch.
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

// buildLiveValuesSOQL composes the distinct-value query. Field is
// the dotted path the user typed; sobject is the FROM target.
// We pass the field path through verbatim — SOQL accepts dotted
// projections in both SELECT and GROUP BY for filterable refs.
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

// runAutocompleteValuesCmd is the tea.Cmd that dispatches the
// SOQL via the REST client + emits autocompleteValuesMsg back.
func runAutocompleteValuesCmd(ctx context.Context, o sf.Org, soql, field string, target soqlSessionTarget, sessionID, gen uint64) tea.Cmd {
	return func() tea.Msg {
		result, err := sf.QueryCtx(ctx, targetArg(o), soql, false)
		if err != nil {
			return autocompleteValuesMsg{
				session: target, sessionID: sessionID, gen: gen,
				field: field, err: err,
			}
		}
		// Extract distinct values: each row has one column (the
		// GROUP BY field). Field map keys are case-sensitive; SF
		// returns the field name as-typed in SELECT, so we walk
		// any non-attributes key on the first record to find it.
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

// applyAutocompleteValues folds a values-fetched message into the
// session's autocomplete state. Drops the message when the gen
// has advanced (cancelled) OR the user moved off the field.
func (m *Model) applyAutocompleteValues(msg autocompleteValuesMsg) {
	s := m.soqlSessionForTarget(msg.session)
	if s == nil || s.id != msg.sessionID || s.autocomplete == nil {
		return
	}
	ac := s.autocomplete
	if msg.gen != ac.ValuesGen {
		// Stale — superseded by a newer fetch or cancelled.
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
	// Inject as suggestions if the user is still in the right
	// context. We REPLACE Items rather than appending — the live
	// values ARE the suggestions in WhereValue context.
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
