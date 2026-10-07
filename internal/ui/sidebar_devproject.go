package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

func (m Model) sidebarProject(inner int) string {
	p, ok := m.projectList.Selected()
	if !ok {
		return sideEmpty("—")
	}
	rows := []kv{{"path", p.Path}}
	if p.Namespace != "" {
		rows = append(rows, kv{"namespace", p.Namespace})
	}
	if p.SourceAPIVersion != "" {
		rows = append(rows, kv{"api version", p.SourceAPIVersion})
	}
	extra := []string{"", sideSection(fmt.Sprintf("package dirs · %d", len(p.PackageDirs)))}
	for _, pd := range p.PackageDirs {
		def := ""
		if pd.Default {
			def = "  (default)"
		}
		extra = append(extra, sideDim("  "+pd.Path+def, inner))
		if pd.Package != "" {
			extra = append(extra, sideDim("    pkg "+pd.Package, inner))
		}
	}
	return renderKVPanel(inner, p.Name, rows, extra...)
}

func (m Model) sidebarRecordDetail(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil || d.RecordDetailCur == "" {
		return sideEmpty("—")
	}
	sobject, id := splitRecordKey(d.RecordDetailCur)
	r, ok := d.RecordDetails[d.RecordDetailCur]
	if !ok || r.FetchedAt().IsZero() {
		return sideEmpty("loading…")
	}
	rec := r.Value()
	rows := []kv{
		{"id", id},
		{"sobject", sobject},
	}
	if v, ok := rec["CreatedDate"].(string); ok && v != "" {
		rows = append(rows, kv{"created", prettyDate(v)})
	}
	if v, ok := rec["LastModifiedDate"].(string); ok && v != "" {
		rows = append(rows, kv{"modified", prettyDate(v)})
	}
	if v, ok := rec["OwnerId"].(string); ok && v != "" {
		rows = append(rows, kv{"owner id", v})
	}
	var extra []string
	if fieldName := d.RecordFieldCursor[d.RecordDetailCur]; fieldName != "" {
		extra = append(extra, "", sideSection("field"))
		extra = append(extra, sideKV("name", fieldName, inner))
		val := recordFieldDisplayValue(rec[fieldName])
		if val == "" {
			extra = append(extra, sideDim("  (empty)", inner))
		} else {
			extra = append(extra, "", sideDim("  "+firstPretty(Keys.InspectPanel)+" for a bigger, scrollable view", inner), "")
			wrapped := val
			if strings.Contains(val, "\n") {
				wrapped = wrapPreserving(val, inner-2)
			} else {
				wrapped = wrap(val, inner-2)
			}
			for _, ln := range strings.Split(wrapped, "\n") {
				extra = append(extra, "  "+ln)
			}
		}
	}
	extra = append(extra, m.sidebarTagsProjectsExtra(devproject.KindRecord, sobject+":"+id, o.Username, inner)...)
	extra = append(extra, "",
		sideDim("  "+firstPretty(Keys.OpenDefault)+" Lightning  ·  "+firstPretty(Keys.Refresh)+" refresh  ·  esc back", inner))
	title := sobject
	if name, ok := rec["Name"].(string); ok && name != "" {
		title = name
	}
	return m.kvPanelTagged(inner, title, nil,
		devproject.KindRecord, sobject+":"+id, o.Username, rows, extra...)
}

func (m Model) cursoredRecordFieldValue() (value, name string, ok bool) {
	if m.tab() != TabRecordDetail {
		return "", "", false
	}
	o, orgOK := m.currentOrg()
	if !orgOK {
		return "", "", false
	}
	d := m.data[o.Username]
	if d == nil || d.RecordDetailCur == "" {
		return "", "", false
	}
	fieldName := d.RecordFieldCursor[d.RecordDetailCur]
	if fieldName == "" {
		return "", "", false
	}
	r, exists := d.RecordDetails[d.RecordDetailCur]
	if !exists || r.FetchedAt().IsZero() {
		return "", "", false
	}
	raw := r.Value()[fieldName]
	switch x := raw.(type) {
	case nil:
		return "", "", false
	case string:
		if x == "" {
			return "", "", false
		}
		return x, fieldName, true
	default:
		if b, err := json.Marshal(raw); err == nil {
			return string(b), fieldName, true
		}
		return fmt.Sprintf("%v", raw), fieldName, true
	}
}

func wrapPreserving(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if lipgloss.Width(line) <= width {
			out = append(out, line)
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		r := []rune(line)
		out = append(out, string(r[:width]))
		r = r[width:]
		contW := width - len([]rune(indent))
		if contW < 8 {
			contW = width // indent too deep to bother — full width
			indent = ""
		}
		for len(r) > contW {
			out = append(out, indent+string(r[:contW]))
			r = r[contW:]
		}
		if len(r) > 0 {
			out = append(out, indent+string(r))
		}
	}
	return strings.Join(out, "\n")
}

func recordFieldDisplayValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		if pretty, ok := prettyJSONString(x); ok {
			return pretty
		}
		return x
	default:
		if b, err := json.MarshalIndent(v, "", "  "); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", v)
	}
}

func prettyJSONString(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if len(t) < 2 || (t[0] != '{' && t[0] != '[') {
		return "", false
	}
	var parsed any
	if err := json.Unmarshal([]byte(t), &parsed); err != nil {
		return "", false
	}
	b, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return "", false
	}
	return string(b), true
}

func (m Model) sidebarDevProject(inner int) string {
	if m.devProjects == nil {
		return sideEmpty("unavailable")
	}
	p, ok := m.devProjectList.Selected()
	if !ok {
		return sideEmpty("no matches")
	}
	rows := []kv{
		{"id", p.ID},
		{"created", humanTimeAgo(p.CreatedAt)},
		{"touched", humanTimeAgo(p.TouchedAt)},
	}
	counts, _ := m.devProjects.CountsForDev(p.ID)
	rows = append(rows,
		kv{"orgs", fmt.Sprintf("%d", counts.Orgs)},
		kv{"items", fmt.Sprintf("%d", counts.Items)},
	)
	extra := []string{}
	if p.Description != "" {
		extra = append(extra, "", sideSection("description"),
			sideDim("  "+wrap(p.Description, inner-2), inner))
	}
	extra = append(extra, "", sideDim("  ↵ open · "+firstPretty(Keys.NewProject)+" new · "+
		firstPretty(Keys.DeleteProject)+" delete · "+firstPretty(Keys.LoadOrgProject)+" load", inner))
	return renderKVPanel(inner, p.Name, rows, extra...)
}

func (m Model) sidebarDevProjectDetail(inner int) string {
	if m.devProjects == nil || m.devProjectCur == "" {
		return sideEmpty("—")
	}
	p, ok := m.devProjectByID(m.devProjectCur)
	if !ok {
		return sideEmpty("not found")
	}
	scope := "this org"
	if m.devProjectShowAllOrgs {
		scope = "all orgs"
	}
	rows := []kv{
		{"items", fmt.Sprintf("%d (%s)", len(m.devProjectItemsView()), scope)},
	}
	counts, _ := m.devProjects.CountsForDev(p.ID)
	rows = append(rows, kv{"reach", fmt.Sprintf("%d orgs", counts.Orgs)})
	if p.Description != "" {
		rows = append(rows, kv{"desc", p.Description})
	}

	extra := []string{}
	if row, _, ok := m.rowAtCursor(); ok {
		it := row.Item
		itemRows := []kv{
			{"kind", devProjectKindLabel(it.Kind)},
		}
		if it.Name != "" {
			itemRows = append(itemRows, kv{"name", it.Name})
		}
		if devProjectKindHasID(it.Kind) {
			itemRows = append(itemRows, kv{"id", it.Ref})
		} else {
			itemRows = append(itemRows, kv{"ref", it.Ref})
		}
		if it.Type != "" {
			itemRows = append(itemRows, kv{"type", it.Type})
		}
		if it.OrgUser != "" {
			itemRows = append(itemRows, kv{"org", it.OrgUser})
		}
		if it.Namespace != "" {
			itemRows = append(itemRows, kv{"namespace", it.Namespace})
		}
		if !it.AddedAt.IsZero() {
			itemRows = append(itemRows, kv{"added", humanTimeAgo(it.AddedAt)})
		}
		if it.Notes != "" {
			itemRows = append(itemRows, kv{"notes", it.Notes})
		}
		extra = append(extra, "", sideSection("focused"))
		for _, r := range itemRows {
			if r.V == "" {
				continue
			}
			extra = append(extra, sideKV(r.K, r.V, inner))
		}
	}

	extra = append(extra,
		"",
		sideDim("  ↵ open · "+firstPretty(Keys.DeleteProject)+" remove · Tab toggle scope · esc back", inner),
	)
	return renderKVPanel(inner, p.Name, rows, extra...)
}

func devProjectKindHasID(k devproject.ItemKind) bool {
	switch k {
	case devproject.KindSObject, devproject.KindField:
		return false
	}
	return true
}
