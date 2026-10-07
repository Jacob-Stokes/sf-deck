package diff

import "strings"

// PrettyXML reflows a (possibly single-line) XML string into indented,
// one-element-per-line form so the line differ can align on individual
// elements and the side-by-side view doesn't run off the screen.
//
// Salesforce readMetadata returns XML with no whitespace between tags
// (<fullName>X</fullName><label>Y</label>…), making the whole component
// a single line — a useless diff. This is a lightweight reflow (not a
// full XML parser): it walks the string, emitting a newline before each
// '<' tag start, and indents by element depth. Leaf elements
// (<tag>text</tag>) stay on one line; container open/close tags get
// their own lines. Text content and attributes are preserved verbatim.
//
// It's deliberately tolerant: malformed fragments still produce
// reasonable output (we never error), because the goal is a readable,
// stably-formatted diff, not validation.
func PrettyXML(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.Count(s, "\n") > 2 {
		return s
	}

	var b strings.Builder
	depth := 0
	indent := func() {
		for i := 0; i < depth; i++ {
			b.WriteString("  ")
		}
	}
	i := 0
	n := len(s)
	firstLine := true
	for i < n {
		if s[i] != '<' {
			j := strings.IndexByte(s[i:], '<')
			if j < 0 {
				b.WriteString(s[i:])
				break
			}
			b.WriteString(s[i : i+j])
			i += j
			continue
		}
		gt := strings.IndexByte(s[i:], '>')
		if gt < 0 {
			b.WriteString(s[i:])
			break
		}
		tag := s[i : i+gt+1] // includes < and >
		i += gt + 1

		switch {
		case strings.HasPrefix(tag, "<?"), strings.HasPrefix(tag, "<!"):
			if !firstLine {
				b.WriteByte('\n')
			}
			indent()
			b.WriteString(tag)
		case strings.HasSuffix(tag, "/>"):
			if !firstLine {
				b.WriteByte('\n')
			}
			indent()
			b.WriteString(tag)
		case strings.HasPrefix(tag, "</"):
			depth--
			if depth < 0 {
				depth = 0
			}
			if !firstLine {
				b.WriteByte('\n')
			}
			indent()
			b.WriteString(tag)
		default:
			name := tagName(tag)
			closeTag := "</" + name + ">"
			rest := s[i:]
			ct := strings.Index(rest, "<")
			if ct >= 0 && strings.HasPrefix(rest[ct:], closeTag) {
				if !firstLine {
					b.WriteByte('\n')
				}
				indent()
				b.WriteString(tag)
				b.WriteString(rest[:ct]) // the text
				b.WriteString(closeTag)
				i += ct + len(closeTag)
			} else {
				if !firstLine {
					b.WriteByte('\n')
				}
				indent()
				b.WriteString(tag)
				depth++
			}
		}
		firstLine = false
	}
	return b.String()
}

func tagName(tag string) string {
	inner := strings.TrimPrefix(tag, "<")
	inner = strings.TrimSuffix(inner, ">")
	if sp := strings.IndexAny(inner, " \t\r\n"); sp >= 0 {
		inner = inner[:sp]
	}
	return inner
}
