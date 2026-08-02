---
# fb2cng-web-4b5a
title: 'Flaky test: TestConvertUsesChosenPresetOverrides races TempDir cleanup'
status: completed
type: bug
priority: normal
created_at: 2026-08-02T13:35:18Z
updated_at: 2026-08-02T13:46:49Z
parent: fb2cng-web-ajic
---

internal/server/presets_http_test.go TestConvertUsesChosenPresetOverrides ~1-in-20 flake: 'TempDir RemoveAll cleanup: directory not empty'. The test does not wait for the async processFiles goroutine to finish writing under the job dir before t.TempDir()'s cleanup runs. Pre-existing (reproduces on code before the i5e5 fix). Fix: poll job status until Done (like waitDone helper) before the test returns, or otherwise synchronize with the worker. Test-hygiene only, no product defect. Source: Plan 2 Task 9 i5e5-fix report.
