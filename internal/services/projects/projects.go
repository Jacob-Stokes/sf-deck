// Package projects is the service layer for DevProjects — the
// cross-org collections that group sObjects, fields, flows, records,
// queries, etc. together for a piece of work.
//
// Same pattern as services/chips and services/tags: typed public
// views, validated input, idempotent writes where it makes sense,
// typed errors that headless wrappers map to JSON error codes.
//
// DevProjects use a caller-provided string ID so the persistence
// layer doesn't have to round-trip an auto-increment back. Headless
// callers either pass --id (rare) or let the service generate one;
// the TUI's chip-manager flow does the same via internal/ui/newID.
//
// What lives here:
//
//   - Project / Item public views (JSON-tagged, decoupled from
//     devproject.DevProject / devproject.Item).
//   - CRUD on projects (list / show / create / update / delete).
//   - Item ops (add / remove / list).
//   - Typed errors (ErrNotFound, ErrNotEmpty, ErrInvalidKind).
//
// What does NOT live here:
//
//   - Collect logic (the "given a UI selection, fold the items into a
//     project" code lives in devproject/collect.go + UI).
//   - Salesforce hydration. AddItem just stores name/ref/kind — it
//     does not validate that the Ref exists in the org.
package projects

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

// Project is the headless-facing view. ItemCount is filled by List
// (single COUNT join) and Show (cheap loop); omitted from Create /
// Update responses because the caller just created the row + knows
// the count is zero / unchanged.
type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	TouchedAt   time.Time `json:"touched_at"`
	ItemCount   int       `json:"item_count,omitempty"`
}

// Item is the headless-facing view of one collected item.
type Item struct {
	DevProjectID string    `json:"project_id"`
	OrgUser      string    `json:"org_user,omitempty"`
	Kind         string    `json:"kind"`
	Ref          string    `json:"ref"`
	Type         string    `json:"type,omitempty"`
	Name         string    `json:"name,omitempty"`
	Notes        string    `json:"notes,omitempty"`
	Namespace    string    `json:"namespace,omitempty"`
	Managed      bool      `json:"managed,omitempty"`
	AddedAt      time.Time `json:"added_at"`
}

// Result is the typed write outcome. Project is the post-write
// project state; Changed reports whether the call mutated state.
type Result struct {
	Project Project
	Changed bool
}

// ItemResult is the write outcome for AddItem / RemoveItem.
type ItemResult struct {
	Item    Item
	Changed bool
}

// ErrNotFound is returned when a project lookup misses.
type ErrNotFound struct {
	ID   string
	Name string
}

func (e ErrNotFound) Error() string {
	if e.Name != "" {
		return fmt.Sprintf("project %q not found", e.Name)
	}
	return fmt.Sprintf("project %s not found", e.ID)
}

// ErrNotEmpty is returned by Delete when the project still owns
// items and force=false. Mirrors devproject.ErrNotEmpty but typed so
// the headless wrapper can attach extra context (item count) without
// branching on the underlying sentinel.
type ErrNotEmpty struct {
	ID    string
	Items int
}

func (e ErrNotEmpty) Error() string {
	return fmt.Sprintf("project %s has %d items; pass --force to cascade", e.ID, e.Items)
}

// ErrInvalidKind is the same shape as services/tags.ErrInvalidKind.
// Duplicated rather than imported so the two services aren't
// coupled — each service owns its own kind set and headless errors.
type ErrInvalidKind struct {
	Kind string
}

func (e ErrInvalidKind) Error() string {
	return fmt.Sprintf("unknown item kind %q (want one of %s)",
		e.Kind, strings.Join(KnownKinds(), ", "))
}

// ErrInvalidRef fires when --ref has the wrong shape for the kind.
// The common case it catches: an agent that fetched the wrong
// flavour of a near-identical Id (e.g. FlowDefinitionView.Id vs.
// the durable FlowDefinition.Id). Hint names the field the caller
// almost certainly wanted, so the message is recoverable in one read.
type ErrInvalidRef struct {
	Kind string
	Ref  string
	Hint string
}

func (e ErrInvalidRef) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("ref %q is not valid for kind %q: %s",
			e.Ref, e.Kind, e.Hint)
	}
	return fmt.Sprintf("ref %q is not valid for kind %q", e.Ref, e.Kind)
}

// validateRef rejects known-bad ref shapes for the given Kind. Only
// the prefixes we can be CERTAIN are wrong are checked — anything
// else falls through. Errs on the side of permissive to avoid
// breaking ad-hoc tests or future Id-prefix changes.
func validateRef(kind devproject.ItemKind, ref string) error {
	switch kind {
	case devproject.KindFlow:
		// FlowDefinition.Id is 300...; FlowDefinitionView.Id is 3dd.
		// Both are 18 chars, share a prefix, easy to mix up.
		if strings.HasPrefix(ref, "3dd") {
			return ErrInvalidRef{
				Kind: string(kind),
				Ref:  ref,
				Hint: "looks like FlowDefinitionView.Id (3dd...); want FlowDefinition.Id (300...) — query FlowDefinitionView.DurableId",
			}
		}
	case devproject.KindFlowVersion:
		// Flow.Id is 301...; same Tooling API as KindFlow.
		if strings.HasPrefix(ref, "3dd") {
			return ErrInvalidRef{
				Kind: string(kind),
				Ref:  ref,
				Hint: "looks like FlowDefinitionView.Id (3dd...); want Flow.Id (301...) from the Tooling API",
			}
		}
	case devproject.KindField:
		// Fields are stored as <SObject>.<FieldApi>, never bare Ids.
		if !strings.Contains(ref, ".") {
			return ErrInvalidRef{
				Kind: string(kind),
				Ref:  ref,
				Hint: "expected <SObject>.<FieldApiName> (e.g. Account.Phone)",
			}
		}
	}
	return nil
}

// validKinds mirrors services/tags. Kept independent so adding a
// project-only kind doesn't accidentally enable tag bindings for it
// (or vice versa).
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

// KnownKinds returns the closed set, sorted.
func KnownKinds() []string {
	out := make([]string, 0, len(validKinds))
	for k := range validKinds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func parseKind(s string) (devproject.ItemKind, error) {
	k, ok := validKinds[s]
	if !ok {
		return "", ErrInvalidKind{Kind: s}
	}
	return k, nil
}

// newID generates a short hex id. Matches the TUI's newID — 12 random
// bytes ≈ 96 bits, plenty for a local store. Caller may override via
// CreateInput.ID for deterministic tests or scripted rollouts.
func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func fromDP(p devproject.DevProject) Project {
	return Project{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		CreatedAt:   p.CreatedAt,
		TouchedAt:   p.TouchedAt,
	}
}

func itemFromDP(it devproject.Item) Item {
	return Item{
		DevProjectID: it.DevProjectID,
		OrgUser:      it.OrgUser,
		Kind:         string(it.Kind),
		Ref:          it.Ref,
		Type:         it.Type,
		Name:         it.Name,
		Notes:        it.Notes,
		Namespace:    it.Namespace,
		Managed:      it.Managed(),
		AddedAt:      it.AddedAt,
	}
}

// List returns every project with item counts. Sorted by TouchedAt
// desc — matches the underlying store order, makes the wire shape
// match what the TUI shows.
func List(s *devproject.Store) ([]Project, error) {
	if s == nil {
		return nil, errors.New("nil store")
	}
	rows, err := s.ListDevProjects()
	if err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(rows))
	for _, p := range rows {
		items, err := s.ListItems(p.ID, "")
		if err != nil {
			return nil, err
		}
		v := fromDP(p)
		v.ItemCount = len(items)
		out = append(out, v)
	}
	return out, nil
}

// Show looks up a single project by ID or by name. Exactly one must
// be set. Returns ErrNotFound on miss.
func Show(s *devproject.Store, id, name string) (Project, error) {
	if s == nil {
		return Project{}, errors.New("nil store")
	}
	if id != "" && name != "" {
		return Project{}, errors.New("specify --id OR --name, not both")
	}
	if id == "" && name == "" {
		return Project{}, errors.New("--id or --name is required")
	}
	if id != "" {
		p, err := s.GetDevProject(id)
		if err != nil {
			return Project{}, err
		}
		if p == nil {
			return Project{}, ErrNotFound{ID: id}
		}
		out := fromDP(*p)
		items, err := s.ListItems(id, "")
		if err != nil {
			return Project{}, err
		}
		out.ItemCount = len(items)
		return out, nil
	}
	// Name lookup. ListDevProjects + linear scan: typical user has
	// <100 projects, so this is cheap, and the store doesn't expose a
	// direct name lookup.
	all, err := s.ListDevProjects()
	if err != nil {
		return Project{}, err
	}
	for _, p := range all {
		if strings.EqualFold(p.Name, name) {
			out := fromDP(p)
			items, err := s.ListItems(p.ID, "")
			if err != nil {
				return Project{}, err
			}
			out.ItemCount = len(items)
			return out, nil
		}
	}
	return Project{}, ErrNotFound{Name: name}
}

// CreateInput is the validated input. ID is optional — when empty,
// service generates one. Letting the caller supply an ID is useful
// for scripted migrations + deterministic tests.
type CreateInput struct {
	ID          string
	Name        string
	Description string
}

// Validate runs the input check Create runs. Exposed for the CLI
// parser.
func (in CreateInput) Validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name is required")
	}
	return nil
}

// Create inserts a new project. Returns the typed Project (with the
// generated ID populated).
func Create(s *devproject.Store, in CreateInput) (Result, error) {
	if s == nil {
		return Result{}, errors.New("nil store")
	}
	if err := in.Validate(); err != nil {
		return Result{}, err
	}
	p := devproject.DevProject{
		ID:          in.ID,
		Name:        strings.TrimSpace(in.Name),
		Description: in.Description,
	}
	if p.ID == "" {
		p.ID = newID()
	}
	if err := s.CreateDevProject(p); err != nil {
		return Result{}, err
	}
	// Re-read so CreatedAt / TouchedAt populated from the row.
	out, err := s.GetDevProject(p.ID)
	if err != nil {
		return Result{}, err
	}
	if out == nil {
		return Result{}, errors.New("created project not found on re-read")
	}
	return Result{Project: fromDP(*out), Changed: true}, nil
}

// UpdateInput is partial-update shape.
type UpdateInput struct {
	Name        *string
	Description *string
}

// HasAny reports whether at least one update field is set.
func (in UpdateInput) HasAny() bool {
	return in.Name != nil || in.Description != nil
}

// Update applies a partial update. Returns ErrNotFound if missing,
// Changed=false when inputs match the existing state.
func Update(s *devproject.Store, id string, in UpdateInput) (Result, error) {
	if s == nil {
		return Result{}, errors.New("nil store")
	}
	if id == "" {
		return Result{}, errors.New("id is required")
	}
	if !in.HasAny() {
		return Result{}, errors.New("no update fields specified")
	}
	cur, err := s.GetDevProject(id)
	if err != nil {
		return Result{}, err
	}
	if cur == nil {
		return Result{}, ErrNotFound{ID: id}
	}
	name := cur.Name
	desc := cur.Description
	changed := false
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return Result{}, errors.New("name is required")
		}
		if *in.Name != name {
			name = *in.Name
			changed = true
		}
	}
	if in.Description != nil && *in.Description != desc {
		desc = *in.Description
		changed = true
	}
	if !changed {
		return Result{Project: fromDP(*cur), Changed: false}, nil
	}
	if err := s.UpdateDevProject(id, name, desc); err != nil {
		return Result{}, err
	}
	out, err := s.GetDevProject(id)
	if err != nil {
		return Result{}, err
	}
	return Result{Project: fromDP(*out), Changed: true}, nil
}

// Delete removes the project. With force=false, returns ErrNotEmpty
// (and leaves state untouched) when the project still owns items.
// With force=true, cascades.
func Delete(s *devproject.Store, id string, force bool) (Result, error) {
	if s == nil {
		return Result{}, errors.New("nil store")
	}
	if id == "" {
		return Result{}, errors.New("id is required")
	}
	cur, err := s.GetDevProject(id)
	if err != nil {
		return Result{}, err
	}
	if cur == nil {
		return Result{}, ErrNotFound{ID: id}
	}
	snapshot := fromDP(*cur)
	if err := s.DeleteDevProject(id, force); err != nil {
		if errors.Is(err, devproject.ErrNotEmpty) {
			items, _ := s.ListItems(id, "")
			return Result{}, ErrNotEmpty{ID: id, Items: len(items)}
		}
		return Result{}, err
	}
	return Result{Project: snapshot, Changed: true}, nil
}

// AddItemInput is the validated input for binding an item to a
// project. Type / Name / Notes / Namespace are optional context.
type AddItemInput struct {
	ProjectID string
	OrgUser   string
	Kind      string
	Ref       string
	Type      string
	Name      string
	Notes     string
	Namespace string
}

// AddItem inserts an item into a project. Idempotent at the store
// layer (INSERT OR IGNORE); Changed reflects whether a new row was
// actually inserted vs. an existing collision.
func AddItem(s *devproject.Store, in AddItemInput) (ItemResult, error) {
	if s == nil {
		return ItemResult{}, errors.New("nil store")
	}
	if in.ProjectID == "" {
		return ItemResult{}, errors.New("project id is required")
	}
	if in.Ref == "" {
		return ItemResult{}, errors.New("item ref is required")
	}
	k, err := parseKind(in.Kind)
	if err != nil {
		return ItemResult{}, err
	}
	if err := validateRef(k, in.Ref); err != nil {
		return ItemResult{}, err
	}
	// Pre-check project exists so the error is typed (not_found vs.
	// a silent no-op).
	cur, err := s.GetDevProject(in.ProjectID)
	if err != nil {
		return ItemResult{}, err
	}
	if cur == nil {
		return ItemResult{}, ErrNotFound{ID: in.ProjectID}
	}
	it := devproject.Item{
		DevProjectID: in.ProjectID,
		OrgUser:      in.OrgUser,
		Kind:         k,
		Ref:          in.Ref,
		Type:         in.Type,
		Name:         in.Name,
		Notes:        in.Notes,
		Namespace:    in.Namespace,
		// Stamp AddedAt ourselves rather than letting the store fill
		// it in — devproject.AddItem takes Item by value, so we
		// wouldn't see the timestamp it assigns.
		AddedAt: time.Now(),
	}
	added, err := s.AddItem(it)
	if err != nil {
		return ItemResult{}, err
	}
	return ItemResult{Item: itemFromDP(it), Changed: added}, nil
}

// RemoveItem deletes one item binding. Changed reflects whether the
// row existed before the delete (devproject.RemoveItem is silently
// idempotent — we pre-check via ListItems so the wire shape can
// distinguish).
func RemoveItem(s *devproject.Store, projectID, orgUser, kind, ref string) (ItemResult, error) {
	if s == nil {
		return ItemResult{}, errors.New("nil store")
	}
	if projectID == "" {
		return ItemResult{}, errors.New("project id is required")
	}
	if ref == "" {
		return ItemResult{}, errors.New("item ref is required")
	}
	k, err := parseKind(kind)
	if err != nil {
		return ItemResult{}, err
	}
	items, err := s.ListItems(projectID, "")
	if err != nil {
		return ItemResult{}, err
	}
	var match *devproject.Item
	for i := range items {
		it := items[i]
		if it.OrgUser == orgUser && it.Kind == k && it.Ref == ref {
			match = &it
			break
		}
	}
	if match == nil {
		return ItemResult{
			Item:    Item{DevProjectID: projectID, OrgUser: orgUser, Kind: kind, Ref: ref},
			Changed: false,
		}, nil
	}
	if err := s.RemoveItem(projectID, orgUser, k, ref); err != nil {
		return ItemResult{}, err
	}
	return ItemResult{Item: itemFromDP(*match), Changed: true}, nil
}

// ListItems returns every item in a project. orgUser="" returns all
// orgs' contributions; pass a specific username to filter.
func ListItems(s *devproject.Store, projectID, orgUser string) ([]Item, error) {
	if s == nil {
		return nil, errors.New("nil store")
	}
	if projectID == "" {
		return nil, errors.New("project id is required")
	}
	cur, err := s.GetDevProject(projectID)
	if err != nil {
		return nil, err
	}
	if cur == nil {
		return nil, ErrNotFound{ID: projectID}
	}
	rows, err := s.ListItems(projectID, orgUser)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(rows))
	for _, r := range rows {
		out = append(out, itemFromDP(r))
	}
	return out, nil
}
