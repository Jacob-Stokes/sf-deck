package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// communityLoginPickerTargetID is the sentinel ID on the synthetic
// "Log in to community as user" OpenTarget that signals fireMenuTarget
// to open the network picker instead of opening a URL in the browser.
const communityLoginPickerTargetID = "community_login_picker"

// contactCommunityLoginTargets returns a single synthetic OpenTarget
// ("Log in to community as user", shortcut `c`) when the row is a
// Contact OR a Person Account whose active community User exists in an
// org with at least one Live Experience site. fireMenuTarget detects
// the sentinel ID and routes to the network sub-picker instead of
// opening a URL.
//
// Person Accounts are supported by resolving the account's implicit
// PersonContactId (the hidden Contact the community User links to) —
// so login-as works in person-account orgs, not just Contact orgs.
//
// Returns nil when:
//   - the row is not a Contact or Person Account
//   - the (person) contact has no active community User
//   - the org has no Live networks
//   - the org's OrgId isn't known (shouldn't happen — comes from sfdx
//     auth blob)
//
// Both backing queries run synchronously the first time they're
// needed, then memoise: the ContactId→UserId lookup on
// d.CommunityUserByContact (per contact, per session), and the
// network list on d.Networks (per org, per session).
func (m Model) contactCommunityLoginTargets(rec map[string]any, o sf.Org) []sf.OpenTarget {
	sobj, id := sf.SObjectAndIDFromRecord(rec)
	if id == "" {
		return nil
	}
	d := m.data[o.Username]
	if d == nil {
		return nil
	}
	contactID := ""
	switch sobj {
	case "Contact":
		contactID = id
	case "Account":
		if pc, ok := rec["PersonContactId"].(string); ok && pc != "" {
			contactID = pc
		} else if pc, known := d.CommunityUserByContact["acct:"+id]; known {
			contactID = pc
		} else {
			pc, err := sf.PersonContactID(targetArg(o), id)
			if err != nil {
				return nil
			}
			d.CommunityUserByContact["acct:"+id] = pc
			contactID = pc
		}
	default:
		return nil
	}
	if contactID == "" {
		return nil
	}
	userID, known := d.CommunityUserByContact[contactID]
	if !known {
		uid, err := sf.ContactCommunityUserID(targetArg(o), contactID)
		if err != nil {
			return nil
		}
		d.CommunityUserByContact[contactID] = uid
		userID = uid
	}
	if userID == "" {
		return nil
	}
	if d.Networks == nil {
		return nil
	}
	if d.Networks.FetchedAt().IsZero() {
		nets, err := sf.ListNetworks(targetArg(o))
		if err != nil {
			return nil
		}
		d.Networks.Set(nets)
	}
	if len(d.Networks.Value()) == 0 {
		return nil
	}
	if o.OrgID == "" {
		return nil
	}
	return []sf.OpenTarget{{
		ID:       communityLoginPickerTargetID,
		Label:    "Log in to community as user",
		Shortcut: "c",
	}}
}

// openCommunityLoginPicker shows the searchable Network picker for
// the cursor's Contact row, then on selection opens
// /servlet/servlet.su with the picked network's id. Called from
// fireMenuTarget when it sees communityLoginPickerTargetID.
//
// Re-resolves the contact id + user id + networks from orgData
// instead of stashing closure state on the synthetic OpenTarget —
// keeps OpenTarget plain-data and avoids a parallel "context per
// target" map. The lookups are O(1) memo hits at this point.
func (m *Model) openCommunityLoginPicker(o sf.Org, rec map[string]any) tea.Cmd {
	sobj, rowID := sf.SObjectAndIDFromRecord(rec)
	if rowID == "" {
		return nil
	}
	d := m.data[o.Username]
	if d == nil {
		return nil
	}
	contactID := ""
	switch sobj {
	case "Contact":
		contactID = rowID
	case "Account":
		if pc, ok := rec["PersonContactId"].(string); ok && pc != "" {
			contactID = pc
		} else if pc, known := d.CommunityUserByContact["acct:"+rowID]; known {
			contactID = pc
		} else if pc, err := sf.PersonContactID(targetArg(o), rowID); err == nil {
			d.CommunityUserByContact["acct:"+rowID] = pc
			contactID = pc
		}
	default:
		return nil
	}
	if contactID == "" {
		return nil
	}
	userID := d.CommunityUserByContact[contactID]
	if userID == "" {
		return nil
	}
	if d.Networks == nil {
		return nil
	}
	networks := d.Networks.Value()
	if len(networks) == 0 {
		return nil
	}
	orgID := o.OrgID
	if orgID == "" {
		return nil
	}

	// Anchor the picker near the top-centre of the screen, same as
	// the chip overflow picker.
	pickerW := modalWidth(m.width, 56, 90) * 2 / 3
	if pickerW < 48 {
		pickerW = 48
	}
	if pickerW > m.width-4 {
		pickerW = m.width - 4
	}
	anchorX := (m.width - pickerW) / 2
	anchorY := 4

	return openPicker(m, pickerSpec[sf.Network]{
		Title:       "Log in to community as " + contactDisplayName(rec),
		Items:       networks,
		Width:       pickerW,
		AnchorX:     anchorX,
		AnchorY:     anchorY,
		Placeholder: "type to filter sites…",
		Match: func(n sf.Network, q string) bool {
			lq := strings.ToLower(q)
			return strings.Contains(strings.ToLower(n.Name), lq) ||
				strings.Contains(strings.ToLower(n.UrlPathPrefix), lq)
		},
		RenderRow: func(n sf.Network, focused bool) string {
			label := n.Name
			if n.UrlPathPrefix != "" {
				label += "  " + lipgloss.NewStyle().Foreground(theme.FgDim).Render("/"+n.UrlPathPrefix)
			}
			line := "  " + label
			if focused {
				line = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " " +
					lipgloss.NewStyle().Foreground(theme.Fg).Bold(true).Render(label)
			}
			return line
		},
		OnPick: func(n sf.Network) tea.Cmd {
			t := sf.OpenTarget{
				ID:    "loginas_" + n.ID,
				Label: "Log in to " + n.Name + " as user",
				Path:  sf.CommunityLoginAsPath(orgID, n.ID, userID, contactID),
			}
			// Closure captures the Model snapshot at picker-open time
			// — fine because openInBrowserCmd only reads settings.
			snap := *m
			return snap.openInBrowserCmd(o, t)
		},
	})
}

func contactDisplayName(rec map[string]any) string {
	if s, ok := rec["Name"].(string); ok && s != "" {
		return s
	}
	if s, ok := rec["Id"].(string); ok {
		return s
	}
	return "this contact"
}
