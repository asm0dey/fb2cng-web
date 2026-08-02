---
# fb2cng-web-opd1
title: 'Editor: group options into key-derived sub-sections'
status: todo
type: feature
priority: normal
created_at: 2026-08-02T19:55:53Z
updated_at: 2026-08-02T19:55:53Z
parent: fb2cng-web-47t7
---

`document.images.optimize` is buried in a flat 45-row `document` accordion. Split each group into collapsible sub-sections derived from the option key path (2nd segment; bare keys -> `general`). Nested `<details>`, collapsed by default, prettified labels, all groups uniformly. Read/render-only; save path untouched.

Spec: docs/superpowers/specs/2026-08-02-editor-option-sections-design.md
Plan: docs/superpowers/plans/2026-08-02-editor-option-sections.md

Tasks (see plan):
1. Section data model + partitioning (editor.go)
2. Nested section rendering + section-aware JS (editor.gohtml)
3. Section CSS (app.css)
