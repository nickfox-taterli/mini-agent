package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server        ServerConfig        `yaml:"server"`
	MinimaxTools  MinimaxToolsConfig  `yaml:"minimax_tools"`
	DockerRuntime DockerRuntimeConfig `yaml:"docker_runtime"`
	Backends      []BackendConfig     `yaml:"backends"`
}

type MinimaxToolsConfig struct {
	APIKeys []string `yaml:"api_keys"`
	APIHost string   `yaml:"api_host"`
}

type DockerRuntimeConfig struct {
	Enabled               bool    `yaml:"enabled"`
	Host                  string  `yaml:"host"`
	SessionTTLSeconds     int     `yaml:"session_ttl_seconds"`
	MaxLifetimeSeconds    int     `yaml:"max_lifetime_seconds"`
	DefaultTimeoutSeconds int     `yaml:"default_timeout_seconds"`
	MaxTimeoutSeconds     int     `yaml:"max_timeout_seconds"`
	MemoryLimit           string  `yaml:"memory_limit"`
	CPULimit              float64 `yaml:"cpu_limit"`
	PidsLimit             int     `yaml:"pids_limit"`
	WorkspaceRoot         string  `yaml:"workspace_root"`
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

	c.normalizeDockerRuntimeDefaults()
	if c.DockerRuntime.Enabled {
		if c.DockerRuntime.MaxLifetimeSeconds <= 0 {
			return fmt.Errorf("docker_runtime.max_lifetime_seconds must be positive")
		}
		if c.DockerRuntime.WorkspaceRoot == "" {
			return fmt.Errorf("docker_runtime.workspace_root is required when docker_runtime.enabled=true")
		}
		if filepath.IsAbs(c.DockerRuntime.WorkspaceRoot) {
			clean := filepath.Clean(c.DockerRuntime.WorkspaceRoot)
			c.DockerRuntime.WorkspaceRoot = clean
		} else {
			abs, err := filepath.Abs(c.DockerRuntime.WorkspaceRoot)
			if err != nil {
				return fmt.Errorf("resolve docker_runtime.workspace_root: %w", err)
			}
			c.DockerRuntime.WorkspaceRoot = abs
		}
		if c.DockerRuntime.DefaultTimeoutSeconds <= 0 || c.DockerRuntime.MaxTimeoutSeconds <= 0 {
			return fmt.Errorf("docker_runtime timeout settings must be positive")
		}
		if c.DockerRuntime.DefaultTimeoutSeconds > c.DockerRuntime.MaxTimeoutSeconds {
			return fmt.Errorf("docker_runtime.default_timeout_seconds must be <= docker_runtime.max_timeout_seconds")
		}
		if c.DockerRuntime.SessionTTLSeconds <= 0 {
			return fmt.Errorf("docker_runtime.session_ttl_seconds must be positive")
		}
		if c.DockerRuntime.PidsLimit <= 0 {
			return fmt.Errorf("docker_runtime.pids_limit must be positive")
		}
		if c.DockerRuntime.CPULimit <= 0 {
			return fmt.Errorf("docker_runtime.cpu_limit must be positive")
		}
	}

	return nil
}

func (c *Config) normalizeDockerRuntimeDefaults() {
	if c.DockerRuntime.Host == "" {
		c.DockerRuntime.Host = ""
	}
	if c.DockerRuntime.SessionTTLSeconds <= 0 {
		c.DockerRuntime.SessionTTLSeconds = 1800
	}
	if c.DockerRuntime.MaxLifetimeSeconds <= 0 {
		c.DockerRuntime.MaxLifetimeSeconds = 3600
	}
	if c.DockerRuntime.DefaultTimeoutSeconds <= 0 {
		c.DockerRuntime.DefaultTimeoutSeconds = 120
	}
	if c.DockerRuntime.MaxTimeoutSeconds <= 0 {
		c.DockerRuntime.MaxTimeoutSeconds = 600
	}
	if c.DockerRuntime.MemoryLimit == "" {
		c.DockerRuntime.MemoryLimit = "512m"
	}
	if c.DockerRuntime.CPULimit <= 0 {
		c.DockerRuntime.CPULimit = 1.0
	}
	if c.DockerRuntime.PidsLimit <= 0 {
		c.DockerRuntime.PidsLimit = 128
	}
	if strings.TrimSpace(c.DockerRuntime.WorkspaceRoot) == "" {
		c.DockerRuntime.WorkspaceRoot = filepath.Join("data", "docker-workspaces")
	}
}
