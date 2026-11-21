#!/bin/bash
# Tests for k8s/platform/longhorn/files/mkfs-guard.sh.
#
# Runs inside the longhornio/longhorn-manager image (the image the guard runs in),
# against regular files in a directory that supports O_DIRECT. Every format call
# passes -n, so the real mke2fs never writes even if the guard wrongly lets it through.
#
#   GUARD=/path/to/mkfs-guard.sh WORK=/scratch bash mkfs-guard-test.sh
set -u
GUARD=${GUARD:?path to mkfs-guard.sh}
WORK=${WORK:?scratch directory}
bin=$WORK/bin
mkdir -p "$bin"
cp "$GUARD" "$bin/mkfs.ext4"
cp "$GUARD" "$bin/mkfs.nosuch"
chmod 0755 "$bin/mkfs.ext4" "$bin/mkfs.nosuch"

fail=0
check() { # name, expected exit, command...
  local name=$1 want=$2 got
  shift 2
  "$@" >"$WORK/out" 2>&1
  got=$?
  if [ "$got" = "$want" ]; then
    echo "ok   $name (exit $got)"
  else
    echo "FAIL $name: want exit $want, got $got"
    sed 's/^/     /' "$WORK/out"
    fail=1
    return 1
  fi
}
img() { # name, MiB
  rm -f "$WORK/$1"
  dd if=/dev/zero of="$WORK/$1" bs=1M count=0 seek="$2" status=none
}
poke() { # name, byte offset
  printf 'x' | dd of="$WORK/$1" bs=1 seek="$2" conv=notrunc status=none
}

export MKFS_GUARD_ACTUAL_SIZE=0

img zeros 64
check "a never-written volume that reads as zeros is formatted" 0 "$bin/mkfs.ext4" -n -F "$WORK/zeros"

img small 10
check "a fresh 10 MiB volume is formatted (read capped to its size)" 0 "$bin/mkfs.ext4" -n -F "$WORK/small"

MKFS_GUARD_ACTUAL_SIZE=1158574080 check "Longhorn actualSize > 0 is refused even when the device reads zeros" 1 "$bin/mkfs.ext4" -n -F "$WORK/zeros"
MKFS_GUARD_ACTUAL_SIZE=abc check "a non-numeric actualSize is refused" 1 "$bin/mkfs.ext4" -n -F "$WORK/zeros"
MKFS_GUARD_ACTUAL_SIZE= check "an empty actualSize is refused" 1 "$bin/mkfs.ext4" -n -F "$WORK/zeros"
(unset MKFS_GUARD_ACTUAL_SIZE; check "a non-Longhorn device with no actualSize is refused" 1 "$bin/mkfs.ext4" -n -F "$WORK/zeros") || fail=1

img sb 64
poke sb 1080
check "data in the first 16 MiB is refused" 1 "$bin/mkfs.ext4" -n -F "$WORK/sb"

img late 64
poke late $((30 * 1048576 + 4096))
check "data in a sampled MiB (offset 30 MiB) is refused" 1 "$bin/mkfs.ext4" -n -F "$WORK/late"

img last 64
poke last $((60 * 1048576 + 7))
check "data in the last sampled MiB (offset 60 MiB) is refused" 1 "$bin/mkfs.ext4" -n -F "$WORK/last"

img gap 64
poke gap $((20 * 1048576 + 512 * 1024))
MKFS_GUARD_ACTUAL_SIZE=4096 check "data between samples is refused by actualSize" 1 "$bin/mkfs.ext4" -n -F "$WORK/gap"

img tiny 0
check "an empty device is refused" 1 "$bin/mkfs.ext4" -n -F "$WORK/tiny"

check "a missing device is refused" 1 "$bin/mkfs.ext4" -n -F "$WORK/does-not-exist"
mkdir -p "$WORK/adir"
check "a directory is refused" 1 "$bin/mkfs.ext4" -n -F "$WORK/adir"
check "an unknown mkfs is refused" 1 "$bin/mkfs.nosuch" -n "$WORK/zeros"

if [ "$fail" = 0 ]; then echo PASS; else echo FAIL; fi
exit "$fail"
