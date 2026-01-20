# Installation Files

This directory contains installation and configuration files for setting up Turing Smart Screen monitors as a system service.

## Files

| File | Description |
|------|-------------|
| `install.sh` | Interactive installer for Arch Linux (also works on other systemd distros) |
| `99-turing-lcd.rules` | Template udev rules for stable device naming |
| `turing-screens.service` | Systemd service unit file |

## Arch Linux Package (Recommended)

Build and install as a native package:

```bash
cd /path/to/go-turing-smart-screen
makepkg -si
```

Then configure your displays:

```bash
# Find USB port paths
udevadm info -q path -n /dev/ttyACM0

# Edit udev rules with your port IDs
sudo nano /etc/udev/rules.d/99-turing-lcd.rules

# Reload and enable
sudo udevadm control --reload-rules && sudo udevadm trigger
sudo systemctl enable --now turing-screens
```

## Quick Install (Without Package)

```bash
# Build and install
cd /path/to/go-turing-smart-screen
go build -mod=vendor -o turing-screens ./cmd/screens
sudo ./install/install.sh
```

## Manual Installation

### 1. udev Rules (Stable Device Names)

The displays use CH340 USB-serial chips which all report the same serial number. To get stable device names that survive reboots, we use udev rules based on physical USB port paths.

```bash
# Find your USB port paths
udevadm info -q path -n /dev/ttyACM0
# Output like: .../usb3/3-9/3-9:1.0/tty/ttyACM0
# The "3-9" part is the port identifier

# Edit the rules file with your port IDs
sudo cp 99-turing-lcd.rules /etc/udev/rules.d/
sudo nano /etc/udev/rules.d/99-turing-lcd.rules

# Reload rules
sudo udevadm control --reload-rules
sudo udevadm trigger

# Verify symlinks were created
ls -la /dev/lcd-*
```

### 2. Systemd Service

```bash
# Copy service file
sudo cp turing-screens.service /etc/systemd/system/

# Edit to match your configuration
sudo nano /etc/systemd/system/turing-screens.service

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable turing-screens
sudo systemctl start turing-screens

# Check status
sudo systemctl status turing-screens
sudo journalctl -u turing-screens -f
```

### 3. Binary Installation

```bash
# Option 1: Build locally
go build -mod=vendor -o /usr/local/bin/turing-screens ./cmd/screens

# Option 2: Download release
curl -fsSL https://github.com/aleksclark/go-turing-smart-screen/releases/latest/download/turing-screens-linux-amd64 \
  -o /usr/local/bin/turing-screens
chmod +x /usr/local/bin/turing-screens
```

## Uninstall

```bash
sudo ./install.sh --uninstall
```

Or manually:

```bash
sudo systemctl stop turing-screens
sudo systemctl disable turing-screens
sudo rm /etc/systemd/system/turing-screens.service
sudo rm /etc/udev/rules.d/99-turing-lcd.rules
sudo rm /usr/local/bin/turing-screens
sudo systemctl daemon-reload
sudo udevadm control --reload-rules
```

## Troubleshooting

### Displays not detected

```bash
# Check if devices are present
ls -la /dev/ttyACM*

# Check USB devices
lsusb | grep -i ch340

# Check dmesg for USB events
dmesg | tail -20
```

### Permission denied

```bash
# Add user to dialout/uucp group
sudo usermod -aG uucp $USER  # Arch
sudo usermod -aG dialout $USER  # Debian/Ubuntu

# Or use the udev rules which set MODE="0666"
```

### Service won't start

```bash
# Check logs
sudo journalctl -u turing-screens -n 50

# Test manually
sudo /usr/local/bin/turing-screens --debug
```

### Symlinks not created

```bash
# Verify udev rules are loaded
udevadm test /dev/ttyACM0

# Check for rule syntax errors
sudo udevadm control --reload-rules 2>&1

# Force re-trigger
sudo udevadm trigger --action=add
```
