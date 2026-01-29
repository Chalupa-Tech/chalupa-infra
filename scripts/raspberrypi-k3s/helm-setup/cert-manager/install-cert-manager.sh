#!/bin/bash

helm repo add jetstack https://charts.jetstack.io --force-update
helm repo update

helm install cert-manager jetstack/cert-manager \
   --namespace cert-manager \
   --create-namespace \
   --set installCRDs=true

# Check if CertManager is installed and healthy
CERT_MANAGER_PODS=$(kubectl get pods -n cert-manager --field-selector=status.phase!=Running -o name)

if [ -z "$CERT_MANAGER_PODS" ]; then
  echo "CertManager is installed and healthy"
else
  echo "CertManager is not installed or not healthy"
  exit 1
fi

kubectl apply -f certissuer-crd.yaml

echo "Waiting for clusterissuer to be ready..."
sleep 10

echo "Checking the clusterissuer... Look for ACMEAccountRegistered"
kubectl describe clusterissuer letsencrypt-prod