---
# fb2cng-web-yp87
title: scripts/fbc-check.sh:13,16,23,32 — Use '[[' instead of '[' for conditional tests. The '...
status: completed
type: task
priority: normal
tags:
    - sonar
    - shelldre-S7688
created_at: 2026-08-02T16:11:37Z
updated_at: 2026-08-02T16:20:22Z
parent: fb2cng-web-y1cx
---

**File:** `scripts/fbc-check.sh`
**Lines:** 13, 16, 23, 32
**Rule:** shelldre:S7688 (MAJOR CODE_SMELL)

Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich.

SonarCloud: https://sonarcloud.io/project/issues?id=asm0dey_fb2cng-web&open=AZ_DO4QyC2tP99pxFsGf
