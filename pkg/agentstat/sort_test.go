package agentstat

import (
	"testing"
	"time"
)

func TestSortByOrbPriority(t *testing.T) {
	now := time.Now().Unix()

	statuses := []Status{
		{Version: 1, Agent: "idle-agent", Instance: "a", Status: "idle", Updated: now},
		{Version: 1, Agent: "working-agent", Instance: "b", Status: "working", Updated: now},
		{Version: 1, Agent: "error-agent", Instance: "c", Status: "error", Updated: now},
		{Version: 1, Agent: "thinking-agent", Instance: "d", Status: "thinking", Updated: now},
	}

	SortStatuses(statuses)

	// Red/yellow first (error, thinking), then green (working), then grey (idle)
	expected := []string{"error-agent", "thinking-agent", "working-agent", "idle-agent"}
	for i, s := range statuses {
		if s.Agent != expected[i] {
			t.Errorf("statuses[%d].Agent = %s, want %s", i, s.Agent, expected[i])
		}
	}
}

func TestSortByTitleWithinSamePriority(t *testing.T) {
	now := time.Now().Unix()

	statuses := []Status{
		{Version: 1, Agent: "cursor", Instance: "a", Status: "working", Updated: now},
		{Version: 1, Agent: "aider", Instance: "b", Status: "working", Updated: now},
		{Version: 1, Agent: "crush", Instance: "c", Status: "working", Updated: now},
	}

	SortStatuses(statuses)

	expected := []string{"aider", "crush", "cursor"}
	for i, s := range statuses {
		if s.Agent != expected[i] {
			t.Errorf("statuses[%d].Agent = %s, want %s", i, s.Agent, expected[i])
		}
	}
}

func TestSortStaleAgentsGoToGrey(t *testing.T) {
	now := time.Now().Unix()

	statuses := []Status{
		{Version: 1, Agent: "stale-worker", Instance: "a", Status: "working", Updated: now, Stale: true},
		{Version: 1, Agent: "fresh-worker", Instance: "b", Status: "working", Updated: now},
		{Version: 1, Agent: "fresh-error", Instance: "c", Status: "error", Updated: now},
	}

	SortStatuses(statuses)

	// error (red) first, then fresh working (green), then stale working (grey)
	expected := []string{"fresh-error", "fresh-worker", "stale-worker"}
	for i, s := range statuses {
		if s.Agent != expected[i] {
			t.Errorf("statuses[%d].Agent = %s, want %s", i, s.Agent, expected[i])
		}
	}
}

func TestSortMixedPrioritiesAndTitles(t *testing.T) {
	now := time.Now().Unix()

	statuses := []Status{
		{Version: 1, Agent: "zeta", Instance: "a", Status: "idle", Updated: now},
		{Version: 1, Agent: "beta", Instance: "b", Status: "error", Updated: now},
		{Version: 1, Agent: "alpha", Instance: "c", Status: "working", Updated: now},
		{Version: 1, Agent: "gamma", Instance: "d", Status: "thinking", Updated: now},
		{Version: 1, Agent: "delta", Instance: "e", Status: "working", Updated: now},
		{Version: 1, Agent: "alpha", Instance: "f", Status: "error", Updated: now},
	}

	SortStatuses(statuses)

	// Priority 0 (red/yellow): alpha-error, beta-error, gamma-thinking
	// Priority 1 (green): alpha-working, delta-working
	// Priority 2 (grey): zeta-idle
	expected := []string{"alpha", "beta", "gamma", "alpha", "delta", "zeta"}
	for i, s := range statuses {
		if s.Agent != expected[i] {
			t.Errorf("statuses[%d].Agent = %s, want %s", i, s.Agent, expected[i])
		}
	}
}

func TestSortPausedIsPriorityZero(t *testing.T) {
	now := time.Now().Unix()

	statuses := []Status{
		{Version: 1, Agent: "worker", Instance: "a", Status: "working", Updated: now},
		{Version: 1, Agent: "paused-agent", Instance: "b", Status: "paused", Updated: now},
	}

	SortStatuses(statuses)

	if statuses[0].Agent != "paused-agent" {
		t.Errorf("statuses[0].Agent = %s, want paused-agent (paused is priority 0)", statuses[0].Agent)
	}
}

func TestSortDoneAndWaitingAreGreen(t *testing.T) {
	now := time.Now().Unix()

	statuses := []Status{
		{Version: 1, Agent: "idle-one", Instance: "a", Status: "idle", Updated: now},
		{Version: 1, Agent: "done-agent", Instance: "b", Status: "done", Updated: now},
		{Version: 1, Agent: "waiting-agent", Instance: "c", Status: "waiting", Updated: now},
	}

	SortStatuses(statuses)

	// done and waiting (green, priority 1) before idle (grey, priority 2)
	expected := []string{"done-agent", "waiting-agent", "idle-one"}
	for i, s := range statuses {
		if s.Agent != expected[i] {
			t.Errorf("statuses[%d].Agent = %s, want %s", i, s.Agent, expected[i])
		}
	}
}

func TestStatusOrbPriority(t *testing.T) {
	tests := []struct {
		status   string
		stale    bool
		expected int
	}{
		{"error", false, 0},
		{"thinking", false, 0},
		{"paused", false, 0},
		{"working", false, 1},
		{"done", false, 1},
		{"waiting", false, 1},
		{"idle", false, 2},
		{"unknown", false, 2},
		{"working", true, 2},
		{"error", true, 2},
	}

	for _, tt := range tests {
		got := StatusOrbPriority(tt.status, tt.stale)
		if got != tt.expected {
			t.Errorf("StatusOrbPriority(%q, %v) = %d, want %d", tt.status, tt.stale, got, tt.expected)
		}
	}
}
