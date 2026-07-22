package ui

// modal_org_groups.go — modal entry points for org grouping +
// auth-lifecycle flows. The actual modal UI is delegated to the
// existing editModal (text input) and choiceModal (picker / confirm)
// primitives so the visual language stays consistent with the rest
// of the app.
//
// What lives here:
//   - openOrgGroupPrompt    : create / rename a group
//   - openOrgMoveToGroupPicker : move cursored org to another group
//   - openAddOrgChoice      : pick web / device login (Step 5)
//   - openDisconnectOrgConfirm : confirm logout (Step 6)
//   - runSetDefaultOrg      : fire-and-forget default switch (Step 6)
//   - openOrgAliasPrompt    : rename alias (Step 6)

import (
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// openOrgGroupPrompt opens a single-line text-input modal for create
// or rename. On submit calls applyOrgGroupPrompt + saves.
func (m *Model) openOrgGroupPrompt(kind orgGroupPromptKind, targetID, initial string) tea.Cmd {
	title := "New group"
	hint := "Type a group name. Enter to save · Esc to cancel."
	if kind == orgGroupPromptRename {
		title = "Rename group"
	}
	return m.openEditModal(editModalState{
		Title:       title,
		Hint:        hint,
		InitialBody: initial,
		Multiline:   false,
		Save: func(val string, _ any) error {
			name := strings.TrimSpace(val)
			if name == "" {
				return fmt.Errorf("name required")
			}
			m.applyOrgGroupPrompt(kind, targetID, name)
			return nil
		},
		OnSuccess: func() tea.Cmd {
			return func() tea.Msg { return orgGroupsChangedMsg{} }
		},
	})
}

// openOrgMoveToGroupPicker shows a choice modal listing every group
// (plus "Ungrouped" at the bottom). On select, the cursored org's
// username is removed from its current group and appended to the
// selected one. Caller has already verified the rail cursor is on
// an org row.
func (m *Model) openOrgMoveToGroupPicker(username string) tea.Cmd {
	groups := m.settings.OrgGroups()
	if len(groups) == 0 {
		// Nothing to pick. Surfacing a flash beats opening an empty
		// modal that immediately closes.
		m.flash("no groups yet — press " + firstPretty(Keys.OrgGroupCreate) + " to create one")
		return nil
	}
	current := m.settings.OrgGroupForUsername(username)

	options := make([]choiceOption, 0, len(groups)+2)
	cursor := 0
	for i, g := range groups {
		hint := ""
		if g.ID == current {
			hint = "current"
		}
		if g.ID == current {
			cursor = i
		}
		options = append(options, choiceOption{
			Label: g.Name,
			Hint:  hint,
			Value: g.ID,
		})
	}
	// "Ungrouped" — moves the org out of every group.
	if current != "" {
		// Only meaningful when the org is currently in a group.
		options = append(options, choiceOption{
			Label: "Ungrouped",
			Hint:  "remove from any group",
			Value: ungroupedID,
		})
	}
	options = append(options, choiceOption{Label: "Cancel", Cancel: true})

	return m.openChoiceModal(choiceModalState{
		Title:      "Move to group…",
		Hint:       fmt.Sprintf("Move %s to:", username),
		Options:    options,
		Cursor:     cursor,
		Searchable: len(groups) > 6,
		Save: func(val any) error {
			targetID, _ := val.(string)
			m.applyMoveOrgToGroup(username, targetID)
			return nil
		},
		OnSuccess: func() tea.Cmd {
			return func() tea.Msg { return orgGroupsChangedMsg{} }
		},
	})
}

// applyMoveOrgToGroup removes username from any current group and
// appends it to targetID. ungroupedID strips group membership
// without re-adding anywhere. No-op when target == current.
func (m *Model) applyMoveOrgToGroup(username, targetID string) {
	groups := m.settings.OrgGroups()
	// Strip from any current group.
	for i, g := range groups {
		out := g.Members[:0]
		for _, u := range g.Members {
			if u == username {
				continue
			}
			out = append(out, u)
		}
		groups[i].Members = out
	}
	// Append to target.
	if targetID != ungroupedID {
		idx, _ := findGroupByID(groups, targetID)
		if idx >= 0 {
			groups[idx].Members = append(groups[idx].Members, username)
		}
	}
	m.settings.SetOrgGroups(groups)
	m.saveSettings("")
	m.syncOrgRailCursorToOrg(username)
}

// openAddOrgChoice shows the add-org method picker. Each option
// kicks off a different `sf org login *` flow via tea.ExecProcess
// so the user can interact with the sf CLI directly.
//
// Web flow chains into an instance-kind picker so users can target
// sandboxes / custom My Domains / pre-release pods without leaving
// sf-deck. sfdx-url skips that (the auth URL embeds the host).
//
// Chained sub-modals are opened by emitting addOrgFlowStepMsg from
// OnSuccessTyped — the dispatcher's value-receiver applyChoiceModalResult
// closes over a stale Model snapshot, so any closure that calls
// m.openX directly would mutate the WRONG Model and the new modal
// would silently fail to render. The msg round-trip puts the step
// transition on the live Model.
func (m *Model) openAddOrgChoice() tea.Cmd {
	return m.openChoiceModal(choiceModalState{
		Title: "Add org · method",
		Hint:  "Pick how to authenticate.",
		Options: []choiceOption{
			{Label: "Web login (browser)", Hint: "sf org login web — recommended", Value: "web"},
			{Label: "Paste sfdx auth URL", Hint: "sf org login sfdx-url — transfer auth from another machine", Value: "sfdx-url"},
			{Label: "Cancel", Cancel: true},
		},
		Save: func(val any) error {
			return nil
		},
		OnSuccess: func() tea.Cmd { return nil },
		OnSuccessTyped: func(val any) tea.Cmd {
			method, _ := val.(string)
			return func() tea.Msg {
				return addOrgFlowStepMsg{Step: "method_picked", Method: method}
			}
		},
	})
}

// addOrgFlowStepMsg drives the multi-step add-org flow. Each step's
// OnSuccessTyped emits one of these so the dispatcher can call the
// next openX on the live Model (closures captured at modal-build
// time see a stale copy — see openAddOrgChoice comment).
type addOrgFlowStepMsg struct {
	Step        string // "method_picked" | "instance_picked" | "custom_url"
	Method      string // "web" | "sfdx-url"
	InstanceURL string // populated for instance_picked / custom_url
}

// openAddOrgInstanceChoice is step 2 of the web-login flow — pick
// which Salesforce login endpoint to target. Standard production
// + sandbox are the headline options; custom + pre-release sit
// below for the long tail.
func (m *Model) openAddOrgInstanceChoice() tea.Cmd {
	return m.openChoiceModal(choiceModalState{
		Title: "Add org · instance",
		Hint:  "Pick the org's login endpoint. Custom is for My Domain URLs.",
		Options: []choiceOption{
			{Label: "Production", Hint: "https://login.salesforce.com", Value: "https://login.salesforce.com"},
			{Label: "Sandbox", Hint: "https://test.salesforce.com", Value: "https://test.salesforce.com"},
			{Label: "Custom My Domain…", Hint: "Prompt for the full https://my-domain.my.salesforce.com URL", Value: "__custom__"},
			{Label: "Pre-release", Hint: "https://prerellogin.pre.salesforce.com", Value: "https://prerellogin.pre.salesforce.com"},
			{Label: "Cancel", Cancel: true},
		},
		Save:      func(val any) error { return nil },
		OnSuccess: func() tea.Cmd { return nil },
		OnSuccessTyped: func(val any) tea.Cmd {
			url, _ := val.(string)
			return func() tea.Msg {
				return addOrgFlowStepMsg{Step: "instance_picked", Method: "web", InstanceURL: url}
			}
		},
	})
}

// openAddOrgCustomURLPrompt is step 3 (custom branch only) — collect
// the user's My Domain URL via a single-line input. Validates the
// https:// scheme client-side so we fail fast instead of letting sf
// open a browser tab that immediately errors.
//
// Save captures the validated URL into a closure-scoped pointer
// then OnSuccess emits addOrgFlowStepMsg so the dispatcher fires
// the login flow on the live Model (same reason as the choice-modal
// chained steps).
func (m *Model) openAddOrgCustomURLPrompt() tea.Cmd {
	var captured string
	return m.openEditModal(editModalState{
		Title:       "Add org · custom URL",
		Hint:        "Paste the org's My Domain URL (e.g. https://acme.my.salesforce.com or https://acme--uat.sandbox.my.salesforce.com). Enter to continue · Esc to cancel.",
		InitialBody: "https://",
		Multiline:   false,
		Save: func(val string, _ any) error {
			val = strings.TrimSpace(val)
			if err := sf.ValidateInstanceURL(val); err != nil {
				return err
			}
			captured = val
			return nil
		},
		OnSuccess: func() tea.Cmd {
			url := captured
			return func() tea.Msg {
				return addOrgFlowStepMsg{Step: "custom_url", Method: "web", InstanceURL: url}
			}
		},
	})
}

// openDisconnectOrgConfirm — Step 6. Confirm modal, then sf.Logout.
func (m *Model) openDisconnectOrgConfirm(o sf.Org) tea.Cmd {
	label := o.Display()
	if label == "" {
		label = o.Username
	}
	return m.openChoiceModal(choiceModalState{
		Title: "Disconnect org",
		Hint:  fmt.Sprintf("Run sf org logout for %s? This only removes local creds — the user / connected app are unchanged in Salesforce.", label),
		Options: []choiceOption{
			{Label: "Cancel", Cancel: true},
			{Label: "Disconnect", Hint: "sf org logout --target-org " + o.Username, Value: o.Username},
		},
		Cursor: 0, // default to Cancel — destructive
		Save: func(val any) error {
			username, _ := val.(string)
			return sf.Logout(username)
		},
		OnSuccess: func() tea.Cmd {
			return func() tea.Msg { return orgsChangedMsg{} }
		},
	})
}

// runSetDefaultDevHub mirrors runSetDefaultOrg but writes the
// target-dev-hub config key. DevHub default is what `sf org create
// scratch` reaches for when no --target-dev-hub is passed.
func (m *Model) runSetDefaultDevHub(o sf.Org) tea.Cmd {
	target := o.Alias
	if target == "" {
		target = o.Username
	}
	return func() tea.Msg {
		if err := sf.SetDefaultDevHub(target); err != nil {
			return orgLifecycleResultMsg{
				Op:      "set-default-devhub",
				Err:     err,
				Message: "set default DevHub failed: " + err.Error(),
			}
		}
		return orgLifecycleResultMsg{
			Op:      "set-default-devhub",
			Message: "default DevHub → " + target,
			Refetch: true,
		}
	}
}

// openUnsetAliasConfirm asks for confirmation before clearing the
// org's alias. The underlying authed org is unchanged — only the
// alias is removed from sfdx's alias store. After unset the org
// shows up under its username only (until a new alias is set).
//
// No-op when the org has no alias to begin with.
func (m *Model) openUnsetAliasConfirm(o sf.Org) tea.Cmd {
	if strings.TrimSpace(o.Alias) == "" {
		m.flash("no alias to clear")
		return nil
	}
	return m.openChoiceModal(choiceModalState{
		Title: "Clear alias",
		Hint:  fmt.Sprintf("Remove the alias %q from sfdx? The org stays authed; only the alias is dropped.", o.Alias),
		Options: []choiceOption{
			{Label: "Cancel", Cancel: true},
			{Label: "Clear alias", Hint: "sf alias unset " + o.Alias, Value: o.Alias},
		},
		Cursor: 0,
		Save: func(val any) error {
			alias, _ := val.(string)
			return sf.UnsetAlias(alias)
		},
		OnSuccess: func() tea.Cmd {
			return func() tea.Msg { return orgsChangedMsg{} }
		},
	})
}

// runSetDefaultOrg — Step 6. Fire-and-forget; flashes on completion
// via orgLifecycleResultMsg which Update routes into m.flash + a
// rail refetch.
func (m *Model) runSetDefaultOrg(o sf.Org) tea.Cmd {
	target := o.Alias
	if target == "" {
		target = o.Username
	}
	return func() tea.Msg {
		if err := sf.SetDefault(target); err != nil {
			return orgLifecycleResultMsg{
				Op:      "set-default",
				Err:     err,
				Message: "set default failed: " + err.Error(),
			}
		}
		return orgLifecycleResultMsg{
			Op:      "set-default",
			Message: "default org → " + target,
			Refetch: true,
		}
	}
}

// openOrgAliasPrompt — Step 6. Single-line input, prefilled with
// existing alias.
func (m *Model) openOrgAliasPrompt(o sf.Org) tea.Cmd {
	return m.openEditModal(editModalState{
		Title:       "Rename alias",
		Hint:        fmt.Sprintf("New alias for %s. Enter to save · Esc to cancel.", o.Username),
		InitialBody: o.Alias,
		Multiline:   false,
		Save: func(val string, _ any) error {
			alias := strings.TrimSpace(val)
			if alias == "" {
				return fmt.Errorf("alias required")
			}
			return sf.SetAlias(o.Username, alias)
		},
		OnSuccess: func() tea.Cmd {
			return func() tea.Msg { return orgsChangedMsg{} }
		},
	})
}

// orgGroupsChangedMsg signals that the persisted org-group state has
// changed and the rail should re-render. The rail reads
// m.settings.OrgGroups() on every render so this msg is mostly a
// post-modal nudge for syncing the rail cursor.
type orgGroupsChangedMsg struct{}

// orgsChangedMsg signals that the underlying authed-org list may
// have changed (login / logout / alias / default). Triggers a
// Refetch on m.orgsRes so the rail picks up new data.
type orgsChangedMsg struct{}

// orgLifecycleResultMsg carries the outcome of an auth-lifecycle
// goroutine (set-default, logout, login, alias). Op identifies the
// action; Err is non-nil on failure. Refetch tells the receiver
// whether to kick m.orgsRes.Refetch() — set on success only.
type orgLifecycleResultMsg struct {
	Op      string
	Err     error
	Message string
	Refetch bool
}

// startLoginFlow runs `sf org login web|sfdx-url` via tea.ExecProcess
// — bubbletea suspends the alt-screen, the user interacts directly
// with sf in their terminal, and on return the TUI resumes. After
// completion we kick orgsRes.Refetch via orgsChangedMsg so the new
// org appears in the rail (in Ungrouped, since the user just added
// it).
//
// instanceURL is the resolved login endpoint for web flow (already
// validated upstream — see ValidateInstanceURL). Ignored for sfdx-url
// because the auth URL embeds the host.
//
// Skip the alias prompt up front: `sf` itself prompts when no
// --alias is passed and the user types it inline in the same flow
// that opens the browser. Adding an extra TUI step would just
// double the input cost.
func (m *Model) startLoginFlow(method, instanceURL string) tea.Cmd {
	if Demo {
		// Demo mode must never launch a real `sf org login` browser
		// flow. Open a local explainer page instead so the gesture
		// still does something visible + honest.
		browser := ""
		if m.settings != nil {
			browser = m.settings.Browser()
		}
		return func() tea.Msg {
			_ = demoAddOrgPage(browser)
			return demoFlashMsg{text: "demo: org login is disabled — opened an explainer"}
		}
	}
	var cmd *exec.Cmd
	switch method {
	case "web":
		cmd = sf.LoginWebCommand("", instanceURL)
	case "sfdx-url":
		// sf reads the URL from stdin, so the user pastes it into
		// the suspended terminal. Cmd inherits stdin from tea.Exec.
		cmd = sf.LoginSfdxURLCommand("")
	default:
		return nil
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return orgLifecycleResultMsg{
				Op:      "login",
				Err:     err,
				Message: "login failed: " + err.Error(),
			}
		}
		return orgLifecycleResultMsg{
			Op:      "login",
			Message: "login complete — refreshing org list",
			Refetch: true,
		}
	})
}

// Compile-time guard so the settings import stays referenced — we
// touch the type indirectly via settings.OrgGroupConfig in callers
// across the package.
var _ = settings.OrgGroupConfig{}
