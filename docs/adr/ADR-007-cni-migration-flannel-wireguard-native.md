# ADR-007: CNI Migration — VXLAN-over-Tailscale to Flannel WireGuard-Native

**Status:** Accepted (in progress — wg0 workaround pending removal)
**Date:** 2026-03-28 (migration started), 2026-03-29 (pod networking fixed)
**Deciders:** ddowell

---

## Context

### The Original Architecture (pre-March 2026)

K3s was configured with `--flannel-iface=tailscale0` and the default VXLAN backend. All pod-to-pod traffic — including same-site — was double-encapsulated through Tailscale:

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  BEFORE: VXLAN-over-Tailscale (all traffic, including same-site)            │
│                                                                              │
│  ┌─────┐    ┌──────┐    ┌──────────────┐    ┌──────────────┐    ┌──────┐   │
│  │ Pod │───►│ cni0 │───►│ flannel.1    │───►│ tailscale0   │───►│ eth0 │   │
│  │     │    │~1170 │    │ VXLAN +50B   │    │ WG +60B      │    │ 1500 │   │
│  └─────┘    └──────┘    │ MTU 1230     │    │ MTU 1280     │    └──────┘   │
│                         └──────────────┘    └──────────────┘               │
│                                                                              │
│  Effective pod MTU: ~1170 bytes                                             │
│  Encapsulation layers: 2 (VXLAN + WireGuard)                               │
│  Same-site latency: high (routed through Tailscale even on same LAN)       │
│  External dependency: Tailscale control plane                               │
└──────────────────────────────────────────────────────────────────────────────┘
```

Every packet was VXLAN-encapsulated and then WireGuard-encrypted through Tailscale. This double encapsulation on a 1280 MTU tunnel produced an effective pod MTU of ~1170 bytes.

### Symptoms

The low MTU and double encapsulation caused cascading failures:

- **Longhorn CSI timeouts** — `context deadline exceeded` reaching `longhorn-backend:9500`. Replica sync between same-site nodes was slow enough to trigger attachment failures and multi-attach deadlocks.
- **ArgoCD repo-server unreachable** — controller couldn't reach repo-server pods cross-node, causing Unknown sync states.
- **Grafana plugin install timeouts** — outbound HTTPS from pods failed on large payloads.
- **DNS resolution failures** — CoreDNS pods inherited Tailscale MagicDNS (`100.100.100.100`) or OCI link-local (`169.254.169.254`) as upstream, neither reachable from pod network namespace.

### Why Tailscale Was the Wrong Layer for Pod Traffic

1. **MTU tax**: Tailscale's WireGuard tunnel reduces MTU from 1500 to 1280. Adding VXLAN on top drops it further to ~1170.
2. **External dependency**: All pod traffic depended on Tailscale's control plane. A Tailscale outage would kill the cluster.
3. **Routing conflicts**: Tailscale's routing table 52 (priority 5270) can override kernel routes, which later caused the pod CIDR hijacking incident (see Phase 7e below).
4. **Unnecessary for same-site**: Nodes on the same LAN don't need VPN encryption — they're on a private network.

---

## Decision

Replace the VXLAN-over-Tailscale pod data plane with **flannel wireguard-native** backend on physical interfaces. Tailscale remains for management access (SSH, kubectl, web UIs) but is removed from the pod traffic path.

### Target Architecture (endgame)

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  ENDGAME: Flannel WireGuard-Native (direct, single encapsulation)           │
│                                                                              │
│  Same-site (LAN):                                                           │
│  ┌─────┐    ┌──────┐    ┌──────────────┐    ┌──────┐                       │
│  │ Pod │───►│ cni0 │───►│ flannel-wg   │───►│ eth0 │───► LAN (192.168.x.x)│
│  │     │    │ 1230 │    │ WG +60B      │    │ 1500 │                       │
│  └─────┘    └──────┘    │ MTU 1420     │    └──────┘                       │
│                         └──────────────┘                                    │
│                                                                              │
│  Cross-site (WAN, via port forward):                                        │
│  ┌─────┐    ┌──────┐    ┌──────────────┐    ┌──────┐                       │
│  │ Pod │───►│ cni0 │───►│ flannel-wg   │───►│ eth0 │───► WAN (port 51820) │
│  │     │    │ 1230 │    │ WG +60B      │    │ 1500 │                       │
│  └─────┘    └──────┘    │ MTU 1420     │    └──────┘                       │
│                         └──────────────┘                                    │
│                                                                              │
│  Effective pod MTU: 1230 bytes                                              │
│  Encapsulation layers: 1 (WireGuard only)                                  │
│  Same-site latency: ~0.5ms (direct LAN)                                    │
│  External dependency: NONE                                                  │
└──────────────────────────────────────────────────────────────────────────────┘
```

Single WireGuard encapsulation. No VXLAN. No Tailscale in the data path.

### Interim Architecture (current — wg0 workaround)

Cross-site flannel-wg can't reach remote nodes directly because the tayven router doesn't yet forward UDP 51820. A temporary wg0 site-to-site tunnel bridges the gap:

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  CURRENT: Flannel WireGuard-Native + wg0 site-to-site tunnel               │
│                                                                              │
│  Same-site (single WG, direct LAN):                                        │
│  ┌─────┐    ┌──────┐    ┌──────────────┐    ┌──────┐                       │
│  │ Pod │───►│ cni0 │───►│ flannel-wg   │───►│ eth0 │───► LAN              │
│  │     │    │ 1230 │    │ :51820       │    │ 1500 │    192.168.x.x       │
│  └─────┘    └──────┘    └──────────────┘    └──────┘                       │
│                                                                              │
│  Cross-site (double WG, via wg0 tunnel — TEMPORARY):                       │
│  ┌─────┐    ┌──────┐    ┌──────────────┐    ┌─────┐    ┌──────┐           │
│  │ Pod │───►│ cni0 │───►│ flannel-wg   │───►│ wg0 │───►│ eth0 │───► WAN  │
│  │     │    │ 1230 │    │ :51820       │    │:51821│    │ 1500 │           │
│  └─────┘    └──────┘    └──────────────┘    └─────┘    └──────┘           │
│                          WG layer 1          WG layer 2                     │
│                                                                              │
│  Effective pod MTU: 1230 bytes (both paths)                                 │
│  Same-site encapsulation: 1 layer   | Cross-site: 2 layers (temporary)     │
│  Same-site latency: ~0.5ms          | Cross-site: ~15-27ms                 │
│  External dependency: NONE                                                  │
└──────────────────────────────────────────────────────────────────────────────┘
```

This is still double encapsulation cross-site, but it's better than before because:

1. **Same-site traffic is single encapsulation over direct LAN** — this is where Longhorn does most of its work (replica sync). Latency dropped to ~0.5ms.
2. **Higher effective MTU** — even cross-site: 1230 + 60 (flannel-wg) + 60 (wg0) = 1350, fits in eth0's 1500. Previously ~1170.
3. **No VXLAN overhead** — pure WireGuard is ~20 bytes smaller per packet than VXLAN.
4. **No Tailscale control plane dependency** — pod traffic works even if Tailscale is down.

### Before vs After Comparison

```
┌─────────────────────┬──────────────────────┬──────────────────────┬────────────────────┐
│                     │ BEFORE               │ CURRENT (interim)    │ ENDGAME            │
│                     │ VXLAN-over-Tailscale  │ flannel-wg + wg0    │ flannel-wg direct  │
├─────────────────────┼──────────────────────┼──────────────────────┼────────────────────┤
│ Same-site encap     │ 2 (VXLAN + TS WG)    │ 1 (flannel-wg)       │ 1 (flannel-wg)     │
│ Cross-site encap    │ 2 (VXLAN + TS WG)    │ 2 (flannel-wg + wg0) │ 1 (flannel-wg)     │
│ Effective pod MTU   │ ~1170                │ 1230                 │ 1230               │
│ Same-site latency   │ high (via Tailscale) │ ~0.5ms (direct LAN)  │ ~0.5ms             │
│ Cross-site latency  │ variable (TS relay?) │ ~15-27ms (via wg0)   │ ~15-27ms           │
│ External dependency │ Tailscale ctrl plane │ NONE                 │ NONE               │
│ Longhorn stable?    │ NO (timeouts)        │ YES                  │ YES                │
│ DNS path            │ Tailscale MagicDNS   │ Cloudflare/Google    │ Cloudflare/Google  │
│ Port forward needed │ No                   │ dpi2:51821 only      │ Both routers:51820 │
└─────────────────────┴──────────────────────┴──────────────────────┴────────────────────┘
```

---

## Implementation History

### Phase 7a–7b: Migration (2026-03-28)

- Changed K3s config from `--flannel-iface=tailscale0` (VXLAN) to `--flannel-backend=wireguard-native`
- Created `k3s_flannel_migrate` Ansible role for rolling migration (cordon → drain → stop → reconfigure → start → verify → uncordon)
- Moved DNS upstream from Tailscale MagicDNS / OCI link-local to Cloudflare (1.1.1.1) + Google (8.8.8.8) via `/etc/rancher/k3s/resolv.conf`

### Phase 7c: Post-Migration Recovery (2026-03-29)

- Fixed CoreDNS external resolution — pods inherited unusable upstream nameservers
- Fixed conntrack stale entries — DNAT entries persisted after CoreDNS pod IP changes, routing ClusterIP traffic to dead IPs
- Created DNS recovery script (`scripts/dns-recovery.sh`)
- Recovered fw1 (Tailscale MagicDNS broken due to resolv.conf not symlinked to systemd-resolved)

### Phase 7d: Cross-Site Routing (2026-03-29)

- **Discovered**: flannel-wg uses `flannel.alpha.coreos.com/public-ip` annotation (LAN IPs) for WireGuard endpoints, not K8s InternalIP (Tailscale IPs). Same-site works over LAN. Cross-site fails because LAN IPs are unreachable across NATs.
- **Solution**: Deployed wg0 site-to-site WireGuard tunnel (dpi2 ↔ tpi1, port 51821) as a temporary bridge. Cross-site flannel-wg peers use wg0 transit IPs (10.100.0.x) as endpoints.
- Fixed OpenBao `api_addr` from POD_IP to stable DNS (HOSTNAME.openbao-internal)
- Recovered OpenBao Raft quorum (openbao-2 as single-node leader via peers.json recovery)

### Phase 7e: Pod-to-Pod Networking Fix (2026-03-29)

- **Root cause**: Tailscale was advertising pod CIDRs (10.42.x.0/24) as subnet routes in routing table 52. Table 52 has priority 5270, checked before the main table (32766) where flannel-wg's /16 route lives. Pod traffic destined for hijacked subnets went through tailscale0 instead of flannel-wg.
- **Contributing factor**: `rp_filter=2` (strict) on flannel-wg and cni0 interfaces. Return packets arriving on flannel-wg were dropped because the kernel's reverse path check said they should arrive via tailscale0.
- **Fix**: Cleared `--advertise-routes` on all 7 nodes. Set `rp_filter=0` on flannel-wg/cni0. Made both persistent in Ansible (k3s_config role).
- **Result**: All pod-to-pod paths working. OpenBao 3-node Raft cluster fully recovered.

---

## Current Network State (2026-03-29)

### Physical Topology

```
                              WAN (Internet)
        ┌──────────────────────────┼──────────────────────────┐
        │                          │                          │
   Dustin Router              Tayven Router            Oracle Cloud
   161.153.114.165            (public IP)              (public IP)
   UDP 51821 → dpi2           (needs 51820 fwd)       UDP 51820 open
        │                          │                          │
════════╪══════════════════════════╪══════════════════════════╪════════
        │                          │                          │
   Site Dustin                Site Tayven               OCI VCN
   192.168.2.0/24             192.168.1.0/24            10.0.0.0/24
        │                          │                          │
┌───────┴───────────────┐  ┌───────┴───────────────┐  ┌──────┴───────┐
│                       │  │                       │  │              │
│ dpi2  .24  [C,E,GW]  │  │ tpi1  .125 [W,GW]    │  │ oracle1 [C,E]│
│ pod: 10.42.6.0/24    │  │ pod: 10.42.1.0/24    │  │ pod: 10.42.4 │
│ wg0: 10.100.0.1      │  │ wg0: 10.100.0.2      │  │ wg0: 10.100. │
│       ▲               │  │       ▲               │  │     0.3      │
│       └───────────────┼──┼───────┘               │  │              │
│        wg0 tunnel     │  │  (port 51821)         │  └──────────────┘
│        (temporary)    │  │                       │
│                       │  │ tpi3  .180 [W]        │
│ dpi1  .6   [W]       │  │ pod: 10.42.3.0/24    │
│ pod: 10.42.5.0/24    │  │                       │
│                       │  │ tpi2  .211 [C,E] DOWN│
│ dpi3  .250 [C,E]     │  │ pod: 10.42.0.0/24    │
│ pod: 10.42.7.0/24    │  └───────────────────────┘
│                       │
│ fw1   .51  [W,VM]    │  Legend:
│ pod: 10.42.2.0/24    │  C=control-plane  E=etcd  W=worker
└───────────────────────┘  GW=WireGuard gateway
```

### Traffic Flow Diagram

```
Same-site pod-to-pod (e.g., dpi1 pod → dpi3 pod):

  dpi1                                              dpi3
  ┌──────────────────────┐                          ┌──────────────────────┐
  │ Pod (10.42.5.x)      │                          │ Pod (10.42.7.x)      │
  │   │                  │                          │   ▲                  │
  │   ▼                  │                          │   │                  │
  │ cni0 (MTU 1230)      │                          │ cni0 (MTU 1230)      │
  │   │                  │                          │   ▲                  │
  │   ▼                  │                          │   │                  │
  │ flannel-wg (:51820) ─┼── LAN 192.168.2.0/24 ──►┼─ flannel-wg (:51820)│
  │ 1 WG encap layer    │    ~0.5ms, MTU 1500      │                      │
  └──────────────────────┘                          └──────────────────────┘


Cross-site pod-to-pod (e.g., dpi1 pod → tpi3 pod, current interim):

  dpi1                    dpi2 (gateway)            tpi1 (gateway)          tpi3
  ┌──────────┐           ┌──────────────┐          ┌──────────────┐        ┌──────────┐
  │ Pod      │           │              │          │              │        │ Pod      │
  │  │       │           │              │          │              │        │  ▲       │
  │  ▼       │           │              │          │              │        │  │       │
  │ cni0     │           │              │          │              │        │ cni0     │
  │  │       │           │              │          │              │        │  ▲       │
  │  ▼       │           │  ▼           │          │         ▲    │        │  │       │
  │ flannel- ├──►LAN────►├─ flannel-wg  │          │ flannel-wg──├───LAN─►├─ flannel-│
  │ wg       │           │  │           │          │         ▲    │        │ wg       │
  └──────────┘           │  ▼           │          │         │    │        └──────────┘
                         │  wg0 ────────┼──►WAN───►┼── wg0   │    │
                         │  :51821      │  ~15ms   │  :51821 │    │
                         └──────────────┘          └──────────────┘
                          2 WG layers               2 WG layers
                          (temporary)               (temporary)
```

### flannel-wg Peer Endpoints (from dpi2)

| Node | Pod CIDR | flannel-wg Endpoint | Path |
|------|----------|-------------------|------|
| dpi1 | 10.42.5.0/24 | 192.168.2.6:51820 | LAN direct |
| dpi3 | 10.42.7.0/24 | 192.168.2.250:51820 | LAN direct |
| fw1 | 10.42.2.0/24 | 192.168.2.51:51820 | LAN direct |
| tpi1 | 10.42.1.0/24 | 10.100.0.2:51820 | via wg0 tunnel |
| tpi3 | 10.42.3.0/24 | 192.168.1.180:51820 | via wg0 tunnel (routed through tpi1) |
| oracle1 | 10.42.4.0/24 | 10.100.0.3:51820 | via wg0 tunnel |
| tpi2 | 10.42.0.0/24 | 192.168.1.211:51820 | DOWN |

### MTU Chain

| Interface | MTU | Purpose |
|-----------|-----|---------|
| eth0 | 1500 | Physical NIC |
| flannel-wg | 1420 | Pod WireGuard tunnel (K3s managed) |
| wg0 | 1420 | Site-to-site tunnel (Ansible managed, temporary) |
| tailscale0 | 1280 | Management VPN (NOT in pod data path) |
| cni0 | 1230 | Pod bridge (K3s managed, inherits from flannel-wg minus overhead) |

### Effective MTU per Path

| Path | Encapsulation | Effective MTU | Fits in |
|------|--------------|---------------|---------|
| Same-site pod-to-pod | cni0 (1230) + flannel-wg (~60) = 1290 | 1230 | eth0 (1500) ✓ |
| Cross-site pod-to-pod | cni0 (1230) + flannel-wg (~60) + wg0 (~60) = 1350 | 1230 | eth0 (1500) ✓ |
| Old VXLAN-over-Tailscale | cni0 (~1170) + VXLAN (~50) + Tailscale WG (~60) = 1280 | ~1170 | tailscale0 (1280) ✓ but very tight |

---

## Port Allocation & Why We Need wg0 At All

A recurring question: if flannel-wg is already a WireGuard overlay, why do we need a second WireGuard tunnel (wg0) to bridge sites?

### Port allocation

- **UDP 51820** — flannel-wg, bound on every K3s node. Default WireGuard port, K3s-managed.
- **UDP 51821** — wg0 site-to-site tunnel, bound on gateway nodes (dpi2, tpi1). Non-default port chosen specifically because flannel-wg already owns 51820 on the same host — two WireGuard services cannot share a UDP port on the same host.
- **UDP 41641** — Tailscale (default). Not in the pod data path. No port conflict with flannel-wg.

### Why wg0 exists (and why it's temporary)

flannel-wg *can* do cross-site directly — it's a full WireGuard mesh. The blocker is NAT: flannel-wg peers on different sites need to reach each other at a routable endpoint. Same-site peers use LAN IPs (automatic). Cross-site peers need `<router-public-ip>:51820` reachable, which requires a UDP 51820 port forward at each site's router to the local flannel-wg gateway node.

**Those port forwards don't exist yet.** wg0 is the workaround: one site-to-site tunnel on 51821 (already forwarded at Dustin's router), through which cross-site flannel-wg peers tunnel using wg0 transit IPs (10.100.0.x) as their negotiated endpoints. This is the double-encapsulation we're paying for in the interim.

### Why the Tailscale hijack (Phase 7e) was NOT a port conflict

The Phase 7e incident looked like a "Tailscale collision" but it was a **routing-table hijack**, not a port clash. Tailscale's routing table 52 has priority 5270, checked before the main table (priority 32766). Tailscale had been configured to advertise pod CIDRs (10.42.0.0/16) as subnet routes, so pod traffic was being pulled into table 52 and sent via tailscale0 instead of flannel-wg — even though flannel-wg was bound correctly on UDP 51820. Fix was to clear `--advertise-routes` on all nodes, not anything port-related.

### What removing wg0 requires

Per-site router config (see "Removing the wg0 Tunnel" below):
1. UDP 51820 port forward at each site's router → local flannel-wg gateway node
2. flannel `public-ip` annotation (or K3s `--node-external-ip`) set to the router's public IP on cross-site gateway nodes
3. OCI security list allows UDP 51820 inbound to oracle1

Once those are in place, flannel-wg peers negotiate directly, wg0 is torn down, and the entire `wireguard_tunnel` role + static cross-site route machinery (`wg-static-routes.service`, `wg-static-routes.sh`) can be deleted. The 2026-04-07 outage — static routes flushed by DHCP — becomes structurally impossible because there are no static cross-site routes to flush.

---

## Removing the wg0 Tunnel (Endgame)

The wg0 site-to-site tunnel exists only because routers don't yet forward UDP 51820 to the flannel-wg gateway nodes. Once port forwarding is configured, flannel-wg peers will connect directly and the wg0 tunnel can be removed.

### What Changes

```
Cross-site pod-to-pod AFTER port forwarding (endgame):

  dpi1                                                               tpi3
  ┌──────────┐                                                      ┌──────────┐
  │ Pod      │                                                      │ Pod      │
  │  │       │                                                      │  ▲       │
  │  ▼       │                                                      │  │       │
  │ cni0     │                                                      │ cni0     │
  │  │       │                                                      │  ▲       │
  │  ▼       │           Dustin         Tayven                      │  │       │
  │ flannel- ├──►LAN────►Router────►WAN────►Router───►LAN──────────►├─ flannel-│
  │ wg       │           :51820     ~15ms    :51820                 │ wg       │
  └──────────┘           (fwd to            (fwd to                 └──────────┘
                          dpi2)              tpi1)
                         1 WG layer — NO wg0 tunnel needed
```

### Prerequisites

1. **Tayven router**: Forward UDP 51820 → tpi1 (192.168.1.125)
2. **Dustin router**: Forward UDP 51820 → dpi2 (192.168.2.24) — may already have 51821 forwarded for wg0
3. **Oracle Cloud**: Security list allows UDP 51820 inbound (oracle1 has public IP)

### Removal Steps

1. **Verify flannel-wg can reach cross-site peers directly**
   ```bash
   # From dpi2, check flannel-wg peer for tpi1
   # Current endpoint: 10.100.0.2:51820 (via wg0)
   # Expected after port forward: <tayven-public-ip>:51820 (direct)
   ```

2. **Update flannel public-ip annotations** (if needed)
   ```bash
   # flannel uses flannel.alpha.coreos.com/public-ip for WireGuard endpoints
   # For same-site: LAN IP (automatic)
   # For cross-site: need router's public IP or NAT traversal
   # K3s may need --node-external-ip set to public IP for cross-site nodes
   ```

3. **Test cross-site pod-to-pod without wg0**
   ```bash
   # Temporarily bring down wg0 on dpi2
   sudo wg-quick down wg0
   # Test pod-to-pod from dpi2 to tpi1
   kubectl exec test-dpi2 -- ping -c3 -W5 <tpi1-pod-ip>
   # If it works, wg0 is no longer needed
   ```

4. **Remove wg0 tunnel infrastructure**
   ```bash
   # On dpi2 and tpi1:
   sudo systemctl disable --now wg-quick@wg0
   sudo rm /etc/wireguard/wg0.conf
   ```

5. **Clean up Ansible**
   - Remove or skip wireguard_tunnel role for these hosts
   - Remove wg0-related routes from k3s_config
   - Update this ADR status to "Completed"

6. **Verify final state**
   - All flannel-wg peers show public IPs as endpoints (not 10.100.0.x)
   - Cross-site pod-to-pod latency should be similar (still WAN)
   - Same-site unchanged (already direct LAN)
   - `ip link show` should not have wg0 interface

### What Changes for oracle1

oracle1 is internet-accessible, so its approach differs:
- Option A: Direct flannel-wg (oracle1 public IP is reachable, just open UDP 51820 in OCI security list)
- Option B: Dedicated WireGuard tunnel (dpi2 ↔ oracle1) if flannel-wg can't negotiate through OCI NAT
- Either way, oracle1 should eventually have its own direct path, not routing through wg0

---

## Remaining Tailscale Dependency

After the wg0 tunnel is removed, Tailscale still provides:

- **SSH access** to all nodes (100.x.x.x addresses)
- **K3s node-ip** (all nodes use Tailscale IPs for API server, etcd, kubelet)
- **Web UI access** (*.tailbecff0.ts.net DNS names for Grafana, ArgoCD, etc.)

K3s `node-ip` migration from Tailscale to LAN/WG IPs is a separate, larger effort that requires:
- Rolling K3s restart with new `--node-ip`
- etcd peer URL migration
- Updating all TLS SANs
- New DNS strategy for web UIs (already partially migrated to pi-k3s.chalupatech.com via Cloudflare)

This is out of scope for the CNI migration but is the natural next step toward full Tailscale removal from the cluster data plane.

---

## Key Lessons

1. **Never route pod traffic through a VPN tunnel with its own routing table.** Tailscale's table 52 at priority 5270 can silently override kernel routes, hijacking pod CIDRs.
2. **rp_filter=2 (strict) is incompatible with multi-interface WireGuard setups.** When packets can arrive on flannel-wg but the routing table says the reply goes via a different interface, strict rp_filter drops them.
3. **flannel uses `public-ip` annotations, not K8s InternalIP, for WireGuard endpoints.** This is why same-site traffic works over LAN even though node-ip is a Tailscale address.
4. **Same-site traffic dominates cluster health.** Longhorn, ArgoCD, and most pod-to-pod communication is intra-site. Fixing same-site encapsulation overhead was the highest-impact change.
5. **VXLAN has higher per-packet overhead than WireGuard.** VXLAN adds ~50 bytes (UDP + VXLAN header + inner Ethernet frame) vs WireGuard's ~60 bytes (but WireGuard replaces both VXLAN and the encryption layer, so net savings).
