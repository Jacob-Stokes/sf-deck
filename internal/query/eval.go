package query

import (
	"strconv"
	"strings"
	"time"
)

// Row is the field accessor an evaluation target implements. Field
// returns the value at the named column plus an `ok` flag — `false`
// means the row doesn't carry that field at all (vs holding nil/empty,
// which return ok=true). Compare nodes use the ok flag to short-circuit
// against bogus field names.
//
// Implementations should be cheap — Eval calls Field once per
// CompareNode + once per OrderBy. Caching at the Row layer is fine
// when a field is computed.
type Row interface {
	Field(name string) (any, bool)
}

// Eval reports whether `row` matches the predicate tree. A nil node
// matches everything (the "All" built-in case) so callers can hand
// in `Query.Where` directly without a nil-check.
//
// Eval never errors — it returns a bool. Type mismatches (comparing
// a string field to an int literal, etc.) evaluate to false rather
// than panicking, since user-authored queries are the typical input
// and a hard failure deep in the render loop is worse UX than a
// silent "no match".
func Eval(node Node, row Row) bool {
	if node == nil {
		return true
	}
	switch n := node.(type) {
	case AndNode:
		for _, c := range n.Children {
			if !Eval(c, row) {
				return false
			}
		}
		return true
	case OrNode:
		if len(n.Children) == 0 {
			return false
		}
		for _, c := range n.Children {
			if Eval(c, row) {
				return true
			}
		}
		return false
	case NotNode:
		return !Eval(n.Child, row)
	case CompareNode:
		return evalCompare(n, row)
	}
	return false
}

// evalCompare runs one CompareNode against a row. Per-op semantics are
// documented on Op.
func evalCompare(c CompareNode, row Row) bool {
	v, ok := row.Field(c.Field)
	if !ok {
		// Unknown field — only IsNull can match (the field is "null"
		// because it doesn't exist).
		return c.Op == OpIsNull
	}
	switch c.Op {
	case OpIsNull:
		return isNil(v) || isZeroString(v)
	case OpEq:
		return equal(v, c.Value)
	case OpNotEq:
		return !equal(v, c.Value)
	case OpContains:
		return foldContains(toString(v), toString(c.Value))
	case OpStartsWith:
		// foldHasPrefix avoids the two strings.ToLower allocations
		// the previous implementation paid for every comparison —
		// hot path on chips like /apex Tests where 6 OpStartsWith /
		// OpEndsWith clauses run against every row on every render.
		return foldHasPrefix(toString(v), toString(c.Value))
	case OpEndsWith:
		return foldHasSuffix(toString(v), toString(c.Value))
	case OpIn:
		list, ok := c.Value.([]any)
		if !ok {
			return false
		}
		for _, e := range list {
			if equal(v, e) {
				return true
			}
		}
		return false
	case OpDateLiteral:
		return evalDateLiteral(toString(v), toString(c.Value))
	case OpGT, OpGTE, OpLT, OpLTE:
		cmp := compareOrdered(v, c.Value)
		if cmp == cmpMismatch {
			return false
		}
		switch c.Op {
		case OpGT:
			return cmp > 0
		case OpGTE:
			return cmp >= 0
		case OpLT:
			return cmp < 0
		case OpLTE:
			return cmp <= 0
		}
	}
	return false
}

// equal does a permissive equality check. Numeric types compare by
// value across int/int64/float64; strings are case-sensitive (use
// OpContains for fold-equal). Bools and nils compare directly.
func equal(a, b any) bool {
	switch av := a.(type) {
	case string:
		bs, ok := b.(string)
		return ok && av == bs
	case bool:
		bb, ok := b.(bool)
		return ok && av == bb
	case int, int64, float64:
		af, aok := toFloat(a)
		bf, bok := toFloat(b)
		return aok && bok && af == bf
	case nil:
		return b == nil
	}
	return false
}

// compareOrdered returns -1/0/+1 like strings.Compare, working over
// strings (lexical) and numbers. Mismatched types are *not* compared
// — when one side is numeric and the other isn't, we return a
// sentinel value (mismatch) and the caller treats both >0 and <0
// checks as false. This avoids surprising "5 > 'Active'" results
// from a string-vs-int compare leaking into the predicate.
func compareOrdered(a, b any) int {
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if aok && bok {
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		}
		return 0
	}
	if aok != bok {
		// Mixed numeric / non-numeric: not orderable. Return a value
		// that causes both > and < tests to fail. Using a huge magic
		// number (math.MaxInt32) would still satisfy `> 0`; instead
		// the eval path checks for this sentinel via comparable().
		return cmpMismatch
	}
	// Both non-numeric — fall back to string compare.
	return strings.Compare(toString(a), toString(b))
}

// cmpMismatch is the sentinel returned by compareOrdered when the two
// operands aren't of compatible ordered types. Eval inspects this
// before applying GT/LT/GTE/LTE so a mismatch fails the predicate
// instead of producing a misleading lexical compare.
const cmpMismatch = -1 << 30

// toString renders any AST-supported value as a string for the
// string-shaped operators (Contains / StartsWith / EndsWith) or the
// fallback compare path. nil → "".
func toString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return itoa64(int64(x))
	case int64:
		return itoa64(x)
	case float64:
		// Avoid "1e+09"-style output on round numbers; render as the
		// integer it really is when possible.
		if x == float64(int64(x)) {
			return itoa64(int64(x))
		}
		return ftoa(x)
	}
	return ""
}

// toFloat coerces numeric AST values to float64 for ordered compare.
// String → false (only numeric types are ordered numerically). The
// caller falls back to lexical compare when toFloat returns false.
func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

// isNil — true for an explicit nil interface or a typed nil.
func isNil(v any) bool {
	return v == nil
}

// isZeroString treats the empty string as null too, matching how
// Salesforce serialises absent values in REST responses.
func isZeroString(v any) bool {
	s, ok := v.(string)
	return ok && s == ""
}

// evalDateLiteral resolves a Salesforce date-literal token (TODAY /
// YESTERDAY / THIS_WEEK / THIS_MONTH / LAST_N_DAYS:N / NEXT_N_DAYS:N)
// against the row's date string. The row value is expected to be ISO-
// 8601 ("2025-04-25" or "2025-04-25T12:34:56Z"). Unknown literals
// fail closed (return false) since the only safe behaviour is "don't
// silently match everything".
func evalDateLiteral(rowDate, literal string) bool {
	if rowDate == "" || literal == "" {
		return false
	}
	// Normalise the row date to a YYYY-MM-DD prefix for window checks.
	day := rowDate
	if len(day) >= 10 {
		day = day[:10]
	}
	rowT, err := time.Parse("2006-01-02", day)
	if err != nil {
		return false
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	upper := strings.ToUpper(strings.TrimSpace(literal))
	switch {
	case upper == "TODAY":
		return rowT.Equal(today)
	case upper == "YESTERDAY":
		return rowT.Equal(today.AddDate(0, 0, -1))
	case upper == "THIS_WEEK":
		// Salesforce week starts on Sunday.
		weekStart := today.AddDate(0, 0, -int(today.Weekday()))
		weekEnd := weekStart.AddDate(0, 0, 7)
		return !rowT.Before(weekStart) && rowT.Before(weekEnd)
	case upper == "LAST_WEEK":
		weekStart := today.AddDate(0, 0, -int(today.Weekday())-7)
		weekEnd := weekStart.AddDate(0, 0, 7)
		return !rowT.Before(weekStart) && rowT.Before(weekEnd)
	case upper == "THIS_MONTH":
		monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
		monthEnd := monthStart.AddDate(0, 1, 0)
		return !rowT.Before(monthStart) && rowT.Before(monthEnd)
	case upper == "LAST_MONTH":
		monthStart := time.Date(today.Year(), today.Month()-1, 1, 0, 0, 0, 0, time.UTC)
		monthEnd := monthStart.AddDate(0, 1, 0)
		return !rowT.Before(monthStart) && rowT.Before(monthEnd)
	case strings.HasPrefix(upper, "LAST_N_DAYS:"):
		n, ok := parseNDays(upper, "LAST_N_DAYS:")
		if !ok {
			return false
		}
		windowStart := today.AddDate(0, 0, -n)
		windowEnd := today.AddDate(0, 0, 1)
		return !rowT.Before(windowStart) && rowT.Before(windowEnd)
	case strings.HasPrefix(upper, "NEXT_N_DAYS:"):
		n, ok := parseNDays(upper, "NEXT_N_DAYS:")
		if !ok {
			return false
		}
		windowStart := today
		windowEnd := today.AddDate(0, 0, n+1)
		return !rowT.Before(windowStart) && rowT.Before(windowEnd)
	}
	return false
}

func parseNDays(s, prefix string) (int, bool) {
	rest := strings.TrimPrefix(s, prefix)
	n, err := strconv.Atoi(rest)
	if err != nil {
		return 0, false
	}
	return n, true
}

// foldContains is a case-insensitive Contains that avoids
// strings.ToLower's allocations — a real hot path on chip
// predicates that run against every row on every render. Walks
// haystack byte-by-byte using foldEqualByte at each candidate
// start position.
//
// Restricted to ASCII — Salesforce sObject + field names are
// ASCII so the cheaper byte-wise comparison is safe. Non-ASCII
// bytes fall back to byte equality (no folding), which is
// acceptable for the names this engine compares against.
func foldContains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	if len(needle) > len(haystack) {
		return false
	}
	last := len(haystack) - len(needle)
	for i := 0; i <= last; i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if !foldEqualByte(haystack[i+j], needle[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// foldHasPrefix is a case-insensitive HasPrefix that avoids the
// strings.ToLower allocations strings.HasPrefix(strings.ToLower(...))
// would pay. Same hot-path rationale as foldContains.
func foldHasPrefix(s, prefix string) bool {
	if len(prefix) > len(s) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if !foldEqualByte(s[i], prefix[i]) {
			return false
		}
	}
	return true
}

// foldHasSuffix mirrors foldHasPrefix for HasSuffix.
func foldHasSuffix(s, suffix string) bool {
	if len(suffix) > len(s) {
		return false
	}
	off := len(s) - len(suffix)
	for i := 0; i < len(suffix); i++ {
		if !foldEqualByte(s[off+i], suffix[i]) {
			return false
		}
	}
	return true
}

// foldEqualByte is case-insensitive ASCII byte equality. Returns
// raw byte equality for non-ASCII bytes — safe for the all-ASCII
// inputs this engine compares against (sObject + field names).
func foldEqualByte(a, b byte) bool {
	if a == b {
		return true
	}
	if a >= 'A' && a <= 'Z' {
		a += 'a' - 'A'
	}
	if b >= 'A' && b <= 'Z' {
		b += 'a' - 'A'
	}
	return a == b
}

// itoa64 / ftoa — tiny conversions to avoid pulling fmt into the hot
// path. Eval runs once per row per chip, which can be tens of thousands
// of calls on large /objects loads.
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func ftoa(f float64) string {
	// Best-effort: rounded to 6 sig figs, no scientific notation. The
	// exact representation isn't important — this only fires on the
	// stringly-compare fallback for fractional values.
	const digits = 6
	intPart := int64(f)
	frac := f - float64(intPart)
	if frac < 0 {
		frac = -frac
	}
	out := itoa64(intPart) + "."
	for i := 0; i < digits; i++ {
		frac *= 10
		d := int64(frac)
		out += string(byte('0' + d))
		frac -= float64(d)
	}
	return out
}
