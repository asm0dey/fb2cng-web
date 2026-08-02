---
# fb2cng-web-km22
title: Regenerate options.json against a real fbc dump
status: completed
type: task
priority: normal
created_at: 2026-08-02T14:50:21Z
updated_at: 2026-08-02T15:00:49Z
parent: fb2cng-web-s726
---

internal/schema/options.json currently holds the 7-option fixture scaffold (real fbc unavailable in this env). The editor grid only exposes those 7 until someone runs 'FBC_BIN=<real fbc> go generate ./...' to regenerate from a real 'fbc dumpconfig --default' (~104 options). Editor machinery is complete/correct; this is a data/ops step to fully deliver Plan 3's grid. Source: final whole-branch review (Minor #1).
