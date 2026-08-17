package ui

import (
	"strconv"
	"strings"
)

const (
	zoneNavOrgs          = "nav:orgs"
	zoneNavTags          = "nav:tags"
	zoneNavDevProjects   = "nav:dev-projects"
	zoneNavLoadedProject = "nav:loaded-project"
	zoneTabOverflow      = "tab:overflow"
	zoneSubtabOverflow   = "subtab:overflow"
	zoneSidebarHide      = "sidebar:hide"
	zoneSidebarStack     = "sidebar:stack"
)

func zoneTabID(t Tab) string {
	return "tab:" + strconv.Itoa(int(t))
}

func parseZoneTabID(id string) (Tab, bool) {
	n, ok := parseZoneInt(id, "tab:")
	return Tab(n), ok
}

func parseZoneChipID(id string) (int, bool) {
	return parseZoneInt(id, "chip:")
}

func zoneSubtabID(i int) string {
	return "subtab:" + strconv.Itoa(i)
}

func parseZoneSubtabID(id string) (int, bool) {
	return parseZoneInt(id, "subtab:")
}

func parseZoneInt(id, prefix string) (int, bool) {
	if !strings.HasPrefix(id, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, prefix))
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func zoneChipWizardRowID(cursor int) string {
	return "wizrow:" + strconv.Itoa(cursor+1)
}

func parseZoneChipWizardRowID(id string) (int, bool) {
	v, ok := parseZoneInt(id, "wizrow:")
	if !ok {
		return 0, false
	}
	return v - 1, true
}
