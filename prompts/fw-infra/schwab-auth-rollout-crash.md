# Investigate go-schwab-auth Rollout Crash

## Problem

The go-schwab-auth Argo Rollout canary pod is in CrashLoopBackOff. The old ReplicaSet pod (`go-schwab-auth-7f57557c4f`) is healthy and running v0.4.0, but the new ReplicaSet pod (`go-schwab-auth-df4b5f7bb`) crashes immediately.

## Root Cause (identified)

The crash is NOT an OOM — it's an OCI runtime error:

```
exec: "/bin/sh": stat /bin/sh: no such file or directory
```

The container image (`gitea.tailbecff0.ts.net/chalupa-tech/go-schwab-auth:v0.4.0`) is a scratch/distroless Go binary that has no shell. The new Rollout pod spec appears to inject a command that references `/bin/sh`, which doesn't exist in the image.

## Investigation Steps

1. Compare the working Deployment pod spec vs the failing Rollout pod spec:
   - `kubectl get pod go-schwab-auth-7f57557c4f-vcl2r -n schwab-ddowell -o yaml` (working)
   - `kubectl get pod go-schwab-auth-df4b5f7bb-jq724 -n schwab-ddowell -o yaml` (crashing)
   - Look for differences in `command`, `args`, or injected containers

2. Check the Rollout manifest at `k8s/apps/schwab/go-schwab-auth/templates/rollout.yaml` — compare with the old `deployment.yaml` (if it still exists in git history)

3. Check if the AnalysisTemplate at `k8s/apps/schwab/go-schwab-auth/templates/analysistemplate.yaml` is injecting anything into the pod

4. Check if other Schwab apps (go-notify, go-schwab-feed) have the same issue — they were also converted from Deployments to Rollouts recently

5. VMAgent alert `go-schwab-auth` scrape target is down — this will resolve once the rollout is fixed

## Context

- The Deployment → Rollout conversion happened in recent PRs (#294, #298)
- go-schwab-auth, go-notify, and go-schwab-feed were all converted
- The old Deployment pod is still running and healthy — no user impact yet
- The Rollout is in `Progressing` phase
