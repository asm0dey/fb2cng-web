---
# fb2cng-web-11t8
title: 'Dockerfile: digest-only FROM lines tracking latest (Sonar docker:S8431)'
status: completed
type: task
created_at: 2026-09-26T11:15:27Z
updated_at: 2026-09-26T11:15:27Z
---

Sonar S8431 flags tag@digest FROM lines. Switched to digest-only; Renovate tracks those as :latest, which the author accepted (more secure, tests gate it). latest == previously pinned stream-musl / 1.26.7-musl / musl digests at switch time.

## Summary of Changes
- Dockerfile FROM lines: image@sha256 only; # syntax line keeps tag@digest (must stay on line 1, not flagged)
- Verified: go test ./... passes; docker build OK; container serves HTTP 200
