---
# fb2cng-web-0f73
title: schemagen drops commented/disabled subtrees (vignettes, stylesheet_path, cover.default_image_path)
status: todo
type: bug
priority: normal
created_at: 2026-08-02T20:46:01Z
updated_at: 2026-08-02T20:46:01Z
parent: fb2cng-web-47t7
---

options.json is generated from `fbc dumpconfig --default`, which omits keys that are commented/empty in config.yaml.tmpl. So schemagen never emitted:
- document.vignettes.{book,chapter,section}.{title_top,title_bottom,end} (8)
- document.stylesheet_path
- document.images.cover.default_image_path

These were added by hand to options.json (commits 6780063, 8499689) so the editor exposes them. But re-running `go generate ./internal/schema` (schemagen against a real fbc dump) will DROP them again — silent regression.

Fix options:
1. schemagen merges a checked-in overlay of always-include keys (dotted key + kind + default + description) on top of the dump.
2. Or parse the commented lines in config.yaml.tmpl too.

Related: [[km22]] (regenerate options.json against real dump), kqdg (description drift).
