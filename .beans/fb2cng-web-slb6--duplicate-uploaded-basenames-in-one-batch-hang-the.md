---
# fb2cng-web-slb6
title: Duplicate uploaded basenames in one batch hang the job
status: completed
type: bug
priority: normal
created_at: 2026-08-02T10:55:13Z
updated_at: 2026-08-02T11:53:38Z
parent: fb2cng-web-odgc
---

internal/server/convert_flow.go (handleConvert/processFiles/updateFile) — if one POST /convert uploads two files whose filepath.Base collides (e.g. two 'book.fb2'), the second os.Create(InputPath) overwrites the first input, and two StatePending rows share the same Input. processFiles runs 'book.fb2' twice; updateFile's loop matches Input and breaks on the FIRST row, so the second pending row never goes terminal -> allTerminal stays false -> Done never true -> job never completes (only TTL sweep clears it). Fix: de-dupe/suffix colliding input basenames on upload, or key FileResults by index rather than Input name. Source: Plan 1 Task 7 review (Minor, out-of-scope). Also note (informational): StateRunning is never set; files jump pending->done/failed.
