---
# fb2cng-web-ha7l
title: internal/server/handlers.go:85 — Define a constant instead of duplicating this literal ...
status: completed
type: task
priority: high
tags:
    - sonar
    - go-S1192
created_at: 2026-08-02T16:11:37Z
updated_at: 2026-08-02T16:20:22Z
parent: fb2cng-web-y1cx
---

**File:** `internal/server/handlers.go`
**Lines:** 85
**Rule:** go:S1192 (CRITICAL CODE_SMELL)

Define a constant instead of duplicating this literal "server error" 5 times.

SonarCloud: https://sonarcloud.io/project/issues?id=asm0dey_fb2cng-web&open=AZ_DO4OAC2tP99pxFsGA
