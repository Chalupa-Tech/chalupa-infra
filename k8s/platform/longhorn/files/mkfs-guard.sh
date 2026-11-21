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
# This formats only a device that reads back cleanly as all zeros (a new Longhorn
# volume is sparse and reads zeros): the first 16 MiB and 1 MiB at 16 points
# spread over the device. A read error, a short read or any non-zero byte refuses
# with exit 1, so the mount fails visibly instead of formatting.
# To put a fresh filesystem on a volume on purpose, give the workload a new PVC.
set -uo pipefail

fs=$(basename "$0")
real=/usr/sbin/$fs
dev=${!#}
say() { echo "mkfs-guard: $*" >&2; }

[ -x "$real" ] || { say "refusing: $real not found"; exit 1; }
if [ -b "$dev" ]; then
  size=$(blockdev --getsize64 "$dev") || { say "refusing: cannot read the size of $dev"; exit 1; }
elif [ -f "$dev" ]; then
  size=$(stat -c %s "$dev") || { say "refusing: cannot read the size of $dev"; exit 1; }
else
  say "refusing: $dev is not a block device"
  exit 1
fi

mib=$((size / 1048576))
checks="0:16"
for i in $(seq 1 16); do
  checks="$checks $((mib * i / 17)):1"
done

for c in $checks; do
  start=${c%%:*}
  count=${c##*:}
  if ! dd if="$dev" bs=1M skip="$start" count="$count" iflag=direct status=none \
      | cmp -s -n $((count * 1048576)) - /dev/zero; then
    say "refusing to format $dev: $count MiB at offset $start MiB is not readable zeros (I/O error, short read or existing data)"
    exit 1
  fi
done

say "$dev reads as zeros; running $real"
exec "$real" "$@"
