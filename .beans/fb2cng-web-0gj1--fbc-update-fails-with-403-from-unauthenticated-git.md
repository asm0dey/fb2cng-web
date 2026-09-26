---
# fb2cng-web-0gj1
title: fbc-update fails with 403 from unauthenticated GitHub API
status: completed
type: bug
created_at: 2026-09-26T10:30:32Z
updated_at: 2026-09-26T10:30:32Z
---

Nightly fbc-update run 36235870562 failed: scripts/fbc-check.sh curl to api.github.com/repos/rupor-github/fb2cng/releases/latest returned 403 (unauthenticated rate limit on shared runner IP).

## Summary of Changes
- fbc-check.sh sends Authorization: Bearer $GH_TOKEN when set
- fbc-update.yml passes GITHUB_TOKEN to the check step
