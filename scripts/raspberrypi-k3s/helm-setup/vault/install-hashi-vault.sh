helm repo add hashicorp https://helm.releases.hashicorp.com

# Set up Vault Namespace
kubectl create namespace vault

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Install Vault to the vault namespace with HA configuration
helm install vault hashicorp/vault --namespace vault -f "${SCRIPT_DIR}/values.yaml"

# Check if Vault is installed and healthy
VAULT_PODS=$(kubectl get pods -n vault --field-selector=status.phase!=Running -o name)

if [ -z "$VAULT_PODS" ]; then
  echo "Vault is installed and healthy"
else
  echo "Vault is not installed or not healthy"
  exit 1
fi

#configure vault