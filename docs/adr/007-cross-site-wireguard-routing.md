# ADR-007: Cross-Site WireGuard Routing

## Status

Accepted (temporary workaround in place, permanent fix pending)

## Context

The K3s cluster spans three physical sites:

- **site_dustin** (192.168.2.0/24): dpi1, dpi2, dpi3, fw1
- **site_tayven** (192.168.1.0/24): tpi1, tpi2, tpi3
- **site_oracle** (10.0.0.0/24): oracle1

After migrating from Tailscale CNI to flannel with `wireguard-native` backend (phase 7),
flannel-wg auto-detects each node's physical LAN IP for WireGuard peer endpoints. These
IPs are only routable within a single site — cross-site flannel-wg handshakes fail because
the routers don't forward UDP 51820.

## Decision

### Permanent fix (pending router access)

Forward **UDP 51820** at both site routers:

- **Dustin router** → 192.168.2.24 (dpi2)
- **Tayven router** → 192.168.1.125 (tpi1)

This lets flannel-wg's per-node WireGuard tunnels work directly across sites.
No gateway, no double encapsulation, no additional software.

### Temporary workaround (current)

A **wg0 site-to-site tunnel** between dpi2 (dustin gateway) and tpi1 (tayven gateway)
on port 51821. Static routes on non-gateway nodes direct cross-site traffic through
the local gateway.

- Dustin router forwards UDP 51821 → dpi2 (done)
- tpi1 initiates outbound to dpi2's public IP — no tayven router access needed
- PersistentKeepalive keeps the tayven NAT mapping alive

**Known limitation**: traffic originating from dpi2 itself to tayven pods fails
(WireGuard-over-WireGuard kernel routing issue on the gateway node). All other
nodes route cross-site traffic through dpi2 successfully.

### When to remove the workaround

Once both routers forward UDP 51820:

1. Stop wg0: `ansible dpi2,tpi1,oracle1 -m systemd -a "name=wg-quick@wg0 state=stopped enabled=false"`
2. Remove static routes: `ansible site_dustin,site_tayven,site_oracle -m systemd -a "name=wg-static-routes state=stopped enabled=false"`
3. Remove dustin router forward for UDP 51821
4. Restart K3s on servers to reset flannel-wg peer endpoints

## Alternatives considered

- **flannel-iface: tailscale0** — Forces flannel-wg to use Tailscale IPs. Works without
  router changes but creates WireGuard-over-WireGuard for ALL traffic (same-site included).
  Rejected as permanent overhead for a routing problem.

- **Kilo CNI** — Purpose-built for multi-site K8s with WireGuard NAT traversal. Would
  require another CNI migration. Rejected as unnecessary — flannel-wg works once ports
  are forwarded.

- **Tailscale subnet routing** — Advertise site subnets via Tailscale. Still routes
  through Tailscale's WireGuard. Same overhead concern as flannel-iface.
