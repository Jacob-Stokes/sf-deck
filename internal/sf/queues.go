package sf

// Queue + QueueSObject helpers — Salesforce stores queues as
// `Group` records with Type='Queue', plus `QueueSObject` rows
// associating each queue with the sObject types it can hold (Cases,
// Leads, custom routables).

import (
	"fmt"
	"strings"
)

// QueueRow is one queue surfaced on /perms Queues. Members count is
// resolved separately to avoid a per-row sub-query in the list path.
type QueueRow struct {
	ID                 string
	Name               string
	DeveloperName      string
	Email              string
	SObjects           []string // QueueSObject.SobjectType — sObjects this queue holds
	Members            int      // GroupMember count when populated
	LastModifiedDate   string
	LastModifiedByName string
}

// Field implements query.Row for chip predicates.
func (q QueueRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return q.ID, true
	case "Name":
		return q.Name, true
	case "DeveloperName":
		return q.DeveloperName, true
	case "Email":
		return q.Email, true
	case "SObjects":
		return strings.Join(q.SObjects, ","), true
	case "Members":
		return q.Members, true
	case "LastModifiedDate":
		return q.LastModifiedDate, true
	case "LastModifiedBy", "LastModifiedBy.Name", "LastModifiedByName":
		return q.LastModifiedByName, true
	}
	return nil, false
}

// Targets routes o on a queue row to the Setup queue page; classic
// Group record redirect resolves to the queue editor.
func (q QueueRow) Targets() []OpenTarget {
	t := []OpenTarget{
		{ID: "list", Label: "Queues (Setup)",
			Path: "/lightning/setup/Queues/home"},
	}
	if q.ID != "" {
		t = append([]OpenTarget{{
			ID:    "view",
			Label: "Queue (classic redirect)",
			Path:  "/" + q.ID,
		}}, t...)
	}
	return t
}

// YankTargets exposes the queue DeveloperName (API) / Name (label) / Id.
func (q QueueRow) YankTargets() []YankTarget {
	return nameLabelIDYankTargets(q.DeveloperName, q.Name, q.ID)
}

// ListQueues returns every Queue (Group where Type='Queue') in the
// org. Joins QueueSObject in a separate query so each row gets the
// sObjects-handled list. Member counts come from a third optional
// query (skipped here — the list view doesn't need them on first
// paint; lazy on drill).
func ListQueues(target string) ([]QueueRow, error) {
	c, err := RESTClient(target)
	if err != nil {
		return nil, err
	}
	q, err := c.QueryREST(
		"SELECT Id, Name, DeveloperName, Email, LastModifiedDate, LastModifiedBy.Name FROM Group "+
			"WHERE Type='Queue' ORDER BY Name", false)
	if err != nil {
		return nil, err
	}
	out := make([]QueueRow, 0, len(q.Records))
	idx := map[string]int{}
	for _, r := range q.Records {
		row := QueueRow{
			ID:                 asString(r["Id"]),
			Name:               asString(r["Name"]),
			DeveloperName:      asString(r["DeveloperName"]),
			Email:              asString(r["Email"]),
			LastModifiedDate:   asString(r["LastModifiedDate"]),
			LastModifiedByName: relationName(r, "LastModifiedBy"),
		}
		idx[row.ID] = len(out)
		out = append(out, row)
	}
	if len(out) == 0 {
		return out, nil
	}

	// QueueSObject for the sObject-handled list. One row per
	// (queue, sobject) pair — group up.
	qs, err := c.QueryREST(
		"SELECT QueueId, SobjectType FROM QueueSObject ORDER BY QueueId", false)
	if err != nil {
		// Soft-fail: the list is still useful without the sobject
		// breakdown. Caller doesn't see this error.
		return out, nil
	}
	for _, r := range qs.Records {
		qid := asString(r["QueueId"])
		if i, ok := idx[qid]; ok {
			out[i].SObjects = append(out[i].SObjects, asString(r["SobjectType"]))
		}
	}
	return out, nil
}

// GroupMemberRow is one resolved member of a Queue or Public Group.
// Salesforce's GroupMember.UserOrGroupId can point at either a User
// or another Group (nested groups), so the row carries a Kind to
// disambiguate. Targets() routes Lightning open URLs accordingly.
type GroupMemberRow struct {
	ID    string // member's User.Id or Group.Id
	Kind  string // "User" or "Group"
	Name  string // display name (User.Name or Group.Name)
	Email string // User only; "" for nested groups
}

// Field implements query.Row for chip predicates.
func (g GroupMemberRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return g.ID, true
	case "Name":
		return g.Name, true
	case "Kind":
		return g.Kind, true
	case "Email":
		return g.Email, true
	}
	return nil, false
}

// Targets routes Open on a member to the Lightning record-detail
// page for the underlying User or Group.
func (g GroupMemberRow) Targets() []OpenTarget {
	if g.ID == "" {
		return nil
	}
	switch g.Kind {
	case "User":
		return []OpenTarget{
			{ID: "view", Label: "User detail",
				Path: "/lightning/r/User/" + g.ID + "/view"},
		}
	case "Group":
		// Group records don't have a Lightning record-detail page
		// (they're a metadata sObject); fall back to the classic
		// redirect which Salesforce routes to the Setup queue /
		// public-group editor.
		return []OpenTarget{
			{ID: "view", Label: "Group (classic redirect)",
				Path: "/" + g.ID},
		}
	}
	return nil
}

// ListGroupMembersDetailed returns resolved members for a Queue or
// Public Group: User name + email when the member is a user, Group
// name when it's a nested group. One Composite request shapes the
// two parallel queries (User by id-set, Group by id-set) so we
// don't fan out per-row.
//
// Used by both queue-detail and public-group-detail surfaces — the
// underlying GroupMember table doesn't distinguish by parent kind
// (queues are just Groups with Type=Queue), so the lookup is
// uniform.
func ListGroupMembersDetailed(target, groupID string) ([]GroupMemberRow, error) {
	if groupID == "" {
		return nil, fmt.Errorf("groupID required")
	}
	c, err := RESTClient(target)
	if err != nil {
		return nil, err
	}
	memberQ, err := c.QueryREST(fmt.Sprintf(
		"SELECT UserOrGroupId FROM GroupMember WHERE GroupId='%s'",
		sqlEscape(groupID)), false)
	if err != nil {
		return nil, err
	}
	if len(memberQ.Records) == 0 {
		return nil, nil
	}
	// Bucket member ids by sObject prefix: 005 = User, 00G = Group.
	var userIDs, groupIDs []string
	for _, r := range memberQ.Records {
		id := asString(r["UserOrGroupId"])
		if id == "" {
			continue
		}
		switch {
		case strings.HasPrefix(id, "005"):
			userIDs = append(userIDs, id)
		case strings.HasPrefix(id, "00G"):
			groupIDs = append(groupIDs, id)
		}
	}
	out := make([]GroupMemberRow, 0, len(memberQ.Records))
	if len(userIDs) > 0 {
		uq, err := c.QueryREST("SELECT Id, Name, Email FROM User WHERE Id IN ("+
			quoteIDs(userIDs)+")", false)
		if err == nil {
			for _, r := range uq.Records {
				out = append(out, GroupMemberRow{
					ID:    asString(r["Id"]),
					Kind:  "User",
					Name:  asString(r["Name"]),
					Email: asString(r["Email"]),
				})
			}
		}
	}
	if len(groupIDs) > 0 {
		gq, err := c.QueryREST("SELECT Id, Name FROM Group WHERE Id IN ("+
			quoteIDs(groupIDs)+")", false)
		if err == nil {
			for _, r := range gq.Records {
				out = append(out, GroupMemberRow{
					ID:   asString(r["Id"]),
					Kind: "Group",
					Name: asString(r["Name"]),
				})
			}
		}
	}
	return out, nil
}

// quoteIDs joins ids into a "'a','b','c'" SOQL IN-list. Caller
// must have validated ids look like SF ids; this only quotes them.
func quoteIDs(ids []string) string {
	if len(ids) == 0 {
		return "''"
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, "'"+sqlEscape(id)+"'")
	}
	return strings.Join(parts, ",")
}
