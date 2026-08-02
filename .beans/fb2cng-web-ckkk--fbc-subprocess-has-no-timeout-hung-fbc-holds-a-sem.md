---
# fb2cng-web-ckkk
title: fbc subprocess has no timeout (hung fbc holds a sem slot forever)
status: completed
type: bug
priority: normal
created_at: 2026-08-02T14:50:21Z
updated_at: 2026-08-02T15:11:16Z
parent: fb2cng-web-s726
---

internal/server/convert_flow.go worker + internal/convert/runner.go use context.Background() for the fbc exec. A pathological input that hangs fbc permanently holds a Server.sem slot; enough of them stall the whole convert feature. Pre-existing pattern, low risk for a self-hosted tool. Fix: context.WithTimeout around Convert/ConvertLogged/Validate. Source: final whole-branch review (Minor #5).
