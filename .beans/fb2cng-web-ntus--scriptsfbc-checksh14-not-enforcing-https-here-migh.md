---
# fb2cng-web-ntus
title: scripts/fbc-check.sh:14 — Not enforcing HTTPS here might allow for redirections to inse...
status: completed
type: bug
priority: normal
tags:
    - sonar
    - shell-S6506
    - security
created_at: 2026-08-02T16:11:37Z
updated_at: 2026-08-02T16:20:38Z
parent: fb2cng-web-y1cx
---

**File:** `scripts/fbc-check.sh`
**Lines:** 14
**Rule:** shell:S6506 (MAJOR VULNERABILITY)

Not enforcing HTTPS here might allow for redirections to insecure websites. Make sure it is safe here.

SonarCloud: https://sonarcloud.io/project/issues?id=asm0dey_fb2cng-web&open=AZ_DO4QyC2tP99pxFsGe



---
Safe: line 14 curl already uses https://api.github.com with -fsSL. Security hotspot acknowledged, no code change needed.
