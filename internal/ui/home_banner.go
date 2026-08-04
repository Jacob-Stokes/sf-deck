package ui

// Home banner — animated ASCII creature pinned at the top of the
// right sidebar on /home. Each connected org gets a stable creature
// assignment so toggling between orgs gives each one its own
// recognisable mascot at a glance.
//
// Creatures are generic species (bear, fox, dog, goat, elephant,
// zebra, mule, flying squirrel, bobcat) — chosen because they
// happen to map to the kinds of admin / dev personas the user
// switches between, but rendered as plain animal art with no
// branded names or styling. Future: per-org override in settings.
//
// Assignment: stable hash of username + org id modulo the creature
// list, so the same org always gets the same creature without
// persisting an explicit mapping. Production orgs always render in
// red regardless of creature so prod always pops as "be careful."

import (
	"crypto/sha1"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// creature is one mascot — a name (for debugging / future settings
// override) plus a list of animation frames. Frames cycle in order;
// caller picks the current frame index modulo len(Frames).
type creature struct {
	Name   string
	Frames []string
}

// creatures is the registry of available mascots. Adding a new one
// is a single entry here. Order is stable — assignment is by hash
// modulo length, so changing the order shuffles every org's
// mascot. Append-only is preferred to keep existing assignments.
var creatures = []creature{
	creatureBear,
	creatureFox,
	creatureDog,
	creatureGoat,
	creatureElephant,
	creatureZebra,
	creatureMule,
	creatureSquirrel,
	creatureBobcat,
}

// Banner frame interval is read from settings (HomeBannerIntervalMs) so
// users can slow it down / speed it up. Caller passes the resolved
// duration; nothing else here needs to know about settings.

type homeBannerTickMsg struct{}

// homeBannerTickCmd schedules a frame advance after `interval`.
// Returns nil when interval <= 0 — caller (Update) treats that as
// "stop the chain"; pair with the DisableHomeBanner check.
func homeBannerTickCmd(interval time.Duration) tea.Cmd {
	if interval <= 0 {
		return nil
	}
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return homeBannerTickMsg{}
	})
}

// orgKindColor — production red, sandbox yellow, scratch magenta,
// devhub cyan. Tied to org kind not creature so the user always
// sees prod in red.
func orgKindColor(o sf.Org) color.Color {
	switch {
	case o.IsScratch:
		return theme.Magenta
	case o.IsSandbox:
		return theme.Yellow
	case o.IsDevHub:
		return theme.Cyan
	}
	return theme.Red
}

// creatureForOrg returns the deterministic creature assignment for
// an org. Hash username+orgId so the same org always shows the
// same creature without persisting a per-org mapping.
func creatureForOrg(o sf.Org) creature {
	if len(creatures) == 0 {
		return creature{}
	}
	h := sha1.Sum([]byte(o.Username + ":" + o.OrgID))
	return creatures[int(h[0])%len(creatures)]
}

// renderHomeBanner produces the banner block: animated creature
// frame, org name, edition, kind pill. Width is the available
// horizontal space (typically the sidebar's inner).
func renderHomeBanner(o sf.Org, info sf.OrgInfo, frame, width int) string {
	c := creatureForOrg(o)
	if frame < 0 {
		frame = 0
	}
	if len(c.Frames) > 0 {
		frame = frame % len(c.Frames)
	}
	accent := orgKindColor(o)
	cs := lipgloss.NewStyle().Foreground(accent).Bold(true)

	var lines []string
	if len(c.Frames) > 0 {
		for _, row := range strings.Split(strings.TrimRight(c.Frames[frame], "\n"), "\n") {
			lines = append(lines, centerInWidth(cs.Render(row), width))
		}
	}

	name := info.Name
	if name == "" {
		name = o.Display()
	}
	nameStyle := lipgloss.NewStyle().Foreground(theme.Fg).Bold(true)
	lines = append(lines, "")
	lines = append(lines, centerInWidth(nameStyle.Render(name), width))
	if info.OrganizationType != "" {
		ed := lipgloss.NewStyle().Foreground(theme.Muted).Render(info.OrganizationType)
		lines = append(lines, centerInWidth(ed, width))
	}
	badge := lipgloss.NewStyle().
		Background(accent).
		Foreground(theme.Bg).
		Bold(true).
		Padding(0, 1).
		Render(o.Kind())
	lines = append(lines, centerInWidth(badge, width))

	return strings.Join(lines, "\n")
}

func centerInWidth(s string, total int) string {
	w := lipgloss.Width(s)
	if w >= total {
		return s
	}
	pad := (total - w) / 2
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + s
}

// --- creatures --------------------------------------------------------
//
// Each creature has 4 frames showing simple movement (head turn,
// blink, tail swish, ear flick). Frames are 7-9 rows tall, fit a
// 24-col sidebar width. Drawn as plain ASCII line-art.

var creatureBear = creature{
	Name: "bear",
	Frames: []string{
		`
   ___       ___
  (o o)     (- -)
  /   \     /   \
 ( ___ )   ( ___ )
  \___/     \___/`,
		`
   ___       ___
  (- o)     (o -)
  /   \     /   \
 ( --- )   ( --- )
  \___/     \___/`,
		`
   ___       ___
  (o o)     (o o)
  /  \\     //  \
 ( ___ )   ( ___ )
  \___/     \___/`,
		`
   ___       ___
  (^ ^)     (^ ^)
  /   \     /   \
 ( vvv )   ( vvv )
  \___/     \___/`,
	},
}

var creatureFox = creature{
	Name: "fox",
	Frames: []string{
		`
   /\   /\
  ( o.o )
   > ^ <
  /(   )\
   ~~~~~`,
		`
   /\   /\
  ( -.- )
   > ^ <
  /(   )\
   ~~~~~`,
		`
   /\   /\
  ( o.o )
   > v <
  /(   )\
   ~~~ ~`,
		`
   /\   /\
  ( o.O )
   > ^ <
   (   )/
   ~ ~~~`,
	},
}

var creatureDog = creature{
	Name: "dog",
	Frames: []string{
		`
    __
  o-''|\_____/)
   \_/|_)     )
      \  o o /
       \____/`,
		`
    __
  o-''|\_____/)
   \_/|_)     )
      \  - - /
       \____/`,
		`
    __
  o-''|\_____/)
   \_/|_)     )
      \  o o /
       \____/~`,
		`
    __
  o-''|\_____/)
   \_/|_)     )
      \  ^ ^ /
       \____/`,
	},
}

var creatureGoat = creature{
	Name: "goat",
	Frames: []string{
		`
   (\_/)
  /(o o)\
  ( ___ )
   |   |
   "   "`,
		`
   (\_/)
  /(- -)\
  ( ___ )
   |   |
   "   "`,
		`
   (\_/)
  /(o o)\
  ( vvv )
   |   |
   ' '  `,
		`
    \_/
   (o o)
  /( _ )\
   |   |
   "   "`,
	},
}

var creatureElephant = creature{
	Name: "elephant",
	Frames: []string{
		`
     ___
   _/   \_
  / o   o \
  \_   v   _/
    \_____/
     |   |
     "   "`,
		`
     ___
   _/   \_
  / -   - \
  \_   v   _/
    \_____/
     |   |
     "   "`,
		`
     ___
   _/   \_
  / o   o \
  \_   ~   _/
    \_____/
     |   |
     "   "`,
		`
     ___
   _/   \_
  / o   o \
  \_   ^   _/
    \_____/~
     |   |
     "   "`,
	},
}

var creatureZebra = creature{
	Name: "zebra",
	Frames: []string{
		`
    /||
   ( oo )
  /||""||
  |||~~|||
   ||  ||
   ""  ""`,
		`
    /||
   ( --)
  /||""||
  |||~~|||
   ||  ||
   ""  ""`,
		`
    \||
   ( oo )
  /||""||\
  |||~~|||
   ||  ||
   ''  ''`,
		`
    /||
   ( ^^ )
  /||""||
  |||~~|||
   ||  ||
   ""  ""`,
	},
}

var creatureMule = creature{
	Name: "mule",
	Frames: []string{
		`
    /\__/\
   /  o o \
  ( ===  = )
   \  ___ /
   //|   |\\
  // |   | \\
     |   |
     "   "`,
		`
    /\__/\
   /  - - \
  ( ===  = )
   \  ___ /
   //|   |\\
  // |   | \\
     |   |
     "   "`,
		`
    /\__/\
   /  o o \
  ( ===  = )
   \  vvv /
   //|   |\\
  // |   | \\
     |   |
     "   "`,
		`
    /\__/\
   /  ^ ^ \
  ( ===  = )
   \  ___ /
   //|   |\\
  // |   | \\~
     |   |
     "   "`,
	},
}

var creatureSquirrel = creature{
	Name: "squirrel",
	Frames: []string{
		`
        /\
   __   ||
  (o o) ||
  ( =^= )/
   \__|_/
    | |
    " "`,
		`
        /\
   __   ||
  (- -) ||
  ( =^= )/
   \__|_/
    | |
    " "`,
		`
        /
   __  /
  (o o)
  ( =v= )==
   \__|_/
    | |
    " "`,
		`
       /\
   __  ||
  (^ ^) ||
  ( =^= )\
   \__|_/
    | |
    " "`,
	},
}

var creatureBobcat = creature{
	Name: "bobcat",
	Frames: []string{
		`
    /\_/\
   ( o.o )
    > ^ <
   /(   )\
   ~~ ~ ~~`,
		`
    /\_/\
   ( -.- )
    > ^ <
   /(   )\
   ~~ ~ ~~`,
		`
    /\_/\
   ( o.O )
    > ~ <
   /(   )\
   ~~ ~ ~~`,
		`
    /\_/\
   ( ^.^ )
    > ^ <
   /(   )\
   ~~ ~ ~~`,
	},
}
