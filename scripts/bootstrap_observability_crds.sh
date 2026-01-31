#!/bin/bash
set -e

echo "Bootstraping Prometheus CRDs with Server-Side Apply..."

# Base URL for the Prometheus Operator version we are using (0.71.2)
BASE_URL="https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/v0.71.2/example/prometheus-operator-crd"

CRDS=(
  "monitoring.coreos.com_alertmanagerconfigs.yaml"
  "monitoring.coreos.com_alertmanagers.yaml"
  "monitoring.coreos.com_podmonitors.yaml"
  "monitoring.coreos.com_probes.yaml"
  "monitoring.coreos.com_prometheusagents.yaml"
  "monitoring.coreos.com_prometheuses.yaml"
  "monitoring.coreos.com_prometheusrules.yaml"
  "monitoring.coreos.com_scrapeconfigs.yaml"
  "monitoring.coreos.com_servicemonitors.yaml"
  "monitoring.coreos.com_thanosrulers.yaml"
)

for crd in "${CRDS[@]}"; do
  echo "Applying $crd..."
  kubectl apply --server-side --force-conflicts -f "$BASE_URL/$crd"
done

echo "CRDs bootstrapped successfully."
echo "You can now sync the ArgoCD observability application."
