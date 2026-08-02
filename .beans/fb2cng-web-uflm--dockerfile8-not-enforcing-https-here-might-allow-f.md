---
# fb2cng-web-uflm
title: Dockerfile:8 — Not enforcing HTTPS here might allow for redirections to insecure websit...
status: completed
type: bug
priority: normal
tags:
    - sonar
    - docker-S6506
    - security
created_at: 2026-08-02T16:11:37Z
updated_at: 2026-08-02T16:20:38Z
parent: fb2cng-web-y1cx
---

**File:** `Dockerfile`
**Lines:** 8
**Rule:** docker:S6506 (MAJOR VULNERABILITY)

Not enforcing HTTPS here might allow for redirections to insecure websites. Make sure it is safe here.

SonarCloud: https://sonarcloud.io/project/issues?id=asm0dey_fb2cng-web&open=AZ_DO4RBC2tP99pxFsGm



---
Safe: line 8 curl already uses an https:// GitHub releases URL with -fsSL. Security hotspot acknowledged, no code change needed.
