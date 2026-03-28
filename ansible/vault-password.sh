#!/bin/bash
# Retrieve Ansible vault password from macOS Keychain or CI env var.
# Local: stored via `security add-generic-password -a "$USER" -s "ansible-vault" -w`
# CI: inject ANSIBLE_VAULT_PASSWORD as a secret env var.
#
# Falls back to interactive prompt if neither source is available.
if [ -n "$ANSIBLE_VAULT_PASSWORD" ]; then
    echo "$ANSIBLE_VAULT_PASSWORD"
elif command -v security &>/dev/null; then
    pw=$(security find-generic-password -a "$USER" -s "ansible-vault" -w 2>/dev/null)
    if [ $? -eq 0 ]; then
        echo "$pw"
    else
        read -rsp "Vault password (keychain entry not found): " pw >&2 </dev/tty
        echo >&2
        echo "$pw"
    fi
else
    read -rsp "Vault password: " pw >&2 </dev/tty
    echo >&2
    echo "$pw"
fi
