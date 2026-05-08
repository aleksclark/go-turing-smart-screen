// Package dpms provides monitor sleep state detection via DRM and systemd-logind.
package dpms

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// State represents the DPMS power state.
type State int

const (
	On State = iota
	Standby
	Suspend
	Off
	Unknown
)

func (s State) String() string {
	switch s {
	case On:
		return "On"
	case Standby:
		return "Standby"
	case Suspend:
		return "Suspend"
	case Off:
		return "Off"
	default:
		return "Unknown"
	}
}

// IsAsleep returns true if the state indicates the monitor is asleep.
func (s State) IsAsleep() bool {
	return s == Standby || s == Suspend || s == Off
}

// GetState returns the current DPMS state of connected monitors.
// Checks both DRM DPMS state and systemd-logind IdleHint for the graphical session.
// Disconnected connectors are ignored.
// Returns On if any connected monitor is on and session is not idle.
func GetState() State {
	// Check systemd-logind IdleHint for the graphical session
	if isSessionIdle() {
		return Off
	}

	// Check DRM DPMS state, but only for connected connectors
	matches, err := filepath.Glob("/sys/class/drm/card*-*/dpms")
	if err != nil || len(matches) == 0 {
		return Unknown
	}

	anyOn := false
	anyAsleep := false

	for _, path := range matches {
		// Skip disconnected connectors — they can report phantom "On" state
		dir := filepath.Dir(path)
		statusData, err := os.ReadFile(filepath.Join(dir, "status"))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(statusData)) != "connected" {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		state := parseState(strings.TrimSpace(string(data)))
		if state == On {
			anyOn = true
		} else if state.IsAsleep() {
			anyAsleep = true
		}
	}

	if anyOn {
		return On
	}
	if anyAsleep {
		return Off
	}
	return Unknown
}

func parseState(s string) State {
	switch s {
	case "On":
		return On
	case "Standby":
		return Standby
	case "Suspend":
		return Suspend
	case "Off":
		return Off
	default:
		return Unknown
	}
}

// Watcher monitors DPMS state changes.
type Watcher struct {
	interval  time.Duration
	lastState State
	onChange  func(State)
	stop     chan struct{}
}

// NewWatcher creates a new DPMS state watcher.
func NewWatcher(interval time.Duration, onChange func(State)) *Watcher {
	return &Watcher{
		interval:  interval,
		lastState: Unknown,
		onChange:  onChange,
		stop:      make(chan struct{}),
	}
}

// Start begins watching for DPMS state changes.
func (w *Watcher) Start() {
	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()

		// Check initial state
		w.lastState = GetState()

		for {
			select {
			case <-ticker.C:
				state := GetState()
				if state != w.lastState && state != Unknown {
					w.lastState = state
					if w.onChange != nil {
						w.onChange(state)
					}
				}
			case <-w.stop:
				return
			}
		}
	}()
}

// Stop stops watching.
func (w *Watcher) Stop() {
	close(w.stop)
}

// isSessionIdle checks if the graphical session on seat0 is idle.
// Finds the graphical session explicitly rather than relying on the default
// session, which may be the service's own session when running under systemd.
func isSessionIdle() bool {
	// Find the session on seat0 (the graphical seat)
	sessionID := findGraphicalSession()
	if sessionID == "" {
		return false
	}

	cmd := exec.Command("loginctl", "show-session", sessionID, "-p", "IdleHint")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	line := strings.TrimSpace(string(output))
	if strings.HasPrefix(line, "IdleHint=") {
		value := strings.TrimPrefix(line, "IdleHint=")
		return value == "yes"
	}

	return false
}

// findGraphicalSession returns the session ID of the graphical session on seat0.
// It looks for wayland or x11 sessions first, then falls back to tty sessions on seat0.
func findGraphicalSession() string {
	cmd := exec.Command("loginctl", "list-sessions", "--no-legend")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var seat0Sessions []string

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		sessionID := fields[0]
		// Seat is typically field index 3
		for _, f := range fields {
			if f == "seat0" {
				seat0Sessions = append(seat0Sessions, sessionID)
				break
			}
		}
	}

	// Check each seat0 session for graphical type
	for _, sid := range seat0Sessions {
		cmd := exec.Command("loginctl", "show-session", sid, "-p", "Type")
		output, err := cmd.Output()
		if err != nil {
			continue
		}
		line := strings.TrimSpace(string(output))
		sessionType := strings.TrimPrefix(line, "Type=")
		if sessionType == "wayland" || sessionType == "x11" {
			return sid
		}
	}

	// Fallback: return first seat0 session (could be tty-based graphical)
	if len(seat0Sessions) > 0 {
		return seat0Sessions[0]
	}

	return ""
}
