#!/usr/bin/env bash
# Bump the integer VERSION when a commit includes app/build code changes.
# Intended as a lefthook pre-commit command. Local-only; not enforced in CI.
# Code = *.go, go.mod, go.sum, Dockerfile, anything under internal/web/.
# Skips when no code is staged, or when VERSION is already staged (manual/prior bump).
# Testable: inject the staged file list via $CHANGED_FILES; set $NO_GIT_ADD=1 to skip restaging.
set -euo pipefail

VERSION_FILE="${VERSION_FILE:-VERSION}"
vf_base="$(basename "$VERSION_FILE")"

if [ -n "${CHANGED_FILES+x}" ]; then
  changed="$CHANGED_FILES"
else
  changed="$(git diff --cached --name-only)"
fi

# VERSION already edited/staged this commit? respect it, do nothing.
if printf '%s\n' "$changed" | grep -qx -- "$vf_base"; then
  echo "bump-version: $vf_base already staged; leaving it"
  exit 0
fi

# Any app/build code staged?
if ! printf '%s\n' "$changed" | grep -Eq '(\.go$|^go\.mod$|^go\.sum$|^Dockerfile$|^internal/web/)'; then
  echo "bump-version: no code changes; VERSION unchanged"
  exit 0
fi

current="$(cat "$VERSION_FILE")"
if ! printf '%s' "$current" | grep -Eq '^[0-9]+$'; then
  echo "bump-version: VERSION ('$current') is not an integer" >&2
  exit 1
fi

next=$((current + 1))
printf '%s\n' "$next" > "$VERSION_FILE"
echo "bump-version: $current -> $next"

if [ "${NO_GIT_ADD:-}" != "1" ]; then
  git add "$VERSION_FILE"
fi
