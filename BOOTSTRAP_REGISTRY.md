# Private Registry Bootstrap Instructions (Host-Mount Strategy)

## Overview
We are deploying the registry using the **Host-Mount Strategy** (ADR 023). 
This decouples the registry from Kubernetes/Vault secrets.

## Role: Registry Host Owner (You choose the node!)
**Decide which node will host the registry (e.g., `dp1` or `tpi2`).**

1.  **Label the Node**:
    ```bash
    # Run this on your control plane (tpi2 via admin, or if you have kubeconfig)
    kubectl label node YOUR_NODE_NAME chalupa.tech/registry-host=true
    ```

2.  **Provision Host Certs**:
    *   On the **Client Node** (e.g. `dp1`), download the bundle (`bootstrap_registry_host.sh`, `server.crt`, `server.key`).
    *   Run:
        ```bash
        chmod +x bootstrap_registry_host.sh
        sudo ./bootstrap_registry_host.sh
        ```
    *   *This will copy the certs to `/var/lib/chalupa-registry/certs/`.*

3.  **Run K3s Update (Bootstrapping Trust)**:
    *   Now run `install.sh` on **ALL NODES** (including `dp1` and `tpi2`).
    *   Use the Tailscale IP of **YOUR_NODE_NAME** (`dp1`) as the `REGISTRY_IP`.

    ```bash
    # Example: If dp1 is 100.80.1.5
    sudo REGISTRY_IP=100.80.1.5 ./install.sh agent  # (or server, depending on the node role)
    ```

4.  **Deployment**:
    *   Commit/Push `chalupa-infra`.
    *   ArgoCD will deploy the registry to the node labeled `chalupa.tech/registry-host=true`.

## Verification
Any node should be able to:
`crictl pull registry.chalupa.local:5000/my-image:latest`
