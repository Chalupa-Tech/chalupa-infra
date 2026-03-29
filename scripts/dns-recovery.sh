#!/usr/bin/env bash
set -euo pipefail

# DNS Recovery Script
# ====================
# Run this after any DNS outage or CoreDNS disruption to clear cached failures
# across the cluster. Designed for the K3s + flannel wireguard-native + Tailscale
# overlay stack.
#
# What this fixes:
#   - Stale conntrack entries pointing to dead CoreDNS pod IPs (ClusterIP DNAT broken)
#   - Pods with cached DNS SERVFAIL/timeout responses
#   - ArgoCD unable to sync (repo-server caches DNS failures in Redis)
#   - external-dns CrashLoopBackOff from DNS timeouts
#
# Prerequisites:
#   - kubectl context pointing to the cluster
#   - conntrack installed on all nodes (apt install conntrack)
#   - Ansible inventory at ../ansible/inventory/hosts.yml
#
# Usage:
#   ./scripts/dns-recovery.sh              # Full recovery
#   ./scripts/dns-recovery.sh --verify     # Verify-only (no changes)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ANSIBLE_DIR="$SCRIPT_DIR/../ansible"
VERIFY_ONLY=false

if [[ "${1:-}" == "--verify" ]]; then
    VERIFY_ONLY=true
fi

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[dns-recovery]${NC} $*"; }
warn() { echo -e "${YELLOW}[dns-recovery]${NC} $*"; }
fail() { echo -e "${RED}[dns-recovery]${NC} $*"; }

# --- Phase 1: Verify CoreDNS is running ---
log "Phase 1: Checking CoreDNS pods"
COREDNS_PODS=$(kubectl get pods -n kube-system -l k8s-app=kube-dns -o wide --no-headers 2>&1)
COREDNS_READY=$(echo "$COREDNS_PODS" | grep -c "Running" || true)

if [[ "$COREDNS_READY" -lt 1 ]]; then
    fail "No CoreDNS pods running! Fix CoreDNS deployment first."
    echo "$COREDNS_PODS"
    exit 1
fi
log "  CoreDNS: $COREDNS_READY pods running"
echo "$COREDNS_PODS" | while read -r line; do echo "    $line"; done

# --- Phase 2: Restart CoreDNS ---
if [[ "$VERIFY_ONLY" == false ]]; then
    log "Phase 2: Restarting CoreDNS deployment"
    kubectl rollout restart deployment coredns -n kube-system
    kubectl rollout status deployment coredns -n kube-system --timeout=90s
    log "  CoreDNS restarted successfully"
else
    log "Phase 2: (skip) Would restart CoreDNS deployment"
fi

# --- Phase 3: Flush conntrack on all nodes ---
if [[ "$VERIFY_ONLY" == false ]]; then
    log "Phase 3: Flushing conntrack tables on all nodes"
    if [[ -d "$ANSIBLE_DIR" ]]; then
        VAULT_FILE="$ANSIBLE_DIR/vault-password.sh"
        VAULT_FLAG=""
        if [[ -f "$VAULT_FILE" ]]; then
            VAULT_FLAG="--vault-password-file $VAULT_FILE"
        else
            VAULT_FLAG="--ask-vault-pass"
        fi

        cd "$ANSIBLE_DIR"
        ansible 'k3s_servers:k3s_ha_servers:k3s_oci_servers:k3s_agents:k3s_vm_agents' \
            -i inventory/hosts.yml $VAULT_FLAG \
            -m shell -a "conntrack -F 2>/dev/null && echo flushed || echo 'conntrack not installed — run: apt install conntrack'" \
            --become 2>&1 | grep -E "flushed|not installed|UNREACHABLE"
        cd "$SCRIPT_DIR/.."
        log "  Conntrack flushed"
    else
        warn "  Ansible directory not found at $ANSIBLE_DIR — flush conntrack manually on each node:"
        warn "    ssh <node> 'sudo conntrack -F'"
    fi
else
    log "Phase 3: (skip) Would flush conntrack on all nodes"
fi

# --- Phase 4: Restart DNS-sensitive services ---
if [[ "$VERIFY_ONLY" == false ]]; then
    log "Phase 4: Restarting DNS-sensitive services"

    # ArgoCD — Redis caches DNS, repo-server caches git refs, controller caches app state
    if kubectl get namespace argocd &>/dev/null; then
        log "  Restarting ArgoCD (Redis → repo-server → controller)"
        kubectl rollout restart deployment argocd-redis -n argocd 2>/dev/null || true
        kubectl rollout status deployment argocd-redis -n argocd --timeout=60s 2>/dev/null || true
        kubectl rollout restart deployment argocd-repo-server -n argocd 2>/dev/null || true
        kubectl rollout restart statefulset argocd-application-controller -n argocd 2>/dev/null || true
        kubectl rollout restart deployment argocd-server -n argocd 2>/dev/null || true
    fi

    # external-dns — usually self-recovers from CrashLoopBackOff but let's help it
    if kubectl get namespace external-dns &>/dev/null; then
        EXTDNS_RESTARTS=$(kubectl get pods -n external-dns --no-headers 2>/dev/null | awk '{print $4}' | head -1)
        if [[ "${EXTDNS_RESTARTS:-0}" -gt 5 ]]; then
            log "  Restarting external-dns ($EXTDNS_RESTARTS restarts)"
            kubectl rollout restart deployment external-dns -n external-dns 2>/dev/null || true
        else
            log "  external-dns looks healthy ($EXTDNS_RESTARTS restarts)"
        fi
    fi

    # Force ArgoCD hard refresh on all apps
    log "  Triggering ArgoCD hard refresh on all applications"
    sleep 15  # wait for ArgoCD components to be ready
    kubectl get applications -n argocd -o name 2>/dev/null | while read -r app; do
        kubectl patch "$app" -n argocd --type merge \
            -p '{"metadata":{"annotations":{"argocd.argoproj.io/refresh":"hard"}}}' 2>/dev/null || true
    done
    log "  ArgoCD refresh triggered"
else
    log "Phase 4: (skip) Would restart ArgoCD, external-dns"
fi

# --- Phase 5: Verify DNS ---
log "Phase 5: Verifying DNS resolution"
kubectl delete pod dns-recovery-test --force 2>/dev/null || true
sleep 2

kubectl run dns-recovery-test --image=busybox:1.36 --restart=Never \
    --command -- sh -c '
        echo "=== Internal DNS ==="
        nslookup kubernetes.default.svc.cluster.local 2>&1
        INTERNAL=$?
        echo "=== External DNS ==="
        nslookup github.com 2>&1
        EXTERNAL=$?
        echo "=== Direct upstream ==="
        nslookup github.com 8.8.8.8 2>&1
        DIRECT=$?
        echo "RESULTS: internal=$INTERNAL external=$EXTERNAL direct=$DIRECT"
    ' 2>/dev/null

sleep 12
RESULTS=$(kubectl logs dns-recovery-test 2>/dev/null || echo "pod failed to run")
echo "$RESULTS"

kubectl delete pod dns-recovery-test --force 2>/dev/null || true

if echo "$RESULTS" | grep -q "RESULTS: internal=0 external=0 direct=0"; then
    log "DNS recovery complete — all checks passed"
elif echo "$RESULTS" | grep -q "direct=0"; then
    warn "External DNS (direct) works but ClusterIP may still have issues"
    warn "Try: kubectl rollout restart deployment coredns -n kube-system"
    warn "Then re-run this script"
else
    fail "DNS still broken. Check:"
    fail "  1. CoreDNS logs: kubectl logs -n kube-system -l k8s-app=kube-dns --tail=20"
    fail "  2. flannel-wg tunnels: ssh <node> 'ping -c1 <coredns-pod-ip>'"
    fail "  3. Node resolv.conf: /etc/rancher/k3s/resolv.conf should have 8.8.8.8"
    exit 1
fi

# --- Phase 6: Node connectivity spot-check ---
log "Phase 6: Spot-checking cross-node pod connectivity"
COREDNS_IPS=$(kubectl get pods -n kube-system -l k8s-app=kube-dns -o jsonpath='{range .items[*]}{.status.podIP}{" "}{.spec.nodeName}{"\n"}{end}')
NODES=$(kubectl get nodes --no-headers -o custom-columns=NAME:.metadata.name | grep -v tpi2)
FAIL_COUNT=0

for node in $NODES; do
    # Pick first CoreDNS pod IP that's NOT on this node
    TARGET_IP=$(echo "$COREDNS_IPS" | grep -v "$node" | head -1 | awk '{print $1}')
    if [[ -z "$TARGET_IP" ]]; then continue; fi

    kubectl delete pod "wg-check-$node" --force 2>/dev/null || true
    kubectl run "wg-check-$node" --image=busybox:1.36 --restart=Never \
        --overrides="{\"spec\":{\"nodeSelector\":{\"kubernetes.io/hostname\":\"$node\"}}}" \
        --command -- sh -c "nslookup github.com $TARGET_IP >/dev/null 2>&1 && echo OK || echo FAIL" 2>/dev/null || true
done

sleep 12

for node in $NODES; do
    RESULT=$(kubectl logs "wg-check-$node" 2>/dev/null | tail -1)
    if [[ "$RESULT" == "OK" ]]; then
        log "  $node → CoreDNS: OK"
    else
        fail "  $node → CoreDNS: FAIL (flannel-wg tunnel may be broken)"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
    kubectl delete pod "wg-check-$node" --force 2>/dev/null || true
done

if [[ "$FAIL_COUNT" -gt 0 ]]; then
    warn "$FAIL_COUNT node(s) have broken flannel-wg tunnels"
    warn "Fix: restart K3s on the affected node, then re-run this script"
else
    log "All nodes can reach CoreDNS cross-node"
fi

log "Done."
