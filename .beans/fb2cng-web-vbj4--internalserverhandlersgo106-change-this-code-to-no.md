---
# fb2cng-web-vbj4
title: internal/server/handlers.go:106 — Change this code to not construct the path from user-...
status: completed
type: bug
priority: critical
tags:
    - sonar
    - gosecurity-S2083
    - security
created_at: 2026-08-02T16:11:37Z
updated_at: 2026-08-02T16:20:38Z
parent: fb2cng-web-y1cx
---

**File:** `internal/server/handlers.go`
**Lines:** 106
**Rule:** gosecurity:S2083 (BLOCKER VULNERABILITY)

Change this code to not construct the path from user-controlled data.

SonarCloud: https://sonarcloud.io/project/issues?id=asm0dey_fb2cng-web&open=AZ_DO4OAC2tP99pxFsGB



---
Resolved: the flagged handlers.go:106 line no longer exists. Download/log paths now go through jobs.OutputFile / LogPath, both guarded by safeName() (rejects `/  ..`) and ValidID(); handleDownload additionally whitelists file against the job's known outputs. No path is built from unsanitized user data.
