# Recent Changes

## Agent Monitor Launch-Order Sorting

The agent status monitor now sorts agents by their launch time (when available), providing a stable chronological view.

**Sorting order:**
1. Launch time (`started` field, earliest first) if available
2. Instance ID alphabetically (for agents without `started` field)

Agents with `started` timestamps are always listed before agents without. When multiple agents have the same launch time (or no launch time), they sort alphabetically by instance ID.

**Location:** `pkg/agentstat/agentstat.go:374-391`

**Tests:** `pkg/agentstat/sort_test.go`

## Enhanced Sleep Detection

The DPMS watcher now checks both systemd-logind IdleHint and DRM DPMS state for more reliable screen sleep detection.

**Detection methods:**
1. **systemd-logind IdleHint** (primary) - via `loginctl show-session -p IdleHint`
   - Faster detection
   - Works with most desktop environments
   - Catches screen blanking/locking
   
2. **DRM DPMS** (fallback) - via `/sys/class/drm/card*-*/dpms`
   - Hardware-level state
   - Works when IdleHint is unavailable

**Why both?** Some systems set IdleHint before DPMS. Others only change DPMS. Using both ensures the LCDs turn off reliably when the computer's monitors go to sleep.

**Location:** `internal/dpms/dpms.go:42-74, 140-160`

**Tests:** `internal/dpms/dpms_test.go`

## Files Modified

- `pkg/agentstat/agentstat.go` - Changed to sort by launch time (started) instead of last update time
- `internal/dpms/dpms.go` - Added systemd-logind IdleHint check
- `pkg/agentstat/sort_test.go` - Comprehensive tests for launch-order sorting
- `internal/dpms/dpms_test.go` - New test file for DPMS state detection
- `AGENTS.md` - Updated documentation for both features

## Testing

All tests pass:
```bash
go test -mod=vendor ./...
```

Build successful:
```bash
go build -mod=vendor -o screens ./cmd/screens
```
