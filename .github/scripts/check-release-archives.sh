#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dist_dir="${1:-"$repo_root/dist"}"
license_dir="$repo_root/docs/third_party_licenses"

if [[ ! -d "$license_dir" ]]; then
  echo "missing third-party licence directory: $license_dir" >&2
  exit 1
fi

shopt -s nullglob
archives=("$dist_dir"/*.tar.gz)
if [[ "${#archives[@]}" -eq 0 ]]; then
  echo "no release archives found in $dist_dir" >&2
  exit 1
fi

expected_licenses="$(mktemp)"
actual_licenses="$(mktemp)"
trap 'rm -f "$expected_licenses" "$actual_licenses"' EXIT

(
  cd "$repo_root"
  find docs/third_party_licenses -type f -print | sort
) >"$expected_licenses"

for archive in "${archives[@]}"; do
  listing="$(tar -tzf "$archive")"

  for required in sf-deck LICENSE NOTICE README.md; do
    if ! grep -qx "$required" <<<"$listing"; then
      echo "$(basename "$archive") is missing $required" >&2
      exit 1
    fi
  done

  grep '^docs/third_party_licenses/' <<<"$listing" | sort >"$actual_licenses"
  if ! diff -u "$expected_licenses" "$actual_licenses"; then
    echo "$(basename "$archive") has an incomplete third-party licence inventory" >&2
    exit 1
  fi

  count="$(wc -l <"$actual_licenses" | tr -d ' ')"
  echo "$(basename "$archive"): release notices present ($count third-party files)"
done

echo "release archive notices: ok (${#archives[@]} archives)"
