---
# fb2cng-web-r1g2
title: internal/server/editor.go:94 — Reduce buildEditorVM cognitive complexity 18→15
status: completed
type: task
priority: high
tags:
    - sonar
    - go-S3776
created_at: 2026-08-02T18:57:41Z
updated_at: 2026-08-02T19:01:40Z
parent: fb2cng-web-wk0l
---

**Rule:** go:S3776 (CRITICAL). buildEditorVM complexity 18 > 15. Extract schema-rows / synthetic-rows loops into helpers.
