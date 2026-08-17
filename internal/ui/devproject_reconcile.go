package ui

// Automatic dev-project reconcile.

import (
	"fmt"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

// reconcileDevProject runs an automatic reconcile of one project across
// every org whose data is currently loaded, applies the plan, and
// flashes a one-line summary when anything changed. Cheap no-op when
// there's nothing to fix, so it's safe to call on every project touch.
func (m *Model) reconcileDevProject(projectID string) {
	if m.devProjects == nil || projectID == "" {
		return
	}
	items, err := m.devProjects.ListItems(projectID, "")
	if err != nil || len(items) == 0 {
		return
	}
	deletes, rewrites := m.planReconcile(projectID, items)
	if len(deletes) == 0 && len(rewrites) == 0 {
		return
	}
	removed, merged, err := m.devProjects.ApplyReconcile(deletes, rewrites)
	if err != nil {
		applog.Warn("devproject.reconcile_failed", map[string]any{"err": err.Error()})
		return
	}
	m.reconcileTags()
	if removed == 0 && merged == 0 {
		return
	}
	m.reloadDevProjects()
	if m.tab() == TabDevProjectDetail && m.devProjectCur == projectID {
		m.reloadDevProjectItems()
	}
	m.flash(reconcileSummary(removed, merged))
}

// reconcileTags is the tag-binding counterpart of reconcileDevProject.
// Tags share the same (kind, ref, org) identity model, so they carry
// the same two risks: a binding on a resource deleted in the org, and
// (historically) a non-canonical ref. It reuses the SAME existence
// oracles, so the safety rule is identical: only act on loaded data,
// only against the item's own org. Cheap no-op when nothing's stale.
func (m *Model) reconcileTags() {
	if m.devProjects == nil {
		return
	}
	bound, err := m.devProjects.ListBoundItems()
	if err != nil || len(bound) == 0 {
		return
	}
	var deletes []devproject.TagBindingDelete
	var rewrites []devproject.TagBindingRewrite

	cache := map[string]*oracleData{}
	get := func(org string, kind devproject.ItemKind) *oracleData {
		key := org + "|" + string(kind)
		if o, ok := cache[key]; ok {
			return o
		}
		o := m.buildOracle(org, kind)
		cache[key] = o
		return o
	}

	for _, b := range bound {
		o := get(b.OrgUser, b.Kind)
		if o == nil || !o.loaded {
			continue
		}
		if b.Kind == devproject.KindFlow {
			if canonical, isDevName := o.byDevName[b.Ref]; isDevName && canonical != b.Ref {
				rewrites = append(rewrites, devproject.TagBindingRewrite{
					Kind: b.Kind, OrgUser: b.OrgUser, FromRef: b.Ref, ToRef: canonical,
				})
				continue
			}
		}
		if !o.present(b.Ref) {
			deletes = append(deletes, devproject.TagBindingDelete(b))
		}
	}
	if len(deletes) == 0 && len(rewrites) == 0 {
		return
	}
	removed, merged, err := m.devProjects.ReconcileTagBindings(deletes, rewrites)
	if err != nil {
		applog.Warn("tags.reconcile_failed", map[string]any{"err": err.Error()})
		return
	}
	if removed == 0 && merged == 0 {
		return
	}
	m.flash(tagReconcileSummary(removed, merged))
}

func tagReconcileSummary(removed, merged int) string {
	switch {
	case merged > 0 && removed > 0:
		return fmt.Sprintf("tags: %d normalised, %d stale removed", merged, removed)
	case merged > 0:
		return fmt.Sprintf("tags: %d binding%s normalised", merged, plural(merged))
	default:
		return fmt.Sprintf("tags: %d stale binding%s removed", removed, plural(removed))
	}
}

func reconcileSummary(removed, merged int) string {
	dup := func(n int) string { return fmt.Sprintf("%d duplicate%s", n, plural(n)) }
	miss := func(n int) string { return fmt.Sprintf("%d missing item%s", n, plural(n)) }
	switch {
	case merged > 0 && removed > 0:
		return "dev project: " + dup(merged) + " merged, " + miss(removed) + " removed"
	case merged > 0:
		return "dev project: " + dup(merged) + " merged"
	default:
		return "dev project: " + miss(removed) + " removed"
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

type oracleData struct {
	loaded        bool
	refs          map[string]bool   // known-present canonical refs
	byDevName     map[string]string // DeveloperName -> canonical ref (flows)
	names         map[string]string // canonical ref -> display name
	parentObjects map[string]bool   // field mode: present parent sObjects
	fieldMode     bool              // ref is "<sobject>.<field>"; check parent
}

func (o *oracleData) present(ref string) bool {
	if o.fieldMode {
		obj := ref
		if i := strings.IndexByte(ref, '.'); i > 0 {
			obj = ref[:i]
		}
		return o.parentObjects[obj]
	}
	return o.refs[ref]
}

// planReconcile builds the delete + rewrite plan. It only reconciles
// kinds whose resource list is loaded for the item's org — untouched
// otherwise (never delete on not-fetched data).
func (m *Model) planReconcile(projectID string, items []devproject.Item) ([]devproject.ItemDelete, []devproject.ItemRewrite) {
	var deletes []devproject.ItemDelete
	var rewrites []devproject.ItemRewrite

	cache := map[string]*oracleData{} // key: org|kind
	get := func(org string, kind devproject.ItemKind) *oracleData {
		key := org + "|" + string(kind)
		if o, ok := cache[key]; ok {
			return o
		}
		o := m.buildOracle(org, kind)
		cache[key] = o
		return o
	}

	for _, it := range items {
		o := get(it.OrgUser, it.Kind)
		if o == nil || !o.loaded {
			continue // kind not reconcilable, or data not loaded — leave it
		}

		if it.Kind == devproject.KindFlow {
			if canonical, isDevName := o.byDevName[it.Ref]; isDevName && canonical != it.Ref {
				rewrites = append(rewrites, devproject.ItemRewrite{
					DevProjectID: projectID, OrgUser: it.OrgUser, Kind: it.Kind,
					FromRef: it.Ref, ToRef: canonical, Name: o.names[canonical],
				})
				continue // handled — don't also evaluate for deletion
			}
		}

		if !o.present(it.Ref) {
			deletes = append(deletes, devproject.ItemDelete{
				DevProjectID: projectID, OrgUser: it.OrgUser, Kind: it.Kind, Ref: it.Ref,
			})
		}
	}
	return deletes, rewrites
}

func (m *Model) buildOracle(org string, kind devproject.ItemKind) *oracleData {
	o := &oracleData{refs: map[string]bool{}, byDevName: map[string]string{}, names: map[string]string{}}
	d := m.data[org]
	if d == nil {
		return o // not loaded
	}
	switch kind {
	case devproject.KindFlow:
		if d.Flows.FetchedAt().IsZero() {
			return o
		}
		for _, f := range d.Flows.Value() {
			if f.DefinitionID != "" {
				o.refs[f.DefinitionID] = true
				nm := f.DeveloperName
				if nm == "" {
					nm = f.MasterLabel
				}
				o.names[f.DefinitionID] = nm
			}
			if f.DeveloperName != "" && f.DefinitionID != "" {
				o.byDevName[f.DeveloperName] = f.DefinitionID
			}
		}
		o.loaded = true

	case devproject.KindApexClass:
		if d.ApexClasses.FetchedAt().IsZero() {
			return o
		}
		for _, a := range d.ApexClasses.Value() {
			if a.ID != "" {
				o.refs[a.ID] = true
			}
		}
		o.loaded = true

	case devproject.KindLWC:
		if d.LWCBundles.FetchedAt().IsZero() {
			return o
		}
		for _, b := range d.LWCBundles.Value() {
			if b.ID != "" {
				o.refs[b.ID] = true
			}
		}
		o.loaded = true

	case devproject.KindSObject:
		if d.SObjects.FetchedAt().IsZero() {
			return o
		}
		for _, s := range d.SObjects.Value() {
			if s.Name != "" {
				o.refs[s.Name] = true
			}
		}
		o.loaded = true

	case devproject.KindField:
		// A field ref is "<sobject>.<field>". We can only confirm the
		// PARENT object exists (confirming the field would need a
		// describe per object). Treat the field as present when its
		// parent object is in the loaded sObject list — so we never
		// delete a field just because its describe isn't loaded.
		if d.SObjects.FetchedAt().IsZero() {
			return o
		}
		present := map[string]bool{}
		for _, s := range d.SObjects.Value() {
			if s.Name != "" {
				present[s.Name] = true
			}
		}
		// Return a specialised oracle: refs() membership is "parent
		// object present". We fold that into refs by NOT enumerating
		// (we can't), so signal via a parent set on the struct.
		o.parentObjects = present
		o.fieldMode = true
		o.loaded = true

	default:
		return o
	}
	return o
}
