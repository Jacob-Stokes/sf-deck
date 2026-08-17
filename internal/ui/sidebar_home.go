package ui

import (
	"fmt"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func sidebarSystemAPI(_ Model, _ int) string {
	return sideEmpty("API usage · read-only")
}

func (m Model) sidebarSetup(inner int) string {
	l, ok := m.setupList.Selected()
	if !ok {
		return sideEmpty("no matches")
	}
	o, _ := m.currentOrg()
	rows := []kv{{"path", l.Path}}
	if o.InstanceURL != "" {
		rows = append(rows, kv{"url", strings.TrimRight(o.InstanceURL, "/") + l.Path})
	}
	extra := []string{"", sideDim(
		"  "+firstPretty(Keys.OpenDefault)+" → open · "+
			firstPretty(Keys.OpenMenu)+" → pick target", inner)}
	return renderKVPanel(inner, l.Name, rows, extra...)
}

func (m Model) sidebarHome(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]

	var info sf.OrgInfo
	if d != nil {
		info = d.OrgInfo.Value()
	}
	banner := ""
	if m.settings == nil || !m.settings.HideHomeBanner() {
		banner = renderHomeBanner(o, info, m.homeBadgeFrame, inner)
	}

	orgRows := []kv{
		{"alias", o.Display()},
		{"username", o.Username},
		{"instance", o.InstanceURL},
		{"org id", o.OrgID},
	}
	if info.InstanceName != "" {
		orgRows = append(orgRows, kv{"pod", info.InstanceName})
	}
	if info.NamespacePrefix != "" {
		orgRows = append(orgRows, kv{"namespace", info.NamespacePrefix})
	}
	if info.PrimaryContact != "" {
		orgRows = append(orgRows, kv{"primary contact", info.PrimaryContact})
	}
	if d != nil {
		h := d.Home.Value()
		if h.APIVersion != "" {
			orgRows = append(orgRows, kv{"api", "v" + h.APIVersion})
		}
		if h.Users.TotalActive > 0 || h.Users.TotalInactive > 0 {
			orgRows = append(orgRows, kv{"users",
				fmt.Sprintf("%d active · %d inactive",
					h.Users.TotalActive, h.Users.TotalInactive)})
		}
	}

	var b strings.Builder
	if banner != "" {
		b.WriteString(banner)
		b.WriteString("\n\n")
	}
	b.WriteString(sideTitle("ORG"))
	b.WriteString("\n")
	b.WriteString(sideSeparator(inner))
	for _, r := range orgRows {
		if r.V == "" {
			continue
		}
		b.WriteString("\n")
		b.WriteString(sideKV(r.K, r.V, inner))
	}
	if d != nil {
		b.WriteString("\n")
		b.WriteString(sideDim("  updated "+humanAge(d.Home.FetchedAt())+
			stateSuffix(d.Home.Busy(), d.Home.Err()), inner))
	}
	return b.String()
}
