#!/usr/bin/env bash
# Render every dashboard binary in scripts/dashboards/cmd/ into its
# committed JSON. Each binary corresponds to one Grafana dashboard;
# its output path is decided here.
#
# CI: `go` is available (chalupa-tech is a Go shop).
# Local: same command works without setup; `go run` resolves deps via
# go.mod. macOS ships /bin/bash 3.2, so we avoid `declare -A`.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# binary:committed-JSON-path-relative-to-REPO_ROOT
RENDERS=(
  "paper-trader:k8s/apps/schwab/go-paper-trader/files/paper-trading.json"
  "schwab-feed:k8s/apps/schwab/go-schwab-feed/files/schwab-feed.json"
)

cd "${REPO_ROOT}/scripts/dashboards"
for entry in "${RENDERS[@]}"; do
  bin="${entry%%:*}"
  rel="${entry#*:}"
  out="${REPO_ROOT}/${rel}"
  echo "render ${bin} -> ${rel}"
  go run "./cmd/${bin}" > "${out}.tmp"
  mv "${out}.tmp" "${out}"
done
