---
# fb2cng-web-cwc1
title: Dead GET /defaults route leftover from old SPA
status: completed
type: bug
priority: normal
created_at: 2026-08-02T14:50:21Z
updated_at: 2026-08-02T15:08:17Z
parent: fb2cng-web-s726
---

server.go GET /defaults -> handlers.go handleDefaults: no template or JS references it after the server-rendered UI replaced the old SPA (old app.js used it to fetch default config JSON). Harmless (behind auth) but dead code. Verify no consumer, then remove route+handler (handleDefaults) or confirm intentional. Source: final whole-branch review (Minor #4).
