#!/bin/bash
# k3s-full-cleanup.sh
# Uninstalls K3s and cleans up remaining configuration and data.

echo "### Stopping K3s services..."
sudo systemctl stop k3s 2>/dev/null
sudo systemctl stop k3s-agent 2>/dev/null

echo "### Running official uninstall scripts..."
if [ -f /usr/local/bin/k3s-uninstall.sh ]; then
    echo "Running /usr/local/bin/k3s-uninstall.sh"
    sudo /usr/local/bin/k3s-uninstall.sh
elif [ -f /usr/local/bin/k3s-agent-uninstall.sh ]; then
    echo "Running /usr/local/bin/k3s-agent-uninstall.sh"
    sudo /usr/local/bin/k3s-agent-uninstall.sh
else
    echo "No official uninstall script found. Proceeding with manual cleanup."
fi

echo "### Removing K3s/Kubernetes directories..."
sudo rm -rf /etc/rancher/k3s
sudo rm -rf /var/lib/rancher/k3s
sudo rm -rf /run/k3s
sudo rm -rf /var/lib/kubelet
sudo rm -rf /etc/kubernetes
sudo rm -rf /var/lib/cni/
sudo rm -rf /etc/cni/net.d/

echo "### Cleaning up networking interfaces..."
# Delete CNI and Flannel bridges
sudo ip link delete cni0 2>/dev/null
sudo ip link delete flannel.1 2>/dev/null

# Clean up any leftover containerd mounts
if command -v containerd &> /dev/null; then
  echo "### Cleaning up containerd mounts..."
  cat /proc/mounts | grep /var/lib/rancher/k3s/storage | awk '{print $2}' | xargs -r sudo umount
fi

echo "### K3s cleanup complete."