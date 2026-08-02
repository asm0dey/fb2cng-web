---
# fb2cng-web-c4z1
title: POST /convert orphans a job dir on rejected file extension
status: completed
type: bug
priority: normal
created_at: 2026-08-02T13:38:26Z
updated_at: 2026-08-02T13:46:49Z
parent: fb2cng-web-ajic
---

internal/server/convert_flow.go handleConvert (~114-117): the per-file extension check ('only .fb2 and .zip accepted') runs AFTER s.jobs.Create + config write, so a bad-extension upload returns an error but leaves an orphaned job dir (with config.yaml) on disk. Pre-existing (Plan 1), same orphaned-job family as i5e5/slb6/wx5d. Fix: validate all uploaded file extensions before s.jobs.Create (fail-fast, no job artifacts). Found in Plan 2 Task 9 i5e5-fix review.
