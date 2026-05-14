// Package config handles persistent configuration from ~/.config/go-turing-smart-screens/config.json.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	dirName  = "go-turing-smart-screens"
	fileName = "config.json"
)

// Config holds all persistent configuration.
type Config struct {
	SignozURL    string `json:"signoz_url"`
	SignozAPIKey string `json:"signoz_api_key"`
}

// Load reads the config file, returning defaults if it doesn't exist.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return &Config{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// Save writes the config to disk.
func Save(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func configPath() (string, error) {
	// Try paths in order: user config, /etc, common home dirs
	candidates := []string{}

	if configDir, err := os.UserConfigDir(); err == nil {
		candidates = append(candidates, filepath.Join(configDir, dirName, fileName))
	}
	candidates = append(candidates,
		filepath.Join("/etc", dirName, fileName),
		filepath.Join("/home/aleks/.config", dirName, fileName),
	)

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// Default to user config dir for creation
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("get config dir: %w", err)
	}
	return filepath.Join(configDir, dirName, fileName), nil
}
