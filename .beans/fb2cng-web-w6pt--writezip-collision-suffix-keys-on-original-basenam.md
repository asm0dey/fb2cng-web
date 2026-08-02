---
# fb2cng-web-w6pt
title: WriteZip collision suffix keys on original basename only
status: completed
type: bug
priority: normal
created_at: 2026-08-02T10:31:43Z
updated_at: 2026-08-02T11:53:38Z
parent: fb2cng-web-odgc
---

internal/jobs/zip_sweep.go:32-37 — collision suffixing derives -N names from the ORIGINAL basename, not the final chosen name. If input A -> book.epub gets suffixed to book-1.epub while a distinct input B genuinely produces book-1.epub, the two silently collide in the zip (archive/zip permits duplicate entry names). Narrow edge case, not covered by shipped test. Source: Plan 1 Task 4 review (Minor, plan-mandated).
