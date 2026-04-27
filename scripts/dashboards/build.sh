#!/usr/bin/env bash
# Render scripts/dashboards/cmd/paper-trader into the committed JSON.
# CI: `go` is available (chalupa-tech is a Go shop).
# Local: same command works without setup; `go run` resolves deps via go.mod.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="${REPO_ROOT}/k8s/apps/schwab/go-paper-trader/files/paper-trading.json"

cd "${REPO_ROOT}/scripts/dashboards"
go run ./cmd/paper-trader > "${OUT}.tmp"
mv "${OUT}.tmp" "${OUT}"
