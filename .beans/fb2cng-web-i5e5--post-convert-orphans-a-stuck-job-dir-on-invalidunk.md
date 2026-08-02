---
# fb2cng-web-i5e5
title: POST /convert orphans a stuck job dir on invalid/unknown preset id
status: completed
type: bug
priority: normal
created_at: 2026-08-02T13:30:10Z
updated_at: 2026-08-02T13:38:26Z
parent: fb2cng-web-ajic
---

internal/server/convert_flow.go handleConvert (~99-146): s.jobs.Create + input-file persistence + status.json seeding (StatePending rows) all happen BEFORE s.buildConfigForPreset(preset). When the preset lookup fails (unknown-but-safe id -> os.ReadFile error, or unsafe id -> validID reject), the handler returns 422 but the job dir, persisted inputs, and status.json survive with StatePending rows. No worker is launched, no job id returned, nothing flips files to StateFailed -> unreachable/unretryable job that sits until TTL sweep. Newly REACHABLE via Plan 2 Task 9 (previously BuildConfig("",...) never errored so the 422 branch was dead code). Sibling of Plan-1 bugs slb6/wx5d (orphaned-Pending-on-failure family). Fix: resolve/validate the preset config (buildConfigForPreset) BEFORE jobs.Create + input persistence; on error return 422 without creating a job. Source: Plan 2 Task 9 review (MEDIUM).
