package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"taterli-agent-chat/backend/internal/backend"
	"taterli-agent-chat/backend/internal/config"
	"taterli-agent-chat/backend/internal/server"
)

func TestStreamChat_Success(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_details\":[{\"text\":\"思\"}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_details\":[{\"text\":\"思考\"}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18888},
		Backends: []config.BackendConfig{{
			ID:             "minimax-main",
			Type:           "openai_compatible",
			BaseURL:        upstream.URL,
			APIKey:         "test",
			Model:          "MiniMax-M2.7",
			Temperature:    1.0,
			ReasoningSplit: true,
			Enabled:        true,
		}},
	}
	manager, err := backend.NewManager(cfg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := server.New(manager, "127.0.0.1", 18888)

	body := map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "你好"}},
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/chat/stream", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	respBody := w.Body.String()
	if !strings.Contains(respBody, `event: reasoning`) {
		t.Fatalf("missing reasoning event: %s", respBody)
	}
	if !strings.Contains(respBody, `{"delta":"思"}`) || !strings.Contains(respBody, `{"delta":"考"}`) {
		t.Fatalf("reasoning delta mismatch: %s", respBody)
	}
	if !strings.Contains(respBody, `event: content`) {
		t.Fatalf("missing content event: %s", respBody)
	}
	if !strings.Contains(respBody, `{"delta":"你"}`) || !strings.Contains(respBody, `{"delta":"好"}`) {
		t.Fatalf("content delta mismatch: %s", respBody)
	}
	if !strings.Contains(respBody, `event: done`) {
		t.Fatalf("missing done event: %s", respBody)
	}
}

func TestStreamChat_UpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18888},
		Backends: []config.BackendConfig{{
			ID:      "minimax-main",
			Type:    "openai_compatible",
			BaseURL: upstream.URL,
			APIKey:  "test",
			Model:   "MiniMax-M2.7",
			Enabled: true,
		}},
	}
	manager, err := backend.NewManager(cfg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	srv := server.New(manager, "127.0.0.1", 18888)

	body := map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "你好"}},
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/chat/stream", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	respBody := w.Body.String()
	if !strings.Contains(respBody, `event: error`) {
		t.Fatalf("missing error event: %s", respBody)
	}
	if !strings.Contains(respBody, `status=401`) {
		t.Fatalf("missing upstream error details: %s", respBody)
	}
}
