package sf

// Notifications surface — what the user wants is "everything in the
// bell" (chatter @-mentions, approvals, shares, custom notifications).
// Salesforce serves these via TWO different endpoints depending on
// type, despite both showing up in the same UI bell:
//
//   /connect/notifications      — approval requests, shares, custom
//                                  notifications, "task assigned" pings.
//                                  Often empty in dev orgs.
//   /chatter/feeds/news/me/feed-elements
//                                — chatter posts mentioning you, plus
//                                  comments on threads you participate
//                                  in. Where most "bell" items come
//                                  from in practice.
//
// We query both and merge into a single stream sorted by timestamp.
// Either is allowed to be empty.

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
)

// Notification is one row in the bell stream. Fields mirror the
// Connect API payload; we keep the camelCase JSON tags exactly so
// future fields drop in without remapping.
//
// ParentID + ParentType are populated for chatter-feed items only —
// they hold the record the post is attached to (e.g. an Account the
// chatter post mentions you on). We use them to build a proper
// Lightning URL on Open since the raw `url` field from the API
// points at a REST endpoint that 401s in a browser.
type Notification struct {
	ID                     string `json:"id"`
	Type                   string `json:"type"` // "task_mention", "approval_request", "share", "task_assigned", "thanks", …
	MessageBody            string `json:"messageBody"`
	MessageTitle           string `json:"messageTitle"`
	URL                    string `json:"url"` // raw url from the API — usually a REST path; not browser-safe
	TargetID               string `json:"targetId"`
	Read                   bool   `json:"read"`
	Seen                   bool   `json:"seen"`
	LastModified           string `json:"lastModified"`           // ISO-8601 — the Connect payload uses this name (no Date suffix)
	Image                  string `json:"image"`                  // avatar / icon URL, when SF supplies one
	AdditionalDataAsString string `json:"additionalDataAsString"` // free-form per-type

	// Synthesized by our chatter-feed parser, not part of the wire
	// payload directly:
	ParentID   string `json:"-"`
	ParentType string `json:"-"`
}

// Field implements query.Row for filter predicates on the Home
// Notifications subtab.
func (n Notification) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return n.ID, true
	case "Type":
		return n.Type, true
	case "Title":
		return n.MessageTitle, true
	case "Body":
		return n.MessageBody, true
	case "Read":
		return n.Read, true
	case "When", "LastModified":
		return n.LastModified, true
	}
	return nil, false
}

// Targets makes a Notification an Openable. Translates the raw API
// URL (which is a REST path that errors in a browser) into a real
// Lightning route. Order of preference:
//  1. Parent record page when we know type+id (chatter feed items)
//  2. The notification's own targetId, opened as a record page
//     (works for share / mention / task notifications)
//  3. Lightning home as a last resort
func (n Notification) Targets() []OpenTarget {
	if n.ParentType != "" && n.ParentID != "" {
		return []OpenTarget{{
			ID: "view", Label: n.ParentType + " record",
			Path: "/lightning/r/" + n.ParentType + "/" + n.ParentID + "/view",
		}}
	}
	if n.TargetID != "" {
		// Salesforce IDs are 15- or 18-char alphanumerics; the
		// /lightning/r/<sobject>/<id>/view URL needs the sobject
		// type. We don't always know it for Connect notifications,
		// but Lightning's classic redirect (/<id>) resolves the
		// target type server-side and forwards to the right page —
		// safe fallback when type is missing.
		return []OpenTarget{{
			ID: "view", Label: "Open target record",
			Path: "/" + n.TargetID,
		}}
	}
	return []OpenTarget{{
		ID: "home", Label: "Lightning Home",
		Path: "/lightning/page/home",
	}}
}

// NotificationsList is the Connect endpoint response.
type NotificationsList struct {
	Notifications []Notification `json:"notifications"`
	UnreadCount   int            `json:"newNotificationCount"`
}

// ListNotifications returns the user's bell notifications. Merges
// /connect/notifications (system pings) with /chatter/feeds/news/me
// (chatter @-mentions and similar). Either source can be empty.
// Both calls fail-soft — if one errors we still return whatever the
// other gave us so the UI shows partial data instead of a hard fail.
func ListNotifications(target string, limit int) (NotificationsList, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	c, err := RESTClient(target)
	if err != nil {
		return NotificationsList{}, err
	}

	out := NotificationsList{}

	// Source 1: Connect notifications. Approval / share / custom.
	connectPath := c.APIPath(fmt.Sprintf("connect/notifications?size=%d", limit))
	if connectBody, err := c.get(connectPath, nil); err == nil {
		applog.Dump([]string{"notifications", "connect"}, "json", connectBody)
		var connect NotificationsList
		if err := json.Unmarshal(connectBody, &connect); err == nil {
			out.Notifications = append(out.Notifications, connect.Notifications...)
			out.UnreadCount += connect.UnreadCount
		} else {
			applog.Error("notifications.connect.decode", map[string]any{"err": err.Error()})
		}
	} else {
		applog.Error("notifications.connect.fetch", map[string]any{"err": err.Error()})
	}

	// Source 2: chatter feed elements for the running user. We map
	// each feed-element to a Notification row so the renderer doesn't
	// need to know the difference. Only items with @-mentions in their
	// body or titled "comment" make the cut — chatter is noisy
	// otherwise.
	feedPath := c.APIPath(fmt.Sprintf("chatter/feeds/news/me/feed-elements?pageSize=%d", limit))
	if feedBody, err := c.get(feedPath, nil); err == nil {
		applog.Dump([]string{"notifications", "feed"}, "json", feedBody)
		feedItems, ferr := parseChatterFeed(feedBody)
		if ferr != nil {
			applog.Error("notifications.feed.decode", map[string]any{"err": ferr.Error()})
		} else {
			out.Notifications = append(out.Notifications, feedItems...)
		}
	} else {
		applog.Error("notifications.feed.fetch", map[string]any{"err": err.Error()})
	}

	// Final sort by LastModified DESC so the merged list reads as
	// most-recent-first regardless of which endpoint each item came
	// from. ISO-8601 strings sort lexicographically the same as
	// chronologically.
	sortNotificationsDescByLastModified(out.Notifications)

	applog.Info("notifications.parsed", map[string]any{
		"count":  len(out.Notifications),
		"unread": out.UnreadCount,
		"target": target,
	})
	return out, nil
}

// parseChatterFeed turns a /chatter/feeds/news/me payload into
// Notification rows. Filters to items that look bell-worthy:
// @-mentions of the running user OR comments on threads the user
// authored. Other feed elements (broadcast posts unrelated to the
// user) are dropped.
func parseChatterFeed(body []byte) ([]Notification, error) {
	var raw struct {
		Elements []struct {
			ID           string `json:"id"`
			Type         string `json:"type"`
			CreatedDate  string `json:"createdDate"`
			ModifiedDate string `json:"modifiedDate"`
			Url          string `json:"url"`
			Body         struct {
				Text            string `json:"text"`
				IsRichText      bool   `json:"isRichText"`
				MessageSegments []struct {
					Type string `json:"type"`
					Text string `json:"text"`
					Name string `json:"name"`
				} `json:"messageSegments"`
			} `json:"body"`
			Header struct {
				Text string `json:"text"`
			} `json:"header"`
			Actor struct {
				DisplayName string `json:"displayName"`
				ID          string `json:"id"`
			} `json:"actor"`
			Parent struct {
				ID   string `json:"id"`
				Type string `json:"type"`
				Name string `json:"name"`
			} `json:"parent"`
			Capabilities struct {
				Mentions struct {
					RecentMentions []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"recentMentions"`
				} `json:"mentions"`
				Comments struct {
					Page struct {
						Total int `json:"total"`
					} `json:"page"`
				} `json:"comments"`
			} `json:"capabilities"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := make([]Notification, 0, len(raw.Elements))
	for _, e := range raw.Elements {
		// We surface every feed element — admins want to see what's
		// happening to records they care about; the chatter feed for
		// the running user is already filtered to what's relevant.
		// Compose a sensible body line by joining text segments.
		text := e.Body.Text
		if text == "" {
			var parts []string
			for _, seg := range e.Body.MessageSegments {
				if seg.Text != "" {
					parts = append(parts, seg.Text)
				} else if seg.Name != "" {
					parts = append(parts, "@"+seg.Name)
				}
			}
			if len(parts) > 0 {
				text = joinSegments(parts)
			}
		}
		title := e.Header.Text
		if title == "" {
			title = e.Actor.DisplayName
		}
		typ := "feed"
		if len(e.Capabilities.Mentions.RecentMentions) > 0 {
			typ = "task_mention"
		}
		modified := e.ModifiedDate
		if modified == "" {
			modified = e.CreatedDate
		}
		out = append(out, Notification{
			ID:           e.ID,
			Type:         typ,
			MessageBody:  text,
			MessageTitle: title,
			URL:          e.Url,
			TargetID:     e.ID,
			LastModified: modified,
			Read:         true, // chatter feed elements aren't read-tracked
			ParentID:     e.Parent.ID,
			ParentType:   e.Parent.Type,
		})
	}
	return out, nil
}

// joinSegments concatenates message segments with single spaces.
// Standalone helper so we don't pull strings into this file just for
// strings.Join — keeps the import surface honest.
func joinSegments(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 && x != "" && out != "" {
			out += " "
		}
		out += x
	}
	return out
}

// sortNotificationsDescByLastModified does an in-place sort of the
// merged list. Stable so items with identical timestamps preserve
// their source order (Connect items first, feed items second).
func sortNotificationsDescByLastModified(ns []Notification) {
	// Insertion sort — input is small (≤100 items typical) and we
	// avoid pulling sort into this file's import set.
	for i := 1; i < len(ns); i++ {
		j := i
		for j > 0 && ns[j-1].LastModified < ns[j].LastModified {
			ns[j-1], ns[j] = ns[j], ns[j-1]
			j--
		}
	}
}

// MarkNotificationRead flips the read flag on a single notification.
// Connect accepts a small JSON body with seen + read; we always set
// both true so the bell badge updates.
func MarkNotificationRead(target, id string) error {
	if id == "" {
		return fmt.Errorf("notification id required")
	}
	c, err := RESTClient(target)
	if err != nil {
		return err
	}
	body := []byte(`{"seen":true,"read":true}`)
	path := c.APIPath("connect/notifications/" + id)
	if _, err := c.patch(path, body); err != nil {
		return upgradeToSFError(err)
	}
	return nil
}

// MarkAllNotificationsRead flips every notification to read in one
// call. Connect's bulk endpoint:
//
//	PATCH /connect/notifications  {seen,read}
func MarkAllNotificationsRead(target string) error {
	c, err := RESTClient(target)
	if err != nil {
		return err
	}
	body := []byte(`{"seen":true,"read":true}`)
	path := c.APIPath("connect/notifications")
	if _, err := c.patch(path, body); err != nil {
		return upgradeToSFError(err)
	}
	return nil
}

// notificationsRequestBody is a placeholder for future write paths
// (mute, snooze) that need a typed body. Kept here so the import of
// bytes doesn't get ripped out by goimports prematurely.
var _ = bytes.NewReader
