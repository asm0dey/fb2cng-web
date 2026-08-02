#!/usr/bin/env bash
# Dependency-free tests for scripts/bump-version.sh.
# No git index touched: staged files injected via $CHANGED_FILES, restaging disabled via NO_GIT_ADD=1.
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
script="$here/../bump-version.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
vf="$tmp/VERSION"
fail=0
assert_eq() {
  local actual=$1 expected=$2 msg=$3
  if [[ "$actual" != "$expected" ]]; then echo "FAIL: $msg (got '$actual' want '$expected')" >&2; fail=1; else echo "ok: $msg"; fi
  return 0
}

# Code staged -> bump 1 -> 2
echo 1 > "$vf"
CHANGED_FILES=$'main.go\nREADME.md' VERSION_FILE="$vf" NO_GIT_ADD=1 bash "$script" >/dev/null
assert_eq "$(cat "$vf")" "2" "go change bumps version"

# Dockerfile staged -> bump
echo 5 > "$vf"
CHANGED_FILES="Dockerfile" VERSION_FILE="$vf" NO_GIT_ADD=1 bash "$script" >/dev/null
assert_eq "$(cat "$vf")" "6" "Dockerfile change bumps version"

# web asset staged -> bump
echo 9 > "$vf"
CHANGED_FILES="internal/web/app.js" VERSION_FILE="$vf" NO_GIT_ADD=1 bash "$script" >/dev/null
assert_eq "$(cat "$vf")" "10" "web asset change bumps version"

# docs/fbc only -> no bump
echo 3 > "$vf"
CHANGED_FILES=$'README.md\ndocs/x.md\nFBC_VERSION' VERSION_FILE="$vf" NO_GIT_ADD=1 bash "$script" >/dev/null
assert_eq "$(cat "$vf")" "3" "docs-only does not bump"

# VERSION already staged -> skip even with code
echo 7 > "$vf"
CHANGED_FILES=$'main.go\nVERSION' VERSION_FILE="$vf" NO_GIT_ADD=1 bash "$script" >/dev/null
assert_eq "$(cat "$vf")" "7" "already-staged VERSION is respected"

# fbc-only change -> no bump
echo 4 > "$vf"
CHANGED_FILES="FBC_VERSION" VERSION_FILE="$vf" NO_GIT_ADD=1 bash "$script" >/dev/null
assert_eq "$(cat "$vf")" "4" "FBC_VERSION-only does not bump"

# non-integer VERSION -> error exit 1
echo "v1.2" > "$vf"
if CHANGED_FILES="main.go" VERSION_FILE="$vf" NO_GIT_ADD=1 bash "$script" >/dev/null 2>&1; then
  echo "FAIL: non-integer VERSION should error" >&2; fail=1
else
  echo "ok: non-integer VERSION errors"
fi

exit $fail
