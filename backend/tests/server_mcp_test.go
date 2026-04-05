package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"taterli-agent-chat/backend/internal/backend"
	"taterli-agent-chat/backend/internal/config"
	"taterli-agent-chat/backend/internal/server"
)

func TestMCPGetSystemTime(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18888},
		Backends: []config.BackendConfig{{
			ID:      "mock-main",
			Type:    "openai_compatible",
			BaseURL: "http://127.0.0.1:1",
			APIKey:  "test",
			Model:   "mock",
			Enabled: true,
		}},
	}
	manager, err := backend.NewManager(cfg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := server.New(manager, "127.0.0.1", 18888)

	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "get_system_time",
			"arguments": map[string]any{},
		},
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/mcp", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v, body=%s", err, w.Body.String())
	}

	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result field: %v", resp)
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("missing structuredContent field: %v", result)
	}

	if _, ok := structured["now_unix"]; !ok {
		t.Fatalf("missing now_unix: %v", structured)
	}
	if _, ok := structured["now_rfc3339"]; !ok {
		t.Fatalf("missing now_rfc3339: %v", structured)
	}
	if _, ok := structured["timezone_name"]; !ok {
		t.Fatalf("missing timezone_name: %v", structured)
	}
}
