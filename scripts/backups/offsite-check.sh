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
#      "hide") is tried last; if it is allowed, the hidden canary must come
#      back through --b2-version-at, the restore path for hidden files.
#
# Needs: rclone, the b2 CLI and ansible-vault (the vault password, from the
# Keychain via ansible/ansible.cfg or asked for). The B2 read key is in the
# vault (vault_offsite_b2_read_key_id / _key): a permanent key that can only
# list and read this bucket and its retention settings. It fetches
# ciphertext; the crypt password, also in the vault, decrypts it. If it is
# ever lost, make another with the B2 master login:
#
#   b2 key create --bucket chalupa-offsite-27c70e32 \
#     offsite-read listBuckets,listFiles,readFiles,readFileRetentions,readBucketRetentions
#
# Usage (from the chalupa-infra repo root):
#   bash scripts/backups/offsite-check.sh
# B2_READ_KEY_ID and B2_READ_KEY in the environment override the vault's
# key. Nothing secret is printed.
#
# In a real restore, the same remotes fetch everything, e.g.
#   rclone copy offsite:openbao-snapshots ./openbao-snapshots
# If files may have been hidden (the job's key can hide; Object Lock keeps
# the versions), read the bucket as it was before that time instead:
#   rclone copy offsite:openbao-snapshots ./openbao-snapshots --b2-version-at 2026-11-20T00:00:00Z
set -euo pipefail

BUCKET=${OFFSITE_B2_BUCKET:-chalupa-offsite-27c70e32}
MINIO_ENDPOINT=${MINIO_ENDPOINT:-http://192.168.2.51:9000}
VAULT_FILE=ansible/inventory/group_vars/all/vault.yml

command -v rclone >/dev/null || { echo "rclone not found (brew install rclone)"; exit 2; }
command -v b2 >/dev/null || { echo "b2 not found (brew install b2-tools)"; exit 2; }
[ -f "$VAULT_FILE" ] || { echo "run from the chalupa-infra repo root"; exit 2; }

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
          "vault_minio_access_key", "vault_minio_secret_key",
          "vault_offsite_b2_read_key_id", "vault_offsite_b2_read_key"]:
    if not v.get(k):
        sys.exit(f"{k} missing from the vault")
    with open(os.path.join(out, k), "w") as f:
        f.write(str(v[k]))
' "$work"
s() { cat "$work/$1"; }
B2_READ_KEY_ID=${B2_READ_KEY_ID:-$(s vault_offsite_b2_read_key_id)}
B2_READ_KEY=${B2_READ_KEY:-$(s vault_offsite_b2_read_key)}

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

# The newest database base backup must be complete: DONE, its data
# present, and every WAL segment from begin_wal to end_wal in the copy
# (the range is computed, so a gap is caught without MinIO). With MinIO
# reachable, its base directory must match the original file for file.
# Newest by backup id (the directory name, a UTC timestamp), whatever the
# server prefix.
info=$(rclone lsf -R --files-only --include "*/base/*/backup.info" "offsite:cnpg-backups" \
  | awk -F/ '{print $(NF-1) " " $0}' | sort | tail -n 1 | cut -d" " -f2-)
if [ -z "$info" ]; then
  echo "FAIL: no base backup (backup.info) in offsite:cnpg-backups"; fail=1
else
  rclone cat "offsite:cnpg-backups/$info" > "$work/backup.info"
  status=$(sed -n 's/^status=//p' "$work/backup.info")
  begin=$(sed -n 's/^begin_wal=//p' "$work/backup.info")
  endw=$(sed -n 's/^end_wal=//p' "$work/backup.info")
  segsize=$(sed -n 's/^xlog_segment_size=//p' "$work/backup.info")
  basedir=${info%/backup.info}
  server=${info%%/*}
  echo "cnpg-backups/$basedir  status=$status  wal=$begin..$endw"
  [ "$status" = DONE ] || { echo "  FAIL: base backup status is not DONE"; fail=1; }
  # Base backups run daily; the newest must have ended within 50 hours.
  age=$(sed -n "s/^end_time=//p" "$work/backup.info" | python3 -c '
import sys, datetime
s = sys.stdin.read().strip().strip("\x27\"")
try:
    t = datetime.datetime.fromisoformat(s)
    print(int((datetime.datetime.now(datetime.timezone.utc) - t).total_seconds() // 3600))
except Exception:
    print(-1)')
  if [ "$age" -ge 0 ] && [ "$age" -le 50 ]; then echo "  ended ${age}h ago"
  else echo "  FAIL: newest base backup ended ${age}h ago (or end_time unreadable)"; fail=1; fi
  rclone lsf -R --files-only "offsite:cnpg-backups/$basedir" > "$work/base.txt"
  if grep -q '^data\.tar' "$work/base.txt"; then echo "  data: $(grep '^data\.tar' "$work/base.txt" | tr '\n' ' ')"
  else echo "  FAIL: no data.tar* in the base backup"; fail=1; fi
  rclone lsf -R --files-only --format "sp" --separator " " "offsite:cnpg-backups/$server/wals" > "$work/wals.txt"
  if python3 - "$begin" "$endw" "${segsize:-16777216}" "$work/wals.txt" <<'WAL'
import sys, os
begin, end, segsize, listing = sys.argv[1], sys.argv[2], int(sys.argv[3]), sys.argv[4]
import re
# A segment counts only as a complete object: 24 hex digits, optionally a
# compression suffix, never .partial, and not empty.
seg_re = re.compile(r"^([0-9A-F]{24})(\.(gz|bz2|xz|snappy|zst|lz4))?$")
have = set()
for line in open(listing):
    size, _, path = line.strip().partition(" ")
    m = seg_re.match(os.path.basename(path))
    if m and size.isdigit() and int(size) > 0:
        have.add(m.group(1))
per_log = 0x100000000 // segsize
tli = begin[:8]
log, seg = int(begin[8:16], 16), int(begin[16:24], 16)
missing, n = [], 0
while True:
    name = f"{tli}{log:08X}{seg:08X}"
    n += 1
    if name not in have:
        missing.append(name)
    if name == end or n > 100000:
        break
    seg += 1
    if seg >= per_log:
        log, seg = log + 1, 0
print(f"  WAL range: {n} segments, {len(missing)} missing" + (f" (first {missing[0]})" if missing else ""))
sys.exit(1 if missing or end[:8] != tli else 0)
WAL
  then :; else echo "  FAIL: the base backup's WAL range is incomplete"; fail=1; fi
  if [ "$minio_ok" = 1 ]; then
    if rclone check --one-way --size-only "minio:cnpg-backups/$basedir" "offsite:cnpg-backups/$basedir" >"$work/check-base.txt" 2>&1; then
      echo "  base directory matches MinIO file for file"
    else echo "  FAIL: base directory differs from MinIO"; grep -E 'ERROR|differ|not found' "$work/check-base.txt" | head -5; fail=1; fi
  fi
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
# What the key is allowed, from B2 itself: restricted to this bucket, with
# list and write only. The key goes in a header from a file, never argv.
caps=$(python3 - "$work" "$BUCKET" <<'CAPS'
import base64, json, sys, urllib.request, os
work, bucket = sys.argv[1], sys.argv[2]
kid = open(os.path.join(work, "vault_offsite_b2_key_id")).read().strip()
key = open(os.path.join(work, "vault_offsite_b2_key")).read().strip()
req = urllib.request.Request("https://api.backblazeb2.com/b2api/v2/b2_authorize_account",
    headers={"Authorization": "Basic " + base64.b64encode(f"{kid}:{key}".encode()).decode()})
a = json.load(urllib.request.urlopen(req, timeout=30)).get("allowed") or {}
caps = set(a.get("capabilities") or [])
bad = caps & {"deleteFiles", "bypassGovernance", "readFiles", "writeFileRetentions",
              "writeBucketRetentions", "writeBuckets", "deleteBuckets", "writeKeys", "deleteKeys", "shareFiles"}
ok = a.get("bucketName") == bucket and not bad and "writeFiles" in caps
print(("ok" if ok else "bad") + " bucket=" + str(a.get("bucketName")) + " caps=" + ",".join(sorted(caps)))
CAPS
) || caps="bad (authorize failed)"
case "$caps" in
  ok*) echo "capabilities: ${caps#ok } (expected)";;
  *) echo "FAIL: write key ${caps#bad }"; fail=1;;
esac
# The write key can hide files (b2_hide_file is a writeFiles call), and the
# lifecycle rule deletes hidden versions. What protects the copy is the
# bucket's default Object Lock retention (governance, 30 days): a locked
# version cannot be deleted by a lifecycle rule or by a key without
# bypassGovernance. So the canary, uploaded the way the job uploads, must
# carry that retention.
# The job's key has no readFiles, so every rclone call that looks at one
# file by name (copyto, cat, deletefile: they HEAD it first) is refused for
# that reason alone. Upload and delete the way the job does, through a
# directory listing (listFiles), and confirm results with the read key.
cname="$(date -u +%Y%m%dT%H%M%SZ).txt"
canary="key-scope-canary/$cname"
mkdir -p "$work/canary"; echo "offsite-check canary" > "$work/canary/$cname"
listed() { rclone lsf "b2r:$BUCKET/key-scope-canary/" --include "$cname" 2>/dev/null | grep -qx "$cname"; }
rb() { rclone "$@" --retries 1 --low-level-retries 1 >/dev/null 2>&1; }
if rb copy "$work/canary" "b2w:$BUCKET/key-scope-canary/" && listed; then
  echo "write: allowed (expected)"
  # The read key needs readFileRetentions to see retention. The b2 CLI and
  # the raw API nest it differently ({"mode"} or {"value": {"mode"}}):
  # accept either, and require a retain-until at least 29 days out.
  ret=$(B2_ACCOUNT_INFO="$work/b2-account-info" B2_APPLICATION_KEY_ID="$B2_READ_KEY_ID" B2_APPLICATION_KEY="$B2_READ_KEY" \
    b2 file info "b2://$BUCKET/$canary" 2>/dev/null | python3 -c '
import json, sys, time
r = json.load(sys.stdin).get("fileRetention") or {}
r = r.get("value") or r
mode, until = r.get("mode") or "none", r.get("retainUntilTimestamp") or 0
days = (int(until) / 1000 - time.time()) / 86400 if until else 0
print(mode, int(days))' 2>/dev/null || echo "unknown 0")
  mode=${ret% *}; days=${ret#* }
  if { [ "$mode" = governance ] || [ "$mode" = compliance ]; } && [ "$days" -ge 29 ]; then
    echo "retention on a job upload: $mode, $days days left (expected)"
  else echo "FAIL: a job upload has retention '$mode' with $days days left, not the bucket default of 30"; fail=1; fi
  if [ "$(rclone cat "b2w:$BUCKET/$canary" --retries 1 --low-level-retries 1 2>/dev/null)" = "offsite-check canary" ]; then
    echo "FAIL: write key can read"; fail=1
  else echo "read: refused (expected)"; fi
  rb delete "b2w:$BUCKET/key-scope-canary/" --include "$cname" --b2-hard-delete || true  # refused is the expected outcome
  if listed; then echo "permanent delete: refused (expected)"
  else echo "FAIL: write key can permanently delete"; fail=1; fi
  # A moment between the upload and the hide, with margin for clock skew.
  sleep 5; before_hide=$(date -u +%Y-%m-%dT%H:%M:%SZ); sleep 5
  rb delete "b2w:$BUCKET/key-scope-canary/" --include "$cname" || true
  if ! listed; then
    echo "hide: allowed (the version stays under retention; lifecycle cannot delete it before then)"
    # A hidden file is missing from a plain listing, so a plain restore
    # would skip it. The restore path for that case reads the bucket as it
    # was before the hide (--b2-version-at); prove it brings the canary back.
    if [ "$(rclone cat "b2r:$BUCKET/$canary" --b2-version-at "$before_hide" 2>/dev/null)" = "offsite-check canary" ]; then
      echo "  version-aware read (--b2-version-at) returns the hidden canary (expected)"
    else echo "  FAIL: --b2-version-at could not read the hidden canary"; fail=1; fi
  else echo "hide: refused"; fi
else
  echo "FAIL: write key cannot upload (retention, read and delete checks skipped)"; fail=1
fi

# Without MinIO nothing was compared with the originals: a file missing from
# B2 (an older snapshot, WAL outside the newest base) would go unnoticed.
if [ "$fail" != 0 ]; then echo "OFFSITE CHECK FAILED"
elif [ "$minio_ok" = 1 ]; then echo "OFFSITE CHECK OK"
else echo "OFFSITE CHECK OK (restore proof and key scope only; completeness not compared: MinIO unreachable)"; fi
exit "$fail"
