---
# fb2cng-web-nut1
title: internal/web/app.js:19,25 — Prefer top-level await over using a promise chain.
status: completed
type: task
priority: normal
tags:
    - sonar
    - javascript-S7785
created_at: 2026-08-02T16:11:37Z
updated_at: 2026-08-02T16:20:38Z
parent: fb2cng-web-y1cx
---

**File:** `internal/web/app.js`
**Lines:** 19, 25
**Rule:** javascript:S7785 (MAJOR CODE_SMELL)

Prefer top-level await over using a promise chain.

SonarCloud: https://sonarcloud.io/project/issues?id=asm0dey_fb2cng-web&open=AZ_DO4QLC2tP99pxFsGE



---
Obsolete: app.js was rewritten (now internal/web/static/app.js). No promise chains remain — theme + drag-drop use event listeners only.
