package chips

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

// CreateInput is the validated input for Create. The CLI parser fills
// this from flags; the TUI's chip-manager save path will fill it from
// in-memory state once both paths share this service. Pointer-typed
// flags would be cleaner for partial updates but Create requires every
// field, so plain types are simpler here.
type CreateInput struct {
	ID        string
	Domain    string
	Scope     string
	Label     string
	Favourite bool
	Columns   []string
	Limit     int
	Clauses   string
}

// Result is the typed write outcome. Chip is the post-write state;
// Changed is true when the call actually mutated settings (false for
// idempotent no-ops, e.g. Favourite re-set to the same value).
type Result struct {
	Chip    Chip
	Changed bool
}

// Persister is the persistence boundary the service writes through.
// settings.Settings.Save satisfies it; tests pass a no-op implementation
// to verify Save is invoked exactly once on a state-mutating call (and
// skipped on a no-op).
//
// Kept as a function type rather than an interface because the only
// thing the service needs is "flush to disk now" — adding methods
// later would balloon the test surface.
type Persister func() error

// ValidateID + ValidateLabel are THE chip naming rules — exported so
// the TUI's chip wizard and the headless CLI parser run the exact
// same checks (they were copies before 2026-06-12; copies drift).
func ValidateID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("id is required")
	}
	// IDs become TOML table keys + JSON keys. Reject characters that
	// would force quoting or break shell completion.
	for _, r := range id {
		if r == ' ' || r == '\t' || r == '\n' || r == '"' || r == '\'' {
			return fmt.Errorf("id %q contains whitespace or quotes", id)
		}
	}
	return nil
}

func ValidateLabel(label string) error {
	if strings.TrimSpace(label) == "" {
		return errors.New("label is required")
	}
	return nil
}

// Validate runs the full input check used by Create + Update. Exposed
// for the CLI parser + the chip command's --validate flag.
func (in CreateInput) Validate() error {
	if err := ValidateID(in.ID); err != nil {
		return err
	}
	if !IsValidDomain(in.Domain) {
		return fmt.Errorf("unknown domain %q (want one of %s)",
			in.Domain, strings.Join(Domains(), ", "))
	}
	if err := ValidateLabel(in.Label); err != nil {
		return err
	}
	if in.Limit < 0 {
		return fmt.Errorf("limit must be >= 0, got %d", in.Limit)
	}
	if strings.TrimSpace(in.Clauses) != "" {
		if _, err := ParseClauses(in.Clauses); err != nil {
			return err
		}
	}
	return nil
}

// Create inserts a new chip. Returns ErrAlreadyExists if a chip with
// the same (domain, id) already exists — Update is the right call in
// that case. On success, runs save() exactly once.
func Create(st *settings.Settings, in CreateInput, save Persister) (Result, error) {
	if st == nil {
		return Result{}, errors.New("nil settings")
	}
	if err := in.Validate(); err != nil {
		return Result{}, err
	}
	for _, c := range st.Chips() {
		if c.Domain == in.Domain && c.ID == in.ID {
			return Result{}, ErrAlreadyExists{Domain: in.Domain, ID: in.ID}
		}
	}
	queryCfg := settings.ChipQueryYAML{
		Columns: append([]string(nil), in.Columns...),
		Limit:   in.Limit,
	}
	if strings.TrimSpace(in.Clauses) != "" {
		parsed, err := ParseClauses(in.Clauses)
		if err != nil {
			return Result{}, err
		}
		if in.Limit > 0 && parsed.Limit > 0 && in.Limit != parsed.Limit {
			return Result{}, fmt.Errorf("limit specified twice: --limit=%d but clauses contain LIMIT %d", in.Limit, parsed.Limit)
		}
		if in.Limit > 0 {
			parsed.Limit = in.Limit
		}
		parsed.Columns = append([]string(nil), in.Columns...)
		queryCfg = parsed
	}
	cfg := settings.ChipConfig{
		ID:        in.ID,
		Domain:    in.Domain,
		Scope:     in.Scope,
		Label:     in.Label,
		Origin:    "user",
		Favourite: in.Favourite,
		Query:     queryCfg,
	}
	st.UpsertChip(cfg)
	if save != nil {
		if err := save(); err != nil {
			return Result{}, fmt.Errorf("save: %w", err)
		}
	}
	return Result{Chip: fromConfig(cfg), Changed: true}, nil
}

// UpdateInput is the partial-update shape. Each pointer field that's
// non-nil triggers an update; nil leaves the existing value intact.
// Domain and ID together are the identity — they can't be changed in
// place. To rename, Delete + Create.
type UpdateInput struct {
	Scope     *string
	Label     *string
	Favourite *bool
	Columns   *[]string
	Limit     *int
	Clauses   *string
}

// HasAny reports whether the input would actually change anything.
// CLI dispatch uses this to fail-fast with invalid_argument when no
// update fields are passed (so the user gets a clear error instead of
// a silent changed=false).
func (in UpdateInput) HasAny() bool {
	return in.Scope != nil || in.Label != nil || in.Favourite != nil ||
		in.Columns != nil || in.Limit != nil || in.Clauses != nil
}

// Update applies a partial update. Returns ErrNotFound when the chip
// doesn't exist. Changed=false when the inputs match the current
// state (idempotent — agents can re-run safely).
func Update(st *settings.Settings, domain, id string, in UpdateInput, save Persister) (Result, error) {
	if st == nil {
		return Result{}, errors.New("nil settings")
	}
	if !IsValidDomain(domain) {
		return Result{}, fmt.Errorf("unknown domain %q", domain)
	}
	if err := ValidateID(id); err != nil {
		return Result{}, err
	}
	chips := st.Chips()
	idx := -1
	for i, c := range chips {
		if c.Domain == domain && c.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return Result{}, ErrNotFound{Domain: domain, ID: id}
	}
	cfg := chips[idx]
	changed := false
	if in.Scope != nil && *in.Scope != cfg.Scope {
		cfg.Scope = *in.Scope
		changed = true
	}
	if in.Label != nil {
		if err := ValidateLabel(*in.Label); err != nil {
			return Result{}, err
		}
		if *in.Label != cfg.Label {
			cfg.Label = *in.Label
			changed = true
		}
	}
	if in.Favourite != nil && *in.Favourite != cfg.Favourite {
		cfg.Favourite = *in.Favourite
		changed = true
	}
	if in.Columns != nil && !stringsEqual(*in.Columns, cfg.Query.Columns) {
		cfg.Query.Columns = append([]string(nil), (*in.Columns)...)
		changed = true
	}
	if in.Clauses != nil {
		parsed, err := ParseClauses(*in.Clauses)
		if err != nil {
			return Result{}, err
		}
		if in.Limit != nil && *in.Limit > 0 && parsed.Limit > 0 && *in.Limit != parsed.Limit {
			return Result{}, fmt.Errorf("limit specified twice: --limit=%d but clauses contain LIMIT %d", *in.Limit, parsed.Limit)
		}
		parsed.Columns = append([]string(nil), cfg.Query.Columns...)
		if in.Limit != nil && *in.Limit > 0 {
			parsed.Limit = *in.Limit
		}
		if !reflect.DeepEqual(parsed, cfg.Query) {
			cfg.Query = parsed
			changed = true
		}
	}
	if in.Limit != nil {
		if *in.Limit < 0 {
			return Result{}, fmt.Errorf("limit must be >= 0, got %d", *in.Limit)
		}
		if *in.Limit != cfg.Query.Limit {
			cfg.Query.Limit = *in.Limit
			changed = true
		}
	}
	if !changed {
		// Idempotent no-op — don't touch disk.
		return Result{Chip: fromConfig(cfg), Changed: false}, nil
	}
	st.UpsertChip(cfg)
	if save != nil {
		if err := save(); err != nil {
			return Result{}, fmt.Errorf("save: %w", err)
		}
	}
	return Result{Chip: fromConfig(cfg), Changed: true}, nil
}

// Delete removes a chip. Returns ErrNotFound on miss — callers may
// want to surface "already absent" as a different exit code than
// "deleted N". Idempotent at the JSON-shape level only via ErrNotFound
// + the caller's policy; we don't silently succeed.
func Delete(st *settings.Settings, domain, id string, save Persister) (Result, error) {
	if st == nil {
		return Result{}, errors.New("nil settings")
	}
	if !IsValidDomain(domain) {
		return Result{}, fmt.Errorf("unknown domain %q", domain)
	}
	if err := ValidateID(id); err != nil {
		return Result{}, err
	}
	before := len(st.Chips())
	// Capture the chip we're about to delete so the response can
	// include its state under data.chip — consumers want to know what
	// they killed.
	var snapshot Chip
	for _, c := range st.Chips() {
		if c.Domain == domain && c.ID == id {
			snapshot = fromConfig(c)
			break
		}
	}
	st.DeleteChip(domain, id)
	if len(st.Chips()) == before {
		return Result{}, ErrNotFound{Domain: domain, ID: id}
	}
	if save != nil {
		if err := save(); err != nil {
			return Result{}, fmt.Errorf("save: %w", err)
		}
	}
	return Result{Chip: snapshot, Changed: true}, nil
}

// Favourite is the idempotent toggle-or-set used by `sf-deck chip
// favourite`. Pass on=true to favourite, on=false to unfavourite.
// Changed reflects whether the value actually shifted.
func Favourite(st *settings.Settings, domain, id string, on bool, save Persister) (Result, error) {
	v := on
	return Update(st, domain, id, UpdateInput{Favourite: &v}, save)
}

// ErrAlreadyExists is returned by Create when a chip with the same
// (domain, id) is already present.
type ErrAlreadyExists struct {
	Domain string
	ID     string
}

func (e ErrAlreadyExists) Error() string {
	return fmt.Sprintf("chip %s/%s already exists", e.Domain, e.ID)
}

func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
