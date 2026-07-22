// Package tags is the service layer for the personal tag annotation
// system stored in devprojects.db.
//
// Tags are user-scoped (one global namespace per machine) and binding
// rows are per-(item, org). This package wraps devproject.Store with a
// validated typed surface shared by the TUI's tag manager and the
// headless `sf-deck tag ...` commands.
//
// What lives here:
//
//   - Public Tag / Binding views with json tags for stable wire shape.
//   - Validated CRUD: List / Show / Create / Update / Delete.
//   - Binding ops: Apply / Remove / Set / TagsFor / ItemsWithTag.
//   - Typed ErrNotFound / ErrAlreadyExists / ErrInvalidKind.
//
// What does NOT live here:
//
//   - SQL or migrations (devproject owns those).
//   - Cross-cutting concerns like org resolution (callers pre-resolve
//     the orgUser string and pass it in).
package tags

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

// Tag is the headless-facing view. Distinct from devproject.Tag so
// the JSON envelope can evolve independently of the SQLite row
// shape.
type Tag struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color,omitempty"`
	Icon      string    `json:"icon,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	// Count is the binding count, populated by List when requested.
	// Omitted from Show / Create / Update responses (we don't fetch
	// it on the single-tag path to save the join).
	Count int `json:"count,omitempty"`
}

// Binding is the public view of one tag→item attachment.
type Binding struct {
	TagID     int64     `json:"tag_id"`
	ItemKind  string    `json:"item_kind"`
	ItemRef   string    `json:"item_ref"`
	OrgUser   string    `json:"org_user,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ErrNotFound is returned when an op references a missing tag (by id
// or by name). Headless wrappers map this to the not_found code.
type ErrNotFound struct {
	// ID is set when the lookup was by id. Zero otherwise.
	ID int64
	// Name is set when the lookup was by name. Empty otherwise.
	Name string
}

func (e ErrNotFound) Error() string {
	if e.Name != "" {
		return fmt.Sprintf("tag %q not found", e.Name)
	}
	return fmt.Sprintf("tag #%d not found", e.ID)
}

// ErrAlreadyExists is returned when Create / Update would collide on
// the unique tag name (case-insensitive).
type ErrAlreadyExists struct {
	Name string
}

func (e ErrAlreadyExists) Error() string {
	return fmt.Sprintf("tag %q already exists", e.Name)
}

// ErrInvalidKind is returned when a binding op references an
// item_kind that isn't in devproject's closed set. Maps to
// invalid_argument; keeps the wire shape clean instead of leaking the
// devproject sentinel directly.
type ErrInvalidKind struct {
	Kind string
}

func (e ErrInvalidKind) Error() string {
	return fmt.Sprintf("unknown item kind %q (want one of %s)",
		e.Kind, strings.Join(KnownKinds(), ", "))
}

// validKinds is the registry of recognised item kinds. Mirrors
// devproject's exported constants. Headless callers must pass one of
// these — anything else is an invalid_argument.
var validKinds = map[string]devproject.ItemKind{
	"sobject":         devproject.KindSObject,
	"field":           devproject.KindField,
	"flow":            devproject.KindFlow,
	"flow_version":    devproject.KindFlowVersion,
	"record":          devproject.KindRecord,
	"apex_class":      devproject.KindApexClass,
	"report":          devproject.KindReport,
	"permset":         devproject.KindPermissionSet,
	"permset_group":   devproject.KindPermissionSetGroup,
	"profile":         devproject.KindProfile,
	"validation_rule": devproject.KindValidationRule,
	"record_type":     devproject.KindRecordType,
	"apex_trigger":    devproject.KindApexTrigger,
	"lwc":             devproject.KindLWC,
	"aura":            devproject.KindAura,
	"queue":           devproject.KindQueue,
	"public_group":    devproject.KindPublicGroup,
	"soql_query":      devproject.KindSOQLQuery,
	"apex_snippet":    devproject.KindApexSnippet,
}

// KnownKinds returns the closed set, sorted for stable help text.
func KnownKinds() []string {
	out := make([]string, 0, len(validKinds))
	for k := range validKinds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// parseKind resolves a string into the devproject typed kind.
// Returns ErrInvalidKind if not recognised. Caller is responsible for
// surfacing this as invalid_argument.
func parseKind(s string) (devproject.ItemKind, error) {
	k, ok := validKinds[s]
	if !ok {
		return "", ErrInvalidKind{Kind: s}
	}
	return k, nil
}

// fromDP converts a devproject.Tag to the public view.
func fromDP(t devproject.Tag) Tag {
	return Tag{
		ID:        t.ID,
		Name:      t.Name,
		Color:     t.Color,
		Icon:      t.Icon,
		CreatedAt: t.CreatedAt,
	}
}

// validateName enforces the rules the TUI's tag manager applies. Used
// by Create + Update so the error surfaces before we even hit SQL.
func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("name is required")
	}
	return nil
}

// List returns every tag with binding counts. With usage_only=true
// only tags that actually bind to something are returned (matches the
// TUI's chip-strip filter).
func List(s *devproject.Store, usageOnly bool) ([]Tag, error) {
	if s == nil {
		return nil, errors.New("nil store")
	}
	rows, err := s.ListTagsWithUsage()
	if err != nil {
		return nil, err
	}
	out := make([]Tag, 0, len(rows))
	for _, r := range rows {
		if usageOnly && r.Count == 0 {
			continue
		}
		t := fromDP(r.Tag)
		t.Count = r.Count
		out = append(out, t)
	}
	return out, nil
}

// Show looks up a single tag by id or by name (exactly one must be
// set). Returns ErrNotFound on miss.
func Show(s *devproject.Store, id int64, name string) (Tag, error) {
	if s == nil {
		return Tag{}, errors.New("nil store")
	}
	if id != 0 && name != "" {
		return Tag{}, errors.New("specify --id OR --name, not both")
	}
	if id == 0 && name == "" {
		return Tag{}, errors.New("--id or --name is required")
	}
	if name != "" {
		t, ok, err := s.FindTagByName(name)
		if err != nil {
			return Tag{}, err
		}
		if !ok {
			return Tag{}, ErrNotFound{Name: name}
		}
		return fromDP(t), nil
	}
	// Lookup by id. The store doesn't expose a single-row getter, so
	// scan via ListTags which is already sorted + fully fetched. The
	// alternative — adding a single-row method — would gain nothing
	// for the small N expected here (<100 tags is realistic).
	all, err := s.ListTags()
	if err != nil {
		return Tag{}, err
	}
	for _, t := range all {
		if t.ID == id {
			return fromDP(t), nil
		}
	}
	return Tag{}, ErrNotFound{ID: id}
}

// CreateInput is the validated input for Create. Icon is intentionally
// not validated for grapheme count — devproject stores it as opaque
// text, and clamping would just bake a UI rule into the service.
type CreateInput struct {
	Name  string
	Color string
	Icon  string
}

// Validate runs the same input check Create runs, exposed so the CLI
// parser can fail-fast with invalid_argument.
func (in CreateInput) Validate() error {
	return validateName(in.Name)
}

// Result is the typed write outcome. Tag is the post-write state;
// Changed is true when the call mutated state (false for idempotent
// no-ops).
type Result struct {
	Tag     Tag
	Changed bool
}

// Create inserts a new tag. Maps devproject.ErrTagExists to
// ErrAlreadyExists so headless callers don't have to import the
// devproject package for error checking.
func Create(s *devproject.Store, in CreateInput) (Result, error) {
	if s == nil {
		return Result{}, errors.New("nil store")
	}
	if err := in.Validate(); err != nil {
		return Result{}, err
	}
	t, err := s.CreateTag(in.Name, in.Color, in.Icon)
	if err != nil {
		if errors.Is(err, devproject.ErrTagExists) {
			return Result{}, ErrAlreadyExists{Name: in.Name}
		}
		return Result{}, err
	}
	return Result{Tag: fromDP(t), Changed: true}, nil
}

// UpdateInput is the partial-update shape. Each pointer field that's
// non-nil triggers an update; nil leaves the existing value intact.
type UpdateInput struct {
	Name  *string
	Color *string
	Icon  *string
}

// HasAny reports whether any update field is set.
func (in UpdateInput) HasAny() bool {
	return in.Name != nil || in.Color != nil || in.Icon != nil
}

// Update applies a partial update to the tag. Returns ErrNotFound if
// the id is missing, ErrAlreadyExists if a rename collides.
//
// Idempotency: the devproject store's UpdateTag always issues an
// UPDATE; we re-fetch and report Changed only when at least one field
// shifted, matching the chips service's contract.
func Update(s *devproject.Store, id int64, in UpdateInput) (Result, error) {
	if s == nil {
		return Result{}, errors.New("nil store")
	}
	if id == 0 {
		return Result{}, errors.New("id is required")
	}
	if !in.HasAny() {
		return Result{}, errors.New("no update fields specified")
	}
	cur, err := Show(s, id, "")
	if err != nil {
		return Result{}, err
	}
	name := cur.Name
	color := cur.Color
	icon := cur.Icon
	changed := false
	if in.Name != nil {
		if err := validateName(*in.Name); err != nil {
			return Result{}, err
		}
		if *in.Name != name {
			name = *in.Name
			changed = true
		}
	}
	if in.Color != nil && *in.Color != color {
		color = *in.Color
		changed = true
	}
	if in.Icon != nil && *in.Icon != icon {
		icon = *in.Icon
		changed = true
	}
	if !changed {
		return Result{Tag: cur, Changed: false}, nil
	}
	if err := s.UpdateTag(id, name, color, icon); err != nil {
		if errors.Is(err, devproject.ErrTagExists) {
			return Result{}, ErrAlreadyExists{Name: name}
		}
		if errors.Is(err, devproject.ErrTagNotFound) {
			return Result{}, ErrNotFound{ID: id}
		}
		return Result{}, err
	}
	updated, err := Show(s, id, "")
	if err != nil {
		return Result{}, err
	}
	return Result{Tag: updated, Changed: true}, nil
}

// Delete removes the tag (cascades all bindings). Returns ErrNotFound
// when the tag didn't exist — devproject.DeleteTag is silently
// idempotent, so we pre-check via Show to make the headless wire
// behaviour explicit ("you deleted nothing" → not_found, separate
// exit code from "you deleted something").
func Delete(s *devproject.Store, id int64) (Result, error) {
	if s == nil {
		return Result{}, errors.New("nil store")
	}
	if id == 0 {
		return Result{}, errors.New("id is required")
	}
	cur, err := Show(s, id, "")
	if err != nil {
		return Result{}, err
	}
	if err := s.DeleteTag(id); err != nil {
		return Result{}, err
	}
	return Result{Tag: cur, Changed: true}, nil
}

// Apply binds a tag to an item. Idempotent at the SQL layer (PRIMARY
// KEY collision is silently ignored). Changed is true when a new
// binding row was inserted, false when the binding already existed.
func Apply(s *devproject.Store, tagID int64, kind, ref, orgUser string) (Result, error) {
	if s == nil {
		return Result{}, errors.New("nil store")
	}
	if tagID == 0 {
		return Result{}, errors.New("tag id is required")
	}
	if ref == "" {
		return Result{}, errors.New("item ref is required")
	}
	k, err := parseKind(kind)
	if err != nil {
		return Result{}, err
	}
	cur, err := Show(s, tagID, "")
	if err != nil {
		return Result{}, err
	}
	// Detect changed by checking presence BEFORE the apply. The
	// devproject store has INSERT OR IGNORE so it doesn't report
	// rows-affected reliably for diagnostic purposes — easier to
	// pre-check via TagsFor.
	existing, err := s.TagsFor(k, ref, orgUser)
	if err != nil {
		return Result{}, err
	}
	already := false
	for _, t := range existing {
		if t.ID == tagID {
			already = true
			break
		}
	}
	if err := s.ApplyTag(tagID, k, ref, orgUser); err != nil {
		if errors.Is(err, devproject.ErrTagNotFound) {
			return Result{}, ErrNotFound{ID: tagID}
		}
		return Result{}, err
	}
	return Result{Tag: cur, Changed: !already}, nil
}

// Remove unbinds a tag from an item. Changed is true when a row was
// actually deleted, false when the binding didn't exist.
func Remove(s *devproject.Store, tagID int64, kind, ref, orgUser string) (Result, error) {
	if s == nil {
		return Result{}, errors.New("nil store")
	}
	if tagID == 0 {
		return Result{}, errors.New("tag id is required")
	}
	if ref == "" {
		return Result{}, errors.New("item ref is required")
	}
	k, err := parseKind(kind)
	if err != nil {
		return Result{}, err
	}
	cur, err := Show(s, tagID, "")
	if err != nil {
		return Result{}, err
	}
	existing, err := s.TagsFor(k, ref, orgUser)
	if err != nil {
		return Result{}, err
	}
	had := false
	for _, t := range existing {
		if t.ID == tagID {
			had = true
			break
		}
	}
	if err := s.RemoveTag(tagID, k, ref, orgUser); err != nil {
		return Result{}, err
	}
	return Result{Tag: cur, Changed: had}, nil
}

// Set replaces the full tag set on an item to exactly tagIDs. Mirrors
// devproject.SetTagsFor but with kind validation. Changed reports
// whether the resulting set differs from the prior state.
func Set(s *devproject.Store, kind, ref, orgUser string, tagIDs []int64) (Result, error) {
	if s == nil {
		return Result{}, errors.New("nil store")
	}
	if ref == "" {
		return Result{}, errors.New("item ref is required")
	}
	k, err := parseKind(kind)
	if err != nil {
		return Result{}, err
	}
	before, err := s.TagsFor(k, ref, orgUser)
	if err != nil {
		return Result{}, err
	}
	prev := make(map[int64]bool, len(before))
	for _, t := range before {
		prev[t.ID] = true
	}
	next := make(map[int64]bool, len(tagIDs))
	for _, id := range tagIDs {
		next[id] = true
	}
	changed := len(prev) != len(next)
	if !changed {
		for id := range next {
			if !prev[id] {
				changed = true
				break
			}
		}
	}
	if err := s.SetTagsFor(k, ref, orgUser, tagIDs); err != nil {
		return Result{}, err
	}
	return Result{Changed: changed}, nil
}

// TagsFor returns every tag bound to one item.
func TagsFor(s *devproject.Store, kind, ref, orgUser string) ([]Tag, error) {
	if s == nil {
		return nil, errors.New("nil store")
	}
	k, err := parseKind(kind)
	if err != nil {
		return nil, err
	}
	rows, err := s.TagsFor(k, ref, orgUser)
	if err != nil {
		return nil, err
	}
	out := make([]Tag, 0, len(rows))
	for _, t := range rows {
		out = append(out, fromDP(t))
	}
	return out, nil
}

// ItemsWithTag returns every item bound to a tag, optionally filtered
// to a single org.
func ItemsWithTag(s *devproject.Store, tagID int64, orgUser string) ([]Binding, error) {
	if s == nil {
		return nil, errors.New("nil store")
	}
	if _, err := Show(s, tagID, ""); err != nil {
		return nil, err
	}
	rows, err := s.ItemsWithTag(tagID, orgUser)
	if err != nil {
		return nil, err
	}
	out := make([]Binding, 0, len(rows))
	for _, b := range rows {
		out = append(out, Binding{
			TagID:     b.TagID,
			ItemKind:  string(b.ItemKind),
			ItemRef:   b.ItemRef,
			OrgUser:   b.OrgUser,
			CreatedAt: b.CreatedAt,
		})
	}
	return out, nil
}
