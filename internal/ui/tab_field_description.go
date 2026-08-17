package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type fieldDescriptionLoadedMsg struct {
	orgUser string
	key     string // "<sobject>.<field>"
	desc    string
	err     error
}

// ensureFieldDescriptionCmd returns a command that fetches the cursored
// field's description via Tooling if it's a custom field and hasn't been
// fetched yet. Returns nil when there's nothing to do (standard field,
// already cached, no field open) so callers can batch it unconditionally.
//
// The command calls sf.CustomFieldID + sf.GetToolingMetadata directly
// (not the d-mutating cached helper) so it never writes orgData off the
// Update goroutine — the result is folded in by the msg handler instead.
func (m Model) ensureFieldDescriptionCmd() tea.Cmd {
	o, ok := m.currentOrg()
	if !ok {
		return nil
	}
	d := m.data[o.Username]
	if d == nil || d.DescribeCur == "" || d.FieldCur == "" {
		return nil
	}
	r, ok := d.Describes[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		return nil
	}
	f, ok := findFieldByName(r.Value().Fields, d.FieldCur)
	if !ok || !f.Custom {
		return nil
	}
	key := d.DescribeCur + "." + d.FieldCur
	if d.FieldDescriptions != nil {
		if _, done := d.FieldDescriptions[key]; done {
			return nil
		}
	}
	alias := targetArg(o)
	sobject := d.DescribeCur
	field := d.FieldCur
	orgUser := o.Username
	return func() tea.Msg {
		id, err := sf.CustomFieldID(alias, sobject, field)
		if err != nil {
			return fieldDescriptionLoadedMsg{orgUser: orgUser, key: key, err: err}
		}
		meta, err := sf.GetToolingMetadata(alias, "CustomField", id)
		if err != nil {
			return fieldDescriptionLoadedMsg{orgUser: orgUser, key: key, err: err}
		}
		desc, _ := meta["description"].(string)
		return fieldDescriptionLoadedMsg{orgUser: orgUser, key: key, desc: desc}
	}
}

func fieldDescriptionCache(d *orgData, sobject, field string) (desc string, loaded bool) {
	if d == nil || d.FieldDescriptions == nil {
		return "", false
	}
	v, ok := d.FieldDescriptions[sobject+"."+field]
	return v, ok
}

func (m *Model) applyFieldDescriptionLoaded(msg fieldDescriptionLoadedMsg) {
	d := m.data[msg.orgUser]
	if d == nil {
		return
	}
	if d.FieldDescriptions == nil {
		d.FieldDescriptions = map[string]string{}
	}
	d.FieldDescriptions[msg.key] = msg.desc
}
