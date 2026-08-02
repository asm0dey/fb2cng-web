#!/usr/bin/env bash
# Dependency-free tests for scripts/fbc-check.sh. No network: LATEST_FBC is injected.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
script="$here/../fbc-check.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

fail=0
readonly OLD_VERSION=v1.4.5
readonly NEW_VERSION=v1.4.6
assert_eq() { # actual expected message
  local actual=$1 expected=$2 msg=$3
  if [[ "$actual" != "$expected" ]]; then
    echo "FAIL: $msg (got '$actual' want '$expected')" >&2; fail=1
  else
    echo "ok: $msg"
  fi
  return 0
}

# Case 1: already up to date -> changed=false, file untouched.
echo "$OLD_VERSION" > "$tmp/FBC_VERSION"
out="$(LATEST_FBC="$OLD_VERSION" FBC_VERSION_FILE="$tmp/FBC_VERSION" bash "$script")"
assert_eq "$(printf '%s\n' "$out" | grep '^changed=')" "changed=false" "no update -> changed=false"
assert_eq "$(cat "$tmp/FBC_VERSION")" "$OLD_VERSION" "no update -> file unchanged"

# Case 2: newer upstream -> changed=true, version reported, file rewritten.
echo "$OLD_VERSION" > "$tmp/FBC_VERSION"
out="$(LATEST_FBC="$NEW_VERSION" FBC_VERSION_FILE="$tmp/FBC_VERSION" bash "$script")"
assert_eq "$(printf '%s\n' "$out" | grep '^changed=')" "changed=true" "update -> changed=true"
assert_eq "$(printf '%s\n' "$out" | grep '^version=')" "version=$NEW_VERSION" "update -> reports latest"
assert_eq "$(cat "$tmp/FBC_VERSION")" "$NEW_VERSION" "update -> file rewritten"

# Case 3: GITHUB_OUTPUT is appended when set.
echo "$OLD_VERSION" > "$tmp/FBC_VERSION"
: > "$tmp/ghout"
LATEST_FBC="$NEW_VERSION" FBC_VERSION_FILE="$tmp/FBC_VERSION" GITHUB_OUTPUT="$tmp/ghout" bash "$script" >/dev/null
assert_eq "$(grep '^changed=' "$tmp/ghout")" "changed=true" "GITHUB_OUTPUT gets changed="
assert_eq "$(grep '^version=' "$tmp/ghout")" "version=$NEW_VERSION" "GITHUB_OUTPUT gets version="

exit $fail
