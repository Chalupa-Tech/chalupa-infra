#!/bin/bash

# Configuration
NVME_DEVICE="/dev/nvme0n1"
BOOT_MOUNT="/boot/firmware"
LOG_FILE="nvme_migration.log"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}" | tee -a "$LOG_FILE"
}

error() {
    echo -e "${RED}[ERROR] $1${NC}" | tee -a "$LOG_FILE"
    exit 1
}

warn() {
    echo -e "${YELLOW}[WARNING] $1${NC}" | tee -a "$LOG_FILE"
}

# 1. Check Root
if [ "$EUID" -ne 0 ]; then
  error "Please run as root (sudo)."
fi

# 2. Check Device
# 2. Check Device with Auto-Enablement of PCIe
if [ ! -b "$NVME_DEVICE" ]; then
    warn "NVMe device $NVME_DEVICE not found. Checking if PCIe is enabled..."
    
    CONFIG_TXT=""
    if [ -f "/boot/firmware/config.txt" ]; then
        CONFIG_TXT="/boot/firmware/config.txt"
    elif [ -f "/boot/config.txt" ]; then
        CONFIG_TXT="/boot/config.txt"
    fi

    if [ -n "$CONFIG_TXT" ]; then
        if grep -q "dtparam=pciex1" "$CONFIG_TXT"; then
             error "PCIe is enabled (dtparam=pciex1 found in $CONFIG_TXT) but $NVME_DEVICE is still not visible. \n       Please check hardware connection (Ribbon cable direction, HAT+ seating)."
        else
             warn "PCIe is NOT enabled. Enabling 'dtparam=pciex1' in $CONFIG_TXT..."
             
             # Backup
             cp "$CONFIG_TXT" "${CONFIG_TXT}.bak"
             
             # Add config
             echo "" >> "$CONFIG_TXT"
             echo "# Enable PCIe for M.2 HAT+" >> "$CONFIG_TXT"
             echo "dtparam=pciex1" >> "$CONFIG_TXT"
             
             warn "PCIe enabled. YOU MUST REBOOT FOR THIS TO TAKE EFFECT."
             warn "Please reboot (sudo reboot) and run this script again."
             exit 0
        fi
    else
        error "Could not find config.txt to enable PCIe. Please enable 'dtparam=pciex1' manually."
    fi
fi

# 3. Check OS / Boot Mount
if [ ! -d "$BOOT_MOUNT" ]; then
    error "Boot directory $BOOT_MOUNT not found. This script expects Raspberry Pi OS Bookworm or newer."
fi

# 4. Confirmation
echo -e "${RED}WARNING: THIS WILL WIPE ALL DATA ON $NVME_DEVICE${NC}"
echo -e "${RED}WARNING: MAKE SURE YOU HAVE A BACKUP${NC}"
read -p "Type 'YES' to continue: " confirm
if [ "$confirm" != "YES" ]; then
    error "Aborted by user."
fi

log "Starting migration..."

# 5. Install Dependencies
log "Installing Dependencies (rsync)..."
apt-get update && apt-get install -y rsync

# 6. Partitioning
log "Partitioning $NVME_DEVICE..."
# Unmount if mounted
umount "${NVME_DEVICE}"* 2>/dev/null || true

# wipefs
wipefs -a "$NVME_DEVICE"

# Create MBR partition table (Pi compatible)
# p1: 512MB FAT32 (Boot)
# p2: Remaining Linux (Root)
sfdisk "$NVME_DEVICE" <<EOF
label: dos
unit: sectors
size=1048576, type=c, bootable
type=83
EOF

partprobe "$NVME_DEVICE"
sleep 2

# 7. Formatting
log "Formatting partitions..."
APP_PART="${NVME_DEVICE}p1" # Using p1/p2 convention for NVMe
ROOT_PART="${NVME_DEVICE}p2"

mkfs.vfat -n "BOOT-NVME" "$APP_PART"
mkfs.ext4 -L "ROOT-NVME" -F "$ROOT_PART"

# 8. Mounting
log "Mounting new partitions..."
mkdir -p /mnt/nvme_boot /mnt/nvme_root
mount "$APP_PART" /mnt/nvme_boot
mount "$ROOT_PART" /mnt/nvme_root

# 9. Cloning
log "Cloning Root Filesystem (this may take a while)..."
# Exclude pseudo-filesystems and the new mount points
rsync -axHAWX --info=progress2 --exclude={"/dev/*","/proc/*","/sys/*","/tmp/*","/run/*","/mnt/*","/media/*","/lost+found"} / /mnt/nvme_root/

log "Cloning Boot Filesystem..."
rsync -axHAWX --info=progress2 "$BOOT_MOUNT/" /mnt/nvme_boot/

# 10. Updating Configuration (fstab & cmdline.txt)
log "Updating Configuration..."

# Get UUIDs (using blkid -o value -s PARTUUID)
NEW_BOOT_UUID=$(blkid -o value -s PARTUUID "$APP_PART")
NEW_ROOT_UUID=$(blkid -o value -s PARTUUID "$ROOT_PART")

if [ -z "$NEW_BOOT_UUID" ] || [ -z "$NEW_ROOT_UUID" ]; then
    error "Failed to get PARTUUIDs."
fi

log "New Boot UUID: $NEW_BOOT_UUID"
log "New Root UUID: $NEW_ROOT_UUID"

# Update fstab
FSTAB="/mnt/nvme_root/etc/fstab"
cp "$FSTAB" "${FSTAB}.bak"

# Replace the UUID/PARTUUID for /boot/firmware
sed -i "s/PARTUUID=[a-zA-Z0-9-]\+\s\+\/boot\/firmware/PARTUUID=$NEW_BOOT_UUID\t\/boot\/firmware/g" "$FSTAB"
# Replace the UUID/PARTUUID for /
sed -i "s/PARTUUID=[a-zA-Z0-9-]\+\s\+\/\s/PARTUUID=$NEW_ROOT_UUID\t\/\t/g" "$FSTAB"

# Update cmdline.txt
CMDLINE="/mnt/nvme_boot/cmdline.txt"
cp "$CMDLINE" "${CMDLINE}.bak"
# Replace root=PARTUUID=... with new UUID
sed -i "s/root=PARTUUID=[a-zA-Z0-9-]\+/root=PARTUUID=$NEW_ROOT_UUID/g" "$CMDLINE"

# 11. Bootloader Configuration
log "Updating Bootloader Order (NVMe Priority)..."

# Ensure PCIE_PROBE=1 is set for NVMe booting
EEPROM_CONFIG=$(rpi-eeprom-config)
if ! echo "$EEPROM_CONFIG" | grep -q "PCIE_PROBE=1"; then
    log "Adding PCIE_PROBE=1 to EEPROM config..."
    # We append it. rpi-eeprom-config edit is interactive, so we use apply
    echo "PCIE_PROBE=1" >> /tmp/eeprom_config.txt
else 
    echo "$EEPROM_CONFIG" > /tmp/eeprom_config.txt
fi

# Set BOOT_ORDER=0xf416 (NVMe first (6), then SD (1), then USB (4), then Restart (f))
# Default is often 0xf41
sed -i 's/^BOOT_ORDER=.*/BOOT_ORDER=0xf416/' /tmp/eeprom_config.txt
if ! grep -q "BOOT_ORDER" /tmp/eeprom_config.txt; then
    echo "BOOT_ORDER=0xf416" >> /tmp/eeprom_config.txt
fi

# Apply config
rpi-eeprom-config --apply /tmp/eeprom_config.txt
rm /tmp/eeprom_config.txt

# 12. Cleanup
log "Unmounting..."
umount /mnt/nvme_boot
umount /mnt/nvme_root

log "Migration Complete!"
warn "Please reboot to test. If successful, the system should boot from NVMe."
warn "You can verify by running 'lsblk' after reboot and checking if '/' is mounted on $NVME_DEVICE."
