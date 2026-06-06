#!/usr/bin/env bash
# Detect whether rupor-github/fb2cng (fbc) has a release newer than the pinned FBC_VERSION.
# - Reads/writes the version file (default ./FBC_VERSION; override with $FBC_VERSION_FILE).
# - "Latest" comes from the GitHub API, or from $LATEST_FBC when set (used by tests).
# - On change: rewrites the file. Always prints `changed=<bool>` and `version=<latest>`,
#   and appends both to $GITHUB_OUTPUT when that var is set (GitHub Actions).
set -euo pipefail

FBC_REPO="${FBC_REPO:-rupor-github/fb2cng}"
FILE="${FBC_VERSION_FILE:-FBC_VERSION}"

latest="${LATEST_FBC:-}"
if [ -z "$latest" ]; then
  latest="$(curl -fsSL "https://api.github.com/repos/${FBC_REPO}/releases/latest" | jq -r '.tag_name')"
fi
if [ -z "$latest" ] || [ "$latest" = "null" ]; then
  echo "fbc-check: failed to determine latest fbc version" >&2
  exit 1
fi

current="$(cat "$FILE")"

if [ "$latest" = "$current" ]; then
  changed=false
else
  changed=true
  printf '%s\n' "$latest" > "$FILE"
fi

echo "changed=$changed"
echo "version=$latest"
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  {
    echo "changed=$changed"
    echo "version=$latest"
  } >> "$GITHUB_OUTPUT"
fi
