---
# fb2cng-web-q2t8
title: WriteZip discards zip.Writer.Close() error
status: completed
type: bug
priority: normal
created_at: 2026-08-02T10:31:43Z
updated_at: 2026-08-02T11:53:38Z
parent: fb2cng-web-odgc
---

internal/jobs/zip_sweep.go:19 — WriteZip does `defer zw.Close()` with an unnamed return, so zip.Writer.Close()'s error is dropped. Close() flushes the central directory to w; if that final write fails (disk full, broken HTTP connection on /jobs/{id}/zip), WriteZip returns nil and the caller treats a truncated/corrupt archive as valid.

Fix: named return + capture:
    func (s *Store) WriteZip(id string, w io.Writer) (err error) {
        zw := zip.NewWriter(w)
        defer func() { if cerr := zw.Close(); err == nil { err = cerr } }()
        ...
    }

Source: Plan 1 Task 4 verbatim brief code; found in task review (Important, plan-mandated). Deferred by user decision.
