package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

type deepCollectKind int

const (
	deepKindParent deepCollectKind = iota
	deepKindCustomFields
	deepKindAllFields
	deepKindValidationRules
	deepKindRecordTypes
	deepKindTriggers
	deepKindPSGComponents
)

type deepCollectOption struct {
	Kind      deepCollectKind
	Label     string
	Picked    bool
	CountHint string
	Disabled  bool
	Hint      string
}

type deepCollectState struct {
	Title  string
	Hint   string
	Target sf.Openable
	Items  []deepCollectOption
	Cursor int
}

func (m *Model) openDeepCollect(target sf.Openable) tea.Cmd {
	switch t := target.(type) {
	case sf.SObject:
		st := m.buildDeepCollectForSObject(t)
		m.deepCollect = st
		return nil
	case sf.PermissionSetGroup:
		st := m.buildDeepCollectForPSG(t)
		m.deepCollect = st
		return nil
	}
	return nil
}

// IsDeepCollectTarget reports whether shift+K should open the wizard
// for this target (vs. the single-item flow).
func IsDeepCollectTarget(target sf.Openable) bool {
	switch target.(type) {
	case sf.SObject, sf.PermissionSetGroup:
		return true
	}
	return false
}

func (m *Model) buildDeepCollectForPSG(g sf.PermissionSetGroup) *deepCollectState {
	return &deepCollectState{
		Title:  "Collect " + g.MasterLabel + " — pick what to include",
		Hint:   "space toggle · enter add to project · esc cancel",
		Target: g,
		Items: []deepCollectOption{
			{Kind: deepKindParent, Label: "this permission set group", Picked: true, CountHint: "(1)"},
			{Kind: deepKindPSGComponents, Label: "component permission sets", Picked: true, CountHint: "(?)",
				Hint: "permsets that the PSG combines — bring them along so collected perms travel with the group"},
		},
	}
}

func (m *Model) buildDeepCollectForSObject(s sf.SObject) *deepCollectState {
	d := m.activeOrgData()
	var customN, allN int
	var customKnown bool
	if d != nil {
		if r, ok := d.Describes[s.Name]; ok && !r.FetchedAt().IsZero() {
			customKnown = true
			for _, f := range r.Value().Fields {
				allN++
				if f.Custom {
					customN++
				}
			}
		}
	}
	vrHint := "(?)"
	rtHint := "(?)"
	trHint := "(?)"
	if d != nil {
		if r, ok := d.ValidationRules.ListFor(s.Name); ok && !r.FetchedAt().IsZero() {
			vrHint = fmt.Sprintf("(%d)", len(r.Value()))
		}
		if r, ok := d.RecordTypes.ListFor(s.Name); ok && !r.FetchedAt().IsZero() {
			rtHint = fmt.Sprintf("(%d)", len(r.Value()))
		}
		if r, ok := d.Triggers.ListFor(s.Name); ok && !r.FetchedAt().IsZero() {
			trHint = fmt.Sprintf("(%d)", len(r.Value()))
		}
	}
	cfHint := "(?)"
	afHint := "(?)"
	if customKnown {
		cfHint = fmt.Sprintf("(%d)", customN)
		afHint = fmt.Sprintf("(%d)", allN)
	}

	return &deepCollectState{
		Title:  "Collect " + s.Name + " — pick what to include",
		Hint:   "space toggle · enter add to project · esc cancel",
		Target: s,
		Items: []deepCollectOption{
			{Kind: deepKindParent, Label: "this sObject", Picked: true, CountHint: "(1)"},
			{Kind: deepKindCustomFields, Label: "custom fields", Picked: true, CountHint: cfHint},
			{Kind: deepKindAllFields, Label: "all fields (incl. standard)", Picked: false, CountHint: afHint,
				Hint: "rare — pick this only when you really mean every field"},
			{Kind: deepKindValidationRules, Label: "validation rules", Picked: true, CountHint: vrHint},
			{Kind: deepKindRecordTypes, Label: "record types", Picked: true, CountHint: rtHint},
			{Kind: deepKindTriggers, Label: "triggers", Picked: false, CountHint: trHint,
				Hint: "off by default — apex deploys are usually their own change set"},
		},
	}
}

func (m Model) renderDeepCollect() string {
	if m.deepCollect == nil {
		return ""
	}
	w := modalWidth(m.width, 60, 92)
	inner := w - 4
	st := m.deepCollect

	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(theme.Fg).Bold(true).Render(st.Title))
	if st.Hint != "" {
		lines = append(lines, theme.Subtle.Render(st.Hint))
	}
	lines = append(lines, "")

	for i, opt := range st.Items {
		mark := "[ ]"
		if opt.Picked {
			mark = "[x]"
		}
		row := fmt.Sprintf("  %s  %s  %s", mark, opt.Label,
			theme.Subtle.Render(opt.CountHint))
		style := lipgloss.NewStyle().Foreground(theme.Fg)
		if opt.Disabled {
			style = lipgloss.NewStyle().Foreground(theme.FgDim)
		}
		if i == st.Cursor {
			row = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " " +
				style.Bold(true).Render(strings.TrimPrefix(row, "  "))
		} else {
			row = style.Render(row)
		}
		lines = append(lines, row)
		if opt.Hint != "" && i == st.Cursor {
			lines = append(lines, "    "+theme.Subtle.Render(opt.Hint))
		}
	}
	lines = append(lines, "")
	lines = append(lines, theme.Subtle.Render("• mutually-exclusive 'all fields' overrides 'custom fields' when both are on"))

	body := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Width(inner).
		Render(body)
}

func (m *Model) handleDeepCollectKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.deepCollect == nil {
		return *m, nil
	}
	st := m.deepCollect
	key := msg.String()
	switch key {
	case "esc":
		m.deepCollect = nil
		return *m, nil
	case "up", "k":
		if st.Cursor > 0 {
			st.Cursor--
		}
		return *m, nil
	case "down", "j":
		if st.Cursor < len(st.Items)-1 {
			st.Cursor++
		}
		return *m, nil
	case "space", " ":
		if st.Cursor >= 0 && st.Cursor < len(st.Items) {
			it := &st.Items[st.Cursor]
			if !it.Disabled {
				it.Picked = !it.Picked
			}
		}
		return *m, nil
	case "enter":
		target := st.Target
		picks := make(map[deepCollectKind]bool, len(st.Items))
		for _, opt := range st.Items {
			picks[opt.Kind] = opt.Picked
		}
		m.deepCollect = nil
		return *m, func() tea.Msg {
			return deepCollectConfirmedMsg{Target: target, Picks: picks}
		}
	}
	return *m, nil
}

type deepCollectConfirmedMsg struct {
	Target sf.Openable
	Picks  map[deepCollectKind]bool
}

type deepCollectPickedMsg struct {
	Items   []devproject.Item
	DevID   string
	OrgUser string
}

func (m *Model) applyDeepCollectConfirmed(msg deepCollectConfirmedMsg) tea.Cmd {
	if m.devProjects == nil {
		m.flash("dev-projects unavailable")
		return nil
	}
	if len(m.orgs) == 0 {
		m.flash("no org selected")
		return nil
	}
	user := m.orgs[m.selected].Username
	o := m.orgs[m.selected]

	items, err := m.resolveDeepCollect(o.Username, msg.Target, msg.Picks)
	if err != nil {
		m.flash("collect: " + err.Error())
		return nil
	}
	if len(items) == 0 {
		m.flash("nothing to collect")
		return nil
	}

	d := m.ensureOrgData(user)
	if d.LoadedDevProjectID != "" {
		devID := d.LoadedDevProjectID
		return func() tea.Msg {
			return deepCollectPickedMsg{Items: items, DevID: devID, OrgUser: user}
		}
	}

	dps, err := m.devProjects.ListDevProjects()
	if err != nil {
		m.flash("collect: " + err.Error())
		return nil
	}
	if len(dps) == 0 {
		m.flash("no dev projects yet — open /dev-projects + press " + firstPretty(Keys.NewProject) + " to create one")
		return nil
	}
	opts := make([]choiceOption, 0, len(dps))
	for _, p := range dps {
		opts = append(opts, choiceOption{
			Label: p.Name,
			Hint:  fmt.Sprintf("touched %s", humanTimeAgo(p.TouchedAt)),
			Value: p.ID,
		})
	}
	state := choiceModalState{
		Title:      fmt.Sprintf("Add %d items — pick dev project (from %s)", len(items), o.Display()),
		Hint:       "Enter to add  ·  Esc to cancel",
		Options:    opts,
		Searchable: true,
		OnSuccessTyped: func(val any) tea.Cmd {
			devID, _ := val.(string)
			return func() tea.Msg {
				return deepCollectPickedMsg{Items: items, DevID: devID, OrgUser: user}
			}
		},
	}
	return m.openChoiceModal(state)
}

func (m *Model) applyDeepCollectPicked(msg deepCollectPickedMsg) tea.Cmd {
	if m.devProjects == nil {
		return nil
	}
	var added, dup int
	var firstErr error
	for _, it := range msg.Items {
		it.DevProjectID = msg.DevID
		it.OrgUser = msg.OrgUser
		ok, err := m.devProjects.AddItem(it)
		if err != nil && firstErr == nil {
			firstErr = err
			continue
		}
		if ok {
			added++
		} else {
			dup++
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
	switch {
	case firstErr != nil:
		m.flash(fmt.Sprintf("added %d (then errored: %s)", added, firstErr.Error()))
	case dup > 0:
		m.flash(fmt.Sprintf("added %d · %d already in project", added, dup))
	default:
		m.flash(fmt.Sprintf("added %d items to project", added))
	}
	return nil
}

// resolveDeepCollect materializes the wizard's picks into Items.
// Synchronously fetches the required Tooling/REST lists. Caller
// (applyDeepCollectConfirmed) runs this from a Cmd or directly from
// Update, never from a render path.
func (m *Model) resolveDeepCollect(target string, op sf.Openable, picks map[deepCollectKind]bool) ([]devproject.Item, error) {
	switch t := op.(type) {
	case sf.SObject:
		return m.resolveDeepCollectSObject(target, t, picks)
	case sf.PermissionSetGroup:
		return m.resolveDeepCollectPSG(target, t, picks)
	}
	return nil, nil
}

func (m *Model) resolveDeepCollectPSG(target string, g sf.PermissionSetGroup, picks map[deepCollectKind]bool) ([]devproject.Item, error) {
	var items []devproject.Item
	label := g.MasterLabel
	if label == "" {
		label = g.DeveloperName
	}
	if picks[deepKindParent] {
		items = append(items, devproject.Item{
			Kind: devproject.KindPermissionSetGroup,
			Ref:  g.ID,
			Type: g.DeveloperName,
			Name: label,
		})
	}
	if picks[deepKindPSGComponents] {
		comps, err := sf.PermSetGroupComponents(target, g.ID)
		if err != nil {
			return nil, fmt.Errorf("psg components: %w", err)
		}
		for _, c := range comps {
			nm := c.PermissionSetLabel
			if nm == "" {
				nm = c.PermissionSetName
			}
			items = append(items, devproject.Item{
				Kind: devproject.KindPermissionSet,
				Ref:  c.PermissionSetID,
				Type: g.ID,
				Name: nm,
			})
		}
	}
	return items, nil
}

func (m *Model) resolveDeepCollectSObject(target string, s sf.SObject, picks map[deepCollectKind]bool) ([]devproject.Item, error) {
	var items []devproject.Item
	if picks[deepKindParent] {
		items = append(items, devproject.Item{
			Kind: devproject.KindSObject,
			Ref:  s.Name,
			Name: s.Label,
		})
	}
	if picks[deepKindAllFields] || picks[deepKindCustomFields] {
		fields, err := m.deepCollectFields(target, s.Name, picks[deepKindAllFields])
		if err != nil {
			return nil, err
		}
		items = append(items, fields...)
	}
	if picks[deepKindValidationRules] {
		vrs, err := sf.ListValidationRules(target, s.Name)
		if err != nil {
			return nil, fmt.Errorf("validation rules: %w", err)
		}
		for _, vr := range vrs {
			label := vr.ValidationName
			items = append(items, devproject.Item{
				Kind: devproject.KindValidationRule,
				Ref:  vr.ID,
				Type: s.Name,
				Name: label,
			})
		}
	}
	if picks[deepKindRecordTypes] {
		rts, err := sf.ListRecordTypes(target, s.Name)
		if err != nil {
			return nil, fmt.Errorf("record types: %w", err)
		}
		for _, rt := range rts {
			label := rt.DeveloperName
			if rt.Name != "" {
				label = rt.Name
			}
			items = append(items, devproject.Item{
				Kind: devproject.KindRecordType,
				Ref:  rt.ID,
				Type: s.Name,
				Name: label,
			})
		}
	}
	if picks[deepKindTriggers] {
		trs, err := sf.ListTriggers(target, s.Name)
		if err != nil {
			return nil, fmt.Errorf("triggers: %w", err)
		}
		for _, tr := range trs {
			items = append(items, devproject.Item{
				Kind: devproject.KindApexTrigger,
				Ref:  tr.ID,
				Type: s.Name,
				Name: tr.Name,
			})
		}
	}
	return items, nil
}

func (m *Model) deepCollectFields(target, sobject string, wantAll bool) ([]devproject.Item, error) {
	d := m.activeOrgData()
	var fields []sf.Field
	if d != nil {
		if r, ok := d.Describes[sobject]; ok && !r.FetchedAt().IsZero() {
			fields = r.Value().Fields
		}
	}
	if len(fields) == 0 {
		desc, err := sf.Describe(target, sobject)
		if err != nil {
			return nil, fmt.Errorf("describe %s: %w", sobject, err)
		}
		fields = desc.Fields
	}
	out := make([]devproject.Item, 0, len(fields))
	for _, f := range fields {
		if !wantAll && !f.Custom {
			continue
		}
		nm := f.Label
		if nm == "" {
			nm = f.Name
		}
		out = append(out, devproject.Item{
			Kind: devproject.KindField,
			Ref:  sobject + "." + f.Name,
			Type: sobject,
			Name: nm,
		})
	}
	return out, nil
}
