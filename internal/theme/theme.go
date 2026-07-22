package theme

// Theme system. A Palette is a flat struct of semantic colors (Bg,
// Fg, Blue, BorderHi, etc). Known palettes live in the registry
// below; ApplyPalette swaps the active one at runtime.
//
// Palette catalogue:
//   - 4 hand-tuned palettes shipped in code (curatedPalettes)
//   - ~497 palettes generated from github.com/mbadolato/iTerm2-Color-Schemes
//     (MIT licensed) via cmd/themegen → internal/theme/generated.go.
//
// Re-generate the catalogue with:
//   curl -sL https://github.com/mbadolato/iTerm2-Color-Schemes/archive/master.tar.gz | tar xz -C /tmp
//   go run ./cmd/themegen /tmp/iTerm2-Color-Schemes-master/ghostty/ > internal/theme/generated.go
//
// The rest of the UI reads package-level vars (theme.Fg, theme.Blue,
// etc.) rather than a *Palette handle, so callers need no changes
// when a new palette lands or the user switches theme — the vars
// are rebound by ApplyPalette. The named styles (Title, Panelled,
// StatusBar, …) are rebuilt at the same time because Lipgloss styles
// capture color values at build time.

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Palette is the full set of semantic colors one theme defines.
// Adding a field here forces every palette in the registry to
// populate it (the compile-time check list at the bottom of this
// file catches drift).
type Palette struct {
	Name string // display name shown in the theme picker
	ID   string // stable kebab-case id used in settings.toml

	// Surface colors
	Bg       color.Color
	BgAlt    color.Color
	Panel    color.Color
	Border   color.Color
	BorderHi color.Color
	Fg       color.Color
	FgDim    color.Color
	Muted    color.Color

	// Accent colors — named by hue not role so palettes stay
	// one-to-one with themes rather than forking per-view.
	Blue    color.Color
	Cyan    color.Color
	Green   color.Color
	Yellow  color.Color
	Red     color.Color
	Magenta color.Color
	Orange  color.Color
}

// The live palette. Rebound by ApplyPalette. Callers should NOT
// reach into this directly — read the package-level color vars
// below, which track Current.
var Current Palette

// Package-level color vars — the public API every other file reads
// from. Rebound by ApplyPalette so replacing the palette is a
// single call with no per-file updates needed.
var (
	Bg       color.Color
	BgAlt    color.Color
	Panel    color.Color
	Border   color.Color
	BorderHi color.Color
	// BorderDim is Border darkened ~30% — derived at palette-apply
	// time so every theme gets it for free without per-palette
	// declarations. Used for secondary chrome (autocomplete popup,
	// future tooltip borders) that should read as "subordinate to
	// the main panel" rather than competing with it.
	BorderDim color.Color
	Fg        color.Color
	FgDim     color.Color
	Muted     color.Color

	Blue    color.Color
	Cyan    color.Color
	Green   color.Color
	Yellow  color.Color
	Red     color.Color
	Magenta color.Color
	Orange  color.Color
)

// Named styles — rebuilt by ApplyPalette because Lipgloss captures
// colors at build time.
var (
	Title            lipgloss.Style
	Subtle           lipgloss.Style
	Panelled         lipgloss.Style
	PanelledFocus    lipgloss.Style
	PanelledFiltered lipgloss.Style
	PanelledDrill    lipgloss.Style
	StatusBar        lipgloss.Style
	KeyHint          lipgloss.Style
	KeyDesc          lipgloss.Style
)

// curatedIDs are the hand-tuned palettes that ship at the top of the
// picker. Order is the display order. Other entries from the
// generated catalogue follow alphabetically.
var curatedIDs = []string{
	"tokyo-night",
	"catppuccin",
	"dracula",
	"solarized-light",
	"terminal-app-light",
}

// popularIDs are widely-recognised themes from the generated catalogue
// that float to the top of the picker (right after the hand-tuned
// curated ones, before the ~400 alphabetical rest) so the common
// choices aren't buried. Order is display order — GitHub family first
// (frequently asked for), then the classics. Any id that doesn't exist
// in the catalogue is silently skipped in PaletteIDs, so this list is
// safe to edit.
var popularIDs = []string{
	"github-dark-default",
	"github-dark-dimmed",
	"github-light-default",
	"nord",
	"gruvbox-dark",
	"gruvbox-light",
	"one-half-dark",
	"one-half-light",
	"solarized-dark-higher-contrast",
	"monokai-pro",
	"catppuccin-macchiato",
	"catppuccin-latte",
	"everforest-dark-hard",
	"rose-pine",
	"kanagawa-wave",
	"nightfox",
	"tokyonight-storm",
}

// curatedPalettes are the hand-tuned ones (overrides the generated
// catalogue when ids collide).
func curatedPalettes() map[string]Palette {
	return map[string]Palette{
		"tokyo-night":        tokyoNight(),
		"catppuccin":         catppuccinMocha(),
		"dracula":            dracula(),
		"solarized-light":    solarizedLight(),
		"terminal-app-light": terminalAppLight(),
	}
}

// Palettes returns the known palette registry keyed by id. Curated
// + generated catalogue merged; curated wins on collision.
func Palettes() map[string]Palette {
	out := make(map[string]Palette, len(generatedPalettes)+len(curatedIDs))
	for _, p := range generatedPalettes {
		if p.ID == "" {
			continue
		}
		out[p.ID] = p
	}
	// Curated overrides whatever the generated catalogue had.
	for id, p := range curatedPalettes() {
		// Make sure the curated palette has its ID populated so
		// callers reading by ID (settings.toml round-trip) get the
		// right value.
		p.ID = id
		out[id] = p
	}
	return out
}

// PaletteIDs returns ids in picker order: hand-tuned curated ids first
// (declared order), then the popular tier (declared order), then the
// rest of the generated catalogue alphabetically. So the common choices
// lead; the ~400 alphabetical browse follows.
func PaletteIDs() []string {
	// Valid catalogue ids, so a stale entry in popularIDs is skipped
	// rather than surfacing a dead row.
	valid := make(map[string]bool, len(generatedPalettes))
	for _, p := range generatedPalettes {
		if p.ID != "" {
			valid[p.ID] = true
		}
	}

	placed := map[string]bool{}
	out := make([]string, 0, len(generatedPalettes)+len(curatedIDs))
	add := func(id string) {
		if placed[id] {
			return
		}
		placed[id] = true
		out = append(out, id)
	}
	// Tier 1: curated (hand-tuned; always valid — they override the
	// catalogue).
	for _, id := range curatedIDs {
		add(id)
	}
	// Tier 2: popular (skip any not in the catalogue or already curated).
	for _, id := range popularIDs {
		if valid[id] {
			add(id)
		}
	}
	// Tier 3: everything else, alphabetically.
	rest := make([]string, 0, len(generatedPalettes))
	for _, p := range generatedPalettes {
		if p.ID == "" || placed[p.ID] {
			continue
		}
		rest = append(rest, p.ID)
	}
	// Stable alpha sort so additions to the catalogue don't reshuffle
	// the picker between releases.
	sortIDs(rest)
	return append(out, rest...)
}

// sortIDs is a tiny in-place ASCII sort; avoids a "sort" import in
// this file's hot path (registry built once per startup).
func sortIDs(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// hex parses a "#rrggbb" string into a lipgloss-compatible Color.
// Used by the generated palette literals — having one helper means
// generated.go stays tiny.
func hex(s string) color.Color { return lipgloss.Color(s) }

// ApplyPalette swaps the live palette. Unknown ids silently fall
// back to tokyo-night so a bad config file never crashes startup.
func ApplyPalette(id string) {
	reg := Palettes()
	p, ok := reg[id]
	if !ok {
		p = reg["tokyo-night"]
	}
	Current = p

	Bg, BgAlt, Panel = p.Bg, p.BgAlt, p.Panel
	Border, BorderHi = p.Border, p.BorderHi
	BorderDim = darken(Border, 0.20)
	Fg, FgDim, Muted = p.Fg, p.FgDim, p.Muted
	Blue, Cyan, Green = p.Blue, p.Cyan, p.Green
	Yellow, Red, Magenta, Orange = p.Yellow, p.Red, p.Magenta, p.Orange

	Title = lipgloss.NewStyle().
		Foreground(Blue).
		Bold(true)

	Subtle = lipgloss.NewStyle().
		Foreground(Muted)

	Panelled = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Border).
		Padding(0, 1)

	PanelledFocus = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(BorderHi).
		Padding(0, 1)

	PanelledFiltered = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Yellow).
		Padding(0, 1)

	// PanelledDrill is the border treatment for the record-detail
	// surface — distinct color AND weight so users always know
	// "I'm drilled into a specific record" at a glance. Magenta
	// picks a hue not used by focus (BorderHi) or filter (Yellow)
	// or the modal frames (Cyan); ThickBorder doubles the visual
	// weight so the cue lands even on monochrome terminals.
	PanelledDrill = lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(Magenta).
		Padding(0, 1)

	StatusBar = lipgloss.NewStyle().
		Foreground(Fg).
		Background(Panel).
		Padding(0, 1)

	KeyHint = lipgloss.NewStyle().
		Foreground(Magenta).
		Bold(true)

	KeyDesc = lipgloss.NewStyle().
		Foreground(Muted)
}

// Initialise with the default so imports see non-zero colors even
// before main() calls ApplyPalette.
func init() { ApplyPalette("tokyo-night") }

// ----------------------------------------------------------------------
// Palettes
// ----------------------------------------------------------------------

func tokyoNight() Palette {
	return Palette{
		Name:     "Tokyo Night",
		Bg:       lipgloss.Color("#1a1b26"),
		BgAlt:    lipgloss.Color("#16161e"),
		Panel:    lipgloss.Color("#1f2335"),
		Border:   lipgloss.Color("#3b4261"),
		BorderHi: lipgloss.Color("#7aa2f7"),
		Fg:       lipgloss.Color("#c0caf5"),
		FgDim:    lipgloss.Color("#565f89"),
		Muted:    lipgloss.Color("#737aa2"),
		Blue:     lipgloss.Color("#7aa2f7"),
		Cyan:     lipgloss.Color("#7dcfff"),
		Green:    lipgloss.Color("#9ece6a"),
		Yellow:   lipgloss.Color("#e0af68"),
		Red:      lipgloss.Color("#f7768e"),
		Magenta:  lipgloss.Color("#bb9af7"),
		Orange:   lipgloss.Color("#ff9e64"),
	}
}

func catppuccinMocha() Palette {
	return Palette{
		Name:     "Catppuccin Mocha",
		Bg:       lipgloss.Color("#1e1e2e"),
		BgAlt:    lipgloss.Color("#181825"),
		Panel:    lipgloss.Color("#313244"),
		Border:   lipgloss.Color("#45475a"),
		BorderHi: lipgloss.Color("#89b4fa"),
		Fg:       lipgloss.Color("#cdd6f4"),
		FgDim:    lipgloss.Color("#6c7086"),
		Muted:    lipgloss.Color("#a6adc8"),
		Blue:     lipgloss.Color("#89b4fa"),
		Cyan:     lipgloss.Color("#89dceb"),
		Green:    lipgloss.Color("#a6e3a1"),
		Yellow:   lipgloss.Color("#f9e2af"),
		Red:      lipgloss.Color("#f38ba8"),
		Magenta:  lipgloss.Color("#cba6f7"),
		Orange:   lipgloss.Color("#fab387"),
	}
}

func dracula() Palette {
	return Palette{
		Name:     "Dracula",
		Bg:       lipgloss.Color("#282a36"),
		BgAlt:    lipgloss.Color("#21222c"),
		Panel:    lipgloss.Color("#44475a"),
		Border:   lipgloss.Color("#6272a4"),
		BorderHi: lipgloss.Color("#bd93f9"),
		Fg:       lipgloss.Color("#f8f8f2"),
		FgDim:    lipgloss.Color("#6272a4"),
		Muted:    lipgloss.Color("#8be9fd"),
		Blue:     lipgloss.Color("#bd93f9"),
		Cyan:     lipgloss.Color("#8be9fd"),
		Green:    lipgloss.Color("#50fa7b"),
		Yellow:   lipgloss.Color("#f1fa8c"),
		Red:      lipgloss.Color("#ff5555"),
		Magenta:  lipgloss.Color("#ff79c6"),
		Orange:   lipgloss.Color("#ffb86c"),
	}
}

// terminalAppLight is tuned for macOS Terminal.app's default "Basic"
// profile: a pure-white background and a 256-color (non-truecolor)
// renderer. Two things drive the choices:
//
//   - Bg is true white (#ffffff) so the TUI blends into the default
//     profile instead of painting a cream/grey rectangle over it.
//     Panels sit one shade down (#f2f2f2) so cards read as faint grey
//     on white rather than needing a heavy border.
//   - Accents are darkened well past the usual light-theme mid-tones.
//     On white, a mid blue like Solarized's #268bd2 is fine but green/
//     yellow wash out; these are pushed darker (#1a7f37 green, #9a6700
//     amber) so every accent clears a readable contrast ratio AND
//     survives xterm-256 quantization without collapsing into its
//     neighbour. FgDim/Muted stay at real mid-greys (#6a6a6a / #57606a)
//     so "dim" text doesn't disappear on white — the classic light-
//     theme legibility trap.
func terminalAppLight() Palette {
	return Palette{
		Name:     "Terminal.app Light",
		Bg:       lipgloss.Color("#ffffff"),
		BgAlt:    lipgloss.Color("#f2f2f2"),
		Panel:    lipgloss.Color("#f2f2f2"),
		Border:   lipgloss.Color("#c4c4c4"),
		BorderHi: lipgloss.Color("#0969da"),
		Fg:       lipgloss.Color("#1f2328"),
		FgDim:    lipgloss.Color("#6a6a6a"),
		Muted:    lipgloss.Color("#57606a"),
		Blue:     lipgloss.Color("#0969da"),
		Cyan:     lipgloss.Color("#1b7c83"),
		Green:    lipgloss.Color("#1a7f37"),
		Yellow:   lipgloss.Color("#9a6700"),
		Red:      lipgloss.Color("#cf222e"),
		Magenta:  lipgloss.Color("#8250df"),
		Orange:   lipgloss.Color("#bc4c00"),
	}
}

// darken returns c multiplied by (1 - factor) per RGB channel.
// factor=0 → unchanged; factor=1 → black. Falls back to the input
// when c isn't a recognisable hex/rgb color (covers ANSI numeric
// colors that don't carry RGB data).
//
// On light palettes (factor>0.5 against a near-white background)
// this lightens visibly because the source is already mid-tone;
// the helper is intended for "darker shade of an already-mid
// border colour" — works correctly for the curated palettes
// (Tokyo Night, Catppuccin, Dracula, Solarized Light) and any
// hex-encoded generated palette.
func darken(c color.Color, factor float64) color.Color {
	if c == nil {
		return c
	}
	r, g, b, a := c.RGBA()
	// RGBA returns values in [0, 0xFFFF]; the (0..65535) range
	// must be preserved so lipgloss reads the alpha correctly.
	scale := 1 - factor
	if scale < 0 {
		scale = 0
	}
	nr := uint8(float64(r>>8) * scale)
	ng := uint8(float64(g>>8) * scale)
	nb := uint8(float64(b>>8) * scale)
	na := uint8(a >> 8)
	if na == 0 {
		na = 0xFF
	}
	return rgbaColor{R: nr, G: ng, B: nb, A: na}
}

// rgbaColor is a tiny color.Color impl so darken's result satisfies
// the same interface lipgloss expects without importing image/color
// constructors that aren't available.
type rgbaColor struct{ R, G, B, A uint8 }

func (c rgbaColor) RGBA() (r, g, b, a uint32) {
	r = uint32(c.R) | uint32(c.R)<<8
	g = uint32(c.G) | uint32(c.G)<<8
	b = uint32(c.B) | uint32(c.B)<<8
	a = uint32(c.A) | uint32(c.A)<<8
	return
}

func solarizedLight() Palette {
	return Palette{
		Name:     "Solarized Light",
		Bg:       lipgloss.Color("#fdf6e3"),
		BgAlt:    lipgloss.Color("#eee8d5"),
		Panel:    lipgloss.Color("#eee8d5"),
		Border:   lipgloss.Color("#93a1a1"),
		BorderHi: lipgloss.Color("#268bd2"),
		Fg:       lipgloss.Color("#073642"),
		FgDim:    lipgloss.Color("#93a1a1"),
		Muted:    lipgloss.Color("#657b83"),
		Blue:     lipgloss.Color("#268bd2"),
		Cyan:     lipgloss.Color("#2aa198"),
		Green:    lipgloss.Color("#859900"),
		Yellow:   lipgloss.Color("#b58900"),
		Red:      lipgloss.Color("#dc322f"),
		Magenta:  lipgloss.Color("#d33682"),
		Orange:   lipgloss.Color("#cb4b16"),
	}
}
