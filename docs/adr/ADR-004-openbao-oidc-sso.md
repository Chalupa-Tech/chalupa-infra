# ADR-004: OpenBao OIDC SSO for Gitea, Grafana, and ArgoCD

**Status:** Accepted  
**Date:** 2026-03-15  
**Deciders:** ddowell, tbigelow

---

## Context

All three internal services (Gitea, Grafana, ArgoCD) originally had separate credential stores. We needed to centralize authentication so human operators only manage one identity: their OpenBao entity. OpenBao (the open-source Vault fork) is already deployed as the secrets backend, making it a natural OIDC provider.

---

## Decision

Use **OpenBao as the OIDC identity provider** for all three services. Operators maintain a single OpenBao userpass credential; all service logins flow through OpenBao OIDC.

---

## Architecture

```
Browser                OpenBao (k3s)            Gitea / Grafana / ArgoCD
   │                       │                              │
   │─── click SSO btn ────▶│                              │
   │                       │◀─── redirect from service ───│
   │──── login (userpass) ─▶│                              │
   │                       │─── auth code ──────────────▶ │
   │                       │◀─── token exchange ─────────  │
   │◀──────────────────────┼──── logged in ───────────────│
```

OpenBao issues ID tokens with the following claims (from the `user` scope):

| Claim | Source |
|---|---|
| `preferred_username` | `identity.entity.name` (OpenBao entity name) |
| `email` | `identity.entity.metadata.email` (stored per entity) |
| `groups` | `identity.entity.groups.names` |

---

## Key Design Choices

### OIDC Scope Template Format

OpenBao's Go template engine **auto-JSON-quotes string values**. Do NOT add manual `"..."` around string placeholders:

```json
// ✅ CORRECT — preferred_username renders as "ddowell" (already quoted)
{"preferred_username":{{identity.entity.name}},"email":{{identity.entity.metadata.email}},"groups":{{identity.entity.groups.names}}}

// ❌ WRONG — creates double-quoted "\"ddowell\"" → invalid JSON, 400 error
{"preferred_username":"{{identity.entity.name}}","email":"..."}
```

### Email Storage

Email addresses are stored as **entity metadata** in OpenBao (not as a computed field), because the template engine cannot concatenate strings to build computed email addresses. Each user's email is set in `managed_users[].email` in `vars.yml` and applied to the entity on every playbook run (idempotent upsert).

### OIDC Assignment: `allow_all`

All OIDC clients (gitea, grafana, argocd) use the `allow_all` built-in assignment. This means **any authenticated OpenBao entity** can use SSO into any of the three services.

> **Future improvement:** Replace `allow_all` with a named group assignment (e.g., `sso-users` group) to more explicitly control which entities can access services. This is low-urgency for a small team but is better practice for auditability.

### Gitea Auto-Provisioning

Gitea's `[oauth2_client]` section (environment variables) controls OIDC behavior:

```ini
[oauth2_client]
ENABLE_AUTO_REGISTRATION = true   # auto-create Gitea user on first OIDC login
ACCOUNT_LINKING = auto            # auto-link to existing account by email
USERNAME = preferred_username     # use OIDC preferred_username as Gitea username
OPENID_CONNECT_SCOPES = openid profile email user  # 'user' = our custom scope
```

`DISABLE_REGISTRATION = true` (in `[service]`) remains set to block direct form-based registration. The `[oauth2_client]` settings operate independently.

### OpenBao Default Policy

The built-in `default` policy is extended to allow all authenticated tokens to use the OIDC authorize endpoint:

```hcl
path "identity/oidc/*" { capabilities = ["read", "update"] }
path "sys/capabilities-self" { capabilities = ["update"] }
path "sys/internal/ui/mounts" { capabilities = ["read"] }
```

---

## Adding a New User

1. Add entry to `managed_users` in `vars.yml`:
   ```yaml
   - name: newuser
     email: newuser@example.com
     pubkey_file: "{{ playbook_dir }}/../ssh_keys/newuser.pub"
     argocd_admin: false
   ```

2. Create OpenBao userpass credential:
   ```bash
   bao write auth/userpass/users/newuser password=<initial-password>
   ```

3. Run `openbao-only.yml` → entity + alias + email metadata created automatically.

4. On first login: user navigates to any service → clicks **Sign in with OpenBao** → logs in with userpass → Gitea/Grafana/ArgoCD account auto-created.

---

## Stale Browser Session Warning

The OpenBao UI stores the user's token in `localStorage`. If a token becomes orphaned (references a deleted entity), OIDC will fail silently with "state token mismatch" even after re-deployment.

**Resolution:** Sign out of the OpenBao UI fully (not just the service), or open an **incognito/private browser window** to force a fresh token.

---

## References

- [OpenBao OIDC Provider Docs](https://openbao.org/docs/secrets/identity/oidc-provider/)
- [Gitea OAuth2 Client Config](https://docs.gitea.com/administration/config-cheat-sheet#oauth2-client-oauth2_client)
- `ansible/roles/openbao_setup/tasks/oidc_setup.yml` — OIDC setup automation
- `ansible/roles/gitea_setup/tasks/main.yml` — Gitea OIDC client config
- `ansible/inventory/group_vars/all/vars.yml` — scope template + managed users
