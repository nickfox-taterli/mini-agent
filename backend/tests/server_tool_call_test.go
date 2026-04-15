package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"taterli-agent-chat/backend/internal/backend"
	"taterli-agent-chat/backend/internal/config"
	"taterli-agent-chat/backend/internal/server"
)

func TestStreamChat_AutoCallMCPTool(t *testing.T) {
	var mu sync.Mutex
	var requestBodies []string
	round := 0

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		mu.Lock()
		requestBodies = append(requestBodies, string(b))
		round++
		currentRound := round
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		if currentRound == 1 {
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"get_system_time\",\"arguments\":\"{}\"}}]}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"系统时间已经获取完成\"}}]}\n\n"))
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
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889", false, "")

	body := map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "现在几点"}},
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
	if !strings.Contains(respBody, `event: content`) {
		t.Fatalf("missing content event: %s", respBody)
	}
	if !strings.Contains(respBody, `系统时间已经获取完成`) {
		t.Fatalf("missing final content: %s", respBody)
	}
	if !strings.Contains(respBody, `event: done`) {
		t.Fatalf("missing done event: %s", respBody)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requestBodies) != 2 {
		t.Fatalf("expected 2 upstream rounds, got %d", len(requestBodies))
	}
	if !strings.Contains(requestBodies[0], `"tools"`) || !strings.Contains(requestBodies[0], `"get_system_time"`) {
		t.Fatalf("first request missing tools definition: %s", requestBodies[0])
	}
	if strings.Contains(requestBodies[0], `"write_frontend_temp_file"`) || strings.Contains(requestBodies[0], `"minimax-xlsx"`) {
		t.Fatalf("first request still includes removed tools: %s", requestBodies[0])
	}
	if !strings.Contains(requestBodies[1], `"role":"tool"`) ||
		!strings.Contains(requestBodies[1], `"tool_call_id":"call_1"`) ||
		!strings.Contains(requestBodies[1], `"name":"get_system_time"`) ||
		!strings.Contains(requestBodies[1], `now_rfc3339`) {
		t.Fatalf("second request missing tool result context: %s", requestBodies[1])
	}
}

func TestStreamChat_AutoCallWebFetchTool(t *testing.T) {
	var mu sync.Mutex
	var requestBodies []string
	round := 0

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		mu.Lock()
		requestBodies = append(requestBodies, string(b))
		round++
		currentRound := round
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		if currentRound == 1 {
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_2\",\"type\":\"function\",\"function\":{\"name\":\"web_fetch\",\"arguments\":\"{\\\"url\\\":\\\"http://example.com\\\"}\"}}]}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"网页内容已获取\"}}]}\n\n"))
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
	srv := server.New(manager, "127.0.0.1", 18888, "http://127.0.0.1:18889", false, "")

	body := map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "抓取 example.com"}},
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
	if !strings.Contains(respBody, `event: content`) {
		t.Fatalf("missing content event: %s", respBody)
	}
	if !strings.Contains(respBody, `网页内容已获取`) {
		t.Fatalf("missing final content: %s", respBody)
	}
	if !strings.Contains(respBody, `event: done`) {
		t.Fatalf("missing done event: %s", respBody)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requestBodies) != 2 {
		t.Fatalf("expected 2 upstream rounds, got %d", len(requestBodies))
	}
	if !strings.Contains(requestBodies[0], `"tools"`) || !strings.Contains(requestBodies[0], `"web_fetch"`) {
		t.Fatalf("first request missing tools definition: %s", requestBodies[0])
	}
	if !strings.Contains(requestBodies[1], `"role":"tool"`) ||
		!strings.Contains(requestBodies[1], `"tool_call_id":"call_2"`) ||
		!strings.Contains(requestBodies[1], `"name":"web_fetch"`) {
		t.Fatalf("second request missing tool result context: %s", requestBodies[1])
	}
}
