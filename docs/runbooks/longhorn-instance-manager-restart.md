# Runbook: Restarting a Node's Longhorn Instance-Manager

Restarting `instance-manager-<hash>` on a node kills every Longhorn engine and replica process on that node at once.
Every volume whose **engine** runs there loses its block device under a mounted filesystem.
Every volume whose **only healthy replica** is there goes faulted.

On 2026-11-17 a restart of tpi1's instance-manager during market hours left nats-1, Grafana and victoria-traces on stale mounts, and kubelet then tried to format the vmsingle volume when its device read back blank.
Incident record: `chalupa-brain/workstreams/platform/briefs/2026-11-17-tpi1-longhorn-incident.md`.

Do not restart an instance-manager to "unstick" something until this runbook's before-steps are done.

## When

- Weekends, or weekdays before 13:00 or after 21:30 UTC (outside US market hours in both DST states).
- With someone watching for the whole window, and a written plan: what runs, what is checked after each step, how to undo it.

## 1. Inventory: what depends on this node

Set the node once:

```bash
NODE=dpi1
```

Volumes with an engine or a replica on the node, and the pod using each:

```bash
kubectl -n longhorn-system get engines.longhorn.io,replicas.longhorn.io -o json > /tmp/lh.json
kubectl get pv -o json > /tmp/pv.json
python3 - "$NODE" <<'EOF'
import json, sys
node = sys.argv[1]
items = json.load(open("/tmp/lh.json"))["items"]
pv = {p["metadata"]["name"]: (p["spec"].get("claimRef") or {}) for p in json.load(open("/tmp/pv.json"))["items"]}
vols = {}
for o in items:
    v = o["spec"]["volumeName"]
    d = vols.setdefault(v, {"engine": None, "replicas": []})
    if o["kind"] == "Engine":
        d["engine"] = o["spec"].get("nodeID")
    else:
        d["replicas"].append(o["spec"].get("nodeID"))
for v, d in sorted(vols.items()):
    if d["engine"] != node and node not in d["replicas"]:
        continue
    c = pv.get(v, {})
    last = d["replicas"] == [node]
    role = "ENGINE" if d["engine"] == node else "replica"
    print(f"{v}  {c.get('namespace')}/{c.get('name')}  {role}  replicas={d['replicas']}{'  ONLY-REPLICA' if last else ''}")
EOF
```

- `ENGINE`: the workload's filesystem is mounted through this node. It must be moved or stopped before the restart.
- `ONLY-REPLICA`: the volume's only copy is here. Its workload must be stopped; the volume goes faulted otherwise.
- `replica` with copies elsewhere: the volume degrades and rebuilds; nothing to stop.

Find the pod behind a claim with `kubectl -n <ns> get pod -o wide | grep <claim-name>` or `kubectl -n <ns> describe pvc <claim-name>` (`Used By`).

## 2. Move or stop the workloads

**Preferred: drain the node.**
Pods reschedule elsewhere, their volumes detach cleanly and re-attach (engine and all) on the new node:

```bash
kubectl drain "$NODE" --ignore-daemonsets --delete-emptydir-data --timeout=10m
```

Longhorn's `node-drain-policy` is `block-if-contains-last-replica`, so the drain stops while an `ONLY-REPLICA` volume is attached; those need the next path.
Since chalupa-infra#496, vmsingle, Grafana, victoria-logs, victoria-traces and NATS may only run in `site-dpi`; draining a dpi node leaves them two dpi nodes.

**For what cannot move (a single-replica volume such as timescaledb's `longhorn-database` volume): stop it.**
ArgoCD runs every app with `selfHeal: true` and would scale a workload straight back, so pause the application controller for the window:

```bash
kubectl -n argocd scale statefulset argocd-application-controller --replicas=0
```

Then stop the workload:

- Deployment or StatefulSet: `kubectl -n <ns> scale <kind>/<name> --replicas=0`
- CloudNativePG cluster: `kubectl -n schwab-ddowell annotate cluster timescaledb cnpg.io/hibernation=on --overwrite`

Wait until every volume from step 1 marked `ENGINE` or `ONLY-REPLICA` is detached:

```bash
kubectl -n longhorn-system get volumes.longhorn.io -o custom-columns=NAME:.metadata.name,STATE:.status.state,NODE:.status.currentNodeID | grep -v detached
```

None of the step-1 volumes may still show `attached` on `$NODE`.

## 3. Restart

```bash
kubectl -n longhorn-system get pod -l longhorn.io/component=instance-manager,longhorn.io/node="$NODE"
kubectl -n longhorn-system delete pod -l longhorn.io/component=instance-manager,longhorn.io/node="$NODE"
kubectl -n longhorn-system wait --for=condition=Ready pod -l longhorn.io/component=instance-manager,longhorn.io/node="$NODE" --timeout=5m
```

## 4. Bring things back, one at a time

1. Wait until every volume is `healthy` again (replicas on `$NODE` rebuild): `kubectl -n longhorn-system get volumes.longhorn.io`. A rebuild across sites can take tens of minutes.
2. `kubectl uncordon "$NODE"` if it was drained.
3. Start each stopped workload separately (`--replicas=1`, or `cnpg.io/hibernation=off`), and before the next one read its pod's events:

   ```bash
   kubectl -n <ns> get events --field-selector involvedObject.name=<pod> | grep -iE 'FailedMount|input/output|I/O error|format|mkfs'
   ```

   **On any match, scale that workload to 0 at once.**
   Longhorn's CSI plugin formats a device on which `blkid` finds no filesystem, and a device returning I/O errors can look blank; every kubelet retry is another chance to format a volume that holds data.
   Then inspect the volume read-only (maintenance-mode attach, `e2fsck -n` from a debug pod on another node) before anything mounts it again.
4. Resume ArgoCD if it was paused: `kubectl -n argocd scale statefulset argocd-application-controller --replicas=1`.

## After

- Every Longhorn volume `attached`/`healthy` (or `detached` for unused ones), none `faulted` or `degraded`.
- Every workload from step 1 Ready.
- ArgoCD application controller at 1 replica, all apps Synced.
- The node uncordoned.

## The mkfs guard

`k8s/platform/longhorn/templates/mkfs-guard.yaml` puts a wrapper over `mkfs.ext4`/`mkfs.xfs` in the `longhorn-csi-plugin` container: it formats only a volume whose Longhorn `status.actualSize` is 0 and whose device reads back as zeros, and otherwise fails the mount with `mkfs-guard: refusing to format …` in the pod's events.
Step 4.3 still applies: the guard stops the format, not the I/O errors.

Check it is in place:

```bash
kubectl -n longhorn-system get ds longhorn-csi-plugin -o jsonpath='{.spec.template.metadata.annotations.chalupa\.tech/mkfs-guard}{"\n"}'
```

An empty answer means the guard is missing.
Longhorn's driver deployer recreates the DaemonSet without it whenever the Longhorn version or CSI image changes, and the `longhorn-mkfs-guard` CronJob puts it back within 5 minutes.
**After a Longhorn upgrade, before any workload restarts**, put it back at once and check:

```bash
kubectl -n longhorn-system create job --from=cronjob/longhorn-mkfs-guard mkfs-guard-now
kubectl -n longhorn-system logs job/mkfs-guard-now
kubectl -n longhorn-system rollout status ds/longhorn-csi-plugin
```

## Stopping a longhorn-manager log replay without restarting the instance-manager

**Symptom.** One node's longhorn-manager logs `pvc-…-e-0: time=…` (or `pvc-…-r-…: time=…`) lines without pause, and `Getting requested log for <instance>` repeats.
That is a log request: a replica failure set `spec.logRequested` on an engine or replica, and the manager that owns it prints the process's whole log file, which has no rotation, so it holds everything since the process started.
The manager marks the request done (`status.logFetched`) only after the whole pass, and if that status write fails the next reconcile starts the pass again from the top (2026-11-17: 30–60 minute passes, repeated for hours).
Restarting the instance-manager ends it only by killing the process and its log; this procedure does not touch the engine.

1. Name the instance that is replaying and the node printing it.
   Each replayed line starts with the instance's name (`msg="pvc-…-e-0: …`), so count the names in each manager's latest output.
   The `Getting requested log for <instance>` line is printed once per pass, which can be an hour apart, and kubelet rotates the manager's log within minutes during a replay, so do not wait for it.

   ```bash
   for P in $(kubectl -n longhorn-system get pod -l app=longhorn-manager -o name); do
     echo "== $P $(kubectl -n longhorn-system get "$P" -o jsonpath='{.spec.nodeName}')"
     kubectl -n longhorn-system logs "$P" -c longhorn-manager --tail=5000 \
       | grep -oE 'msg="pvc-[0-9a-f-]+-(e-[0-9]+|r-[0-9a-f]+): ' | sort | uniq -c
   done
   ```

   Set `INSTANCE` to the name with thousands of lines and `NODE` to that pod's node, and confirm the flag is set on it (`engines.longhorn.io` for a name ending `-e-0`, `replicas.longhorn.io` for `-r-…`):

   ```bash
   INSTANCE=<name>; NODE=<node>; KIND=engines.longhorn.io   # or replicas.longhorn.io
   kubectl -n longhorn-system get "$KIND" "$INSTANCE" -o jsonpath='{.status.ownerID} {.spec.logRequested} {.status.logFetched}{"\n"}'
   ```

   The owner should be `$NODE` and `logRequested` `true`.
   Other objects may also have `logRequested` set for a failure that is not replaying; leave them, since clearing the flag skips that failure's log for good.

2. Clear the flag on that instance only:

   ```bash
   kubectl -n longhorn-system patch "$KIND" "$INSTANCE" --type=merge -p '{"spec":{"logRequested":false}}'
   ```

   Clear the flag **before** restarting anything: a manager that starts while the flag is still true begins a new pass at once.
   The volume controller sets the flag again only on a new replica failure, so the value stays false.
   With the flag false, the current pass still runs to its end but is not repeated; step 3 cuts it short.

3. Restart the longhorn-manager on `$NODE`, and wait for its replacement to be Ready.
   Stop at the first line that prints `STOP`; the node has no working manager until a replacement is Ready.

   ```bash
   OLD=$(kubectl -n longhorn-system get pod -l app=longhorn-manager --field-selector spec.nodeName="$NODE" -o jsonpath='{.items[0].metadata.name}')
   kubectl -n longhorn-system delete pod "$OLD" --wait=true || echo "STOP: delete failed"
   NEW=""
   for i in $(seq 1 60); do
     NEW=$(kubectl -n longhorn-system get pod -l app=longhorn-manager --field-selector spec.nodeName="$NODE" -o jsonpath='{.items[*].metadata.name}')
     [ -n "$NEW" ] && [ "$NEW" != "$OLD" ] && break; NEW=""; sleep 5
   done
   [ -n "$NEW" ] || echo "STOP: no replacement pod after 5 minutes"
   kubectl -n longhorn-system wait --for=condition=Ready pod "$NEW" --timeout=5m && echo "replaced: $OLD -> $NEW" || echo "STOP: $NEW not Ready"
   ```

   This restarts only the control plane on that node: engines and replicas run in the instance-manager and keep serving I/O.

4. Check that it stopped, twice a minute or more apart: no replayed lines for the instance in the replacement's output, and the flag still false.

   ```bash
   kubectl -n longhorn-system logs "$NEW" -c longhorn-manager --tail=5000 > /tmp/lh-new.log && \
     echo "replayed lines: $(grep -c "msg=\"$INSTANCE: " /tmp/lh-new.log)  requests: $(grep -c "Getting requested log for $INSTANCE" /tmp/lh-new.log)"
   kubectl -n longhorn-system get "$KIND" "$INSTANCE" -o jsonpath='{.spec.logRequested}{"\n"}'
   ```

   Both counts 0 and `false` on both reads: stopped.
   If the `logs` command fails, nothing is printed; that is not 0, so read again.
   A request set later by a new replica failure starts a new, separate pass.

**Tested** in session 276 (2026-11-26) on a throwaway 2-replica volume (engine on dpi1, replicas on dpi1 and dpi2) with a pod writing to it every second:

| Step | Result |
|---|---|
| Replica set to `ERR` from the engine (`longhorn update-replica --mode ERR`), twice | Replica marked failed, `logRequested` set on the **replica** only; dpi2's manager printed the replica's log once (11 lines); the engine's flag was never set (its update from the same reconcile did not land). Rebuilt in about 10s; no write failed |
| Engine `logRequested` patched to `true` by hand | Accepted; dpi1's manager printed the whole engine log (57 lines, from process start) within a second, `logFetched` persisted, the volume controller cleared the flag; no second pass |
| Flag false, dpi1 manager restarted | New manager printed nothing; all 9 engines in the cluster kept their state, port and instance-manager; all volumes stayed healthy; the writer saw no failed write |
| Manager on dpi1 deleted, flag set to `true` while it was down | No other node's manager took the instance over during the 34s it was down; the new dpi1 manager printed the whole engine log on startup, then cleared the flag |

**Not tested:** a real loop.
The throwaway volume's logs were seconds old, so each pass took under a second and its `logFetched` write landed; the repeat and the long pass that step 3 cuts short were not reproduced.
That the manager restart ends an in-flight pass rests on the pass running inside the manager process (`printInstanceLogs`, longhorn-manager v1.7.2 `controller/instance_handler.go`), not on observation.

## Not covered

- Clearing one stale errored process without restarting the whole instance-manager (open question, platform phase-97 priority 2).
