#!/usr/bin/env bash
# Dependency-free tests for scripts/fbc-check.sh. No network: LATEST_FBC is injected.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
script="$here/../fbc-check.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

fail=0
assert_eq() { # actual expected message
  if [ "$1" != "$2" ]; then
    echo "FAIL: $3 (got '$1' want '$2')"; fail=1
  else
    echo "ok: $3"
  fi
}

# Case 1: already up to date -> changed=false, file untouched.
echo "v1.4.5" > "$tmp/FBC_VERSION"
out="$(LATEST_FBC=v1.4.5 FBC_VERSION_FILE="$tmp/FBC_VERSION" bash "$script")"
assert_eq "$(printf '%s\n' "$out" | grep '^changed=')" "changed=false" "no update -> changed=false"
assert_eq "$(cat "$tmp/FBC_VERSION")" "v1.4.5" "no update -> file unchanged"

# Case 2: newer upstream -> changed=true, version reported, file rewritten.
echo "v1.4.5" > "$tmp/FBC_VERSION"
out="$(LATEST_FBC=v1.4.6 FBC_VERSION_FILE="$tmp/FBC_VERSION" bash "$script")"
assert_eq "$(printf '%s\n' "$out" | grep '^changed=')" "changed=true" "update -> changed=true"
assert_eq "$(printf '%s\n' "$out" | grep '^version=')" "version=v1.4.6" "update -> reports latest"
assert_eq "$(cat "$tmp/FBC_VERSION")" "v1.4.6" "update -> file rewritten"

# Case 3: GITHUB_OUTPUT is appended when set.
echo "v1.4.5" > "$tmp/FBC_VERSION"
: > "$tmp/ghout"
LATEST_FBC=v1.4.6 FBC_VERSION_FILE="$tmp/FBC_VERSION" GITHUB_OUTPUT="$tmp/ghout" bash "$script" >/dev/null
assert_eq "$(grep '^changed=' "$tmp/ghout")" "changed=true" "GITHUB_OUTPUT gets changed="
assert_eq "$(grep '^version=' "$tmp/ghout")" "version=v1.4.6" "GITHUB_OUTPUT gets version="

exit $fail
