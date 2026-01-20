package agentstat

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidate_RequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		status  Status
		wantErr error
	}{
		{
			name:    "empty status",
			status:  Status{},
			wantErr: ErrMissingVersion,
		},
		{
			name:    "missing agent",
			status:  Status{Version: 1},
			wantErr: ErrMissingAgent,
		},
		{
			name:    "missing instance",
			status:  Status{Version: 1, Agent: "test"},
			wantErr: ErrMissingInstance,
		},
		{
			name:    "missing status",
			status:  Status{Version: 1, Agent: "test", Instance: "abc"},
			wantErr: ErrMissingStatus,
		},
		{
			name:    "missing updated",
			status:  Status{Version: 1, Agent: "test", Instance: "abc", Status: "idle"},
			wantErr: ErrMissingUpdated,
		},
		{
			name: "valid minimal",
			status: Status{
				Version:  1,
				Agent:    "test",
				Instance: "abc123",
				Status:   "idle",
				Updated:  time.Now().Unix(),
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.status.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Errorf("Validate() error = nil, want %v", tt.wantErr)
				return
			}
			var vErr *ValidationError
			if !errors.As(err, &vErr) {
				t.Errorf("Validate() error type = %T, want *ValidationError", err)
				return
			}
			if !errors.Is(vErr.Err, tt.wantErr) {
				t.Errorf("Validate() error = %v, want %v", vErr.Err, tt.wantErr)
			}
		})
	}
}

func TestValidate_InvalidValues(t *testing.T) {
	base := Status{
		Version:  1,
		Agent:    "test",
		Instance: "abc",
		Status:   "idle",
		Updated:  time.Now().Unix(),
	}

	tests := []struct {
		name    string
		modify  func(*Status)
		wantErr error
	}{
		{
			name:    "invalid version",
			modify:  func(s *Status) { s.Version = 99 },
			wantErr: ErrInvalidVersion,
		},
		{
			name:    "invalid agent - uppercase",
			modify:  func(s *Status) { s.Agent = "TEST" },
			wantErr: ErrInvalidAgent,
		},
		{
			name:    "invalid agent - starts with number",
			modify:  func(s *Status) { s.Agent = "123test" },
			wantErr: ErrInvalidAgent,
		},
		{
			name:    "invalid agent - spaces",
			modify:  func(s *Status) { s.Agent = "test agent" },
			wantErr: ErrInvalidAgent,
		},
		{
			name:    "invalid status value",
			modify:  func(s *Status) { s.Status = "unknown" },
			wantErr: ErrInvalidStatus,
		},
		{
			name:    "invalid provider",
			modify:  func(s *Status) { s.Provider = "invalid-provider" },
			wantErr: ErrInvalidProvider,
		},
		{
			name:    "negative cost",
			modify:  func(s *Status) { s.CostUSD = -1.0 },
			wantErr: ErrInvalidCost,
		},
		{
			name:    "negative started",
			modify:  func(s *Status) { s.Started = -1 },
			wantErr: ErrInvalidStarted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := base // copy
			tt.modify(&s)
			err := s.Validate()
			if err == nil {
				t.Errorf("Validate() error = nil, want %v", tt.wantErr)
				return
			}
			var vErr *ValidationError
			if !errors.As(err, &vErr) {
				t.Errorf("Validate() error type = %T, want *ValidationError", err)
				return
			}
			if !errors.Is(vErr.Err, tt.wantErr) {
				t.Errorf("Validate() error = %v, want %v", vErr.Err, tt.wantErr)
			}
		})
	}
}

func TestValidate_Tools(t *testing.T) {
	base := Status{
		Version:  1,
		Agent:    "test",
		Instance: "abc",
		Status:   "working",
		Updated:  time.Now().Unix(),
	}

	t.Run("valid tools", func(t *testing.T) {
		s := base
		s.Tools = &ToolsInfo{
			Active: "edit",
			Recent: []string{"view", "grep", "edit"},
			Counts: map[string]int{"edit": 5, "view": 12},
		}
		if err := s.Validate(); err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("too many recent tools", func(t *testing.T) {
		s := base
		s.Tools = &ToolsInfo{
			Recent: make([]string, 11), // max is 10
		}
		err := s.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want error")
		}
		var vErr *ValidationError
		if !errors.As(err, &vErr) || !errors.Is(vErr.Err, ErrTooManyRecent) {
			t.Errorf("Validate() error = %v, want ErrTooManyRecent", err)
		}
	})

	t.Run("negative tool count", func(t *testing.T) {
		s := base
		s.Tools = &ToolsInfo{
			Counts: map[string]int{"edit": -1},
		}
		err := s.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want error")
		}
		var vErr *ValidationError
		if !errors.As(err, &vErr) || !errors.Is(vErr.Err, ErrNegativeToolCount) {
			t.Errorf("Validate() error = %v, want ErrNegativeToolCount", err)
		}
	})
}

func TestValidate_Tokens(t *testing.T) {
	base := Status{
		Version:  1,
		Agent:    "test",
		Instance: "abc",
		Status:   "working",
		Updated:  time.Now().Unix(),
	}

	t.Run("valid tokens", func(t *testing.T) {
		s := base
		s.Tokens = &TokensInfo{
			Input:      125000,
			Output:     15000,
			CacheRead:  80000,
			CacheWrite: 45000,
		}
		if err := s.Validate(); err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	tests := []struct {
		name   string
		tokens TokensInfo
		field  string
	}{
		{"negative input", TokensInfo{Input: -1}, "tokens.input"},
		{"negative output", TokensInfo{Output: -1}, "tokens.output"},
		{"negative cache_read", TokensInfo{CacheRead: -1}, "tokens.cache_read"},
		{"negative cache_write", TokensInfo{CacheWrite: -1}, "tokens.cache_write"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := base
			s.Tokens = &tt.tokens
			err := s.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
			var vErr *ValidationError
			if !errors.As(err, &vErr) {
				t.Errorf("Validate() error type = %T, want *ValidationError", err)
				return
			}
			if vErr.Field != tt.field {
				t.Errorf("ValidationError.Field = %q, want %q", vErr.Field, tt.field)
			}
			if !errors.Is(vErr.Err, ErrNegativeTokens) {
				t.Errorf("Validate() error = %v, want ErrNegativeTokens", vErr.Err)
			}
		})
	}
}

func TestValidate_AllStatuses(t *testing.T) {
	for _, status := range ValidStatuses {
		t.Run(status, func(t *testing.T) {
			s := Status{
				Version:  1,
				Agent:    "test",
				Instance: "abc",
				Status:   status,
				Updated:  time.Now().Unix(),
			}
			if err := s.Validate(); err != nil {
				t.Errorf("Validate() error = %v for status %q", err, status)
			}
		})
	}
}

func TestValidate_AllProviders(t *testing.T) {
	for _, provider := range ValidProviders {
		t.Run(provider, func(t *testing.T) {
			s := Status{
				Version:  1,
				Agent:    "test",
				Instance: "abc",
				Status:   "idle",
				Updated:  time.Now().Unix(),
				Provider: provider,
			}
			if err := s.Validate(); err != nil {
				t.Errorf("Validate() error = %v for provider %q", err, provider)
			}
		})
	}
}

func TestValidate_AgentIdentifiers(t *testing.T) {
	validAgents := []string{
		"crush",
		"cursor",
		"claude-code",
		"aider",
		"copilot",
		"a",
		"agent123",
		"my-agent-v2",
	}

	invalidAgents := []string{
		"CRUSH",       // uppercase
		"Claude-Code", // mixed case
		"123agent",    // starts with number
		"-agent",      // starts with hyphen
		"agent_test",  // underscore
		"agent test",  // space
		"",            // empty
	}

	for _, agent := range validAgents {
		t.Run("valid_"+agent, func(t *testing.T) {
			s := Status{
				Version:  1,
				Agent:    agent,
				Instance: "abc",
				Status:   "idle",
				Updated:  time.Now().Unix(),
			}
			if err := s.Validate(); err != nil {
				t.Errorf("Validate() error = %v for agent %q", err, agent)
			}
		})
	}

	for _, agent := range invalidAgents {
		t.Run("invalid_"+agent, func(t *testing.T) {
			s := Status{
				Version:  1,
				Agent:    agent,
				Instance: "abc",
				Status:   "idle",
				Updated:  time.Now().Unix(),
			}
			err := s.Validate()
			if agent == "" {
				// Empty should give ErrMissingAgent
				var vErr *ValidationError
				if !errors.As(err, &vErr) || !errors.Is(vErr.Err, ErrMissingAgent) {
					t.Errorf("Validate() error = %v, want ErrMissingAgent", err)
				}
			} else if err == nil {
				t.Errorf("Validate() error = nil for invalid agent %q", agent)
			}
		})
	}
}

func TestValidateAll(t *testing.T) {
	s := Status{} // All required fields missing
	errs := s.ValidateAll()

	expectedCount := 5 // v, agent, instance, status, updated
	if len(errs) != expectedCount {
		t.Errorf("ValidateAll() returned %d errors, want %d", len(errs), expectedCount)
	}
}

func TestIsValid(t *testing.T) {
	valid := Status{
		Version:  1,
		Agent:    "test",
		Instance: "abc",
		Status:   "idle",
		Updated:  time.Now().Unix(),
	}

	invalid := Status{}

	if !valid.IsValid() {
		t.Error("IsValid() = false for valid status")
	}
	if invalid.IsValid() {
		t.Error("IsValid() = true for invalid status")
	}
}

func TestJSONRoundtrip(t *testing.T) {
	original := Status{
		Version:  1,
		Agent:    "crush",
		Instance: "a1b2c3",
		PID:      12345,
		Project:  "test-project",
		CWD:      "/home/user/work",
		Status:   "working",
		Task:     "implementing feature",
		Model:    "claude-sonnet-4-20250514",
		Provider: "anthropic",
		Tools: &ToolsInfo{
			Active: "edit",
			Recent: []string{"view", "grep", "edit"},
			Counts: map[string]int{"edit": 5, "view": 12, "grep": 3},
		},
		Tokens: &TokensInfo{
			Input:      125000,
			Output:     15000,
			CacheRead:  80000,
			CacheWrite: 45000,
		},
		CostUSD: 0.42,
		Started: 1737276000,
		Updated: 1737276300,
		Error:   "",
		Context: map[string]any{"custom": "value"},
	}

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Unmarshal
	var parsed Status
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Validate parsed
	if err := parsed.Validate(); err != nil {
		t.Errorf("Validate() error = %v after roundtrip", err)
	}

	// Check fields
	if parsed.Version != original.Version {
		t.Errorf("Version = %d, want %d", parsed.Version, original.Version)
	}
	if parsed.Agent != original.Agent {
		t.Errorf("Agent = %q, want %q", parsed.Agent, original.Agent)
	}
	if parsed.Tools.Active != original.Tools.Active {
		t.Errorf("Tools.Active = %q, want %q", parsed.Tools.Active, original.Tools.Active)
	}
	if parsed.Tokens.Input != original.Tokens.Input {
		t.Errorf("Tokens.Input = %d, want %d", parsed.Tokens.Input, original.Tokens.Input)
	}
}

func TestReadAllWithErrors(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	t.Setenv("AGENT_STATUS_DIR", tmpDir)

	now := time.Now().Unix()

	// Valid file
	validData := `{"v":1,"agent":"test","instance":"abc","status":"idle","updated":` + itoa(int(now)) + `}`
	if err := os.WriteFile(filepath.Join(tmpDir, "test-abc.json"), []byte(validData), 0644); err != nil {
		t.Fatal(err)
	}

	// Invalid JSON file
	if err := os.WriteFile(filepath.Join(tmpDir, "bad-json.json"), []byte(`{invalid`), 0644); err != nil {
		t.Fatal(err)
	}

	// Invalid schema file (missing required fields)
	invalidData := `{"v":1,"agent":"test"}`
	if err := os.WriteFile(filepath.Join(tmpDir, "invalid-schema.json"), []byte(invalidData), 0644); err != nil {
		t.Fatal(err)
	}

	// Old file (should be filtered by maxAge)
	oldData := `{"v":1,"agent":"old","instance":"xyz","status":"idle","updated":1000000000}`
	if err := os.WriteFile(filepath.Join(tmpDir, "old-xyz.json"), []byte(oldData), 0644); err != nil {
		t.Fatal(err)
	}

	statuses, readErrors, err := ReadAllWithErrors(5 * time.Minute)
	if err != nil {
		t.Fatalf("ReadAllWithErrors() error = %v", err)
	}

	// Should have 1 valid status
	if len(statuses) != 1 {
		t.Errorf("len(statuses) = %d, want 1", len(statuses))
	}
	if len(statuses) > 0 && statuses[0].Agent != "test" {
		t.Errorf("statuses[0].Agent = %q, want %q", statuses[0].Agent, "test")
	}

	// Should have 2 read errors (bad JSON + invalid schema)
	if len(readErrors) != 2 {
		t.Errorf("len(readErrors) = %d, want 2", len(readErrors))
		for _, e := range readErrors {
			t.Logf("  error: %v", e)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		count int
		want  string
	}{
		{0, "0"},
		{100, "100"},
		{999, "999"},
		{1000, "1.0k"},
		{1500, "1.5k"},
		{15000, "15.0k"},
		{125000, "125.0k"},
		{1000000, "1.0M"},
		{1500000, "1.5M"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatTokens(tt.count)
			if got != tt.want {
				t.Errorf("FormatTokens(%d) = %q, want %q", tt.count, got, tt.want)
			}
		})
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		cost float64
		want string
	}{
		{0, "$0.000"},
		{0.001, "$0.001"},
		{0.009, "$0.009"},
		{0.01, "$0.0"},
		{0.42, "$0.4"},
		{1.5, "$1.5"},
		{10.0, "$10.0"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatCost(tt.cost)
			if got != tt.want {
				t.Errorf("FormatCost(%f) = %q, want %q", tt.cost, got, tt.want)
			}
		})
	}
}

func TestStatusDir(t *testing.T) {
	// With env var
	t.Setenv("AGENT_STATUS_DIR", "/custom/path")
	if got := StatusDir(); got != "/custom/path" {
		t.Errorf("StatusDir() with env = %q, want %q", got, "/custom/path")
	}

	// Without env var (uses home dir)
	t.Setenv("AGENT_STATUS_DIR", "")
	got := StatusDir()
	if got == "" {
		t.Error("StatusDir() = empty without env")
	}
	if filepath.Base(got) != ".agent-status" {
		t.Errorf("StatusDir() = %q, want suffix .agent-status", got)
	}
}

func TestCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGENT_STATUS_DIR", tmpDir)

	// Recent file (should be kept)
	now := time.Now().Unix()
	recentData := `{"v":1,"agent":"recent","instance":"abc","status":"idle","updated":` + itoa(int(now)) + `}`
	if err := os.WriteFile(filepath.Join(tmpDir, "recent-abc.json"), []byte(recentData), 0644); err != nil {
		t.Fatal(err)
	}

	// Old file (should be removed)
	oldData := `{"v":1,"agent":"old","instance":"xyz","status":"idle","updated":1000000000}`
	if err := os.WriteFile(filepath.Join(tmpDir, "old-xyz.json"), []byte(oldData), 0644); err != nil {
		t.Fatal(err)
	}

	// Invalid JSON (should be removed)
	if err := os.WriteFile(filepath.Join(tmpDir, "invalid.json"), []byte(`{bad`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Cleanup(time.Hour); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	// Check remaining files
	entries, _ := os.ReadDir(tmpDir)
	if len(entries) != 1 {
		t.Errorf("len(entries) after cleanup = %d, want 1", len(entries))
	}
	if len(entries) > 0 && entries[0].Name() != "recent-abc.json" {
		t.Errorf("remaining file = %q, want recent-abc.json", entries[0].Name())
	}
}

func TestValidationError(t *testing.T) {
	err := &ValidationError{Field: "test_field", Err: ErrMissingAgent}

	if err.Error() != "test_field: missing required field: agent" {
		t.Errorf("Error() = %q", err.Error())
	}

	if !errors.Is(err, ErrMissingAgent) {
		t.Error("errors.Is() = false, want true")
	}
}
