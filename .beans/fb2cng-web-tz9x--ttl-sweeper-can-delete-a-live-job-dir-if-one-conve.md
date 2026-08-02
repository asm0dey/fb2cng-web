---
# fb2cng-web-tz9x
title: TTL sweeper can delete a live job dir if one conversion exceeds JOBS_TTL
status: completed
type: bug
priority: normal
created_at: 2026-08-02T14:50:22Z
updated_at: 2026-08-02T15:19:33Z
parent: fb2cng-web-s726
---

internal/jobs Sweep keys on dir mtime; a single conversion (or queue backlog) exceeding JOBS_TTL between status.json writes could let RemoveAll delete a live job dir. Blast radius = one ephemeral job (updateFile's Load fails silently, downloads 404); no server-wide impact. Practically unreachable with 1h default + second-scale conversions. Low priority. Fix: skip dirs with a non-terminal status.json, or touch mtime on job start. Source: final whole-branch review (Minor #6).
