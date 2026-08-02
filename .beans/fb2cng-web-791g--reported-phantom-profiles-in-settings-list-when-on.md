---
# fb2cng-web-791g
title: 'Reported: phantom profiles in Settings list when only Defaults expected (NOT reproduced)'
status: scrapped
type: bug
priority: normal
created_at: 2026-08-02T15:26:54Z
updated_at: 2026-08-02T15:31:33Z
parent: fb2cng-web-s726
---

User reported the Settings/Presets list showing many profiles when only Defaults should exist. NOT reproduced: live :8080 (PRESETS_DIR=/data/presets missing -> List returns only Defaults) shows only Defaults; a fresh instance with a writable PRESETS_DIR also shows only Defaults, and duplicate creates exactly one 'Defaults copy'. presets.Store.List() seeds builtin() first then lists <id>.yaml files, skipping _meta.yaml/dirs. Need user's PRESETS_DIR value + the profile NAMES shown to repro; likely stale state or a PRESETS_DIR pointed at a populated dir. Revisit after the PRESETS_DIR-default fix.
