package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server      ServerConfig      `yaml:"server"`
	MinimaxTools MinimaxToolsConfig `yaml:"minimax_tools"`
	Backends    []BackendConfig   `yaml:"backends"`
}

type MinimaxToolsConfig struct {
	APIKeys []string `yaml:"api_keys"`
	APIHost string   `yaml:"api_host"`
}

type ServerConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	FrontendURL string `yaml:"frontend_url"`
}

type BackendConfig struct {
	ID             string  `yaml:"id"`
	Type           string  `yaml:"type"`
	BaseURL        string  `yaml:"base_url"`
	APIKey         string  `yaml:"api_key"`
	Model          string  `yaml:"model"`
	Temperature    float64 `yaml:"temperature"`
	ReasoningSplit bool    `yaml:"reasoning_split"`
	ToolMaxRounds  int     `yaml:"tool_max_rounds"`
	Enabled        bool    `yaml:"enabled"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.Server.Host == "" {
		return fmt.Errorf("server.host is required")
	}
	if c.Server.Port <= 1024 {
		return fmt.Errorf("server.port must be a high port (>1024)")
	}
	if len(c.Backends) == 0 {
		return fmt.Errorf("at least one backend is required")
	}

	seen := map[string]struct{}{}
	enabledCount := 0
	for _, b := range c.Backends {
		if b.ID == "" {
			return fmt.Errorf("backend id is required")
		}
		if _, ok := seen[b.ID]; ok {
			return fmt.Errorf("duplicate backend id: %s", b.ID)
		}
		seen[b.ID] = struct{}{}
		if b.ToolMaxRounds < 0 {
			return fmt.Errorf("backend %s tool_max_rounds must be >= 0", b.ID)
		}
		if b.Enabled {
			enabledCount++
		}
	}
	if enabledCount == 0 {
		return fmt.Errorf("at least one backend must be enabled")
	}
	if c.Server.FrontendURL == "" || !strings.HasPrefix(c.Server.FrontendURL, "http") {
		return fmt.Errorf("server.frontend_url is required and must start with http")
	}
	c.Server.FrontendURL = strings.TrimRight(c.Server.FrontendURL, "/")

	return nil
}
