// Package agentstat provides reading of agent status files.
package agentstat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Status represents an agent's current status.
type Status struct {
	Version  int               `json:"v"`
	Agent    string            `json:"agent"`
	Instance string            `json:"instance"`
	PID      int               `json:"pid,omitempty"`
	Project  string            `json:"project,omitempty"`
	CWD      string            `json:"cwd,omitempty"`
	Status   string            `json:"status"`
	Task     string            `json:"task,omitempty"`
	Model    string            `json:"model,omitempty"`
	Provider string            `json:"provider,omitempty"`
	Tools    *ToolsInfo        `json:"tools,omitempty"`
	Tokens   *TokensInfo       `json:"tokens,omitempty"`
	CostUSD  float64           `json:"cost_usd,omitempty"`
	Started  int64             `json:"started,omitempty"`
	Updated  int64             `json:"updated"`
	Error    string            `json:"error,omitempty"`
	Context  map[string]any    `json:"context,omitempty"`

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

// StatusDir returns the agent status directory.
func StatusDir() string {
	if dir := os.Getenv("AGENT_STATUS_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agent-status")
}

// ReadAll reads all agent status files.
func ReadAll(maxAge time.Duration) ([]Status, error) {
	dir := StatusDir()
	
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var statuses []Status

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var s Status
		if err := json.Unmarshal(data, &s); err != nil {
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

	return statuses, nil
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
