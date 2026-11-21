#!/usr/bin/env bash
# Check the off-site copy of the fw1 backups on Backblaze B2 (platform
# phase-94a), from the operator's machine, the way a restore after losing
# the site would read it.
#
#   1. Restore proof: every bucket is non-empty and has a file from the
#      last 50h; when MinIO on fw1 is reachable, every MinIO file older than
#      30h is in B2 with the same size. The newest database base backup is
#      DONE and its first and last WAL are present. The newest OpenBao and
#      etcd snapshots are fetched and decrypted; the OpenBao archive must
#      list, and both must match the MinIO original (sha256).
#   2. Key scope: with the CronJob's write key, upload a canary file; it
#      must carry the bucket's default Object Lock retention. Reading it and
#      deleting it permanently must both be refused. A plain delete (B2
#      "hide") is tried last and reported as found.
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
command -v b2 >/dev/null || { echo "b2 not found (brew install b2-tools)"; exit 2; }
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
# Every bucket must be non-empty and fresh: the newest file (its source
# modification time, kept by rclone) within 50 hours. etcd uploads every
# 12h, OpenBao and the base backup daily, WAL continuously.
for b in cnpg-backups etcd-snapshots openbao-snapshots; do
  n=$(rclone lsf -R --files-only "offsite:$b" | wc -l | tr -d ' ')
  sz=$(rclone size --json "offsite:$b" | python3 -c 'import json,sys; print(round(json.load(sys.stdin)["bytes"]/1e9,2))')
  echo "offsite:$b  files=$n  GB=$sz"
  if [ "$n" = 0 ]; then echo "  FAIL: empty"; fail=1; continue; fi
  recent=$(rclone lsf -R --files-only --max-age 50h "offsite:$b" | wc -l | tr -d ' ')
  if [ "$recent" = 0 ]; then echo "  FAIL: nothing newer than 50h"; fail=1; fi
  # Completeness: every MinIO file older than 30h (so at least one daily
  # copy has run since it appeared) is in B2 with the same size. crypt
  # reports decrypted sizes, so nothing is downloaded.
  if [ "$minio_ok" = 1 ]; then
    if rclone check --one-way --size-only --min-age 30h "minio:$b" "offsite:$b" >"$work/check-$b.txt" 2>&1; then
      echo "  every MinIO file older than 30h is here, same size"
    else
      echo "  FAIL: missing or different files:"; grep -E 'ERROR|differ|not found' "$work/check-$b.txt" | head -10; fail=1
    fi
  fi
done

# The newest database base backup must be complete, and the WAL it needs
# (begin_wal to end_wal) must be in the copy.
info=$(rclone lsf -R --files-only --include "*/base/*/backup.info" "offsite:cnpg-backups" | sort | tail -n 1)
if [ -z "$info" ]; then
  echo "FAIL: no base backup (backup.info) in offsite:cnpg-backups"; fail=1
else
  rclone cat "offsite:cnpg-backups/$info" > "$work/backup.info"
  status=$(sed -n 's/^status=//p' "$work/backup.info")
  begin=$(sed -n 's/^begin_wal=//p' "$work/backup.info")
  endw=$(sed -n 's/^end_wal=//p' "$work/backup.info")
  echo "cnpg-backups/$info  status=$status  wal=$begin..$endw"
  [ "$status" = DONE ] || { echo "  FAIL: base backup status is not DONE"; fail=1; }
  server=${info%%/*}
  rclone lsf -R --files-only "offsite:cnpg-backups/$server/wals" > "$work/wals.txt"
  for w in "$begin" "$endw"; do
    if [ -n "$w" ] && grep -q "/$w" "$work/wals.txt"; then echo "  WAL $w present"
    else echo "  FAIL: WAL ${w:-<none>} missing"; fail=1; fi
  done
fi

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
# The write key can hide files (b2_hide_file is a writeFiles call), and the
# lifecycle rule deletes hidden versions. What protects the copy is the
# bucket's default Object Lock retention (governance, 30 days): a locked
# version cannot be deleted by a lifecycle rule or by a key without
# bypassGovernance. So the canary, uploaded the way the job uploads, must
# carry that retention.
canary="key-scope-canary/$(date -u +%Y%m%dT%H%M%SZ).txt"
echo "offsite-check canary" > "$work/canary.txt"
if rclone copyto "$work/canary.txt" "b2w:$BUCKET/$canary" --retries 1 --low-level-retries 1 >/dev/null 2>&1; then
  echo "write: allowed (expected)"
else echo "FAIL: write key cannot upload"; fail=1; fi
mode=$(B2_ACCOUNT_INFO="$work/b2-account-info" B2_APPLICATION_KEY_ID="$B2_READ_KEY_ID" B2_APPLICATION_KEY="$B2_READ_KEY" \
  b2 file info "b2://$BUCKET/$canary" 2>/dev/null \
  | python3 -c 'import json,sys; print(((json.load(sys.stdin).get("fileRetention") or {}).get("mode")) or "none")' || echo "unknown")
if [ "$mode" = governance ] || [ "$mode" = compliance ]; then
  echo "retention on a job upload: $mode (expected)"
else echo "FAIL: a job upload has retention '$mode', not the bucket default"; fail=1; fi
if rclone cat "b2w:$BUCKET/$canary" --retries 1 --low-level-retries 1 >/dev/null 2>&1; then
  echo "FAIL: write key can read"; fail=1
else echo "read: refused (expected)"; fi
if rclone deletefile "b2w:$BUCKET/$canary" --b2-hard-delete --retries 1 --low-level-retries 1 >/dev/null 2>&1; then
  echo "FAIL: write key can permanently delete"; fail=1
else echo "permanent delete: refused (expected)"; fi
if rclone deletefile "b2w:$BUCKET/$canary" --retries 1 --low-level-retries 1 >/dev/null 2>&1; then
  echo "hide: allowed (the version stays under retention; lifecycle cannot delete it before then)"
else echo "hide: refused"; fi

[ "$fail" = 0 ] && echo "OFFSITE CHECK OK" || echo "OFFSITE CHECK FAILED"
exit "$fail"
