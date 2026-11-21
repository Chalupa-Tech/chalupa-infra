#!/bin/bash
# Runs scripts/longhorn/mkfs-guard-test.sh in a throwaway pod on the Longhorn image,
# with the scratch files on an emptyDir (node disk, O_DIRECT works there). Needs kubectl.
#   bash scripts/longhorn/run-mkfs-guard-test.sh
set -u
cd "$(dirname "$0")/../.." || exit 1
IMAGE=longhornio/longhorn-manager:v1.7.2
POD=mkfs-guard-test-$$
overrides=$(cat <<EOF
{"spec":{"restartPolicy":"Never","nodeSelector":{"compute-class":"pi5"},
 "containers":[{"name":"t","image":"$IMAGE","command":["bash","-c","cat > /w/guard.sh; GUARD=/w/guard.sh WORK=/w bash /dev/fd/3 3< <(echo \"\$TEST_B64\" | base64 -d)"],
  "stdin":true,"stdinOnce":true,
  "env":[{"name":"TEST_B64","value":"$(base64 < scripts/longhorn/mkfs-guard-test.sh | tr -d '\n')"}],
  "volumeMounts":[{"name":"w","mountPath":"/w"}],
  "resources":{"requests":{"cpu":"50m","memory":"64Mi"},"limits":{"memory":"256Mi"}}}],
 "volumes":[{"name":"w","emptyDir":{}}]}}
EOF
)
kubectl -n default run "$POD" --image="$IMAGE" --restart=Never -i --rm --quiet \
  --overrides="$overrides" < k8s/platform/longhorn/files/mkfs-guard.sh
