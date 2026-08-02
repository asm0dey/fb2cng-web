---
# fb2cng-web-opd1
title: 'Editor: group options into key-derived sub-sections'
status: completed
type: feature
priority: normal
created_at: 2026-08-02T19:55:53Z
updated_at: 2026-08-02T20:19:13Z
parent: fb2cng-web-47t7
---

`document.images.optimize` is buried in a flat 45-row `document` accordion. Split each group into collapsible sub-sections derived from the option key path (2nd segment; bare keys -> `general`). Nested `<details>`, collapsed by default, prettified labels, all groups uniformly. Read/render-only; save path untouched.

Spec: docs/superpowers/specs/2026-08-02-editor-option-sections-design.md
Plan: docs/superpowers/plans/2026-08-02-editor-option-sections.md

Tasks (see plan):
1. Section data model + partitioning (editor.go)
2. Nested section rendering + section-aware JS (editor.gohtml)
3. Section CSS (app.css)

## Summary of Changes

Split editor option groups into key-derived sub-sections. 3 commits + coverage test:
- `ec45b25` editor.go: `sectionVM`, `groupVM.Sections/Flat`, `sectionName`/`prettify`/`partition`, `finalize` sums over sections.
- `3f54fe3` editor.gohtml: `{{define "optrow"}}` partial, nested `<details class="opt-section">` (flat groups render rows directly), section-aware `apply()`/`recount()` JS.
- `d0cf702` app.css: `.opt-section*` styles + CSS token guard.
- `304e27b` test: `TestPrettify` covers underscore path.

Section rule: key split on `.`, `>=3` segments → 2nd segment, else `general`; applies to synthetic rows. Sections keep schema first-seen order, collapsed by default, prettified labels. E2E verified against real schema: document→9 sections (General…Text transformations), logging→console/file, version/reporting flat. Save path untouched. Final review: ready to merge.
