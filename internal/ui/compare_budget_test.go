package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/diff"
)

// newBudgetRun builds a run with empty snapshot/hash maps and the given
// per-body cap + ceiling (bytes), as startCompare would.
func newBudgetRun(bodyCap int, ceiling int64) *compareRun {
	return &compareRun{
		snapA: diff.Snapshot{}, snapB: diff.Snapshot{},
		hashA: diff.Snapshot{}, hashB: diff.Snapshot{},
		bodyCap: bodyCap, retainCeiling: ceiling,
	}
}

func TestRecordComponentsPerBodyCap(t *testing.T) {
	run := newBudgetRun(100 /*bytes*/, 0 /*no ceiling*/)
	small := strings.Repeat("x", 50) // under cap → retained
	big := strings.Repeat("y", 5000) // over cap → hash only
	run.recordComponents("source", "Profile", map[string]string{
		"Small": small, "Big": big,
	})

	// Hashes recorded for BOTH (so the diff is complete).
	if run.hashA["Profile"]["Small"] == "" || run.hashA["Profile"]["Big"] == "" {
		t.Fatalf("expected hashes for both; got %+v", run.hashA["Profile"])
	}
	// Body retained only for the small one.
	if _, ok := run.snapA["Profile"]["Small"]; !ok {
		t.Error("small body should be retained")
	}
	if _, ok := run.snapA["Profile"]["Big"]; ok {
		t.Error("oversized body should NOT be retained")
	}
	if run.retainedBytes != int64(len(small)) {
		t.Errorf("retainedBytes = %d, want %d", run.retainedBytes, len(small))
	}
}

func TestRecordComponentsTotalCeiling(t *testing.T) {
	// No per-body cap; ceiling of 120 bytes. Three 50-byte bodies: first
	// two fit (100), third would exceed 120 → hash only.
	run := newBudgetRun(0, 120)
	body := strings.Repeat("z", 50)
	run.recordComponents("target", "Layout", map[string]string{"A": body})
	run.recordComponents("target", "Layout", map[string]string{"B": body})
	run.recordComponents("target", "Layout", map[string]string{"C": body})

	got := 0
	for _, k := range []string{"A", "B", "C"} {
		if _, ok := run.snapB["Layout"][k]; ok {
			got++
		}
		if run.hashB["Layout"][k] == "" {
			t.Errorf("hash missing for %s", k) // all hashes must exist
		}
	}
	if got != 2 {
		t.Errorf("retained %d bodies, want 2 (ceiling stops the third)", got)
	}
}

func TestHashBodyDistinguishesContentAndLength(t *testing.T) {
	if hashBody("abc") == hashBody("abd") {
		t.Error("different content hashed equal")
	}
	first, second := hashBody("abc"), hashBody("abc")
	if first != second {
		t.Error("same content hashed unequal")
	}
	// Length-prefix guards against a same-hash different-length collision.
	if strings.SplitN(hashBody("abc"), ":", 2)[0] != "3" {
		t.Errorf("hash should be length-prefixed: %q", hashBody("abc"))
	}
}

// Diffing over hashes (what maybeFinishCompare does) yields the same
// Same/Different verdicts as diffing over bodies — proving dropped-body
// rows still get a correct status from their hash.
func TestCompareOverHashesMatchesBodies(t *testing.T) {
	run := newBudgetRun(1 /*tiny cap → everything dropped*/, 0)
	run.recordComponents("source", "Flow", map[string]string{"Same": "AAA", "Diff": "AAA"})
	run.recordComponents("target", "Flow", map[string]string{"Same": "AAA", "Diff": "BBB"})
	inv := diff.CompareSnapshots(run.hashA, run.hashB)
	byKey := map[string]diff.Status{}
	for _, r := range inv.Rows {
		byKey[r.Key] = r.Status
	}
	if byKey["Same"] != diff.StatusSame {
		t.Errorf("Same row status = %v", byKey["Same"])
	}
	if byKey["Diff"] != diff.StatusDifferent {
		t.Errorf("Diff row status = %v", byKey["Diff"])
	}
}

// TestCompareHashesNormalizeCosmeticXML covers audit-7 #14: the
// production path diffs over HASHES, so the hash must be of the
// NORMALIZED body — otherwise CompareSnapshots (which can't re-normalize
// a hash) flags cosmetic-only XML differences (whitespace / reflow /
// trailing newline) as Different. recordComponents now hashes the
// normalized body, so these compare Same.
func TestCompareHashesNormalizeCosmeticXML(t *testing.T) {
	run := newBudgetRun(1<<20, 0)
	// Same XML, different cosmetic formatting between the two orgs.
	srcXML := "<root><a>1</a><b>2</b></root>"
	tgtXML := "<root>\n  <a>1</a>\n  <b>2</b>\n</root>\n"
	run.recordComponents("source", "CustomObject", map[string]string{"Acct": srcXML})
	run.recordComponents("target", "CustomObject", map[string]string{"Acct": tgtXML})
	inv := diff.CompareSnapshots(run.hashA, run.hashB)
	if len(inv.Rows) != 1 {
		t.Fatalf("expected 1 row, got %+v", inv.Rows)
	}
	if inv.Rows[0].Status != diff.StatusSame {
		t.Errorf("cosmetic-only XML difference flagged %v, want Same", inv.Rows[0].Status)
	}
}

func TestHumanizeCompareAge(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "moments"},
		{5 * time.Minute, "5m"},
		{3 * time.Hour, "3h"},
		{50 * time.Hour, "2d"},
	}
	for _, c := range cases {
		if got := humanizeCompareAge(c.d); got != c.want {
			t.Errorf("humanizeCompareAge(%s) = %q, want %q", c.d, got, c.want)
		}
	}
}
