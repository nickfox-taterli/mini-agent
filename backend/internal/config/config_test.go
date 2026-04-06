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
		Server: ServerConfig{Host: "127.0.0.1", Port: 18888},
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
		Server: ServerConfig{Host: "127.0.0.1", Port: 18888},
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
