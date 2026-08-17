package soqlauto

import (
	"regexp"
	"strings"
)

// Classify inspects snap.Query[:snap.CursorPos] to figure out where
// the caret sits and what kind of suggestions belong there. Mirrors
// Inspector Reloaded's queryAutocompleteHandler cascade — regex
// match against the text-before-caret, first hit wins.
//
// Returns a fully-populated Classification. Callers pass it to
// Suggest to generate the actual list.
func Classify(snap Snapshot) Classification {
	cursor := clampCursor(snap.Query, snap.CursorPos)
	before := snap.Query[:cursor]

	c := Classification{
		Context:     ContextTopLevel,
		SearchToken: trailingToken(before),
		ContextPath: trailingDottedPath(before),
		Sobject:     resolveSObject(snap.Query, cursor),
	}

	if isInSubquery(before) {
		c.InSubquery = true
		if child := innermostSubquerySObject(before); child != "" {
			c.Sobject = child
		}
	}

	// 1. Right after FROM keyword — suggest sObjects. Matches
	// both the empty slot (`FROM `) and partial-name typing
	// (`FROM Acc`). The trailing token (sObject name in progress)
	// is the SearchToken already populated above.
	if afterFromKeywordRe.MatchString(before) {
		c.Context = ContextAfterFromKeyword
		return c
	}

	// 2. RHS of IN (...) value list — must come before plain
	// operator-RHS because `Industry IN ('Tech', '` matches both.
	if m := inWithValuesRe.FindStringSubmatch(before); m != nil {
		c.Context = ContextInWithValues
		c.OperatorRHS = true
		c.OperatorOp = "IN"
		c.SearchToken = strings.TrimPrefix(m[1], "'")
		c.WhereField = priorOperatorLHS(before, "in")
		c.InSubquery = false
		return c
	}

	if m := operatorRHSRe.FindStringSubmatch(before); m != nil {
		c.Context = ContextWhereValue
		c.OperatorRHS = true
		c.OperatorOp = strings.ToUpper(m[1])
		c.SearchToken = strings.TrimPrefix(m[2], "'")
		c.WhereField = priorOperatorLHS(before, c.OperatorOp)
		return c
	}

	switch lastClauseKeyword(before) {
	case "order by":
		c.Context = ContextOrderByField
		return c
	case "group by":
		c.Context = ContextGroupByField
		return c
	case "where":
		c.Context = ContextWhereField
		return c
	case "select":
		c.Context = ContextAfterSelectKeyword
		return c
	case "limit", "offset":
		c.Context = ContextNumericLiteral
		return c
	}

	c.Context = ContextTopLevel
	return c
}

var (
	// Match the sObject slot after FROM: either empty (`FROM `) or
	// a partial name being typed (`FROM Acco`). Requires at least
	// one whitespace after FROM so we don't match `from` mid-word
	// (e.g. a custom field named `Promotional_From__c`).
	afterFromKeywordRe = regexp.MustCompile(`(?i)(?:^|\s)from\s+[a-z0-9_]*$`)

	operatorRHSRe = regexp.MustCompile(`(?i)\s*([<>=!]+|like)\s+('?[^'\s]*)$`)

	// IN (...) value list: at least one quoted-and-comma'd token OR
	// the opening of the first token. Matches Inspector's regex.
	inWithValuesRe = regexp.MustCompile(`(?i)\s*in\s*\(\s*(?:(?:'[^']*'\s*,\s*)+|)('?[^'\s]*)$`)

	fromObjectRe = regexp.MustCompile(`(?i)(?:^|\s)from\s+([a-z0-9_]+)`)

	subqueryFromRe = regexp.MustCompile(`(?i)\(\s*select[^()]*\sfrom\s+([a-z0-9_]+)`)

	// Trailing-token + trailing-dotted-path regexes. Token is
	// strictly word chars; the dotted path includes `.` so we can
	// peel hops off it.
	trailingTokenRe      = regexp.MustCompile(`[a-zA-Z0-9_]*$`)
	trailingDottedPathRe = regexp.MustCompile(`[a-zA-Z0-9_.]*$`)
)

// clampCursor ensures the cursor falls within [0, len(query)].
func clampCursor(query string, cursor int) int {
	if cursor < 0 {
		return 0
	}
	if cursor > len(query) {
		return len(query)
	}
	return cursor
}

func trailingToken(before string) string {
	m := trailingTokenRe.FindString(before)
	return m
}

// trailingDottedPath returns the trailing run of word-or-dot chars.
// Includes the trailing token; callers strip it to get the hop
// prefix (e.g. "Account.Owner.").
func trailingDottedPath(before string) string {
	return trailingDottedPathRe.FindString(before)
}

// HopsBeforeToken splits a dotted path like "Account.Owner.Manager"
// into ["Account", "Owner"] (every hop EXCEPT the trailing token).
// "Account." → ["Account"], "Name" → []. Used by Suggest to
// traverse describes.
func HopsBeforeToken(dottedPath string) []string {
	if dottedPath == "" {
		return nil
	}
	parts := strings.Split(dottedPath, ".")
	// The last element is the search token (may be empty when the
	// user just typed `.`). Drop it.
	return parts[:len(parts)-1]
}

// resolveSObject scans the query for the first `FROM <name>` outside
// parens. Returns "" when no FROM has been typed yet.
//
// We deliberately ignore subquery FROMs here — subqueries are
// detected separately by isInSubquery + innermostSubquerySObject.
func resolveSObject(query string, cursor int) string {
	depth := 0
	var b strings.Builder
	b.Grow(len(query))
	for i := 0; i < len(query); i++ {
		ch := query[i]
		switch ch {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteByte(ch)
			}
		}
	}
	if m := fromObjectRe.FindStringSubmatch(b.String()); m != nil {
		return m[1]
	}
	_ = cursor
	return ""
}

func isInSubquery(before string) bool {
	inStr := false
	var stack []bool
	for i := 0; i < len(before); i++ {
		ch := before[i]
		if ch == '\'' && (i == 0 || before[i-1] != '\\') {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		switch ch {
		case '(':
			j := i + 1
			for j < len(before) && (before[j] == ' ' || before[j] == '\t' || before[j] == '\n') {
				j++
			}
			isSelect := j+6 <= len(before) &&
				(before[j] == 's' || before[j] == 'S') &&
				(before[j+1] == 'e' || before[j+1] == 'E') &&
				(before[j+2] == 'l' || before[j+2] == 'L') &&
				(before[j+3] == 'e' || before[j+3] == 'E') &&
				(before[j+4] == 'c' || before[j+4] == 'C') &&
				(before[j+5] == 't' || before[j+5] == 'T')
			stack = append(stack, isSelect)
		case ')':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	for _, isSub := range stack {
		if isSub {
			return true
		}
	}
	return false
}

func innermostSubquerySObject(before string) string {
	matches := subqueryFromRe.FindAllStringSubmatchIndex(before, -1)
	if len(matches) == 0 {
		return ""
	}
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		segment := before[m[0]:]
		depth := 0
		open := false
		for j := 0; j < len(segment); j++ {
			ch := segment[j]
			switch ch {
			case '(':
				depth++
				open = true
			case ')':
				if depth > 0 {
					depth--
				}
			}
		}
		if open && depth > 0 {
			return before[m[2]:m[3]]
		}
	}
	return ""
}

func lastClauseKeyword(before string) string {
	lower := strings.ToLower(before)
	keywords := []string{"select", "from", "where", "group by", "having", "order by", "limit", "offset"}
	bestIdx := -1
	bestKw := ""
	for _, kw := range keywords {
		idx := lastIndexWord(lower, kw)
		if idx > bestIdx {
			bestIdx = idx
			bestKw = kw
		}
	}
	switch bestKw {
	case "where", "group by", "order by", "select":
		return bestKw
	case "limit", "offset":
		return bestKw
	case "from", "having":
		return ""
	}
	return ""
}

func lastIndexWord(s, word string) int {
	idx := -1
	off := 0
	for {
		i := strings.Index(s[off:], word)
		if i < 0 {
			return idx
		}
		start := off + i
		end := start + len(word)
		leftOK := start == 0 || !isWordChar(s[start-1])
		rightOK := end == len(s) || !isWordChar(s[end])
		if leftOK && rightOK {
			idx = start
		}
		off = start + 1
	}
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '_'
}

func priorOperatorLHS(before, op string) string {
	lower := strings.ToLower(before)
	opLower := strings.ToLower(op)
	idx := strings.LastIndex(lower, opLower)
	if idx <= 0 {
		return ""
	}
	end := idx
	for end > 0 && before[end-1] == ' ' {
		end--
	}
	start := end
	for start > 0 {
		ch := before[start-1]
		if isWordChar(ch) || ch == '.' {
			start--
			continue
		}
		break
	}
	return before[start:end]
}
