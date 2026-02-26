# Tailscale Operator — Vault Setup

The Vault policy and Kubernetes auth role for the Tailscale operator are **automatically created** by the Ansible `openbao_setup` role during cluster provisioning.

The only manual step required is injecting your actual Tailscale OAuth credentials into OpenBao.

## 1. Get OpenBao Credentials

The root token is automatically generated during cluster bootstrapping. You can find it on your primary K3s server node:

```bash
# SSH into your primary K3s server node
cat /var/lib/rancher/k3s/server/openbao_init_data.json | grep root_token
```

## 2. Write secrets to OpenBao

Using the `root_token` from step 1, execute into the OpenBao pod and write the credentials for the Tailscale operator:

```bash
# On any machine with kubectl access to the cluster:
export BAO_TOKEN="<root_token_here>"

kubectl exec -n openbao openbao-0 -- sh -c "
  BAO_TOKEN=$BAO_TOKEN bao kv put secret/platform/tailscale-operator \\
    client_id=\"<YOUR_OAUTH_CLIENT_ID>\" \\
    client_secret=\"<YOUR_OAUTH_CLIENT_SECRET>\"
"
```

## 3. Verify

Once ArgoCD syncs the `tailscale-operator` application:

```bash
kubectl get externalsecret operator-oauth -n tailscale-operator
kubectl get secret operator-oauth -n tailscale-operator
kubectl get pods -n tailscale-operator
```

The operator pod should come up and a new device tagged `tag:k8s-operator` should appear in the Tailscale admin console.

If ESO sync fails, check:

```bash
kubectl get externalsecret operator-oauth -n tailscale-operator \
  -o jsonpath='{.status.conditions}'
```

> **Rollback note:** If a temporary manual secret is created as a fallback, revoke
> it immediately once ESO sync is restored — do not leave it in place.
