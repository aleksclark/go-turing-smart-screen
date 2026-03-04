package dpms

import (
	"testing"
)

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{On, "On"},
		{Standby, "Standby"},
		{Suspend, "Suspend"},
		{Off, "Off"},
		{Unknown, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("State.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStateIsAsleep(t *testing.T) {
	tests := []struct {
		state State
		want  bool
	}{
		{On, false},
		{Standby, true},
		{Suspend, true},
		{Off, true},
		{Unknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			if got := tt.state.IsAsleep(); got != tt.want {
				t.Errorf("State.IsAsleep() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseState(t *testing.T) {
	tests := []struct {
		input string
		want  State
	}{
		{"On", On},
		{"Standby", Standby},
		{"Suspend", Suspend},
		{"Off", Off},
		{"unknown", Unknown},
		{"", Unknown},
		{"invalid", Unknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := parseState(tt.input); got != tt.want {
				t.Errorf("parseState(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetState(t *testing.T) {
	// This test will only work on systems with DRM or loginctl
	// Just verify it doesn't crash
	state := GetState()
	t.Logf("Current state: %v", state)
	
	// State should be one of the valid values
	validStates := []State{On, Standby, Suspend, Off, Unknown}
	valid := false
	for _, s := range validStates {
		if state == s {
			valid = true
			break
		}
	}
	if !valid {
		t.Errorf("GetState() returned invalid state: %v", state)
	}
}
