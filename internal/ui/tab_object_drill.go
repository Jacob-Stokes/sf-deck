package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func (m Model) renderObjectDrill(w, innerH int) string {
	subs := objectDrillSubtabs()
	sel := m.objectSubtab()
	if sel < 0 || sel >= len(subs) {
		sel = 0
	}

	// Strip is the pinned subset + a synthetic More… slot when
	// overflow exists. The body dispatches off the FULL subs[]
	// using sel — so picking an overflow subtab via the modal
	// (which sets sel to its full-list index) renders correctly.
	stripSubs := m.tabSubtabsForStrip()
	stripSel := stripSelectedFor(sel, m)
	strip := renderSubtabStrip(stripSubs, stripSel, w-4)

	var content string
	switch subs[sel].ID {
	case SubtabDetails:
		content = m.renderObjectDetails(w, innerH-subtabReserve(strip))
	case SubtabSchema:
		content = m.renderObjectDetail(w, innerH-subtabReserve(strip))
	case SubtabValidation:
		content = m.renderObjectValidation(w, innerH-subtabReserve(strip))
	case SubtabRecordTypes:
		content = m.renderObjectRecordTypes(w, innerH-subtabReserve(strip))
	case SubtabTriggers:
		content = m.renderObjectTriggers(w, innerH-subtabReserve(strip))
	case SubtabFLS:
		content = m.renderObjectFLS(w, innerH-subtabReserve(strip))
	case SubtabRecords:
		// Records subtab = the old records view, but scoped to the
		// currently-drilled sObject. We map it onto the legacy
		// RecordsSObjectCur state so renderRecordsList works unchanged.
		content = m.renderObjectRecords(w, innerH-subtabReserve(strip))
	case SubtabObjectLayouts:
		content = m.renderObjectLayouts(w, innerH-subtabReserve(strip))
	case SubtabObjectFlows:
		content = m.renderObjectFlows(w, innerH-subtabReserve(strip))
	default:
		content = ""
	}

	if strip == "" {
		return content
	}
	return strings.Join([]string{strip, content}, "\n")
}

func subtabReserve(strip string) int {
	if strip == "" {
		return 0
	}
	return 1 + strings.Count(strip, "\n") // just the strip line(s), no extra padding for now
}

func (m Model) renderObjectRecords(w, innerH int) string {
	o, ok := m.currentOrg()
	if !ok {
		return lipgloss.NewStyle().Render("  no org selected")
	}
	d := m.ensureOrgDataRef(o.Username)
	sobj := d.DescribeCur
	if sobj == "" {
		return lipgloss.NewStyle().Render("  no sObject selected")
	}
	// Keep the legacy RecordsSObjectCur synced for the Local path.
	if d.RecordsSObjectCur != sobj {
		d.RecordsSObjectCur = sobj
	}
	if currentChipMode(d, sobj) == ChipModeSalesforce {
		selected := selectedRecordsChip(d, sobj)
		if selected == sfRecentlyViewedChipID {
			// Synthetic SF Recently Viewed chip — routes through the
			// records-list renderer because the data comes via
			// EnsureChipRecords (SOQL `Id IN (...)`), not the
			// /listviews/<id>/results endpoint.  renderRecordsList
			// reads d.ChipRecords + handles the empty-state hint
			// (including the not-recently-viewable / mruEnabled gate,
			// rendered under the chip strip).
			return m.renderRecordsList(d, w, innerH)
		}
		if selected == "" {
			r, hasRes := d.ListViewsPerSObject[sobj]
			if !hasRes || r.FetchedAt().IsZero() {
				return lipgloss.NewStyle().Render("  loading list views…")
			}
			return lipgloss.NewStyle().Render("  no Salesforce list views for " + sobj +
				" — press " + firstPretty(Keys.LensModeToggle) + " to switch back to sf-deck views")
		}
		return m.renderListViewResult(d, sobj, selected, w, innerH)
	}
	return m.renderRecordsList(d, w, innerH)
}
