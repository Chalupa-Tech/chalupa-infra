#!/usr/bin/env bash
# Check the off-site copy of the fw1 backups on Backblaze B2 (platform
# phase-94a), from the operator's machine, the way a restore after losing
# the site would read it.
#
#   1. Restore proof: fetch the newest OpenBao and etcd snapshots from B2,
#      decrypt them, check the OpenBao archive lists, and compare each with
#      the original in MinIO on fw1 (sha256), when MinIO is reachable.
#   2. Key scope: with the CronJob's write key, upload a canary file, then
#      try to read it and to delete it permanently. Both must be refused.
#      A plain delete (B2 "hide") is tried last and reported as found.
#
# Needs: rclone, ansible-vault (the vault password, from the Keychain via
# ansible/ansible.cfg or asked for), and a short-lived B2
# key that can list and read the bucket, made with the B2 CLI:
#
#   b2 key create --bucket chalupa-offsite-27c70e32 --duration 3600 \
#     offsite-check listBuckets,listFiles,readFiles
#
# Usage (from the chalupa-infra repo root):
#   B2_READ_KEY_ID=... B2_READ_KEY=... bash scripts/backups/offsite-check.sh
# Without the two variables it asks for them. Nothing secret is printed.
#
# In a real restore, the same remotes fetch everything, e.g.
#   rclone copy offsite:openbao-snapshots ./openbao-snapshots
set -euo pipefail

BUCKET=${OFFSITE_B2_BUCKET:-chalupa-offsite-27c70e32}
MINIO_ENDPOINT=${MINIO_ENDPOINT:-http://192.168.2.51:9000}
VAULT_FILE=ansible/inventory/group_vars/all/vault.yml

command -v rclone >/dev/null || { echo "rclone not found (brew install rclone)"; exit 2; }
[ -f "$VAULT_FILE" ] || { echo "run from the chalupa-infra repo root"; exit 2; }

if [ -z "${B2_READ_KEY_ID:-}" ]; then read -r -p "B2 read key id: " B2_READ_KEY_ID; fi
if [ -z "${B2_READ_KEY:-}" ]; then read -r -s -p "B2 read key: " B2_READ_KEY; echo; fi

work=$(mktemp -d "${TMPDIR:-/tmp}/offsite-check.XXXXXX")
trap 'rm -rf "$work"' EXIT
umask 077

# One vault prompt; each value goes to its own 0600 file, never to argv.
# From ansible/ so ansible.cfg supplies the vault password (Keychain).
(cd ansible && ansible-vault view inventory/group_vars/all/vault.yml) | python3 -c '
import sys, yaml, os
v = yaml.safe_load(sys.stdin)
out = sys.argv[1]
for k in ["vault_offsite_crypt_password", "vault_offsite_crypt_salt",
          "vault_offsite_b2_key_id", "vault_offsite_b2_key",
          "vault_minio_access_key", "vault_minio_secret_key"]:
    if not v.get(k):
        sys.exit(f"{k} missing from the vault")
    with open(os.path.join(out, k), "w") as f:
        f.write(str(v[k]))
' "$work"
s() { cat "$work/$1"; }

export RCLONE_CONFIG="$work/rclone.conf"
export RCLONE_CONFIG_B2R_TYPE=b2 RCLONE_CONFIG_B2W_TYPE=b2
export RCLONE_CONFIG_B2R_ACCOUNT="$B2_READ_KEY_ID" RCLONE_CONFIG_B2R_KEY="$B2_READ_KEY"
RCLONE_CONFIG_B2W_ACCOUNT=$(s vault_offsite_b2_key_id)
RCLONE_CONFIG_B2W_KEY=$(s vault_offsite_b2_key)
export RCLONE_CONFIG_B2W_ACCOUNT RCLONE_CONFIG_B2W_KEY
export RCLONE_CONFIG_OFFSITE_TYPE=crypt RCLONE_CONFIG_OFFSITE_REMOTE="b2r:$BUCKET"
RCLONE_CONFIG_OFFSITE_PASSWORD=$(rclone obscure - < "$work/vault_offsite_crypt_password")
RCLONE_CONFIG_OFFSITE_PASSWORD2=$(rclone obscure - < "$work/vault_offsite_crypt_salt")
export RCLONE_CONFIG_OFFSITE_PASSWORD RCLONE_CONFIG_OFFSITE_PASSWORD2
export RCLONE_CONFIG_MINIO_TYPE=s3 RCLONE_CONFIG_MINIO_PROVIDER=Minio RCLONE_CONFIG_MINIO_ENDPOINT="$MINIO_ENDPOINT"
RCLONE_CONFIG_MINIO_ACCESS_KEY_ID=$(s vault_minio_access_key)
RCLONE_CONFIG_MINIO_SECRET_ACCESS_KEY=$(s vault_minio_secret_key)
export RCLONE_CONFIG_MINIO_ACCESS_KEY_ID RCLONE_CONFIG_MINIO_SECRET_ACCESS_KEY

fail=0
minio_ok=1
rclone lsf --max-depth 1 minio:openbao-snapshots --contimeout 5s --low-level-retries 1 --retries 1 >/dev/null 2>&1 || {
  minio_ok=0; echo "note: MinIO at $MINIO_ENDPOINT not reachable from here; skipping the comparison with the originals"; }

echo "== 1. restore proof"
for b in cnpg-backups etcd-snapshots openbao-snapshots; do
  n=$(rclone lsf -R --files-only "offsite:$b" | wc -l | tr -d ' ')
  sz=$(rclone size --json "offsite:$b" | python3 -c 'import json,sys; print(round(json.load(sys.stdin)["bytes"]/1e9,2))')
  echo "offsite:$b  files=$n  GB=$sz"
done

newest() { rclone lsf --files-only --format "tp" --separator " " "offsite:$1" | sort | tail -n 1 | cut -d" " -f3-; }
for b in openbao-snapshots etcd-snapshots; do
  f=$(newest "$b")
  [ -n "$f" ] || { echo "FAIL: no file in offsite:$b"; fail=1; continue; }
  rclone copyto "offsite:$b/$f" "$work/$f"
  sum=$(shasum -a 256 "$work/$f" | cut -d" " -f1)
  echo "$b/$f  bytes=$(wc -c < "$work/$f" | tr -d ' ')  sha256=$sum"
  if [ "$b" = openbao-snapshots ]; then
    if tar -tzf "$work/$f" >"$work/tar.txt"; then echo "  archive lists: $(tr '\n' ' ' < "$work/tar.txt")"
    else echo "  FAIL: not a readable archive"; fail=1; fi
  fi
  if [ "$minio_ok" = 1 ]; then
    orig=$(rclone cat "minio:$b/$f" | shasum -a 256 | cut -d" " -f1)
    if [ "$orig" = "$sum" ]; then echo "  matches the MinIO original"
    else echo "  FAIL: differs from the MinIO original ($orig)"; fail=1; fi
  fi
done

echo "== 2. key scope (the CronJob's write key)"
canary="key-scope-canary/$(date -u +%Y%m%dT%H%M%SZ).txt"
echo "offsite-check canary" > "$work/canary.txt"
if rclone copyto "$work/canary.txt" "b2w:$BUCKET/$canary" --retries 1 --low-level-retries 1 >/dev/null 2>&1; then
  echo "write: allowed (expected)"
else echo "FAIL: write key cannot upload"; fail=1; fi
if rclone cat "b2w:$BUCKET/$canary" --retries 1 --low-level-retries 1 >/dev/null 2>&1; then
  echo "FAIL: write key can read"; fail=1
else echo "read: refused (expected)"; fi
if rclone deletefile "b2w:$BUCKET/$canary" --b2-hard-delete --retries 1 --low-level-retries 1 >/dev/null 2>&1; then
  echo "FAIL: write key can permanently delete"; fail=1
else echo "permanent delete: refused (expected)"; fi
if rclone deletefile "b2w:$BUCKET/$canary" --retries 1 --low-level-retries 1 >/dev/null 2>&1; then
  echo "hide: allowed (the lifecycle rule deletes hidden files after 7 days)"
else echo "hide: refused"; fi

[ "$fail" = 0 ] && echo "OFFSITE CHECK OK" || echo "OFFSITE CHECK FAILED"
exit "$fail"
