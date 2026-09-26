---
# fb2cng-web-3181
title: 'Apply Renovate precedent and fold in pending Renovate updates (#24, #26, #28)'
status: completed
type: task
priority: normal
created_at: 2026-09-26T10:40:57Z
updated_at: 2026-09-26T11:06:19Z
---

Port renovate.json verbatim from calit/slidev-polls (precedent #renovate-in-every-project-non-major-updates-grou-1789065174).

## Summary of Changes
- renovate.json: vulnerabilityAlerts, docker.pinDigests, one grouped PR for minor/patch/digest/pin/bump

- Folded in #24 (alpaquita-linux-go 1.26.7), #26 (go-oidc v3.21.0), #28 (oauth2 v0.37.0); go test ./... passes

- Follow-up: no digest pins for docker-compose.example.yml (user-facing example); Dockerfile S8431 findings accepted in Sonar
