package backend

import (
	"testing"

	"taterli-agent-chat/backend/internal/config"
)

func TestManagerPickDefaultAndSpecific(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18888},
		Backends: []config.BackendConfig{
			{ID: "a", Type: "openai_compatible", BaseURL: "http://example.com", APIKey: "k", Model: "m", Enabled: true},
			{ID: "b", Type: "openai_compatible", BaseURL: "http://example.com", APIKey: "k", Model: "m", Enabled: true},
		},
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	def, err := m.Pick("")
	if err != nil {
		t.Fatalf("pick default: %v", err)
	}
	if def.ID() != "a" {
		t.Fatalf("expected default backend a, got %s", def.ID())
	}

	specific, err := m.Pick("b")
	if err != nil {
		t.Fatalf("pick specific: %v", err)
	}
	if specific.ID() != "b" {
		t.Fatalf("expected backend b, got %s", specific.ID())
	}
}

func TestManagerPickInvalid(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18888},
		Backends: []config.BackendConfig{
			{ID: "a", Type: "openai_compatible", BaseURL: "http://example.com", APIKey: "k", Model: "m", Enabled: true},
		},
	}

	m, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	if _, err := m.Pick("missing"); err == nil {
		t.Fatalf("expected error for missing backend")
	}
}
