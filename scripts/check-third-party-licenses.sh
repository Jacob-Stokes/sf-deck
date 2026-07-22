#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
generated="$(mktemp -d)"
trap 'rm -rf "$generated"' EXIT

# google/go-licenses v1.6.0 misclassifies modernc.org/mathutil's
# BSD-3-Clause LICENSE as unknown, so check every other dependency through the
# tool and copy that module's license explicitly from the selected Go module.
go run github.com/google/go-licenses@v1.6.0 save ./cmd/sf-deck \
  --save_path "$generated" \
  --force \
  --ignore github.com/Jacob-Stokes/sf-deck \
  --ignore modernc.org/mathutil

mathutil_dir="$(go list -m -f '{{.Dir}}' modernc.org/mathutil)"
mkdir -p "$generated/modernc.org/mathutil"
cp "$mathutil_dir/LICENSE" "$generated/modernc.org/mathutil/LICENSE"

# go-licenses follows the current host's build tags. These modules are used by
# the Darwin and Windows builds but not by Linux, where CI runs. Include their
# notices explicitly so the checked inventory is deterministic and covers every
# release target.
for module in github.com/mattn/go-isatty github.com/ncruces/go-strftime; do
  if [[ ! -f "$generated/$module/LICENSE" ]]; then
    module_dir="$(go list -m -f '{{.Dir}}' "$module")"
    mkdir -p "$generated/$module"
    cp "$module_dir/LICENSE" "$generated/$module/LICENSE"
  fi
done

if ! diff -ru "$repo_root/third_party_licenses" "$generated"; then
  echo "third_party_licenses is stale; regenerate it before release" >&2
  exit 1
fi
