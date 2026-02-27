# Infrastructure Conventions - chalupa-infra

This document defines the architectural and operational standards for the `chalupa-infra` project. These conventions MUST be followed by all contributors and automated agents.

## 1. Networking & Security

### 1.1 SSL/TLS Standard
- **Mandatory HTTPS**: All external-facing services MUST be exposed via HTTPS (SSL/TLS).
- **Certificate Management**: Certificates are managed exclusively by `cert-manager`.
- **ClusterIssuer**: The standard ClusterIssuer is `letsencrypt-prod` (ACME production).
- **DNS Solver**: DNS-01 challenge via Cloudflare is the primary method for certificate issuance.
- **Ingress Controller**: Traefik is the default ingress controller. All ingresses must use the `websecure` entrypoint.
- **Backend Communication**: 
    - Full SSL (encrypted backend) is preferred for critical management services like ArgoCD.
    - Standard service-to-service communication within the cluster may remain HTTP for simplicity, provided it occurs over the internal Kubernetes network.

### 1.2 Domain Naming
- **Root Domain**: `chalupatech.com`
- **Pattern**: Services MUST be exposed using the pattern `{service-name}.chalupatech.com`.

## 2. Infrastructure Management

### 2.1 Ansible Roles
- Roles are modular and located in `scripts/raspberrypi-k3s/ansible/roles/`.
- All sensitive variables MUST be encrypted using Ansible Vault in `vault.yml`.

### 2.2 GitOps (ArgoCD)
- ArgoCD is the source of truth for application lifecycle after the cluster is bootstrapped.
- Use `ApplicationSet` for grouping related platform services.
- Server-side apply and automated pruning should be enabled by default.

## 3. Secret Management

### 3.1 OpenBao (Vault)
- OpenBao is used for runtime secret management.
- Integration with Kubernetes is via the `external-secrets` operator.
- Always use the `openbao` namespace for OpenBao resources.
