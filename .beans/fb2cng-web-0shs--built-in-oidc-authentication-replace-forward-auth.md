---
# fb2cng-web-0shs
title: Built-in OIDC authentication (replace forward-auth)
status: in-progress
type: epic
created_at: 2026-08-03T08:12:15Z
updated_at: 2026-08-03T08:12:15Z
---

App becomes an OIDC Relying Party; runs the auth-code flow itself, gates on a group claim, own signed session cookie. Replaces reverse-proxy forward-auth entirely. Optional (AUTH_MODE=off|oidc). Spec: docs/superpowers/specs/2026-08-03-oidc-auth-design.md
