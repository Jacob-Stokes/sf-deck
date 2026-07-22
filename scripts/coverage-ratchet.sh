#!/usr/bin/env bash
# Coverage ratchet: per-package coverage must never drop below the
# checked-in baseline (scripts/coverage-baseline.txt). When a package
# RISES above its baseline, the script tells you to raise it — gains
# become permanent, losses fail the build.
#
# Usage:
#   scripts/coverage-ratchet.sh           # check against baseline
#   scripts/coverage-ratchet.sh --update  # rewrite baseline from current
#
# Tolerance of 0.3pt absorbs run-to-run jitter (parallel test
# scheduling can shift fractions). A real coverage loss is bigger.
set -euo pipefail
cd "$(dirname "$0")/.."

BASELINE="scripts/coverage-baseline.txt"
TOLERANCE="0.3"
MODE="${1:-check}"

current="$(go test -count=1 -cover ./... 2>/dev/null \
  | awk '/coverage:/ { for (i=1;i<=NF;i++) if ($i=="coverage:") printf "%s %s\n", $2, $(i+1) }' \
  | sed 's/%.*//' | sort)"

if [ "$MODE" = "--update" ]; then
  echo "$current" > "$BASELINE"
  echo "baseline rewritten:"
  cat "$BASELINE"
  exit 0
fi

if [ ! -f "$BASELINE" ]; then
  echo "no baseline at $BASELINE — run: scripts/coverage-ratchet.sh --update" >&2
  exit 1
fi

fail=0
while read -r pkg pct; do
  [ -z "$pkg" ] && continue
  base="$(awk -v p="$pkg" '$1==p {print $2}' "$BASELINE")"
  if [ -z "$base" ]; then
    echo "NEW package $pkg at ${pct}% — add it: scripts/coverage-ratchet.sh --update"
    continue
  fi
  drop="$(echo "$base - $pct" | bc -l)"
  if [ "$(echo "$drop > $TOLERANCE" | bc -l)" = "1" ]; then
    echo "FAIL: $pkg coverage ${pct}% is below baseline ${base}%"
    fail=1
  fi
  rise="$(echo "$pct - $base" | bc -l)"
  if [ "$(echo "$rise > 1.0" | bc -l)" = "1" ]; then
    echo "note: $pkg rose to ${pct}% (baseline ${base}%) — lock it in: scripts/coverage-ratchet.sh --update"
  fi
done <<< "$current"

if [ "$fail" = "1" ]; then
  echo ""
  echo "Coverage dropped. Either add tests or (with justification)"
  echo "rewrite the baseline: scripts/coverage-ratchet.sh --update"
  exit 1
fi
echo "coverage ratchet: ok"
