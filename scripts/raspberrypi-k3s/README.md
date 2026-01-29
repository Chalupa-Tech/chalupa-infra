# Raspberry Pi K3s with Tailscale

This directory contains scripts to deploy a **High Availability (HA) K3s cluster** on Raspberry Pis, utilizing **Tailscale** for secure, distributed networking.

This setup allows your nodes to be on different physical networks (e.g., home, office, cloud) while behaving as a single local cluster.

## Prerequisites

1.  **Raspberry Pi OS** (64-bit recommended) installed on all nodes.
2.  **Tailscale Account**: You need a valid [Tailscale Auth Key](https://login.tailscale.com/admin/settings/keys).
    *   *Recommended:* Create a **Reusable** and **Ephemeral** key for ease of setup.
    *   *Required:* Ensure your Tailscale ACLs allow the K3s pod CIDR (default `10.42.0.0/16`) if running strict ACLs.
3.  **Root Access**: The script must be run with `sudo`.

## Files

*   `install.sh`: The main automated installer.

## Usage

### 1. Prepare Environment Variables
On **every node**, you need to export the following variables before running the script.

```bash
# Required: Your Tailscale Auth Key
export TS_AUTHKEY="tskey-auth-..."

# Required for HA/Cluster Joining: A shared secret for your cluster.
# Pick a strong password. It must be the same on ALL nodes.
export K3S_TOKEN="my-secure-cluster-token"
```

### 2. First Node (Cluster Initialization)
Run this on the **first** Raspberry Pi. This initializes the cluster.

```bash
# Run as root. 'server' mode without an IP initializes the cluster.
sudo -E bash install.sh server
```
*The `-E` flag preserves your environment variables (`TS_AUTHKEY`, `K3S_TOKEN`) for the sudo session.*

Once complete, this node will display its **Tailscale IP**. Note this IP.

### 3. Additional Server Nodes (HA Control Plane)
To add more masters (control plane nodes) for High Availability (minimum 3 total recommended for etcd):

```bash
# Replace <FIRST-NODE-IP> with the Tailscale IP of the first node
sudo -E bash install.sh server <FIRST-NODE-IP>
```

### 4. Agent Nodes (Workers)
To add worker nodes that only run workloads:

```bash
# Replace <FIRST-NODE-IP> with the Tailscale IP of any server node
sudo -E bash install.sh agent <FIRST-NODE-IP>
```

## What the Script Does

1.  **System Prep (Raspberry Pi specific):**
    *   Checks `/boot/cmdline.txt` (or `/boot/firmware/cmdline.txt`) for `cgroup_enable=memory cgroup_memory=1`.
    *   **Auto-fix:** If missing, it appends them and prompts for a **reboot**.
    *   Checks for legacy `iptables` support (info only).
2.  **Tailscale Setup:**
    *   Installs Tailscale if missing.
    *   Authenticates using `$TS_AUTHKEY`.
    *   Retrieves the node's Tailscale IP.
3.  **K3s Installation:**
    *   Installs K3s using the official script.
    *   Configures `--vpn-auth` to use Tailscale for node authentication.
    *   Sets `--node-external-ip` to the Tailscale IP.
    *   Sets `--flannel-iface=tailscale0` so pod traffic goes over VPN.

## Troubleshooting

*   **"Missing cgroup_enable=memory"**: The script will tell you if this happens. Reboot the Pi (`sudo reboot`) and run the script again.
*   **"Job for k3s.service failed"**: Check logs with `journalctl -u k3s -f`.
    *   Common issue: `cgroups` not active (did you reboot?).
    *   Common issue: `duplicate node name` if you re-imaged a Pi without clearing it from Tailscale/K3s.
*   **Networking Issues**: Ensure `10.42.0.0/16` (Pod CIDR) and `10.43.0.0/16` (Service CIDR) are not blocked by your Tailscale ACLs.

## Verification

On any server node, check the nodes:
```bash
sudo kubectl get nodes -o wide
```
You should see all nodes using their **Tailscale IPs** as their InternalIP.
