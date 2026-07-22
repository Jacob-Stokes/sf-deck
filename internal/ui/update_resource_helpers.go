package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/resource"
)

type orgResourceRoute struct {
	Prefix string
	Handle func(rest string) (bool, tea.Cmd)
}

func applyAndMaybeRefreshResource[T any](m Model, r *Resource[T], msg resource.UpdatedMsg) (bool, tea.Cmd) {
	if r == nil || !r.Apply(msg) {
		return false, nil
	}
	if msg.FromCache {
		return true, r.MaybeRefreshAfterCacheLoad(m.cache)
	}
	return true, nil
}

func (m Model) applyOrgPrefixResourceMsg(d *orgData, msg resource.UpdatedMsg) (bool, tea.Cmd) {
	routes := []orgResourceRoute{
		{Prefix: "describe_v3:", Handle: func(name string) (bool, tea.Cmd) {
			handled, refresh := applyAndMaybeRefreshResource(m, d.Describes[name], msg)
			if handled {
				// SOQL autocomplete may be waiting on this describe
				// to resolve a relationship hop or surface picklist
				// values. Invalidate so the next render re-runs
				// Classify+Suggest against the freshly loaded data.
				(&m).autocompleteInvalidate()
				// When the describe for the currently-drilled
				// record's sObject lands, the per-record reference
				// names + child counts can finally be Ensured (both
				// require the describe to construct their SOQL).
				// Fire them now so the RELATIONSHIPS + RELATED
				// panels populate without a second user action.
				if m.tab() == TabRecordDetail && d.RecordDetailCur != "" {
					sobj, id := splitRecordKey(d.RecordDetailCur)
					if sobj == name && id != "" {
						o, ok := m.currentOrg()
						if ok {
							alias := targetArg(o)
							var follow tea.Cmd
							if r := d.EnsureRecordReferenceNames(alias, sobj, id); r != nil {
								follow = r.Ensure(m.cache)
							}
							if r := d.EnsureRecordChildCounts(alias, sobj, id); r != nil {
								c := r.Ensure(m.cache)
								if follow == nil {
									follow = c
								} else {
									follow = tea.Batch(follow, c)
								}
							}
							if follow != nil {
								if refresh == nil {
									refresh = follow
								} else {
									refresh = tea.Batch(refresh, follow)
								}
							}
						}
					}
				}
			}
			return handled, refresh
		}},
		{Prefix: "object_baseline:", Handle: func(name string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.CustomObjectBaselines[name], msg)
		}},
		{Prefix: "flowversions:", Handle: func(defID string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.FlowVersions[defID], msg)
		}},
		{Prefix: "flowversiondef:", Handle: func(id string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.FlowVersionDetail[id], msg)
		}},
		{Prefix: "apex_class:", Handle: func(id string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.ApexClassDetail[id], msg)
		}},
		{Prefix: "lwc_bundle:", Handle: func(id string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.LWCDetail[id], msg)
		}},
		{Prefix: "deploy_detail:", Handle: func(id string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.DeployDetailMap[id], msg)
		}},
		{Prefix: "metalist:", Handle: func(metaType string) (bool, tea.Cmd) {
			handled, follow := applyAndMaybeRefreshResource(m, d.MetaTypeItems[metaType], msg)
			if handled && metaType == d.MetaTypeCur {
				d.SyncMetaTypeItemList()
			}
			return handled, follow
		}},
		{Prefix: "aura_bundle:", Handle: func(id string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.AuraDetail[id], msg)
		}},
		{Prefix: "reportrun:", Handle: func(id string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.ReportRuns[id], msg)
		}},
		{Prefix: "recorddetail:", Handle: func(key string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.RecordDetails[key], msg)
		}},
		{Prefix: "recordrefs:", Handle: func(key string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.RecordReferenceNames[key], msg)
		}},
		{Prefix: "recordchildcounts:", Handle: func(key string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.RecordChildCounts[key], msg)
		}},
		{Prefix: "chiprecords:", Handle: func(composite string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.ChipRecords[composite], msg)
		}},
		{Prefix: "chipusers:", Handle: func(chipID string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.ChipUsers[chipID], msg)
		}},
		{Prefix: "records:", Handle: func(sobj string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.Records[sobj], msg)
		}},
		{Prefix: "listviews:", Handle: func(sobj string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.ListViewsPerSObject[sobj], msg)
		}},
		{Prefix: "recently_viewed_per_sobject:", Handle: func(sobj string) (bool, tea.Cmd) {
			handled, refresh := applyAndMaybeRefreshResource(m, d.RecentlyViewedPerSObject[sobj], msg)
			if handled {
				// When the active records surface is this sObject in SF
				// mode with the synthetic Recently Viewed chip selected,
				// rebuild the chip-records Resource now that visited IDs
				// are in hand. EnsureChipRecords' fetch closure captures
				// the chip predicate, and this synthetic chip keeps the
				// same ID while its Id IN (...) payload changes.
				if d2, sobj2 := m.activeRecordsSObject(); sobj2 == sobj &&
					currentChipMode(d2, sobj2) == ChipModeSalesforce &&
					selectedRecordsChip(d2, sobj2) == sfRecentlyViewedChipID {
					if rc := m.rebuildSalesforceRecentlyViewedChipRecords(d2, sobj2, msg.Scope); rc != nil {
						refresh = tea.Batch(refresh, rc)
					}
				}
			}
			return handled, refresh
		}},
		{Prefix: "listview:", Handle: func(key string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.ListViewResults[key], msg)
		}},
		{Prefix: "fls:", Handle: func(key string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.FLS[key], msg)
		}},
		{Prefix: "objectperms:", Handle: func(key string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.ObjectPerms[key], msg)
		}},
		{Prefix: "systemperms:", Handle: func(key string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.SystemPerms[key], msg)
		}},
		{Prefix: "assignedusers:", Handle: func(key string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.AssignedUsers[key], msg)
		}},
		{Prefix: "groupmembers:", Handle: func(key string) (bool, tea.Cmd) {
			return applyAndMaybeRefreshResource(m, d.GroupMembers[key], msg)
		}},
		{Prefix: "usersessions:", Handle: func(key string) (bool, tea.Cmd) {
			handled, refresh := applyAndMaybeRefreshResource(m, d.UserSessions[key], msg)
			// Mirror the just-applied session rows into the shared
			// UserSessionList when this is the drilled-in user, so the
			// list surface re-renders.
			if handled && key == d.SessionUserID {
				d.SyncUserSessionList()
			}
			return handled, refresh
		}},
		{Prefix: "communitypages:", Handle: func(key string) (bool, tea.Cmd) {
			handled, refresh := applyAndMaybeRefreshResource(m, d.CommunityPages[key], msg)
			if handled && key == communityPageKey(d.CommunityCur) {
				d.SyncCommunityPageList()
			}
			return handled, refresh
		}},
	}
	for _, route := range routes {
		if strings.HasPrefix(msg.Key, route.Prefix) {
			handled, refresh := route.Handle(strings.TrimPrefix(msg.Key, route.Prefix))
			if handled {
				return true, refresh
			}
			return true, nil
		}
	}
	return false, nil
}

func (m Model) rebuildSalesforceRecentlyViewedChipRecords(d *orgData, sobject, orgUser string) tea.Cmd {
	if d == nil || sobject == "" {
		return nil
	}
	c, ok := m.salesforceVisitedRecordsChip(d, sobject, orgUser)
	if !ok {
		return nil
	}
	o, ok := m.currentOrg()
	if !ok {
		return nil
	}
	key := sobject + ":" + sfRecentlyViewedChipID
	delete(d.ChipRecords, key)
	delete(d.visibleRecordsCache, key)
	rr := d.EnsureChipRecords(targetArg(o), sobject, c, qchip.Substitutions{})
	return rr.Ensure(m.cache)
}
