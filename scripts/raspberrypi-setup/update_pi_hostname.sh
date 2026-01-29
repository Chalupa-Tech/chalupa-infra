#!/bin/bash

# Check if running as root
if [ "$EUID" -ne 0 ]; then
  echo "Error: Please run as root (use sudo)."
  exit 1
fi

# Check for argument
if [ -z "$1" ]; then
  echo "Usage: $0 <new_hostname>"
  exit 1
fi

NEW_HOSTNAME="$1"
CURRENT_HOSTNAME=$(hostname)

if [ ! -f /etc/hostname ]; then
    echo "Error: /etc/hostname not found. Are you running this on a Raspberry Pi?"
    exit 1
fi

echo "Updating hostname from '$CURRENT_HOSTNAME' to '$NEW_HOSTNAME'..."

# 1. Update /etc/hostname
echo "$NEW_HOSTNAME" > /etc/hostname

# 2. Update /etc/hosts
# We look for the line 127.0.1.1 <old_hostname> and replace the hostname part
if grep -q "127.0.1.1.*$CURRENT_HOSTNAME" /etc/hosts; then
    sed -i "s/127.0.1.1.*$CURRENT_HOSTNAME/127.0.1.1\t$NEW_HOSTNAME/g" /etc/hosts
else
    # Fallback: Just ensure the new hostname is mapped if the old one wasn't found where expected
    echo "127.0.1.1\t$NEW_HOSTNAME" >> /etc/hosts
fi

# 3. Use hostnamectl if available (standard on Raspberry Pi OS / systemd)
if command -v hostnamectl &> /dev/null; then
    hostnamectl set-hostname "$NEW_HOSTNAME"
fi

# 4. Restart avahi-daemon if active (handles mDNS / .local resolution)
if systemctl is-active --quiet avahi-daemon; then
    echo "Restarting avahi-daemon to update mDNS..."
    systemctl restart avahi-daemon
fi

# 5. Force DHCP release and renew to update DNS server (Router)
echo "Forcing DHCP release/renew to register new hostname with the router/DNS..."

# Raspberry Pi OS default (dhcpcd)
if command -v dhcpcd &> /dev/null; then
    echo "Releasing and renewing DHCP lease via dhcpcd..."
    dhcpcd -k  # Release
    dhcpcd -n  # Reload/Renew
    
# Debian/Ubuntu default (dhclient)
elif command -v dhclient &> /dev/null; then
    echo "Releasing and renewing DHCP lease via dhclient..."
    dhclient -r # Release
    dhclient    # Renew
fi

# 6. Handle cloud-init (Update source config instead of disabling)
CLOUD_INIT_FOUND=0

# List of common locations for cloud-init config on Pi/Ubuntu
POSSIBLE_FILES=(
    "/boot/firmware/user-data"
    "/boot/firmware/meta-data"
    "/boot/user-data"
    "/boot/meta-data"
    "/var/lib/cloud/seed/nocloud-net/user-data"
    "/var/lib/cloud/seed/nocloud-net/meta-data"
)

for file in "${POSSIBLE_FILES[@]}"; do
    if [ -f "$file" ]; then
        echo "Found cloud-init source: $file"
        
        # Update 'hostname: ...'
        if grep -q "^hostname:" "$file"; then
            sed -i "s/^hostname:.*/hostname: $NEW_HOSTNAME/" "$file"
            echo "  Updated 'hostname' entry."
            CLOUD_INIT_FOUND=1
        fi
        
        # Update 'local-hostname: ...'
        if grep -q "^local-hostname:" "$file"; then
            sed -i "s/^local-hostname:.*/local-hostname: $NEW_HOSTNAME/" "$file"
            echo "  Updated 'local-hostname' entry."
            CLOUD_INIT_FOUND=1
        fi
    fi
done

if [ "$CLOUD_INIT_FOUND" -eq 1 ]; then
    echo "Cloud-init configuration updated. Clearing cache to apply on reboot..."
    if command -v cloud-init &> /dev/null; then
        cloud-init clean
    else
        rm -rf /var/lib/cloud/*
    fi
elif [ -f "/etc/cloud/cloud.cfg" ]; then
    # Fallback if we have cloud-init but couldn't find the source file
    echo "Warning: Cloud-init is installed but source config not found."
    echo "Enabling 'preserve_hostname' to prevent reset..."
    if grep -q "preserve_hostname: false" /etc/cloud/cloud.cfg; then
        sed -i 's/preserve_hostname: false/preserve_hostname: true/g' /etc/cloud/cloud.cfg
    elif ! grep -q "preserve_hostname" /etc/cloud/cloud.cfg; then
        echo "preserve_hostname: true" >> /etc/cloud/cloud.cfg
    fi
fi

echo "-------------------------------------------------------"
echo "Hostname updated to: $NEW_HOSTNAME"
echo "PLEASE REBOOT NOW to ensure all services (like Wifi/Networking) pick up the change."
echo "Command: sudo reboot"
