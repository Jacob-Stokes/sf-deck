// Package chips is the service layer for the user's chip catalogue.
package chips

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

// Chip is the public, headless-facing view of one user chip. Mirrors
// settings.ChipConfig but uses ordinary field names + json tags so the
// JSON envelope under data.chip stays stable independently of the
// TOML layout (which is allowed to evolve).
type Chip struct {
	ID        string   `json:"id"`
	Domain    string   `json:"domain"`
	Scope     string   `json:"scope,omitempty"`
	Label     string   `json:"label"`
	Origin    string   `json:"origin,omitempty"`
	Favourite bool     `json:"favourite"`
	Columns   []string `json:"columns,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Clauses   string   `json:"clauses,omitempty"`
}

var validDomains = map[string]bool{
	"records": true,
	"objects": true,
	"flows":   true,
}

// IsValidDomain reports whether s is one of the known chip domains.
// Exported because the CLI parser uses it before constructing a
// CreateInput, so the error surfaces as invalid_argument rather than
// service-level validation.
func IsValidDomain(s string) bool {
	return validDomains[s]
}

// Domains returns the closed set, sorted for stable help text.
func Domains() []string {
	out := make([]string, 0, len(validDomains))
	for d := range validDomains {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func fromConfig(c settings.ChipConfig) Chip {
	q := queryFromConfig(c.Query)
	return Chip{
		ID:        c.ID,
		Domain:    c.Domain,
		Scope:     c.Scope,
		Label:     c.Label,
		Origin:    c.Origin,
		Favourite: c.Favourite,
		Columns:   append([]string(nil), c.Query.Columns...),
		Limit:     c.Query.Limit,
		Clauses:   query.ToSOQLClauses(q),
	}
}

// List returns every chip in the user's catalogue, optionally
// filtered by domain. Empty domain means "all". The slice is freshly
// allocated; callers can sort / mutate freely.
func List(st *settings.Settings, domain string) ([]Chip, error) {
	if st == nil {
		return nil, errors.New("nil settings")
	}
	if domain != "" && !IsValidDomain(domain) {
		return nil, fmt.Errorf("unknown domain %q (want one of %s)",
			domain, strings.Join(Domains(), ", "))
	}
	in := st.Chips()
	out := make([]Chip, 0, len(in))
	for _, c := range in {
		if domain != "" && c.Domain != domain {
			continue
		}
		out = append(out, fromConfig(c))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}
		if out[i].Label != out[j].Label {
			return out[i].Label < out[j].Label
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Show returns the single chip matching (domain, id). Returns
// ErrNotFound when absent so headless wrappers can map to the typed
// not_found code without string-matching.
func Show(st *settings.Settings, domain, id string) (Chip, error) {
	if st == nil {
		return Chip{}, errors.New("nil settings")
	}
	if !IsValidDomain(domain) {
		return Chip{}, fmt.Errorf("unknown domain %q", domain)
	}
	if id == "" {
		return Chip{}, errors.New("id is required")
	}
	for _, c := range st.Chips() {
		if c.Domain == domain && c.ID == id {
			return fromConfig(c), nil
		}
	}
	return Chip{}, ErrNotFound{Domain: domain, ID: id}
}

// ErrNotFound is returned when Show / Update / Delete / Favourite
// can't find a chip by (domain, id). Headless callers branch on
// errors.As to surface the not_found code.
type ErrNotFound struct {
	Domain string
	ID     string
}

func (e ErrNotFound) Error() string {
	return fmt.Sprintf("chip %s/%s not found", e.Domain, e.ID)
}
