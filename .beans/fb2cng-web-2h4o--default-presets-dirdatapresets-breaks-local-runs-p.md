---
# fb2cng-web-2h4o
title: Default PRESETS_DIR=/data/presets breaks local runs (preset writes 422 mkdir permission denied)
status: completed
type: bug
priority: normal
created_at: 2026-08-02T15:26:54Z
updated_at: 2026-08-02T15:30:33Z
parent: fb2cng-web-s726
---

Running ./fb2cng-web without env: PRESETS_DIR defaults to /data/presets (container path). /data is not writable/creatable by a normal user, so every preset write (Create/Duplicate/Save) fails lazily at write time with a raw 422 'mkdir /data: permission denied'. Repro: POST /settings/preset/defaults/duplicate -> 422. Fix: default PRESETS_DIR to a writable per-user path (os.UserConfigDir()/fb2cng/presets, fallback TempDir); MkdirAll it at startup in main.go (like JobsDir) and Fatalf with an actionable message on failure. Container keeps /data/presets via Dockerfile ENV. Found by running the app.
