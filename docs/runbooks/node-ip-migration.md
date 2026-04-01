# Runbook: K3s Node IP Migration

Migrate a K3s node's `node-ip` from one address to another (e.g., Tailscale → LAN).

## Prerequisites

- SSH access to the target node
- `kubectl` access to the cluster
- Ansible vault password (for automated path)
- For server nodes: access to etcd client certs on a healthy server

## Agent Nodes (Safe — No etcd)

Agent nodes only run kubelet and kube-proxy. Changing `node-ip` requires a config
update and service restart. No coordination needed.

### Steps

1. Set `k3s_node_ip` in the node's host_vars:
   ```yaml
   # ansible/inventory/host_vars/<node>/k3s.yml
   k3s_node_ip: "192.168.x.x"
   ```

2. Apply with the rolling restart playbook:
   ```bash
   ansible-playbook playbooks/k3s-rolling-restart.yml \
     --limit <node> \
     -e k3s_restart_drain=false \
     -e k3s_restart_reboot=false
   ```

3. Verify:
   ```bash
   kubectl get node <node> -o jsonpath='{.status.addresses}'
   ```
   The cloud controller may take 10-20 seconds to update the node object.

## Server Nodes (Critical — etcd Peer URL Must Be Updated First)

Server nodes run etcd. When `node-ip` changes, K3s regenerates the etcd config
with new `listen-peer-urls` and `advertise-client-urls`. **If the etcd cluster's
member list still has the old peer URL, the node cannot rejoin — Raft messages
overflow and the node gets stuck in NotReady.**

### Steps

1. **Take an etcd snapshot** (safety net):
   ```bash
   ssh ddowell@<healthy-server> "sudo k3s etcd-snapshot save --name pre-migration-<node>"
   ```

2. **Get the etcd member ID** for the target node:
   ```bash
   ssh ddowell@<healthy-server> "sudo curl -sk \
     --cacert /var/lib/rancher/k3s/server/tls/etcd/server-ca.crt \
     --cert /var/lib/rancher/k3s/server/tls/etcd/client.crt \
     --key /var/lib/rancher/k3s/server/tls/etcd/client.key \
     -X POST 'https://127.0.0.1:2382/v3/cluster/member/list' \
     -H 'Content-Type: application/json' -d '{}'"
   ```

3. **Update the etcd peer URL** to the new IP **before restarting**:
   ```bash
   ssh ddowell@<healthy-server> "sudo curl -sk \
     --cacert /var/lib/rancher/k3s/server/tls/etcd/server-ca.crt \
     --cert /var/lib/rancher/k3s/server/tls/etcd/client.crt \
     --key /var/lib/rancher/k3s/server/tls/etcd/client.key \
     -X POST 'https://127.0.0.1:2382/v3/cluster/member/update' \
     -H 'Content-Type: application/json' \
     -d '{\"ID\":\"<MEMBER_ID>\",\"peerURLs\":[\"https://<NEW_IP>:2380\"]}'"
   ```

4. **Set `k3s_node_ip`** in host_vars and apply:
   ```bash
   ansible-playbook playbooks/k3s-rolling-restart.yml \
     --limit <node> \
     -e k3s_restart_drain=false \
     -e k3s_restart_reboot=false
   ```

5. **Verify etcd health and node status**:
   ```bash
   kubectl get --raw=/healthz/etcd
   kubectl get node <node> -o wide
   ```

### Server Migration Order

Always migrate **one server at a time** to maintain etcd quorum:

1. Migrate the least-critical HA server first
2. Verify etcd health after each server
3. Migrate the primary server last
4. Never migrate two servers simultaneously

### Rollback

If a server gets stuck after migration:

1. SSH to the node and revert `/etc/rancher/k3s/config.yaml` to the old `node-ip`
2. Restart K3s: `sudo systemctl restart k3s`
3. Update the etcd member peer URL back to the old IP (step 3 above, reversed)

## What NOT to Do

- **Do not change `node-ip` on a server without updating the etcd peer URL first.**
  This causes Raft buffer overflow and the node cannot rejoin the cluster.
- **Do not migrate all servers simultaneously.** If any fail to restart, you lose
  etcd quorum with no recovery path except snapshot restore.
- **Do not remove Tailscale before migrating `node-ip`.** K3s agent load balancer
  and server URL may still reference Tailscale FQDNs.
