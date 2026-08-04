// Package highlight wraps chroma to give every code-detail surface
// (Apex class body, Apex trigger body, LWC / Aura resources, …) a
// consistent syntax-highlighted render.
//
// Two design points worth knowing about:
//
//  1. The output is a slice of one styled string per source line.
//     Callers wrap each line with a gutter, line number, truncation,
//     etc. without having to parse ANSI escapes back out. We DO NOT
//     return one big multi-line string because callers always need
//     per-line control for gutters + viewport clipping.
//
//  2. Highlighting is cached by (sha256(body), language, themeID).
//     Apex bodies are typically loaded once and rendered on every
//     paint; re-tokenising the same string each redraw would burn
//     CPU for no reason. The cache is a sync.Map (concurrent-safe;
//     the renderer is called from the Update goroutine but multiple
//     orgs / surfaces can interleave). Theme identity is part of the
//     key, so switching themes never reuses stale colours. The cache
//     is unbounded for the lifetime of the process.
//     A 50-class org with avg 200-line classes caches at ~10MB worst
//     case. Acceptable.
package highlight

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// Language is the chroma lexer name for a known surface kind. Use
// these constants when you have a kind in hand but no filename to
// dispatch on (e.g. an Apex body always parses as Java syntactically;
// chroma doesn't ship a dedicated Apex lexer, so we use Java which
// covers ~95% of Apex constructs).
const (
	LangApex       = "java"       // closest match — Apex IS Java syntactically
	LangJavaScript = "javascript" // LWC controllers, helpers
	LangTypeScript = "typescript" // newer LWC bundles
	LangHTML       = "html"       // LWC templates, Aura .cmp
	LangCSS        = "css"
	LangXML        = "xml" // .xml meta, Aura .design / .auradoc / .cmp
	LangJSON       = "json"
	LangSOQL       = "sql"   // close enough for a SOQL string body
	LangPlain      = "plain" // sentinel — return raw lines unstyled
)

// LanguageForFilename maps a resource path or filename suffix to a
// chroma lexer name. Returns LangPlain when the extension is unknown.
//
// Used by the LWC + Aura render paths where each bundle resource has
// a FilePath and we dispatch by extension. Apex / trigger detail
// callers don't need this — they pass LangApex directly.
func LanguageForFilename(name string) string {
	name = strings.ToLower(name)
	// Strip any trailing component after the last dot.
	dot := strings.LastIndexByte(name, '.')
	if dot < 0 {
		return LangPlain
	}
	switch name[dot+1:] {
	case "html", "htm":
		return LangHTML
	case "js", "mjs", "cjs":
		return LangJavaScript
	case "ts":
		return LangTypeScript
	case "css":
		return LangCSS
	case "xml", "cmp", "evt", "design", "auradoc", "tokens", "app":
		// Aura uses several markup formats that all parse as XML; the
		// .cmp / .evt etc. suffixes are Aura-specific but their syntax
		// is XML-shaped, so the XML lexer renders them correctly.
		return LangXML
	case "json":
		return LangJSON
	case "svg":
		return LangXML
	case "soql", "sql":
		return LangSOQL
	}
	return LangPlain
}

// LanguageForAuraDefType maps Aura's DefType + Format pair to a
// chroma lexer name. Aura resources don't carry a FilePath; the
// metadata is split across two columns. Mirrors LanguageForFilename
// but keyed differently. DefType is uppercase ("CONTROLLER",
// "HELPER", "STYLE", "COMPONENT" …); Format is uppercase
// ("JS", "CSS", "XML" …).
func LanguageForAuraDefType(defType, format string) string {
	// Format is the more reliable signal — it's the file format
	// rather than the role within the bundle.
	switch strings.ToLower(format) {
	case "js":
		return LangJavaScript
	case "ts":
		return LangTypeScript
	case "css":
		return LangCSS
	case "xml":
		return LangXML
	case "html":
		return LangHTML
	case "json":
		return LangJSON
	}
	// Some Aura DefTypes are markup with no Format set (e.g. legacy
	// component metadata). Default to XML — every Aura definition
	// that lacks a format is markup-shaped.
	return LangXML
}

// Highlight returns one styled line per line of body, ready to be
// rendered as-is. ANSI escape sequences are emitted directly so the
// returned strings can be concatenated with lipgloss-styled gutter
// content without conflict.
//
// language uses one of the Lang* constants or any chroma lexer name.
// LangPlain (or an unknown lexer) falls back to splitting body on
// newlines with no styling — callers don't need a separate code path
// for "I don't have highlighting for this kind."
func Highlight(body, language string) []string {
	if body == "" {
		return nil
	}
	if language == "" || language == LangPlain {
		return strings.Split(body, "\n")
	}
	// Cache hit: serve the same per-line slice. The slice is read-only
	// for callers (they compose, don't mutate), so sharing is fine.
	themeID := theme.Current.ID
	key := cacheKey(body, language, themeID)
	if cached, ok := cache.Load(key); ok {
		return cached.([]string)
	}
	lines := highlightUncached(body, language)
	cache.Store(key, lines)
	return lines
}

// highlightUncached runs chroma without consulting the cache. Pulled
// out so the caching path is obvious.
func highlightUncached(body, language string) []string {
	lexer := lexers.Get(language)
	if lexer == nil {
		// Unknown lexer — render plain. Don't error; the call site
		// shouldn't have to think about lexer availability.
		return strings.Split(body, "\n")
	}
	lexer = chroma.Coalesce(lexer)
	style := chromaStyleForCurrentTheme()
	formatter := formatters.Get("terminal16m")
	if formatter == nil {
		// Should never happen — terminal16m is built in. Fallback
		// to plain just in case the chroma API ever changes.
		return strings.Split(body, "\n")
	}
	iterator, err := lexer.Tokenise(nil, body)
	if err != nil {
		return strings.Split(body, "\n")
	}
	var buf strings.Builder
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return strings.Split(body, "\n")
	}
	// chroma emits a single string with embedded newlines + ANSI
	// resets. Split into per-line slices for caller composition.
	out := strings.Split(buf.String(), "\n")
	// chroma sometimes adds a trailing empty line for files that
	// already end with \n — strip it so the gutter line count
	// matches the source line count exactly.
	if n := len(out); n > 0 && out[n-1] == "" && !strings.HasSuffix(body, "\n\n") {
		out = out[:n-1]
	}
	return out
}

// cache is keyed by (sha256(body), language, themeID). The value is
// []string (one styled line per source line).
var cache sync.Map

func cacheKey(body, language, themeID string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:8]) + "|" + language + "|" + themeID
}

// cachedStyle holds the chroma Style built from the current theme
// palette. Rebuilt lazily on first highlight after a theme change
// (caller signals via ThemeChanged); within a session it's stable.
var (
	cachedStyle        *chroma.Style
	cachedStyleThemeID string
	styleMu            sync.Mutex
)

// chromaStyleForCurrentTheme returns the chroma style mapped from
// the active sf-deck theme palette. We map chroma's token classes
// to existing semantic colours (Magenta for keywords, Cyan for
// types, etc.) so the highlighting follows whatever theme the user
// has active.
func chromaStyleForCurrentTheme() *chroma.Style {
	styleMu.Lock()
	defer styleMu.Unlock()
	currentID := theme.Current.ID
	if cachedStyle != nil && cachedStyleThemeID == currentID {
		return cachedStyle
	}
	entries := chroma.StyleEntries{
		// Generic categories first; specific token kinds shadow them.
		chroma.Background:            "bg:" + hexFromColor(theme.Bg),
		chroma.Text:                  hexFromColor(theme.Fg),
		chroma.Error:                 hexFromColor(theme.Red),
		chroma.Comment:               "italic " + hexFromColor(theme.FgDim),
		chroma.CommentMultiline:      "italic " + hexFromColor(theme.FgDim),
		chroma.CommentSingle:         "italic " + hexFromColor(theme.FgDim),
		chroma.CommentSpecial:        "italic " + hexFromColor(theme.Muted),
		chroma.CommentPreproc:        hexFromColor(theme.Muted),
		chroma.Keyword:               "bold " + hexFromColor(theme.Magenta),
		chroma.KeywordConstant:       "bold " + hexFromColor(theme.Orange),
		chroma.KeywordDeclaration:    "bold " + hexFromColor(theme.Magenta),
		chroma.KeywordNamespace:      hexFromColor(theme.Magenta),
		chroma.KeywordPseudo:         hexFromColor(theme.Magenta),
		chroma.KeywordReserved:       "bold " + hexFromColor(theme.Magenta),
		chroma.KeywordType:           hexFromColor(theme.Cyan),
		chroma.Operator:              hexFromColor(theme.Magenta),
		chroma.OperatorWord:          "bold " + hexFromColor(theme.Magenta),
		chroma.Punctuation:           hexFromColor(theme.Fg),
		chroma.Name:                  hexFromColor(theme.Fg),
		chroma.NameAttribute:         hexFromColor(theme.Cyan),
		chroma.NameBuiltin:           hexFromColor(theme.Yellow),
		chroma.NameBuiltinPseudo:     hexFromColor(theme.Yellow),
		chroma.NameClass:             hexFromColor(theme.Cyan),
		chroma.NameConstant:          hexFromColor(theme.Orange),
		chroma.NameDecorator:         hexFromColor(theme.Yellow),
		chroma.NameEntity:            hexFromColor(theme.Cyan),
		chroma.NameException:         hexFromColor(theme.Red),
		chroma.NameFunction:          hexFromColor(theme.Yellow),
		chroma.NameProperty:          hexFromColor(theme.Cyan),
		chroma.NameLabel:             hexFromColor(theme.Yellow),
		chroma.NameNamespace:         hexFromColor(theme.Cyan),
		chroma.NameOther:             hexFromColor(theme.Fg),
		chroma.NameTag:               hexFromColor(theme.Magenta),
		chroma.NameVariable:          hexFromColor(theme.Fg),
		chroma.NameVariableClass:     hexFromColor(theme.Cyan),
		chroma.NameVariableGlobal:    hexFromColor(theme.Orange),
		chroma.NameVariableInstance:  hexFromColor(theme.Fg),
		chroma.LiteralString:         hexFromColor(theme.Green),
		chroma.LiteralStringDoc:      "italic " + hexFromColor(theme.FgDim),
		chroma.LiteralNumber:         hexFromColor(theme.Orange),
		chroma.LiteralNumberFloat:    hexFromColor(theme.Orange),
		chroma.LiteralNumberHex:      hexFromColor(theme.Orange),
		chroma.LiteralNumberInteger:  hexFromColor(theme.Orange),
		chroma.LiteralStringSymbol:   hexFromColor(theme.Green),
		chroma.LiteralStringRegex:    hexFromColor(theme.Yellow),
		chroma.LiteralStringEscape:   hexFromColor(theme.Yellow),
		chroma.LiteralStringInterpol: hexFromColor(theme.Yellow),
		chroma.GenericDeleted:        hexFromColor(theme.Red),
		chroma.GenericInserted:       hexFromColor(theme.Green),
		chroma.GenericHeading:        "bold " + hexFromColor(theme.Fg),
		chroma.GenericSubheading:     "bold " + hexFromColor(theme.FgDim),
		chroma.GenericEmph:           "italic " + hexFromColor(theme.Fg),
		chroma.GenericStrong:         "bold " + hexFromColor(theme.Fg),
	}
	style, err := chroma.NewStyle("sfdeck", entries)
	if err != nil {
		// Fallback to a built-in if our entries somehow malform —
		// guarantees Highlight() never panics.
		style = styles.Get("nord")
	}
	cachedStyle = style
	cachedStyleThemeID = currentID
	return style
}

// hexFromColor flattens an image/color.Color to a chroma-style hex
// string ("#rrggbb"). chroma accepts named colours too but hex is
// the most direct representation when we already have RGB values.
func hexFromColor(c interface {
	RGBA() (uint32, uint32, uint32, uint32)
}) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}
