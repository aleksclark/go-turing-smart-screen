# Agent Guide for go-turing-smart-screen

System monitor displays for Turing Smart Screen USB-C LCD panels, written in Go. This is a hardware interface project that renders system information (CPU, RAM, agent status) to physical 3.5" LCD displays via serial communication.

## Project Overview

**Purpose**: Display real-time system information on Turing Smart Screen USB-C LCD panels  
**Language**: Go 1.22  
**Target Platform**: Linux (primary), Windows, macOS  
**Hardware**: Turing Smart Screen 3.5" Rev A protocol displays

### What This Does

- Monitors system resources (CPU per-core, RAM, processes)
- Displays coding agent status by reading `~/.agent-status/*.json` files
- Communicates with LCD panels via USB serial (typically `/dev/ttyACM0-2`)
- Supports DPMS sleep detection to turn off displays when monitors sleep
- Designed to run as a systemd service on Linux

## Essential Commands

### Building

```bash
# Standard build
go build -mod=vendor -o screens ./cmd/screens

# Build with optimizations (as used in PKGBUILD)
go build -mod=vendor -ldflags="-s -w" -o turing-screens ./cmd/screens

# Build for different platforms
GOOS=windows GOARCH=amd64 go build -mod=vendor -o screens.exe ./cmd/screens
GOOS=darwin GOARCH=arm64 go build -mod=vendor -o screens ./cmd/screens
```

### Testing

```bash
# Run tests
go test -mod=vendor -v ./...

# Run tests for specific package
go test -mod=vendor -v ./pkg/agentstat
go test -mod=vendor -v ./internal/monitor

# Run with coverage
go test -mod=vendor -v -cover ./...
```

### Running

```bash
# Run with simulated displays (no hardware needed)
./screens --simulated

# Run with specific ports
./screens --cpu-port /dev/ttyACM0 --ram-port /dev/ttyACM1 --agent-port /dev/ttyACM2

# Run with custom intervals and brightness
./screens --cpu-interval 500ms --ram-interval 1s --brightness 50

# Debug mode
./screens --debug

# Disable specific monitors
./screens --no-agent --no-ram
```

### Linting

```bash
# Run golangci-lint (used in CI)
golangci-lint run --timeout=5m

# Auto-fix issues where possible
golangci-lint run --fix
```

### Dependency Management

```bash
# Dependencies are vendored - always use -mod=vendor flag
go mod verify
go mod vendor

# After modifying go.mod, re-vendor
go mod tidy
go mod vendor
git add vendor/
```

### Package Building

```bash
# Build Arch Linux package
makepkg -si

# Clean build artifacts
makepkg -c
```

## Project Structure

```
├── cmd/screens/               # Main binary entry point
│   └── main.go                # CLI flags, monitor orchestration, signal handling
├── internal/                  # Private packages
│   ├── lcd/                   # LCD serial communication (Rev A protocol)
│   │   └── lcd.go             # Display, Simulated types, serial commands
│   ├── monitor/               # Monitor implementations
│   │   ├── base.go            # Base monitor type, rendering helpers
│   │   ├── cpu.go             # CPU usage monitor
│   │   ├── ram.go             # RAM/process monitor
│   │   └── agent.go           # Agent status monitor
│   ├── sysinfo/               # System information gathering
│   │   └── sysinfo.go         # Wrapper around gopsutil
│   └── dpms/                  # DPMS sleep detection (Linux DRM)
│       └── dpms.go            # Monitor power state via /sys/class/drm
├── pkg/                       # Public packages
│   └── agentstat/             # Agent status file API
│       ├── agentstat.go       # Read/validate ~/.agent-status/*.json
│       └── agentstat_test.go  # Comprehensive validation tests
├── install/                   # Installation resources
│   ├── install.sh             # Interactive installer script
│   ├── turing-screens.service # systemd service unit
│   └── 99-turing-lcd.rules    # udev rules template
├── vendor/                    # Vendored dependencies (committed)
├── go.mod                     # Go module definition
├── PKGBUILD                   # Arch Linux package build script
├── AGENT_STATUS_REPORTING.md  # Agent status protocol spec
└── agent-status.schema.json   # JSON schema for validation
```

### Key Files

- **cmd/screens/main.go**: Entry point, flag parsing, monitor lifecycle management
- **internal/lcd/lcd.go**: Low-level serial communication with LCD hardware
- **internal/monitor/base.go**: Shared rendering primitives (bars, text, colors)
- **pkg/agentstat/agentstat.go**: Public API for agent status files (reusable)

## Code Organization

### Package Hierarchy

```
internal/lcd         → Low-level hardware protocol
internal/sysinfo     → System data gathering
internal/monitor     → High-level display logic
pkg/agentstat        → Public API (importable by other projects)
cmd/screens          → CLI application
```

### Monitor Interface

All monitors implement this interface (internal/monitor/base.go:111-115):

```go
type Monitor interface {
    Name() string
    Run() error  // Blocking loop, exits when Stop() called
    Stop()       // Signal monitor to stop
}
```

Each monitor:
- Embeds `*Base` for common functionality
- Manages its own update loop via ticker
- Renders to an image buffer, then sends to LCD
- Uses region-based updates for efficiency

### LCD Abstraction

`lcd.Screen` interface (internal/lcd/lcd.go):
- `Display` - real hardware via serial port
- `Simulated` - no-op implementation for testing

Both implement: `DrawImage()`, `ScreenOn()`, `ScreenOff()`, `SetBrightness()`, `Width()`, `Height()`, `Close()`

## Naming Conventions

### Files and Packages

- Package names: lowercase, single word (e.g., `lcd`, `monitor`, `agentstat`)
- File names: lowercase with underscores for test files (e.g., `agentstat_test.go`)
- One package per directory

### Code Style

- Go standard formatting (use `gofmt` / `goimports`)
- Exported names: PascalCase (e.g., `NewCPUMonitor`, `GetState`)
- Unexported names: camelCase (e.g., `setupLayout`, `drawStatic`)
- Constants: PascalCase for exported, camelCase for private
- Acronyms: Uppercase (e.g., `CPU`, `RAM`, `LCD`, `DPMS`, `USB`)

### Variable Conventions

- Short variable names in tight scopes (`m` for monitor, `dc` for drawing context, `r` for renderer)
- Descriptive names for broader scopes
- Receivers: Single letter of type name (e.g., `m *CPUMonitor`, `b *Base`)

## Testing Approach

### Test Coverage

- **pkg/agentstat**: Comprehensive unit tests for validation logic
- **internal packages**: Currently minimal testing (relies on hardware interaction)
- CI runs all tests with `-mod=vendor`

### Writing Tests

```go
// Test file naming: <package>_test.go
package agentstat

import "testing"

func TestValidation(t *testing.T) {
    status := Status{
        Version: 1,
        Agent: "test",
        // ...
    }
    
    if err := status.Validate(); err != nil {
        t.Errorf("unexpected error: %v", err)
    }
}
```

### Testing Without Hardware

Use `--simulated` flag to run without physical displays:

```bash
# Tests rendering logic without serial communication
./screens --simulated --debug
```

The `Simulated` type logs operations but doesn't communicate with hardware.

## Important Patterns and Gotchas

### Serial Communication

**Hardware initialization sequence (internal/lcd/lcd.go:64-101)**:
1. Open serial port (115200 baud)
2. Send HELLO handshake
3. Reset display (closes/reopens serial)
4. Send HELLO again
5. Set orientation and brightness

**Critical**: After reset, the serial connection closes. The code must reopen it.

### Display Orientation

The displays are physically portrait (320x480) but used in landscape mode:
- Orientation `ReverseLandscape` (3) is typical
- After orientation change, width/height are swapped in rendering logic

### Sleep Detection (Linux)

The system uses two methods to detect when monitors go to sleep:

1. **systemd-logind IdleHint** (primary)
   - Checks session idle state via `loginctl show-session -p IdleHint`
   - Detects screen blanking/locking immediately
   - Works with most desktop environments (GNOME, KDE, etc.)

2. **DRM DPMS** (fallback)
   - Reads `/sys/class/drm/card*-*/dpms` files
   - States: `On`, `Standby`, `Suspend`, `Off`
   - Hardware-level monitor power state

**Why both?** Some systems set IdleHint before DPMS changes. Others only change DPMS. Checking both ensures reliable detection.

**Gotcha**: Only works on Linux with systemd-logind. On other platforms, detection returns `Unknown` and LCDs stay on.

### Agent Status Files

**Location**: `~/.agent-status/*.json`  
**Freshness**: Files older than 5 minutes are considered stale  
**Validation**: Use `pkg/agentstat` for reading and validation

**Sorting**: Agents are sorted by:
1. Activity orb color priority: red/yellow/orange (`error`, `thinking`, `paused`) first, then green/blue/teal (`working`, `done`, `waiting`), then grey (`idle`, unknown, or any stale agent)
2. Agent name alphabetically within each priority group

Stale agents are always sorted into the grey group regardless of their status. This keeps the most attention-worthy agents at the top of the display.

**Important**: Files must be written atomically (write to temp, then rename) to prevent partial reads.

See [AGENT_STATUS_REPORTING.md](./AGENT_STATUS_REPORTING.md) for full specification.

### Rendering Performance

Monitors use a cached image buffer and region-based updates:

```go
// Full redraw (rare, only on init)
m.ClearBuffer()
m.drawStatic()        // Static elements
m.DrawFullBuffer()    // Send entire buffer to LCD

// Incremental update (common, every tick)
m.drawDynamic()       // Update only changed regions
m.DrawRegion(region)  // Send only changed region to LCD
```

**Why**: Serial transfers are slow (~50-100ms for full screen). Region updates reduce latency.

### Font Handling

The renderer searches common font paths (internal/monitor/base.go:52-74):
- JetBrains Mono (preferred)
- DejaVu Sans Mono (fallback)
- Liberation Mono, Noto Sans Mono
- macOS: Monaco, SF Mono
- Windows: Consola, Courier New

**Gotcha**: If no fonts are found, rendering will fail silently. The PKGBUILD lists `ttf-jetbrains-mono` as optional dependency.

### Vendored Dependencies

This project uses **vendored dependencies** (vendor/ is committed).

**Always use `-mod=vendor` flag** with Go commands:
- ✅ `go build -mod=vendor`
- ✅ `go test -mod=vendor`
- ❌ `go build` (will fail or use wrong versions)

**Rationale**: Ensures reproducible builds, especially for Arch PKGBUILD.

### Color Palette

Default "htop-style" green theme (internal/monitor/base.go:30-42):
- Text: Bright green (`#00FF00`)
- Background: Black (`#000000`)
- Bars: Green (low), Yellow (medium), Red (high)
- Headers: Cyan (`#00FFFF`)

Agent status uses specific colors (internal/monitor/agent.go:14-22):
- `idle`: Gray
- `thinking`: Yellow
- `working`: Green
- `waiting`: Blue
- `error`: Red
- `done`: Teal
- `paused`: Orange

## Hardware-Specific Context

### Turing Smart Screen Rev A Protocol

**Command set** (internal/lcd/lcd.go:24-32):
- `101`: Reset display
- `102`: Clear screen
- `108/109`: Screen off/on
- `110`: Set brightness (0-255)
- `121`: Set orientation (0-3)
- `197`: Display bitmap data

**Bitmap format**: RGB565 (16-bit color), big-endian, sent row-by-row.

### USB Serial Settings

- **Baud rate**: 115200
- **Data bits**: 8
- **Parity**: None
- **Stop bits**: 1
- **Flow control**: None

### Device Detection

On Linux, displays appear as `/dev/ttyACM*`. The installer script:
1. Scans for devices with USB VID/PID matching Turing screens
2. Creates udev rules for stable device naming (`/dev/lcd-cpu`, `/dev/lcd-ram`, `/dev/lcd-agent`)
3. Maps physical USB ports to logical function

**Gotcha**: USB device order can change on reboot. Use udev rules with `KERNELS` (USB port path) for stability.

## Development Workflow

### Making Changes

1. **Read related code first**: Understand the monitor's structure and rendering pattern
2. **Test with simulation**: Use `--simulated` to verify rendering without hardware
3. **Test with hardware**: Run against physical displays
4. **Update tests**: Add/update tests in `pkg/agentstat` if changing validation logic
5. **Run lint**: `golangci-lint run` to catch style issues
6. **Update vendored deps**: `go mod tidy && go mod vendor` if dependencies changed

### Adding a New Monitor

1. Create file in `internal/monitor/` (e.g., `disk.go`)
2. Implement `Monitor` interface:
   - Embed `*Base` for common functionality
   - Implement `Name()`, `Run()`, `Stop()` methods
3. Add initialization in `cmd/screens/main.go`:
   - Add CLI flags (`--disk-port`, `--no-disk`, `--disk-interval`)
   - Add goroutine in initialization block
4. Update `README.md` with new monitor type
5. Test with `--simulated` first

### Modifying the LCD Protocol

**Warning**: Changes to `internal/lcd/` require physical hardware to test.

The protocol is reverse-engineered and device-specific. Test thoroughly:
- Different orientation modes
- Brightness levels
- Partial vs. full screen updates
- Reset behavior

### Working with Agent Status

If modifying the agent status format:
1. Update `AGENT_STATUS_REPORTING.md` (spec)
2. Update `agent-status.schema.json` (JSON schema)
3. Update `pkg/agentstat/agentstat.go` (Go types and validation)
4. Update tests in `pkg/agentstat/agentstat_test.go`
5. Increment schema version if breaking changes

**Validation philosophy**: Strict validation with helpful error messages. See `ValidateAll()` for collecting all errors at once.

## CI/CD

### GitHub Actions Workflows

**CI** (.github/workflows/ci.yml):
- Runs on push to `master` and PRs
- Verifies vendored dependencies match go.mod
- Builds all packages
- Runs tests
- Runs golangci-lint

**Release** (.github/workflows/release.yml):
- Triggered by version tags (e.g., `v0.1.0`)
- Builds binaries for Linux (amd64, arm64), Windows, macOS
- Creates GitHub release with binaries

### CI Requirements

- All builds must use `-mod=vendor`
- Tests must pass
- Linter must pass (golangci-lint with 5m timeout)
- Vendor directory must match go.mod/go.sum

## Common Tasks

### Add a new dependency

```bash
# Add to go.mod
go get github.com/example/package

# Vendor it
go mod tidy
go mod vendor

# Commit vendor changes
git add go.mod go.sum vendor/
git commit -m "Add github.com/example/package dependency"
```

### Change refresh intervals

Intervals are configurable via flags, but defaults are in `cmd/screens/main.go:32-34`:
- CPU: 1 second
- RAM: 2 seconds
- Agent: 2 seconds

### Adjust display brightness

Brightness is set at initialization (`--brightness 0-100`). To change at runtime, you'd need to:
1. Add signal handler for brightness change
2. Call `screen.SetBrightness(value)` on each display

Currently not implemented.

### Debug serial communication

Enable debug logging to see serial operations:

```bash
./screens --debug 2>&1 | grep -i serial
```

The `Display` type logs each command sent to the hardware.

### Handle new agent status field

1. Add field to `Status` struct in `pkg/agentstat/agentstat.go`
2. Add validation logic if needed (e.g., in `Validate()` method)
3. Update rendering in `internal/monitor/agent.go` to display the field
4. Add tests for validation

## Installation and Deployment

### Arch Linux Package

The PKGBUILD installs:
- Binary to `/usr/bin/turing-screens`
- systemd service to `/usr/lib/systemd/system/turing-screens.service`
- udev rules template to `/etc/udev/rules.d/99-turing-lcd.rules`
- Docs to `/usr/share/doc/turing-smart-screen/`

**Post-install**: User must edit udev rules with their USB port paths, then enable service.

### Interactive Installer

`install/install.sh`:
- Auto-detects connected devices
- Prompts user to assign displays to monitor types
- Generates udev rules with correct USB port paths
- Installs binary and systemd service
- Enables auto-start

**Requires**: Root permissions (uses `sudo`)

### systemd Service

`install/turing-screens.service`:
- Type: `simple`
- Restart: `always` (5s delay)
- Runs as root (needs access to serial ports)
- Waits for `multi-user.target`

**Managing**:
```bash
sudo systemctl start turing-screens
sudo systemctl stop turing-screens
sudo systemctl restart turing-screens
sudo systemctl status turing-screens
sudo journalctl -u turing-screens -f
```

## Troubleshooting

### Display not found

**Check device exists**:
```bash
ls -la /dev/ttyACM*
dmesg | grep tty
```

**Check permissions**:
```bash
# User needs to be in dialout/uucp group
sudo usermod -aG dialout $USER
# Log out and back in
```

### Display shows garbage

**Common causes**:
- Wrong orientation (try different values 0-3)
- Incorrect protocol for hardware revision
- Serial port settings mismatch
- USB cable issue

**Debug**:
```bash
./screens --debug
# Look for serial errors in logs
```

### Agent monitor shows no agents

**Check status files exist**:
```bash
ls -lah ~/.agent-status/
```

**Check file timestamps** (files >5min old are ignored):
```bash
find ~/.agent-status/ -name "*.json" -mmin -5
```

**Validate JSON**:
```bash
cat ~/.agent-status/crush-*.json | jq .
```

### High CPU usage

The monitors use timers with 1-2s intervals. High CPU usage usually indicates:
- Too fast refresh interval
- Serial communication blocking
- System info gathering is slow

**Check**:
```bash
strace -p $(pgrep screens) -c
# Look for system calls taking too long
```

## Resources

### Documentation Files

- [README.md](./README.md) - User-facing documentation
- [AGENT_STATUS_REPORTING.md](./AGENT_STATUS_REPORTING.md) - Agent status protocol spec
- [install/README.md](./install/README.md) - Installation instructions
- [agent-status.schema.json](./agent-status.schema.json) - JSON schema for validation

### External References

- Turing Smart Screen: https://turzx.com/
- gopsutil docs: https://github.com/shirou/gopsutil
- Go serial library: https://github.com/bugst/go-serial
- fogleman/gg (drawing): https://github.com/fogleman/gg

### Related Projects

This is a Go port of the Python turing-smart-screen project, focusing on system monitoring use case rather than general-purpose display.

## Security Considerations

- Service runs as root (needs serial port access)
- Agent status files read from user home directory
- No network communication
- No remote control interface
- Status files should not contain sensitive data (see AGENT_STATUS_REPORTING.md security section)

## Future Enhancements Ideas

These are potential improvements, not current features:

- Network monitor (bandwidth, connections)
- Disk I/O monitor
- GPU monitoring (NVIDIA/AMD)
- Container stats (Docker/Podman)
- Configuration file support (replace CLI flags)
- Dynamic brightness based on time of day
- Web dashboard for remote viewing
- Support for newer Turing Smart Screen revisions

---

**Last Updated**: January 2026  
**Go Version**: 1.22  
**Schema Version**: 1
