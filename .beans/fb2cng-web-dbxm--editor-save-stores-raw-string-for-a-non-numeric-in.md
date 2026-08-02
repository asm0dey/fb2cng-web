---
# fb2cng-web-dbxm
title: Editor Save stores raw string for a non-numeric int field
status: completed
type: bug
priority: normal
created_at: 2026-08-02T14:26:53Z
updated_at: 2026-08-02T14:43:32Z
parent: fb2cng-web-47t7
---

internal/server/editor.go parseValue KindInt branch: on strconv.Atoi failure it returns the raw string, which is then stored as a non-numeric override for a schema-declared int field (surfaces later in convert.BuildConfig). Editor uses <input type=number> so normal browsers constrain input, but a crafted POST bypasses it. Brief-verbatim; low severity. Fix: on Atoi failure, drop the field (treat as default) or reject. Source: Plan 3 Task 7 review (Minor).
