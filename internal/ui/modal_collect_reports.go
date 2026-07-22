package ui

// Reports-folder collect — special case of triggerCollect for /reports.
//
// A report folder isn't an Openable (no canonical Lightning URL for a
// folder), but it IS collect-able as a bulk operation: "add every
// report in this folder (and optionally its subfolders) to a project."
// The user picks "this folder only" or "include subfolders" via a
// choice modal; both branches resolve to a flat list of report ids
// at collect time and add them as KindReport items.
//
// Why not store folder refs (KindReportFolder) and resolve at use?
// Two reasons:
//   - Projects on every other surface are static bags of leaf refs.
//     Adding a folder-ref kind would split the Scope abstraction
//     into "items" + "live folder watchers" — needlessly complex.
//   - Users know what's in their project at the moment they collect.
//     Live folder watching surprises them later when reports get
//     added to (or removed from) a folder. Re-collect to refresh.

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/treechip"
)

// triggerCollectReportFolder detects "K / ctrl+k on a /reports folder
// row" and runs the folder-collect flow. Returns (nil, false) when
// the user isn't on a report folder; the generic triggerCollect path
// then handles report rows + every other surface as before.
func (m *Model) triggerCollectReportFolder() (tea.Cmd, bool) {
	if !m.onReportsBrowser() {
		return nil, false
	}
	d := m.activeOrgData()
	if d == nil || d.ReportFolders == nil {
		return nil, false
	}
	subs, _ := m.visibleReportsItems()
	row := m.reportsRowCursor()
	if row >= len(subs) {
		// Cursor on a report row, not a folder — defer to the
		// standard collect path (cursorOpenable returns the
		// ReportSummary, FromOpenable maps it to KindReport).
		return nil, false
	}
	folder := subs[row]

	// Compute "direct" and "recursive" report sets up-front so the
	// modal options can show counts. Direct is filterReportsByFolder
	// for the folder's id; recursive walks the subtree via the
	// treechip source's Children method.
	all := d.ReportList.Items()
	direct := filterReportsByFolder(all, folder.ID)
	descIDs := collectReportFolderDescendants(d.ReportFolders, folder.ID)
	recursive := append([]sf.ReportSummary(nil), direct...)
	for fid := range descIDs {
		recursive = append(recursive, filterReportsByFolder(all, fid)...)
	}

	// Resolve the dev-project picker once — same idempotent
	// "must have a project to collect into" check as triggerCollect.
	user := m.orgs[m.selected].Username
	dps, err := m.devProjects.ListDevProjects()
	if err != nil {
		m.flash("collect: " + err.Error())
		return nil, true
	}
	if len(dps) == 0 {
		m.flash("no dev projects yet — open /dev-projects + press " + firstPretty(Keys.NewProject) + " to create one")
		return nil, true
	}

	// Two-step modal: pick-scope, then pick-project. The scope choice
	// closes over the report slices; the project pick writes the
	// final dev-project id + origin org into collectFolderPickedMsg.
	// No package globals — the typed-value channel threads everything.
	scopeOpts := []choiceOption{
		{
			Label: directLabel(folder.Label, len(direct)),
			Hint:  "reports directly inside this folder",
			Value: "direct",
		},
		{
			Label: recursiveLabel(folder.Label, len(recursive), len(descIDs)),
			Hint:  "reports in this folder + every subfolder",
			Value: "recursive",
		},
	}
	folderLabel := folder.Label
	finish := func(reports []sf.ReportSummary, devID string) tea.Cmd {
		return func() tea.Msg {
			return collectFolderPickedMsg{
				Reports: reports,
				DevID:   devID,
				OrgUser: user,
				Label:   folderLabel,
			}
		}
	}
	state := choiceModalState{
		Title:   "Collect from folder · " + folderLabel,
		Hint:    "Enter to choose scope  ·  Esc to cancel",
		Options: scopeOpts,
		OnSuccessTyped: func(val any) tea.Cmd {
			scope, _ := val.(string)
			reports := direct
			if scope == "recursive" {
				reports = recursive
			}
			d := m.ensureOrgData(user)
			if d.LoadedDevProjectID != "" {
				return finish(reports, d.LoadedDevProjectID)
			}
			projOpts := make([]choiceOption, 0, len(dps))
			for _, p := range dps {
				projOpts = append(projOpts, choiceOption{
					Label: p.Name,
					Value: p.ID,
				})
			}
			return m.openChoiceModal(choiceModalState{
				Title:      "Pick a project",
				Hint:       "Enter to add · adds from " + user + "  ·  Esc to cancel",
				Options:    projOpts,
				Searchable: true,
				OnSuccessTyped: func(val any) tea.Cmd {
					devID, _ := val.(string)
					return finish(reports, devID)
				},
			})
		},
	}
	return m.openChoiceModal(state), true
}

// collectFolderPickedMsg lands on the main loop after the user has
// picked both scope (this-only / recursive) and dev project. Carries
// the staged payload directly so the apply path doesn't need a
// per-flow package global.
type collectFolderPickedMsg struct {
	Reports []sf.ReportSummary
	DevID   string
	OrgUser string
	Label   string
}

// applyCollectFolderPicked persists the staged report slice as
// KindReport items under the chosen project, tagged with the
// origin org. Idempotent on (dev-project, org, kind, ref) — re-
// collecting a folder won't duplicate reports the project already
// had from this org.
func (m *Model) applyCollectFolderPicked(msg collectFolderPickedMsg) tea.Cmd {
	if m.devProjects == nil || msg.DevID == "" {
		return nil
	}
	added := 0
	for _, r := range msg.Reports {
		if r.ID == "" {
			continue
		}
		ok, err := m.devProjects.AddItem(devproject.Item{
			DevProjectID: msg.DevID,
			OrgUser:      msg.OrgUser,
			Kind:         devproject.KindReport,
			Ref:          r.ID,
			Type:         r.FolderName,
			Name:         r.Name,
		})
		if err != nil {
			applog.Error("collect.folder", map[string]any{
				"err": err.Error(),
				"ref": r.ID,
			})
			continue
		}
		if ok {
			added++
		}
	}
	m.reloadDevProjects()
	if m.tab() == TabDevProjectDetail && m.devProjectCur == msg.DevID {
		m.reloadDevProjectItems()
	}
	if len(m.orgs) > 0 {
		d := m.ensureOrgData(m.orgs[m.selected].Username)
		if d.LoadedDevProjectID == msg.DevID && msg.OrgUser == m.orgs[m.selected].Username {
			m.refreshLoadedScope(d)
		}
	}
	if added == 0 {
		m.flash("no new reports added (already in project)")
	} else {
		m.flash(intToStr(added) + " reports added from " + msg.Label)
	}
	return nil
}

// collectReportFolderDescendants returns the set of folder ids that
// are strict descendants (children, grand-children, ...) of root in
// the loaded report-folder tree. Walks the registry's TreeSource via
// Children rather than re-querying SF — the source is already
// fully-hydrated by the time the user can see the strip.
func collectReportFolderDescendants(reg *treechip.Registry, root string) map[string]bool {
	out := map[string]bool{}
	if reg == nil || root == "" {
		return out
	}
	src := reg.Source()
	if src == nil {
		return out
	}
	stack := []string{root}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		kids, err := src.Children(cur)
		if err != nil {
			continue
		}
		for _, k := range kids {
			if out[k.ID] {
				continue
			}
			out[k.ID] = true
			stack = append(stack, k.ID)
		}
	}
	return out
}

func directLabel(folder string, n int) string {
	return "This folder only · " + intToStr(n) + " reports"
}

func recursiveLabel(folder string, n, subFolders int) string {
	return "Include subfolders · " + intToStr(n) + " reports across " + intToStr(subFolders) + " subfolders"
}

// intToStr formats a non-negative int. Used by the folder-collect
// labels; we don't pull strconv in just for this.
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
