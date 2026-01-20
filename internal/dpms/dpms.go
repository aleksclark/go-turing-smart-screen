// Package dpms provides monitor sleep state detection via DRM.
package dpms

import (
	"os"
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

// GetState returns the current DPMS state of any connected monitor.
// Returns On if any monitor is on, otherwise returns the sleep state.
func GetState() State {
	matches, err := filepath.Glob("/sys/class/drm/card*-*/dpms")
	if err != nil || len(matches) == 0 {
		return Unknown
	}

	anyOn := false
	anyAsleep := false

	for _, path := range matches {
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
	interval time.Duration
	lastState State
	onChange func(State)
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
