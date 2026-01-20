// Package agentstat provides reading and validation of agent status files.
package agentstat

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// Schema version supported by this package.
const SchemaVersion = 1

// Valid status values per the schema.
var ValidStatuses = []string{"idle", "thinking", "working", "waiting", "error", "done", "paused"}

// Valid provider values per the schema.
var ValidProviders = []string{"anthropic", "openai", "bedrock", "vertex", "ollama", "local", "azure", "google"}

// agentPattern matches valid agent identifiers (lowercase, starting with letter).
var agentPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Status represents an agent's current status.
type Status struct {
	Version  int            `json:"v"`
	Agent    string         `json:"agent"`
	Instance string         `json:"instance"`
	PID      int            `json:"pid,omitempty"`
	Project  string         `json:"project,omitempty"`
	CWD      string         `json:"cwd,omitempty"`
	Status   string         `json:"status"`
	Task     string         `json:"task,omitempty"`
	Model    string         `json:"model,omitempty"`
	Provider string         `json:"provider,omitempty"`
	Tools    *ToolsInfo     `json:"tools,omitempty"`
	Tokens   *TokensInfo    `json:"tokens,omitempty"`
	CostUSD  float64        `json:"cost_usd,omitempty"`
	Started  int64          `json:"started,omitempty"`
	Updated  int64          `json:"updated"`
	Error    string         `json:"error,omitempty"`
	Context  map[string]any `json:"context,omitempty"`

	// Computed fields (not from JSON)
	Age   time.Duration `json:"-"`
	Stale bool          `json:"-"`
	File  string        `json:"-"`
}

// ToolsInfo holds tool usage information.
type ToolsInfo struct {
	Active string         `json:"active,omitempty"`
	Recent []string       `json:"recent,omitempty"`
	Counts map[string]int `json:"counts,omitempty"`
}

// TokensInfo holds token usage counters.
type TokensInfo struct {
	Input      int `json:"input,omitempty"`
	Output     int `json:"output,omitempty"`
	CacheRead  int `json:"cache_read,omitempty"`
	CacheWrite int `json:"cache_write,omitempty"`
}

// Thresholds for status freshness.
const (
	FreshThreshold = 60 * time.Second
	StaleThreshold = 300 * time.Second
)

// Validation errors.
var (
	ErrMissingVersion    = errors.New("missing required field: v")
	ErrInvalidVersion    = errors.New("unsupported schema version")
	ErrMissingAgent      = errors.New("missing required field: agent")
	ErrInvalidAgent      = errors.New("invalid agent identifier (must be lowercase, start with letter)")
	ErrMissingInstance   = errors.New("missing required field: instance")
	ErrEmptyInstance     = errors.New("instance cannot be empty")
	ErrMissingStatus     = errors.New("missing required field: status")
	ErrInvalidStatus     = errors.New("invalid status value")
	ErrMissingUpdated    = errors.New("missing required field: updated")
	ErrInvalidUpdated    = errors.New("updated must be a positive timestamp")
	ErrInvalidPID        = errors.New("pid must be positive if provided")
	ErrInvalidProvider   = errors.New("invalid provider value")
	ErrInvalidCost       = errors.New("cost_usd must be non-negative")
	ErrInvalidStarted    = errors.New("started must be non-negative")
	ErrTooManyRecent     = errors.New("tools.recent exceeds maximum of 10 items")
	ErrNegativeTokens    = errors.New("token counts must be non-negative")
	ErrNegativeToolCount = errors.New("tool counts must be non-negative")
)

// ValidationError wraps a validation error with field context.
type ValidationError struct {
	Field string
	Err   error
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %v", e.Field, e.Err)
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}

// Validate checks if the status conforms to the schema.
// Returns nil if valid, or an error describing the first validation failure.
func (s *Status) Validate() error {
	// Required fields
	if s.Version == 0 {
		return &ValidationError{"v", ErrMissingVersion}
	}
	if s.Version != SchemaVersion {
		return &ValidationError{"v", ErrInvalidVersion}
	}

	if s.Agent == "" {
		return &ValidationError{"agent", ErrMissingAgent}
	}
	if !agentPattern.MatchString(s.Agent) {
		return &ValidationError{"agent", ErrInvalidAgent}
	}

	if s.Instance == "" {
		return &ValidationError{"instance", ErrMissingInstance}
	}

	if s.Status == "" {
		return &ValidationError{"status", ErrMissingStatus}
	}
	if !isValidStatus(s.Status) {
		return &ValidationError{"status", ErrInvalidStatus}
	}

	if s.Updated == 0 {
		return &ValidationError{"updated", ErrMissingUpdated}
	}
	if s.Updated < 0 {
		return &ValidationError{"updated", ErrInvalidUpdated}
	}

	// Optional field validation
	if s.PID < 0 {
		return &ValidationError{"pid", ErrInvalidPID}
	}

	if s.Provider != "" && !isValidProvider(s.Provider) {
		return &ValidationError{"provider", ErrInvalidProvider}
	}

	if s.CostUSD < 0 {
		return &ValidationError{"cost_usd", ErrInvalidCost}
	}

	if s.Started < 0 {
		return &ValidationError{"started", ErrInvalidStarted}
	}

	// Tools validation
	if s.Tools != nil {
		if len(s.Tools.Recent) > 10 {
			return &ValidationError{"tools.recent", ErrTooManyRecent}
		}
		for name, count := range s.Tools.Counts {
			if count < 0 {
				return &ValidationError{fmt.Sprintf("tools.counts.%s", name), ErrNegativeToolCount}
			}
		}
	}

	// Tokens validation
	if s.Tokens != nil {
		if s.Tokens.Input < 0 {
			return &ValidationError{"tokens.input", ErrNegativeTokens}
		}
		if s.Tokens.Output < 0 {
			return &ValidationError{"tokens.output", ErrNegativeTokens}
		}
		if s.Tokens.CacheRead < 0 {
			return &ValidationError{"tokens.cache_read", ErrNegativeTokens}
		}
		if s.Tokens.CacheWrite < 0 {
			return &ValidationError{"tokens.cache_write", ErrNegativeTokens}
		}
	}

	return nil
}

// ValidateAll returns all validation errors, not just the first.
func (s *Status) ValidateAll() []error {
	var errs []error

	// Required fields
	if s.Version == 0 {
		errs = append(errs, &ValidationError{"v", ErrMissingVersion})
	} else if s.Version != SchemaVersion {
		errs = append(errs, &ValidationError{"v", ErrInvalidVersion})
	}

	if s.Agent == "" {
		errs = append(errs, &ValidationError{"agent", ErrMissingAgent})
	} else if !agentPattern.MatchString(s.Agent) {
		errs = append(errs, &ValidationError{"agent", ErrInvalidAgent})
	}

	if s.Instance == "" {
		errs = append(errs, &ValidationError{"instance", ErrMissingInstance})
	}

	if s.Status == "" {
		errs = append(errs, &ValidationError{"status", ErrMissingStatus})
	} else if !isValidStatus(s.Status) {
		errs = append(errs, &ValidationError{"status", ErrInvalidStatus})
	}

	if s.Updated == 0 {
		errs = append(errs, &ValidationError{"updated", ErrMissingUpdated})
	} else if s.Updated < 0 {
		errs = append(errs, &ValidationError{"updated", ErrInvalidUpdated})
	}

	// Optional field validation
	if s.PID < 0 {
		errs = append(errs, &ValidationError{"pid", ErrInvalidPID})
	}

	if s.Provider != "" && !isValidProvider(s.Provider) {
		errs = append(errs, &ValidationError{"provider", ErrInvalidProvider})
	}

	if s.CostUSD < 0 {
		errs = append(errs, &ValidationError{"cost_usd", ErrInvalidCost})
	}

	if s.Started < 0 {
		errs = append(errs, &ValidationError{"started", ErrInvalidStarted})
	}

	// Tools validation
	if s.Tools != nil {
		if len(s.Tools.Recent) > 10 {
			errs = append(errs, &ValidationError{"tools.recent", ErrTooManyRecent})
		}
		for name, count := range s.Tools.Counts {
			if count < 0 {
				errs = append(errs, &ValidationError{fmt.Sprintf("tools.counts.%s", name), ErrNegativeToolCount})
			}
		}
	}

	// Tokens validation
	if s.Tokens != nil {
		if s.Tokens.Input < 0 {
			errs = append(errs, &ValidationError{"tokens.input", ErrNegativeTokens})
		}
		if s.Tokens.Output < 0 {
			errs = append(errs, &ValidationError{"tokens.output", ErrNegativeTokens})
		}
		if s.Tokens.CacheRead < 0 {
			errs = append(errs, &ValidationError{"tokens.cache_read", ErrNegativeTokens})
		}
		if s.Tokens.CacheWrite < 0 {
			errs = append(errs, &ValidationError{"tokens.cache_write", ErrNegativeTokens})
		}
	}

	return errs
}

// IsValid returns true if the status is valid per the schema.
func (s *Status) IsValid() bool {
	return s.Validate() == nil
}

func isValidStatus(status string) bool {
	for _, s := range ValidStatuses {
		if s == status {
			return true
		}
	}
	return false
}

func isValidProvider(provider string) bool {
	for _, p := range ValidProviders {
		if p == provider {
			return true
		}
	}
	return false
}

// StatusDir returns the agent status directory.
func StatusDir() string {
	if dir := os.Getenv("AGENT_STATUS_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agent-status")
}

// ReadAll reads all agent status files.
// It skips invalid files silently (use ReadAllWithErrors for diagnostics).
func ReadAll(maxAge time.Duration) ([]Status, error) {
	statuses, _, err := ReadAllWithErrors(maxAge)
	return statuses, err
}

// ReadError represents an error reading a specific status file.
type ReadError struct {
	File string
	Err  error
}

func (e *ReadError) Error() string {
	return fmt.Sprintf("%s: %v", e.File, e.Err)
}

// ReadAllWithErrors reads all agent status files and returns both valid statuses
// and any errors encountered while reading/validating individual files.
func ReadAllWithErrors(maxAge time.Duration) ([]Status, []ReadError, error) {
	dir := StatusDir()

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	now := time.Now()
	var statuses []Status
	var readErrors []ReadError

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			readErrors = append(readErrors, ReadError{path, err})
			continue
		}

		var s Status
		if err := json.Unmarshal(data, &s); err != nil {
			readErrors = append(readErrors, ReadError{path, fmt.Errorf("invalid JSON: %w", err)})
			continue
		}

		if err := s.Validate(); err != nil {
			readErrors = append(readErrors, ReadError{path, fmt.Errorf("validation failed: %w", err)})
			continue
		}

		age := now.Sub(time.Unix(s.Updated, 0))
		if age > maxAge {
			continue
		}

		s.Age = age
		s.Stale = age > FreshThreshold
		s.File = path
		statuses = append(statuses, s)
	}

	// Sort by updated time (most recent first)
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Updated > statuses[j].Updated
	})

	return statuses, readErrors, nil
}

// Cleanup removes status files older than maxAge.
func Cleanup(maxAge time.Duration) error {
	dir := StatusDir()

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			os.Remove(path) // Remove unreadable files
			continue
		}

		var s Status
		if err := json.Unmarshal(data, &s); err != nil {
			os.Remove(path) // Remove malformed files
			continue
		}

		age := now.Sub(time.Unix(s.Updated, 0))
		if age > maxAge {
			os.Remove(path)
		}
	}

	return nil
}

// FormatTokens formats a token count for display.
func FormatTokens(count int) string {
	switch {
	case count >= 1_000_000:
		return formatFloat(float64(count)/1_000_000) + "M"
	case count >= 1_000:
		return formatFloat(float64(count)/1_000) + "k"
	default:
		return itoa(count)
	}
}

// FormatCost formats a cost in USD.
func FormatCost(cost float64) string {
	if cost < 0.01 {
		return "$" + formatFloat3(cost)
	}
	return "$" + formatFloat(cost)
}

func formatFloat(f float64) string {
	i := int(f * 10)
	if i < 10 {
		return "0." + string(rune('0'+i))
	}
	return itoa(i/10) + "." + string(rune('0'+i%10))
}

func formatFloat3(f float64) string {
	i := int(f * 1000)
	return "0." + string(rune('0'+i/100)) + string(rune('0'+(i/10)%10)) + string(rune('0'+i%10))
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
