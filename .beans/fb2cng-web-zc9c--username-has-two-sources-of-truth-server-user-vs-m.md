---
# fb2cng-web-zc9c
title: Username has two sources of truth (server .User vs /me JS)
status: completed
type: bug
priority: normal
created_at: 2026-08-02T14:50:22Z
updated_at: 2026-08-02T15:19:19Z
parent: fb2cng-web-s726
---

base.gohtml #user is filled server-side from .User AND overwritten client-side by app.js fetch('/me'). Server-side .User is inconsistent across pages: handleIndex uses Remote-Name, handlePresetEditor uses Remote-User, handleSettings sets none. The /me JS fetch masks it so it's not user-visible. Pick one source. Source: final whole-branch review (Minor #3).
