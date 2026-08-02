---
# fb2cng-web-wx5d
title: Retry orphans files as Pending if config build/write fails
status: completed
type: bug
priority: normal
created_at: 2026-08-02T11:34:10Z
updated_at: 2026-08-02T11:53:38Z
parent: fb2cng-web-odgc
---

internal/server/convert_flow.go handleRetry (~408-417): failed files are flipped to StatePending and Save()d under s.mu BEFORE convert.BuildConfig / retry-config.yaml write. If that config step fails, files stay StatePending with no worker launched, and a later retry sees no StateFailed to re-catch -> stuck forever (until TTL sweep). Fix: build/write the config BEFORE flipping to Pending, or roll back to StateFailed on config error. Pre-existing ordering from Task 10 stub; found in Task 10 fix re-review (non-blocking).
