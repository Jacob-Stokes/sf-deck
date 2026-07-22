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

	// Subquery detection — paren-balance on text BEFORE the caret.
	// If we're inside an unbalanced `(`, look for the innermost
	// `(SELECT ... FROM <child>` and use that as the active sObject.
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
		// IN(...) opens a paren that isn't a subquery — clear the
		// false-positive subquery flag set by isInSubquery above.
		c.InSubquery = false
		return c
	}

	// 3. Right side of comparison operator — pull picklist /
	// boolean / date literal values for the LHS field. Regex
	// captures the operator (group 1) and the partial value
	// (group 2 — what the user has typed of the literal).
	if m := operatorRHSRe.FindStringSubmatch(before); m != nil {
		c.Context = ContextWhereValue
		c.OperatorRHS = true
		c.OperatorOp = strings.ToUpper(m[1])
		c.SearchToken = strings.TrimPrefix(m[2], "'")
		c.WhereField = priorOperatorLHS(before, c.OperatorOp)
		return c
	}

	// 4. Clause keywords — ORDER BY > GROUP BY > WHERE.
	// Walk back from the cursor to find the most-recent clause
	// keyword that's still "open" (no closing keyword between it
	// and the cursor).
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

	// 5. Fall-through: caller is at top-level.
	c.Context = ContextTopLevel
	return c
}

var (
	// Match the sObject slot after FROM: either empty (`FROM `) or
	// a partial name being typed (`FROM Acco`). Requires at least
	// one whitespace after FROM so we don't match `from` mid-word
	// (e.g. a custom field named `Promotional_From__c`).
	afterFromKeywordRe = regexp.MustCompile(`(?i)(?:^|\s)from\s+[a-z0-9_]*$`)

	// Operator RHS: any of = != <> < > <= >= LIKE (case-insensitive
	// for LIKE — the symbols don't care). Inspector's regex is
	// `\s*[<>=!]+\s*('?[^'\s]*)$` — symbols only. We extend to
	// catch `LIKE` since Inspector's WHERE-value branch fires for
	// any operator including LIKE via a separate code path; here we
	// fold it into one regex.
	operatorRHSRe = regexp.MustCompile(`(?i)\s*([<>=!]+|like)\s+('?[^'\s]*)$`)

	// IN (...) value list: at least one quoted-and-comma'd token OR
	// the opening of the first token. Matches Inspector's regex.
	inWithValuesRe = regexp.MustCompile(`(?i)\s*in\s*\(\s*(?:(?:'[^']*'\s*,\s*)+|)('?[^'\s]*)$`)

	// FROM <name> scanner used to resolve the outer-query's
	// active sObject. Captures the name (may be empty mid-type).
	fromObjectRe = regexp.MustCompile(`(?i)(?:^|\s)from\s+([a-z0-9_]+)`)

	// Subquery FROM scanner — same but inside parens.
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

// trailingToken returns the trailing run of word characters at the
// end of `before`. Empty when the caret sits on whitespace or a
// non-word character.
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
	// Strip out parenthesized substrings so subquery FROMs don't
	// shadow the outer one. Cheap state machine — easier than
	// nested-regex.
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

// isInSubquery checks whether the caret is inside an unbalanced
// `(SELECT ...` block. Plain unbalanced parens (e.g. `IN (`,
// `Count(`) don't count — only ones immediately followed by a
// SELECT keyword. Quote-aware: characters inside '...' literals
// don't shift depth.
//
// Strategy: scan left-to-right tracking paren depth + a stack of
// "is this paren a subquery?" flags. Return true when the outermost
// still-open paren on the stack is flagged as a subquery.
func isInSubquery(before string) bool {
	inStr := false
	// Stack of bool: true means "this open `(` is followed by SELECT".
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
			// Peek ahead past whitespace to look for SELECT.
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

// innermostSubquerySObject finds the FROM of the deepest
// still-open `(SELECT ...` block before the caret.
func innermostSubquerySObject(before string) string {
	// Walk all subquery FROM matches; the last one whose opening
	// paren is still unbalanced at the cursor is the active one.
	matches := subqueryFromRe.FindAllStringSubmatchIndex(before, -1)
	if len(matches) == 0 {
		return ""
	}
	// Iterate in reverse — innermost (latest) match wins.
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		// m[0] is the start of the `(` for this subquery match.
		// Count parens between m[0] and len(before). If balanced
		// is < 0 (we've passed the closing paren) skip it.
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

// lastClauseKeyword reports the most-recent open clause keyword
// before the caret. "Open" means no later clause keyword has
// appeared that would close it. Priority within the same scan:
// ORDER BY > GROUP BY > HAVING > WHERE > SELECT.
//
// Returns "" when no clause keyword is open (e.g. caret is in
// LIMIT/OFFSET territory, or before SELECT).
func lastClauseKeyword(before string) string {
	lower := strings.ToLower(before)
	// Walk known keywords; track the latest index seen.
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
		// FROM — afterFromKeywordRe handles the sObject-name slot.
		// HAVING — uses aggregate expressions we don't model yet.
		return ""
	}
	return ""
}

// lastIndexWord finds the last occurrence of `word` in `s` that is
// surrounded by word boundaries (start/end of string or non-word
// char). Case-sensitive — caller should ToLower first.
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
		// Word boundary on both sides.
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

// priorOperatorLHS extracts the field path that's the LHS of the
// most-recent operator before the cursor. e.g. for
// "WHERE Owner.Name = '" with op `=`, returns "Owner.Name".
func priorOperatorLHS(before, op string) string {
	// Find the last occurrence of the operator.
	lower := strings.ToLower(before)
	opLower := strings.ToLower(op)
	idx := strings.LastIndex(lower, opLower)
	if idx <= 0 {
		return ""
	}
	// Walk left past whitespace, then collect word-or-dot chars.
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
