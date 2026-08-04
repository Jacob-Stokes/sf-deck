// Package diff provides line-level text diffing for sf-deck's metadata
// compare feature. No external diff library exists in the dependency
// tree, so this is a self-contained LCS-based line differ producing
// aligned hunks suitable for both unified and side-by-side rendering.
//
// The algorithm is the classic longest-common-subsequence dynamic
// program over whole lines (not characters). Metadata bodies are
// modest (a few hundred to low-thousands of lines), so the O(n·m)
// table is fine — we don't need the memory-optimised Myers variant.
package diff

// Op is the role of one aligned row in a diff.
type Op int

const (
	OpEqual  Op = iota // line present and identical on both sides
	OpDelete           // line present in A only (removed)
	OpInsert           // line present in B only (added)
)

// Line is one row of an aligned diff. For OpEqual both A and B hold the
// (identical) text and ALine/BLine are both set. For OpDelete only the
// A side is meaningful (BLine == 0). For OpInsert only the B side is
// meaningful (ALine == 0). Line numbers are 1-based; 0 means "no line
// on this side" — the renderer shows a gap there.
type Line struct {
	Op    Op
	Text  string // the line content (A's for delete/equal, B's for insert)
	BText string // B's content for OpEqual convenience (== Text); set only on equal
	ALine int    // 1-based line number in A, or 0
	BLine int    // 1-based line number in B, or 0
}

// Result is a full aligned diff plus a quick changed-line count.
type Result struct {
	Lines   []Line
	Added   int // count of OpInsert
	Removed int // count of OpDelete
}

// Changed reports whether the two inputs differ at all.
func (r Result) Changed() bool { return r.Added > 0 || r.Removed > 0 }

// Lines diffs two slices of lines. The result is an aligned sequence:
// equal runs are emitted as OpEqual, divergences as OpDelete (A) then
// OpInsert (B), in source order.
func Lines(a, b []string) Result {
	// LCS length table. lcs[i][j] = LCS length of a[i:] and b[j:].
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var out Result
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out.Lines = append(out.Lines, Line{
				Op: OpEqual, Text: a[i], BText: b[j],
				ALine: i + 1, BLine: j + 1,
			})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out.Lines = append(out.Lines, Line{
				Op: OpDelete, Text: a[i], ALine: i + 1,
			})
			out.Removed++
			i++
		default:
			out.Lines = append(out.Lines, Line{
				Op: OpInsert, Text: b[j], BLine: j + 1,
			})
			out.Added++
			j++
		}
	}
	for ; i < n; i++ {
		out.Lines = append(out.Lines, Line{Op: OpDelete, Text: a[i], ALine: i + 1})
		out.Removed++
	}
	for ; j < m; j++ {
		out.Lines = append(out.Lines, Line{Op: OpInsert, Text: b[j], BLine: j + 1})
		out.Added++
	}
	return out
}

// Text is Lines over two whole strings, split on "\n". A trailing
// newline does not produce a spurious empty final line.
func Text(a, b string) Result {
	return Lines(splitLines(a), splitLines(b))
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			// Drop a trailing \r so CRLF and LF inputs compare equal.
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			out = append(out, line)
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
