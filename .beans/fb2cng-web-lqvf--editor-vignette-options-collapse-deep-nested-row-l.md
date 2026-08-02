---
# fb2cng-web-lqvf
title: 'Editor: vignette options + collapse deep-nested row labels + stylesheet_path/cover paths'
status: completed
type: feature
priority: normal
created_at: 2026-08-02T20:54:48Z
updated_at: 2026-08-02T20:54:48Z
parent: fb2cng-web-47t7
---

Follow-on to [[opd1]] (which had already been marked complete). Done in commits 6780063, 8499689 on feat/editor-option-sections (PR #20).

## Summary of Changes

- Row display label = key path after its section segment (editor.go `rowLabel`): `document.vignettes.chapter.end` → `chapter.end`, `document.text_transformations.speech.enable` → `speech.enable`. 3-segment and bare keys unchanged. Disambiguates the pre-existing text_transformations rows (were all `enable/from/to`).
- Added the 3 commented-out config blocks that `dumpconfig --default` omits, so the editor exposes them:
  - document.vignettes.{book,chapter,section}.{title_top,title_bottom,end} (8) — chapter/section dividers (empty/builtin/path).
  - document.stylesheet_path — custom CSS override.
  - document.images.cover.default_image_path — fallback cover image.
- Schema 52 → 62 options. Tests: rowLabel, deep-label section build; e2e verified Vignettes section + collapsed labels render against real schema. Full suite green.

Follow-up: [[0f73]] — schemagen will drop these hand-added keys on regeneration.
