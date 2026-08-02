---
# fb2cng-web-x9g6
title: Preset editor page had no CSS (raw unstyled column)
status: completed
type: bug
created_at: 2026-08-02T15:46:43Z
updated_at: 2026-08-02T15:46:43Z
parent: fb2cng-web-s726
---

GET /settings/preset/{id} rendered every option as a raw stacked label/input/default with zero styling — Plan 3 built editor.gohtml with .editor-grid/.option-row/.opt-group/.effective classes but never added the matching CSS to app.css (Plan 3 didn't list app.css as modified). Fixed by porting mockup SCREEN 5 into app.css + restructuring editor.gohtml into collapsible <details> groups, 3-col rows, amber changed-row markers, and a sticky effective-config sidebar; also broadened IsTemplate to all *_template keys so they render as textareas. Verified in browser. Commit 232a4c0.

## Summary of Changes
- internal/web/static/app.css: full editor style block (toolbar, group details, 3-col option rows, changed amber rule/dot/tint, effective sidebar, mobile breakpoint).
- internal/web/templates/editor.gohtml: collapsible groups + collapse/expand-all; template rows -> textareas.
- internal/server/editor.go: IsTemplate = HasSuffix(key, '_template').
- internal/server/editor_test.go: assertions updated to new markup.
