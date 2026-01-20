# go-turing-smart-screen

System monitor displays for Turing Smart Screen USB-C LCD panels, written in Go.

[![CI](https://github.com/aleksclark/go-turing-smart-screen/actions/workflows/ci.yml/badge.svg)](https://github.com/aleksclark/go-turing-smart-screen/actions/workflows/ci.yml)
[![Release](https://github.com/aleksclark/go-turing-smart-screen/actions/workflows/release.yml/badge.svg)](https://github.com/aleksclark/go-turing-smart-screen/releases)

## Features

- **CPU Monitor** - Per-core usage bars, frequency, load averages, temperature
- **RAM Monitor** - Memory/swap usage, top processes with smart aggregation
- **Agent Monitor** - Coding agent status display (reads `~/.agent-status/*.json`)

## Installation

### Arch Linux (PKGBUILD)

Build and install as a native Arch package:

```bash
git clone https://github.com/aleksclark/go-turing-smart-screen.git
cd go-turing-smart-screen
makepkg -si
```

After installation, configure your USB ports:

```bash
# Find USB port paths for each display
udevadm info -q path -n /dev/ttyACM0  # Note port like "3-9"
udevadm info -q path -n /dev/ttyACM1
udevadm info -q path -n /dev/ttyACM2

# Edit udev rules with your port IDs
sudo nano /etc/udev/rules.d/99-turing-lcd.rules

# Reload and start
sudo udevadm control --reload-rules && sudo udevadm trigger
sudo systemctl enable --now turing-screens
```

### Arch Linux (Interactive Installer)

The interactive installer auto-detects devices and configures everything:

```bash
git clone https://github.com/aleksclark/go-turing-smart-screen.git
cd go-turing-smart-screen
go build -mod=vendor -o turing-screens ./cmd/screens
sudo ./install/install.sh
```

The installer will:
1. Detect connected Turing Smart Screen devices
2. Let you assign each physical display to a monitor type (CPU/RAM/Agent)
3. Create udev rules for stable device naming (`/dev/lcd-cpu`, `/dev/lcd-ram`, `/dev/lcd-agent`)
4. Install the binary and systemd service
5. Enable auto-start on boot

```bash
# Manage the service
sudo systemctl status turing-screens
sudo systemctl restart turing-screens
sudo journalctl -u turing-screens -f

# Uninstall
sudo ./install/install.sh --uninstall

# Detect devices only (no install)
sudo ./install/install.sh --detect
```

### Pre-built binaries

Download from [Releases](https://github.com/aleksclark/go-turing-smart-screen/releases).

### From source

```bash
go install github.com/aleksclark/go-turing-smart-screen/cmd/screens@latest
```

### Build locally

```bash
git clone https://github.com/aleksclark/go-turing-smart-screen.git
cd go-turing-smart-screen
go build -mod=vendor -o screens ./cmd/screens
```

### Manual udev setup (other distros)

If you want stable device names without the installer:

```bash
# 1. Find your USB port paths
udevadm info -q path -n /dev/ttyACM0  # Note the port like "3-9", "9-1.1"

# 2. Create udev rules
sudo cp install/99-turing-lcd.rules /etc/udev/rules.d/
sudo nano /etc/udev/rules.d/99-turing-lcd.rules  # Edit KERNELS values

# 3. Reload rules
sudo udevadm control --reload-rules && sudo udevadm trigger

# 4. Verify symlinks
ls -la /dev/lcd-*
```

## Usage

```bash
# Run all monitors (auto-detects /dev/ttyACM0, ACM1, ACM2)
./screens

# Specify ports
./screens --cpu-port /dev/ttyACM0 --ram-port /dev/ttyACM1 --agent-port /dev/ttyACM2

# Windows
./screens.exe --cpu-port COM3 --ram-port COM4 --agent-port COM5

# Adjust brightness (0-100)
./screens --brightness 50

# Disable specific monitors
./screens --no-agent

# Test without hardware
./screens --simulated

# Debug logging
./screens --debug
```

## Supported Hardware

- Turing Smart Screen 3.5" (Rev A protocol)
- UsbPCMonitor 3.5"
- Similar USB-C LCD panels using the same protocol

## Project Structure

```
├── cmd/screens/        # Main binary
├── internal/
│   ├── lcd/            # LCD serial protocol
│   ├── monitor/        # Monitor implementations
│   └── sysinfo/        # System info (via gopsutil)
├── pkg/agentstat/      # Agent status file API (public)
├── install/            # Installation scripts and configs
└── vendor/             # Vendored dependencies
```

## Agent Status Format

The Agent Monitor reads JSON status files from `~/.agent-status/`. 

### Quick Start

```json
{
  "v": 1,
  "agent": "my-agent",
  "instance": "abc123",
  "status": "working",
  "task": "implementing feature",
  "updated": 1737276300
}
```

### Resources

- [Agent Status Reporting Specification](./AGENT_STATUS_REPORTING.md) - Full documentation
- [JSON Schema](./agent-status.schema.json) - Formal schema for validation

### Status Values

| Status | Description |
|--------|-------------|
| `idle` | Waiting for user input |
| `thinking` | Processing/reasoning |
| `working` | Executing tools |
| `waiting` | Waiting for external resource |
| `error` | Encountered an error |
| `done` | Task completed |
| `paused` | Paused by user |

### Validation

The `pkg/agentstat` package provides Go APIs for reading and validating status files:

```go
import "github.com/aleksclark/go-turing-smart-screen/pkg/agentstat"

// Read all active agents
statuses, _ := agentstat.ReadAll(5 * time.Minute)

// Validate a status
status := agentstat.Status{...}
if err := status.Validate(); err != nil {
    log.Printf("invalid: %v", err)
}
```

Validate with CLI tools:

```bash
# ajv-cli
npx ajv validate -s agent-status.schema.json -d ~/.agent-status/*.json

# check-jsonschema
check-jsonschema --schemafile agent-status.schema.json ~/.agent-status/*.json
```

## License

MIT
