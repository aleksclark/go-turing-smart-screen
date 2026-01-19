# go-turing-smart-screen

System monitor displays for Turing Smart Screen USB-C LCD panels, written in Go.

[![CI](https://github.com/aleksclark/go-turing-smart-screen/actions/workflows/ci.yml/badge.svg)](https://github.com/aleksclark/go-turing-smart-screen/actions/workflows/ci.yml)
[![Release](https://github.com/aleksclark/go-turing-smart-screen/actions/workflows/release.yml/badge.svg)](https://github.com/aleksclark/go-turing-smart-screen/releases)

## Features

- **CPU Monitor** - Per-core usage bars, frequency, load averages, temperature
- **RAM Monitor** - Memory/swap usage, top processes with smart aggregation
- **Agent Monitor** - Coding agent status display (reads `~/.agent-status/*.json`)

## Installation

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
└── vendor/             # Vendored dependencies
```

## Agent Status Format

The Agent Monitor reads JSON status files from `~/.agent-status/`. See the [Agent Status Reporting Specification](https://github.com/aleksclark/go-turing-smart-screen/blob/main/AGENT_STATUS_REPORTING.md) for details.

## License

MIT
