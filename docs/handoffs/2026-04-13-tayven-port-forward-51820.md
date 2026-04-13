# Handoff: Port Forward UDP 51820 (Tayven)

**Date:** 2026-04-13
**Owner:** Tayven
**Blocks:** Removal of wg0 site-to-site tunnel, `wireguard_tunnel` role, and all cross-site static route machinery.

## Ask

Forward **UDP 51820 → tpi1 (192.168.1.125)** on the Tayven-site router.

## Why

Our K3s cluster uses flannel with WireGuard-native as the pod overlay. Same-site pod traffic already goes directly over LAN. For cross-site pods (Tayven site ↔ Dustin site ↔ Oracle), flannel-wg peers need to reach each other on UDP 51820 — but right now the Tayven router doesn't forward that port, so we built a temporary site-to-site tunnel (wg0 on UDP 51821) to bridge the gap.

## Problem this workaround causes

The wg0 bridge relies on per-node static routes (e.g., `192.168.1.0/24 via 192.168.2.24`) that DHCP renewals silently flush. This caused a 44h metrics+logs outage on 2026-04-07. Fixing the route persistence (networkd, nmcli, dhclient hooks) is wasted effort because wg0 itself is scheduled for removal.

## After the forward is in place

Dustin will:
1. Forward UDP 51820 on the Dustin router → dpi2 (192.168.2.24)
2. Update flannel `public-ip` / K3s `--node-external-ip` on dpi2, tpi1, oracle1 to use each site's router public IP
3. Validate cross-site flannel-wg handshakes at the new endpoints
4. Tear down wg0, the `wireguard_tunnel` Ansible role, and the `wg-static-routes.service` + script

After this: no more DHCP route fragility, single WireGuard encapsulation cross-site, and one fewer failure domain.

## Full architecture context

- `docs/adr/ADR-007-cni-migration-flannel-wireguard-native.md` — see "Port Allocation & Why We Need wg0 At All" and "Removing the wg0 Tunnel (Endgame)"
