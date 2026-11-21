#!/bin/bash
# mkfs guard (chalupa-infra, platform phase-97).
#
# Mounted over /usr/local/sbin/mkfs.ext4 and /usr/local/sbin/mkfs.xfs in the
# longhorn-csi-plugin container, where /usr/local/sbin comes first on PATH, so
# Longhorn's NodeStageVolume (mount-utils FormatAndMount) runs this instead of
# the real mkfs. mount-utils formats whenever `blkid -p` finds no filesystem, and
# a device that returns I/O errors reads as having none: on 2026-11-17 that ran
# `mkfs.ext4 -F` against two volumes holding data.
#
# Two checks, both must pass before the real mkfs runs:
# 1. Longhorn's own record: the Volume's status.actualSize must be 0, i.e. nothing
#    has ever been written to it. This does not depend on reading the device.
# 2. The device reads back cleanly as zeros: its first 16 MiB (or all of it, if
#    smaller) and 1 MiB at 16 points spread over it. A read error, short read or
#    non-zero byte refuses.
# Anything else exits 1, so the mount fails visibly instead of formatting.
# To put a fresh filesystem on a volume on purpose, give the workload a new PVC.
# Encrypted Longhorn volumes are not supported (none exist here): their filesystem
# goes on a /dev/mapper device, which this refuses.
#
# MKFS_GUARD_ACTUAL_SIZE, if set, replaces the Longhorn lookup (tests only).
set -uo pipefail

fs=$(basename "$0")
real=/usr/sbin/$fs
dev=${!#}
say() { echo "mkfs-guard: $*" >&2; }
refuse() { say "refusing to format $dev: $*"; exit 1; }

actual_size() { # longhorn volume name -> status.actualSize in bytes
  if [ -n "${MKFS_GUARD_ACTUAL_SIZE+set}" ]; then
    echo "$MKFS_GUARD_ACTUAL_SIZE"
    return 0
  fi
  local sa=/var/run/secrets/kubernetes.io/serviceaccount ns
  ns=$(cat "$sa/namespace") || return 1
  curl -sSf --max-time 10 --cacert "$sa/ca.crt" -H "Authorization: Bearer $(cat "$sa/token")" \
    "https://kubernetes.default.svc/apis/longhorn.io/v1beta2/namespaces/$ns/volumes/$1" \
    | python3 -c '
import json, sys
v = json.load(sys.stdin)
size = (v.get("status") or {}).get("actualSize") if v.get("kind") == "Volume" else None
if size is None:
    sys.exit(1)
print(int(size))'
}

[ -x "$real" ] || refuse "$real not found"

case "$dev" in
  /dev/longhorn/*) ;;
  *) [ -n "${MKFS_GUARD_ACTUAL_SIZE+set}" ] || refuse "not a Longhorn device" ;;
esac
vol=$(basename "$dev")
actual=$(actual_size "$vol") || refuse "cannot read Longhorn's actualSize for volume $vol"
case "$actual" in
  ''|*[!0-9]*) refuse "Longhorn's actualSize for $vol is not a number: '$actual'" ;;
esac
[ "$actual" -eq 0 ] || refuse "Longhorn volume $vol has actualSize $actual bytes: it has held data"

if [ -b "$dev" ]; then
  size=$(blockdev --getsize64 "$dev") || refuse "cannot read its size"
elif [ -f "$dev" ]; then
  size=$(stat -c %s "$dev") || refuse "cannot read its size"
else
  refuse "not a block device"
fi
mib=$((size / 1048576))
[ "$mib" -ge 1 ] || refuse "smaller than 1 MiB ($size bytes)"

first=$((mib < 16 ? mib : 16))
checks="0:$first"
if [ "$mib" -gt 16 ]; then
  for i in $(seq 1 16); do
    checks="$checks $((mib * i / 17)):1"
  done
fi
for c in $checks; do
  start=${c%%:*}
  count=${c##*:}
  if ! dd if="$dev" bs=1M skip="$start" count="$count" iflag=direct status=none \
      | cmp -s -n $((count * 1048576)) - /dev/zero; then
    refuse "$count MiB at offset $start MiB is not readable zeros (I/O error, short read or existing data)"
  fi
done

say "$dev: Longhorn actualSize 0 and reads as zeros; running $real"
exec "$real" "$@"
