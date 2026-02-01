#!/bin/bash
set -e

# bootstrap_registry_host.sh
# Purpose: Provisions the Registry TLS certificates to the Host Filesystem on the Admin Node.
# Usage: sudo ./bootstrap_registry_host.sh

if [ "$EUID" -ne 0 ]; then
  echo "Error: Please run as root (use sudo)."
  exit 1
fi

echo "=== Private Registry Host Bootstrap ==="

# 1. Define Paths
REGISTRY_CERT_DIR="/var/lib/chalupa-registry/certs"
DATA_DIR="/opt/local-path-provisioner/registry"

# 2. Check for artifacts
if [ ! -f "./server.key" ] || [ ! -f "./server.crt" ]; then
    echo "Error: ./server.key or ./server.crt not found in current directory."
    echo "Please ensure the Bootstrap Bundle is present."
    exit 1
fi

# 3. Provision Certs
echo "Provisioning Certificates to $REGISTRY_CERT_DIR..."
mkdir -p "$REGISTRY_CERT_DIR"
cp ./server.crt "$REGISTRY_CERT_DIR/tls.crt"
cp ./server.key "$REGISTRY_CERT_DIR/tls.key"

# Secure Keys (Root RW only)
chmod 600 "$REGISTRY_CERT_DIR/tls.key"
chmod 644 "$REGISTRY_CERT_DIR/tls.crt"

# 4. Create Data Directory (Optional, ensures permission)
echo "Ensuring Data Directory $DATA_DIR exists..."
mkdir -p "$DATA_DIR"
# Registry runs as root in container (usually), or 1000. 
# HostPath permissions can be tricky. permissive for bootstrap.
chmod 755 "$DATA_DIR"

echo "✅ Registry Host Configuration Complete."
echo "You can now push 'chalupa-infra' to allow ArgoCD to deploy the Registry."
