package config

import "testing"

func TestValidatePort(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Host: "127.0.0.1", Port: 80},
		Backends: []BackendConfig{{
			ID:      "a",
			Type:    "openai_compatible",
			BaseURL: "http://example.com",
			APIKey:  "x",
			Model:   "m",
			Enabled: true,
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected low port validation error")
	}
}

func TestValidateEnabledBackend(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Host: "127.0.0.1", Port: 18888, FrontendURL: "http://127.0.0.1:18889"},
		Backends: []BackendConfig{{
			ID:      "a",
			Type:    "openai_compatible",
			BaseURL: "http://example.com",
			APIKey:  "x",
			Model:   "m",
			Enabled: false,
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected enabled backend validation error")
	}
}

func TestValidateToolMaxRounds(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Host: "127.0.0.1", Port: 18888, FrontendURL: "http://127.0.0.1:18889"},
		Backends: []BackendConfig{{
			ID:            "a",
			Type:          "openai_compatible",
			BaseURL:       "http://example.com",
			APIKey:        "x",
			Model:         "m",
			ToolMaxRounds: -1,
			Enabled:       true,
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected tool_max_rounds validation error")
	}
}

func TestValidateDockerRuntimeDefaults(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Host: "127.0.0.1", Port: 18888, FrontendURL: "http://127.0.0.1:18889"},
		Backends: []BackendConfig{{
			ID:      "a",
			Type:    "openai_compatible",
			BaseURL: "http://example.com",
			APIKey:  "x",
			Model:   "m",
			Enabled: true,
		}},
		DockerRuntime: DockerRuntimeConfig{Enabled: true},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate docker runtime defaults: %v", err)
	}
	if cfg.DockerRuntime.DefaultTimeoutSeconds != 120 {
		t.Fatalf("expected default timeout 120, got %d", cfg.DockerRuntime.DefaultTimeoutSeconds)
	}
	if cfg.DockerRuntime.MaxTimeoutSeconds != 600 {
		t.Fatalf("expected max timeout 600, got %d", cfg.DockerRuntime.MaxTimeoutSeconds)
	}
	if cfg.DockerRuntime.SessionTTLSeconds != 1800 {
		t.Fatalf("expected session ttl 1800, got %d", cfg.DockerRuntime.SessionTTLSeconds)
	}
}
