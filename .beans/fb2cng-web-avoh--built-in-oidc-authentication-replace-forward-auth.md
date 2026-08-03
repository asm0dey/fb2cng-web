---
# fb2cng-web-avoh
title: Built-in OIDC authentication (replace forward-auth)
status: in-progress
type: epic
priority: normal
created_at: 2026-08-03T08:12:21Z
updated_at: 2026-08-03T08:33:55Z
---

App becomes an OIDC Relying Party; runs the auth-code flow itself, gates on a group claim, own signed session cookie. Replaces reverse-proxy forward-auth entirely. Optional (AUTH_MODE=off|oidc). Spec: docs/superpowers/specs/2026-08-03-oidc-auth-design.md

## Docs
- Spec: docs/superpowers/specs/2026-08-03-oidc-auth-design.md
- Plan: docs/superpowers/plans/2026-08-03-oidc-auth.md
- Authelia guide: docs/oidc-authelia.md

## Tasks (mirror plan)
- [ ] fb2cng-web-k853 — T1 Config
- [ ] fb2cng-web-mmov — T2 Session cookie
- [ ] fb2cng-web-x2qg — T3 OIDC verify + group gate
- [ ] fb2cng-web-17f6 — T4 HTTP handlers + middleware
- [ ] fb2cng-web-akte — T5 Wire server + main; delete forward-auth
- [ ] fb2cng-web-7im2 — T6 Docs
